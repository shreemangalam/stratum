package treesitter

// scanner provides shared utilities for heuristic structural parsers.
// It handles brace/paren/bracket matching while respecting strings and comments.

type scanner struct {
	src  []byte
	pos  int
	line int
	col  int
}

func newScanner(src []byte) *scanner {
	return &scanner{src: src, line: 1, col: 0}
}

func (s *scanner) eof() bool { return s.pos >= len(s.src) }

func (s *scanner) peek() byte {
	if s.eof() {
		return 0
	}
	return s.src[s.pos]
}

func (s *scanner) peek2() byte {
	if s.pos+1 >= len(s.src) {
		return 0
	}
	return s.src[s.pos+1]
}

func (s *scanner) advance() byte {
	if s.eof() {
		return 0
	}
	ch := s.src[s.pos]
	s.pos++
	if ch == '\n' {
		s.line++
		s.col = 0
	} else {
		s.col++
	}
	return ch
}

func (s *scanner) skipWhitespace() {
	for !s.eof() && isWhitespace(s.peek()) {
		s.advance()
	}
}

func (s *scanner) skipToEOL() {
	for !s.eof() && s.peek() != '\n' {
		s.advance()
	}
}

func (s *scanner) skipLineComment() {
	s.skipToEOL()
}

func (s *scanner) skipBlockComment() {
	for !s.eof() {
		if s.peek() == '*' && s.peek2() == '/' {
			s.advance()
			s.advance()
			return
		}
		s.advance()
	}
}

func (s *scanner) skipString(quote byte) {
	s.advance() // opening quote
	for !s.eof() {
		ch := s.advance()
		if ch == '\\' {
			s.advance() // skip escaped char
		} else if ch == quote {
			return
		}
	}
}

func (s *scanner) skipTemplateString() {
	s.advance() // opening backtick
	depth := 0
	for !s.eof() {
		ch := s.peek()
		if ch == '\\' {
			s.advance()
			s.advance()
		} else if ch == '$' && s.peek2() == '{' {
			s.advance()
			s.advance()
			depth++
		} else if ch == '}' && depth > 0 {
			s.advance()
			depth--
		} else if ch == '`' && depth == 0 {
			s.advance()
			return
		} else {
			s.advance()
		}
	}
}

// skipBalanced advances past a matched pair of braces/parens/brackets.
// The opening character must have been consumed already.
func (s *scanner) skipBalanced(open, close byte) {
	depth := 1
	for !s.eof() && depth > 0 {
		ch := s.peek()
		switch {
		case ch == open:
			depth++
			s.advance()
		case ch == close:
			depth--
			s.advance()
		case ch == '/' && s.peek2() == '/':
			s.skipLineComment()
		case ch == '/' && s.peek2() == '*':
			s.advance()
			s.advance()
			s.skipBlockComment()
		case ch == '\'' || ch == '"':
			s.skipString(ch)
		case ch == '`':
			s.skipTemplateString()
		default:
			s.advance()
		}
	}
}

// readWord reads an identifier-like word at the current position.
func (s *scanner) readWord() string {
	start := s.pos
	for !s.eof() && isIdentChar(s.peek()) {
		s.advance()
	}
	return string(s.src[start:s.pos])
}

// skipWhitespaceAndComments skips whitespace and comments.
func (s *scanner) skipWhitespaceAndComments() {
	for !s.eof() {
		ch := s.peek()
		if isWhitespace(ch) {
			s.advance()
		} else if ch == '/' && s.peek2() == '/' {
			s.skipLineComment()
		} else if ch == '/' && s.peek2() == '*' {
			s.advance()
			s.advance()
			s.skipBlockComment()
		} else {
			break
		}
	}
}

// matchWord checks if the source at pos starts with a keyword followed by non-ident char.
func (s *scanner) matchWord(word string) bool {
	end := s.pos + len(word)
	if end > len(s.src) {
		return false
	}
	if string(s.src[s.pos:end]) != word {
		return false
	}
	if end < len(s.src) && isIdentChar(s.src[end]) {
		return false
	}
	return true
}

func isWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func isIdentChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '$'
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' || ch == '$'
}
