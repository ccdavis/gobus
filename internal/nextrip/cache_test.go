package nextrip

import (
	"sync"
	"testing"
	"time"
)

func TestCache_SetGet(t *testing.T) {
	c := &Cache{
		entries: make(map[string]cacheEntry),
		ttl:     1 * time.Minute,
	}

	c.Set("key1", "value1")
	got, ok := c.Get("key1")
	if !ok {
		t.Fatal("Get('key1') should return true")
	}
	if got != "value1" {
		t.Errorf("Get('key1') = %v, want 'value1'", got)
	}
}

func TestCache_Miss(t *testing.T) {
	c := &Cache{
		entries: make(map[string]cacheEntry),
		ttl:     1 * time.Minute,
	}

	_, ok := c.Get("missing")
	if ok {
		t.Error("Get('missing') should return false")
	}
}

func TestCache_Expiry(t *testing.T) {
	c := &Cache{
		entries: make(map[string]cacheEntry),
		ttl:     50 * time.Millisecond,
	}

	c.Set("key", "value")

	// Should be present immediately
	if _, ok := c.Get("key"); !ok {
		t.Fatal("key should be present immediately after Set")
	}

	// Wait for expiry
	time.Sleep(60 * time.Millisecond)

	if _, ok := c.Get("key"); ok {
		t.Error("key should be expired after TTL")
	}
}

func TestCache_Overwrite(t *testing.T) {
	c := &Cache{
		entries: make(map[string]cacheEntry),
		ttl:     1 * time.Minute,
	}

	c.Set("key", "v1")
	c.Set("key", "v2")

	got, ok := c.Get("key")
	if !ok {
		t.Fatal("Get should return true")
	}
	if got != "v2" {
		t.Errorf("Get = %v, want 'v2'", got)
	}
}

func TestCache_Cleanup(t *testing.T) {
	c := &Cache{
		entries: make(map[string]cacheEntry),
		ttl:     50 * time.Millisecond,
	}

	c.Set("a", 1)
	c.Set("b", 2)

	time.Sleep(60 * time.Millisecond)

	// Add a fresh entry
	c.Set("c", 3)

	// Run cleanup manually
	c.cleanup()

	c.mu.RLock()
	defer c.mu.RUnlock()

	if _, ok := c.entries["a"]; ok {
		t.Error("expired entry 'a' should be cleaned up")
	}
	if _, ok := c.entries["b"]; ok {
		t.Error("expired entry 'b' should be cleaned up")
	}
	if _, ok := c.entries["c"]; !ok {
		t.Error("fresh entry 'c' should still be present")
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	c := &Cache{
		entries: make(map[string]cacheEntry),
		ttl:     1 * time.Second,
	}

	var wg sync.WaitGroup
	// Concurrent writers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := "key"
			c.Set(key, n)
		}(i)
	}
	// Concurrent readers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Get("key")
		}()
	}
	// Concurrent cleanup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.cleanup()
	}()

	wg.Wait()

	// Just verify no panic/deadlock - the value is non-deterministic
	_, ok := c.Get("key")
	if !ok {
		t.Error("key should exist after concurrent writes")
	}
}

func TestCache_DifferentTypes(t *testing.T) {
	c := &Cache{
		entries: make(map[string]cacheEntry),
		ttl:     1 * time.Minute,
	}

	// Cache can store any type
	c.Set("string", "hello")
	c.Set("int", 42)
	c.Set("slice", []string{"a", "b"})
	c.Set("ptr", &Response{})

	if got, ok := c.Get("string"); !ok || got != "hello" {
		t.Errorf("string = %v, %v", got, ok)
	}
	if got, ok := c.Get("int"); !ok || got != 42 {
		t.Errorf("int = %v, %v", got, ok)
	}
	if got, ok := c.Get("ptr"); !ok || got == nil {
		t.Errorf("ptr = %v, %v", got, ok)
	}
}
