package xslt

import (
	"context"
	"testing"

	"github.com/shreemangalam/stratum/server/internal/core"
)

const sampleXSLT = `<?xml version="1.0"?>
<xsl:stylesheet xmlns:xsl="http://www.w3.org/1999/XSL/Transform" version="1.0">
  <xsl:template match="/order">
    <invoice>
      <xsl:value-of select="total"/>
    </invoice>
  </xsl:template>
  <xsl:template name="lineItem">
    <item>fixed</item>
  </xsl:template>
</xsl:stylesheet>`

func findByKind(n *core.Node, kind string) []*core.Node {
	var out []*core.Node
	if n.Kind == kind {
		out = append(out, n)
	}
	for _, c := range n.Children {
		out = append(out, findByKind(c, kind)...)
	}
	return out
}

func TestParse_XSLTStructure(t *testing.T) {
	tree, err := New().Parse(context.Background(), []byte(sampleXSLT))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	templates := findByKind(tree.Root, "xsl:template")
	if len(templates) != 2 {
		t.Fatalf("expected 2 xsl:template nodes, got %d", len(templates))
	}
	first := templates[0]
	second := templates[1]
	if first == nil || first.Label != "/order" {
		t.Errorf("match template should carry its match attr as label, got %q", first.Label)
	}
	if second == nil || second.Label != "lineItem" {
		t.Errorf("named template should carry its name attr as label, got %q", second.Label)
	}

	if got := findByKind(tree.Root, "xsl:value-of"); len(got) != 1 {
		t.Errorf("expected 1 xsl:value-of, got %d", len(got))
	}
	if got := findByKind(tree.Root, "invoice"); len(got) != 1 {
		t.Errorf("expected 1 invoice element, got %d", len(got))
	}

	texts := findByKind(tree.Root, "text")
	if len(texts) != 1 || texts[0] == nil || texts[0].Value != "fixed" {
		t.Errorf("expected one text node %q, got %+v", "fixed", texts)
	}
}

func TestParse_AttributeOrderIrrelevant(t *testing.T) {
	a := `<root><item name="x" type="big" mode="fast"/></root>`
	b := `<root><item mode="fast" type="big" name="x"/></root>`

	ta, err := New().Parse(context.Background(), []byte(a))
	if err != nil {
		t.Fatal(err)
	}
	tb, err := New().Parse(context.Background(), []byte(b))
	if err != nil {
		t.Fatal(err)
	}

	core.HashTree(ta)
	core.HashTree(tb)
	if ta.Root.Hash != tb.Root.Hash {
		t.Error("attribute reordering must not change the tree hash")
	}
}

func TestParse_AttributeValueChange_Detected(t *testing.T) {
	a := `<root><item select="old/path"/></root>`
	b := `<root><item select="new/path"/></root>`

	ta, err := New().Parse(context.Background(), []byte(a))
	if err != nil {
		t.Fatal(err)
	}
	tb, err := New().Parse(context.Background(), []byte(b))
	if err != nil {
		t.Fatal(err)
	}

	m := core.Match(ta, tb, core.DefaultMatchConfig())
	es := core.GenerateEditScript(ta, tb, m)

	updates := 0
	for _, op := range es.Operations {
		if op.Kind == core.OpUpdate && op.LeftNode != nil && op.LeftNode.Kind == "attribute" {
			updates++
		}
	}
	if updates != 1 {
		t.Errorf("expected exactly 1 attribute update, got %d (ops: %+v)", updates, es.Operations)
	}
}

func TestParse_MalformedXML_Errors(t *testing.T) {
	if _, err := New().Parse(context.Background(), []byte(`<a><b></a>`)); err == nil {
		t.Error("mismatched tags should fail to parse")
	}
}

func TestParse_Spans(t *testing.T) {
	tree, err := New().Parse(context.Background(), []byte(sampleXSLT))
	if err != nil {
		t.Fatal(err)
	}

	templates := findByKind(tree.Root, "xsl:template")
	first := templates[0]
	if first.Span.Start.Line != 3 {
		t.Errorf("first template should start on line 3, got %d", first.Span.Start.Line)
	}
	if first.Span.End.Line != 7 {
		t.Errorf("first template should end on line 7, got %d", first.Span.End.Line)
	}
}
