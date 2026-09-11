package core

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// HashTree computes Merkle hashes for every node in the tree, bottom-up.
// A node's hash is derived from its kind, label, value, and its children's
// hashes. Two subtrees with identical structure and content produce
// identical hashes.
func HashTree(t *Tree) {
	hashNode(t.Root)
}

func hashNode(n *Node) string {
	if n == nil {
		return ""
	}

	var childHashes []string
	for _, c := range n.Children {
		childHashes = append(childHashes, hashNode(c))
	}

	h := sha256.New()
	_, _ = fmt.Fprintf(h, "kind:%s\n", n.Kind)
	_, _ = fmt.Fprintf(h, "label:%s\n", n.Label)
	_, _ = fmt.Fprintf(h, "value:%s\n", n.Value)
	_, _ = fmt.Fprintf(h, "children:%s\n", strings.Join(childHashes, ","))

	n.Hash = fmt.Sprintf("%x", h.Sum(nil))
	return n.Hash
}
