package core

import "slices"

// Matching maps nodes from the left tree to nodes in the right tree.
// Approximate lists matched pairs whose interiors exceeded the node
// budget and were left unaligned; their differences degrade to plain
// insert/delete and the edit script marks the region approximate.
type Matching struct {
	LeftToRight map[NodeID]NodeID
	RightToLeft map[NodeID]NodeID
	Approximate [][2]NodeID
}

// NewMatching creates an empty matching.
func NewMatching() *Matching {
	return &Matching{
		LeftToRight: make(map[NodeID]NodeID),
		RightToLeft: make(map[NodeID]NodeID),
	}
}

// Add records a match between a left node and a right node.
func (m *Matching) Add(left, right NodeID) {
	m.LeftToRight[left] = right
	m.RightToLeft[right] = left
}

// LeftMatched returns true if the left node has a match.
func (m *Matching) LeftMatched(id NodeID) bool {
	_, ok := m.LeftToRight[id]
	return ok
}

// RightMatched returns true if the right node has a match.
func (m *Matching) RightMatched(id NodeID) bool {
	_, ok := m.RightToLeft[id]
	return ok
}

// MatchConfig holds tuning parameters for the matching algorithm.
type MatchConfig struct {
	DiceThreshold float64
	NodeBudget    int
}

// DefaultMatchConfig returns sensible defaults.
func DefaultMatchConfig() MatchConfig {
	return MatchConfig{
		DiceThreshold: 0.6,
		NodeBudget:    100,
	}
}

// Match runs all three matching phases and returns the combined matching.
// All phases traverse in document order, so the result is deterministic
// for a given pair of trees.
func Match(left, right *Tree, cfg MatchConfig) *Matching {
	HashTree(left)
	HashTree(right)

	m := NewMatching()
	topDownMatch(left, right, m)
	bottomUpMatch(left, right, m, cfg.DiceThreshold)
	recoveryMatch(left, right, m, cfg.NodeBudget, cfg.DiceThreshold)
	return m
}

// Phase 1: Top-down hash matching.
//
// A left node whose Merkle hash appears on the right is matched together
// with its entire subtree: identical hashes guarantee identical shape, so
// the descendant mapping is forced. Matching subtrees as units prevents
// descendants of one occurrence from scattering across hash-identical
// copies elsewhere in the file (e.g. two functions with the same
// parameter list).
//
// When several right-side candidates share the hash, the one at the same
// child index is preferred, then document order breaks the tie.
func topDownMatch(left, right *Tree, m *Matching) {
	rightByHash := make(map[string][]*Node)
	for _, n := range preOrder(right.Root) {
		if n.Hash != "" {
			rightByHash[n.Hash] = append(rightByHash[n.Hash], n)
		}
	}

	// Larger subtrees first: a whole unchanged function must claim its
	// statements before a smaller orphaned statement elsewhere can grab
	// one of the hash-identical copies inside it. The sort is stable, so
	// equal sizes keep document order and the result stays deterministic.
	leftNodes := preOrder(left.Root)
	sizes := make(map[NodeID]int, len(leftNodes))
	for _, n := range leftNodes {
		sizes[n.ID] = len(Descendants(n))
	}
	slices.SortStableFunc(leftNodes, func(a, b *Node) int {
		return sizes[b.ID] - sizes[a.ID]
	})

	for _, ln := range leftNodes {
		if ln.Hash == "" || m.LeftMatched(ln.ID) {
			continue
		}

		var best *Node
		for _, rn := range rightByHash[ln.Hash] {
			if m.RightMatched(rn.ID) {
				continue
			}
			if best == nil {
				best = rn
			}
			if childIndex(rn) == childIndex(ln) {
				best = rn
				break
			}
		}
		if best != nil {
			matchSubtrees(ln, best, m)
		}
	}
}

// matchSubtrees maps two hash-identical subtrees node by node.
func matchSubtrees(l, r *Node, m *Matching) {
	m.Add(l.ID, r.ID)
	for i, lc := range l.Children {
		matchSubtrees(lc, r.Children[i], m)
	}
}

// Phase 2: Bottom-up dice coefficient matching.
//
// Containers whose descendants are already substantially matched to the
// descendants of a right-side container are matched to it. Leaves are
// excluded: they carry no descendant evidence and are recovered inside
// matched containers in phase 3, which keeps a leaf from pairing with an
// arbitrary distant twin.
func bottomUpMatch(left, right *Tree, m *Matching, threshold float64) {
	rightByKind := make(map[string][]*Node)
	for _, n := range preOrder(right.Root) {
		rightByKind[n.Kind] = append(rightByKind[n.Kind], n)
	}

	for _, ln := range postOrder(left.Root) {
		if m.LeftMatched(ln.ID) || len(ln.Children) == 0 {
			continue
		}

		var best *Node
		var bestScore float64

		for _, rn := range rightByKind[ln.Kind] {
			if m.RightMatched(rn.ID) {
				continue
			}
			score := diceCoefficient(ln, rn, m)
			if score > bestScore {
				bestScore = score
				best = rn
				continue
			}
			// Equal score: prefer the candidate under the parent that
			// matches this node's parent.
			if score == bestScore && best != nil && score >= threshold &&
				!parentsMatched(ln, best, m) && parentsMatched(ln, rn, m) {
				best = rn
			}
		}

		if bestScore >= threshold && best != nil {
			m.Add(ln.ID, best.ID)
		}
	}
}

func parentsMatched(ln, rn *Node, m *Matching) bool {
	if ln.Parent == nil || rn.Parent == nil {
		return false
	}
	return m.LeftToRight[ln.Parent.ID] == rn.Parent.ID
}

func diceCoefficient(left, right *Node, m *Matching) float64 {
	leftDesc := Descendants(left)
	rightDesc := Descendants(right)
	if len(leftDesc)+len(rightDesc) == 0 {
		return 0.0
	}

	rightSet := make(map[NodeID]struct{}, len(rightDesc))
	for _, rd := range rightDesc {
		rightSet[rd.ID] = struct{}{}
	}

	matched := 0
	for _, ld := range leftDesc {
		if rid, ok := m.LeftToRight[ld.ID]; ok {
			if _, in := rightSet[rid]; in {
				matched++
			}
		}
	}

	return 2.0 * float64(matched) / float64(len(leftDesc)+len(rightDesc))
}

// Phase 3: Recovery inside matched containers.
//
// For every pair matched by phases 1-2, unmatched children are aligned
// with an LCS over child kinds and the alignment recurses into the new
// pairs. Recovery never reaches outside a matched pair, so an unmatched
// subtree stays whole (a clean insert or delete) instead of donating
// leaves to distant lookalikes.
//
// If the unmatched populations of a pair exceed the budget, that pair is
// skipped and its differences degrade to plain insert/delete.
func recoveryMatch(left, right *Tree, m *Matching, budget int, threshold float64) {
	// Two versions of one file share a root; seed it so recovery has a
	// starting pair even when nothing else matched.
	if !m.LeftMatched(left.Root.ID) && !m.RightMatched(right.Root.ID) &&
		left.Root.Kind == right.Root.Kind {
		m.Add(left.Root.ID, right.Root.ID)
	}

	for _, ln := range preOrder(left.Root) {
		rid, ok := m.LeftToRight[ln.ID]
		if !ok {
			continue
		}
		alignChildren(ln, right.NodeMap[rid], m, budget, threshold)
	}
}

// alignChildren aligns the unmatched children of an already-matched pair
// and recurses into every pair it creates.
func alignChildren(l, r *Node, m *Matching, budget int, threshold float64) {
	n := len(l.Children)
	o := len(r.Children)
	if n == 0 || o == 0 {
		return
	}

	lu := countUnmatched(l, m, true)
	ru := countUnmatched(r, m, false)
	if lu == 0 || ru == 0 {
		return
	}
	if lu*ru > budget*budget {
		m.Approximate = append(m.Approximate, [2]NodeID{l.ID, r.ID})
		return
	}

	// LCS over children: pairs already matched to each other anchor the
	// alignment with their subtree weight; unmatched same-kind pairs are
	// candidates.
	score := func(lc, rc *Node) int {
		if rid, ok := m.LeftToRight[lc.ID]; ok {
			if rid == rc.ID {
				return 1 + len(Descendants(lc))
			}
			return 0
		}
		if m.RightMatched(rc.ID) || lc.Kind != rc.Kind {
			return 0
		}
		return 1
	}

	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, o+1)
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= o; j++ {
			s := score(l.Children[i-1], r.Children[j-1])
			dp[i][j] = max(dp[i-1][j], dp[i][j-1])
			if dp[i-1][j-1]+s > dp[i][j] {
				dp[i][j] = dp[i-1][j-1] + s
			}
		}
	}

	i, j := n, o
	for i > 0 && j > 0 {
		lc := l.Children[i-1]
		rc := r.Children[j-1]
		s := score(lc, rc)
		if s > 0 && dp[i][j] == dp[i-1][j-1]+s {
			if !m.LeftMatched(lc.ID) {
				m.Add(lc.ID, rc.ID)
				alignChildren(lc, rc, m, budget, threshold)
			}
			i--
			j--
		} else if dp[i-1][j] >= dp[i][j-1] {
			i--
		} else {
			j--
		}
	}

	// The LCS preserves order, so a reordered sibling never aligns.
	// Pair leftovers by kind and label; the reordering itself is
	// reported as a move by the edit script.
	for _, lc := range l.Children {
		if m.LeftMatched(lc.ID) {
			continue
		}
		for _, rc := range r.Children {
			if m.RightMatched(rc.ID) || rc.Kind != lc.Kind || rc.Label != lc.Label {
				continue
			}
			m.Add(lc.ID, rc.ID)
			alignChildren(lc, rc, m, budget, threshold)
			break
		}
	}

	// Last pass: same kind, different label — a renamed sibling whose
	// body was also edited (so no hash match and no label match). Pair
	// by leaf-content similarity so it surfaces as rename + inner edits
	// instead of delete + insert. Best candidate wins; document order
	// breaks ties.
	for _, lc := range l.Children {
		if m.LeftMatched(lc.ID) {
			continue
		}
		var best *Node
		var bestScore float64
		for _, rc := range r.Children {
			if m.RightMatched(rc.ID) || rc.Kind != lc.Kind {
				continue
			}
			if score := leafSimilarity(lc, rc); score > bestScore {
				bestScore = score
				best = rc
			}
		}
		if best != nil && bestScore >= threshold {
			m.Add(lc.ID, best.ID)
			alignChildren(lc, best, m, budget, threshold)
		}
	}
}

// leafSimilarity is a dice coefficient over the leaf contents of two
// subtrees, independent of any existing matching. It is the cheap stand-
// in for "do these two nodes contain mostly the same code".
func leafSimilarity(l, r *Node) float64 {
	ll := Leaves(l)
	rl := Leaves(r)
	if len(ll)+len(rl) == 0 {
		return 0.0
	}

	counts := make(map[string]int, len(ll))
	for _, n := range ll {
		counts[n.Kind+"\x00"+n.Label+"\x00"+n.Value]++
	}
	common := 0
	for _, n := range rl {
		key := n.Kind + "\x00" + n.Label + "\x00" + n.Value
		if counts[key] > 0 {
			counts[key]--
			common++
		}
	}

	return 2.0 * float64(common) / float64(len(ll)+len(rl))
}

func countUnmatched(n *Node, m *Matching, isLeft bool) int {
	count := 0
	if isLeft && !m.LeftMatched(n.ID) {
		count++
	} else if !isLeft && !m.RightMatched(n.ID) {
		count++
	}
	for _, c := range n.Children {
		count += countUnmatched(c, m, isLeft)
	}
	return count
}

func preOrder(n *Node) []*Node {
	if n == nil {
		return nil
	}
	result := []*Node{n}
	for _, c := range n.Children {
		result = append(result, preOrder(c)...)
	}
	return result
}

func postOrder(n *Node) []*Node {
	if n == nil {
		return nil
	}
	var result []*Node
	for _, c := range n.Children {
		result = append(result, postOrder(c)...)
	}
	result = append(result, n)
	return result
}
