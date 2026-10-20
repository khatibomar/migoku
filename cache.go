package main

import (
	"log/slog"
	"sync"
	"time"
)

const (
	defaultCacheTTL = 10 * time.Second
)

// CacheEntry stores cached data with expiration
type CacheEntry struct {
	Data      any
	ExpiresAt time.Time
}

// Cache manages in-memory caching
type Cache struct {
	mu    sync.RWMutex
	cache map[string]*CacheEntry
	ttl   time.Duration
}

func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		cache: make(map[string]*CacheEntry),
		ttl:   ttl,
	}
}

func (c *Cache) Get(key string) (any, bool) {
	c.mu.RLock()
	entry, exists := c.cache[key]
	expired := exists && time.Now().After(entry.ExpiresAt)
	c.mu.RUnlock()

	if !exists {
		slog.Default().Debug("Cache miss", "key", key)
		return nil, false
	}

	if !expired {
		slog.Default().Debug("Cache hit", "key", key)
		return entry.Data, true
	}

	// The entry expired. Evicting it is a map write, so it needs the write
	// lock; doing it under the read lock would let concurrent Get calls
	// write the map at the same time and crash the process.
	c.mu.Lock()
	defer c.mu.Unlock()

	// Re-check under the write lock: a concurrent Set may have refreshed the
	// entry between the read lock and the write lock, and that fresh entry
	// must not be deleted.
	if current, ok := c.cache[key]; ok && time.Now().After(current.ExpiresAt) {
		delete(c.cache, key)
	}

	slog.Default().Debug("Cache expired", "key", key)
	return nil, false
}

func (c *Cache) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[key] = &CacheEntry{
		Data:      value,
		ExpiresAt: time.Now().Add(c.ttl),
	}
	slog.Default().Debug("Cache set", "key", key, "ttl", c.ttl.String())
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*CacheEntry)
	slog.Default().Debug("Cache cleared")
}

func (c *Cache) RefreshTTL(newTTL time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ttl = newTTL
	slog.Default().Debug("Cache TTL updated", "ttl", newTTL.String())
}

// TTL returns the currently configured entry lifetime.
func (c *Cache) TTL() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.ttl
}
