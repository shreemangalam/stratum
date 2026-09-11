package xslt

import (
	"context"
	"testing"
)

func FuzzParseXSLT(f *testing.F) {
	f.Add([]byte(`<?xml version="1.0"?>
<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/">
    <html><body><xsl:apply-templates/></body></html>
  </xsl:template>
</xsl:stylesheet>`))
	f.Add([]byte(`<root><child attr="val">text</child></root>`))
	f.Add([]byte(``))
	f.Add([]byte(`not xml at all`))
	f.Add([]byte(`<a><b><c/></b></a>`))
	f.Add([]byte(`<?xml version="1.0"?><empty/>`))

	f.Fuzz(func(t *testing.T, src []byte) {
		p := New()
		tree, err := p.Parse(context.Background(), src)
		if err != nil {
			return
		}
		if tree == nil {
			t.Fatal("Parse returned nil tree without error")
		}
		if tree.Root == nil {
			t.Fatal("Parse returned tree with nil root")
		}
	})
}
