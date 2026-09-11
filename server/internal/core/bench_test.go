package core

import (
	"fmt"
	"testing"
)

// generateTree builds a synthetic tree with the given breadth and depth.
// Each internal node has `breadth` children. Total nodes ≈ breadth^depth.
func generateTree(idStart NodeID, breadth, depth int) (*Node, NodeID) {
	id := idStart
	root := &Node{ID: id, Kind: "root", Label: "root"}
	id++

	if depth <= 0 {
		return root, id
	}

	for i := range breadth {
		child, nextID := generateSubtree(id, breadth, depth-1, i)
		child.Parent = root
		root.Children = append(root.Children, child)
		id = nextID
	}
	return root, id
}

func generateSubtree(idStart NodeID, breadth, depth, index int) (*Node, NodeID) {
	id := idStart
	n := &Node{
		ID:    id,
		Kind:  "func",
		Label: fmt.Sprintf("fn_%d", index),
	}
	id++

	if depth <= 0 {
		n.Kind = "leaf"
		n.Value = fmt.Sprintf("val_%d", index)
		return n, id
	}

	for i := range breadth {
		child, nextID := generateSubtree(id, breadth, depth-1, i)
		child.Parent = n
		n.Children = append(n.Children, child)
		id = nextID
	}
	return n, id
}

// generateModifiedTree creates a right-side tree that differs from the
// left-side tree: one child is renamed, one is moved (reordered), and
// one new child is inserted. This exercises all three matching phases.
func generateModifiedTree(idStart NodeID, breadth, depth int) (*Node, NodeID) {
	root, nextID := generateTree(idStart, breadth, depth)
	if len(root.Children) < 2 {
		return root, nextID
	}

	// Rename first child.
	root.Children[0].Label = "renamed_fn"

	// Swap the first two children (move).
	root.Children[0], root.Children[1] = root.Children[1], root.Children[0]
	root.Children[0].Parent = root
	root.Children[1].Parent = root

	// Insert a new child.
	newChild := &Node{
		ID:     nextID,
		Kind:   "func",
		Label:  "new_fn",
		Parent: root,
	}
	nextID++
	root.Children = append(root.Children, newChild)

	return root, nextID
}

func benchmarkMatchAndEditScript(b *testing.B, breadth, depth int) {
	leftRoot, nextID := generateTree(1, breadth, depth)
	rightRoot, _ := generateModifiedTree(nextID, breadth, depth)

	lt := NewTree(leftRoot, "bench", nil)
	rt := NewTree(rightRoot, "bench", nil)

	b.ReportMetric(float64(lt.Size()), "left_nodes")
	b.ReportMetric(float64(rt.Size()), "right_nodes")

	cfg := DefaultMatchConfig()

	b.ResetTimer()
	for range b.N {
		m := Match(lt, rt, cfg)
		GenerateEditScript(lt, rt, m)
	}
}

func BenchmarkMatch_10nodes(b *testing.B)   { benchmarkMatchAndEditScript(b, 3, 2) }
func BenchmarkMatch_40nodes(b *testing.B)   { benchmarkMatchAndEditScript(b, 5, 2) }
func BenchmarkMatch_120nodes(b *testing.B)  { benchmarkMatchAndEditScript(b, 4, 3) }
func BenchmarkMatch_500nodes(b *testing.B)  { benchmarkMatchAndEditScript(b, 7, 3) }
func BenchmarkMatch_1500nodes(b *testing.B) { benchmarkMatchAndEditScript(b, 6, 4) }

func BenchmarkHash_120nodes(b *testing.B) {
	root, _ := generateTree(1, 4, 3)
	t := NewTree(root, "bench", nil)
	b.ReportMetric(float64(t.Size()), "nodes")
	b.ResetTimer()
	for range b.N {
		HashTree(t)
	}
}

func BenchmarkHash_1500nodes(b *testing.B) {
	root, _ := generateTree(1, 6, 4)
	t := NewTree(root, "bench", nil)
	b.ReportMetric(float64(t.Size()), "nodes")
	b.ResetTimer()
	for range b.N {
		HashTree(t)
	}
}

func BenchmarkIdenticalTrees_120nodes(b *testing.B) {
	root1, nextID := generateTree(1, 4, 3)
	root2, _ := generateTree(nextID, 4, 3)
	lt := NewTree(root1, "bench", nil)
	rt := NewTree(root2, "bench", nil)
	cfg := DefaultMatchConfig()

	b.ReportMetric(float64(lt.Size()), "nodes")
	b.ResetTimer()
	for range b.N {
		m := Match(lt, rt, cfg)
		GenerateEditScript(lt, rt, m)
	}
}

func BenchmarkIdenticalTrees_1500nodes(b *testing.B) {
	root1, nextID := generateTree(1, 6, 4)
	root2, _ := generateTree(nextID, 6, 4)
	lt := NewTree(root1, "bench", nil)
	rt := NewTree(root2, "bench", nil)
	cfg := DefaultMatchConfig()

	b.ReportMetric(float64(lt.Size()), "nodes")
	b.ResetTimer()
	for range b.N {
		m := Match(lt, rt, cfg)
		GenerateEditScript(lt, rt, m)
	}
}
