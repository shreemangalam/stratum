package core

import "testing"

func fnTree(base NodeID, name, literal string) *Node {
	return buildTree(base, "function_declaration", name,
		buildTree(base+1, "parameter_list", ""),
		buildTree(base+2, "block", "",
			buildTree(base+3, "return_statement", "",
				&Node{ID: base + 4, Kind: "literal", Value: literal})),
	)
}

func linkChildren(n *Node) *Node {
	for _, c := range n.Children {
		c.Parent = n
		linkChildren(c)
	}
	return n
}

func TestRenamedFunctionWithEditedBody_Matched(t *testing.T) {
	left := linkChildren(buildTree(1, "source_file", "", fnTree(2, "calculateTotal", "1")))
	right := linkChildren(buildTree(101, "source_file", "", fnTree(102, "computeSum", "2")))

	lt := NewTree(left, "test", nil)
	rt := NewTree(right, "test", nil)

	m := Match(lt, rt, DefaultMatchConfig())
	es := GenerateEditScript(lt, rt, m)

	renames, updates := 0, 0
	for _, op := range es.Operations {
		switch op.Kind {
		case OpRename:
			renames++
		case OpUpdate:
			updates++
		case OpDelete, OpInsert:
			t.Errorf("renamed+edited function should match, got %s on %+v", op.Kind, op)
		}
	}
	if renames != 1 || updates != 1 {
		t.Errorf("expected 1 rename and 1 update, got %d renames, %d updates", renames, updates)
	}
}

func TestSemantic_BodyEdit_Changing(t *testing.T) {
	left := linkChildren(buildTree(1, "source_file", "", fnTree(2, "f", "1")))
	right := linkChildren(buildTree(101, "source_file", "", fnTree(102, "f", "2")))

	lt := NewTree(left, "test", nil)
	rt := NewTree(right, "test", nil)

	m := Match(lt, rt, DefaultMatchConfig())
	es := GenerateEditScript(lt, rt, m)

	if len(es.Semantic) != 1 {
		t.Fatalf("expected 1 semantic entry, got %d", len(es.Semantic))
	}
	sc := es.Semantic[0]
	if sc.Verdict != VerdictChanging {
		t.Errorf("literal edit should be %s, got %s (%s)", VerdictChanging, sc.Verdict, sc.Reason)
	}
}

func TestSemantic_RenameOnly_Preserving(t *testing.T) {
	left := linkChildren(buildTree(1, "source_file", "", fnTree(2, "oldName", "1")))
	right := linkChildren(buildTree(101, "source_file", "", fnTree(102, "newName", "1")))

	lt := NewTree(left, "test", nil)
	rt := NewTree(right, "test", nil)

	m := Match(lt, rt, DefaultMatchConfig())
	es := GenerateEditScript(lt, rt, m)

	if len(es.Semantic) != 1 {
		t.Fatalf("expected 1 semantic entry, got %d", len(es.Semantic))
	}
	sc := es.Semantic[0]
	if sc.Verdict != VerdictPreserving {
		t.Errorf("pure rename should be %s, got %s (%s)", VerdictPreserving, sc.Verdict, sc.Reason)
	}
}

func TestSemantic_RelocatedOnly_Preserving(t *testing.T) {
	left := linkChildren(buildTree(1, "source_file", "",
		fnTree(2, "alpha", "1"),
		fnTree(10, "beta", "2"),
	))
	right := linkChildren(buildTree(101, "source_file", "",
		fnTree(110, "beta", "2"),
		fnTree(102, "alpha", "1"),
	))

	lt := NewTree(left, "test", nil)
	rt := NewTree(right, "test", nil)

	m := Match(lt, rt, DefaultMatchConfig())
	es := GenerateEditScript(lt, rt, m)

	if len(es.Semantic) != 1 {
		t.Fatalf("expected 1 semantic entry for the moved function, got %d", len(es.Semantic))
	}
	sc := es.Semantic[0]
	if sc.Verdict != VerdictPreserving || sc.Reason != "relocated only" {
		t.Errorf("pure relocation should be preserving/relocated only, got %s (%s)",
			sc.Verdict, sc.Reason)
	}
}

func TestApproximateRegion_RecordedOnBudget(t *testing.T) {
	// Ten differing statements on each side under one block; budget 1
	// makes the 10x10 unmatched product exceed budget^2.
	mkFn := func(base NodeID, seed string) *Node {
		stmts := make([]*Node, 10)
		for i := range stmts {
			stmts[i] = &Node{
				ID:    base + 2 + NodeID(i),
				Kind:  "stmt",
				Value: seed + string(rune('a'+i)),
			}
		}
		return buildTree(base, "function_declaration", "big",
			buildTree(base+1, "block", "", stmts...))
	}
	left := linkChildren(buildTree(1, "source_file", "", mkFn(2, "x")))
	right := linkChildren(buildTree(101, "source_file", "", mkFn(102, "y")))

	lt := NewTree(left, "test", nil)
	rt := NewTree(right, "test", nil)

	cfg := DefaultMatchConfig()
	cfg.NodeBudget = 1
	m := Match(lt, rt, cfg)
	es := GenerateEditScript(lt, rt, m)

	if len(es.Approximate) == 0 {
		t.Error("expected an approximate region when the budget is exceeded")
	}
}
