package core

import (
	"strings"
)

// GenerateMergedSource synthesizes the merged file content from a merge plan
// and the three input trees. Auto-resolved entries emit the chosen side's
// source text; conflicts emit git-style conflict markers.
func GenerateMergedSource(plan *MergePlan, base, left, right *Tree) string {
	var b strings.Builder
	first := true

	for _, entry := range plan.Entries {
		switch entry.Decision {
		case MergeDelete:
			continue

		case MergeUnchanged, MergeTakeEither:
			text := resolveNodeText(entry.BaseNode, base)
			if text == "" {
				text = resolveNodeText(entry.LeftNode, left)
			}
			writeEntry(&b, text, &first)

		case MergeTakeLeft:
			text := resolveNodeText(entry.LeftNode, left)
			writeEntry(&b, text, &first)

		case MergeTakeRight:
			text := resolveNodeText(entry.RightNode, right)
			writeEntry(&b, text, &first)

		case MergeConflict:
			leftText := resolveNodeText(entry.LeftNode, left)
			rightText := resolveNodeText(entry.RightNode, right)
			writeConflict(&b, leftText, rightText, &first)
		}
	}

	return b.String()
}

func resolveNodeText(ref *NodeRef, tree *Tree) string {
	if ref == nil || tree == nil {
		return ""
	}
	node := tree.NodeMap[ref.ID]
	if node == nil {
		return ""
	}
	return extractNodeSource(tree.Source, node)
}

func extractNodeSource(source []byte, n *Node) string {
	if source == nil || n.Span.End.Offset <= n.Span.Start.Offset {
		return ""
	}
	end := n.Span.End.Offset
	if end > len(source) {
		end = len(source)
	}
	start := n.Span.Start.Offset
	if start > len(source) {
		return ""
	}
	return string(source[start:end])
}

func writeEntry(b *strings.Builder, text string, first *bool) {
	if text == "" {
		return
	}
	if !*first {
		b.WriteString("\n\n")
	}
	*first = false
	b.WriteString(text)
}

func writeConflict(b *strings.Builder, leftText, rightText string, first *bool) {
	if !*first {
		b.WriteString("\n\n")
	}
	*first = false

	b.WriteString("<<<<<<< left\n")
	b.WriteString(leftText)
	if !strings.HasSuffix(leftText, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("=======\n")
	b.WriteString(rightText)
	if !strings.HasSuffix(rightText, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(">>>>>>> right")
}
