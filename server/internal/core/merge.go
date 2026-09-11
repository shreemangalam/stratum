package core

import "fmt"

// MergeDecision describes how a structural unit participates in a three-way
// merge plan.
type MergeDecision string

const (
	// MergeUnchanged keeps the base structural unit because neither side changed it.
	MergeUnchanged MergeDecision = "unchanged"
	// MergeTakeLeft selects the structural unit from the left side.
	MergeTakeLeft MergeDecision = "take-left"
	// MergeTakeRight selects the structural unit from the right side.
	MergeTakeRight MergeDecision = "take-right"
	// MergeTakeEither selects either side because both produced the same unit.
	MergeTakeEither MergeDecision = "take-either"
	// MergeDelete removes the structural unit from the result.
	MergeDelete MergeDecision = "delete"
	// MergeConflict requires an explicit resolution before rendering the result.
	MergeConflict MergeDecision = "conflict"
)

// MergeConflictKind is a stable category for an incompatible pair of changes.
type MergeConflictKind string

const (
	// ConflictModifyModify indicates incompatible edits to the same structural unit.
	ConflictModifyModify MergeConflictKind = "modify-modify"
	// ConflictDeleteModify indicates that one side deleted a unit the other changed.
	ConflictDeleteModify MergeConflictKind = "delete-modify"
	// ConflictRenameRename indicates that both sides chose different new identities.
	ConflictRenameRename MergeConflictKind = "rename-rename"
	// ConflictAddAdd indicates divergent additions with the same structural identity.
	ConflictAddAdd MergeConflictKind = "add-add"
)

// MergeEntry is one deterministic decision in a three-way merge plan.
type MergeEntry struct {
	BaseNode     *NodeRef           `json:"base_node,omitempty"`
	LeftNode     *NodeRef           `json:"left_node,omitempty"`
	RightNode    *NodeRef           `json:"right_node,omitempty"`
	Decision     MergeDecision      `json:"decision"`
	ConflictKind *MergeConflictKind `json:"conflict_kind,omitempty"`
	Reason       string             `json:"reason"`
}

// MergePlan describes structural decisions without synthesizing source text.
type MergePlan struct {
	Entries       []MergeEntry `json:"entries"`
	ConflictCount int          `json:"conflict_count"`
	HasConflicts  bool         `json:"has_conflicts"`
}

// PlanThreeWayMerge plans a merge of direct root children. Pairwise matching
// anchors both edited sides to the common base. The output order is base order,
// followed by left-only and then right-only additions.
func PlanThreeWayMerge(base, left, right *Tree, cfg MatchConfig) (*MergePlan, error) {
	if base == nil || left == nil || right == nil || base.Root == nil || left.Root == nil || right.Root == nil {
		return nil, fmt.Errorf("base, left, and right trees with roots are required")
	}
	if base.Language != left.Language || base.Language != right.Language {
		return nil, fmt.Errorf("merge languages must match: base=%q left=%q right=%q", base.Language, left.Language, right.Language)
	}

	baseToLeft := Match(base, left, cfg)
	baseToRight := Match(base, right, cfg)
	plan := &MergePlan{Entries: []MergeEntry{}}

	for _, baseNode := range base.Root.Children {
		leftNode := matchedNode(left, baseToLeft.LeftToRight[baseNode.ID])
		rightNode := matchedNode(right, baseToRight.LeftToRight[baseNode.ID])
		entry := planBaseNode(baseNode, leftNode, rightNode)
		plan.append(entry)
	}

	leftAdds := unmatchedRootChildren(left, baseToLeft.RightToLeft)
	rightAdds := unmatchedRootChildren(right, baseToRight.RightToLeft)
	usedRight := make(map[NodeID]bool)

	for _, leftNode := range leftAdds {
		if rightNode := findIdenticalAddition(leftNode, rightAdds, usedRight); rightNode != nil {
			usedRight[rightNode.ID] = true
			plan.append(MergeEntry{
				LeftNode:  mergeNodeRef(leftNode),
				RightNode: mergeNodeRef(rightNode),
				Decision:  MergeTakeEither,
				Reason:    "both sides added the same structural unit",
			})
			continue
		}
		if rightNode := findSameIdentityAddition(leftNode, rightAdds, usedRight); rightNode != nil {
			usedRight[rightNode.ID] = true
			kind := ConflictAddAdd
			plan.append(MergeEntry{
				LeftNode:     mergeNodeRef(leftNode),
				RightNode:    mergeNodeRef(rightNode),
				Decision:     MergeConflict,
				ConflictKind: &kind,
				Reason:       "both sides added different versions of the same structural unit",
			})
			continue
		}
		plan.append(MergeEntry{
			LeftNode: mergeNodeRef(leftNode),
			Decision: MergeTakeLeft,
			Reason:   "structural unit added only on the left",
		})
	}

	for _, rightNode := range rightAdds {
		if usedRight[rightNode.ID] {
			continue
		}
		plan.append(MergeEntry{
			RightNode: mergeNodeRef(rightNode),
			Decision:  MergeTakeRight,
			Reason:    "structural unit added only on the right",
		})
	}

	return plan, nil
}

func (p *MergePlan) append(entry MergeEntry) {
	p.Entries = append(p.Entries, entry)
	if entry.Decision == MergeConflict {
		p.ConflictCount++
		p.HasConflicts = true
	}
}

func planBaseNode(baseNode, leftNode, rightNode *Node) MergeEntry {
	entry := MergeEntry{
		BaseNode:  mergeNodeRef(baseNode),
		LeftNode:  mergeNodeRef(leftNode),
		RightNode: mergeNodeRef(rightNode),
	}

	leftUnchanged := sameSubtree(baseNode, leftNode)
	rightUnchanged := sameSubtree(baseNode, rightNode)

	switch {
	case leftNode == nil && rightNode == nil:
		entry.Decision = MergeDelete
		entry.Reason = "both sides deleted the structural unit"
	case leftNode == nil && rightUnchanged:
		entry.Decision = MergeDelete
		entry.Reason = "left deleted the structural unit and right left it unchanged"
	case rightNode == nil && leftUnchanged:
		entry.Decision = MergeDelete
		entry.Reason = "right deleted the structural unit and left left it unchanged"
	case leftNode == nil:
		setConflict(&entry, ConflictDeleteModify, "left deleted the structural unit while right modified it")
	case rightNode == nil:
		setConflict(&entry, ConflictDeleteModify, "right deleted the structural unit while left modified it")
	case leftUnchanged && rightUnchanged:
		entry.Decision = MergeUnchanged
		entry.Reason = "neither side changed the structural unit"
	case leftUnchanged:
		entry.Decision = MergeTakeRight
		entry.Reason = "structural unit changed only on the right"
	case rightUnchanged:
		entry.Decision = MergeTakeLeft
		entry.Reason = "structural unit changed only on the left"
	case sameSubtree(leftNode, rightNode):
		entry.Decision = MergeTakeEither
		entry.Reason = "both sides made the same structural change"
	case renamedDifferently(baseNode, leftNode, rightNode):
		setConflict(&entry, ConflictRenameRename, "both sides renamed the structural unit differently")
	default:
		setConflict(&entry, ConflictModifyModify, "both sides modified the structural unit differently")
	}

	return entry
}

func setConflict(entry *MergeEntry, kind MergeConflictKind, reason string) {
	entry.Decision = MergeConflict
	entry.ConflictKind = &kind
	entry.Reason = reason
}

// matchedNode resolves a matched node ID in the target tree. NodeID 0 means
// "no match" because all parsers assign IDs starting at 1 through an atomic
// counter, and Go's map zero-value for a missing key is 0.
func matchedNode(tree *Tree, id NodeID) *Node {
	if id == 0 {
		return nil
	}
	return tree.NodeMap[id]
}

func mergeNodeRef(node *Node) *NodeRef {
	if node == nil {
		return nil
	}
	return nodeRef(node)
}

func unmatchedRootChildren(tree *Tree, matched map[NodeID]NodeID) []*Node {
	var nodes []*Node
	for _, node := range tree.Root.Children {
		if _, ok := matched[node.ID]; !ok {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func findIdenticalAddition(leftNode *Node, rightAdds []*Node, used map[NodeID]bool) *Node {
	for _, rightNode := range rightAdds {
		if !used[rightNode.ID] && sameSubtree(leftNode, rightNode) {
			return rightNode
		}
	}
	return nil
}

func findSameIdentityAddition(leftNode *Node, rightAdds []*Node, used map[NodeID]bool) *Node {
	if leftNode.Label == "" {
		return nil
	}
	for _, rightNode := range rightAdds {
		if !used[rightNode.ID] && leftNode.Kind == rightNode.Kind && leftNode.Label == rightNode.Label {
			return rightNode
		}
	}
	return nil
}

func sameSubtree(a, b *Node) bool {
	return a != nil && b != nil && a.Hash != "" && a.Hash == b.Hash
}

func renamedDifferently(baseNode, leftNode, rightNode *Node) bool {
	return baseNode.Label != "" && leftNode.Label != baseNode.Label && rightNode.Label != baseNode.Label && leftNode.Label != rightNode.Label
}
