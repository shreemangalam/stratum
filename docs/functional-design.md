# Functional Design

This document defines what Stratum does from the user's perspective. Every
feature described here has a corresponding section in the technical design.
If a feature isn't in this document, it doesn't ship.

---

## 1. Problem

Line-based diffs fail on structured documents. Moving a function 200 lines
down shows as a deletion and an addition, not a move. Renaming a variable
shows as a change to every line that references it, not a single rename.
Indentation changes from wrapping a block in an if-statement contaminate
the entire block with false positives.

Developers waste time reverse-engineering the semantic intent behind line
noise. Code reviewers miss real changes buried in formatting diffs.

## 2. Product

Stratum is a structural diff and merge tool. It parses source files into
abstract syntax trees, matches nodes across versions, and produces an edit
script that names the structural operations: moves, renames, insertions,
deletions, and value changes. A web UI renders the result as a side-by-side
view with move arrows and collapsible unchanged regions.

## 3. Users

**Primary:** Software developers reviewing code changes - their own or
others' - in pull requests, merge requests, or local comparisons.

**Secondary:** Teams maintaining transformation-heavy codebases (XSLT, XSLT
mappings in SAP CPI, DSL configurations) where line diffs are nearly
useless.

## 4. Core Workflow

### 4.1 Submit Code (Paste Code tab)

The user provides two versions of a file:
- Paste text into a syntax-highlighted left/right editor.
- Upload or drag-drop files into either pane.
- Language auto-detection examines the first 500 characters for patterns
  (package/func for Go, XML declarations for XSLT, type annotations for
  TypeScript, etc.). The detected language is shown below the editor and
  code is highlighted in real time.
- If auto-detection fails, the user must select a language manually. The
  system does not guess or silently default.

### 4.2 Submit Code (Git Diff tab)

The user provides a local git repository path and two refs:
- Repository path: absolute path to a local git repository.
- From ref (base) and To ref (target): branch names, tags, commit hashes,
  or relative refs (HEAD~1, HEAD~5).
- Click "List files" to see all changed files between the two refs.
- Files display in a folder-grouped tree with collapse/expand, status
  letters (A/D/M/R), and a count per folder.
- Select a file and click "Diff selected file". Language is auto-detected
  from the file extension.
- If no files changed, an empty state message is shown.

### 4.3 Diff Computation

On submission, Stratum:
1. Parses both versions into ASTs using the language-specific parser.
2. Runs the matching algorithm (top-down hash, bottom-up dice, residual
   optimal) to pair nodes across versions.
3. Generates an edit script describing the minimal set of structural
   operations that transform the left tree into the right tree.
4. Classifies each top-level declaration with a semantic verdict.

The computation is asynchronous. The user sees a progress indicator during
processing. Jobs are idempotent: submitting the same pair of files returns
the cached result.

### 4.4 Diff View

The result renders on a permalink page with:
- **Result header:** Total operations with a breakdown by type (insert,
  delete, move, rename, update), each with its colored indicator dot.
  Language label. Copy link and copy summary buttons.
- **Change summary:** Plain-language list of what happened (which nodes
  were renamed, moved, added, modified).
- **Semantic verdict pills:** Clickable pills classifying each top-level
  declaration as behavior-preserving, behavior-changing, or indeterminate.
  Clicking scrolls both panes to the relevant code.
- **Diff panes:** Two virtualized scroll-synced panes with syntax
  highlighting. Changed lines have a colored left-edge bar indicating the
  operation type. Within changed lines, word-level inline highlighting
  marks the specific tokens that differ.
- **Move arrows:** Orange SVG curves connecting moved nodes across panes.
  Hover to highlight both source and destination lines.
- **Collapsed regions:** Unchanged code folds into a single row showing
  the collapsed line count and 3 lines of context. Click to expand.

### 4.5 Permalinks and Sharing

Every diff result gets a unique URL. Original and modified source code are
stored alongside the result. "Copy link" copies the URL; "Copy summary"
copies a formatted text summary with operation counts, change descriptions,
and the URL.

### 4.6 Bounded Computation

When the algorithm's cost estimate exceeds a configurable budget (node
count threshold), Stratum degrades gracefully:
- The over-budget subtree falls back to line-based diff.
- The region is marked "approximate" in the UI with a visible indicator.
- The rest of the tree uses full structural diff.

The user never waits indefinitely.

### 4.7 Error Handling

- **API unreachable:** A banner on the main page with a retry button.
- **Network errors:** Descriptive messages ("Cannot reach the server")
  with retry buttons on the diff result page.
- **Request too large (>1MB):** Explicit 413 error message.
- **Large files (>5000 lines):** Warning that processing may take longer.
- **Language detection failure:** Error message asking the user to select
  a language from the dropdown. No silent fallback.
- **Expired sources:** Hint that sources are cleaned up after job
  completion with a "New diff" button.

## 5. Edit Operations

The edit script contains these operation types:

| Operation    | Meaning                                          |
|-------------|--------------------------------------------------|
| `insert`    | Node exists only in the right tree.              |
| `delete`    | Node exists only in the left tree.               |
| `move`      | Node matched but parent or sibling position changed. |
| `rename`    | Node's identifier label changed, structure held. |
| `update`    | Node's value changed, structure held.            |
| `align`     | Children reordered within a matched parent.      |

Each operation carries:
- The node path in both trees (where applicable).
- The node kind (from the parser).
- Source location (line, column, offset) in both versions.

## 6. Language Support

### 6.1 Parser Plugin Interface

Languages are added through a parser plugin interface. A plugin provides:
- A `Parse(ctx, source []byte) (*Tree, error)` function.
- A language identifier string.
- File extension associations.

The matcher and edit script generator are language-agnostic. They operate
on the `Tree` type returned by parsers.

### 6.2 Implemented Parsers

| Language       | Parser Type          | What It Detects                                   |
|---------------|---------------------|---------------------------------------------------|
| Go            | Full AST (stdlib)    | Functions, types, interfaces, methods, statements, expressions |
| XSLT / XML    | Custom XML parser    | Elements, attributes, templates, text nodes        |
| JavaScript    | Structural scanner   | Functions, classes, methods, imports, exports, variables |
| TypeScript    | Structural scanner   | Functions, classes, interfaces, types, methods, imports |
| Python        | Structural scanner   | Functions, classes, methods, imports               |
| Java          | Structural scanner   | Classes, interfaces, enums, methods, fields, imports |
| C / C++       | Structural scanner   | Functions, structs, unions, enums, typedefs, preprocessor directives |

**Full AST** parsers produce a complete syntax tree. **Structural** parsers
identify major declarations for accurate move and rename detection while
treating internal code as text.

### 6.3 Adding a Language

Adding a new language requires:
1. Implementing the parser plugin interface.
2. Registering the plugin in the parser registry.
3. Adding golden test fixtures.

No changes to the matcher, edit script generator, or frontend.

## 7. Caching

Stratum uses content-addressed caching:
- Parse results are cached by SHA-256 of the input source.
- Diff results are cached by the pair of source hashes.
- Cache entries have a configurable TTL.

Identical inputs never recompute.

## 8. Async Processing

Diff computation runs as an asynchronous job:
1. Client submits a diff request, receives a job ID.
2. Client subscribes via SSE for status updates (with polling fallback).
3. On completion, the result renders on a permalink page.

Jobs are persisted in Postgres. The system is crash-recoverable: stale
running jobs are recovered on worker startup.

## 9. Non-Functional Requirements

- **Latency target:** < 2 seconds for files under 2,000 lines with full
  structural diff. Files over the budget threshold degrade within 500ms.
- **Concurrency:** Multiple users can submit diffs simultaneously. Jobs
  execute in a bounded worker pool.
- **Idempotency:** Resubmitting the same pair of files returns the cached
  result, not a new job.
- **Data retention:** Uploaded files and results are ephemeral. No
  long-term storage of user code. Sources cleaned up after job completion.

## 10. What Is Not in v1

- Three-way merge and conflict resolution (v3).
- Cross-file rename and refactor detection (v3).
- User accounts and authentication.
- Collaboration features.
- CLI tool.
- IDE integration.

## 11. v2 Features

### 11.1 Semantic Change Detection via Dataflow Analysis

For languages with full AST support (currently Go), Stratum performs
lightweight def/use analysis on function bodies to classify statement
reorderings:

- **Independent reordering:** Two pure statements (no function calls)
  whose def/use sets are disjoint. Verdict: behavior-preserving.
- **Dependent reordering:** Two statements where one writes a variable
  the other reads. Verdict: behavior-changing.
- **Side-effect reordering:** Statements containing function calls,
  goroutines, defers, or control flow. Verdict: indeterminate (without
  interprocedural analysis, side effects cannot be ruled out).

The analysis is bounded: blocks with more than 50 statements fall back
to indeterminate. Only direct children of the function body block are
analyzed; nested statement reorderings are deferred.

For languages with structural scanners (JS, TS, Python, Java, C, C++),
function bodies are opaque text. Semantic verdicts for those languages
remain heuristic, based on edit-script shape.

### 11.2 v2 Roadmap (remaining)

- Public labeled benchmark on a real open-source project's git history
  with published move-detection precision and recall.
- Blog post on findings from running Stratum across public SAP CPI
  mappings on GitHub.

## 12. v3 Roadmap

- Three-way merge with structural conflict resolution.
- Cross-file rename and refactor detection.
- Additional language plugins driven by demand.
