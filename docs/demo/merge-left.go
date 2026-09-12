package cache

import (
	"sync"
	"time"
)

// Cache is a simple thread-safe key-value store with TTL support.
type Cache struct {
	mu    sync.RWMutex
	items map[string]entry
}

type entry struct {
	value     string
	expiresAt time.Time
}

// New creates an empty cache.
func New() *Cache {
	return &Cache{items: make(map[string]entry)}
}

// Get retrieves a value by key, returning false if expired.
func (c *Cache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.items[key]
	if !ok || time.Now().After(e.expiresAt) {
		return "", false
	}
	return e.value, true
}

// Set stores a key-value pair with a time-to-live.
func (c *Cache) Set(key, value string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = entry{value: value, expiresAt: time.Now().Add(ttl)}
}

// Delete removes a key from the cache.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}
