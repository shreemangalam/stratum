package parse

import (
	"fmt"
	"strings"
)

// Registry maps file extensions and language identifiers to parsers.
type Registry struct {
	byLanguage  map[string]Parser
	byExtension map[string]Parser
}

// NewRegistry creates an empty parser registry.
func NewRegistry() *Registry {
	return &Registry{
		byLanguage:  make(map[string]Parser),
		byExtension: make(map[string]Parser),
	}
}

// Register adds a parser to the registry.
func (r *Registry) Register(p Parser) {
	r.byLanguage[p.Language()] = p
	for _, ext := range p.Extensions() {
		ext = strings.TrimPrefix(ext, ".")
		r.byExtension[ext] = p
	}
}

// ForLanguage returns the parser for a language identifier.
func (r *Registry) ForLanguage(lang string) (Parser, error) {
	p, ok := r.byLanguage[lang]
	if !ok {
		return nil, fmt.Errorf("no parser registered for language %q", lang)
	}
	return p, nil
}

// ForExtension returns the parser for a file extension.
func (r *Registry) ForExtension(ext string) (Parser, error) {
	ext = strings.TrimPrefix(ext, ".")
	p, ok := r.byExtension[ext]
	if !ok {
		return nil, fmt.Errorf("no parser registered for extension %q", ext)
	}
	return p, nil
}

// Languages returns all registered language identifiers.
func (r *Registry) Languages() []string {
	langs := make([]string, 0, len(r.byLanguage))
	for l := range r.byLanguage {
		langs = append(langs, l)
	}
	return langs
}
