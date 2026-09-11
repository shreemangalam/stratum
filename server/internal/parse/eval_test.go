package parse_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shreemangalam/stratum/server/internal/core"
)

type evalResult struct {
	Language      string         `json:"language"`
	Case          string         `json:"case"`
	LeftNodes     int            `json:"left_nodes"`
	RightNodes    int            `json:"right_nodes"`
	Matched       int            `json:"matched_pairs"`
	MatchCoverage float64        `json:"match_coverage"`
	Ops           map[string]int `json:"operations"`
	TotalOps      int            `json:"total_ops"`
	Semantic      int            `json:"semantic_verdicts"`
}

type evalSummary struct {
	Timestamp  string       `json:"timestamp"`
	TotalCases int          `json:"total_cases"`
	Languages  []string     `json:"languages"`
	Results    []evalResult `json:"results"`
	Aggregates aggMetrics   `json:"aggregates"`
}

type aggMetrics struct {
	AvgMatchCoverage float64        `json:"avg_match_coverage"`
	OpDistribution   map[string]int `json:"op_distribution"`
	TotalNodes       int            `json:"total_nodes_processed"`
}

func TestEvalCorpus(t *testing.T) {
	if os.Getenv("STRATUM_EVAL") == "" {
		t.Skip("set STRATUM_EVAL=1 to run evaluation corpus")
	}

	langDirs, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}

	var results []evalResult
	langSet := map[string]bool{}

	for _, langDir := range langDirs {
		if !langDir.IsDir() {
			continue
		}
		lang := langDir.Name()
		langSet[lang] = true

		caseDirs, err := os.ReadDir(filepath.Join("testdata", lang))
		if err != nil {
			t.Fatal(err)
		}

		for _, caseDir := range caseDirs {
			if !caseDir.IsDir() {
				continue
			}
			caseName := caseDir.Name()
			dir := filepath.Join("testdata", lang, caseName)

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
				t.Fatalf("%s/%s: parse before: %v", lang, caseName, err)
			}
			rightTree, err := newParser(t, lang).Parse(ctx, normalize(after))
			if err != nil {
				t.Fatalf("%s/%s: parse after: %v", lang, caseName, err)
			}

			cfg := core.DefaultMatchConfig()
			m := core.Match(leftTree, rightTree, cfg)
			es := core.GenerateEditScript(leftTree, rightTree, m)

			ops := map[string]int{}
			for _, op := range es.Operations {
				ops[string(op.Kind)]++
			}

			matchCoverage := 0.0
			totalNodes := leftTree.Size() + rightTree.Size()
			matchedPairs := len(m.LeftToRight)
			if totalNodes > 0 {
				matchCoverage = float64(matchedPairs*2) / float64(totalNodes)
			}

			results = append(results, evalResult{
				Language:      lang,
				Case:          caseName,
				LeftNodes:     leftTree.Size(),
				RightNodes:    rightTree.Size(),
				Matched:       matchedPairs,
				MatchCoverage: matchCoverage,
				Ops:           ops,
				TotalOps:      len(es.Operations),
				Semantic:      len(es.Semantic),
			})
		}
	}

	langs := make([]string, 0, len(langSet))
	for l := range langSet {
		langs = append(langs, l)
	}

	agg := computeAggregates(results)

	summary := evalSummary{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		TotalCases: len(results),
		Languages:  langs,
		Results:    results,
		Aggregates: agg,
	}

	out, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join("..", "..", "..", "docs", "private", "interview-prep", "eval-corpus-results.json")
	if err := os.WriteFile(outPath, out, 0o644); err != nil {
		t.Logf("could not write results file: %v", err)
	}

	t.Logf("Evaluation complete: %d cases across %d languages", len(results), len(langs))
	t.Logf("Average match coverage: %.1f%%", agg.AvgMatchCoverage*100)
	t.Logf("Op distribution: %v", formatOps(agg.OpDistribution))
	t.Logf("Total nodes processed: %d", agg.TotalNodes)

	for _, r := range results {
		t.Logf("  %s/%s: %d+%d nodes, %d matched (%.0f%%), %d ops",
			r.Language, r.Case, r.LeftNodes, r.RightNodes, r.Matched,
			r.MatchCoverage*100, r.TotalOps)
	}
}

func computeAggregates(results []evalResult) aggMetrics {
	if len(results) == 0 {
		return aggMetrics{OpDistribution: map[string]int{}}
	}

	var totalMatchCoverage float64
	var totalNodes int
	opDist := map[string]int{}
	for _, r := range results {
		totalMatchCoverage += r.MatchCoverage
		totalNodes += r.LeftNodes + r.RightNodes
		for op, count := range r.Ops {
			opDist[op] += count
		}
	}

	return aggMetrics{
		AvgMatchCoverage: totalMatchCoverage / float64(len(results)),
		OpDistribution:   opDist,
		TotalNodes:       totalNodes,
	}
}

func formatOps(ops map[string]int) string {
	parts := make([]string, 0, len(ops))
	for k, v := range ops {
		parts = append(parts, fmt.Sprintf("%s=%d", k, v))
	}
	return strings.Join(parts, ", ")
}
