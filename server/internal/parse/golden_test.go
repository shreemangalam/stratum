package parse_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/shreemangalam/stratum/server/internal/core"
	"github.com/shreemangalam/stratum/server/internal/parse"
	"github.com/shreemangalam/stratum/server/internal/parse/treesitter"
	"github.com/shreemangalam/stratum/server/internal/parse/xslt"
)

var update = flag.Bool("update", false, "regenerate golden expected.json files")

// newParser returns a fresh parser instance so node IDs start from 1
// for every file and the golden output is reproducible.
func newParser(t *testing.T, lang string) parse.Parser {
	t.Helper()
	switch lang {
	case "go":
		return treesitter.NewGo()
	case "xslt":
		return xslt.New()
	case "javascript", "typescript", "python", "java", "c", "cpp":
		for _, sl := range treesitter.SupportedLanguages() {
			if sl.Lang == lang {
				return treesitter.NewGeneric(sl.Lang, sl.Exts)
			}
		}
		t.Fatalf("no supported language entry for %q", lang)
		return nil
	default:
		t.Fatalf("no parser for golden language %q", lang)
		return nil
	}
}

func findGoldenInput(t *testing.T, dir, stem string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, stem+".*"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no %s.* input in %s", stem, dir)
	}
	var picked []string
	for _, m := range matches {
		if filepath.Ext(m) != ".json" {
			picked = append(picked, m)
		}
	}
	if len(picked) != 1 {
		t.Fatalf("expected exactly one %s input in %s, got %v", stem, dir, picked)
	}
	return picked[0]
}

// TestGolden runs the full parse -> match -> edit script pipeline on
// each testdata/<language>/<case>/ directory and compares the JSON
// edit script against expected.json. Regenerate with:
//
//	go test ./internal/parse -run TestGolden -update
func TestGolden(t *testing.T) {
	langDirs, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("reading testdata: %v", err)
	}

	for _, langDir := range langDirs {
		if !langDir.IsDir() {
			continue
		}
		lang := langDir.Name()

		caseDirs, err := os.ReadDir(filepath.Join("testdata", lang))
		if err != nil {
			t.Fatal(err)
		}

		for _, caseDir := range caseDirs {
			if !caseDir.IsDir() {
				continue
			}
			name := lang + "/" + caseDir.Name()
			dir := filepath.Join("testdata", lang, caseDir.Name())

			t.Run(name, func(t *testing.T) {
				before, err := os.ReadFile(findGoldenInput(t, dir, "before"))
				if err != nil {
					t.Fatal(err)
				}
				after, err := os.ReadFile(findGoldenInput(t, dir, "after"))
				if err != nil {
					t.Fatal(err)
				}

				ctx := context.Background()
				leftTree, err := newParser(t, lang).Parse(ctx, normalize(before))
				if err != nil {
					t.Fatalf("parsing before: %v", err)
				}
				rightTree, err := newParser(t, lang).Parse(ctx, normalize(after))
				if err != nil {
					t.Fatalf("parsing after: %v", err)
				}

				m := core.Match(leftTree, rightTree, core.DefaultMatchConfig())
				es := core.GenerateEditScript(leftTree, rightTree, m)

				got, err := json.MarshalIndent(es, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				got = append(got, '\n')

				expectedPath := filepath.Join(dir, "expected.json")
				if *update {
					if err := os.WriteFile(expectedPath, got, 0o644); err != nil {
						t.Fatal(err)
					}
					t.Logf("regenerated %s", expectedPath)
					return
				}

				want, err := os.ReadFile(expectedPath)
				if err != nil {
					t.Fatalf("reading %s (run with -update to create): %v", expectedPath, err)
				}
				if !bytes.Equal(normalize(got), normalize(want)) {
					t.Errorf("edit script for %s differs from golden.\nRegenerate deliberately with -update.\ngot:\n%s",
						name, got)
				}
			})
		}
	}
}

// normalize strips carriage returns so goldens compare identically
// regardless of the checkout's line endings.
func normalize(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("\r"), nil)
}
