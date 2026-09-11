package core

// OpKind classifies an edit operation.
type OpKind string

const (
	// OpInsert adds a node that exists only in the right tree.
	OpInsert OpKind = "insert"
	// OpDelete removes a node that exists only in the left tree.
	OpDelete OpKind = "delete"
	// OpMove relocates a matched node within the tree.
	OpMove OpKind = "move"
	// OpRename changes the label of a matched node.
	OpRename OpKind = "rename"
	// OpUpdate changes the value of a matched node.
	OpUpdate OpKind = "update"
	// OpAlign records a structural alignment without a content change.
	OpAlign OpKind = "align"
)

// NodeRef references a node in its source tree.
type NodeRef struct {
	ID       NodeID   `json:"id"`
	Path     string   `json:"path"`
	Kind     string   `json:"kind"`
	Label    string   `json:"label"`
	Location Location `json:"location"`
}

// Operation is a single edit script entry.
type Operation struct {
	Kind      OpKind   `json:"kind"`
	LeftNode  *NodeRef `json:"left_node,omitempty"`
	RightNode *NodeRef `json:"right_node,omitempty"`
}

// Region marks a subtree that fell back to line diff.
type Region struct {
	LeftSpan  Span `json:"left_span"`
	RightSpan Span `json:"right_span"`
}

// EditScript is the output of the diff pipeline.
type EditScript struct {
	Operations  []Operation      `json:"operations"`
	LeftRoot    NodeID           `json:"left_root"`
	RightRoot   NodeID           `json:"right_root"`
	Approximate []Region         `json:"approximate,omitempty"`
	Semantic    []SemanticChange `json:"semantic,omitempty"`
}

// GenerateEditScript produces an edit script from a matching between two
// trees. Operations are reported at the top-most level that explains the
// change: an unmatched subtree is one delete or insert, not one per
// descendant, and a node that travels with its moved parent is not itself
// a move. Traversal is document order, so output is deterministic.
func GenerateEditScript(left, right *Tree, m *Matching) *EditScript {
	es := &EditScript{
		// Non-nil so an empty script marshals as [], not null — the
		// contract declares operations as a required array.
		Operations: []Operation{},
		LeftRoot:   left.Root.ID,
		RightRoot:  right.Root.ID,
	}

	for _, ln := range preOrder(left.Root) {
		rid, matched := m.LeftToRight[ln.ID]
		if !matched {
			// Only the top-most unmatched node is a delete; its subtree
			// is implied.
			if ln.Parent == nil || m.LeftMatched(ln.Parent.ID) {
				es.Operations = append(es.Operations, Operation{
					Kind:     OpDelete,
					LeftNode: nodeRef(ln),
				})
			}
			continue
		}
		rn := right.NodeMap[rid]

		if ln.Label != "" && rn.Label != "" && ln.Label != rn.Label {
			es.Operations = append(es.Operations, Operation{
				Kind:      OpRename,
				LeftNode:  nodeRef(ln),
				RightNode: nodeRef(rn),
			})
		}

		if ln.Value != rn.Value && len(ln.Children) == 0 && len(rn.Children) == 0 {
			es.Operations = append(es.Operations, Operation{
				Kind:      OpUpdate,
				LeftNode:  nodeRef(ln),
				RightNode: nodeRef(rn),
			})
		}

		if crossParentMove(ln, rn, m) {
			es.Operations = append(es.Operations, Operation{
				Kind:      OpMove,
				LeftNode:  nodeRef(ln),
				RightNode: nodeRef(rn),
			})
		}

		// Reordering among this pair's children. Nodes that changed
		// parents are handled by crossParentMove above.
		for _, pair := range orderingMoves(ln, rn, m) {
			es.Operations = append(es.Operations, Operation{
				Kind:      OpMove,
				LeftNode:  nodeRef(pair[0]),
				RightNode: nodeRef(pair[1]),
			})
		}
	}

	for _, rn := range preOrder(right.Root) {
		if !m.RightMatched(rn.ID) && (rn.Parent == nil || m.RightMatched(rn.Parent.ID)) {
			es.Operations = append(es.Operations, Operation{
				Kind:      OpInsert,
				RightNode: nodeRef(rn),
			})
		}
	}

	for _, pair := range m.Approximate {
		ln := left.NodeMap[pair[0]]
		rn := right.NodeMap[pair[1]]
		if ln == nil || rn == nil {
			continue
		}
		es.Approximate = append(es.Approximate, Region{
			LeftSpan:  ln.Span,
			RightSpan: rn.Span,
		})
	}

	es.Semantic = classifyFunctions(left, right, m, es.Operations)

	return es
}

// crossParentMove reports whether a matched pair sits under parents that
// are not matched to each other. Children of a moved node have consistent
// parent pairs and are not themselves flagged, so only the top-most
// relocated node reports.
func crossParentMove(ln, rn *Node, m *Matching) bool {
	if ln.Parent == nil && rn.Parent == nil {
		return false
	}
	if ln.Parent == nil || rn.Parent == nil {
		return true
	}
	return m.LeftToRight[ln.Parent.ID] != rn.Parent.ID
}

// orderingMoves detects reordering among the matched children of a
// matched pair. The pairs are listed in left order; the longest
// increasing subsequence of their right-side positions is the stable
// backbone, and every pair outside it moved. An insertion or deletion
// shifts positions without breaking relative order, so it produces no
// moves; a swap produces one.
func orderingMoves(lp, rp *Node, m *Matching) [][2]*Node {
	rightIndex := make(map[NodeID]int, len(rp.Children))
	for i, rc := range rp.Children {
		rightIndex[rc.ID] = i
	}

	type pair struct {
		l, r *Node
		ri   int
	}
	var pairs []pair
	for _, lc := range lp.Children {
		rid, ok := m.LeftToRight[lc.ID]
		if !ok {
			continue
		}
		ri, sameParent := rightIndex[rid]
		if !sameParent {
			continue
		}
		var rc *Node
		for _, c := range rp.Children {
			if c.ID == rid {
				rc = c
				break
			}
		}
		pairs = append(pairs, pair{l: lc, r: rc, ri: ri})
	}
	if len(pairs) < 2 {
		return nil
	}

	// Longest increasing subsequence over right positions, weighted by
	// subtree size so a small node moves around a large one, not the
	// other way around.
	weights := make([]int, len(pairs))
	for i, p := range pairs {
		weights[i] = 1 + len(Descendants(p.l))
	}
	best := make([]int, len(pairs))
	prev := make([]int, len(pairs))
	for i := range pairs {
		best[i] = weights[i]
		prev[i] = -1
		for j := range i {
			if pairs[j].ri < pairs[i].ri && best[j]+weights[i] > best[i] {
				best[i] = best[j] + weights[i]
				prev[i] = j
			}
		}
	}
	end := 0
	for i := range best {
		if best[i] > best[end] {
			end = i
		}
	}
	stable := make(map[int]bool, len(pairs))
	for i := end; i >= 0; i = prev[i] {
		stable[i] = true
		if prev[i] == -1 {
			break
		}
	}

	var moves [][2]*Node
	for i, p := range pairs {
		if !stable[i] {
			moves = append(moves, [2]*Node{p.l, p.r})
		}
	}
	return moves
}

func childIndex(n *Node) int {
	if n.Parent == nil {
		return 0
	}
	for i, c := range n.Parent.Children {
		if c.ID == n.ID {
			return i
		}
	}
	return -1
}

func nodeRef(n *Node) *NodeRef {
	return &NodeRef{
		ID:    n.ID,
		Path:  nodePath(n),
		Kind:  n.Kind,
		Label: n.Label,
		Location: Location{
			Line:   n.Span.Start.Line,
			Column: n.Span.Start.Column,
			Offset: n.Span.Start.Offset,
		},
	}
}

func nodePath(n *Node) string {
	if n.Parent == nil {
		return n.Kind
	}
	return nodePath(n.Parent) + " > " + n.Kind
}
