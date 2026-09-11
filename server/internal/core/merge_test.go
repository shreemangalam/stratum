package core

import "testing"

func mergeTree(id NodeID, lang string, children ...*Node) *Tree {
	root := buildTree(id, "source_file", "", children...)
	return NewTree(root, lang, nil)
}

func funcNode(id NodeID, name, bodyVal string) *Node {
	return buildTree(id, "function_declaration", name,
		buildTree(id+1, "block", bodyVal),
	)
}

func TestMerge_NilTrees(t *testing.T) {
	t.Parallel()
	valid := mergeTree(1, "go")
	cases := []struct {
		name       string
		base, l, r *Tree
	}{
		{"nil base", nil, valid, valid},
		{"nil left", valid, nil, valid},
		{"nil right", valid, valid, nil},
		{"all nil", nil, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := PlanThreeWayMerge(tc.base, tc.l, tc.r, DefaultMatchConfig())
			if err == nil {
				t.Fatal("expected error for nil tree")
			}
		})
	}
}

func TestMerge_NilRoots(t *testing.T) {
	t.Parallel()
	valid := mergeTree(1, "go")
	nilRoot := &Tree{Root: nil, Language: "go", NodeMap: map[NodeID]*Node{}}

	_, err := PlanThreeWayMerge(nilRoot, valid, valid, DefaultMatchConfig())
	if err == nil {
		t.Fatal("expected error for nil root")
	}
}

func TestMerge_EmptyRoots(t *testing.T) {
	t.Parallel()
	base := mergeTree(1, "go")
	left := mergeTree(101, "go")
	right := mergeTree(201, "go")

	plan, err := PlanThreeWayMerge(base, left, right, DefaultMatchConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Entries) != 0 {
		t.Errorf("expected 0 entries for empty roots, got %d", len(plan.Entries))
	}
	if plan.HasConflicts || plan.ConflictCount != 0 {
		t.Errorf("empty merge should have no conflicts")
	}
}

func TestMerge_CompletelyUnchanged(t *testing.T) {
	t.Parallel()
	base := mergeTree(1, "go",
		funcNode(10, "f", "return 1"),
		funcNode(20, "g", "return 2"),
	)
	left := mergeTree(101, "go",
		funcNode(110, "f", "return 1"),
		funcNode(120, "g", "return 2"),
	)
	right := mergeTree(201, "go",
		funcNode(210, "f", "return 1"),
		funcNode(220, "g", "return 2"),
	)

	plan, err := PlanThreeWayMerge(base, left, right, DefaultMatchConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.HasConflicts || plan.ConflictCount != 0 {
		t.Error("unchanged file should have no conflicts")
	}
	for _, e := range plan.Entries {
		if e.Decision != MergeUnchanged {
			t.Errorf("entry %q: got decision %q, want unchanged", e.BaseNode.Label, e.Decision)
		}
	}
}

func TestMerge_LanguageMismatch(t *testing.T) {
	t.Parallel()
	base := mergeTree(1, "go")
	left := mergeTree(101, "go")
	right := mergeTree(201, "python")

	_, err := PlanThreeWayMerge(base, left, right, DefaultMatchConfig())
	if err == nil {
		t.Fatal("expected error for language mismatch")
	}
}

func TestMerge_LeftOnlyModification(t *testing.T) {
	t.Parallel()
	base := mergeTree(1, "go", funcNode(10, "f", "return 1"))
	left := mergeTree(101, "go", funcNode(110, "f", "return 2"))
	right := mergeTree(201, "go", funcNode(210, "f", "return 1"))

	plan, err := PlanThreeWayMerge(base, left, right, DefaultMatchConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.HasConflicts {
		t.Error("left-only modification should not conflict")
	}
	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Decision != MergeTakeLeft {
		t.Errorf("decision = %q, want take-left", plan.Entries[0].Decision)
	}
}

func TestMerge_RightOnlyModification(t *testing.T) {
	t.Parallel()
	base := mergeTree(1, "go", funcNode(10, "f", "return 1"))
	left := mergeTree(101, "go", funcNode(110, "f", "return 1"))
	right := mergeTree(201, "go", funcNode(210, "f", "return 2"))

	plan, err := PlanThreeWayMerge(base, left, right, DefaultMatchConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.HasConflicts {
		t.Error("right-only modification should not conflict")
	}
	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Decision != MergeTakeRight {
		t.Errorf("decision = %q, want take-right", plan.Entries[0].Decision)
	}
}

func TestMerge_LeftDeleteRightUnchanged(t *testing.T) {
	t.Parallel()
	base := mergeTree(1, "go", funcNode(10, "f", "return 1"))
	left := mergeTree(101, "go")
	right := mergeTree(201, "go", funcNode(210, "f", "return 1"))

	plan, err := PlanThreeWayMerge(base, left, right, DefaultMatchConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.HasConflicts {
		t.Error("left-delete right-unchanged should not conflict")
	}
	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Decision != MergeDelete {
		t.Errorf("decision = %q, want delete", plan.Entries[0].Decision)
	}
}

func TestMerge_RightDeleteLeftUnchanged(t *testing.T) {
	t.Parallel()
	base := mergeTree(1, "go", funcNode(10, "f", "return 1"))
	left := mergeTree(101, "go", funcNode(110, "f", "return 1"))
	right := mergeTree(201, "go")

	plan, err := PlanThreeWayMerge(base, left, right, DefaultMatchConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.HasConflicts {
		t.Error("right-delete left-unchanged should not conflict")
	}
	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Decision != MergeDelete {
		t.Errorf("decision = %q, want delete", plan.Entries[0].Decision)
	}
}

func TestMerge_BothDeleteSameNode(t *testing.T) {
	t.Parallel()
	base := mergeTree(1, "go", funcNode(10, "f", "return 1"))
	left := mergeTree(101, "go")
	right := mergeTree(201, "go")

	plan, err := PlanThreeWayMerge(base, left, right, DefaultMatchConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.HasConflicts {
		t.Error("both-delete should not conflict")
	}
	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Decision != MergeDelete {
		t.Errorf("decision = %q, want delete", plan.Entries[0].Decision)
	}
}

func TestMerge_DeleteVsModifyConflict(t *testing.T) {
	t.Parallel()
	base := mergeTree(1, "go", funcNode(10, "f", "return 1"))
	left := mergeTree(101, "go")
	right := mergeTree(201, "go", funcNode(210, "f", "return 2"))

	plan, err := PlanThreeWayMerge(base, left, right, DefaultMatchConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !plan.HasConflicts || plan.ConflictCount != 1 {
		t.Fatalf("expected 1 conflict, got count=%d has=%v", plan.ConflictCount, plan.HasConflicts)
	}
	e := plan.Entries[0]
	if e.Decision != MergeConflict {
		t.Errorf("decision = %q, want conflict", e.Decision)
	}
	if e.ConflictKind == nil || *e.ConflictKind != ConflictDeleteModify {
		t.Errorf("conflict kind = %v, want delete-modify", e.ConflictKind)
	}
}

func TestMerge_IdenticalConcurrentModification(t *testing.T) {
	t.Parallel()
	base := mergeTree(1, "go", funcNode(10, "f", "return 1"))
	left := mergeTree(101, "go", funcNode(110, "f", "return 2"))
	right := mergeTree(201, "go", funcNode(210, "f", "return 2"))

	plan, err := PlanThreeWayMerge(base, left, right, DefaultMatchConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.HasConflicts {
		t.Error("identical concurrent modification should not conflict")
	}
	if plan.Entries[0].Decision != MergeTakeEither {
		t.Errorf("decision = %q, want take-either", plan.Entries[0].Decision)
	}
}

func TestMerge_DivergentModificationConflict(t *testing.T) {
	t.Parallel()
	base := mergeTree(1, "go", funcNode(10, "f", "return 1"))
	left := mergeTree(101, "go", funcNode(110, "f", "return 2"))
	right := mergeTree(201, "go", funcNode(210, "f", "return 3"))

	plan, err := PlanThreeWayMerge(base, left, right, DefaultMatchConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !plan.HasConflicts || plan.ConflictCount != 1 {
		t.Fatalf("expected 1 conflict, got count=%d", plan.ConflictCount)
	}
	if plan.Entries[0].ConflictKind == nil || *plan.Entries[0].ConflictKind != ConflictModifyModify {
		t.Errorf("conflict kind = %v, want modify-modify", plan.Entries[0].ConflictKind)
	}
}

func TestMerge_RenameRenameConflict(t *testing.T) {
	t.Parallel()
	base := mergeTree(1, "go", funcNode(10, "f", "return 1"))
	left := mergeTree(101, "go", funcNode(110, "g", "return 1"))
	right := mergeTree(201, "go", funcNode(210, "h", "return 1"))

	plan, err := PlanThreeWayMerge(base, left, right, DefaultMatchConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !plan.HasConflicts {
		t.Fatal("divergent renames should conflict")
	}
	found := false
	for _, e := range plan.Entries {
		if e.ConflictKind != nil && *e.ConflictKind == ConflictRenameRename {
			found = true
		}
	}
	if !found {
		t.Error("expected rename-rename conflict kind")
	}
}

func TestMerge_BaseOrderPreserved(t *testing.T) {
	t.Parallel()
	base := mergeTree(1, "go",
		funcNode(10, "a", "1"),
		funcNode(20, "b", "2"),
		funcNode(30, "c", "3"),
	)
	left := mergeTree(101, "go",
		funcNode(110, "a", "1"),
		funcNode(120, "b", "modified"),
		funcNode(130, "c", "3"),
	)
	right := mergeTree(201, "go",
		funcNode(210, "a", "1"),
		funcNode(220, "b", "2"),
		funcNode(230, "c", "modified"),
	)

	plan, err := PlanThreeWayMerge(base, left, right, DefaultMatchConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.HasConflicts {
		t.Fatal("independent modifications should not conflict")
	}
	if len(plan.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(plan.Entries))
	}
	labels := []string{"a", "b", "c"}
	for i, want := range labels {
		got := plan.Entries[i].BaseNode.Label
		if got != want {
			t.Errorf("entry[%d] label = %q, want %q", i, got, want)
		}
	}
	if plan.Entries[0].Decision != MergeUnchanged {
		t.Errorf("a: decision = %q, want unchanged", plan.Entries[0].Decision)
	}
	if plan.Entries[1].Decision != MergeTakeLeft {
		t.Errorf("b: decision = %q, want take-left", plan.Entries[1].Decision)
	}
	if plan.Entries[2].Decision != MergeTakeRight {
		t.Errorf("c: decision = %q, want take-right", plan.Entries[2].Decision)
	}
}

func TestMerge_AdditionOrderDeterministic(t *testing.T) {
	t.Parallel()
	base := mergeTree(1, "go")
	left := mergeTree(101, "go",
		funcNode(110, "leftA", "1"),
		funcNode(120, "leftB", "2"),
	)
	right := mergeTree(201, "go",
		funcNode(210, "rightA", "1"),
		funcNode(220, "rightB", "2"),
	)

	plan, err := PlanThreeWayMerge(base, left, right, DefaultMatchConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.HasConflicts {
		t.Fatal("independent additions should not conflict")
	}
	if len(plan.Entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(plan.Entries))
	}

	expected := []struct {
		label    string
		decision MergeDecision
	}{
		{"leftA", MergeTakeLeft},
		{"leftB", MergeTakeLeft},
		{"rightA", MergeTakeRight},
		{"rightB", MergeTakeRight},
	}
	for i, want := range expected {
		var label string
		if plan.Entries[i].LeftNode != nil {
			label = plan.Entries[i].LeftNode.Label
		} else if plan.Entries[i].RightNode != nil {
			label = plan.Entries[i].RightNode.Label
		}
		if label != want.label {
			t.Errorf("entry[%d] label = %q, want %q", i, label, want.label)
		}
		if plan.Entries[i].Decision != want.decision {
			t.Errorf("entry[%d] decision = %q, want %q", i, plan.Entries[i].Decision, want.decision)
		}
	}
}

func TestMerge_MultipleConflictsCountedCorrectly(t *testing.T) {
	t.Parallel()
	base := mergeTree(1, "go",
		funcNode(10, "f", "return 1"),
		funcNode(20, "g", "return 2"),
		funcNode(30, "h", "return 3"),
	)
	left := mergeTree(101, "go",
		funcNode(110, "f", "return A"),
		funcNode(120, "g", "return 2"),
		funcNode(130, "h", "return C"),
	)
	right := mergeTree(201, "go",
		funcNode(210, "f", "return B"),
		funcNode(220, "g", "return 2"),
		funcNode(230, "h", "return D"),
	)

	plan, err := PlanThreeWayMerge(base, left, right, DefaultMatchConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.ConflictCount != 2 {
		t.Errorf("conflict count = %d, want 2", plan.ConflictCount)
	}
	if !plan.HasConflicts {
		t.Error("HasConflicts should be true")
	}

	conflicts := 0
	for _, e := range plan.Entries {
		if e.Decision == MergeConflict {
			conflicts++
		}
	}
	if conflicts != plan.ConflictCount {
		t.Errorf("counted %d conflict entries but ConflictCount = %d", conflicts, plan.ConflictCount)
	}
}

func TestMerge_ModificationsAlongsideAdditions(t *testing.T) {
	t.Parallel()
	base := mergeTree(1, "go", funcNode(10, "f", "return 1"))
	left := mergeTree(101, "go",
		funcNode(110, "f", "return 2"),
		funcNode(120, "newLeft", "added"),
	)
	right := mergeTree(201, "go",
		funcNode(210, "f", "return 1"),
		funcNode(220, "newRight", "added"),
	)

	plan, err := PlanThreeWayMerge(base, left, right, DefaultMatchConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.HasConflicts {
		t.Fatal("modification plus independent additions should not conflict")
	}
	if len(plan.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Decision != MergeTakeLeft {
		t.Errorf("base node f: decision = %q, want take-left", plan.Entries[0].Decision)
	}
}

func TestMerge_UnlabeledAdditions(t *testing.T) {
	t.Parallel()
	base := mergeTree(1, "go")
	left := mergeTree(101, "go",
		buildTree(110, "comment", "", buildTree(111, "text", "hello")),
	)
	right := mergeTree(201, "go",
		buildTree(210, "comment", "", buildTree(211, "text", "world")),
	)

	plan, err := PlanThreeWayMerge(base, left, right, DefaultMatchConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.HasConflicts {
		t.Error("unlabeled additions with different content should not conflict as add-add")
	}
	if len(plan.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Decision != MergeTakeLeft {
		t.Errorf("entry[0] decision = %q, want take-left", plan.Entries[0].Decision)
	}
	if plan.Entries[1].Decision != MergeTakeRight {
		t.Errorf("entry[1] decision = %q, want take-right", plan.Entries[1].Decision)
	}
}

func TestMerge_SymmetricDeleteModify(t *testing.T) {
	t.Parallel()
	base := mergeTree(1, "go", funcNode(10, "f", "return 1"))

	leftDeletes := mergeTree(101, "go")
	rightModifies := mergeTree(201, "go", funcNode(210, "f", "return 2"))

	rightDeletes := mergeTree(301, "go")
	leftModifies := mergeTree(401, "go", funcNode(410, "f", "return 2"))

	planA, err := PlanThreeWayMerge(base, leftDeletes, rightModifies, DefaultMatchConfig())
	if err != nil {
		t.Fatalf("plan A: %v", err)
	}
	planB, err := PlanThreeWayMerge(base, leftModifies, rightDeletes, DefaultMatchConfig())
	if err != nil {
		t.Fatalf("plan B: %v", err)
	}

	if planA.ConflictCount != planB.ConflictCount {
		t.Errorf("symmetric conflict counts differ: %d vs %d", planA.ConflictCount, planB.ConflictCount)
	}
	if planA.Entries[0].Decision != planB.Entries[0].Decision {
		t.Errorf("symmetric decisions differ: %q vs %q", planA.Entries[0].Decision, planB.Entries[0].Decision)
	}
	if planA.Entries[0].ConflictKind == nil || planB.Entries[0].ConflictKind == nil {
		t.Fatal("both should be conflicts")
	}
	if *planA.Entries[0].ConflictKind != *planB.Entries[0].ConflictKind {
		t.Errorf("symmetric conflict kinds differ: %q vs %q", *planA.Entries[0].ConflictKind, *planB.Entries[0].ConflictKind)
	}
}

func TestMerge_DeterministicAcrossRuns(t *testing.T) {
	t.Parallel()
	base := mergeTree(1, "go",
		funcNode(10, "f", "return 1"),
		funcNode(20, "g", "return 2"),
	)
	left := mergeTree(101, "go",
		funcNode(110, "f", "return modified"),
		funcNode(120, "g", "return 2"),
		funcNode(130, "newLeft", "added"),
	)
	right := mergeTree(201, "go",
		funcNode(210, "f", "return 1"),
		funcNode(220, "g", "return modified"),
		funcNode(230, "newRight", "added"),
	)

	cfg := DefaultMatchConfig()
	var plans []*MergePlan
	for range 5 {
		plan, err := PlanThreeWayMerge(base, left, right, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		plans = append(plans, plan)
	}

	first := plans[0]
	for i := 1; i < len(plans); i++ {
		p := plans[i]
		if len(p.Entries) != len(first.Entries) {
			t.Fatalf("run %d: entry count %d != %d", i, len(p.Entries), len(first.Entries))
		}
		for j := range first.Entries {
			if p.Entries[j].Decision != first.Entries[j].Decision {
				t.Errorf("run %d entry %d: decision %q != %q", i, j, p.Entries[j].Decision, first.Entries[j].Decision)
			}
			if p.Entries[j].Reason != first.Entries[j].Reason {
				t.Errorf("run %d entry %d: reason %q != %q", i, j, p.Entries[j].Reason, first.Entries[j].Reason)
			}
		}
	}
}

func TestMerge_ConflictCountMatchesEntries(t *testing.T) {
	t.Parallel()
	base := mergeTree(1, "go",
		funcNode(10, "a", "1"),
		funcNode(20, "b", "2"),
		funcNode(30, "c", "3"),
	)
	left := mergeTree(101, "go",
		funcNode(110, "a", "left"),
		funcNode(120, "b", "left"),
		funcNode(130, "c", "3"),
	)
	right := mergeTree(201, "go",
		funcNode(210, "a", "right"),
		funcNode(220, "b", "right"),
		funcNode(230, "c", "right"),
	)

	plan, err := PlanThreeWayMerge(base, left, right, DefaultMatchConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	counted := 0
	for _, e := range plan.Entries {
		if e.Decision == MergeConflict {
			counted++
			if e.ConflictKind == nil {
				t.Errorf("conflict entry %q has nil ConflictKind", e.Reason)
			}
		} else {
			if e.ConflictKind != nil {
				t.Errorf("non-conflict entry %q has ConflictKind %q", e.Reason, *e.ConflictKind)
			}
		}
	}
	if counted != plan.ConflictCount {
		t.Errorf("counted %d conflict entries but ConflictCount = %d", counted, plan.ConflictCount)
	}
	if (plan.ConflictCount > 0) != plan.HasConflicts {
		t.Errorf("HasConflicts = %v but ConflictCount = %d", plan.HasConflicts, plan.ConflictCount)
	}
}
