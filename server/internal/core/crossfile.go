package core

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
)

// CrossFileKind classifies a cross-file relationship.
type CrossFileKind string

// Cross-file relationship kinds.
const (
	CrossFileMove       CrossFileKind = "move"
	CrossFileRenameMove CrossFileKind = "rename-move"
)

// FileDiff carries one file's diff result with enough context for
// cross-file analysis. Trees and sources may be nil for pure
// additions or deletions.
type FileDiff struct {
	Path        string
	LeftSource  []byte
	RightSource []byte
	LeftTree    *Tree
	RightTree   *Tree
	Script      *EditScript
}

// CrossFileMatch links a deleted node in one file to an inserted node
// in another, indicating a cross-file move or rename-move.
type CrossFileMatch struct {
	Kind       CrossFileKind `json:"kind"`
	Score      float64       `json:"score"`
	SourceFile string        `json:"source_file"`
	TargetFile string        `json:"target_file"`
	SourceNode NodeRef       `json:"source_node"`
	TargetNode NodeRef       `json:"target_node"`
}

// CrossFileResult holds all detected cross-file relationships.
type CrossFileResult struct {
	Matches []CrossFileMatch `json:"matches"`
}

type candidate struct {
	file    string
	node    *Node
	ref     NodeRef
	content string
	hash    string
}

// DetectCrossFileChanges correlates deleted nodes in one file with
// inserted nodes in another to detect cross-file moves and renames.
// It runs three phases: exact hash match, label+similarity match,
// and kind+similarity match (rename-move).
func DetectCrossFileChanges(diffs []FileDiff) *CrossFileResult {
	if len(diffs) < 2 {
		return &CrossFileResult{Matches: []CrossFileMatch{}}
	}

	sources, targets := collectCandidates(diffs)
	if len(sources) == 0 || len(targets) == 0 {
		return &CrossFileResult{Matches: []CrossFileMatch{}}
	}

	var matches []CrossFileMatch
	usedSources := make(map[int]bool)
	usedTargets := make(map[int]bool)

	// Phase 1: exact content match across different files.
	hashToSources := make(map[string][]int)
	hashToTargets := make(map[string][]int)
	for i := range sources {
		hashToSources[sources[i].hash] = append(hashToSources[sources[i].hash], i)
	}
	for i := range targets {
		hashToTargets[targets[i].hash] = append(hashToTargets[targets[i].hash], i)
	}
	for hash, srcIdxs := range hashToSources {
		tgtIdxs, ok := hashToTargets[hash]
		if !ok {
			continue
		}
		for _, si := range srcIdxs {
			if usedSources[si] {
				continue
			}
			for _, ti := range tgtIdxs {
				if usedTargets[ti] {
					continue
				}
				if sources[si].file == targets[ti].file {
					continue
				}
				matches = append(matches, CrossFileMatch{
					Kind:       CrossFileMove,
					Score:      1.0,
					SourceFile: sources[si].file,
					TargetFile: targets[ti].file,
					SourceNode: sources[si].ref,
					TargetNode: targets[ti].ref,
				})
				usedSources[si] = true
				usedTargets[ti] = true
				break
			}
		}
	}

	// Phase 2: same kind+label, different file, similarity >= 0.6.
	type kindLabel struct {
		kind, label string
	}
	klSources := make(map[kindLabel][]int)
	klTargets := make(map[kindLabel][]int)
	for i := range sources {
		if usedSources[i] || sources[i].ref.Label == "" {
			continue
		}
		kl := kindLabel{sources[i].ref.Kind, sources[i].ref.Label}
		klSources[kl] = append(klSources[kl], i)
	}
	for i := range targets {
		if usedTargets[i] || targets[i].ref.Label == "" {
			continue
		}
		kl := kindLabel{targets[i].ref.Kind, targets[i].ref.Label}
		klTargets[kl] = append(klTargets[kl], i)
	}
	for kl, srcIdxs := range klSources {
		tgtIdxs, ok := klTargets[kl]
		if !ok {
			continue
		}
		for _, si := range srcIdxs {
			if usedSources[si] {
				continue
			}
			bestIdx := -1
			bestScore := 0.0
			for _, ti := range tgtIdxs {
				if usedTargets[ti] || sources[si].file == targets[ti].file {
					continue
				}
				score := jaccardLines(sources[si].content, targets[ti].content)
				if score >= 0.6 && score > bestScore {
					bestScore = score
					bestIdx = ti
				}
			}
			if bestIdx >= 0 {
				matches = append(matches, CrossFileMatch{
					Kind:       CrossFileMove,
					Score:      bestScore,
					SourceFile: sources[si].file,
					TargetFile: targets[bestIdx].file,
					SourceNode: sources[si].ref,
					TargetNode: targets[bestIdx].ref,
				})
				usedSources[si] = true
				usedTargets[bestIdx] = true
			}
		}
	}

	// Phase 3: same kind, different label, different file, similarity >= 0.75 → rename-move.
	for si := range sources {
		if usedSources[si] {
			continue
		}
		bestIdx := -1
		bestScore := 0.0
		for ti := range targets {
			if usedTargets[ti] || sources[si].file == targets[ti].file {
				continue
			}
			if sources[si].ref.Kind != targets[ti].ref.Kind {
				continue
			}
			score := jaccardLines(sources[si].content, targets[ti].content)
			if score >= 0.75 && score > bestScore {
				bestScore = score
				bestIdx = ti
			}
		}
		if bestIdx >= 0 {
			matches = append(matches, CrossFileMatch{
				Kind:       CrossFileRenameMove,
				Score:      bestScore,
				SourceFile: sources[si].file,
				TargetFile: targets[bestIdx].file,
				SourceNode: sources[si].ref,
				TargetNode: targets[bestIdx].ref,
			})
			usedSources[si] = true
			usedTargets[bestIdx] = true
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	return &CrossFileResult{Matches: matches}
}

func collectCandidates(diffs []FileDiff) (sources, targets []candidate) {
	for _, d := range diffs {
		if d.Script == nil {
			continue
		}
		for _, op := range d.Script.Operations {
			switch op.Kind {
			case OpDelete:
				if op.LeftNode == nil || d.LeftTree == nil {
					continue
				}
				node := d.LeftTree.NodeMap[op.LeftNode.ID]
				if node == nil || !isRootChild(node) {
					continue
				}
				content := extractNodeContent(d.LeftSource, node)
				if len(strings.TrimSpace(content)) < 10 {
					continue
				}
				sources = append(sources, candidate{
					file:    d.Path,
					node:    node,
					ref:     *op.LeftNode,
					content: content,
					hash:    contentHash(content),
				})
			case OpInsert:
				if op.RightNode == nil || d.RightTree == nil {
					continue
				}
				node := d.RightTree.NodeMap[op.RightNode.ID]
				if node == nil || !isRootChild(node) {
					continue
				}
				content := extractNodeContent(d.RightSource, node)
				if len(strings.TrimSpace(content)) < 10 {
					continue
				}
				targets = append(targets, candidate{
					file:    d.Path,
					node:    node,
					ref:     *op.RightNode,
					content: content,
					hash:    contentHash(content),
				})
			}
		}
	}
	return
}

func isRootChild(n *Node) bool {
	return n.Parent != nil && n.Parent.Parent == nil
}

func extractNodeContent(source []byte, n *Node) string {
	if source == nil || n.Span.End.Offset <= n.Span.Start.Offset {
		return ""
	}
	end := n.Span.End.Offset
	if end > len(source) {
		end = len(source)
	}
	start := n.Span.Start.Offset
	if start > len(source) {
		return ""
	}
	return string(source[start:end])
}

func contentHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h)
}

func jaccardLines(a, b string) float64 {
	setA := lineSet(a)
	setB := lineSet(b)
	if len(setA) == 0 && len(setB) == 0 {
		return 1.0
	}

	intersection := 0
	for line := range setA {
		if setB[line] {
			intersection++
		}
	}

	union := len(setA)
	for line := range setB {
		if !setA[line] {
			union++
		}
	}

	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func lineSet(s string) map[string]bool {
	set := make(map[string]bool)
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			set[trimmed] = true
		}
	}
	return set
}
