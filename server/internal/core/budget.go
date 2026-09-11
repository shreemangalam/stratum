package core

// BudgetExceeded returns true if the cost of optimal alignment between
// two subtrees would exceed the configured node budget.
func BudgetExceeded(left, right *Node, budget int) bool {
	lSize := subtreeSize(left)
	rSize := subtreeSize(right)
	return lSize*rSize > budget*budget
}

func subtreeSize(n *Node) int {
	if n == nil {
		return 0
	}
	size := 1
	for _, c := range n.Children {
		size += subtreeSize(c)
	}
	return size
}
