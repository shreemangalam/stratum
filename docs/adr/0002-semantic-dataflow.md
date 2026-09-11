# ADR-0002: Lightweight Dataflow Analysis for Semantic Verdicts

**Date:** 2026-09-11
**Status:** Accepted

## Context

The v1 semantic classifier returns "indeterminate" for all statement
reorderings within a function. The edit-script shape alone cannot
distinguish safe reorderings (swapping two independent assignments)
from unsafe ones (swapping two assignments where the second depends on
the first). The v2 roadmap calls for lightweight dataflow analysis to
improve confidence.

Only Go has a full AST with statement-level and expression-level nodes.
The structural scanners for JS, TS, Python, Java, C, and C++ treat
function bodies as opaque text, so dataflow analysis has nothing to
work with there.

## Decisions

### Add assignment_lhs/assignment_rhs wrapper nodes to the Go parser

The current parser flattens LHS and RHS expressions into a single
children list under `assignment_statement`, losing the boundary between
defined and used identifiers. Wrapper nodes make def/use extraction
unambiguous. The `assignment_statement` Label stores the token (`:=`
or `=`).

**Alternative:** Store `len(Lhs)` in the Label. Rejected because it
conflates metadata with identifier labels, affecting hash matching.

### Def/use extraction in core/dataflow.go

A new `DefUseSet` type with `{Defs, Uses map[string]bool, HasSideEffects
bool}`. The `extractDefUse` function walks a statement's subtree:

- Identifiers under `assignment_lhs`: defs.
- All other identifiers: uses.
- `call_expression`, `go_statement`, `defer_statement`: side effects.
- Control flow (`if_statement`, `for_statement`, etc.): side effects
  (conservative; control flow makes reordering non-trivial).

### Conservative call treatment

Without interprocedural analysis, all function calls are assumed to have
side effects. Only pure statements (assignments/declarations with no
calls) get definitive verdicts. This avoids false positives.

### Integration as a refinement step

The dataflow check runs only when:
1. `classifyOps` returned `indeterminate, "statements reordered"`.
2. The tree's language is `"go"`.
3. All moved nodes are direct children of the function body block.
4. The block has 50 or fewer statements (bounded computation).

### Three possible upgrades

| Condition | Verdict |
|-----------|---------|
| All reversed pairs are pure and independent | `preserving, "independent statements reordered"` |
| Any reversed pair shares defs/uses | `changing, "dependent statements reordered"` |
| Any reversed pair has side effects | `indeterminate, "statements reordered"` (unchanged) |

## Consequences

- Go functions with reordered pure assignments get accurate verdicts.
- Non-Go languages are unaffected; their verdicts remain heuristic.
- Golden tests need regeneration due to the parser structure change.
- The module is pure and lives in `core/`, consistent with the I/O
  boundary rule.
- Foundation for future work: better "body edited" classification,
  nested statement analysis, extension to other full-AST languages.
