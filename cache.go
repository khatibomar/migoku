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
	defer c.mu.RUnlock()

	entry, exists := c.cache[key]
	if !exists {
		slog.Default().Debug("Cache miss", "key", key)
		return nil, false
	}

	if time.Now().After(entry.ExpiresAt) {
		slog.Default().Debug("Cache expired", "key", key)
		delete(c.cache, key)
		return nil, false
	}

	slog.Default().Debug("Cache hit", "key", key)
	return entry.Data, true
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
