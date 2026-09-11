package core

import "testing"

func buildTree(id NodeID, kind, label string, children ...*Node) *Node {
	n := &Node{ID: id, Kind: kind, Label: label, Children: children}
	for _, c := range children {
		c.Parent = n
	}
	return n
}

func TestIdenticalTrees_EmptyEditScript(t *testing.T) {
	left := buildTree(1, "root", "",
		buildTree(2, "func", "foo",
			buildTree(3, "body", "")),
	)
	right := buildTree(101, "root", "",
		buildTree(102, "func", "foo",
			buildTree(103, "body", "")),
	)

	lt := NewTree(left, "test", nil)
	rt := NewTree(right, "test", nil)

	m := Match(lt, rt, DefaultMatchConfig())
	es := GenerateEditScript(lt, rt, m)

	for _, op := range es.Operations {
		if op.Kind == OpInsert || op.Kind == OpDelete {
			t.Errorf("identical trees should not produce %s operations", op.Kind)
		}
	}
}

func TestSingleInsertion_Detected(t *testing.T) {
	left := buildTree(1, "root", "",
		buildTree(2, "func", "foo"),
	)
	right := buildTree(101, "root", "",
		buildTree(102, "func", "foo"),
		buildTree(103, "func", "bar"),
	)

	lt := NewTree(left, "test", nil)
	rt := NewTree(right, "test", nil)

	m := Match(lt, rt, DefaultMatchConfig())
	es := GenerateEditScript(lt, rt, m)

	insertCount := 0
	for _, op := range es.Operations {
		if op.Kind == OpInsert {
			insertCount++
		}
	}
	if insertCount == 0 {
		t.Error("expected at least one insert operation")
	}
}

func TestSingleDeletion_Detected(t *testing.T) {
	left := buildTree(1, "root", "",
		buildTree(2, "func", "foo"),
		buildTree(3, "func", "bar"),
	)
	right := buildTree(101, "root", "",
		buildTree(102, "func", "foo"),
	)

	lt := NewTree(left, "test", nil)
	rt := NewTree(right, "test", nil)

	m := Match(lt, rt, DefaultMatchConfig())
	es := GenerateEditScript(lt, rt, m)

	deleteCount := 0
	for _, op := range es.Operations {
		if op.Kind == OpDelete {
			deleteCount++
		}
	}
	if deleteCount == 0 {
		t.Error("expected at least one delete operation")
	}
}

func TestRename_Detected(t *testing.T) {
	left := buildTree(1, "root", "",
		buildTree(2, "func", "oldName"),
	)
	right := buildTree(101, "root", "",
		buildTree(102, "func", "newName"),
	)

	lt := NewTree(left, "test", nil)
	rt := NewTree(right, "test", nil)

	m := Match(lt, rt, DefaultMatchConfig())
	es := GenerateEditScript(lt, rt, m)

	renameCount := 0
	for _, op := range es.Operations {
		if op.Kind == OpRename {
			renameCount++
		}
	}
	if renameCount == 0 {
		t.Error("expected a rename operation")
	}
}

func TestMove_Detected(t *testing.T) {
	inner := buildTree(4, "stmt", "x")
	left := buildTree(1, "root", "",
		buildTree(2, "block_a", "",
			inner,
		),
		buildTree(3, "block_b", ""),
	)

	movedInner := buildTree(104, "stmt", "x")
	right := buildTree(101, "root", "",
		buildTree(102, "block_a", ""),
		buildTree(103, "block_b", "",
			movedInner,
		),
	)

	lt := NewTree(left, "test", nil)
	rt := NewTree(right, "test", nil)

	m := Match(lt, rt, DefaultMatchConfig())
	es := GenerateEditScript(lt, rt, m)

	moveCount := 0
	for _, op := range es.Operations {
		if op.Kind == OpMove {
			moveCount++
		}
	}
	if moveCount == 0 {
		t.Error("expected a move operation for the relocated node")
	}
}

func TestInsertedSubtree_SingleOperation(t *testing.T) {
	left := buildTree(1, "root", "",
		buildTree(2, "func", "foo",
			buildTree(3, "body", "")),
	)
	right := buildTree(101, "root", "",
		buildTree(102, "func", "foo",
			buildTree(103, "body", "")),
		buildTree(104, "func", "bar",
			buildTree(105, "body", "",
				buildTree(106, "stmt", "a"),
				buildTree(107, "stmt", "b"))),
	)

	lt := NewTree(left, "test", nil)
	rt := NewTree(right, "test", nil)

	m := Match(lt, rt, DefaultMatchConfig())
	es := GenerateEditScript(lt, rt, m)

	inserts := 0
	for _, op := range es.Operations {
		if op.Kind == OpInsert {
			inserts++
			if op.RightNode == nil || op.RightNode.Label != "bar" {
				t.Errorf("insert should report the subtree root, got %+v", op.RightNode)
			}
		}
	}
	if inserts != 1 {
		t.Errorf("inserted subtree should produce exactly 1 insert, got %d", inserts)
	}
}

func TestSwappedSiblings_SingleMove(t *testing.T) {
	left := buildTree(1, "root", "",
		buildTree(2, "func", "alpha",
			buildTree(3, "body", "")),
		buildTree(4, "func", "beta",
			buildTree(5, "body", "")),
	)
	right := buildTree(101, "root", "",
		buildTree(102, "func", "beta",
			buildTree(103, "body", "")),
		buildTree(104, "func", "alpha",
			buildTree(105, "body", "")),
	)

	lt := NewTree(left, "test", nil)
	rt := NewTree(right, "test", nil)

	m := Match(lt, rt, DefaultMatchConfig())
	es := GenerateEditScript(lt, rt, m)

	moves := 0
	for _, op := range es.Operations {
		if op.Kind == OpMove {
			moves++
		}
	}
	if moves != 1 {
		t.Errorf("a swap of two siblings needs exactly 1 move, got %d", moves)
	}
}

func TestInsertionBefore_NoMoves(t *testing.T) {
	left := buildTree(1, "root", "",
		buildTree(2, "func", "alpha",
			buildTree(3, "body", "")),
		buildTree(4, "func", "beta",
			buildTree(5, "body", "")),
	)
	right := buildTree(101, "root", "",
		buildTree(102, "func", "inserted",
			buildTree(103, "body", "")),
		buildTree(104, "func", "alpha",
			buildTree(105, "body", "")),
		buildTree(106, "func", "beta",
			buildTree(107, "body", "")),
	)

	lt := NewTree(left, "test", nil)
	rt := NewTree(right, "test", nil)

	m := Match(lt, rt, DefaultMatchConfig())
	es := GenerateEditScript(lt, rt, m)

	for _, op := range es.Operations {
		if op.Kind == OpMove {
			t.Errorf("an insertion must not mark shifted siblings as moved: %+v -> %+v",
				op.LeftNode, op.RightNode)
		}
	}
}

func TestMatch_Deterministic(t *testing.T) {
	build := func(base NodeID) (*Tree, *Tree) {
		left := buildTree(base, "root", "",
			buildTree(base+1, "func", "a", buildTree(base+2, "params", "")),
			buildTree(base+3, "func", "b", buildTree(base+4, "params", "")),
			buildTree(base+5, "func", "c", buildTree(base+6, "params", "")),
		)
		right := buildTree(base+100, "root", "",
			buildTree(base+101, "func", "c", buildTree(base+102, "params", "")),
			buildTree(base+103, "func", "a", buildTree(base+104, "params", "")),
			buildTree(base+105, "func", "d", buildTree(base+106, "params", "")),
		)
		return NewTree(left, "test", nil), NewTree(right, "test", nil)
	}

	lt1, rt1 := build(1)
	m1 := Match(lt1, rt1, DefaultMatchConfig())
	es1 := GenerateEditScript(lt1, rt1, m1)

	for range 20 {
		lt2, rt2 := build(1)
		m2 := Match(lt2, rt2, DefaultMatchConfig())
		es2 := GenerateEditScript(lt2, rt2, m2)

		if len(es1.Operations) != len(es2.Operations) {
			t.Fatalf("operation count varies between runs: %d vs %d",
				len(es1.Operations), len(es2.Operations))
		}
		for i, op := range es1.Operations {
			other := es2.Operations[i]
			if op.Kind != other.Kind {
				t.Fatalf("operation %d kind varies between runs: %s vs %s", i, op.Kind, other.Kind)
			}
		}
	}
}

func TestTopDownMatch_IdenticalSubtrees(t *testing.T) {
	child := buildTree(2, "func", "foo", buildTree(3, "body", ""))
	left := buildTree(1, "root", "", child)

	childR := buildTree(102, "func", "foo", buildTree(103, "body", ""))
	right := buildTree(101, "root", "", childR)

	lt := NewTree(left, "test", nil)
	rt := NewTree(right, "test", nil)
	HashTree(lt)
	HashTree(rt)

	m := NewMatching()
	topDownMatch(lt, rt, m)

	if len(m.LeftToRight) < 2 {
		t.Errorf("expected at least 2 matches from identical subtrees, got %d", len(m.LeftToRight))
	}
}
