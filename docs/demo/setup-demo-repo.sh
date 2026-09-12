#!/usr/bin/env bash
# Creates a temporary git repository with known commits for reproducible
# Stratum demos.  The resulting repo has two commits; diffing HEAD~1..HEAD
# shows a function move between files, a renamed function, and an edited
# function body — exercising diff, merge, and cross-file detection.
#
# Usage:
#   bash docs/demo/setup-demo-repo.sh [target-dir]
#
# If target-dir is omitted a temporary directory is created and printed.

set -euo pipefail

TARGET="${1:-$(mktemp -d)}"
mkdir -p "$TARGET"
cd "$TARGET"

git init -b main
git config user.name  "Demo User"
git config user.email "demo@example.com"

# ── Commit 1: initial state ──────────────────────────────────────────

mkdir -p pkg

cat > pkg/math.go << 'GOEOF'
package pkg

// Add returns the sum of two integers.
func Add(a, b int) int {
	return a + b
}

// Multiply returns the product of two integers.
func Multiply(a, b int) int {
	result := 0
	for i := 0; i < b; i++ {
		result += a
	}
	return result
}

// Abs returns the absolute value of n.
func Abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
GOEOF

cat > pkg/strings.go << 'GOEOF'
package pkg

import "strings"

// Reverse returns s reversed.
func Reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// Contains checks whether s contains substr.
func Contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
GOEOF

git add -A
git commit -m "Initial commit: math and string utilities"

# ── Commit 2: refactor ──────────────────────────────────────────────
#
# Changes:
#   1. Multiply is rewritten to use the * operator (body edit).
#   2. Abs is moved from math.go to strings.go (cross-file move).
#   3. Contains is renamed to HasSubstring (rename).

cat > pkg/math.go << 'GOEOF'
package pkg

// Add returns the sum of two integers.
func Add(a, b int) int {
	return a + b
}

// Multiply returns the product of two integers.
func Multiply(a, b int) int {
	return a * b
}
GOEOF

cat > pkg/strings.go << 'GOEOF'
package pkg

import "strings"

// Reverse returns s reversed.
func Reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// HasSubstring checks whether s contains substr.
func HasSubstring(s, substr string) bool {
	return strings.Contains(s, substr)
}

// Abs returns the absolute value of n.
func Abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
GOEOF

git add -A
git commit -m "Refactor: simplify Multiply, move Abs, rename Contains"

echo ""
echo "Demo repo ready at: $TARGET"
echo "  git log --oneline    → two commits"
echo "  Use HEAD~1 and HEAD as left/right refs in Stratum's git diff UI."
