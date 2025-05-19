package osm

import (
	"sync"
	"time"
)

// TTLCache is a generic thread-safe cache with TTL support
type TTLCache[K comparable, V any] struct {
	mu    sync.RWMutex
	items map[K]cacheItem[V]
	ttl   time.Duration
}

type cacheItem[V any] struct {
	value     V
	expiresAt time.Time
}

// NewTTLCache creates a new TTL cache with the specified TTL duration
func NewTTLCache[K comparable, V any](ttl time.Duration) *TTLCache[K, V] {
	return &TTLCache[K, V]{
		items: make(map[K]cacheItem[V]),
		ttl:   ttl,
	}
}

// Get retrieves a value from the cache if it exists and hasn't expired
func (c *TTLCache[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()

	item, exists := c.items[key]
	if !exists {
		c.mu.RUnlock()
		var zero V
		return zero, false
	}

	if time.Now().After(item.expiresAt) {
		// Upgrade from read to write lock to safely delete expired entry
		c.mu.RUnlock()
		c.mu.Lock()
		// Re-check after obtaining write lock in case it was updated
		if latest, ok := c.items[key]; ok && time.Now().After(latest.expiresAt) {
			delete(c.items, key)
		}
		c.mu.Unlock()
		var zero V
		return zero, false
	}

	value := item.value
	c.mu.RUnlock()
	return value, true
}

// Set adds a value to the cache with the configured TTL
func (c *TTLCache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = cacheItem[V]{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Delete removes a value from the cache
func (c *TTLCache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// Clear removes all items from the cache
func (c *TTLCache[K, V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[K]cacheItem[V])
}

// Size returns the number of items in the cache
func (c *TTLCache[K, V]) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Cleanup removes expired items from the cache
func (c *TTLCache[K, V]) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, item := range c.items {
		if now.After(item.expiresAt) {
			delete(c.items, key)
		}
	}
}
