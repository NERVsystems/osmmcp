// Package tools provides the OpenStreetMap MCP tools implementations.
package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/NERVsystems/osmmcp/pkg/core"
)

// GetMapImageTool returns a tool definition for retrieving and displaying map images
func GetMapImageTool() mcp.Tool {
	return mcp.NewTool("get_map_image",
		mcp.WithDescription("Retrieve and display an OpenStreetMap image for analysis"),
		mcp.WithNumber("latitude",
			mcp.Required(),
			mcp.Description("The latitude coordinate"),
		),
		mcp.WithNumber("longitude",
			mcp.Required(),
			mcp.Description("The longitude coordinate"),
		),
		mcp.WithNumber("zoom",
			mcp.Description("Zoom level (1-19, higher values show more detail)"),
			mcp.DefaultNumber(14),
		),
	)
}

// HandleGetMapImage implements map image retrieval and display functionality
func HandleGetMapImage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "get_map_image")

	// Parse and validate input parameters
	lat, lon, err := core.ParseCoordsWithLog(req, logger, "latitude", "longitude")
	if err != nil {
		return core.NewError(core.ErrInvalidInput, err.Error()).ToMCPResult(), nil
	}

	// Parse zoom level
	zoom := int(mcp.ParseFloat64(req, "zoom", 14))
	if zoom < 1 || zoom > 19 {
		return core.NewError(core.ErrInvalidInput, "Zoom level must be between 1 and 19").ToMCPResult(), nil
	}

	// Convert coordinates to tile coordinates
	tileX, tileY := core.LatLonToTile(lat, lon, zoom)

	// Get tile information
	tileInfo := core.GetTileInfo(tileX, tileY, zoom)

	// Create a direct URL to view this location on OpenStreetMap
	osmURL := fmt.Sprintf("https://www.openstreetmap.org/#map=%d/%.6f/%.6f", zoom, lat, lon)

	// Fetch the tile data (using the existing core function)
	tileData, err := core.FetchMapTile(ctx, tileX, tileY, zoom)
	if err != nil {
		logger.Error("failed to fetch tile", "error", err)
		return core.NewError(core.ErrInternalError, "Failed to fetch map tile").ToMCPResult(), nil
	}

	// Encode tile to base64 with data URL prefix
	base64Image := "data:image/png;base64," + base64.StdEncoding.EncodeToString(tileData)

	// Detect which response format to use based on request name
	toolName := req.Params.Name

	if toolName == "get_map_tile" || req.Params.Arguments["format"] == "json" {
		// Return JSON format for test_maptile.go compatibility
		response := struct {
			Tile struct {
				Base64Image string        `json:"base64_image"`
				TileInfo    core.TileInfo `json:"tile_info"`
			} `json:"tile"`
		}{}

		response.Tile.Base64Image = base64Image
		response.Tile.TileInfo = tileInfo

		// Convert to JSON
		jsonResponse, err := json.Marshal(response)
		if err != nil {
			logger.Error("failed to marshal response", "error", err)
			return core.NewError(core.ErrInternalError, "Failed to generate result").ToMCPResult(), nil
		}

		// Return the JSON response as text content
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: string(jsonResponse),
				},
			},
		}, nil
	}

	// Original response format
	// Create a text description of the location
	description := fmt.Sprintf("Map location: %.6f, %.6f (zoom level: %d)\n", lat, lon, zoom)
	description += fmt.Sprintf("View this location on OpenStreetMap: %s\n\n", osmURL)
	description += "Map area information:\n"
	description += fmt.Sprintf("- Bounds: North: %.6f, South: %.6f, East: %.6f, West: %.6f\n",
		tileInfo.NorthLat, tileInfo.SouthLat, tileInfo.EastLon, tileInfo.WestLon)
	description += fmt.Sprintf("- Scale: %s (%.2f meters per pixel)\n", tileInfo.MapScale, tileInfo.PixelSize)
	description += fmt.Sprintf("- Tile: %d/%d/%d\n", zoom, tileX, tileY)
	description += "- Attribution: © OpenStreetMap contributors"

	// Create metadata for the response
	metadata := struct {
		Coordinates struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"coordinates"`
		Zoom     int           `json:"zoom"`
		TileInfo core.TileInfo `json:"tile_info"`
		MapURL   string        `json:"map_url"`
	}{
		Coordinates: struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		}{
			Latitude:  lat,
			Longitude: lon,
		},
		Zoom:     zoom,
		TileInfo: tileInfo,
		MapURL:   osmURL,
	}

	// Convert metadata to JSON
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		logger.Error("failed to marshal metadata", "error", err)
		return core.NewError(core.ErrInternalError, "Failed to generate result").ToMCPResult(), nil
	}

	// Return both the text description, metadata, and the image as the result
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.ImageContent{
				Type:     "image",
				Data:     encodeToBase64(tileData),
				MIMEType: "image/png",
			},
			mcp.TextContent{
				Type: "text",
				Text: description + "\n\nMetadata: " + string(metadataJSON),
			},
		},
	}, nil
}

// fetchImageFromURL retrieves an image from a URL
func fetchImageFromURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "NERV-MCP-Client/1.0 (contact: ops@nerv.systems)")
	req.Header.Set("Referer", "https://github.com/NERVsystems/osmmcp")

	resp, err := core.DoWithRetry(ctx, req, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// encodeToBase64 encodes binary data to base64 string
func encodeToBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}
