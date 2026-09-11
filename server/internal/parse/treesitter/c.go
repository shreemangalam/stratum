package treesitter

import (
	"context"

	"github.com/shreemangalam/stratum/server/internal/core"
)

func (p *Parser) parseC(_ context.Context, source []byte) (*core.Tree, error) {
	s := newScanner(source)
	root := p.makeNode("source_file", "", 1, 0, 0)

	for !s.eof() {
		s.skipWhitespaceAndComments()
		if s.eof() {
			break
		}

		startLine, startCol, startOff := s.line, s.col, s.pos

		switch {
		case s.peek() == '#':
			node := p.parseCPreprocessor(s, source, startLine, startCol, startOff)
			p.addChild(root, node)

		case s.matchWord("typedef"):
			node := p.parseCTypedef(s, source, startLine, startCol, startOff)
			p.addChild(root, node)

		case s.matchWord("struct") || s.matchWord("union"):
			node := p.parseCStructOrUnion(s, source, startLine, startCol, startOff)
			p.addChild(root, node)

		case s.matchWord("enum"):
			node := p.parseCEnum(s, source, startLine, startCol, startOff)
			p.addChild(root, node)

		case s.matchWord("extern"):
			s.readWord()
			s.skipWhitespace()
			if s.peek() == '"' {
				s.skipString('"')
				s.skipWhitespaceAndComments()
				if s.peek() == '{' {
					s.advance()
					s.skipBalanced('{', '}')
				}
			} else {
				p.cSkipStatement(s)
			}

		default:
			node := p.parseCTopLevel(s, source, startLine, startCol, startOff)
			if node != nil {
				p.addChild(root, node)
			}
		}
	}

	root.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
	p.setValues(root, source)
	return core.NewTree(root, p.lang, source), nil
}

func (p *Parser) parseCPreprocessor(s *scanner, src []byte, line, col, off int) *core.Node {
	s.advance() // skip '#'
	s.skipWhitespace()

	directive := ""
	if isIdentStart(s.peek()) {
		directive = s.readWord()
	}

	kind := "preprocessor"
	label := "#" + directive

	switch directive {
	case "include":
		kind = "include_directive"
		s.skipWhitespace()
		start := s.pos
		if s.peek() == '<' || s.peek() == '"' {
			end := byte('>')
			if s.peek() == '"' {
				end = '"'
			}
			s.advance()
			for !s.eof() && s.peek() != end && s.peek() != '\n' {
				s.advance()
			}
			if !s.eof() && s.peek() == end {
				s.advance()
			}
			label = string(src[start:s.pos])
		}
	case "define":
		kind = "macro_definition"
		s.skipWhitespace()
		if isIdentStart(s.peek()) {
			label = s.readWord()
		}
	}

	// Consume rest of line, handling backslash continuation
	for !s.eof() {
		if s.peek() == '\n' {
			if s.pos > 0 && src[s.pos-1] == '\\' {
				s.advance() // continue to next line
			} else {
				s.advance()
				break
			}
		} else {
			s.advance()
		}
	}

	node := p.makeNode(kind, label, line, col, off)
	node.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
	return node
}

func (p *Parser) parseCTypedef(s *scanner, _ []byte, line, col, off int) *core.Node {
	s.readWord() // "typedef"
	s.skipWhitespaceAndComments()

	// typedef struct/union/enum { ... } Name;
	if s.matchWord("struct") || s.matchWord("union") || s.matchWord("enum") {
		s.readWord()
		s.skipWhitespaceAndComments()
		// optional tag name
		if isIdentStart(s.peek()) {
			s.readWord()
			s.skipWhitespaceAndComments()
		}
		if s.peek() == '{' {
			s.advance()
			s.skipBalanced('{', '}')
			s.skipWhitespaceAndComments()
		}
	}

	// Now find the name (last identifier before ';')
	name := ""
	for !s.eof() && s.peek() != ';' {
		if s.peek() == '(' {
			s.advance()
			s.skipBalanced('(', ')')
		} else if isIdentStart(s.peek()) {
			name = s.readWord()
		} else {
			s.advance()
		}
	}
	if s.peek() == ';' {
		s.advance()
	}

	node := p.makeNode("typedef_declaration", name, line, col, off)
	node.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
	return node
}

func (p *Parser) parseCStructOrUnion(s *scanner, _ []byte, line, col, off int) *core.Node {
	keyword := s.readWord() // "struct" or "union"
	s.skipWhitespaceAndComments()

	kind := "struct_declaration"
	if keyword == "union" {
		kind = "union_declaration"
	}

	name := ""
	if isIdentStart(s.peek()) {
		name = s.readWord()
		s.skipWhitespaceAndComments()
	}

	parent := p.makeNode(kind, name, line, col, off)

	if s.peek() == '{' {
		s.advance()
		p.parseCStructBody(s, parent)
	}

	// Could be followed by variable declarations
	for !s.eof() && s.peek() != ';' {
		s.advance()
	}
	if s.peek() == ';' {
		s.advance()
	}

	parent.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
	return parent
}

func (p *Parser) parseCStructBody(s *scanner, parent *core.Node) {
	for !s.eof() {
		s.skipWhitespaceAndComments()
		if s.eof() || s.peek() == '}' {
			if !s.eof() {
				s.advance()
			}
			return
		}

		mLine, mCol, mOff := s.line, s.col, s.pos

		// Read type + name for field declaration
		name := ""
		for !s.eof() && s.peek() != ';' && s.peek() != '}' {
			if isIdentStart(s.peek()) {
				name = s.readWord()
			} else if s.peek() == '(' {
				s.advance()
				s.skipBalanced('(', ')')
			} else if s.peek() == '[' {
				s.advance()
				s.skipBalanced('[', ']')
			} else {
				s.advance()
			}
		}
		if s.peek() == ';' {
			s.advance()
		}

		if name != "" {
			field := p.makeNode("field_declaration", name, mLine, mCol, mOff)
			field.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
			p.addChild(parent, field)
		}
	}
}

func (p *Parser) parseCEnum(s *scanner, _ []byte, line, col, off int) *core.Node {
	s.readWord() // "enum"
	s.skipWhitespaceAndComments()

	name := ""
	if isIdentStart(s.peek()) {
		name = s.readWord()
		s.skipWhitespaceAndComments()
	}

	enumNode := p.makeNode("enum_declaration", name, line, col, off)

	if s.peek() == '{' {
		s.advance()
		p.parseCEnumBody(s, enumNode)
	}

	for !s.eof() && s.peek() != ';' {
		s.advance()
	}
	if s.peek() == ';' {
		s.advance()
	}

	enumNode.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
	return enumNode
}

func (p *Parser) parseCEnumBody(s *scanner, parent *core.Node) {
	for !s.eof() {
		s.skipWhitespaceAndComments()
		if s.eof() || s.peek() == '}' {
			if !s.eof() {
				s.advance()
			}
			return
		}

		mLine, mCol, mOff := s.line, s.col, s.pos

		if isIdentStart(s.peek()) {
			name := s.readWord()
			s.skipWhitespaceAndComments()
			// Skip = value
			if s.peek() == '=' {
				s.advance()
				for !s.eof() && s.peek() != ',' && s.peek() != '}' {
					s.advance()
				}
			}
			if s.peek() == ',' {
				s.advance()
			}

			member := p.makeNode("enum_constant", name, mLine, mCol, mOff)
			member.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
			p.addChild(parent, member)
		} else {
			s.advance()
		}
	}
}

func (p *Parser) parseCTopLevel(s *scanner, _ []byte, line, col, off int) *core.Node {
	// Skip storage class and qualifiers
	for !s.eof() {
		if s.matchWord("static") || s.matchWord("inline") || s.matchWord("const") ||
			s.matchWord("volatile") || s.matchWord("register") || s.matchWord("auto") ||
			s.matchWord("unsigned") || s.matchWord("signed") || s.matchWord("long") ||
			s.matchWord("short") || s.matchWord("__attribute__") {
			w := s.readWord()
			s.skipWhitespaceAndComments()
			if w == "__attribute__" && s.peek() == '(' {
				s.advance()
				s.skipBalanced('(', ')')
				s.skipWhitespaceAndComments()
			}
		} else {
			break
		}
	}

	// Check for struct/union/enum as return type
	if s.matchWord("struct") || s.matchWord("union") || s.matchWord("enum") {
		s.readWord()
		s.skipWhitespaceAndComments()
		if isIdentStart(s.peek()) {
			s.readWord()
			s.skipWhitespaceAndComments()
		}
		if s.peek() == '{' {
			s.advance()
			s.skipBalanced('{', '}')
			s.skipWhitespaceAndComments()
		}
	}

	if s.eof() || !isIdentStart(s.peek()) {
		if !s.eof() {
			s.advance()
		}
		return nil
	}

	firstName := s.readWord()
	s.skipWhitespaceAndComments()

	// Pointer stars
	for s.peek() == '*' {
		s.advance()
		s.skipWhitespaceAndComments()
	}

	if s.peek() == '(' {
		// Could be a function with no return type (e.g. main(...)) or func ptr
		s.advance()
		s.skipBalanced('(', ')')
		s.skipWhitespaceAndComments()

		if s.peek() == '{' {
			s.advance()
			s.skipBalanced('{', '}')
			node := p.makeNode("function_definition", firstName, line, col, off)
			node.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
			return node
		}
		if s.peek() == ';' {
			s.advance()
			node := p.makeNode("function_declaration", firstName, line, col, off)
			node.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
			return node
		}
		// Possibly something else, skip to semicolon
		p.cSkipStatement(s)
		return nil
	}

	if !isIdentStart(s.peek()) && s.peek() != '*' {
		p.cSkipStatement(s)
		return nil
	}

	// Read second identifier (the function/variable name)
	for s.peek() == '*' {
		s.advance()
		s.skipWhitespaceAndComments()
	}

	secondName := ""
	if isIdentStart(s.peek()) {
		secondName = s.readWord()
		s.skipWhitespaceAndComments()
	}

	// Array brackets
	for s.peek() == '[' {
		s.advance()
		s.skipBalanced('[', ']')
		s.skipWhitespaceAndComments()
	}

	if s.peek() == '(' {
		// Function: firstName is return type, secondName is name
		s.advance()
		s.skipBalanced('(', ')')
		s.skipWhitespaceAndComments()

		// GCC attributes
		if s.matchWord("__attribute__") {
			s.readWord()
			if s.peek() == '(' {
				s.advance()
				s.skipBalanced('(', ')')
			}
			s.skipWhitespaceAndComments()
		}

		if s.peek() == '{' {
			s.advance()
			s.skipBalanced('{', '}')
			node := p.makeNode("function_definition", secondName, line, col, off)
			node.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
			return node
		}
		if s.peek() == ';' {
			s.advance()
			node := p.makeNode("function_declaration", secondName, line, col, off)
			node.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
			return node
		}

		p.cSkipStatement(s)
		return nil
	}

	// Global variable declaration
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

	node := p.makeNode("variable_declaration", name, line, col, off)
	node.Span.End = core.Location{Line: s.line, Column: s.col, Offset: s.pos}
	return node
}

func (p *Parser) cSkipStatement(s *scanner) {
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
