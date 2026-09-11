// Package xslt parses XML and XSLT documents into core trees using
// encoding/xml. It exists to prove the parser plugin boundary: adding
// this language touched nothing outside internal/parse. The tree is
// structure-aware, not line-based:
//
//   - The node kind is the element name ("xsl:template", "map"), so
//     matching never pairs unrelated elements.
//   - The label is the element's identity attribute (name, match, or
//     id), so a renamed template reports as a rename.
//   - Attributes are child nodes sorted by attribute name: XML
//     attribute order is semantically meaningless, so a pure reorder
//     of attributes produces an identical tree and an empty diff.
package xslt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/shreemangalam/stratum/server/internal/core"
)

const xslNamespace = "http://www.w3.org/1999/XSL/Transform"

// Parser implements parse.Parser for XML and XSLT sources.
type Parser struct {
	nextID atomic.Int64
}

// New creates the XSLT/XML parser.
func New() *Parser {
	return &Parser{}
}

// Language returns the canonical XSLT language identifier.
func (p *Parser) Language() string { return "xslt" }

// Extensions returns the XML and XSLT filename extensions handled by the parser.
func (p *Parser) Extensions() []string { return []string{".xsl", ".xslt", ".xml"} }

// Parse converts XML or XSLT source into Stratum's tree representation.
func (p *Parser) Parse(_ context.Context, source []byte) (*core.Tree, error) {
	lines := lineIndex(source)
	dec := xml.NewDecoder(strings.NewReader(string(source)))

	root := &core.Node{
		ID:   core.NodeID(p.nextID.Add(1)),
		Kind: "document",
		Span: core.Span{
			Start: lines.locate(0),
			End:   lines.locate(len(source)),
		},
	}

	stack := []*core.Node{root}

	for {
		startOff := int(dec.InputOffset())
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("xml parse failed: %w", err)
		}
		endOff := int(dec.InputOffset())
		parent := stack[len(stack)-1]

		switch t := tok.(type) {
		case xml.StartElement:
			el := &core.Node{
				ID:     core.NodeID(p.nextID.Add(1)),
				Kind:   elementKind(t.Name),
				Label:  identityLabel(t.Attr),
				Parent: parent,
				Span: core.Span{
					Start: lines.locate(startOff),
					// Adjusted to the end tag when it closes.
					End: lines.locate(endOff),
				},
			}
			parent.Children = append(parent.Children, el)

			attrs := make([]xml.Attr, len(t.Attr))
			copy(attrs, t.Attr)
			sort.Slice(attrs, func(i, j int) bool {
				return attrName(attrs[i].Name) < attrName(attrs[j].Name)
			})
			for _, a := range attrs {
				an := &core.Node{
					ID:     core.NodeID(p.nextID.Add(1)),
					Kind:   "attribute",
					Label:  attrName(a.Name),
					Value:  a.Value,
					Parent: el,
					// Token-level spans; encoding/xml does not expose
					// per-attribute positions.
					Span: core.Span{
						Start: lines.locate(startOff),
						End:   lines.locate(endOff),
					},
				}
				el.Children = append(el.Children, an)
			}

			stack = append(stack, el)

		case xml.EndElement:
			if len(stack) > 1 {
				closing := stack[len(stack)-1]
				closing.Span.End = lines.locate(endOff)
				stack = stack[:len(stack)-1]
			}

		case xml.CharData:
			text := strings.TrimSpace(string(t))
			if text == "" {
				continue
			}
			parent.Children = append(parent.Children, &core.Node{
				ID:     core.NodeID(p.nextID.Add(1)),
				Kind:   "text",
				Value:  text,
				Parent: parent,
				Span: core.Span{
					Start: lines.locate(startOff),
					End:   lines.locate(endOff),
				},
			})

		case xml.Comment:
			parent.Children = append(parent.Children, &core.Node{
				ID:     core.NodeID(p.nextID.Add(1)),
				Kind:   "comment",
				Value:  strings.TrimSpace(string(t)),
				Parent: parent,
				Span: core.Span{
					Start: lines.locate(startOff),
					End:   lines.locate(endOff),
				},
			})
		}
	}

	return core.NewTree(root, "xslt", source), nil
}

// elementKind renders an element name as the node kind. XSLT-namespace
// elements keep the conventional xsl: prefix regardless of the prefix
// the document declared; other elements use their local name.
func elementKind(name xml.Name) string {
	if name.Space == xslNamespace {
		return "xsl:" + name.Local
	}
	return name.Local
}

// identityLabel picks the attribute that names an element, in priority
// order. For xsl:template this is name or match; for plain elements a
// name or id attribute.
func identityLabel(attrs []xml.Attr) string {
	priority := []string{"name", "match", "id"}
	for _, want := range priority {
		for _, a := range attrs {
			if a.Name.Local == want {
				return a.Value
			}
		}
	}
	return ""
}

func attrName(name xml.Name) string {
	if name.Space == "" {
		return name.Local
	}
	if name.Space == "xmlns" {
		return "xmlns:" + name.Local
	}
	return name.Space + ":" + name.Local
}

// lineStarts records the byte offset of each line start, so byte
// offsets from the decoder convert to line/column locations.
type lineStarts []int

func lineIndex(source []byte) lineStarts {
	starts := lineStarts{0}
	for i, b := range source {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func (ls lineStarts) locate(offset int) core.Location {
	lo, hi := 0, len(ls)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if ls[mid] <= offset {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return core.Location{
		Line:   lo + 1,
		Column: offset - ls[lo],
		Offset: offset,
	}
}
