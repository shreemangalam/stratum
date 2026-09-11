# Technical Design

This document describes how Stratum is built. Every section maps to a
requirement in the functional design. If a component described here does
not serve a functional requirement, it is decoration and should be removed.

---

## 1. System Architecture

Single server binary. Single database. No message brokers, no caches
outside the application, no microservice splits.

```
                    ┌─────────────────────┐
                    │   Next.js Frontend  │
                    │   (web/)            │
                    └────────┬────────────┘
                             │ HTTP/SSE
                    ┌────────▼────────────┐
                    │   Go API Server     │
                    │   (server/)         │
                    └────────┬────────────┘
                             │ SQL
                    ┌────────▼────────────┐
                    │   PostgreSQL        │
                    │   (docker-compose)  │
                    └─────────────────────┘
```

## 2. Go Server Structure

```
server/
  cmd/api/main.go           Entry point, wiring
  internal/
    core/                   Algorithm, pure, no I/O
      tree.go               Tree and Node data types
      hash.go               Merkle hashing for content-addressed identity
      match.go              Top-down + bottom-up matching
      editscript.go         Edit script generation
      semantic.go           Semantic classification (heuristic + dataflow)
      dataflow.go           Def/use extraction and statement independence
      budget.go             Cost estimation and bounded computation
    parse/                  Parser plugin interface and registry
      plugin.go             Plugin interface definition
      registry.go           Plugin registry
      treesitter/           Language parsers (Go full AST, structural scanners, line fallback)
        go.go               Full AST via go/parser stdlib
        treesitter.go       Generic parser dispatcher and line-based fallback
        scanner.go          Shared scanner utilities for structural parsers
        javascript.go       JS structural scanner (functions, classes, imports)
        python.go           Python structural scanner (def, class, import)
        java.go             Java structural scanner (class, interface, enum, method)
        c.go                C/C++ structural scanner (functions, structs, enums, typedefs)
      xslt/                 XSLT/XML-specific parser (pure Go, encoding/xml)
    cache/                  Content-addressed cache
      cache.go              SHA-256 keyed, TTL-based
    jobs/                   Async job execution
      worker.go             Bounded worker pool
    store/                  Postgres persistence
      store.go              Repository interface
      postgres.go           Implementation
    http/                   HTTP layer
      server.go             Router, middleware
      handlers.go           Request handlers
      generated/            Types generated from OpenAPI (oapi-codegen), do not edit
  migrations/               SQL migrations, versioned
```

### 2.1 The `internal/core/` Boundary

This is the most important architectural constraint. `core/` contains the
matching algorithm and tree data structures. It imports nothing outside the
standard library. No database, no HTTP, no file I/O.

This means:
- The algorithm can be tested with hand-built trees, no fixtures.
- The algorithm can be benchmarked in isolation.
- A future CLI tool reuses `core/` without importing the server.

If a function in `core/` needs data from the database, it takes that data
as a parameter. If it needs to emit progress, it takes a callback.

### 2.2 Dependency Injection

No global state. Every component receives its dependencies through its
constructor. The `cmd/api/main.go` entry point wires everything together.

```go
func main() {
    db := store.NewPostgres(cfg.DatabaseURL)
    cache := cache.New(cfg.CacheTTL)
    registry := parse.NewRegistry()
    // register parsers
    registry.Register(treesitter.New())
    registry.Register(xslt.New())

    workers := jobs.NewPool(cfg.WorkerCount, db, cache, registry)
    srv := http.NewServer(db, workers)
    srv.ListenAndServe(cfg.Addr)
}
```

## 3. The Matching Algorithm

Three phases, executed in sequence on the two parse trees.

### 3.1 Phase 1: Top-Down Hash Matching

Compute a Merkle hash for every node in both trees. The hash of a node is
derived from:
- Its kind (e.g., `function_declaration`, `if_statement`).
- Its label (e.g., function name, variable name).
- Its literal value (for leaf nodes).
- The ordered hashes of its children.

Nodes with identical hashes are matched immediately, and a matched pair
maps its entire subtree as a unit: identical hashes guarantee identical
shape, so the descendant mapping is forced. This captures all unchanged
subtrees in O(n) time.

Left nodes are processed largest-subtree-first (stable within equal
sizes, so document order and determinism are preserved). A whole
unchanged function must claim its statements before a smaller orphaned
statement elsewhere can grab a hash-identical copy inside it; without
this ordering, files with repetitive bodies produce phantom cross-
function matches. Among ambiguous candidates, the one at the same child
index is preferred, then document order.

**Rationale:** Identical subtrees are the common case in a typical diff.
Matching them first reduces the problem size for the expensive phases.

### 3.2 Phase 2: Bottom-Up Dice Matching

For unmatched nodes, compute a similarity score based on the fraction of
their descendants that are already matched (the dice coefficient).

Starting from leaf nodes and working up:
1. For each unmatched node in the left tree, find candidate nodes of the
   same kind in the right tree.
2. Compute the dice coefficient: `2 * |matched_descendants| / (|left_descendants| + |right_descendants|)`.
3. If the score exceeds a threshold (configurable, default 0.6), match the
   pair with the highest score.

**Threshold rationale:** 0.6 balances false positives (matching unrelated
nodes) against false negatives (failing to match restructured nodes).
This is a tuning parameter exposed in configuration, not a constant.

### 3.3 Phase 3: Recovery Inside Matched Pairs

For every pair matched by phases 1-2, align the still-unmatched children
and recurse into the pairs that creates. Recovery never reaches outside
a matched pair, so an unmatched subtree stays whole — one clean insert
or delete — instead of donating leaves to distant lookalikes. Three
passes per pair, all bounded by the node budget:

1. LCS over child kinds, anchored by children already matched to each
   other (weighted by subtree size).
2. Leftovers paired by exact kind + label, even when crossed — the LCS
   is order-preserving, so a reordered sibling can never align in pass
   1. The reordering itself is reported by the edit script as a move.
3. Remaining leftovers of the same kind paired by leaf-content
   similarity (dice over leaf kind/label/value multisets) above the
   threshold. This is what lets a function that was renamed *and*
   edited surface as a rename plus inner edits rather than
   delete + insert.

Bottom-up matching (phase 2) is restricted to containers because leaves
carry no descendant evidence; recovery is where leaves match, and only
ever within a matched parent.

**Budget rationale:** Optimal tree alignment is O(n^2 * m^2) in the worst
case. The budget prevents this from dominating total computation time.
Subtrees exceeding the budget skip residual alignment, leave unmatched
nodes as insertions/deletions, and are marked approximate.

### 3.4 Bounded Computation

Before starting Phase 3 on a subtree, estimate the cost:
- If `left_nodes * right_nodes > budget^2`, skip residual alignment.
- Mark the region as approximate in the edit script.
- The UI renders an indicator so the user knows.

The budget is a server configuration parameter, not exposed to users.

## 4. Edit Script Generation

Given the matching, generate the edit script:

1. **Deletions:** top-most left nodes with no match; the subtree is
   implied, one operation per deleted subtree.
2. **Insertions:** top-most right nodes with no match, same rule.
3. **Moves:** two cases. Cross-parent: a matched pair whose parents are
   not matched to each other (children of a moved node have consistent
   parent pairs, so only the relocated root reports). Reordering: among
   the matched children of a pair, the longest increasing subsequence of
   right-side positions — weighted by subtree size — is the stable
   backbone; every pair outside it moved. An insertion shifts positions
   without breaking relative order, so it flags nothing; a swap costs
   exactly one move (the smaller sibling).
4. **Renames:** Matched nodes whose label (identifier) changed.
5. **Updates:** Matched nodes whose literal value changed.

Move detection is the key differentiator. A function that moved from line
20 to line 150 produces one `move` operation, not a delete+insert pair.

### 4.0.1 Semantic Classification

Every matched function pair touched by at least one operation gets a
verdict — `behavior-preserving`, `behavior-changing`, or `indeterminate`
— derived first from the shape of its edit script, then refined by
dataflow analysis for languages with full AST support.

**Heuristic phase** (all languages):
- Consistent renames only (old->new names form a bijection): preserving.
- Relocated with no inner edits: preserving.
- Signature touched (parameter/result nodes in any operation) or body
  edited (inserts, deletes, updates): changing.
- Statements reordered, nothing else: indeterminate — passed to the
  dataflow refinement phase.

**Dataflow refinement phase** (Go only, requires full AST):
When the heuristic returns "statements reordered," `ClassifyReordering`
in `dataflow.go` performs def/use analysis on the function body. See
§4.0.2 for details. This upgrades indeterminate verdicts to either
preserving or changing when the analysis can determine independence.

Verdicts ride on the edit script as `semantic` and render in the UI
result header as clickable pills.

### 4.0.2 Dataflow Analysis

`dataflow.go` implements lightweight def/use set extraction to classify
statement reorderings without full interprocedural analysis. It lives in
`internal/core/` and imports only the standard library.

**Def/use extraction.** `ExtractDefUse` walks a statement's AST subtree.
Identifiers under `assignment_lhs` wrapper nodes are defs (writes);
all other identifiers are uses (reads). Nodes that imply side effects
set `HasSideEffects`:
- `call_expression`, `go_statement`, `defer_statement`: function calls
  may have arbitrary side effects.
- `return_statement`: alters control flow.
- `if_statement`, `for_statement`, `range_statement`, `switch_statement`,
  `case_clause`: control flow implies ordering dependence.

**Independence check.** `DefUseSetsIndependent` tests two def/use sets
for data conflicts: write-write (WAW), write-read (RAW), read-write
(WAR). Shared reads (use-use) are not conflicts.

**Reordering classification.** `ClassifyReordering` is called when the
heuristic phase returns "statements reordered." It:
1. Checks the language is Go (structural scanners lack statement nodes).
2. Finds the function body block in both trees.
3. Bounds-checks: blocks with >50 statements fall back to indeterminate.
4. Verifies all moved statements are direct children of the body block.
5. Builds position maps and extracts def/use sets for each statement.
6. For every pair of matched statements whose relative order reversed,
   checks side effects and data independence.

Three outcomes:
- All reversed pairs are pure and independent → `behavior-preserving`.
- Any reversed pair has a data conflict → `behavior-changing`.
- Any reversed pair has side effects or the analysis cannot proceed →
  `indeterminate` (unchanged from the heuristic).

**Parser support.** The Go parser wraps assignment left-hand sides in
`assignment_lhs` and right-hand sides in `assignment_rhs` nodes,
giving `ExtractDefUse` a clean boundary between defs and uses. The
assignment statement's `Label` carries the operator (`:=` or `=`).
See ADR-0002 for the design rationale.

### 4.1 Edit Script Data Model

```go
type EditScript struct {
    Operations []Operation
    LeftRoot   NodeID
    RightRoot  NodeID
    Approximate []Region  // subtrees whose residual alignment was skipped
}

type Operation struct {
    Kind      OpKind     // insert, delete, move, rename, update, align
    LeftNode  *NodeRef   // nil for insert
    RightNode *NodeRef   // nil for delete
    Details   OpDetails  // kind-specific payload
}

type NodeRef struct {
    ID       NodeID
    Path     string     // e.g., "function_declaration > block > if_statement"
    Kind     string     // node kind (e.g., function_declaration, if_statement)
    Label    string     // identifier label, if any
    Location Location   // line, column, byte offset
}
```

This is the contract between the backend and frontend. Design it once,
version it in the OpenAPI spec, don't churn.

## 5. Parser Plugin Interface

```go
type Parser interface {
    // Parse transforms source bytes into a tree.
    Parse(ctx context.Context, source []byte) (*core.Tree, error)

    // Language returns the identifier (e.g., "go", "xslt").
    Language() string

    // Extensions returns associated file extensions (e.g., [".go"]).
    Extensions() []string
}
```

The registry maps file extensions to parsers. The matcher never knows which
parser produced the tree.

### 5.1 Parser Implementation Details

**Go parser** (`treesitter/go.go`): Full AST via the stdlib `go/parser`.
Produces a complete tree with every statement, expression, and declaration
as a node. This is the highest-fidelity parser and serves as the reference
for matching algorithm behavior.

**Structural scanners** (`treesitter/javascript.go`, `python.go`, `java.go`,
`c.go`): Hand-written scanners that identify top-level declarations
(functions, classes, structs, enums, imports) without producing a full AST.
All share a common scanner utility (`treesitter/scanner.go`) providing:
- `skipWhitespaceAndComments()` — skips whitespace, `//` line comments,
  and `/* */` block comments.
- `skipBalanced(open, close)` — skips balanced brace/bracket/paren pairs.
- `readWord()` — reads an identifier.
- `matchWord(word)` — peeks whether the current position starts with a
  specific keyword (without consuming it).
- `skipString(quote)` — skips a string literal with escape handling.

Each structural scanner follows the same pattern: iterate through the
source, recognize declarations by keyword patterns, skip over their bodies
with `skipBalanced`, and emit nodes with kind and label. The internal
code within function bodies is treated as a single text value, not parsed
into sub-nodes.

**Line-based fallback** (`treesitter/treesitter.go`): For languages without
a dedicated parser, each non-blank line becomes a node. Trivial lines
(empty lines, lines containing only braces `{}`, brackets `[]`, parentheses
`()`, or punctuation) are filtered out to reduce false-positive matches.

**XSLT parser** (`xslt/`): Pure Go over `encoding/xml`. Structure-aware
choices, each one a matching decision:

- The node kind is the element name (`xsl:template`, `map`) — matching
  never pairs unrelated elements, and the XSLT namespace is normalized
  to the `xsl:` prefix regardless of the prefix the document declared.
- The label is the element's identity attribute (`name`, `match`, or
  `id` in priority order) — a renamed template reports as a rename.
- Attributes are child nodes sorted by attribute name: XML attribute
  order is semantically meaningless, so a pure reorder hashes
  identically and produces an empty diff.
- Text and comments are leaf nodes; whitespace-only text is dropped.
- `xsl:template` is registered as XSLT's function-level kind, so
  templates get semantic verdicts like Go functions do.

### 5.2 Language Auto-Detection

`server/internal/parse/detect.go` examines the first 500 characters of
input for language-specific patterns:

| Pattern                              | Detected Language |
|--------------------------------------|-------------------|
| `package ... func`                   | Go                |
| `<?xml` or `<xsl:`                   | XSLT / XML        |
| `#include` or `#define`              | C / C++           |
| `def ... :` or `import ... from`     | Python            |
| `public class` or `interface ... {`  | Java              |
| Type annotations (`: string`, etc.)  | TypeScript        |
| `function`, `const`, `=>`, `require` | JavaScript        |

Detection also uses file extensions when submitted via the Git diff
endpoint (`.go`, `.xml`, `.xslt`, `.js`, `.ts`, `.py`, `.java`, `.c`,
`.cpp`, `.h`).

### 5.3 Golden Tests

`internal/parse/testdata/<language>/<case>/` holds `before.*`,
`after.*`, and `expected.json` — the full edit script JSON for the
pair. The test runs the real parse -> match -> edit script pipeline.
Regenerating goldens requires `go test -run TestGolden -update`, so a
behavior change shows up in review as a diff to `expected.json`.
Deterministic matching (document-order traversal everywhere) is what
makes byte-exact goldens possible.

## 6. Content-Addressed Cache

- Key: SHA-256 of input source bytes.
- Stored: parse results and diff results.
- Diff cache key: sorted pair of source hashes.
- TTL: configurable, default 24 hours.
- Storage: in-memory with Postgres overflow for persistence across
  restarts.

```go
type Cache interface {
    GetParseResult(ctx context.Context, hash string) (*core.Tree, bool)
    PutParseResult(ctx context.Context, hash string, tree *core.Tree) error
    GetDiffResult(ctx context.Context, leftHash, rightHash string) (*core.EditScript, bool)
    PutDiffResult(ctx context.Context, leftHash, rightHash string, script *core.EditScript) error
}
```

## 7. Async Jobs

### 7.1 Job Queue

Postgres-backed. A job row tracks:
- ID (UUID).
- Status: `pending`, `running`, `completed`, `failed`.
- Input hashes.
- Result (JSON edit script on completion).
- Created/updated timestamps.
- Idempotency key (derived from sorted input hashes).

### 7.2 Worker Pool

Bounded pool of N goroutines (configurable, default 4). Each worker:
1. Claims a pending job with `SELECT ... FOR UPDATE SKIP LOCKED`.
2. Runs the parse-match-editscript pipeline.
3. Writes the result back to the job row.

### 7.3 SSE Streaming

Clients subscribe to job progress via Server-Sent Events:
- `status` events: pending, running, completed, failed.
- `progress` events: phase name and percentage (coarse-grained).
- `result` event: the edit script on completion.
- `error` event: failure message on failed jobs.

Ordering contract: payload events (`result`, `error`) are emitted before
their terminal `status` event (`completed`, `failed`). The stream handler
closes the connection on a terminal status, so a payload emitted after it
would be lost.

Race handling: the stream handler subscribes to the job's event channel
first, then re-reads the job row. If the job reached a terminal state
before the subscription, the stored result or error is replayed directly
from the row; otherwise events arrive via the channel. Without the
re-read, a job completing between the initial status check and the
subscription would leave the stream open forever.

## 8. HTTP API

Contract-first. `contract/openapi.yaml` is the source of truth. Go types
and TypeScript types are generated from it via `oapi-codegen` and
`openapi-typescript`. Handlers are hand-written but must conform to the
generated type shapes. CI fails if generated files are stale.

### 8.1 Endpoints (v1)

| Method | Path                        | Purpose                            |
|--------|-----------------------------|------------------------------------|
| POST   | /api/v1/diffs               | Submit a diff request (paste mode) |
| GET    | /api/v1/diffs/{id}          | Get diff result by job ID          |
| GET    | /api/v1/diffs/{id}/stream   | SSE stream for job progress        |
| POST   | /api/v1/diffs/git           | Submit a git diff request          |
| GET    | /api/v1/git/files           | List changed files between git refs|
| GET    | /api/v1/languages           | List supported languages           |
| GET    | /api/v1/health              | Health check                       |

### 8.2 Git Diff Endpoints

These routes are registered only when `GIT_ENABLED=true`. They are disabled
by default because they execute Git against repositories on the API host's
filesystem. A public deployment keeps them disabled; the frontend Git tab is
likewise controlled at build time by `NEXT_PUBLIC_GIT_ENABLED`.

**POST /api/v1/diffs/git** accepts:
```json
{
  "repo_path": "/absolute/path/to/repo",
  "left_ref": "HEAD~1",
  "right_ref": "HEAD",
  "file_path": "src/main.go",
  "language": "go"
}
```

The server validates that `repo_path` is absolute and contains a `.git`
directory, then executes `git show <ref>:<file_path>` to extract file
contents at each ref. The extracted content is fed through the normal
diff pipeline. Language is auto-detected from the file extension if not
provided; unrecognized extensions fall back to line-based comparison.

**POST /api/v1/git/files** accepts:
```json
{
  "repo_path": "/absolute/path/to/repo",
  "left_ref": "HEAD~1",
  "right_ref": "HEAD"
}
```

Executes `git diff --name-status <left_ref> <right_ref>` in the repo
directory. Returns `{"files": [{path, status}]}` where status is one
of `added`, `deleted`, `modified`, `renamed`.

Security: `repo_path` must be an absolute path to a directory containing
`.git`. Ref and file path parameters are passed as `exec.Command`
arguments, never interpolated into shell strings. File paths reject `..`
traversal.

### 8.3 Idempotency

POST /api/v1/diffs is idempotent on content. If a diff for the same pair
of source hashes already exists (completed or in progress), return the
existing job ID instead of creating a new one.

### 8.4 Error Responses

All errors return JSON with a consistent shape:
```json
{
  "error": "Human-readable error message"
}
```

Status codes:
- 400: Invalid input (missing fields, bad language, invalid ref).
- 413: Request body too large (>1 MB per request).
- 404: Diff not found.
- 500: Internal server error.

### 8.5 Structured Logging

Every request gets a correlation ID (X-Request-ID header or generated).
All log entries include the correlation ID. Structured JSON format. No
distributed tracing — correlation IDs through a single service are enough.

## 9. Frontend

Next.js with TypeScript in strict mode. Server components by default.
Client components only for interactive elements (the diff view, forms).

### 9.1 Page Structure

| Route              | Component          | Type    | Purpose                       |
|--------------------|--------------------|---------|-------------------------------|
| `/`                | `app/page.tsx`     | Client  | Main page with tab forms      |
| `/diffs/[id]`      | `app/diffs/[id]/page.tsx` | Client | Diff result view       |
| `/help`            | `app/help/page.tsx`| Server  | Documentation page            |

### 9.2 Main Page (/)

The form always includes "Paste code". It includes the "Git diff" tab only
when `NEXT_PUBLIC_GIT_ENABLED=true` (or during local frontend development).

**Paste code tab:**
- Left and right textarea editors with a Shiki-highlighted backdrop.
- Language dropdown (auto-detect or manual selection).
- File upload via drag-drop or file picker.
- Submit via button or Ctrl+Enter.

**Git diff tab:**
- Repository path input (auto-focused).
- From ref and To ref inputs (default: HEAD~1 and HEAD).
- "List files" button loads changed files.
- File tree displays in a folder-grouped hierarchy:
  - `buildFileTree()` constructs a `FolderNode` tree from flat paths.
  - `flattenSingleChildDirs()` collapses `a/b/c/` chains into `a/b/c/`.
  - Each file shows its git status letter (A/D/M/R) with color coding.
  - Folders show file counts and are collapsible.
- Enter key: loads files if not loaded; submits if file selected.
- Empty state when no files changed.

**API health check:** On mount, the page pings `/api/v1/health` with a
3-second timeout. If unreachable, a banner with a retry button appears.

### 9.3 Diff View Architecture

The diff view (`app/diffs/[id]/page.tsx`) is a client component with
these sub-components:

- **DiffPanes:** Two virtualized panels (left/right) rendering source code.
  Each row is a `DiffRow` component that memoizes on the row data.
- **GutterColumn:** Line numbers per pane.
- **SyntaxLayer:** Syntax highlighting via Shiki (WASM-based), loaded on
  demand after the initial render to prevent flash-of-unstyled-text. The
  first render shows plain code; Shiki tokens overlay once loaded.
- **DiffHighlightLayer:** Background tinting for changed regions. Color-
  coded left-edge bars indicate operation type (green=insert, red=delete,
  orange=move, blue=rename/update).
- **InlineDiffHighlight:** Word-level token diff within changed lines.
  Uses a longest-common-subsequence algorithm on whitespace-split tokens.
- **MoveArrowOverlay:** SVG overlay connecting moved nodes across panes.
  Renders orange cubic Bezier curves. Hover to highlight both endpoints.
- **CollapsedRegion:** Collapsed unchanged regions showing line count and
  3 lines of context. Click to expand.
- **ResultHeader:** Operation count summary with colored dots, language
  label, copy-link and copy-summary buttons.
- **ChangeSummary:** Plain-language list of structural changes.
- **SemanticVerdicts:** Clickable pills scrolling to relevant code.

### 9.4 Virtualization

Both panes virtualize rows. Only visible rows plus a buffer are rendered.
The move arrow overlay must resolve endpoints through:
- Virtualized (off-screen) rows: arrows point to the pane edge with a
  label indicating the target line.
- Collapsed regions: arrows route around or terminate at the collapsed
  row indicator.

### 9.5 Scroll Sync

The scroll anchor is the topmost fully visible matched node in the left
pane. When the left pane scrolls, the right pane scrolls to keep the
matched node aligned. Users can break sync temporarily by scrolling the
right pane independently; sync re-engages on the next left-pane scroll.

### 9.6 Error States

The diff result page handles these error states:
- **Loading:** Spinner with "Loading diff..." message.
- **Network error:** Warning icon, "Cannot reach the server" message,
  Retry and New diff buttons. `retryCount` state triggers re-fetch.
- **Application error:** X icon with server error message, same buttons.
- **Expired sources:** Hourglass icon with "Sources are cleaned up after
  processing" hint, New diff button.

### 9.7 API Client

`web/lib/api/client.ts` wraps all API calls:
- `safeFetch()` catches TypeError (network unreachable) and returns a
  friendly error message.
- `parseErrorBody()` extracts `error.error` from JSON responses.
- 413 status returns "Files are too large - maximum 1MB per file".
- All functions: `createDiff`, `getDiff`, `getLanguages`, `createGitDiff`,
  `getGitFiles`.

### 9.8 Generated Types

`web/lib/api/generated/schema.ts` is generated from the OpenAPI spec by
`openapi-typescript`. Never edited by hand. The handwritten API client in
`web/lib/api/client.ts` defines its own types that must stay consistent
with the generated schema; CI fails on drift.

### 9.9 CSS Architecture

All styles in `web/app/globals.css`. No CSS modules, no Tailwind.
CSS custom properties for all colors (dark theme by default):
- `--background`, `--foreground`, `--border`, etc.
- `--diff-add`, `--diff-remove`, `--diff-move`, `--diff-change` for
  diff-specific colors.
- `--shiki-*` tokens for syntax highlighting colors.

Responsive layout with `max-width: 1400px` content area.

## 10. Database

PostgreSQL. Schema managed by versioned SQL migrations in
`server/migrations/`.

### 10.1 Tables (v1)

- **jobs:** Async job tracking (id, status, input_hashes, result,
  timestamps, left_source, right_source for permalink storage).
- **cache_entries:** Persistent cache overflow (hash, kind, data,
  expires_at).

No user tables in v1. No authentication tables. Keep the schema minimal.

### 10.2 Migrations

Sequential numbered SQL files. Applied with a migration runner at server
startup. Forward-only; rollback is a new migration.

## 11. Testing Strategy

### 11.1 Unit Tests (internal/core/)

Hand-built trees. Test the algorithm on known inputs with known expected
outputs. No file I/O, no database, no fixtures larger than a few dozen
nodes.

Examples:
- Identical trees produce empty edit script.
- Single node insertion detected.
- Function move detected as one `move` operation.
- Budget exceeded triggers approximate flag.

### 11.2 Parser Tests (internal/parse/)

Each structural scanner has dedicated tests:
- **Go:** TestParseGo_BasicFunction, TestParseGo_ExtractsLabels,
  TestParseGo_StructAndInterface, TestParseGo_IfStatement.
- **Python:** TestParsePython_StructuralParser (functions with labels).
- **JavaScript:** TestParseJavaScript_FunctionsAndClasses (functions,
  classes with methods, arrow functions).
- **Java:** TestParseJava_ClassWithMethods (package, import, class with
  constructor and methods).
- **C/C++:** TestParseC_StructuralParser (includes, macros, structs,
  enums, function definitions with name verification).
- **Line fallback:** TestParseGeneric_LineBasedFallback (uses Lua to
  verify line-based parsing for unsupported languages).

### 11.3 Integration Tests (server/)

Use `testcontainers-go` for a real Postgres instance. Test the HTTP API
end-to-end: submit a diff, poll for completion, verify the result.

### 11.4 Golden Tests

21 golden test cases across 8 languages. For each supported language,
a `testdata/<language>/<case>/` directory with:
- `before.ext` — original source.
- `after.ext` — modified source.
- `expected.json` — expected edit script.

Regenerating goldens requires an explicit flag (`go test -run TestGolden -update`).
Languages with golden tests: Go (5 cases), XSLT (2), JavaScript (3),
Python (3), Java (2), C (2), C++ (2), TypeScript (2). Edge cases include
identical files (zero-op) and empty-to-content transitions.

### 11.5 Benchmark Tests

Core algorithm benchmarks (`internal/core/bench_test.go`): synthetic trees
at 10, 40, 120, 500, and 1500 nodes. Dedicated benchmarks for hash
computation and identical-tree fast path.

Full pipeline benchmarks (`internal/parse/treesitter/bench_test.go`):
Go source parsing + matching + edit script at 5, 20, 50, and 100 functions.
Run with `go test -bench=. -benchmem`.

### 11.6 Fuzz Tests

10 fuzz targets covering all parser paths and the core matching algorithm:
- Go, JavaScript, TypeScript, Python, Java, C, C++, generic line-based
  (`internal/parse/treesitter/fuzz_test.go`)
- XSLT (`internal/parse/xslt/fuzz_test.go`)
- Match + EditScript on arbitrary trees (`internal/core/fuzz_test.go`)

Invariant: Parse and Match never panic on any input.
Run with `go test -fuzz=FuzzParseGo -fuzztime=30s`.

### 11.7 End-to-End Tests

14 Playwright tests in `web/e2e/`:
- `homepage.spec.ts`: tab rendering, API banner, text areas, language
  selector, diff button, tab switching.
- `help.spec.ts`: heading, navigation, content.
- `diff-form.spec.ts`: typing, submission, language dropdown, clear,
  API retry.

Run with `npm run test:e2e` from the `web/` directory. Tests verify
frontend-only interactions; full-stack tests (diff submission, move
arrows, scroll sync) require the Go backend.

### 11.8 Evaluation Corpus

`TestEvalCorpus` runs the full pipeline on all golden test cases and
produces descriptive metrics: match coverage, operation counts, and semantic
verdicts. Run with `STRATUM_EVAL=1 go test -run TestEvalCorpus` in
`internal/parse/`. Results are written to
`docs/private/interview-prep/eval-corpus-results.json`.

This corpus is a regression/evaluation fixture, not a latency benchmark.
Performance is measured separately with Go benchmarks so sub-millisecond cases
are repeated enough to produce meaningful timings.

## 12. Configuration

Single configuration source: environment variables. Loaded once at startup.

| Variable             | Default    | Purpose                       |
|---------------------|------------|-------------------------------|
| `PORT`              | `8080`     | HTTP listen port              |
| `DATABASE_URL`      | (required) | Postgres connection string    |
| `WORKER_COUNT`      | `4`        | Job worker pool size          |
| `CACHE_TTL`         | `24h`      | Cache entry time-to-live      |
| `NODE_BUDGET`       | `100`      | Max nodes for optimal alignment |
| `DICE_THRESHOLD`    | `0.6`      | Bottom-up matching threshold  |
| `LOG_LEVEL`         | `info`     | Structured log level          |

Default development database URL:
`postgres://stratum:stratum@localhost:5433/stratum?sslmode=disable`

## 13. Build and Deploy

### 13.1 Development

- `make setup` — install tools, configure hooks.
- `make dev` — start Postgres, run server and frontend in dev mode.
- `make test` — run all test tiers.
- `make gen` — regenerate from OpenAPI.
- `make lint` — run all linters.
- `docker-compose up` — Postgres on port 5433.
- Go API server on port 8080.
- Next.js frontend on port 3000.

### 13.2 Docker

Both services have production Dockerfiles with multi-stage builds:

- **`server/Dockerfile`**: Go 1.25 builder -> Alpine with `ca-certificates`
  and `git`. Static binary (~15 MB), non-root user, health check.
- **`web/Dockerfile`**: Node 22 Alpine, Next.js standalone output. Copies
  only the traced files needed at runtime, non-root user.

Full-stack deployment:

```bash
docker compose --profile full up --build
```

This starts Postgres, the Go API, and the Next.js frontend. Without
`--profile full`, only Postgres starts (development mode).

### 13.3 Production Deployment

The Dockerfiles produce self-contained images deployable to any container
host (Fly.io, Railway, Cloud Run, ECS). Environment variables control
all configuration — no config files to mount.

### 13.3 Development Prerequisites

- Go 1.25+
- Node.js 18+ with npm
- Docker (for Postgres)
- No C toolchain required — all parsers are pure Go

## 14. Security Considerations

- Git operations validate that `repo_path` is an absolute path and that
  the directory contains a `.git` directory.
- Ref parameters are passed as arguments to `git` commands, not
  interpolated into shell strings.
- File paths are validated to reject `..` traversal and leading slashes.
- Request body limit enforced at 1 MB per request (413 response).
- No user authentication in v1 — the tool is designed for local or
  trusted-network use.
- CORS configured for development (localhost origins).
- No secrets stored in the database.
