package cache

import (
	"testing"
	"time"
)

func TestTTLCacheSetAndGet(t *testing.T) {
	c := NewTTLCache(1*time.Second, 0, 10)
	defer c.Stop()

	c.Set("key", "value")

	if c.Count() != 1 {
		t.Fatalf("expected count 1, got %d", c.Count())
	}

	v, ok := c.Get("key")
	if !ok {
		t.Fatalf("expected to find key")
	}
	if s, ok := v.(string); !ok || s != "value" {
		t.Errorf("expected value 'value', got %v", v)
	}
}

func TestTTLCacheExpiration(t *testing.T) {
	c := NewTTLCache(50*time.Millisecond, 10*time.Millisecond, 10)
	defer c.Stop()

	c.Set("temp", "data")
	time.Sleep(100 * time.Millisecond)

	if _, ok := c.Get("temp"); ok {
		t.Errorf("expected item to expire")
	}
	if c.Count() != 0 {
		t.Errorf("expected cache to be empty after expiration, got %d", c.Count())
	}
}

func TestTTLCacheEviction(t *testing.T) {
	c := NewTTLCache(1*time.Second, 0, 2)
	defer c.Stop()

	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3) // should evict "a"

	if c.Count() != 2 {
		t.Fatalf("expected count 2 after eviction, got %d", c.Count())
	}

	if _, ok := c.Get("a"); ok {
		t.Errorf("expected 'a' to be evicted")
	}

	if v, ok := c.Get("b"); !ok || v.(int) != 2 {
		t.Errorf("expected to get 2 for 'b', got %v", v)
	}
	if v, ok := c.Get("c"); !ok || v.(int) != 3 {
		t.Errorf("expected to get 3 for 'c', got %v", v)
	}
}
