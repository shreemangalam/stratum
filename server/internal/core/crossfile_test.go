package core

import "testing"

func cfTree(id NodeID, source string, children ...*Node) *Tree {
	root := buildTree(id, "source_file", "", children...)
	t := NewTree(root, "go", []byte(source))
	HashTree(t)
	return t
}

func cfFunc(id NodeID, name string, startOff, endOff int) *Node {
	n := buildTree(id, "function_declaration", name,
		buildTree(id+1, "block", "body"),
	)
	n.Span = Span{
		Start: Location{Line: 1, Column: 0, Offset: startOff},
		End:   Location{Line: 5, Column: 0, Offset: endOff},
	}
	return n
}

func cfDiff(path, leftSrc, rightSrc string, leftTree, rightTree *Tree, ops []Operation) FileDiff {
	return FileDiff{
		Path:        path,
		LeftSource:  []byte(leftSrc),
		RightSource: []byte(rightSrc),
		LeftTree:    leftTree,
		RightTree:   rightTree,
		Script:      &EditScript{Operations: ops},
	}
}

func TestCrossFile_ExactMove(t *testing.T) {
	t.Parallel()
	funcBody := "func hello() {\n\tfmt.Println(\"hello\")\n}"

	leftTree := cfTree(1, funcBody, cfFunc(10, "hello", 0, len(funcBody)))
	rightTree := cfTree(100, funcBody, cfFunc(110, "hello", 0, len(funcBody)))

	leftRef := nodeRef(leftTree.NodeMap[10])
	rightRef := nodeRef(rightTree.NodeMap[110])

	diffs := []FileDiff{
		cfDiff("a.go", funcBody, "", leftTree, nil, []Operation{
			{Kind: OpDelete, LeftNode: leftRef},
		}),
		cfDiff("b.go", "", funcBody, nil, rightTree, []Operation{
			{Kind: OpInsert, RightNode: rightRef},
		}),
	}

	result := DetectCrossFileChanges(diffs)
	if len(result.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(result.Matches))
	}
	m := result.Matches[0]
	if m.Kind != CrossFileMove {
		t.Errorf("expected move, got %s", m.Kind)
	}
	if m.Score != 1.0 {
		t.Errorf("expected score 1.0, got %f", m.Score)
	}
	if m.SourceFile != "a.go" || m.TargetFile != "b.go" {
		t.Errorf("expected a.go -> b.go, got %s -> %s", m.SourceFile, m.TargetFile)
	}
}

func TestCrossFile_MoveWithEdits(t *testing.T) {
	t.Parallel()
	srcBody := "func greet(name string) {\n\tfmt.Println(\"hi \" + name)\n\treturn\n}"
	tgtBody := "func greet(name string) {\n\tfmt.Println(\"hello \" + name)\n\treturn\n}"

	leftTree := cfTree(1, srcBody, cfFunc(10, "greet", 0, len(srcBody)))
	rightTree := cfTree(100, tgtBody, cfFunc(110, "greet", 0, len(tgtBody)))

	diffs := []FileDiff{
		cfDiff("old.go", srcBody, "", leftTree, nil, []Operation{
			{Kind: OpDelete, LeftNode: nodeRef(leftTree.NodeMap[10])},
		}),
		cfDiff("new.go", "", tgtBody, nil, rightTree, []Operation{
			{Kind: OpInsert, RightNode: nodeRef(rightTree.NodeMap[110])},
		}),
	}

	result := DetectCrossFileChanges(diffs)
	if len(result.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(result.Matches))
	}
	m := result.Matches[0]
	if m.Kind != CrossFileMove {
		t.Errorf("expected move, got %s", m.Kind)
	}
	if m.Score < 0.6 || m.Score >= 1.0 {
		t.Errorf("expected similarity in [0.6, 1.0), got %f", m.Score)
	}
}

func TestCrossFile_RenameMove(t *testing.T) {
	t.Parallel()
	srcBody := "func oldName(a, b int) int {\n\tx := a * 2\n\ty := b * 3\n\tresult := x + y\n\tif result < 0 {\n\t\treturn 0\n\t}\n\treturn result\n}"
	tgtBody := "func newName(a, b int) int {\n\tx := a * 2\n\ty := b * 3\n\tresult := x + y\n\tif result < 0 {\n\t\treturn 0\n\t}\n\treturn result\n}"

	leftTree := cfTree(1, srcBody, cfFunc(10, "oldName", 0, len(srcBody)))
	rightTree := cfTree(100, tgtBody, cfFunc(110, "newName", 0, len(tgtBody)))

	diffs := []FileDiff{
		cfDiff("utils.go", srcBody, "", leftTree, nil, []Operation{
			{Kind: OpDelete, LeftNode: nodeRef(leftTree.NodeMap[10])},
		}),
		cfDiff("helpers.go", "", tgtBody, nil, rightTree, []Operation{
			{Kind: OpInsert, RightNode: nodeRef(rightTree.NodeMap[110])},
		}),
	}

	result := DetectCrossFileChanges(diffs)
	if len(result.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(result.Matches))
	}
	m := result.Matches[0]
	if m.Kind != CrossFileRenameMove {
		t.Errorf("expected rename-move, got %s", m.Kind)
	}
	if m.SourceNode.Label != "oldName" || m.TargetNode.Label != "newName" {
		t.Errorf("expected oldName -> newName, got %s -> %s", m.SourceNode.Label, m.TargetNode.Label)
	}
}

func TestCrossFile_MultipleMovesInChangeset(t *testing.T) {
	t.Parallel()
	funcA := "func alpha() {\n\treturn 1\n}"
	funcB := "func beta() {\n\treturn 2\n}"
	both := funcA + "\n" + funcB

	leftTree := cfTree(1, both,
		cfFunc(10, "alpha", 0, len(funcA)),
		cfFunc(20, "beta", len(funcA)+1, len(both)),
	)
	rightTreeA := cfTree(100, funcA, cfFunc(110, "alpha", 0, len(funcA)))
	rightTreeB := cfTree(200, funcB, cfFunc(210, "beta", 0, len(funcB)))

	diffs := []FileDiff{
		cfDiff("combined.go", both, "", leftTree, nil, []Operation{
			{Kind: OpDelete, LeftNode: nodeRef(leftTree.NodeMap[10])},
			{Kind: OpDelete, LeftNode: nodeRef(leftTree.NodeMap[20])},
		}),
		cfDiff("alpha.go", "", funcA, nil, rightTreeA, []Operation{
			{Kind: OpInsert, RightNode: nodeRef(rightTreeA.NodeMap[110])},
		}),
		cfDiff("beta.go", "", funcB, nil, rightTreeB, []Operation{
			{Kind: OpInsert, RightNode: nodeRef(rightTreeB.NodeMap[210])},
		}),
	}

	result := DetectCrossFileChanges(diffs)
	if len(result.Matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(result.Matches))
	}
}

func TestCrossFile_NoFalsePositive_UnrelatedChanges(t *testing.T) {
	t.Parallel()
	srcBody := "func doWork() {\n\tprocessItems()\n\tsendResults()\n}"
	tgtBody := "func handleRequest() {\n\tvalidateInput()\n\tresponseWriter()\n}"

	leftTree := cfTree(1, srcBody, cfFunc(10, "doWork", 0, len(srcBody)))
	rightTree := cfTree(100, tgtBody, cfFunc(110, "handleRequest", 0, len(tgtBody)))

	diffs := []FileDiff{
		cfDiff("worker.go", srcBody, "", leftTree, nil, []Operation{
			{Kind: OpDelete, LeftNode: nodeRef(leftTree.NodeMap[10])},
		}),
		cfDiff("handler.go", "", tgtBody, nil, rightTree, []Operation{
			{Kind: OpInsert, RightNode: nodeRef(rightTree.NodeMap[110])},
		}),
	}

	result := DetectCrossFileChanges(diffs)
	if len(result.Matches) != 0 {
		t.Errorf("expected 0 matches for unrelated changes, got %d", len(result.Matches))
	}
}

func TestCrossFile_SingleFile_NoMatches(t *testing.T) {
	t.Parallel()
	body := "func single() {\n\treturn 42\n}"
	tree := cfTree(1, body, cfFunc(10, "single", 0, len(body)))

	diffs := []FileDiff{
		cfDiff("only.go", body, "", tree, nil, []Operation{
			{Kind: OpDelete, LeftNode: nodeRef(tree.NodeMap[10])},
		}),
	}

	result := DetectCrossFileChanges(diffs)
	if len(result.Matches) != 0 {
		t.Errorf("expected 0 matches for single file, got %d", len(result.Matches))
	}
}

func TestCrossFile_AddedFile_NoSources(t *testing.T) {
	t.Parallel()
	body := "func newFunc() {\n\treturn \"new\"\n}"
	tree := cfTree(100, body, cfFunc(110, "newFunc", 0, len(body)))

	diffs := []FileDiff{
		cfDiff("existing.go", "unchanged", "unchanged", nil, nil, []Operation{}),
		cfDiff("brand_new.go", "", body, nil, tree, []Operation{
			{Kind: OpInsert, RightNode: nodeRef(tree.NodeMap[110])},
		}),
	}

	result := DetectCrossFileChanges(diffs)
	if len(result.Matches) != 0 {
		t.Errorf("expected 0 matches when no deletes exist, got %d", len(result.Matches))
	}
}

func TestCrossFile_DeletedFile_NoTargets(t *testing.T) {
	t.Parallel()
	body := "func removed() {\n\treturn \"gone\"\n}"
	tree := cfTree(1, body, cfFunc(10, "removed", 0, len(body)))

	diffs := []FileDiff{
		cfDiff("old.go", body, "", tree, nil, []Operation{
			{Kind: OpDelete, LeftNode: nodeRef(tree.NodeMap[10])},
		}),
		cfDiff("other.go", "unchanged", "unchanged", nil, nil, []Operation{}),
	}

	result := DetectCrossFileChanges(diffs)
	if len(result.Matches) != 0 {
		t.Errorf("expected 0 matches when no inserts exist, got %d", len(result.Matches))
	}
}

func TestCrossFile_SameNameBothInserted_NoMatch(t *testing.T) {
	t.Parallel()
	bodyA := "func init() {\n\tsetupA()\n}"
	bodyB := "func init() {\n\tsetupB()\n}"

	treeA := cfTree(100, bodyA, cfFunc(110, "init", 0, len(bodyA)))
	treeB := cfTree(200, bodyB, cfFunc(210, "init", 0, len(bodyB)))

	diffs := []FileDiff{
		cfDiff("a.go", "", bodyA, nil, treeA, []Operation{
			{Kind: OpInsert, RightNode: nodeRef(treeA.NodeMap[110])},
		}),
		cfDiff("b.go", "", bodyB, nil, treeB, []Operation{
			{Kind: OpInsert, RightNode: nodeRef(treeB.NodeMap[210])},
		}),
	}

	result := DetectCrossFileChanges(diffs)
	if len(result.Matches) != 0 {
		t.Errorf("expected 0 matches for two inserts with no deletes, got %d", len(result.Matches))
	}
}

func TestCrossFile_EmptyDiffs(t *testing.T) {
	t.Parallel()
	result := DetectCrossFileChanges(nil)
	if result == nil || len(result.Matches) != 0 {
		t.Error("expected empty result for nil diffs")
	}
	result = DetectCrossFileChanges([]FileDiff{})
	if result == nil || len(result.Matches) != 0 {
		t.Error("expected empty result for empty diffs")
	}
}

func TestJaccardLines(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		a, b string
		min  float64
		max  float64
	}{
		{"identical", "line1\nline2\nline3", "line1\nline2\nline3", 1.0, 1.0},
		{"empty both", "", "", 1.0, 1.0},
		{"disjoint", "aaa\nbbb", "ccc\nddd", 0.0, 0.0},
		{"partial overlap", "line1\nline2\nline3", "line1\nline2\nline4", 0.4, 0.7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			score := jaccardLines(tc.a, tc.b)
			if score < tc.min || score > tc.max {
				t.Errorf("expected score in [%f, %f], got %f", tc.min, tc.max, score)
			}
		})
	}
}
