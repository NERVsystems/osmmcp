// Package core provides shared utilities for the OpenStreetMap MCP tools.
package core

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	"log/slog"

	"github.com/NERVsystems/osmmcp/pkg/cache"
)

const (
	// DefaultTileProvider is the default OSM tile server
	DefaultTileProvider = "https://tile.openstreetmap.org"

	// DefaultTileSize is the size of OSM tiles in pixels
	DefaultTileSize = 256

	// TileCacheTTL is how long to cache tiles
	TileCacheTTL = 24 * time.Hour
)

// TileCache is the cache for map tiles
var tileCache *cache.TTLCache

// TileResourceManager is the global tile resource manager
var tileResourceManager *cache.TileResourceManager

// InitTileCache initializes the tile cache
func InitTileCache() {
	// Use the existing cache implementation
	if tileCache == nil {
		tileCache = cache.NewTTLCache(TileCacheTTL, time.Minute, 1000)
	}
}

// InitTileResourceManager initializes the tile resource manager
func InitTileResourceManager(logger *slog.Logger) {
	if tileResourceManager == nil {
		tileResourceManager = cache.NewTileResourceManager(logger)
	}
}

// GetTileResourceManager returns the global tile resource manager
func GetTileResourceManager() *cache.TileResourceManager {
	return tileResourceManager
}

// LatLonToTile converts latitude, longitude and zoom to tile coordinates
func LatLonToTile(lat, lon float64, zoom int) (x, y int) {
	lat = math.Max(-85.05112878, math.Min(85.05112878, lat))
	n := math.Pow(2, float64(zoom))

	x = int(math.Floor((lon + 180.0) / 360.0 * n))
	y = int(math.Floor((1.0 - math.Log(math.Tan(lat*math.Pi/180.0)+1.0/math.Cos(lat*math.Pi/180.0))/math.Pi) / 2.0 * n))

	return x, y
}

// TileToLatLon converts tile coordinates to latitude, longitude
func TileToLatLon(x, y, zoom int) (lat, lon float64) {
	n := math.Pow(2, float64(zoom))
	lon = float64(x)/n*360.0 - 180.0

	latRad := math.Atan(math.Sinh(math.Pi * (1 - 2*float64(y)/n)))
	lat = latRad * 180.0 / math.Pi

	return lat, lon
}

// FetchMapTile retrieves a map tile with caching and resource management
func FetchMapTile(ctx context.Context, x, y, zoom int) ([]byte, error) {
	logger := slog.Default().With("service", "tile_fetcher")

	// Initialize cache if needed
	InitTileCache()

	// Create cache key for legacy cache
	cacheKey := fmt.Sprintf("tile:%d:%d:%d", zoom, x, y)

	// Check legacy cache first
	if cachedData, found := tileCache.Get(cacheKey); found {
		logger.Debug("tile cache hit", "key", cacheKey)
		tileData := cachedData.([]byte)

		// Update resource manager if available
		if tileResourceManager != nil {
			uri := fmt.Sprintf("osm://tile/%d/%d/%d", zoom, x, y)
			err := tileResourceManager.SetTileData(uri, tileData)
			if err != nil {
				logger.Warn("failed to update tile resource", "error", err)
			}
		}

		return tileData, nil
	}

	logger.Debug("tile cache miss", "key", cacheKey)

	// Build the tile URL
	tileURL := fmt.Sprintf("%s/%d/%d/%d.png", DefaultTileProvider, zoom, x, y)

	// Create HTTP request with retry factory
	requestFactory := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, tileURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "NERV-MCP-Client/1.0 (contact: ops@nerv.systems)")
		req.Header.Set("Referer", "https://github.com/NERVsystems/osmmcp")
		return req, nil
	}

	// Execute request with retries
	resp, err := WithRetryFactory(ctx, requestFactory, nil, DefaultRetryOptions)
	if err != nil {
		return nil, ServiceError("TileServer", http.StatusServiceUnavailable, "Failed to fetch map tile")
	}
	defer resp.Body.Close()

	// Handle error response
	if resp.StatusCode != http.StatusOK {
		return nil, ServiceError("TileServer", resp.StatusCode, fmt.Sprintf("Tile server error: %d", resp.StatusCode))
	}

	// Read tile data
	tileData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, NewError(ErrInternalError, "Failed to read tile data")
	}

	// Cache the result in legacy cache
	tileCache.Set(cacheKey, tileData)

	// Cache as resource if resource manager is available
	if tileResourceManager != nil {
		uri := fmt.Sprintf("osm://tile/%d/%d/%d", zoom, x, y)
		err := tileResourceManager.SetTileData(uri, tileData)
		if err != nil {
			logger.Warn("failed to cache tile as resource", "error", err)
		} else {
			logger.Debug("tile cached as resource", "uri", uri)
		}
	}

	return tileData, nil
}

// TileInfo contains information about a map tile
type TileInfo struct {
	Zoom      int     `json:"zoom"`
	X         int     `json:"x"`
	Y         int     `json:"y"`
	CenterLat float64 `json:"center_lat"`
	CenterLon float64 `json:"center_lon"`
	NorthLat  float64 `json:"north_lat"`
	SouthLat  float64 `json:"south_lat"`
	EastLon   float64 `json:"east_lon"`
	WestLon   float64 `json:"west_lon"`
	TileURL   string  `json:"tile_url"`
	PixelSize float64 `json:"pixel_size_meters"` // Approximate meters per pixel at this zoom/latitude
	MapScale  string  `json:"map_scale"`         // Approximate map scale (e.g. "1:10000")
}

// GetTileInfo returns information about a tile
func GetTileInfo(x, y, zoom int) TileInfo {
	// Calculate tile center using float to avoid truncation
	yCenter := float64(y) + 0.5
	centerLat, centerLon := TileToLatLon(x, int(yCenter), zoom)

	// Calculate tile bounds
	northLat, westLon := TileToLatLon(x, y, zoom)
	southLat, eastLon := TileToLatLon(x+1, y+1, zoom)

	// Calculate approximate meters per pixel at this latitude
	metersPerPixel := 156543.03 * math.Cos(centerLat*math.Pi/180) / math.Pow(2, float64(zoom))

	// Calculate approximate map scale (1:X)
	// Assuming 96 DPI (1 pixel = 0.26 mm)
	mapScale := metersPerPixel / 0.00026

	return TileInfo{
		Zoom:      zoom,
		X:         x,
		Y:         y,
		CenterLat: centerLat,
		CenterLon: centerLon,
		NorthLat:  northLat,
		SouthLat:  southLat,
		EastLon:   eastLon,
		WestLon:   westLon,
		TileURL:   fmt.Sprintf("%s/%d/%d/%d.png", DefaultTileProvider, zoom, x, y),
		PixelSize: metersPerPixel,
		MapScale:  "1:" + strconv.FormatInt(int64(math.Round(mapScale)), 10),
	}
}
