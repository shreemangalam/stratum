package cache

import (
	"testing"
	"time"

	"github.com/shreemangalam/stratum/server/internal/core"
)

func TestContentHash_Deterministic(t *testing.T) {
	h1 := ContentHash([]byte("hello"))
	h2 := ContentHash([]byte("hello"))
	if h1 != h2 {
		t.Error("same input should produce same hash")
	}

	h3 := ContentHash([]byte("world"))
	if h1 == h3 {
		t.Error("different input should produce different hash")
	}
}

func TestDiffKey_Canonical(t *testing.T) {
	k1 := DiffKey("aaa", "bbb", "go")
	k2 := DiffKey("bbb", "aaa", "go")
	if k1 != k2 {
		t.Error("diff key should be order-independent")
	}
	k3 := DiffKey("aaa", "bbb", "xslt")
	if k1 == k3 {
		t.Error("diff key should differ by language")
	}
}

func TestCache_ParseRoundTrip(t *testing.T) {
	c := New(time.Hour)

	tree := &core.Tree{Root: &core.Node{ID: 1, Kind: "root"}, Language: "go"}

	if got := c.GetParse("abc"); got != nil {
		t.Error("expected nil for cache miss")
	}

	c.PutParse("abc", tree)

	got := c.GetParse("abc")
	if got == nil {
		t.Fatal("expected cache hit")
	}
	if got.Language != "go" {
		t.Errorf("expected language 'go', got %q", got.Language)
	}
}

func TestCache_DiffRoundTrip(t *testing.T) {
	c := New(time.Hour)

	es := &core.EditScript{Operations: []core.Operation{{Kind: core.OpInsert}}}

	c.PutDiff("aaa", "bbb", "go", es)

	got := c.GetDiff("bbb", "aaa", "go")
	if got == nil {
		t.Fatal("expected cache hit (order-independent)")
	}
	if len(got.Operations) != 1 {
		t.Errorf("expected 1 operation, got %d", len(got.Operations))
	}
}

func TestCache_TTLExpiry(t *testing.T) {
	c := New(time.Millisecond)

	tree := &core.Tree{Root: &core.Node{ID: 1, Kind: "root"}}
	c.PutParse("abc", tree)

	time.Sleep(5 * time.Millisecond)

	if got := c.GetParse("abc"); got != nil {
		t.Error("expected nil for expired entry")
	}
}

func TestCache_Evict(t *testing.T) {
	c := New(time.Millisecond)

	c.PutParse("a", &core.Tree{Root: &core.Node{ID: 1, Kind: "root"}})
	c.PutDiff("x", "y", "go", &core.EditScript{})

	if c.Size() != 2 {
		t.Fatalf("expected 2 entries, got %d", c.Size())
	}

	time.Sleep(5 * time.Millisecond)
	removed := c.Evict()

	if removed != 2 {
		t.Errorf("expected 2 evicted, got %d", removed)
	}
	if c.Size() != 0 {
		t.Errorf("expected 0 entries after eviction, got %d", c.Size())
	}
}

func TestCache_EvictKeepsLive(t *testing.T) {
	c := New(time.Hour)

	c.PutParse("live", &core.Tree{Root: &core.Node{ID: 1, Kind: "root"}})
	c.PutDiff("a", "b", "go", &core.EditScript{})

	removed := c.Evict()
	if removed != 0 {
		t.Errorf("expected 0 evicted for live entries, got %d", removed)
	}
	if c.Size() != 2 {
		t.Errorf("expected 2 entries, got %d", c.Size())
	}
}
