package treesitter

import (
	"context"
	"os"
	"testing"

	"github.com/shreemangalam/stratum/server/internal/core"
)

func TestFullPipeline_GoMoveAndRename(t *testing.T) {
	before, err := os.ReadFile("testdata/before.go")
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile("testdata/after.go")
	if err != nil {
		t.Fatal(err)
	}

	p := NewGo()

	leftTree, err := p.Parse(context.Background(), before)
	if err != nil {
		t.Fatal(err)
	}

	p2 := NewGo()
	rightTree, err := p2.Parse(context.Background(), after)
	if err != nil {
		t.Fatal(err)
	}

	m := core.Match(leftTree, rightTree, core.DefaultMatchConfig())
	es := core.GenerateEditScript(leftTree, rightTree, m)

	if len(es.Operations) == 0 {
		t.Fatal("expected operations in edit script")
	}

	ops := make(map[core.OpKind]int)
	for _, op := range es.Operations {
		ops[op.Kind]++
	}

	t.Logf("Edit script: %d operations", len(es.Operations))
	for kind, count := range ops {
		t.Logf("  %s: %d", kind, count)
	}

	if ops[core.OpInsert] == 0 {
		t.Error("expected insert operations (multiply function was added)")
	}

	hasMove := ops[core.OpMove] > 0
	hasUpdate := ops[core.OpUpdate] > 0
	if !hasMove && !hasUpdate {
		t.Error("expected move or update operations (greet was moved and its return value changed)")
	}
}

func TestFullPipeline_IdenticalFiles(t *testing.T) {
	src, err := os.ReadFile("testdata/before.go")
	if err != nil {
		t.Fatal(err)
	}

	p1 := NewGo()
	p2 := NewGo()

	leftTree, err := p1.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	rightTree, err := p2.Parse(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	m := core.Match(leftTree, rightTree, core.DefaultMatchConfig())
	es := core.GenerateEditScript(leftTree, rightTree, m)

	for _, op := range es.Operations {
		if op.Kind == core.OpInsert || op.Kind == core.OpDelete {
			t.Errorf("identical files should not produce %s operations", op.Kind)
		}
	}
}

func TestFullPipeline_ClassifiesReorderedGoStatements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		left        string
		right       string
		wantVerdict core.Verdict
		wantReason  string
	}{
		{
			name: "independent statements preserve behavior",
			left: `package main
func f() int {
	x := 11
	y := 22
	return x + y
}`,
			right: `package main
func f() int {
	y := 22
	x := 11
	return x + y
}`,
			wantVerdict: core.VerdictPreserving,
			wantReason:  "independent statements reordered",
		},
		{
			name: "dependent statements change behavior",
			left: `package main
func f() int {
	x := 31
	y := x + 41
	return y
}`,
			right: `package main
func f() int {
	y := x + 41
	x := 31
	return y
}`,
			wantVerdict: core.VerdictChanging,
			wantReason:  "dependent statements reordered",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			leftParser := NewGo()
			rightParser := NewGo()
			leftTree, err := leftParser.Parse(context.Background(), []byte(tt.left))
			if err != nil {
				t.Fatalf("parse left source: %v", err)
			}
			rightTree, err := rightParser.Parse(context.Background(), []byte(tt.right))
			if err != nil {
				t.Fatalf("parse right source: %v", err)
			}

			matches := core.Match(leftTree, rightTree, core.DefaultMatchConfig())
			script := core.GenerateEditScript(leftTree, rightTree, matches)
			if len(script.Semantic) != 1 {
				t.Fatalf("semantic changes = %d, want 1", len(script.Semantic))
			}

			got := script.Semantic[0]
			if got.Verdict != tt.wantVerdict {
				t.Fatalf("verdict = %q, want %q (reason: %q)", got.Verdict, tt.wantVerdict, got.Reason)
			}
			if got.Reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", got.Reason, tt.wantReason)
			}
		})
	}
}
