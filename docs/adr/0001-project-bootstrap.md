# ADR-0001: Project Bootstrap Decisions

**Date:** 2026-09-01
**Status:** Accepted

## Context

Stratum is a new project. This ADR records the foundational technology
choices made at project inception.

## Decisions

### Go for the backend

**Choice:** Go (originally 1.23+, now 1.25+ due to dependency requirements).

**Rationale:** The core algorithm is CPU-bound tree traversal and hashing.
Go provides the performance characteristics needed without the complexity
of Rust's ownership model. The concurrency model (goroutines, channels)
maps well to the async job execution pattern. The stdlib `go/parser`
gives full AST access for Go source with zero dependencies.

**Alternatives considered:** Rust (higher performance ceiling but slower
iteration for a solo/small-team project), TypeScript/Node (insufficient
CPU performance for the algorithm).

### Next.js for the frontend

**Choice:** Next.js with TypeScript in strict mode.

**Rationale:** Server components reduce client bundle size for the
non-interactive parts of the app. The diff view is inherently a client
component, but everything around it (submission form, language list, empty
states) benefits from server rendering. The React ecosystem has the
strongest virtualization libraries needed for the diff pane.

### PostgreSQL as the only data store

**Choice:** Postgres for jobs, cache overflow, and future needs.

**Rationale:** One data store to operate, back up, and reason about. The
job queue pattern (SELECT FOR UPDATE SKIP LOCKED) is well-proven in
Postgres. No Redis, no message broker. If Postgres becomes a bottleneck,
it can be scaled vertically or read-replicated long before a second data
store is justified.

### Pure Go parsers, no cgo

**Choice:** Language-specific pure Go parsers. Go full AST via `go/parser`
stdlib, XSLT/XML via `encoding/xml`, and heuristic structural scanners
for JS, TS, Python, Java, C, and C++.

**Rationale:** Keeping all parsers in pure Go eliminates cgo build
complexity and cross-compilation issues. The Go stdlib parser gives full
AST fidelity for the primary language. Structural scanners identify
top-level declarations by keyword patterns, which is sufficient for
accurate move and rename detection. The plugin interface allows adding
higher-fidelity parsers (including tree-sitter via cgo) per language if
demand justifies the dependency.

**Alternatives considered:** Tree-sitter via cgo bindings (broader
coverage per grammar, but adds C toolchain requirement and complicates
cross-compilation; not justified until scanner accuracy is proven
insufficient on real-world code).

### Contract-first API design

**Choice:** OpenAPI 3.1 as the source of truth for the wire contract.
Go server stubs and TypeScript client generated from the spec.

**Rationale:** The frontend and backend are developed in the same repo but
must agree on the wire format. Code generation from a single spec prevents
drift. Breaking changes to the API surface are visible as spec changes in
code review, not as runtime surprises.

### Content-addressed caching

**Choice:** SHA-256 of source bytes as the cache key for parse results,
sorted hash pair for diff results.

**Rationale:** The same file submitted twice should never be reparsed.
Content addressing makes this automatic — no explicit invalidation needed.
TTL handles staleness for the cache-overflow layer in Postgres.

### Bounded computation with graceful degradation

**Choice:** When optimal alignment would exceed a node-count budget, fall
back to line diff for that subtree and mark it approximate.

**Rationale:** The optimal tree alignment phase has super-quadratic worst
case. Users must never experience a hang or timeout. Degrading to line diff
with a visible "approximate" marker preserves trust — the tool tells you
when it can't give a perfect answer.

## Consequences

- All code lives in one repository (monorepo with Go + Next.js).
- The only infrastructure dependency is Postgres.
- No C toolchain required — all parsers are pure Go.
- Adding a language requires only implementing the parser plugin interface.
- The algorithm can be tested and benchmarked without any infrastructure.
- The wire contract is explicit and checked by CI.
