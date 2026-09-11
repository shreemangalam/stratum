package treesitter

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"sync/atomic"

	"github.com/shreemangalam/stratum/server/internal/core"
)

// Parser implements parse.Parser. Go uses the stdlib ast parser for full
// AST fidelity. JS, TS, Python, Java, C, and C++ use heuristic structural
// scanners. Unsupported languages fall back to line-based parsing.
type Parser struct {
	lang   string
	exts   []string
	nextID atomic.Int64
}

// NewGo creates a parser for Go source using the stdlib go/parser.
func NewGo() *Parser {
	return &Parser{lang: "go", exts: []string{".go"}}
}

// NewGeneric creates a line-based fallback parser for any language.
func NewGeneric(lang string, exts []string) *Parser {
	return &Parser{lang: lang, exts: exts}
}

// SupportedLanguages returns language configs for all built-in parsers.
func SupportedLanguages() []struct {
	Lang string
	Exts []string
} {
	return []struct {
		Lang string
		Exts []string
	}{
		{"go", []string{".go"}},
		{"javascript", []string{".js", ".jsx", ".mjs"}},
		{"typescript", []string{".ts", ".tsx"}},
		{"python", []string{".py"}},
		{"java", []string{".java"}},
		{"c", []string{".c", ".h"}},
		{"cpp", []string{".cpp", ".hpp", ".cc", ".cxx"}},
	}
}

func (p *Parser) Language() string     { return p.lang }
func (p *Parser) Extensions() []string { return p.exts }

func (p *Parser) Parse(ctx context.Context, source []byte) (*core.Tree, error) {
	switch p.lang {
	case "go":
		return p.parseGo(ctx, source)
	case "javascript", "typescript":
		return p.parseJavaScript(ctx, source)
	case "python":
		return p.parsePython(ctx, source)
	case "java":
		return p.parseJava(ctx, source)
	case "c", "cpp":
		return p.parseC(ctx, source)
	default:
		return p.parseGeneric(source)
	}
}

func (p *Parser) parseGo(_ context.Context, source []byte) (*core.Tree, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "input.go", source, parser.AllErrors)
	if err != nil && file == nil {
		return nil, fmt.Errorf("go parse failed: %w", err)
	}

	root := p.convertGoFile(fset, file, source)
	return core.NewTree(root, "go", source), nil
}

func (p *Parser) convertGoFile(fset *token.FileSet, file *ast.File, source []byte) *core.Node {
	root := p.newNode("source_file", "", fset, file.Pos(), file.End(), source)

	if file.Name != nil {
		pkg := p.newNode("package_clause", file.Name.Name, fset, file.Package, file.Name.End(), source)
		root.Children = append(root.Children, pkg)
		pkg.Parent = root
	}

	for _, decl := range file.Decls {
		child := p.convertGoDecl(fset, decl, source)
		if child != nil {
			child.Parent = root
			root.Children = append(root.Children, child)
		}
	}

	return root
}

func (p *Parser) convertGoDecl(fset *token.FileSet, decl ast.Decl, source []byte) *core.Node {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		node := p.newNode("function_declaration", d.Name.Name, fset, d.Pos(), d.End(), source)
		if d.Recv != nil && len(d.Recv.List) > 0 {
			node.Kind = "method_declaration"
		}
		p.convertFuncBody(fset, d, node, source)
		return node

	case *ast.GenDecl:
		switch d.Tok {
		case token.IMPORT:
			node := p.newNode("import_declaration", "", fset, d.Pos(), d.End(), source)
			for _, spec := range d.Specs {
				if is, ok := spec.(*ast.ImportSpec); ok {
					label := ""
					if is.Name != nil {
						label = is.Name.Name
					}
					child := p.newNode("import_spec", label, fset, is.Pos(), is.End(), source)
					child.Value = is.Path.Value
					child.Parent = node
					node.Children = append(node.Children, child)
				}
			}
			return node

		case token.TYPE:
			node := p.newNode("type_declaration", "", fset, d.Pos(), d.End(), source)
			for _, spec := range d.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok {
					child := p.newNode("type_spec", ts.Name.Name, fset, ts.Pos(), ts.End(), source)
					p.convertGoTypeExpr(fset, ts.Type, child, source)
					child.Parent = node
					node.Children = append(node.Children, child)
				}
			}
			return node

		case token.VAR, token.CONST:
			kind := "var_declaration"
			if d.Tok == token.CONST {
				kind = "const_declaration"
			}
			node := p.newNode(kind, "", fset, d.Pos(), d.End(), source)
			for _, spec := range d.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					for _, name := range vs.Names {
						child := p.newNode("var_spec", name.Name, fset, vs.Pos(), vs.End(), source)
						child.Parent = node
						node.Children = append(node.Children, child)
					}
				}
			}
			return node
		}
	}
	return nil
}

func (p *Parser) convertFuncBody(fset *token.FileSet, d *ast.FuncDecl, parent *core.Node, source []byte) {
	if d.Type != nil && d.Type.Params != nil {
		params := p.newNode("parameter_list", "", fset, d.Type.Params.Pos(), d.Type.Params.End(), source)
		params.Parent = parent
		for _, field := range d.Type.Params.List {
			for _, name := range field.Names {
				param := p.newNode("parameter", name.Name, fset, field.Pos(), field.End(), source)
				param.Parent = params
				params.Children = append(params.Children, param)
			}
		}
		parent.Children = append(parent.Children, params)
	}

	if d.Type != nil && d.Type.Results != nil {
		results := p.newNode("result_list", "", fset, d.Type.Results.Pos(), d.Type.Results.End(), source)
		results.Parent = parent
		parent.Children = append(parent.Children, results)
	}

	if d.Body != nil {
		body := p.convertGoBlock(fset, d.Body, source)
		body.Parent = parent
		parent.Children = append(parent.Children, body)
	}
}

func (p *Parser) convertGoBlock(fset *token.FileSet, block *ast.BlockStmt, source []byte) *core.Node {
	node := p.newNode("block", "", fset, block.Pos(), block.End(), source)

	for _, stmt := range block.List {
		child := p.convertGoStmt(fset, stmt, source)
		if child != nil {
			child.Parent = node
			node.Children = append(node.Children, child)
		}
	}

	return node
}

func (p *Parser) convertGoStmt(fset *token.FileSet, stmt ast.Stmt, source []byte) *core.Node {
	switch s := stmt.(type) {
	case *ast.ReturnStmt:
		node := p.newNode("return_statement", "", fset, s.Pos(), s.End(), source)
		for _, expr := range s.Results {
			child := p.convertGoExpr(fset, expr, source)
			if child != nil {
				child.Parent = node
				node.Children = append(node.Children, child)
			}
		}
		return node

	case *ast.IfStmt:
		node := p.newNode("if_statement", "", fset, s.Pos(), s.End(), source)
		if s.Cond != nil {
			cond := p.convertGoExpr(fset, s.Cond, source)
			if cond != nil {
				cond.Parent = node
				node.Children = append(node.Children, cond)
			}
		}
		if s.Body != nil {
			body := p.convertGoBlock(fset, s.Body, source)
			body.Parent = node
			node.Children = append(node.Children, body)
		}
		if s.Else != nil {
			elseNode := p.convertGoStmt(fset, s.Else, source)
			if elseNode != nil {
				elseNode.Parent = node
				node.Children = append(node.Children, elseNode)
			}
		}
		return node

	case *ast.ForStmt:
		node := p.newNode("for_statement", "", fset, s.Pos(), s.End(), source)
		if s.Body != nil {
			body := p.convertGoBlock(fset, s.Body, source)
			body.Parent = node
			node.Children = append(node.Children, body)
		}
		return node

	case *ast.RangeStmt:
		node := p.newNode("range_statement", "", fset, s.Pos(), s.End(), source)
		if s.Body != nil {
			body := p.convertGoBlock(fset, s.Body, source)
			body.Parent = node
			node.Children = append(node.Children, body)
		}
		return node

	case *ast.AssignStmt:
		node := p.newNode("assignment_statement", s.Tok.String(), fset, s.Pos(), s.End(), source)
		if len(s.Lhs) > 0 {
			lhsNode := p.newNode("assignment_lhs", "", fset, s.Lhs[0].Pos(), s.Lhs[len(s.Lhs)-1].End(), source)
			lhsNode.Parent = node
			for _, expr := range s.Lhs {
				child := p.convertGoExpr(fset, expr, source)
				if child != nil {
					child.Parent = lhsNode
					lhsNode.Children = append(lhsNode.Children, child)
				}
			}
			node.Children = append(node.Children, lhsNode)
		}
		if len(s.Rhs) > 0 {
			rhsNode := p.newNode("assignment_rhs", "", fset, s.Rhs[0].Pos(), s.Rhs[len(s.Rhs)-1].End(), source)
			rhsNode.Parent = node
			for _, expr := range s.Rhs {
				child := p.convertGoExpr(fset, expr, source)
				if child != nil {
					child.Parent = rhsNode
					rhsNode.Children = append(rhsNode.Children, child)
				}
			}
			node.Children = append(node.Children, rhsNode)
		}
		return node

	case *ast.ExprStmt:
		return p.convertGoExpr(fset, s.X, source)

	case *ast.DeclStmt:
		if gd, ok := s.Decl.(*ast.GenDecl); ok {
			return p.convertGoDecl(fset, gd, source)
		}

	case *ast.BlockStmt:
		return p.convertGoBlock(fset, s, source)

	case *ast.SwitchStmt:
		node := p.newNode("switch_statement", "", fset, s.Pos(), s.End(), source)
		if s.Body != nil {
			body := p.convertGoBlock(fset, s.Body, source)
			body.Parent = node
			node.Children = append(node.Children, body)
		}
		return node

	case *ast.CaseClause:
		node := p.newNode("case_clause", "", fset, s.Pos(), s.End(), source)
		for _, stmt := range s.Body {
			child := p.convertGoStmt(fset, stmt, source)
			if child != nil {
				child.Parent = node
				node.Children = append(node.Children, child)
			}
		}
		return node

	case *ast.DeferStmt:
		node := p.newNode("defer_statement", "", fset, s.Pos(), s.End(), source)
		if call := p.convertGoExpr(fset, s.Call, source); call != nil {
			call.Parent = node
			node.Children = append(node.Children, call)
		}
		return node

	case *ast.GoStmt:
		node := p.newNode("go_statement", "", fset, s.Pos(), s.End(), source)
		if call := p.convertGoExpr(fset, s.Call, source); call != nil {
			call.Parent = node
			node.Children = append(node.Children, call)
		}
		return node
	}

	return p.newNode("statement", "", fset, stmt.Pos(), stmt.End(), source)
}

func (p *Parser) convertGoExpr(fset *token.FileSet, expr ast.Expr, source []byte) *core.Node {
	switch e := expr.(type) {
	case *ast.Ident:
		node := p.newNode("identifier", e.Name, fset, e.Pos(), e.End(), source)
		node.Value = e.Name
		return node

	case *ast.BasicLit:
		node := p.newNode("literal", "", fset, e.Pos(), e.End(), source)
		node.Value = e.Value
		return node

	case *ast.CallExpr:
		node := p.newNode("call_expression", "", fset, e.Pos(), e.End(), source)
		fn := p.convertGoExpr(fset, e.Fun, source)
		if fn != nil {
			fn.Parent = node
			node.Children = append(node.Children, fn)
		}
		for _, arg := range e.Args {
			child := p.convertGoExpr(fset, arg, source)
			if child != nil {
				child.Parent = node
				node.Children = append(node.Children, child)
			}
		}
		return node

	case *ast.SelectorExpr:
		node := p.newNode("selector_expression", e.Sel.Name, fset, e.Pos(), e.End(), source)
		x := p.convertGoExpr(fset, e.X, source)
		if x != nil {
			x.Parent = node
			node.Children = append(node.Children, x)
		}
		return node

	case *ast.BinaryExpr:
		node := p.newNode("binary_expression", e.Op.String(), fset, e.Pos(), e.End(), source)
		if l := p.convertGoExpr(fset, e.X, source); l != nil {
			l.Parent = node
			node.Children = append(node.Children, l)
		}
		if r := p.convertGoExpr(fset, e.Y, source); r != nil {
			r.Parent = node
			node.Children = append(node.Children, r)
		}
		return node

	case *ast.UnaryExpr:
		node := p.newNode("unary_expression", e.Op.String(), fset, e.Pos(), e.End(), source)
		if x := p.convertGoExpr(fset, e.X, source); x != nil {
			x.Parent = node
			node.Children = append(node.Children, x)
		}
		return node

	case *ast.CompositeLit:
		node := p.newNode("composite_literal", "", fset, e.Pos(), e.End(), source)
		for _, elt := range e.Elts {
			child := p.convertGoExpr(fset, elt, source)
			if child != nil {
				child.Parent = node
				node.Children = append(node.Children, child)
			}
		}
		return node

	case *ast.FuncLit:
		node := p.newNode("function_literal", "", fset, e.Pos(), e.End(), source)
		if e.Body != nil {
			body := p.convertGoBlock(fset, e.Body, source)
			body.Parent = node
			node.Children = append(node.Children, body)
		}
		return node

	case *ast.IndexExpr:
		node := p.newNode("index_expression", "", fset, e.Pos(), e.End(), source)
		if x := p.convertGoExpr(fset, e.X, source); x != nil {
			x.Parent = node
			node.Children = append(node.Children, x)
		}
		return node

	case *ast.SliceExpr:
		return p.newNode("slice_expression", "", fset, e.Pos(), e.End(), source)

	case *ast.TypeAssertExpr:
		return p.newNode("type_assertion", "", fset, e.Pos(), e.End(), source)

	case *ast.StarExpr:
		node := p.newNode("pointer_expression", "", fset, e.Pos(), e.End(), source)
		if x := p.convertGoExpr(fset, e.X, source); x != nil {
			x.Parent = node
			node.Children = append(node.Children, x)
		}
		return node

	case *ast.KeyValueExpr:
		node := p.newNode("key_value", "", fset, e.Pos(), e.End(), source)
		if k := p.convertGoExpr(fset, e.Key, source); k != nil {
			k.Parent = node
			node.Children = append(node.Children, k)
		}
		if v := p.convertGoExpr(fset, e.Value, source); v != nil {
			v.Parent = node
			node.Children = append(node.Children, v)
		}
		return node

	case *ast.ParenExpr:
		return p.convertGoExpr(fset, e.X, source)
	}

	if expr != nil {
		return p.newNode("expression", "", fset, expr.Pos(), expr.End(), source)
	}
	return nil
}

func (p *Parser) convertGoTypeExpr(fset *token.FileSet, expr ast.Expr, parent *core.Node, source []byte) {
	switch e := expr.(type) {
	case *ast.StructType:
		st := p.newNode("struct_type", "", fset, e.Pos(), e.End(), source)
		st.Parent = parent
		if e.Fields != nil {
			for _, field := range e.Fields.List {
				for _, name := range field.Names {
					f := p.newNode("field", name.Name, fset, field.Pos(), field.End(), source)
					f.Parent = st
					st.Children = append(st.Children, f)
				}
			}
		}
		parent.Children = append(parent.Children, st)

	case *ast.InterfaceType:
		it := p.newNode("interface_type", "", fset, e.Pos(), e.End(), source)
		it.Parent = parent
		if e.Methods != nil {
			for _, method := range e.Methods.List {
				for _, name := range method.Names {
					m := p.newNode("method_spec", name.Name, fset, method.Pos(), method.End(), source)
					m.Parent = it
					it.Children = append(it.Children, m)
				}
			}
		}
		parent.Children = append(parent.Children, it)
	}
}

func (p *Parser) newNode(kind, label string, fset *token.FileSet, pos, end token.Pos, source []byte) *core.Node {
	startPos := fset.Position(pos)
	endPos := fset.Position(end)

	node := &core.Node{
		ID:    core.NodeID(p.nextID.Add(1)),
		Kind:  kind,
		Label: label,
		Span: core.Span{
			Start: core.Location{
				Line:   startPos.Line,
				Column: startPos.Column - 1,
				Offset: startPos.Offset,
			},
			End: core.Location{
				Line:   endPos.Line,
				Column: endPos.Column - 1,
				Offset: endPos.Offset,
			},
		},
	}

	if len(source) > 0 && startPos.Offset >= 0 && endPos.Offset <= len(source) && startPos.Offset < endPos.Offset {
		node.Value = string(source[startPos.Offset:endPos.Offset])
	}

	return node
}

// parseGeneric creates a line-based tree as a fallback for languages
// without a native Go parser. Each non-trivial line becomes a leaf node.
// Empty lines and lines containing only braces/punctuation are excluded
// to prevent the matcher from generating spurious move operations.
func (p *Parser) parseGeneric(source []byte) (*core.Tree, error) {
	root := &core.Node{
		ID:   core.NodeID(p.nextID.Add(1)),
		Kind: "source_file",
		Span: core.Span{
			Start: core.Location{Line: 1, Column: 0, Offset: 0},
			End:   core.Location{Line: 1, Column: 0, Offset: len(source)},
		},
	}

	line := 1
	start := 0
	for i, b := range source {
		if b == '\n' {
			text := string(source[start:i])
			if !trivialLine(text) {
				lineNode := &core.Node{
					ID:     core.NodeID(p.nextID.Add(1)),
					Kind:   "line",
					Value:  text,
					Parent: root,
					Span: core.Span{
						Start: core.Location{Line: line, Column: 0, Offset: start},
						End:   core.Location{Line: line, Column: i - start, Offset: i},
					},
				}
				root.Children = append(root.Children, lineNode)
			}
			line++
			start = i + 1
		}
	}
	if start < len(source) {
		text := string(source[start:])
		if !trivialLine(text) {
			lineNode := &core.Node{
				ID:     core.NodeID(p.nextID.Add(1)),
				Kind:   "line",
				Value:  text,
				Parent: root,
				Span: core.Span{
					Start: core.Location{Line: line, Column: 0, Offset: start},
					End:   core.Location{Line: line, Column: len(source) - start, Offset: len(source)},
				},
			}
			root.Children = append(root.Children, lineNode)
		}
	}

	root.Span.End.Line = line
	return core.NewTree(root, p.lang, source), nil
}

// trivialLine returns true for lines that carry no semantic signal and
// would produce false-positive matches: empty/whitespace, lone braces,
// and short punctuation-only lines.
func trivialLine(s string) bool {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) == 0 {
		return true
	}
	for _, r := range trimmed {
		if r != '{' && r != '}' && r != '(' && r != ')' && r != ';' && r != '[' && r != ']' {
			return false
		}
	}
	return true
}
