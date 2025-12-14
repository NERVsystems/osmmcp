// Package tools provides tile cache management tools.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/NERVsystems/osmmcp/pkg/cache"
	"github.com/NERVsystems/osmmcp/pkg/core"
)

// GetTileCacheTool returns a tool definition for managing cached tiles
func GetTileCacheTool() mcp.Tool {
	return mcp.NewTool("tile_cache",
		mcp.WithDescription("Manage and access cached map tiles"),
		mcp.WithString("action",
			mcp.Required(),
			mcp.Description("Action to perform: 'list', 'get', 'stats'"),
		),
		mcp.WithNumber("x",
			mcp.Description("Tile X coordinate (required for 'get' action)"),
		),
		mcp.WithNumber("y",
			mcp.Description("Tile Y coordinate (required for 'get' action)"),
		),
		mcp.WithNumber("zoom",
			mcp.Description("Tile zoom level (required for 'get' action)"),
		),
	)
}

// HandleTileCache implements tile cache management functionality
func HandleTileCache(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "tile_cache")

	// Parse action parameter
	action := mcp.ParseString(req, "action", "")
	if action == "" {
		return core.NewError(core.ErrInvalidInput, "Action parameter is required").ToMCPResult(), nil
	}

	tileManager := core.GetTileResourceManager()
	if tileManager == nil {
		return core.NewError(core.ErrInternalError, "Tile resource manager not available").ToMCPResult(), nil
	}

	switch action {
	case "list":
		return handleListCachedTiles(ctx, tileManager, logger)
	case "get":
		return handleGetCachedTile(ctx, req, tileManager, logger)
	case "stats":
		return handleTileCacheStats(ctx, tileManager, logger)
	default:
		return core.NewError(core.ErrInvalidInput, fmt.Sprintf("Unknown action: %s. Use 'list', 'get', or 'stats'", action)).ToMCPResult(), nil
	}
}

// handleListCachedTiles lists all cached tile resources
func handleListCachedTiles(ctx context.Context, tileManager *cache.TileResourceManager, logger *slog.Logger) (*mcp.CallToolResult, error) {
	resources := tileManager.ListTileResources()

	// Create a simplified view of cached tiles
	type TileInfo struct {
		URI         string `json:"uri"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Zoom        int    `json:"zoom"`
		X           int    `json:"x"`
		Y           int    `json:"y"`
	}

	var tiles []TileInfo
	for _, resource := range resources {
		// Parse coordinates from URI (osm://tile/zoom/x/y)
		var zoom, x, y int
		if n, err := fmt.Sscanf(resource.URI, "osm://tile/%d/%d/%d", &zoom, &x, &y); err == nil && n == 3 {
			tiles = append(tiles, TileInfo{
				URI:         resource.URI,
				Name:        resource.Name,
				Description: resource.Description,
				Zoom:        zoom,
				X:           x,
				Y:           y,
			})
		}
	}

	response := struct {
		CachedTiles []TileInfo `json:"cached_tiles"`
		Count       int        `json:"count"`
	}{
		CachedTiles: tiles,
		Count:       len(tiles),
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		logger.Error("failed to marshal tile list", "error", err)
		return core.NewError(core.ErrInternalError, "Failed to serialize tile list").ToMCPResult(), nil
	}

	logger.Info("listed cached tiles", "count", len(tiles))

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(jsonResponse),
			},
		},
	}, nil
}

// handleGetCachedTile retrieves a specific cached tile
func handleGetCachedTile(ctx context.Context, req mcp.CallToolRequest, tileManager *cache.TileResourceManager, logger *slog.Logger) (*mcp.CallToolResult, error) {
	// Parse coordinates
	x := int(mcp.ParseFloat64(req, "x", -1))
	y := int(mcp.ParseFloat64(req, "y", -1))
	zoom := int(mcp.ParseFloat64(req, "zoom", -1))

	if x < 0 || y < 0 || zoom < 0 {
		return core.NewError(core.ErrInvalidInput, "x, y, and zoom parameters are required for 'get' action").ToMCPResult(), nil
	}

	// Create URI for the tile
	uri := fmt.Sprintf("osm://tile/%d/%d/%d", zoom, x, y)

	// Read the tile resource
	result, err := tileManager.ReadTileResource(ctx, uri)
	if err != nil {
		logger.Warn("tile not found in cache", "uri", uri, "error", err)
		return core.NewError(core.ErrInternalError, fmt.Sprintf("Tile not found in cache: %s", uri)).ToMCPResult(), nil
	}

	// Convert resource contents to tool response
	var contents []mcp.Content

	for _, content := range result.Contents {
		switch c := content.(type) {
		case mcp.TextResourceContents:
			contents = append(contents, mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Metadata for %s:\n%s", c.URI, c.Text),
			})
		case mcp.BlobResourceContents:
			// For images, we provide the base64 data and info
			preview := c.Blob
			if len(c.Blob) > 100 {
				preview = c.Blob[:100] + "..."
			}
			contents = append(contents, mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Tile image data (base64): %d bytes\nData: %s", len(c.Blob), preview),
			})
		}
	}

	if len(contents) == 0 {
		contents = append(contents, mcp.TextContent{
			Type: "text",
			Text: fmt.Sprintf("Tile found but no content available for %s", uri),
		})
	}

	logger.Info("retrieved cached tile", "uri", uri, "contents", len(result.Contents))

	return &mcp.CallToolResult{
		Content: contents,
	}, nil
}

// handleTileCacheStats returns cache statistics
func handleTileCacheStats(ctx context.Context, tileManager *cache.TileResourceManager, logger *slog.Logger) (*mcp.CallToolResult, error) {
	stats := tileManager.GetCacheStats()

	jsonResponse, err := json.Marshal(stats)
	if err != nil {
		logger.Error("failed to marshal cache stats", "error", err)
		return core.NewError(core.ErrInternalError, "Failed to serialize cache stats").ToMCPResult(), nil
	}

	logger.Info("retrieved cache stats", "cached_tiles", stats["cached_tiles"])

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(jsonResponse),
			},
		},
	}, nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
