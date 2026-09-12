package core

import (
	"strings"
	"testing"
)

func renderTree(id NodeID, source string, children ...*Node) *Tree {
	root := buildTree(id, "source_file", "", children...)
	t := NewTree(root, "go", []byte(source))
	HashTree(t)
	return t
}

func renderFunc(id NodeID, name, bodyVal string, startOff, endOff int) *Node {
	n := buildTree(id, "function_declaration", name,
		buildTree(id+1, "block", bodyVal),
	)
	n.Span = Span{
		Start: Location{Line: 1, Column: 0, Offset: startOff},
		End:   Location{Line: 5, Column: 0, Offset: endOff},
	}
	return n
}

func TestGenerateMergedSource_AllAutoResolved(t *testing.T) {
	t.Parallel()
	baseSrc := "func add(a, b int) int {\n\treturn a + b\n}\n\nfunc sub(a, b int) int {\n\treturn a - b\n}"
	leftSrc := "func add(a, b int) int {\n\treturn a + b\n}\n\nfunc sub(a, b int) int {\n\treturn a - b\n}\n\nfunc mul(a, b int) int {\n\treturn a * b\n}"
	rightSrc := "func add(a, b int) int {\n\tsum := a + b\n\treturn sum\n}\n\nfunc sub(a, b int) int {\n\treturn a - b\n}"

	addEnd := strings.Index(baseSrc, "\n\nfunc sub")
	subStart := addEnd + 2
	subEnd := len(baseSrc)

	baseTree := renderTree(1, baseSrc,
		renderFunc(10, "add", "a+b", 0, addEnd),
		renderFunc(20, "sub", "a-b", subStart, subEnd),
	)

	leftAddEnd := strings.Index(leftSrc, "\n\nfunc sub")
	leftSubStart := leftAddEnd + 2
	leftSubEnd := strings.Index(leftSrc, "\n\nfunc mul")
	leftMulStart := leftSubEnd + 2
	leftTree := renderTree(100, leftSrc,
		renderFunc(110, "add", "a+b", 0, leftAddEnd),
		renderFunc(120, "sub", "a-b", leftSubStart, leftSubEnd),
		renderFunc(130, "mul", "a*b", leftMulStart, len(leftSrc)),
	)

	rightAddEnd := strings.Index(rightSrc, "\n\nfunc sub")
	rightSubStart := rightAddEnd + 2
	rightTree := renderTree(200, rightSrc,
		renderFunc(210, "add", "sum", 0, rightAddEnd),
		renderFunc(220, "sub", "a-b", rightSubStart, len(rightSrc)),
	)

	plan, err := PlanThreeWayMerge(baseTree, leftTree, rightTree, DefaultMatchConfig())
	if err != nil {
		t.Fatal(err)
	}

	merged := GenerateMergedSource(plan, baseTree, leftTree, rightTree)

	if !strings.Contains(merged, "sum := a + b") {
		t.Error("expected right's add implementation in merged output")
	}
	if !strings.Contains(merged, "return a - b") {
		t.Error("expected unchanged sub in merged output")
	}
	if !strings.Contains(merged, "func mul") {
		t.Error("expected left's mul addition in merged output")
	}
	if strings.Contains(merged, "<<<<<<<") {
		t.Error("expected no conflict markers")
	}
}

func TestGenerateMergedSource_WithConflict(t *testing.T) {
	t.Parallel()
	baseSrc := "func compute(x int) int {\n\treturn x + 1\n}"
	leftSrc := "func compute(x int) int {\n\treturn x * 2\n}"
	rightSrc := "func compute(x int) int {\n\treturn x * 3\n}"

	baseTree := renderTree(1, baseSrc, renderFunc(10, "compute", "x+1", 0, len(baseSrc)))
	leftTree := renderTree(100, leftSrc, renderFunc(110, "compute", "x*2", 0, len(leftSrc)))
	rightTree := renderTree(200, rightSrc, renderFunc(210, "compute", "x*3", 0, len(rightSrc)))

	plan, err := PlanThreeWayMerge(baseTree, leftTree, rightTree, DefaultMatchConfig())
	if err != nil {
		t.Fatal(err)
	}

	merged := GenerateMergedSource(plan, baseTree, leftTree, rightTree)

	if !strings.Contains(merged, "<<<<<<< left") {
		t.Error("expected left conflict marker")
	}
	if !strings.Contains(merged, "=======") {
		t.Error("expected separator marker")
	}
	if !strings.Contains(merged, ">>>>>>> right") {
		t.Error("expected right conflict marker")
	}
	if !strings.Contains(merged, "x * 2") {
		t.Error("expected left's code in conflict")
	}
	if !strings.Contains(merged, "x * 3") {
		t.Error("expected right's code in conflict")
	}
}

func TestGenerateMergedSource_DeletedNode(t *testing.T) {
	t.Parallel()
	baseSrc := "func keep() {}\n\nfunc remove() {}"
	leftSrc := "func keep() {}"
	rightSrc := "func keep() {}"

	keepEnd := strings.Index(baseSrc, "\n\nfunc remove")
	removeStart := keepEnd + 2

	baseTree := renderTree(1, baseSrc,
		renderFunc(10, "keep", "k", 0, keepEnd),
		renderFunc(20, "remove", "r", removeStart, len(baseSrc)),
	)
	leftTree := renderTree(100, leftSrc,
		renderFunc(110, "keep", "k", 0, len(leftSrc)),
	)
	rightTree := renderTree(200, rightSrc,
		renderFunc(210, "keep", "k", 0, len(rightSrc)),
	)

	plan, err := PlanThreeWayMerge(baseTree, leftTree, rightTree, DefaultMatchConfig())
	if err != nil {
		t.Fatal(err)
	}

	merged := GenerateMergedSource(plan, baseTree, leftTree, rightTree)

	if !strings.Contains(merged, "func keep") {
		t.Error("expected keep function in output")
	}
	if strings.Contains(merged, "func remove") {
		t.Error("expected remove function to be absent")
	}
}

func TestGenerateMergedSource_EmptyPlan(t *testing.T) {
	t.Parallel()
	plan := &MergePlan{Entries: []MergeEntry{}}
	baseTree := renderTree(1, "")
	merged := GenerateMergedSource(plan, baseTree, baseTree, baseTree)
	if merged != "" {
		t.Errorf("expected empty string for empty plan, got %q", merged)
	}
}

func TestGenerateMergedSource_DeleteModifyConflict(t *testing.T) {
	t.Parallel()
	baseSrc := "func f() { return 1 }"
	leftSrc := ""
	rightSrc := "func f() { return 2 }"

	baseTree := renderTree(1, baseSrc, renderFunc(10, "f", "v1", 0, len(baseSrc)))
	leftTree := renderTree(100, leftSrc)
	rightTree := renderTree(200, rightSrc, renderFunc(210, "f", "v2", 0, len(rightSrc)))

	plan, err := PlanThreeWayMerge(baseTree, leftTree, rightTree, DefaultMatchConfig())
	if err != nil {
		t.Fatal(err)
	}

	merged := GenerateMergedSource(plan, baseTree, leftTree, rightTree)

	if !strings.Contains(merged, "<<<<<<< left") {
		t.Error("expected conflict markers for delete-modify")
	}
	if !strings.Contains(merged, "return 2") {
		t.Error("expected right's code in conflict")
	}
}
