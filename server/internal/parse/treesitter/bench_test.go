package treesitter

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/shreemangalam/stratum/server/internal/core"
)

func BenchmarkParseGo_Small(b *testing.B) {
	src, err := os.ReadFile("testdata/before.go")
	if err != nil {
		b.Fatal(err)
	}
	p := NewGo()
	b.ReportMetric(float64(len(src)), "bytes")
	b.ResetTimer()
	for range b.N {
		if _, err := p.Parse(context.Background(), src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFullPipeline_Small(b *testing.B) {
	before, err := os.ReadFile("testdata/before.go")
	if err != nil {
		b.Fatal(err)
	}
	after, err := os.ReadFile("testdata/after.go")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportMetric(float64(len(before)+len(after)), "total_bytes")
	b.ResetTimer()
	for range b.N {
		p1 := NewGo()
		lt, err := p1.Parse(context.Background(), before)
		if err != nil {
			b.Fatal(err)
		}
		p2 := NewGo()
		rt, err := p2.Parse(context.Background(), after)
		if err != nil {
			b.Fatal(err)
		}
		m := core.Match(lt, rt, core.DefaultMatchConfig())
		core.GenerateEditScript(lt, rt, m)
	}
}

// generateGoSource creates a synthetic Go file with n functions.
func generateGoSource(numFuncs int) []byte {
	var b strings.Builder
	b.WriteString("package bench\n\n")
	for i := range numFuncs {
		b.WriteString("func fn")
		b.WriteString(strings.Repeat("_", i%5))
		b.WriteString("_")
		b.WriteString(string(rune('A' + i%26)))
		b.WriteString("(")
		for p := range i % 3 {
			if p > 0 {
				b.WriteString(", ")
			}
			b.WriteString("p")
			b.WriteString(string(rune('a' + p)))
			b.WriteString(" int")
		}
		b.WriteString(") int {\n")
		for s := range 3 + i%4 {
			b.WriteString("\tx := x + ")
			b.WriteString(string(rune('0' + s%10)))
			b.WriteString("\n")
		}
		b.WriteString("\treturn x\n}\n\n")
	}
	return []byte(b.String())
}

func generateModifiedGoSource(numFuncs int) []byte {
	var b strings.Builder
	b.WriteString("package bench\n\n")
	for i := range numFuncs {
		idx := i
		// Swap first two functions (move detection).
		if i == 0 {
			idx = 1
		} else if i == 1 {
			idx = 0
		}

		label := "fn"
		// Rename the third function.
		if idx == 2 {
			label = "renamed_fn"
		}

		b.WriteString("func ")
		b.WriteString(label)
		b.WriteString(strings.Repeat("_", idx%5))
		b.WriteString("_")
		b.WriteString(string(rune('A' + idx%26)))
		b.WriteString("(")
		for p := range idx % 3 {
			if p > 0 {
				b.WriteString(", ")
			}
			b.WriteString("p")
			b.WriteString(string(rune('a' + p)))
			b.WriteString(" int")
		}
		b.WriteString(") int {\n")

		bodyLines := 3 + idx%4
		// Edit the body of the fourth function.
		if idx == 3 {
			bodyLines += 2
		}
		for s := range bodyLines {
			b.WriteString("\tx := x + ")
			b.WriteString(string(rune('0' + s%10)))
			b.WriteString("\n")
		}
		b.WriteString("\treturn x\n}\n\n")
	}

	// Insert a new function at the end.
	b.WriteString("func newlyAdded() int {\n\treturn 42\n}\n")
	return []byte(b.String())
}

func benchmarkPipeline(b *testing.B, numFuncs int) {
	before := generateGoSource(numFuncs)
	after := generateModifiedGoSource(numFuncs)

	b.ReportMetric(float64(len(before)), "left_bytes")
	b.ReportMetric(float64(len(after)), "right_bytes")
	b.ResetTimer()

	for range b.N {
		p1 := NewGo()
		lt, err := p1.Parse(context.Background(), before)
		if err != nil {
			b.Fatal(err)
		}
		p2 := NewGo()
		rt, err := p2.Parse(context.Background(), after)
		if err != nil {
			b.Fatal(err)
		}
		m := core.Match(lt, rt, core.DefaultMatchConfig())
		es := core.GenerateEditScript(lt, rt, m)
		_ = es
	}
}

func BenchmarkPipeline_5funcs(b *testing.B)   { benchmarkPipeline(b, 5) }
func BenchmarkPipeline_20funcs(b *testing.B)  { benchmarkPipeline(b, 20) }
func BenchmarkPipeline_50funcs(b *testing.B)  { benchmarkPipeline(b, 50) }
func BenchmarkPipeline_100funcs(b *testing.B) { benchmarkPipeline(b, 100) }
