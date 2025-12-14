// Package cache provides tile caching functionality exposed as MCP resources.
package cache

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/NERVsystems/osmmcp/pkg/tracing"
	"github.com/mark3labs/mcp-go/mcp"
	"go.opentelemetry.io/otel/attribute"
)

const (
	// TileResourceScheme is the URI scheme for tile resources
	TileResourceScheme = "osm"

	// TileResourceType is the resource type for tiles
	TileResourceType = "tile"

	// DefaultTileCacheTTL is the default TTL for cached tiles
	DefaultTileCacheTTL = 24 * time.Hour

	// MaxCachedTiles is the maximum number of tiles to cache
	MaxCachedTiles = 1000
)

// TileResource represents a cached map tile as an MCP resource
type TileResource struct {
	URI         string       `json:"uri"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	MimeType    string       `json:"mimeType"`
	Data        []byte       `json:"-"` // Don't serialize raw data in JSON
	Metadata    TileMetadata `json:"metadata"`
	CachedAt    time.Time    `json:"cachedAt"`
}

// TileMetadata contains information about a tile resource
type TileMetadata struct {
	Zoom      int     `json:"zoom"`
	X         int     `json:"x"`
	Y         int     `json:"y"`
	CenterLat float64 `json:"centerLat"`
	CenterLon float64 `json:"centerLon"`
	NorthLat  float64 `json:"northLat"`
	SouthLat  float64 `json:"southLat"`
	EastLon   float64 `json:"eastLon"`
	WestLon   float64 `json:"westLon"`
	PixelSize float64 `json:"pixelSizeMeters"`
	MapScale  string  `json:"mapScale"`
}

// TileResourceManager manages tile resources for MCP
type TileResourceManager struct {
	cache  *TTLCache
	logger *slog.Logger
}

// NewTileResourceManager creates a new tile resource manager
func NewTileResourceManager(logger *slog.Logger) *TileResourceManager {
	return &TileResourceManager{
		cache:  NewTTLCache(DefaultTileCacheTTL, time.Minute, MaxCachedTiles),
		logger: logger.With("component", "tile_resource_manager"),
	}
}

// GetTileResource retrieves a tile resource by coordinates
func (trm *TileResourceManager) GetTileResource(ctx context.Context, x, y, zoom int) (*TileResource, error) {
	// Start tracing span
	_, span := tracing.StartSpan(ctx, "tile_cache.get_resource")
	defer span.End()

	span.SetAttributes(
		attribute.Int("tile.x", x),
		attribute.Int("tile.y", y),
		attribute.Int("tile.zoom", zoom),
	)

	logger := trm.logger.With("x", x, "y", y, "zoom", zoom)

	// Validate coordinates
	if zoom < 0 || zoom > 19 {
		return nil, fmt.Errorf("invalid zoom level: %d (must be 0-19)", zoom)
	}

	maxTiles := 1 << zoom
	if x < 0 || x >= maxTiles || y < 0 || y >= maxTiles {
		return nil, fmt.Errorf("invalid tile coordinates for zoom %d: x=%d, y=%d", zoom, x, y)
	}

	// Generate URI and cache key
	uri := formatTileURI(x, y, zoom)
	cacheKey := fmt.Sprintf("resource:%d:%d:%d", zoom, x, y)

	// Check if already cached as resource
	if cached, found := trm.cache.Get(cacheKey); found {
		logger.Debug("tile resource cache hit")
		// Record cache hit
		span.SetAttributes(tracing.CacheAttributes(tracing.CacheTypeTile, true, cacheKey)...)
		return cached.(*TileResource), nil
	}

	logger.Debug("tile resource cache miss, creating new resource")
	// Record cache miss
	span.SetAttributes(tracing.CacheAttributes(tracing.CacheTypeTile, false, cacheKey)...)

	// We'll need to fetch the tile data - this should be integrated with core.FetchMapTile
	// For now, create the resource structure without data
	metadata := trm.createTileMetadata(x, y, zoom)

	resource := &TileResource{
		URI:         uri,
		Name:        fmt.Sprintf("Map Tile %d/%d/%d", zoom, x, y),
		Description: fmt.Sprintf("OpenStreetMap tile at zoom %d, coordinates (%d, %d)", zoom, x, y),
		MimeType:    "image/png",
		Metadata:    metadata,
		CachedAt:    time.Now(),
	}

	// Cache the resource
	trm.cache.Set(cacheKey, resource)

	return resource, nil
}

// SetTileData updates a tile resource with actual image data
func (trm *TileResourceManager) SetTileData(uri string, data []byte) error {
	// Start tracing span
	ctx := context.Background()
	_, span := tracing.StartSpan(ctx, "tile_cache.set_data")
	defer span.End()

	span.SetAttributes(
		attribute.String("tile.uri", uri),
		attribute.Int("tile.data_size", len(data)),
	)

	x, y, zoom, err := parseTileURI(uri)
	if err != nil {
		return fmt.Errorf("invalid tile URI: %w", err)
	}

	cacheKey := fmt.Sprintf("resource:%d:%d:%d", zoom, x, y)

	// Get existing resource or create new one
	var resource *TileResource
	if cached, found := trm.cache.Get(cacheKey); found {
		resource = cached.(*TileResource)
	} else {
		// Create new resource
		metadata := trm.createTileMetadata(x, y, zoom)
		resource = &TileResource{
			URI:         uri,
			Name:        fmt.Sprintf("Map Tile %d/%d/%d", zoom, x, y),
			Description: fmt.Sprintf("OpenStreetMap tile at zoom %d, coordinates (%d, %d)", zoom, x, y),
			MimeType:    "image/png",
			Metadata:    metadata,
			CachedAt:    time.Now(),
		}
	}

	// Update with data
	resource.Data = data
	resource.CachedAt = time.Now()

	// Cache the updated resource
	trm.cache.Set(cacheKey, resource)

	trm.logger.Debug("tile data cached as resource", "uri", uri, "size", len(data))
	return nil
}

// ListTileResources returns a list of cached tile resources
func (trm *TileResourceManager) ListTileResources() []mcp.Resource {
	trm.cache.mu.RLock()
	defer trm.cache.mu.RUnlock()

	var resources []mcp.Resource

	for key, item := range trm.cache.items {
		if !strings.HasPrefix(key, "resource:") {
			continue
		}

		if item.Expired() {
			continue
		}

		tileResource, ok := item.Value.(*TileResource)
		if !ok {
			continue
		}

		// Convert to MCP resource
		mcpResource := mcp.Resource{
			URI:         tileResource.URI,
			Name:        tileResource.Name,
			Description: tileResource.Description,
			MIMEType:    tileResource.MimeType,
		}

		resources = append(resources, mcpResource)
	}

	trm.logger.Debug("listed tile resources", "count", len(resources))
	return resources
}

// ReadTileResource reads a tile resource by URI
func (trm *TileResourceManager) ReadTileResource(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	// Start tracing span
	_, span := tracing.StartSpan(ctx, "tile_cache.read_resource")
	defer span.End()

	span.SetAttributes(attribute.String("tile.uri", uri))

	logger := trm.logger.With("uri", uri)

	x, y, zoom, err := parseTileURI(uri)
	if err != nil {
		logger.Warn("invalid tile URI format", "error", err)
		span.RecordError(err)
		return nil, fmt.Errorf("invalid tile URI: %w", err)
	}

	span.SetAttributes(
		attribute.Int("tile.x", x),
		attribute.Int("tile.y", y),
		attribute.Int("tile.zoom", zoom),
	)

	cacheKey := fmt.Sprintf("resource:%d:%d:%d", zoom, x, y)

	cached, found := trm.cache.Get(cacheKey)
	if !found {
		logger.Debug("tile resource not found in cache")
		// Record cache miss
		span.SetAttributes(tracing.CacheAttributes(tracing.CacheTypeTile, false, cacheKey)...)
		return nil, fmt.Errorf("tile resource not found: %s", uri)
	}

	tileResource, ok := cached.(*TileResource)
	if !ok {
		logger.Error("invalid cached tile resource type")
		return nil, fmt.Errorf("invalid cached resource type")
	}

	// Prepare metadata as JSON
	metadataJSON, err := json.Marshal(tileResource.Metadata)
	if err != nil {
		logger.Error("failed to marshal tile metadata", "error", err)
		return nil, fmt.Errorf("failed to serialize metadata: %w", err)
	}

	var contents []mcp.ResourceContents

	// Add metadata as text content
	textContent := mcp.TextResourceContents{
		URI:      uri + "#metadata",
		MIMEType: "application/json",
		Text:     string(metadataJSON),
	}
	contents = append(contents, textContent)

	// Add image data if available
	if len(tileResource.Data) > 0 {
		base64Data := base64.StdEncoding.EncodeToString(tileResource.Data)
		blobContent := mcp.BlobResourceContents{
			URI:      uri,
			MIMEType: tileResource.MimeType,
			Blob:     base64Data,
		}
		contents = append(contents, blobContent)
	}

	logger.Debug("tile resource read successfully", "contents", len(contents))

	return &mcp.ReadResourceResult{
		Contents: contents,
	}, nil
}

// createTileMetadata creates metadata for a tile
func (trm *TileResourceManager) createTileMetadata(x, y, zoom int) TileMetadata {
	// This should use the same logic as core.GetTileInfo
	// For now, implementing basic calculations

	// Calculate tile bounds using Web Mercator projection
	n := float64(int(1) << zoom)

	// West and East longitude
	westLon := float64(x)/n*360.0 - 180.0
	eastLon := float64(x+1)/n*360.0 - 180.0

	// North and South latitude (Web Mercator inverse)
	northLatRad := math.Atan(math.Sinh(math.Pi * (1 - 2*float64(y)/n)))
	southLatRad := math.Atan(math.Sinh(math.Pi * (1 - 2*float64(y+1)/n)))

	northLat := northLatRad * 180.0 / math.Pi
	southLat := southLatRad * 180.0 / math.Pi

	// Center coordinates
	centerLon := (westLon + eastLon) / 2
	centerLat := (northLat + southLat) / 2

	// Calculate approximate meters per pixel
	pixelSize := 156543.03 * math.Cos(centerLat*math.Pi/180) / math.Pow(2, float64(zoom))

	// Map scale approximation
	mapScale := fmt.Sprintf("1:%d", int(pixelSize/0.00026))

	return TileMetadata{
		Zoom:      zoom,
		X:         x,
		Y:         y,
		CenterLat: centerLat,
		CenterLon: centerLon,
		NorthLat:  northLat,
		SouthLat:  southLat,
		EastLon:   eastLon,
		WestLon:   westLon,
		PixelSize: pixelSize,
		MapScale:  mapScale,
	}
}

// formatTileURI creates a URI for a tile resource
func formatTileURI(x, y, zoom int) string {
	return fmt.Sprintf("%s://%s/%d/%d/%d", TileResourceScheme, TileResourceType, zoom, x, y)
}

// parseTileURI parses a tile URI to extract coordinates
func parseTileURI(uri string) (x, y, zoom int, err error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid URI format: %w", err)
	}

	if parsed.Scheme != TileResourceScheme {
		return 0, 0, 0, fmt.Errorf("invalid scheme: expected %s, got %s", TileResourceScheme, parsed.Scheme)
	}

	if parsed.Host != TileResourceType {
		return 0, 0, 0, fmt.Errorf("invalid resource type: expected %s, got %s", TileResourceType, parsed.Host)
	}

	// Parse path: /zoom/x/y
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("invalid path format: expected /zoom/x/y, got %s", parsed.Path)
	}

	zoom, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid zoom: %w", err)
	}

	x, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid x coordinate: %w", err)
	}

	y, err = strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid y coordinate: %w", err)
	}

	return x, y, zoom, nil
}

// GetCacheStats returns statistics about the tile cache
func (trm *TileResourceManager) GetCacheStats() map[string]interface{} {
	return map[string]interface{}{
		"cached_tiles": trm.cache.Count(),
		"max_tiles":    MaxCachedTiles,
		"ttl_hours":    DefaultTileCacheTTL.Hours(),
	}
}
