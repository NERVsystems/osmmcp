// Package cache provides caching mechanisms for API responses
// to improve performance and reduce external API calls.
package cache

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/NERVsystems/osmmcp/pkg/tracing"
)

// Item represents a cached item with expiration
type Item struct {
	Value      interface{}
	Expiration int64
}

// Expired checks if the item has expired
func (item Item) Expired() bool {
	if item.Expiration == 0 {
		return false
	}
	return time.Now().UnixNano() > item.Expiration
}

// TTLCache is a thread-safe cache with time-based expiration
type TTLCache struct {
	items           map[string]Item
	mu              sync.RWMutex
	defaultTTL      time.Duration
	cleanupInterval time.Duration
	maxItems        int
	stopCleanup     chan bool
	cleanupStarted  sync.Once
	cleanupStopped  sync.Once
}

// NewTTLCache creates a new cache with the specified TTL and cleanup interval
// maxItems specifies the maximum number of items before oldest are evicted
func NewTTLCache(defaultTTL, cleanupInterval time.Duration, maxItems int) *TTLCache {
	cache := &TTLCache{
		items:           make(map[string]Item),
		defaultTTL:      defaultTTL,
		cleanupInterval: cleanupInterval,
		maxItems:        maxItems,
		stopCleanup:     make(chan bool),
	}

	// Start the cleanup process
	cache.startCleanupTimer()

	return cache
}

// Set adds an item to the cache with the default TTL
func (c *TTLCache) Set(key string, value interface{}) {
	// Create context and start span for tracing
	ctx := context.Background()
	_, span := tracing.StartSpan(ctx, "cache.set")
	defer span.End()

	span.SetAttributes(
		attribute.String(tracing.AttrCacheType, tracing.CacheTypeOSM),
		attribute.String(tracing.AttrCacheKey, key),
		attribute.Int64("cache.ttl_ms", c.defaultTTL.Milliseconds()),
	)

	c.SetWithTTL(key, value, c.defaultTTL)
}

// SetWithTTL adds an item to the cache with a specific TTL
func (c *TTLCache) SetWithTTL(key string, value interface{}, ttl time.Duration) {
	// Create context and start span for tracing
	ctx := context.Background()
	_, span := tracing.StartSpan(ctx, "cache.set_with_ttl")
	defer span.End()

	var expiration int64

	if ttl > 0 {
		expiration = time.Now().Add(ttl).UnixNano()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = Item{
		Value:      value,
		Expiration: expiration,
	}

	// Set tracing attributes
	span.SetAttributes(
		attribute.String(tracing.AttrCacheType, tracing.CacheTypeOSM),
		attribute.String(tracing.AttrCacheKey, key),
		attribute.Int64("cache.ttl_ms", ttl.Milliseconds()),
		attribute.Int("cache.items_count", len(c.items)),
	)

	// If we're over capacity, remove oldest items
	if c.maxItems > 0 && len(c.items) > c.maxItems {
		c.evictOldest()
		span.SetAttributes(attribute.Bool("cache.eviction_triggered", true))
	}
}

// Get retrieves an item from the cache
// Returns the item and a bool indicating if the item was found
func (c *TTLCache) Get(key string) (interface{}, bool) {
	// Create context and start span for tracing
	ctx := context.Background()
	_, span := tracing.StartSpan(ctx, "cache.get")
	defer span.End()

	c.mu.RLock()
	item, found := c.items[key]
	c.mu.RUnlock()

	if !found {
		// Record cache miss
		span.SetAttributes(tracing.CacheAttributes(tracing.CacheTypeOSM, false, key)...)
		return nil, false
	}

	// Check if the item has expired
	if item.Expired() {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		// Record cache miss due to expiration
		span.SetAttributes(tracing.CacheAttributes(tracing.CacheTypeOSM, false, key)...)
		span.SetAttributes(attribute.Bool("cache.expired", true))
		return nil, false
	}

	// Record cache hit
	span.SetAttributes(tracing.CacheAttributes(tracing.CacheTypeOSM, true, key)...)
	return item.Value, true
}

// Delete removes an item from the cache
func (c *TTLCache) Delete(key string) {
	// Create context and start span for tracing
	ctx := context.Background()
	_, span := tracing.StartSpan(ctx, "cache.delete")
	defer span.End()

	span.SetAttributes(
		attribute.String(tracing.AttrCacheType, tracing.CacheTypeOSM),
		attribute.String(tracing.AttrCacheKey, key),
	)

	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

// Count returns the number of items in the cache
func (c *TTLCache) Count() int {
	c.mu.RLock()
	count := len(c.items)
	c.mu.RUnlock()
	return count
}

// Clear removes all items from the cache
func (c *TTLCache) Clear() {
	// Create context and start span for tracing
	ctx := context.Background()
	_, span := tracing.StartSpan(ctx, "cache.clear")
	defer span.End()

	c.mu.Lock()
	itemsCount := len(c.items)
	c.items = make(map[string]Item)
	c.mu.Unlock()

	span.SetAttributes(
		attribute.String(tracing.AttrCacheType, tracing.CacheTypeOSM),
		attribute.Int("cache.items_cleared", itemsCount),
	)
}

// evictOldest removes the oldest items when cache exceeds maxItems
// This function assumes the lock is already held
func (c *TTLCache) evictOldest() {
	// Create a slice of keys and their expiration times
	type keyExpiration struct {
		key        string
		expiration int64
	}

	// Calculate how many items to remove
	itemsToRemove := len(c.items) - c.maxItems
	if itemsToRemove <= 0 {
		return
	}

	// Collect all key expirations
	keyExpirations := make([]keyExpiration, 0, len(c.items))
	for k, v := range c.items {
		// Use MaxInt64 for items without expiration to treat them as lowest eviction priority
		exp := v.Expiration
		if exp == 0 {
			exp = math.MaxInt64
		}
		keyExpirations = append(keyExpirations, keyExpiration{k, exp})
	}

	// Sort by expiration time (oldest first)
	sort.Slice(keyExpirations, func(i, j int) bool {
		return keyExpirations[i].expiration < keyExpirations[j].expiration
	})

	// Delete the oldest items
	for i := 0; i < itemsToRemove; i++ {
		delete(c.items, keyExpirations[i].key)
	}
}

// startCleanupTimer starts the cleanup timer
func (c *TTLCache) startCleanupTimer() {
	if c.cleanupInterval <= 0 {
		return
	}

	c.cleanupStarted.Do(func() {
		ticker := time.NewTicker(c.cleanupInterval)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					// Log the panic and restart the cleanup goroutine
					// Note: We can't use a logger here as it's not available in this context
					// In a production system, you'd want to inject a logger

					// Restart the cleanup goroutine after a brief delay to prevent tight loops
					time.Sleep(time.Second)
					c.cleanupStarted = sync.Once{} // Reset the Once to allow restart
					c.startCleanupTimer()
				}
			}()

			for {
				select {
				case <-ticker.C:
					// Add panic recovery around the cleanup operation
					func() {
						defer func() {
							if r := recover(); r != nil {
								// Log panic but don't stop the cleanup loop
								// The cleanup will continue on the next tick
							}
						}()
						c.deleteExpired()
					}()
				case <-c.stopCleanup:
					ticker.Stop()
					return
				}
			}
		}()
	})
}

// deleteExpired deletes all expired items
func (c *TTLCache) deleteExpired() {
	now := time.Now().UnixNano()

	c.mu.Lock()
	for k, v := range c.items {
		if v.Expiration > 0 && v.Expiration < now {
			delete(c.items, k)
		}
	}
	c.mu.Unlock()
}

// Stop stops the cleanup timer
func (c *TTLCache) Stop() {
	c.cleanupStopped.Do(func() {
		close(c.stopCleanup)
	})
}

// Global cache instance
var (
	globalCache     *TTLCache
	globalCacheOnce sync.Once
	globalCacheMu   sync.Mutex
)

// GetGlobalCache returns the global cache instance
func GetGlobalCache() *TTLCache {
	globalCacheOnce.Do(func() {
		// 5 minute TTL, cleanup every minute, max 1000 items
		globalCache = NewTTLCache(5*time.Minute, time.Minute, 1000)
	})
	return globalCache
}

// StopGlobalCache stops the global cache cleanup routine
func StopGlobalCache() {
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()

	if globalCache != nil {
		globalCache.Stop()
	}
}
