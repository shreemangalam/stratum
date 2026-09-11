package core

// Verdict classifies a function-level change. The classifier combines edit
// shape with conservative Go dataflow checks and says "indeterminate" when
// the available evidence is too thin to support a stronger claim.
type Verdict string

const (
	VerdictPreserving    Verdict = "behavior-preserving"
	VerdictChanging      Verdict = "behavior-changing"
	VerdictIndeterminate Verdict = "indeterminate"
)

// SemanticChange is a function-level classification of the edits that
// touched a matched function pair.
type SemanticChange struct {
	LeftNode  *NodeRef `json:"left_node"`
	RightNode *NodeRef `json:"right_node"`
	Verdict   Verdict  `json:"verdict"`
	Reason    string   `json:"reason"`
}

// functionKinds are the node kinds classified at function level.
// Language plugins declaring their own kinds is future work; these
// cover the Go parser, the structural scanners, and XSLT,
// where the template is the unit of behavior.
var functionKinds = map[string]bool{
	"function_declaration": true,
	"method_declaration":   true,
	"function_definition":  true,
	"xsl:template":         true,
}

// classifyFunctions produces one SemanticChange per matched function
// pair that has at least one operation touching it.
func classifyFunctions(left, right *Tree, m *Matching, ops []Operation) []SemanticChange {
	var out []SemanticChange

	for _, fl := range preOrder(left.Root) {
		if !functionKinds[fl.Kind] {
			continue
		}
		rid, ok := m.LeftToRight[fl.ID]
		if !ok {
			continue
		}
		fr := right.NodeMap[rid]

		var inside []Operation
		selfMoved := false
		for _, op := range ops {
			l := resolve(left, op.LeftNode)
			r := resolve(right, op.RightNode)
			if (l != nil && l.ID == fl.ID) || (r != nil && r.ID == fr.ID) {
				if op.Kind == OpMove {
					selfMoved = true
					continue
				}
				// A rename of the function itself participates in the
				// rename-consistency check below.
				inside = append(inside, op)
				continue
			}
			if (l != nil && within(l, fl)) || (r != nil && within(r, fr)) {
				inside = append(inside, op)
			}
		}

		if len(inside) == 0 {
			if selfMoved {
				out = append(out, SemanticChange{
					LeftNode:  nodeRef(fl),
					RightNode: nodeRef(fr),
					Verdict:   VerdictPreserving,
					Reason:    "relocated only",
				})
			}
			continue
		}

		verdict, reason := classifyOps(fl, fr, inside)

		if verdict == VerdictIndeterminate && reason == "statements reordered" {
			v, r := ClassifyReordering(fl, fr, inside, left, right, m)
			verdict, reason = v, r
		}

		out = append(out, SemanticChange{
			LeftNode:  nodeRef(fl),
			RightNode: nodeRef(fr),
			Verdict:   verdict,
			Reason:    reason,
		})
	}

	return out
}

func classifyOps(fl, fr *Node, ops []Operation) (Verdict, string) {
	onlyRenames := true
	onlyRenamesAndMoves := true
	signatureTouched := false

	for _, op := range ops {
		switch op.Kind {
		case OpRename:
		case OpMove:
			onlyRenames = false
		default:
			onlyRenames = false
			onlyRenamesAndMoves = false
		}
		for _, ref := range []*NodeRef{op.LeftNode, op.RightNode} {
			if ref == nil {
				continue
			}
			switch ref.Kind {
			case "parameter", "parameter_list", "result_list",
				"xsl:param", "xsl:with-param":
				signatureTouched = true
			}
		}
	}

	if signatureTouched {
		return VerdictChanging, "signature changed"
	}

	if onlyRenames {
		if consistentRenames(ops) {
			return VerdictPreserving, "consistent renames only"
		}
		return VerdictIndeterminate, "inconsistent renames"
	}

	if onlyRenamesAndMoves {
		// Statement reordering may or may not matter; without dataflow
		// analysis the honest answer is indeterminate.
		return VerdictIndeterminate, "statements reordered"
	}

	return VerdictChanging, "body edited"
}

// consistentRenames reports whether the rename operations form a
// bijection: every old name maps to exactly one new name and vice
// versa. A consistent renaming of identifiers preserves behavior; the
// same old name mapping to two different new names does not.
func consistentRenames(ops []Operation) bool {
	oldToNew := make(map[string]string)
	newToOld := make(map[string]string)
	for _, op := range ops {
		if op.Kind != OpRename || op.LeftNode == nil || op.RightNode == nil {
			continue
		}
		from := op.LeftNode.Label
		to := op.RightNode.Label
		if prev, ok := oldToNew[from]; ok && prev != to {
			return false
		}
		if prev, ok := newToOld[to]; ok && prev != from {
			return false
		}
		oldToNew[from] = to
		newToOld[to] = from
	}
	return true
}

func resolve(t *Tree, ref *NodeRef) *Node {
	if ref == nil {
		return nil
	}
	return t.NodeMap[ref.ID]
}

// within reports whether n is a strict descendant of ancestor.
func within(n, ancestor *Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.ID == ancestor.ID {
			return true
		}
	}
	return false
}
