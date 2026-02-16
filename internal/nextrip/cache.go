package nextrip

import (
	"sync"
	"time"
)

// Cache is a simple in-memory TTL cache for NexTrip API responses.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	value     any
	expiresAt time.Time
}

// NewCache creates a cache with the given TTL.
func NewCache(ttl time.Duration) *Cache {
	c := &Cache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
	}
	// Background cleanup every 5 minutes
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			c.cleanup()
		}
	}()
	return c
}

// Get retrieves a cached value if it exists and hasn't expired.
func (c *Cache) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.value, true
}

// Set stores a value in the cache.
func (c *Cache) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *Cache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for k, v := range c.entries {
		if now.After(v.expiresAt) {
			delete(c.entries, k)
		}
	}
}
