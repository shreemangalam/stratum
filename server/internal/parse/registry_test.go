package parse

import (
	"context"
	"testing"

	"github.com/shreemangalam/stratum/server/internal/core"
)

type stubParser struct {
	lang string
	exts []string
}

func (s *stubParser) Parse(_ context.Context, _ []byte) (*core.Tree, error) {
	return nil, nil
}

func (s *stubParser) Language() string     { return s.lang }
func (s *stubParser) Extensions() []string { return s.exts }

func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubParser{lang: "go", exts: []string{".go"}})

	p, err := r.ForLanguage("go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Language() != "go" {
		t.Errorf("expected language 'go', got %q", p.Language())
	}

	p, err = r.ForExtension("go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Language() != "go" {
		t.Errorf("expected language 'go' from extension lookup, got %q", p.Language())
	}
}

func TestRegistry_UnknownLanguage(t *testing.T) {
	r := NewRegistry()
	_, err := r.ForLanguage("unknown")
	if err == nil {
		t.Error("expected error for unknown language")
	}
}

func TestRegistry_Languages(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubParser{lang: "go", exts: []string{".go"}})
	r.Register(&stubParser{lang: "xslt", exts: []string{".xsl", ".xslt"}})

	langs := r.Languages()
	if len(langs) != 2 {
		t.Fatalf("expected 2 languages, got %d", len(langs))
	}
}
