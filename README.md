# Stratum

Structural diff and merge tool. Parses source files into ASTs, matches
nodes across versions, and produces an edit script that names moves,
renames, and semantic changes rather than line changes.

## What it does

Line-based diffs show you *what lines changed*. Stratum shows you *what
happened*: a function moved, a variable was renamed, a condition was
inverted. It parses both versions of a file into syntax trees, matches
nodes across them, and generates an edit script of structural operations.

A web UI renders the result as a side-by-side view with move arrows and
collapsible unchanged regions.

## How it works

1. **Parse** both file versions into structural trees using language-specific
   parsers. Go gets a full AST via the stdlib parser; XSLT/XML uses a
   structure-aware XML parser; JS, TS, Python, Java, C, and C++ use
   heuristic structural scanners that identify declarations without a
   full AST.
2. **Match** nodes across trees in three phases: top-down hash matching
   for identical subtrees, bottom-up dice matching for restructured
   subtrees, and residual optimal alignment for small ambiguous regions.
3. **Generate** an edit script classifying each change as an insert,
   delete, move, rename, update, or alignment change.
4. **Render** the diff in a web UI with scroll-synced panes, syntax
   highlighting, diff annotations, and SVG move arrows.

When the optimal-alignment cost estimate exceeds a configurable budget,
Stratum leaves the remaining nodes as insertions/deletions and marks the
region approximate instead of attempting an expensive alignment.

## Setup

Prerequisites: Go 1.25+, Node.js 20+, Docker.

```bash
make setup
```

Start development:

```bash
make dev
cd server && go run ./cmd/api    # in one terminal
cd web && npm run dev             # in another terminal
```

Run tests:

```bash
make test
```

See [Verification and Evidence](docs/testing.md) for the test matrix, latest
results, representative benchmarks, and the limits of the current evidence.

## Docker deployment

Run the full stack (Postgres + API + frontend) with Docker Compose:

```bash
docker compose --profile full up --build
```

Without `--profile full`, only Postgres starts (for local development).

The API reads `ALLOWED_ORIGINS` to restrict CORS (comma-separated origins,
or `*` for any). It defaults to `http://localhost:3000`. Local Git endpoints
are opt-in through `GIT_ENABLED=true`; the Compose development stack enables
them, while a production deployment should leave them disabled.

Individual images:

```bash
docker build -t stratum-api ./server
docker build -t stratum-web ./web
```

## Project structure

```
server/           Go backend
  cmd/api/        Entry point
  internal/
    core/         Algorithm (pure, no I/O)
    parse/        Language parsers and plugin registry
    cache/        Content-addressed cache
    jobs/         Async job execution
    store/        Postgres persistence
    http/         Handlers, middleware
  migrations/     SQL migrations
web/              Next.js frontend
contract/         OpenAPI spec (source of truth for wire format)
docs/             Design documents and ADRs
```

## Roadmap

### v1

Core structural diff with web UI. Go full AST parser, XSLT/XML
structure-aware parser, and heuristic structural scanners for JS, TS,
Python, Java, C, and C++. Async job execution, content-addressed
caching, bounded computation with graceful degradation.

### Language fidelity

| Language     | Parser type                  | Fidelity                                     |
|-------------|------------------------------|----------------------------------------------|
| Go          | Full AST (go/parser stdlib)  | Every statement and expression is a node     |
| XSLT / XML  | Structure-aware XML parser   | Elements, attributes, templates, text nodes  |
| JavaScript  | Heuristic structural scanner | Functions, classes, methods, imports, exports |
| TypeScript  | Heuristic structural scanner | Functions, classes, interfaces, types, imports|
| Python      | Heuristic structural scanner | Functions, classes, methods, imports          |
| Java        | Heuristic structural scanner | Classes, interfaces, enums, methods, imports  |
| C / C++     | Heuristic structural scanner | Functions, structs, enums, typedefs, macros  |

Full AST parsers produce a complete syntax tree. Structural scanners
identify top-level declarations for accurate move and rename detection
but treat function bodies as opaque text. Unsupported languages fall
back to line-by-line comparison with trivial-line filtering.

### Known limitations

- **Structural scanners are not full parsers.** JS/TS/Python/Java/C/C++
  scanners identify declarations by keyword patterns. Deeply nested or
  unusual syntax may be missed or misclassified.
- **Semantic verdicts are conservative heuristics, not proof.** Go
  statement reorderings receive an additional bounded def/use analysis.
  Other Go changes and scanner-backed languages still rely on edit-script
  shape and can be misclassified.
- **Body limit is per request, not per file.** The server enforces a 1 MB
  limit on the entire JSON request body.
- **Git mode accesses the local filesystem.** When explicitly enabled,
  the Git diff tab runs `git show` and `git diff` against local repositories.
  Keep `GIT_ENABLED=false` on a public deployment; the API routes and frontend
  tab are disabled in that configuration.

### v2 (current)

Function-level semantic verdicts classify changes as behavior-preserving,
behavior-changing, or indeterminate. For Go, lightweight def/use analysis
distinguishes independent from dependent direct statement reorderings.
Calls, control flow, nested reordering, and blocks over 50 statements stay
indeterminate. Remaining v2 work is a labeled public benchmark with
precision/recall and a published case study.

### v3

Three-way merge with structural conflict resolution. Cross-file rename
and refactor detection. Additional language plugins driven by demand.

## License

MIT
