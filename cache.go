package main

import (
	"sync"
	"time"
)

const (
	defaultCacheTTL = 5 * time.Minute
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
		return nil, false
	}

	if time.Now().After(entry.ExpiresAt) {
		delete(c.cache, key)
		return nil, false
	}

	return entry.Data, true
}

func (c *Cache) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[key] = &CacheEntry{
		Data:      value,
		ExpiresAt: time.Now().Add(c.ttl),
	}
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*CacheEntry)
}

func (c *Cache) RefreshTTL(newTTL time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ttl = newTTL
}
