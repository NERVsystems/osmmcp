package cache

import (
	"errors"
	"log/slog"
	"time"
)

// tileResourceTTL defines how long tile data is kept in the cache
const tileResourceTTL = 24 * time.Hour

// TileResourceManager manages cached tile data
// identified by a URI string.
type TileResourceManager struct {
	cache  *TTLCache
	logger *slog.Logger
}

// NewTileResourceManager creates a TileResourceManager with
// an internal TTL cache and logger.
func NewTileResourceManager(logger *slog.Logger) *TileResourceManager {
	if logger == nil {
		logger = slog.Default()
	}

	c := NewTTLCache(tileResourceTTL, time.Minute, 1000)

	return &TileResourceManager{
		cache:  c,
		logger: logger,
	}
}

// SetTileData stores tile bytes in the internal cache.
// The URI acts as the cache key.
func (m *TileResourceManager) SetTileData(uri string, data []byte) error {
	if uri == "" {
		return errors.New("uri cannot be empty")
	}
	if m == nil || m.cache == nil {
		return errors.New("tile resource manager not initialized")
	}

	m.cache.Set(uri, data)
	if m.logger != nil {
		m.logger.Debug("tile resource stored", "uri", uri)
	}
	return nil
}

// GetTileData retrieves cached tile bytes by URI.
func (m *TileResourceManager) GetTileData(uri string) ([]byte, bool) {
	if m == nil || m.cache == nil {
		return nil, false
	}
	if v, ok := m.cache.Get(uri); ok {
		if data, ok := v.([]byte); ok {
			return data, true
		}
	}
	return nil, false
}
