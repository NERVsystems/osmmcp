package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"github.com/NERVsystems/osmmcp/pkg/core"
	"github.com/NERVsystems/osmmcp/pkg/tools"
	"github.com/mark3labs/mcp-go/mcp"
)

func main() {
	// Create a test request
	req := mcp.CallToolRequest{
		Params: struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments,omitempty"`
			Meta      *struct {
				ProgressToken mcp.ProgressToken `json:"progressToken,omitempty"`
			} `json:"_meta,omitempty"`
		}{
			Name: "get_map_image",
			Arguments: map[string]any{
				"latitude":  37.7749,
				"longitude": -122.4194,
				"zoom":      14,
				"format":    "json", // Add this to force JSON format
			},
		},
	}

	// Call the handler directly
	result, err := tools.HandleGetMapImage(context.Background(), req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Extract content
	var content string
	for _, c := range result.Content {
		if text, ok := c.(mcp.TextContent); ok {
			content = text.Text
			break
		}
	}

	// Check if content contains an error message
	if content == "" {
		fmt.Printf("Error: No content in response\n")
		os.Exit(1)
	}

	fmt.Printf("DEBUG: Content received: %s\n", content)

	// Try to parse as error first
	var errorResponse struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(content), &errorResponse); err == nil && errorResponse.Error.Message != "" {
		fmt.Printf("Error: %s\n", errorResponse.Error.Message)
		os.Exit(1)
	}

	// Parse the result
	var output struct {
		Tile struct {
			Base64Image string        `json:"base64_image"`
			TileInfo    core.TileInfo `json:"tile_info"`
		} `json:"tile"`
	}

	if err := json.Unmarshal([]byte(content), &output); err != nil {
		fmt.Printf("Failed to parse result: %v\n", err)
		os.Exit(1)
	}

	// Print success and tile info
	fmt.Printf("Success! Map tile retrieved.\n")
	fmt.Printf("Tile Info: X=%d, Y=%d, Zoom=%d\n",
		output.Tile.TileInfo.X,
		output.Tile.TileInfo.Y,
		output.Tile.TileInfo.Zoom)

	// Save the image to a file
	if len(output.Tile.Base64Image) > 22 { // Skip "data:image/png;base64," prefix
		imgData := output.Tile.Base64Image[22:] // Remove data URL prefix
		imgBytes, err := base64.StdEncoding.DecodeString(imgData)
		if err != nil {
			fmt.Printf("Failed to decode base64 image: %v\n", err)
			os.Exit(1)
		}

		// Write to file
		if err := os.WriteFile("test_tile.png", imgBytes, 0644); err != nil {
			fmt.Printf("Failed to write image file: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Saved tile image to test_tile.png")
	}
}
