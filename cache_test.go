package main

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestCacheConcurrentExpiredEviction is a regression test for the
// "fatal error: concurrent map writes" crash: Cache.Get used to evict
// expired entries while holding only the read lock, so two concurrent
// Get calls wrote the map at the same time.
func TestCacheConcurrentExpiredEviction(t *testing.T) {
	// A TTL of 1ns makes every entry expired, so every Get takes the
	// eviction path and the map is written from many goroutines at once.
	c := NewCache(time.Nanosecond)

	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for j := range 1000 {
				key := "key-" + strconv.Itoa(j%4)
				c.Set(key, j)
				c.Get(key)
			}
		})
	}
	wg.Wait()

	if _, ok := c.Get("key-0"); ok {
		t.Error("expected expired entry to be reported as a miss")
	}
}

// TestCacheConcurrentGetSet exercises the hit path concurrently with writes.
func TestCacheConcurrentGetSet(t *testing.T) {
	c := NewCache(time.Minute)

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for j := range 200 {
				key := "key-" + strconv.Itoa(j%8)
				c.Set(key, j)
				if _, ok := c.Get(key); !ok {
					t.Errorf("expected cache hit for key %q", key)
					return
				}
			}
		})
	}
	wg.Wait()
}

func TestCacheGetExpired(t *testing.T) {
	c := NewCache(time.Millisecond)
	c.Set("key", "value")

	if _, ok := c.Get("key"); !ok {
		t.Fatal("expected cache hit before expiry")
	}

	time.Sleep(5 * time.Millisecond)

	if _, ok := c.Get("key"); ok {
		t.Fatal("expected cache miss after expiry")
	}

	c.mu.RLock()
	_, stillThere := c.cache["key"]
	c.mu.RUnlock()
	if stillThere {
		t.Error("expired entry was not evicted")
	}
}

func TestCacheGetMiss(t *testing.T) {
	c := NewCache(time.Minute)

	if _, ok := c.Get("missing"); ok {
		t.Fatal("expected cache miss for unknown key")
	}
}

func TestCacheSetGet(t *testing.T) {
	c := NewCache(time.Minute)
	c.Set("key", 42)

	value, ok := c.Get("key")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if value != 42 {
		t.Fatalf("expected 42, got %v", value)
	}
}

func TestCacheClear(t *testing.T) {
	c := NewCache(time.Minute)
	c.Set("key", "value")
	c.Clear()

	if _, ok := c.Get("key"); ok {
		t.Fatal("expected cache miss after clear")
	}
}

func TestCacheTTL(t *testing.T) {
	c := NewCache(time.Minute)
	if c.TTL() != time.Minute {
		t.Fatalf("expected 1m, got %v", c.TTL())
	}

	c.RefreshTTL(30 * time.Second)
	if c.TTL() != 30*time.Second {
		t.Fatalf("expected 30s, got %v", c.TTL())
	}
}
