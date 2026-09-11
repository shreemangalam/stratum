package treesitter

import (
	"context"

	"github.com/shreemangalam/stratum/server/internal/core"
)

func (p *Parser) parseJava(_ context.Context, source []byte) (*core.Tree, error) {
	s := newScanner(source)
	root := p.makeNode("source_file", "", 1, 0, 0)

	for !s.eof() {
		s.skipWhitespaceAndComments()
		if s.eof() {
			break
		}

		startLine, startCol, startOff := s.line, s.col, s.pos

		switch {
		case s.matchWord("package"):
			node := p.parseJavaPackage(s, startLine, startCol, startOff)
			p.addChild(root, node)

		case s.matchWord("import"):
			node := p.parseJavaImport(s, source, startLine, startCol, startOff)
			p.addChild(root, node)

		case s.peek() == '@':
			s.advance()
			if isIdentStart(s.peek()) {
				s.readWord()
			}
			if !s.eof() && s.peek() == '(' {
				s.advance()
				s.skipBalanced('(', ')')
			}

		case p.javaIsClassLike(s):
			node := p.parseJavaClass(s, source, startLine, startCol, startOff)
			p.addChild(root, node)

		default:
			// Skip modifiers
			if p.javaSkipModifiers(s) {
				continue
			}
			p.javaSkipStatement(s)
		}
	}

	root.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
	p.setValues(root, source)
	return core.NewTree(root, p.lang, source), nil
}

func (p *Parser) parseJavaPackage(s *scanner, line, col, off int) *core.Node {
	s.pos += 7 // "package"
	s.col += 7
	s.skipWhitespace()
	name := ""
	start := s.pos
	for !s.eof() && s.peek() != ';' && s.peek() != '\n' {
		s.advance()
	}
	name = string(s.src[start:s.pos])
	if !s.eof() && s.peek() == ';' {
		s.advance()
	}
	node := p.makeNode("package_declaration", name, line, col, off)
	node.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
	return node
}

func (p *Parser) parseJavaImport(s *scanner, _ []byte, line, col, off int) *core.Node {
	s.pos += 6 // "import"
	s.col += 6
	s.skipWhitespace()

	isStatic := false
	if s.matchWord("static") {
		isStatic = true
		s.pos += 6
		s.col += 6
		s.skipWhitespace()
	}
	_ = isStatic

	start := s.pos
	for !s.eof() && s.peek() != ';' && s.peek() != '\n' {
		s.advance()
	}
	label := string(s.src[start:s.pos])
	if !s.eof() && s.peek() == ';' {
		s.advance()
	}

	node := p.makeNode("import_declaration", label, line, col, off)
	node.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
	return node
}

func (p *Parser) javaIsClassLike(s *scanner) bool {
	return s.matchWord("class") || s.matchWord("interface") || s.matchWord("enum") || s.matchWord("record")
}

func (p *Parser) parseJavaClass(s *scanner, _ []byte, line, col, off int) *core.Node {
	keyword := s.readWord()
	s.skipWhitespaceAndComments()

	name := ""
	if isIdentStart(s.peek()) {
		name = s.readWord()
	}

	kind := "class_declaration"
	switch keyword {
	case "interface":
		kind = "interface_declaration"
	case "enum":
		kind = "enum_declaration"
	case "record":
		kind = "record_declaration"
	}

	// Skip type params, extends, implements
	for !s.eof() && s.peek() != '{' {
		if s.peek() == '<' {
			s.advance()
			s.skipBalanced('<', '>')
		} else {
			s.advance()
		}
	}

	classNode := p.makeNode(kind, name, line, col, off)

	if !s.eof() && s.peek() == '{' {
		s.advance()
		p.parseJavaClassBody(s, classNode)
	}

	classNode.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
	return classNode
}

func (p *Parser) parseJavaClassBody(s *scanner, parent *core.Node) {
	depth := 1
	for !s.eof() && depth > 0 {
		s.skipWhitespaceAndComments()
		if s.eof() {
			break
		}

		if s.peek() == '}' {
			depth--
			s.advance()
			continue
		}

		mLine, mCol, mOff := s.line, s.col, s.pos

		// Skip annotations
		if s.peek() == '@' {
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

		// Skip modifiers
		p.javaSkipModifiers(s)
		s.skipWhitespaceAndComments()

		// Nested class
		if p.javaIsClassLike(s) {
			inner := p.parseJavaClass(s, nil, mLine, mCol, mOff)
			p.addChild(parent, inner)
			continue
		}

		// Could be a constructor, method, or field
		// Read type + name, look for '('
		if !isIdentStart(s.peek()) && s.peek() != '<' {
			s.advance()
			continue
		}

		// Skip type parameters
		if s.peek() == '<' {
			s.advance()
			s.skipBalanced('<', '>')
			s.skipWhitespaceAndComments()
		}

		firstName := ""
		if isIdentStart(s.peek()) {
			firstName = s.readWord()
		}
		s.skipWhitespaceAndComments()

		// Type params after name
		if s.peek() == '<' {
			s.advance()
			s.skipBalanced('<', '>')
			s.skipWhitespaceAndComments()
		}

		// Array brackets
		for s.peek() == '[' {
			s.advance()
			s.skipBalanced('[', ']')
			s.skipWhitespaceAndComments()
		}

		if s.peek() == '(' {
			// Constructor or method with no return type distinction from name
			s.advance()
			s.skipBalanced('(', ')')
			s.skipWhitespaceAndComments()

			// throws clause
			if s.matchWord("throws") {
				s.readWord()
				for !s.eof() && s.peek() != '{' && s.peek() != ';' {
					s.advance()
				}
			}

			if s.peek() == '{' {
				s.advance()
				s.skipBalanced('{', '}')
			} else if s.peek() == ';' {
				s.advance()
			}

			method := p.makeNode("method_declaration", firstName, mLine, mCol, mOff)
			method.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
			p.addChild(parent, method)
			continue
		}

		// This is "Type name" — read the second identifier
		secondName := ""
		if isIdentStart(s.peek()) {
			secondName = s.readWord()
		}
		s.skipWhitespaceAndComments()

		// Array brackets after name
		for s.peek() == '[' {
			s.advance()
			s.skipBalanced('[', ']')
			s.skipWhitespaceAndComments()
		}

		if s.peek() == '(' {
			// Method: firstName is return type, secondName is method name
			s.advance()
			s.skipBalanced('(', ')')
			s.skipWhitespaceAndComments()

			if s.matchWord("throws") {
				s.readWord()
				for !s.eof() && s.peek() != '{' && s.peek() != ';' {
					s.advance()
				}
			}

			if s.peek() == '{' {
				s.advance()
				s.skipBalanced('{', '}')
			} else if s.peek() == ';' {
				s.advance()
			}

			method := p.makeNode("method_declaration", secondName, mLine, mCol, mOff)
			method.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
			p.addChild(parent, method)
		} else {
			// Field declaration
			name := secondName
			if name == "" {
				name = firstName
			}
			for !s.eof() && s.peek() != ';' {
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

			field := p.makeNode("field_declaration", name, mLine, mCol, mOff)
			field.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
			p.addChild(parent, field)
		}
	}
}

func (p *Parser) javaSkipModifiers(s *scanner) bool {
	found := false
	for !s.eof() {
		if s.matchWord("public") || s.matchWord("private") || s.matchWord("protected") ||
			s.matchWord("static") || s.matchWord("final") || s.matchWord("abstract") ||
			s.matchWord("synchronized") || s.matchWord("volatile") || s.matchWord("transient") ||
			s.matchWord("native") || s.matchWord("default") || s.matchWord("strictfp") {
			s.readWord()
			s.skipWhitespaceAndComments()
			found = true
		} else {
			break
		}
	}
	return found
}

func (p *Parser) javaSkipStatement(s *scanner) {
	for !s.eof() {
		ch := s.peek()
		switch {
		case ch == ';':
			s.advance()
			return
		case ch == '{':
			s.advance()
			s.skipBalanced('{', '}')
			return
		case ch == '\'' || ch == '"':
			s.skipString(ch)
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
