package cache

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/shreemangalam/stratum/server/internal/core"
)

// ContentHash returns the SHA-256 hex digest of source bytes.
func ContentHash(source []byte) string {
	h := sha256.Sum256(source)
	return fmt.Sprintf("%x", h)
}

// DiffKey returns a canonical cache key for a pair of source hashes and language.
func DiffKey(leftHash, rightHash, language string) string {
	if leftHash > rightHash {
		leftHash, rightHash = rightHash, leftHash
	}
	return leftHash + ":" + rightHash + ":" + language
}

// ParseKey returns a cache key for a parsed tree, qualified by language
// since the same content produces different trees under different parsers.
func ParseKey(contentHash, language string) string {
	return contentHash + ":" + language
}

type parseEntry struct {
	tree      *core.Tree
	expiresAt time.Time
}

type diffEntry struct {
	script    *core.EditScript
	expiresAt time.Time
}

// Cache is a TTL-based in-memory cache for parse and diff results.
type Cache struct {
	mu       sync.RWMutex
	ttl      time.Duration
	parses   map[string]parseEntry
	diffs    map[string]diffEntry
}

// New creates a cache with the given TTL.
func New(ttl time.Duration) *Cache {
	return &Cache{
		ttl:    ttl,
		parses: make(map[string]parseEntry),
		diffs:  make(map[string]diffEntry),
	}
}

// GetParse returns a cached parse result, or nil if not found or expired.
func (c *Cache) GetParse(hash string) *core.Tree {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.parses[hash]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil
	}
	return entry.tree
}

// PutParse stores a parse result.
func (c *Cache) PutParse(hash string, tree *core.Tree) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.parses[hash] = parseEntry{
		tree:      tree,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// GetDiff returns a cached diff result, or nil if not found or expired.
func (c *Cache) GetDiff(leftHash, rightHash, language string) *core.EditScript {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := DiffKey(leftHash, rightHash, language)
	entry, ok := c.diffs[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil
	}
	return entry.script
}

// PutDiff stores a diff result.
func (c *Cache) PutDiff(leftHash, rightHash, language string, script *core.EditScript) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := DiffKey(leftHash, rightHash, language)
	c.diffs[key] = diffEntry{
		script:    script,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Size returns the total number of cached entries.
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.parses) + len(c.diffs)
}

// Evict removes all expired entries and returns the count removed.
func (c *Cache) Evict() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	removed := 0

	for k, e := range c.parses {
		if now.After(e.expiresAt) {
			delete(c.parses, k)
			removed++
		}
	}
	for k, e := range c.diffs {
		if now.After(e.expiresAt) {
			delete(c.diffs, k)
			removed++
		}
	}
	return removed
}

// StartEviction runs periodic eviction in a background goroutine.
// It stops when the context is cancelled.
func (c *Cache) StartEviction(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n := c.Evict(); n > 0 {
					slog.Info("cache eviction", "removed", n, "remaining", c.Size())
				}
			}
		}
	}()
}
