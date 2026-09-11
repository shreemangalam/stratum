package treesitter

import (
	"context"

	"github.com/shreemangalam/stratum/server/internal/core"
)

func (p *Parser) parseJavaScript(_ context.Context, source []byte) (*core.Tree, error) {
	s := newScanner(source)
	root := p.makeNode("source_file", "", 1, 0, 0)
	root.Span.End = core.Location{Line: s.line, Column: s.col, Offset: len(source)}

	for !s.eof() {
		s.skipWhitespaceAndComments()
		if s.eof() {
			break
		}

		startLine, startCol, startOff := s.line, s.col, s.pos

		switch {
		case s.matchWord("import"):
			node := p.parseJSImport(s, source, startLine, startCol, startOff)
			p.addChild(root, node)

		case s.matchWord("export"):
			node := p.parseJSExport(s, source, startLine, startCol, startOff)
			p.addChild(root, node)

		case s.matchWord("function"):
			node := p.parseJSFunction(s, source, startLine, startCol, startOff)
			p.addChild(root, node)

		case s.matchWord("async") && p.jsLookaheadFunction(s):
			node := p.parseJSAsyncFunction(s, source, startLine, startCol, startOff)
			p.addChild(root, node)

		case s.matchWord("class"):
			node := p.parseJSClass(s, source, startLine, startCol, startOff)
			p.addChild(root, node)

		case s.matchWord("const") || s.matchWord("let") || s.matchWord("var"):
			node := p.parseJSVarDecl(s, source, startLine, startCol, startOff)
			p.addChild(root, node)

		case s.matchWord("interface") || (s.matchWord("type") && p.lang == "typescript"):
			node := p.parseJSTypeDecl(s, source, startLine, startCol, startOff)
			p.addChild(root, node)

		default:
			p.jsSkipStatement(s)
		}
	}

	root.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
	p.setValues(root, source)
	return core.NewTree(root, p.lang, source), nil
}

func (p *Parser) parseJSImport(s *scanner, source []byte, line, col, off int) *core.Node {
	s.pos += 6 // "import"
	s.col += 6
	label := ""
	for !s.eof() && s.peek() != '\n' && s.peek() != ';' {
		s.advance()
	}
	if s.peek() == ';' {
		s.advance()
	}
	text := string(source[off:s.pos])
	if from := extractFrom(text); from != "" {
		label = from
	}
	return p.makeNode("import_declaration", label, line, col, off)
}

func (p *Parser) parseJSExport(s *scanner, source []byte, line, col, off int) *core.Node {
	s.pos += 6 // "export"
	s.col += 6
	s.skipWhitespaceAndComments()

	if s.matchWord("default") {
		s.pos += 7
		s.col += 7
		s.skipWhitespaceAndComments()
	}

	if s.matchWord("function") {
		inner := p.parseJSFunction(s, source, line, col, off)
		inner.Kind = "export_function"
		return inner
	}
	if s.matchWord("async") && p.jsLookaheadFunction(s) {
		inner := p.parseJSAsyncFunction(s, source, line, col, off)
		inner.Kind = "export_function"
		return inner
	}
	if s.matchWord("class") {
		inner := p.parseJSClass(s, source, line, col, off)
		inner.Kind = "export_class"
		return inner
	}
	if s.matchWord("const") || s.matchWord("let") || s.matchWord("var") {
		inner := p.parseJSVarDecl(s, source, line, col, off)
		inner.Kind = "export_declaration"
		return inner
	}
	if s.matchWord("interface") || s.matchWord("type") {
		inner := p.parseJSTypeDecl(s, source, line, col, off)
		inner.Kind = "export_type"
		return inner
	}

	// export { ... } or export * from
	for !s.eof() && s.peek() != '\n' && s.peek() != ';' {
		if s.peek() == '{' {
			s.advance()
			s.skipBalanced('{', '}')
		} else {
			s.advance()
		}
	}
	if !s.eof() && s.peek() == ';' {
		s.advance()
	}
	return p.makeNode("export_declaration", "", line, col, off)
}

func (p *Parser) parseJSFunction(s *scanner, _ []byte, line, col, off int) *core.Node {
	s.pos += 8 // "function"
	s.col += 8
	if !s.eof() && s.peek() == '*' {
		s.advance()
	}
	s.skipWhitespaceAndComments()

	name := ""
	if !s.eof() && isIdentStart(s.peek()) {
		name = s.readWord()
	}

	s.skipWhitespaceAndComments()
	if !s.eof() && s.peek() == '(' {
		s.advance()
		s.skipBalanced('(', ')')
	}

	// TS return type
	s.skipWhitespaceAndComments()
	if !s.eof() && s.peek() == ':' {
		s.advance()
		p.jsSkipTypeAnnotation(s)
	}

	s.skipWhitespaceAndComments()
	if !s.eof() && s.peek() == '{' {
		s.advance()
		s.skipBalanced('{', '}')
	}

	node := p.makeNode("function_declaration", name, line, col, off)
	node.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
	return node
}

func (p *Parser) parseJSAsyncFunction(s *scanner, source []byte, line, col, off int) *core.Node {
	s.pos += 5 // "async"
	s.col += 5
	s.skipWhitespaceAndComments()
	node := p.parseJSFunction(s, source, line, col, off)
	return node
}

func (p *Parser) parseJSClass(s *scanner, _ []byte, line, col, off int) *core.Node {
	s.pos += 5 // "class"
	s.col += 5
	s.skipWhitespaceAndComments()

	name := ""
	if !s.eof() && isIdentStart(s.peek()) {
		name = s.readWord()
	}

	// skip extends/implements
	for !s.eof() && s.peek() != '{' {
		s.advance()
	}

	classNode := p.makeNode("class_declaration", name, line, col, off)

	if !s.eof() && s.peek() == '{' {
		bodyStart := s.pos
		s.advance()
		p.parseJSClassBody(s, classNode, bodyStart)
	}

	classNode.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
	return classNode
}

func (p *Parser) parseJSClassBody(s *scanner, parent *core.Node, _ int) {
	depth := 1
	for !s.eof() && depth > 0 {
		s.skipWhitespaceAndComments()
		if s.eof() {
			break
		}

		ch := s.peek()
		if ch == '}' {
			depth--
			s.advance()
			continue
		}

		mLine, mCol, mOff := s.line, s.col, s.pos

		// Skip decorators
		if ch == '@' {
			s.advance()
			if isIdentStart(s.peek()) {
				s.readWord()
			}
			if !s.eof() && s.peek() == '(' {
				s.advance()
				s.skipBalanced('(', ')')
			}
			continue
		}

		// Skip access modifiers (TS)
		if s.matchWord("static") || s.matchWord("private") || s.matchWord("protected") || s.matchWord("public") || s.matchWord("readonly") || s.matchWord("abstract") || s.matchWord("override") {
			s.readWord()
			s.skipWhitespaceAndComments()
		}

		isAsync := false
		if s.matchWord("async") {
			isAsync = true
			s.readWord()
			s.skipWhitespaceAndComments()
			_ = isAsync
		}

		isGet := false
		isSetter := false
		if s.matchWord("get") && s.pos+3 < len(s.src) && !isIdentChar(s.src[s.pos+3]) {
			isGet = true
			s.readWord()
			s.skipWhitespaceAndComments()
		} else if s.matchWord("set") && s.pos+3 < len(s.src) && !isIdentChar(s.src[s.pos+3]) {
			isSetter = true
			s.readWord()
			s.skipWhitespaceAndComments()
		}
		_, _ = isGet, isSetter

		if s.peek() == '*' {
			s.advance()
			s.skipWhitespaceAndComments()
		}

		if s.peek() == '[' {
			s.advance()
			s.skipBalanced('[', ']')
			s.skipWhitespaceAndComments()
		}

		name := ""
		if isIdentStart(s.peek()) {
			name = s.readWord()
		}
		s.skipWhitespaceAndComments()

		if s.peek() == '(' || s.peek() == '<' {
			if s.peek() == '<' {
				s.advance()
				s.skipBalanced('<', '>')
				s.skipWhitespaceAndComments()
			}
			if s.peek() == '(' {
				s.advance()
				s.skipBalanced('(', ')')
			}
			s.skipWhitespaceAndComments()
			if s.peek() == ':' {
				s.advance()
				p.jsSkipTypeAnnotation(s)
			}
			s.skipWhitespaceAndComments()
			if s.peek() == '{' {
				s.advance()
				s.skipBalanced('{', '}')
			}
			method := p.makeNode("method_definition", name, mLine, mCol, mOff)
			method.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
			p.addChild(parent, method)
		} else {
			// Property
			for !s.eof() && s.peek() != ';' && s.peek() != '\n' && s.peek() != '}' {
				if s.peek() == '{' {
					s.advance()
					s.skipBalanced('{', '}')
				} else {
					s.advance()
				}
			}
			if s.peek() == ';' {
				s.advance()
			}
			if name != "" {
				prop := p.makeNode("property_declaration", name, mLine, mCol, mOff)
				prop.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
				p.addChild(parent, prop)
			}
		}
	}
}

func (p *Parser) parseJSVarDecl(s *scanner, source []byte, line, col, off int) *core.Node {
	keyword := s.readWord()
	_ = keyword
	s.skipWhitespaceAndComments()

	name := ""
	if !s.eof() && isIdentStart(s.peek()) {
		name = s.readWord()
	} else if !s.eof() && (s.peek() == '{' || s.peek() == '[') {
		// destructuring
		open := s.peek()
		close := byte('}')
		if open == '[' {
			close = ']'
		}
		s.advance()
		s.skipBalanced(open, close)
	}

	s.skipWhitespaceAndComments()

	// Check if this is an arrow function or function expression
	kind := "variable_declaration"
	if s.peek() == ':' {
		s.advance()
		p.jsSkipTypeAnnotation(s)
		s.skipWhitespaceAndComments()
	}

	if s.peek() == '=' {
		s.advance()
		s.skipWhitespaceAndComments()

		if s.matchWord("function") || (s.matchWord("async") && p.jsLookaheadFunction(s)) {
			kind = "function_declaration"
		} else if p.jsLookaheadArrow(s, source) {
			kind = "function_declaration"
		}
	}

	// Skip to end of statement
	for !s.eof() && s.peek() != ';' && s.peek() != '\n' {
		ch := s.peek()
		switch ch {
		case '{':
			s.advance()
			s.skipBalanced('{', '}')
		case '(':
			s.advance()
			s.skipBalanced('(', ')')
		case '[':
			s.advance()
			s.skipBalanced('[', ']')
		case '`':
			s.skipTemplateString()
		case '\'', '"':
			s.skipString(ch)
		default:
			s.advance()
		}
	}
	if !s.eof() && s.peek() == ';' {
		s.advance()
	}

	node := p.makeNode(kind, name, line, col, off)
	node.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
	return node
}

func (p *Parser) parseJSTypeDecl(s *scanner, _ []byte, line, col, off int) *core.Node {
	keyword := s.readWord()
	s.skipWhitespaceAndComments()

	name := ""
	if isIdentStart(s.peek()) {
		name = s.readWord()
	}

	kind := "type_alias"
	if keyword == "interface" {
		kind = "interface_declaration"
	}

	// Skip to the body or end
	for !s.eof() {
		ch := s.peek()
		if ch == '{' {
			s.advance()
			s.skipBalanced('{', '}')
			break
		}
		if ch == ';' || ch == '\n' {
			if ch == ';' {
				s.advance()
			}
			break
		}
		s.advance()
	}

	node := p.makeNode(kind, name, line, col, off)
	node.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
	return node
}

func (p *Parser) jsSkipStatement(s *scanner) {
	for !s.eof() {
		ch := s.peek()
		switch {
		case ch == ';':
			s.advance()
			return
		case ch == '\n':
			s.advance()
			return
		case ch == '{':
			s.advance()
			s.skipBalanced('{', '}')
			return
		case ch == '(':
			s.advance()
			s.skipBalanced('(', ')')
		case ch == '\'' || ch == '"':
			s.skipString(ch)
		case ch == '`':
			s.skipTemplateString()
		case ch == '/' && s.peek2() == '/':
			s.skipLineComment()
		case ch == '/' && s.peek2() == '*':
			s.advance()
			s.advance()
			s.skipBlockComment()
		default:
			s.advance()
		}
	}
}

func (p *Parser) jsSkipTypeAnnotation(s *scanner) {
	depth := 0
	for !s.eof() {
		ch := s.peek()
		if ch == '<' {
			depth++
			s.advance()
		} else if ch == '>' && depth > 0 {
			depth--
			s.advance()
		} else if depth == 0 && (ch == '{' || ch == '(' || ch == ',' || ch == ';' || ch == '\n') {
			return
		} else if ch == '=' && s.peek2() == '>' {
			return
		} else {
			s.advance()
		}
	}
}

func (p *Parser) jsLookaheadFunction(s *scanner) bool {
	saved := *s
	saved.pos += 5 // "async"
	for saved.pos < len(s.src) && isWhitespace(s.src[saved.pos]) {
		saved.pos++
	}
	if saved.pos+8 <= len(s.src) && string(s.src[saved.pos:saved.pos+8]) == "function" {
		return true
	}
	return false
}

func (p *Parser) jsLookaheadArrow(s *scanner, _ []byte) bool {
	saved := *s
	if saved.peek() == '(' {
		saved.advance()
		saved.skipBalanced('(', ')')
	} else if isIdentStart(saved.peek()) {
		saved.readWord()
	} else {
		return false
	}

	saved.skipWhitespaceAndComments()
	if saved.peek() == ':' {
		saved.advance()
		for !saved.eof() && saved.peek() != '=' && saved.peek() != '{' && saved.peek() != '\n' {
			saved.advance()
		}
	}

	return saved.peek() == '=' && saved.peek2() == '>'
}

func extractFrom(text string) string {
	idx := 0
	for idx < len(text) {
		if idx+4 < len(text) && text[idx:idx+4] == "from" && (idx == 0 || text[idx-1] == ' ') {
			rest := text[idx+4:]
			for i := 0; i < len(rest); i++ {
				if rest[i] == '\'' || rest[i] == '"' {
					end := i + 1
					for end < len(rest) && rest[end] != rest[i] {
						end++
					}
					if end < len(rest) {
						return rest[i+1 : end]
					}
				}
			}
		}
		idx++
	}
	return ""
}

func (p *Parser) makeNode(kind, label string, line, col, off int) *core.Node {
	return &core.Node{
		ID:    core.NodeID(p.nextID.Add(1)),
		Kind:  kind,
		Label: label,
		Span: core.Span{
			Start: core.Location{Line: line, Column: col, Offset: off},
			End:   core.Location{Line: line, Column: col, Offset: off},
		},
	}
}

func (p *Parser) addChild(parent, child *core.Node) {
	child.Parent = parent
	parent.Children = append(parent.Children, child)
}

func (p *Parser) setValues(node *core.Node, source []byte) {
	if node.Span.Start.Offset >= 0 && node.Span.End.Offset <= len(source) && node.Span.Start.Offset < node.Span.End.Offset {
		node.Value = string(source[node.Span.Start.Offset:node.Span.End.Offset])
	}
	for _, child := range node.Children {
		if child.Span.End.Offset == child.Span.Start.Offset {
			child.Span.End = node.Span.End
		}
		p.setValues(child, source)
	}
}
