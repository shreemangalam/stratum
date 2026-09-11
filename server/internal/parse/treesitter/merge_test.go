package treesitter

import (
	"context"
	"testing"

	"github.com/shreemangalam/stratum/server/internal/core"
)

func TestThreeWayMergePlan_GoDeclarations(t *testing.T) {
	t.Parallel()

	baseFunction := `package main
func f() int { return 1 }
`

	tests := []struct {
		name         string
		base         string
		left         string
		right        string
		label        string
		wantDecision core.MergeDecision
		wantConflict core.MergeConflictKind
	}{
		{
			name:         "left-only modification",
			base:         baseFunction,
			left:         "package main\nfunc f() int { return 2 }\n",
			right:        baseFunction,
			label:        "f",
			wantDecision: core.MergeTakeLeft,
		},
		{
			name:         "identical concurrent modification",
			base:         baseFunction,
			left:         "package main\nfunc f() int { return 2 }\n",
			right:        "package main\nfunc f() int { return 2 }\n",
			label:        "f",
			wantDecision: core.MergeTakeEither,
		},
		{
			name:         "divergent modification",
			base:         baseFunction,
			left:         "package main\nfunc f() int { return 2 }\n",
			right:        "package main\nfunc f() int { return 3 }\n",
			label:        "f",
			wantDecision: core.MergeConflict,
			wantConflict: core.ConflictModifyModify,
		},
		{
			name:         "delete versus modify",
			base:         baseFunction,
			left:         "package main\n",
			right:        "package main\nfunc f() int { return 2 }\n",
			label:        "f",
			wantDecision: core.MergeConflict,
			wantConflict: core.ConflictDeleteModify,
		},
		{
			name:         "both delete",
			base:         baseFunction,
			left:         "package main\n",
			right:        "package main\n",
			label:        "f",
			wantDecision: core.MergeDelete,
		},
		{
			name:         "divergent rename",
			base:         baseFunction,
			left:         "package main\nfunc g() int { return 1 }\n",
			right:        "package main\nfunc h() int { return 1 }\n",
			label:        "f",
			wantDecision: core.MergeConflict,
			wantConflict: core.ConflictRenameRename,
		},
		{
			name:         "divergent same-name addition",
			base:         "package main\n",
			left:         "package main\nfunc f() int { return 1 }\n",
			right:        "package main\nfunc f() int { return 2 }\n",
			label:        "f",
			wantDecision: core.MergeConflict,
			wantConflict: core.ConflictAddAdd,
		},
		{
			name:         "identical concurrent addition",
			base:         "package main\n",
			left:         "package main\nfunc f() int { return 1 }\n",
			right:        "package main\nfunc f() int { return 1 }\n",
			label:        "f",
			wantDecision: core.MergeTakeEither,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan, err := core.PlanThreeWayMerge(
				parseGoMergeTree(t, tt.base),
				parseGoMergeTree(t, tt.left),
				parseGoMergeTree(t, tt.right),
				core.DefaultMatchConfig(),
			)
			if err != nil {
				t.Fatalf("plan merge: %v", err)
			}

			entry := findMergeEntry(t, plan, tt.label)
			if entry.Decision != tt.wantDecision {
				t.Fatalf("decision = %q, want %q (reason: %s)", entry.Decision, tt.wantDecision, entry.Reason)
			}
			if tt.wantConflict == "" {
				if entry.ConflictKind != nil {
					t.Errorf("conflict kind = %q, want none", *entry.ConflictKind)
				}
				return
			}
			if entry.ConflictKind == nil || *entry.ConflictKind != tt.wantConflict {
				t.Errorf("conflict kind = %v, want %q", entry.ConflictKind, tt.wantConflict)
			}
		})
	}
}

func TestThreeWayMergePlan_IndependentAdditions(t *testing.T) {
	t.Parallel()

	plan, err := core.PlanThreeWayMerge(
		parseGoMergeTree(t, "package main\n"),
		parseGoMergeTree(t, "package main\nfunc leftOnly() {}\n"),
		parseGoMergeTree(t, "package main\nfunc rightOnly() {}\n"),
		core.DefaultMatchConfig(),
	)
	if err != nil {
		t.Fatalf("plan merge: %v", err)
	}

	if got := findMergeEntry(t, plan, "leftOnly").Decision; got != core.MergeTakeLeft {
		t.Errorf("left addition decision = %q, want %q", got, core.MergeTakeLeft)
	}
	if got := findMergeEntry(t, plan, "rightOnly").Decision; got != core.MergeTakeRight {
		t.Errorf("right addition decision = %q, want %q", got, core.MergeTakeRight)
	}
	if plan.HasConflicts || plan.ConflictCount != 0 {
		t.Errorf("independent additions reported conflicts: %+v", plan)
	}
}

func TestThreeWayMergePlan_RejectsLanguageMismatch(t *testing.T) {
	t.Parallel()

	base := parseGoMergeTree(t, "package main\n")
	left := parseGoMergeTree(t, "package main\n")
	right := parseGoMergeTree(t, "package main\n")
	right.Language = "python"

	if _, err := core.PlanThreeWayMerge(base, left, right, core.DefaultMatchConfig()); err == nil {
		t.Fatal("expected language mismatch error")
	}
}

func parseGoMergeTree(t *testing.T, source string) *core.Tree {
	t.Helper()
	tree, err := NewGo().Parse(context.Background(), []byte(source))
	if err != nil {
		t.Fatalf("parse Go source: %v", err)
	}
	return tree
}

func findMergeEntry(t *testing.T, plan *core.MergePlan, label string) core.MergeEntry {
	t.Helper()
	for _, entry := range plan.Entries {
		for _, ref := range []*core.NodeRef{entry.BaseNode, entry.LeftNode, entry.RightNode} {
			if ref != nil && ref.Label == label {
				return entry
			}
		}
	}
	t.Fatalf("merge entry %q not found in %+v", label, plan)
	return core.MergeEntry{}
}
