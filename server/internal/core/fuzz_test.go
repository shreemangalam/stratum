package core

import (
	"fmt"
	"testing"
)

func buildFuzzTree(data []byte, idStart NodeID) (*Tree, NodeID) {
	if len(data) == 0 {
		root := &Node{ID: idStart, Kind: "root"}
		return NewTree(root, "fuzz", nil), idStart + 1
	}

	root := &Node{ID: idStart, Kind: "root", Label: "root"}
	id := idStart + 1

	breadth := int(data[0]%8) + 1
	for i := range breadth {
		if int(i+1) >= len(data) {
			break
		}
		b := data[(i+1)%len(data)]
		child := &Node{
			ID:     id,
			Kind:   fmt.Sprintf("kind_%d", b%4),
			Label:  fmt.Sprintf("label_%d", b%16),
			Value:  fmt.Sprintf("val_%d", b),
			Parent: root,
		}
		id++
		root.Children = append(root.Children, child)

		depth := int(b%3) + 1
		for d := range depth {
			if int(i+d+2) >= len(data) {
				break
			}
			gb := data[(i+d+2)%len(data)]
			grandchild := &Node{
				ID:     id,
				Kind:   fmt.Sprintf("leaf_%d", gb%3),
				Label:  fmt.Sprintf("l_%d", gb%8),
				Value:  fmt.Sprintf("v_%d", gb),
				Parent: child,
			}
			id++
			child.Children = append(child.Children, grandchild)
		}
	}

	return NewTree(root, "fuzz", nil), id
}

func FuzzMatchAndEditScript(f *testing.F) {
	f.Add([]byte{3, 10, 20, 30}, []byte{3, 10, 25, 30, 40})
	f.Add([]byte{1, 5}, []byte{1, 5})
	f.Add([]byte{}, []byte{1})
	f.Add([]byte{1}, []byte{})
	f.Add([]byte{7, 1, 2, 3, 4, 5, 6, 7}, []byte{7, 7, 6, 5, 4, 3, 2, 1})

	f.Fuzz(func(t *testing.T, left, right []byte) {
		lt, nextID := buildFuzzTree(left, 1)
		rt, _ := buildFuzzTree(right, nextID)

		cfg := DefaultMatchConfig()
		m := Match(lt, rt, cfg)
		es := GenerateEditScript(lt, rt, m)
		_ = es
	})
}
