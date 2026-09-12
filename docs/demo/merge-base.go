package cache

import "sync"

// Cache is a simple thread-safe key-value store.
type Cache struct {
	mu    sync.RWMutex
	items map[string]string
}

// New creates an empty cache.
func New() *Cache {
	return &Cache{items: make(map[string]string)}
}

// Get retrieves a value by key.
func (c *Cache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.items[key]
	return val, ok
}

// Set stores a key-value pair.
func (c *Cache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = value
}

// Delete removes a key from the cache.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}
