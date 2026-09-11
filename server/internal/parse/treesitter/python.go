package treesitter

import (
	"context"

	"github.com/shreemangalam/stratum/server/internal/core"
)

func (p *Parser) parsePython(_ context.Context, source []byte) (*core.Tree, error) {
	lines := splitLines(source)
	root := p.makeNode("source_file", "", 1, 0, 0)

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := trimLeadingWhitespace(line.text)
		indent := len(line.text) - len(trimmed)

		if trimmed == "" || trimmed[0] == '#' {
			i++
			continue
		}

		switch {
		case startsWith(trimmed, "import ") || startsWith(trimmed, "from "):
			node := p.makePythonImport(line, trimmed)
			p.addChild(root, node)
			i++

		case startsWith(trimmed, "class "):
			node, end := p.parsePythonClass(lines, i, indent)
			p.addChild(root, node)
			i = end

		case startsWith(trimmed, "def ") || startsWith(trimmed, "async def "):
			node, end := p.parsePythonFunction(lines, i, indent)
			p.addChild(root, node)
			i = end

		case indent == 0 && (startsWith(trimmed, "@")):
			// Decorator — skip it, let the next def/class pick it up
			i++

		default:
			i++
		}
	}

	root.Span.End = core.Location{Line: len(lines), Column: 0, Offset: len(source)}
	p.setValues(root, source)
	return core.NewTree(root, p.lang, source), nil
}

func (p *Parser) makePythonImport(line sourceLine, trimmed string) *core.Node {
	label := ""
	if startsWith(trimmed, "from ") {
		rest := trimmed[5:]
		if sp := indexOf(rest, ' '); sp >= 0 {
			label = rest[:sp]
		}
	} else if startsWith(trimmed, "import ") {
		rest := trimmed[7:]
		if sp := indexOf(rest, ' '); sp >= 0 {
			label = rest[:sp]
		} else {
			label = rest
		}
	}
	node := p.makeNode("import_statement", label, line.num, 0, line.off)
	node.Span.End = core.Location{Line: line.num, Column: len(line.text), Offset: line.off + len(line.text)}
	return node
}

func (p *Parser) parsePythonFunction(lines []sourceLine, start, baseIndent int) (*core.Node, int) {
	line := lines[start]
	trimmed := trimLeadingWhitespace(line.text)

	name := ""
	prefix := "def "
	if startsWith(trimmed, "async def ") {
		prefix = "async def "
	}
	rest := trimmed[len(prefix):]
	if paren := indexOf(rest, '('); paren >= 0 {
		name = rest[:paren]
	}

	node := p.makeNode("function_definition", name, line.num, baseIndent, line.off)

	end := p.pythonBlockEnd(lines, start, baseIndent)
	if end > start {
		endLine := lines[end-1]
		node.Span.End = core.Location{Line: endLine.num, Column: len(endLine.text), Offset: endLine.off + len(endLine.text)}
	}
	return node, end
}

func (p *Parser) parsePythonClass(lines []sourceLine, start, baseIndent int) (*core.Node, int) {
	line := lines[start]
	trimmed := trimLeadingWhitespace(line.text)

	name := ""
	rest := trimmed[6:] // skip "class "
	for i := 0; i < len(rest); i++ {
		if rest[i] == '(' || rest[i] == ':' {
			name = rest[:i]
			break
		}
	}
	if name == "" {
		name = rest
	}

	classNode := p.makeNode("class_definition", name, line.num, baseIndent, line.off)
	end := p.pythonBlockEnd(lines, start, baseIndent)

	// Find methods inside the class
	for i := start + 1; i < end; i++ {
		memberLine := lines[i]
		memberTrimmed := trimLeadingWhitespace(memberLine.text)
		memberIndent := len(memberLine.text) - len(memberTrimmed)

		if memberIndent <= baseIndent {
			continue
		}

		if startsWith(memberTrimmed, "def ") || startsWith(memberTrimmed, "async def ") {
			method, methodEnd := p.parsePythonFunction(lines, i, memberIndent)
			method.Kind = "method_definition"
			p.addChild(classNode, method)
			i = methodEnd - 1
		}
	}

	if end > start {
		endLine := lines[end-1]
		classNode.Span.End = core.Location{Line: endLine.num, Column: len(endLine.text), Offset: endLine.off + len(endLine.text)}
	}
	return classNode, end
}

// pythonBlockEnd returns the line index past the end of a block starting at start.
func (p *Parser) pythonBlockEnd(lines []sourceLine, start, baseIndent int) int {
	i := start + 1
	for i < len(lines) {
		trimmed := trimLeadingWhitespace(lines[i].text)
		if trimmed == "" || trimmed[0] == '#' {
			i++
			continue
		}
		indent := len(lines[i].text) - len(trimmed)
		if indent <= baseIndent {
			return i
		}
		i++
	}
	return i
}

type sourceLine struct {
	text string
	num  int
	off  int
}

func splitLines(source []byte) []sourceLine {
	var lines []sourceLine
	line := 1
	start := 0
	for i, b := range source {
		if b == '\n' {
			lines = append(lines, sourceLine{text: string(source[start:i]), num: line, off: start})
			line++
			start = i + 1
		}
	}
	if start <= len(source) {
		lines = append(lines, sourceLine{text: string(source[start:]), num: line, off: start})
	}
	return lines
}

func trimLeadingWhitespace(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			return s[i:]
		}
	}
	return ""
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func indexOf(s string, ch byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ch {
			return i
		}
	}
	return -1
}
