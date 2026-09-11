# ADR-0003: Three-Way Structural Merge Plan

**Date:** 2026-09-11
**Status:** Accepted

## Context

The v3 roadmap calls for three-way merge with structural conflict resolution.
A three-way merge compares a common base with two independently edited sides.
Line-oriented merge treats nearby text edits as conflicts even when they touch
different declarations, while a structural merge can reason about the syntax
units that changed.

Stratum already provides deterministic pairwise tree matching and Merkle
hashing. It does not yet have a model for representing three-way decisions,
and it cannot safely synthesize formatted source for every supported language.
The first v3 slice therefore needs to separate structural planning from source
rendering.

## Decision

### Introduce a pure merge planner in `internal/core`

`PlanThreeWayMerge` accepts base, left, and right trees plus the existing match
configuration. It performs base-to-left and base-to-right matching and emits a
deterministic `MergePlan`. The planner has no parser, filesystem, database, or
HTTP dependencies.

### Start at direct-child granularity

The first implementation plans changes to direct children of the parsed root.
For Go and the structural scanners, these are file-level declarations and
other top-level syntax units. This delivers useful conflict separation without
claiming recursive merge correctness. Nested structural merging is deferred.

### Use explicit decisions

Each merge entry records references to the available base, left, and right
nodes and one decision:

| Decision | Meaning |
|---|---|
| `unchanged` | Neither side changed the base node. |
| `take-left` | Only the left side changed or inserted the node. |
| `take-right` | Only the right side changed or inserted the node. |
| `take-either` | Both sides produced the same subtree. |
| `delete` | Deletion is unopposed by a modification. |
| `conflict` | The sides made incompatible changes requiring resolution. |

The first conflict taxonomy is `modify-modify`, `delete-modify`,
`rename-rename`, and `add-add`. Reasons remain human-readable, while the typed
kind is stable for future API and UI behavior.

### Treat Merkle equality as exact structural equality

After pairwise matching computes hashes, equal non-empty subtree hashes mean
the syntax structure and values are identical. Concurrent identical edits are
deduplicated as `take-either`. Different hashes do not by themselves prove a
conflict; the base comparison determines whether one or both sides changed.

### Defer source synthesis

The merge plan does not emit merged source in this slice. Rendering requires
language-aware ordering, trivia and comment preservation, import handling, and
validated span replacement. Returning a trustworthy plan is preferable to
returning malformed code. A later ADR may add renderers per parser capability.

## Consequences

- Independent changes to different top-level declarations do not conflict.
- Delete-versus-unchanged resolves to deletion; delete-versus-modify conflicts.
- Concurrent identical additions are deduplicated; same-identity divergent
  additions conflict.
- Moves, nested edits, comments, and formatting are not yet merged recursively.
- The model can be exposed through OpenAPI later without coupling core logic to
  transport concerns.
