package core

import "testing"

// assignTree builds: assignment_statement(:=) -> [assignment_lhs -> [idents...], assignment_rhs -> [exprs...]]
func assignTree(base NodeID, lhsNames []string, rhsNodes ...*Node) *Node {
	node := &Node{ID: base, Kind: "assignment_statement", Label: ":="}
	nextID := base + 1

	lhs := &Node{ID: nextID, Kind: "assignment_lhs", Parent: node}
	nextID++
	for _, name := range lhsNames {
		ident := &Node{ID: nextID, Kind: "identifier", Label: name, Value: name, Parent: lhs}
		nextID++
		lhs.Children = append(lhs.Children, ident)
	}
	node.Children = append(node.Children, lhs)

	rhs := &Node{ID: nextID, Kind: "assignment_rhs", Parent: node}
	for _, r := range rhsNodes {
		r.Parent = rhs
		rhs.Children = append(rhs.Children, r)
	}
	node.Children = append(node.Children, rhs)

	return node
}

func identNode(id NodeID, name string) *Node {
	return &Node{ID: id, Kind: "identifier", Label: name, Value: name}
}

func litNode(id NodeID, val string) *Node {
	return &Node{ID: id, Kind: "literal", Value: val}
}

func callNode(id NodeID, fnName string, args ...*Node) *Node {
	n := &Node{ID: id, Kind: "call_expression"}
	fn := &Node{ID: id + 1, Kind: "identifier", Label: fnName, Value: fnName, Parent: n}
	n.Children = append(n.Children, fn)
	for _, a := range args {
		a.Parent = n
		n.Children = append(n.Children, a)
	}
	return n
}

func TestExtractDefUse_SimpleAssign(t *testing.T) {
	// x := 1
	stmt := assignTree(1, []string{"x"}, litNode(10, "1"))
	du := ExtractDefUse(stmt)

	if !du.Defs["x"] {
		t.Error("expected x in defs")
	}
	if len(du.Defs) != 1 {
		t.Errorf("expected 1 def, got %d", len(du.Defs))
	}
	if len(du.Uses) != 0 {
		t.Errorf("expected 0 uses, got %d", len(du.Uses))
	}
	if du.HasSideEffects {
		t.Error("pure assignment should not have side effects")
	}
}

func TestExtractDefUse_AssignWithUse(t *testing.T) {
	// x := y + 1
	rhs := &Node{ID: 10, Kind: "binary_expression", Label: "+"}
	yIdent := &Node{ID: 11, Kind: "identifier", Label: "y", Value: "y", Parent: rhs}
	lit := &Node{ID: 12, Kind: "literal", Value: "1", Parent: rhs}
	rhs.Children = []*Node{yIdent, lit}

	stmt := assignTree(1, []string{"x"}, rhs)
	du := ExtractDefUse(stmt)

	if !du.Defs["x"] {
		t.Error("expected x in defs")
	}
	if !du.Uses["y"] {
		t.Error("expected y in uses")
	}
	if du.HasSideEffects {
		t.Error("pure assignment should not have side effects")
	}
}

func TestExtractDefUse_MultiAssign(t *testing.T) {
	// x, y := 1, 2
	stmt := assignTree(1, []string{"x", "y"}, litNode(10, "1"), litNode(11, "2"))
	du := ExtractDefUse(stmt)

	if !du.Defs["x"] || !du.Defs["y"] {
		t.Error("expected x and y in defs")
	}
	if len(du.Uses) != 0 {
		t.Errorf("expected 0 uses, got %d", len(du.Uses))
	}
}

func TestExtractDefUse_CallHasSideEffects(t *testing.T) {
	// x := f(y)
	stmt := assignTree(1, []string{"x"}, callNode(10, "f", identNode(20, "y")))
	du := ExtractDefUse(stmt)

	if !du.Defs["x"] {
		t.Error("expected x in defs")
	}
	if !du.Uses["f"] || !du.Uses["y"] {
		t.Error("expected f and y in uses")
	}
	if !du.HasSideEffects {
		t.Error("call expression should have side effects")
	}
}

func TestExtractDefUse_BareCall(t *testing.T) {
	// fmt.Println(x) — bare expression statement
	stmt := callNode(1, "fmt", identNode(10, "x"))
	du := ExtractDefUse(stmt)

	if len(du.Defs) != 0 {
		t.Errorf("bare call should have no defs, got %d", len(du.Defs))
	}
	if !du.Uses["fmt"] || !du.Uses["x"] {
		t.Error("expected fmt and x in uses")
	}
	if !du.HasSideEffects {
		t.Error("call should have side effects")
	}
}

func TestExtractDefUse_ReturnStatement(t *testing.T) {
	// return x + y
	ret := &Node{ID: 1, Kind: "return_statement"}
	bin := &Node{ID: 2, Kind: "binary_expression", Label: "+", Parent: ret}
	x := &Node{ID: 3, Kind: "identifier", Label: "x", Value: "x", Parent: bin}
	y := &Node{ID: 4, Kind: "identifier", Label: "y", Value: "y", Parent: bin}
	bin.Children = []*Node{x, y}
	ret.Children = []*Node{bin}

	du := ExtractDefUse(ret)
	if !du.Uses["x"] || !du.Uses["y"] {
		t.Error("expected x and y in uses")
	}
	if !du.HasSideEffects {
		t.Error("return should have side effects")
	}
}

func TestExtractDefUse_IfStatement(t *testing.T) {
	ifStmt := &Node{ID: 1, Kind: "if_statement"}
	du := ExtractDefUse(ifStmt)
	if !du.HasSideEffects {
		t.Error("if statement should have side effects")
	}
}

func TestDefUseSetsIndependent_Disjoint(t *testing.T) {
	a := DefUseSet{Defs: map[string]bool{"x": true}, Uses: map[string]bool{}}
	b := DefUseSet{Defs: map[string]bool{"y": true}, Uses: map[string]bool{}}
	if !DefUseSetsIndependent(a, b) {
		t.Error("disjoint def sets should be independent")
	}
}

func TestDefUseSetsIndependent_SharedReads(t *testing.T) {
	a := DefUseSet{Defs: map[string]bool{"x": true}, Uses: map[string]bool{"z": true}}
	b := DefUseSet{Defs: map[string]bool{"y": true}, Uses: map[string]bool{"z": true}}
	if !DefUseSetsIndependent(a, b) {
		t.Error("shared reads should be independent")
	}
}

func TestDefUseSetsIndependent_WriteReadConflict(t *testing.T) {
	a := DefUseSet{Defs: map[string]bool{"x": true}, Uses: map[string]bool{}}
	b := DefUseSet{Defs: map[string]bool{}, Uses: map[string]bool{"x": true}}
	if DefUseSetsIndependent(a, b) {
		t.Error("write-read conflict should be dependent")
	}
}

func TestDefUseSetsIndependent_ReadWriteConflict(t *testing.T) {
	a := DefUseSet{Defs: map[string]bool{}, Uses: map[string]bool{"x": true}}
	b := DefUseSet{Defs: map[string]bool{"x": true}, Uses: map[string]bool{}}
	if DefUseSetsIndependent(a, b) {
		t.Error("read-write conflict should be dependent")
	}
}

func TestDefUseSetsIndependent_WriteWriteConflict(t *testing.T) {
	a := DefUseSet{Defs: map[string]bool{"x": true}, Uses: map[string]bool{}}
	b := DefUseSet{Defs: map[string]bool{"x": true}, Uses: map[string]bool{}}
	if DefUseSetsIndependent(a, b) {
		t.Error("write-write conflict should be dependent")
	}
}

// assignStmtForBlock builds a complete assignment statement with parent linkage.
func assignStmtForBlock(base NodeID, lhsNames []string, rhsNodes ...*Node) *Node {
	stmt := assignTree(base, lhsNames, rhsNodes...)
	linkChildren(stmt)
	return stmt
}

func TestClassifyReordering_IndependentSwap(t *testing.T) {
	// Left: x := 1; y := 2
	// Right: y := 2; x := 1
	s1L := assignStmtForBlock(10, []string{"x"}, litNode(20, "1"))
	s2L := assignStmtForBlock(30, []string{"y"}, litNode(40, "2"))
	leftFn := linkChildren(buildTree(1, "function_declaration", "f",
		buildTree(2, "parameter_list", ""),
		&Node{ID: 3, Kind: "block", Children: []*Node{s1L, s2L}},
	))

	s1R := assignStmtForBlock(110, []string{"y"}, litNode(120, "2"))
	s2R := assignStmtForBlock(130, []string{"x"}, litNode(140, "1"))
	rightFn := linkChildren(buildTree(101, "function_declaration", "f",
		buildTree(102, "parameter_list", ""),
		&Node{ID: 103, Kind: "block", Children: []*Node{s1R, s2R}},
	))

	left := NewTree(linkChildren(buildTree(0, "source_file", "", leftFn)), "go", nil)
	right := NewTree(linkChildren(buildTree(100, "source_file", "", rightFn)), "go", nil)
	m := Match(left, right, DefaultMatchConfig())
	es := GenerateEditScript(left, right, m)

	found := false
	for _, sc := range es.Semantic {
		if sc.LeftNode.Label == "f" {
			found = true
			if sc.Verdict != VerdictPreserving {
				t.Errorf("expected preserving, got %s (%s)", sc.Verdict, sc.Reason)
			}
			if sc.Reason != "independent statements reordered" {
				t.Errorf("expected 'independent statements reordered', got %q", sc.Reason)
			}
		}
	}
	if !found {
		t.Error("no semantic entry found for function f")
	}
}

func TestClassifyReordering_DependentSwap(t *testing.T) {
	// Left: x := 1; y := x + 1
	// Right: y := x + 1; x := 1
	s1L := assignStmtForBlock(10, []string{"x"}, litNode(20, "1"))
	rhsBin := &Node{ID: 41, Kind: "binary_expression", Label: "+"}
	rhsBin.Children = []*Node{identNode(42, "x"), litNode(43, "1")}
	s2L := assignStmtForBlock(30, []string{"y"}, rhsBin)

	leftFn := linkChildren(buildTree(1, "function_declaration", "f",
		buildTree(2, "parameter_list", ""),
		&Node{ID: 3, Kind: "block", Children: []*Node{s1L, s2L}},
	))

	rhsBin2 := &Node{ID: 141, Kind: "binary_expression", Label: "+"}
	rhsBin2.Children = []*Node{identNode(142, "x"), litNode(143, "1")}
	s1R := assignStmtForBlock(130, []string{"y"}, rhsBin2)
	s2R := assignStmtForBlock(110, []string{"x"}, litNode(120, "1"))

	rightFn := linkChildren(buildTree(101, "function_declaration", "f",
		buildTree(102, "parameter_list", ""),
		&Node{ID: 103, Kind: "block", Children: []*Node{s1R, s2R}},
	))

	left := NewTree(linkChildren(buildTree(0, "source_file", "", leftFn)), "go", nil)
	right := NewTree(linkChildren(buildTree(100, "source_file", "", rightFn)), "go", nil)
	m := Match(left, right, DefaultMatchConfig())
	es := GenerateEditScript(left, right, m)

	found := false
	for _, sc := range es.Semantic {
		if sc.LeftNode.Label == "f" {
			found = true
			if sc.Verdict != VerdictChanging {
				t.Errorf("expected changing, got %s (%s)", sc.Verdict, sc.Reason)
			}
		}
	}
	if !found {
		t.Error("no semantic entry found for function f")
	}
}

func TestClassifyReordering_WithSideEffects(t *testing.T) {
	// Left: x := f(); y := 1
	// Right: y := 1; x := f()
	s1L := assignStmtForBlock(10, []string{"x"}, callNode(20, "f"))
	s2L := assignStmtForBlock(30, []string{"y"}, litNode(40, "1"))

	leftFn := linkChildren(buildTree(1, "function_declaration", "g",
		buildTree(2, "parameter_list", ""),
		&Node{ID: 3, Kind: "block", Children: []*Node{s1L, s2L}},
	))

	s1R := assignStmtForBlock(130, []string{"y"}, litNode(140, "1"))
	s2R := assignStmtForBlock(110, []string{"x"}, callNode(120, "f"))

	rightFn := linkChildren(buildTree(101, "function_declaration", "g",
		buildTree(102, "parameter_list", ""),
		&Node{ID: 103, Kind: "block", Children: []*Node{s1R, s2R}},
	))

	left := NewTree(linkChildren(buildTree(0, "source_file", "", leftFn)), "go", nil)
	right := NewTree(linkChildren(buildTree(100, "source_file", "", rightFn)), "go", nil)
	m := Match(left, right, DefaultMatchConfig())
	es := GenerateEditScript(left, right, m)

	found := false
	for _, sc := range es.Semantic {
		if sc.LeftNode.Label == "g" {
			found = true
			if sc.Verdict != VerdictIndeterminate {
				t.Errorf("expected indeterminate, got %s (%s)", sc.Verdict, sc.Reason)
			}
		}
	}
	if !found {
		t.Error("no semantic entry found for function g")
	}
}

func TestClassifyReordering_NonGo(t *testing.T) {
	// Same structure but language is "python" — should stay indeterminate.
	s1L := assignStmtForBlock(10, []string{"x"}, litNode(20, "1"))
	s2L := assignStmtForBlock(30, []string{"y"}, litNode(40, "2"))
	leftFn := linkChildren(buildTree(1, "function_declaration", "f",
		buildTree(2, "parameter_list", ""),
		&Node{ID: 3, Kind: "block", Children: []*Node{s1L, s2L}},
	))

	s1R := assignStmtForBlock(130, []string{"y"}, litNode(140, "2"))
	s2R := assignStmtForBlock(110, []string{"x"}, litNode(120, "1"))
	rightFn := linkChildren(buildTree(101, "function_declaration", "f",
		buildTree(102, "parameter_list", ""),
		&Node{ID: 103, Kind: "block", Children: []*Node{s1R, s2R}},
	))

	left := NewTree(linkChildren(buildTree(0, "source_file", "", leftFn)), "python", nil)
	right := NewTree(linkChildren(buildTree(100, "source_file", "", rightFn)), "python", nil)
	m := Match(left, right, DefaultMatchConfig())
	es := GenerateEditScript(left, right, m)

	for _, sc := range es.Semantic {
		if sc.LeftNode.Label == "f" && sc.Verdict == VerdictPreserving {
			t.Error("non-Go language should not get preserving verdict from dataflow")
		}
	}
}
