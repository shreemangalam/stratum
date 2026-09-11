package core

import "testing"

func TestNewTree_IndexesAllNodes(t *testing.T) {
	child1 := &Node{ID: 2, Kind: "identifier", Label: "x"}
	child2 := &Node{ID: 3, Kind: "identifier", Label: "y"}
	root := &Node{ID: 1, Kind: "block", Children: []*Node{child1, child2}}
	child1.Parent = root
	child2.Parent = root

	tree := NewTree(root, "test", nil)

	if tree.Size() != 3 {
		t.Fatalf("expected 3 nodes, got %d", tree.Size())
	}
	for _, id := range []NodeID{1, 2, 3} {
		if _, ok := tree.NodeMap[id]; !ok {
			t.Errorf("node %d not in map", id)
		}
	}
}

func TestDescendants(t *testing.T) {
	grandchild := &Node{ID: 3, Kind: "literal"}
	child := &Node{ID: 2, Kind: "expr", Children: []*Node{grandchild}}
	grandchild.Parent = child
	root := &Node{ID: 1, Kind: "block", Children: []*Node{child}}
	child.Parent = root

	desc := Descendants(root)
	if len(desc) != 2 {
		t.Fatalf("expected 2 descendants, got %d", len(desc))
	}
}

func TestLeaves(t *testing.T) {
	leaf1 := &Node{ID: 2, Kind: "lit"}
	leaf2 := &Node{ID: 3, Kind: "lit"}
	inner := &Node{ID: 4, Kind: "expr", Children: []*Node{leaf2}}
	root := &Node{ID: 1, Kind: "block", Children: []*Node{leaf1, inner}}

	leaves := Leaves(root)
	if len(leaves) != 2 {
		t.Fatalf("expected 2 leaves, got %d", len(leaves))
	}
}
