package core

// DefUseSet tracks which identifiers a statement defines (writes to),
// which it uses (reads from), and whether it contains operations with
// potential side effects (function calls, goroutines, defers, control
// flow). Two statements can be safely reordered only if their def/use
// sets are independent and neither has side effects.
type DefUseSet struct {
	Defs           map[string]bool
	Uses           map[string]bool
	HasSideEffects bool
}

// ExtractDefUse walks a statement subtree and collects identifiers into
// defs and uses. Identifiers under assignment_lhs are defs; all others
// are uses. Nodes that imply side effects or control flow set
// HasSideEffects.
func ExtractDefUse(stmt *Node) DefUseSet {
	s := DefUseSet{
		Defs: make(map[string]bool),
		Uses: make(map[string]bool),
	}
	extractDefUseRecursive(stmt, false, &s)
	return s
}

func extractDefUseRecursive(n *Node, inLHS bool, s *DefUseSet) {
	if n == nil {
		return
	}

	switch n.Kind {
	case "assignment_lhs":
		for _, c := range n.Children {
			extractDefUseRecursive(c, true, s)
		}
		return

	case "assignment_rhs":
		for _, c := range n.Children {
			extractDefUseRecursive(c, false, s)
		}
		return

	case "identifier":
		if inLHS {
			s.Defs[n.Label] = true
		} else {
			s.Uses[n.Label] = true
		}
		return

	case "call_expression", "go_statement", "defer_statement":
		s.HasSideEffects = true

	case "return_statement":
		s.HasSideEffects = true

	case "if_statement", "for_statement", "range_statement",
		"switch_statement", "case_clause":
		s.HasSideEffects = true
	}

	for _, c := range n.Children {
		extractDefUseRecursive(c, inLHS, s)
	}
}

// DefUseSetsIndependent reports whether two def/use sets have no data
// dependencies: no write-write, write-read, or read-write conflicts.
// Shared reads (use-use) are not a conflict.
func DefUseSetsIndependent(a, b DefUseSet) bool {
	for name := range a.Defs {
		if b.Defs[name] || b.Uses[name] {
			return false
		}
	}
	for name := range b.Defs {
		if a.Uses[name] {
			return false
		}
	}
	return true
}

const maxDataflowStatements = 50

// ClassifyReordering refines the "statements reordered" verdict using
// def/use analysis. It returns a non-indeterminate verdict only when
// all reordered statement pairs are pure (no side effects) and their
// def/use independence can be determined.
func ClassifyReordering(fl, fr *Node, ops []Operation, left, right *Tree, m *Matching) (Verdict, string) {
	if left.Language != "go" {
		return VerdictIndeterminate, "statements reordered"
	}

	leftBlock := findBodyBlock(fl)
	rightBlock := findBodyBlock(fr)
	if leftBlock == nil || rightBlock == nil {
		return VerdictIndeterminate, "statements reordered"
	}

	if len(leftBlock.Children) > maxDataflowStatements || len(rightBlock.Children) > maxDataflowStatements {
		return VerdictIndeterminate, "statements reordered"
	}

	// Verify all moves are direct children of the body block.
	leftBlockChildren := make(map[NodeID]bool, len(leftBlock.Children))
	for _, c := range leftBlock.Children {
		leftBlockChildren[c.ID] = true
	}
	for _, op := range ops {
		if op.Kind != OpMove {
			continue
		}
		if op.LeftNode == nil {
			continue
		}
		ln := left.NodeMap[op.LeftNode.ID]
		if ln == nil || !leftBlockChildren[ln.ID] {
			return VerdictIndeterminate, "statements reordered"
		}
	}

	// Build position maps: each statement's index in left and right blocks.
	leftPos := make(map[NodeID]int, len(leftBlock.Children))
	for i, c := range leftBlock.Children {
		leftPos[c.ID] = i
	}

	rightPos := make(map[NodeID]int, len(rightBlock.Children))
	for i, c := range rightBlock.Children {
		rightPos[c.ID] = i
	}

	// Extract def/use sets for each left-block statement.
	defUseSets := make(map[NodeID]DefUseSet, len(leftBlock.Children))
	for _, c := range leftBlock.Children {
		defUseSets[c.ID] = ExtractDefUse(c)
	}

	// Find pairs of matched statements whose relative order reversed.
	type matchedStmt struct {
		leftID  NodeID
		leftIdx int
	}
	var matched []matchedStmt
	for _, c := range leftBlock.Children {
		rid, ok := m.LeftToRight[c.ID]
		if !ok {
			continue
		}
		if _, inRight := rightPos[rid]; !inRight {
			continue
		}
		matched = append(matched, matchedStmt{leftID: c.ID, leftIdx: leftPos[c.ID]})
	}

	for i := 0; i < len(matched); i++ {
		for j := i + 1; j < len(matched); j++ {
			si := matched[i]
			sj := matched[j]

			riID := m.LeftToRight[si.leftID]
			rjID := m.LeftToRight[sj.leftID]
			ri := rightPos[riID]
			rj := rightPos[rjID]

			leftBefore := si.leftIdx < sj.leftIdx
			rightBefore := ri < rj
			if leftBefore == rightBefore {
				continue
			}

			// This pair reversed order. Check independence.
			duI := defUseSets[si.leftID]
			duJ := defUseSets[sj.leftID]

			if duI.HasSideEffects || duJ.HasSideEffects {
				return VerdictIndeterminate, "statements reordered"
			}

			if !DefUseSetsIndependent(duI, duJ) {
				return VerdictChanging, "dependent statements reordered"
			}
		}
	}

	return VerdictPreserving, "independent statements reordered"
}

func findBodyBlock(fn *Node) *Node {
	for _, c := range fn.Children {
		if c.Kind == "block" {
			return c
		}
	}
	return nil
}
