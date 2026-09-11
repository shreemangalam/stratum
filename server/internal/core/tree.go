package core

// NodeID uniquely identifies a node within a single tree.
type NodeID int

// Location marks a position in source text.
type Location struct {
	Line   int `json:"line"`
	Column int `json:"column"`
	Offset int `json:"offset"`
}

// Span marks a range in source text.
type Span struct {
	Start Location `json:"start"`
	End   Location `json:"end"`
}

// Node is a single node in a syntax tree.
type Node struct {
	ID       NodeID
	Kind     string
	Label    string
	Value    string
	Span     Span
	Children []*Node
	Parent   *Node
	Hash     string
}

// Tree is a parsed syntax tree with metadata.
type Tree struct {
	Root     *Node
	Language string
	Source   []byte
	NodeMap  map[NodeID]*Node
}

// NewTree creates a tree from a root node, building the node map.
func NewTree(root *Node, language string, source []byte) *Tree {
	t := &Tree{
		Root:     root,
		Language: language,
		Source:   source,
		NodeMap:  make(map[NodeID]*Node),
	}
	t.indexNodes(root)
	return t
}

func (t *Tree) indexNodes(n *Node) {
	if n == nil {
		return
	}
	t.NodeMap[n.ID] = n
	for _, c := range n.Children {
		t.indexNodes(c)
	}
}

// Size returns the total number of nodes in the tree.
func (t *Tree) Size() int {
	return len(t.NodeMap)
}

// Descendants returns all nodes under n, not including n itself.
func Descendants(n *Node) []*Node {
	var result []*Node
	for _, c := range n.Children {
		result = append(result, c)
		result = append(result, Descendants(c)...)
	}
	return result
}

// Leaves returns all leaf nodes under n (including n if it has no children).
func Leaves(n *Node) []*Node {
	if len(n.Children) == 0 {
		return []*Node{n}
	}
	var result []*Node
	for _, c := range n.Children {
		result = append(result, Leaves(c)...)
	}
	return result
}
