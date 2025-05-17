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

	"github.com/NERVsystems/osmmcp/pkg/core"
	"github.com/mark3labs/mcp-go/mcp"
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

	// Create a text description of the location
	description := fmt.Sprintf("Map location: %.6f, %.6f (zoom level: %d)\n", lat, lon, zoom)
	description += fmt.Sprintf("View this location on OpenStreetMap: %s\n\n", osmURL)
	description += fmt.Sprintf("Map area information:\n")
	description += fmt.Sprintf("- Bounds: North: %.6f, South: %.6f, East: %.6f, West: %.6f\n",
		tileInfo.NorthLat, tileInfo.SouthLat, tileInfo.EastLon, tileInfo.WestLon)
	description += fmt.Sprintf("- Scale: %s (%.2f meters per pixel)\n", tileInfo.MapScale, tileInfo.PixelSize)
	description += fmt.Sprintf("- Tile: %d/%d/%d\n", zoom, tileX, tileY)
	description += fmt.Sprintf("- Attribution: © OpenStreetMap contributors")

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

	// Get the tile image URL
	tileURL := fmt.Sprintf("https://tile.openstreetmap.org/%d/%d/%d.png", zoom, tileX, tileY)

	// Fetch the image data
	imageData, err := fetchImageFromURL(ctx, tileURL)
	if err != nil {
		logger.Error("failed to fetch image", "error", err)
		return core.NewError(core.ErrInternalError, "Failed to fetch map image").ToMCPResult(), nil
	}

	// Encode image to base64
	base64Image := encodeToBase64(imageData)

	// Return both the text description, metadata, and the image as the result
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.ImageContent{
				Type:     "image",
				Data:     base64Image,
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
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "NERV-MCP-Client/1.0 (contact: ops@nerv.systems)")
	req.Header.Set("Referer", "https://github.com/NERVsystems/osmmcp")

	resp, err := http.DefaultClient.Do(req)
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
