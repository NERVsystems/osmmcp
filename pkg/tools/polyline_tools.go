package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/NERVsystems/osmmcp/pkg/geo"
	"github.com/NERVsystems/osmmcp/pkg/osm"
	"github.com/mark3labs/mcp-go/mcp"
)

// PolylineDecodeInput defines the input parameters for decoding a polyline
type PolylineDecodeInput struct {
	Polyline string `json:"polyline"`
}

// PolylineDecodeOutput defines the output for decoded polyline points
type PolylineDecodeOutput struct {
	Points []geo.Location `json:"points"`
}

// PolylineDecodeTool returns a tool definition for decoding polylines
func PolylineDecodeTool() mcp.Tool {
	return mcp.NewTool("polyline_decode",
		mcp.WithDescription("Decode an encoded polyline string into a series of geographic coordinates"),
		mcp.WithString("polyline",
			mcp.Required(),
			mcp.Description("The encoded polyline string to decode"),
		),
	)
}

// HandlePolylineDecode implements polyline decoding
func HandlePolylineDecode(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "polyline_decode")

	// Parse input
	polyline := mcp.ParseString(req, "polyline", "")
	if polyline == "" {
		logger.Error("missing polyline input")
		return ErrorResponse("Polyline string is required"), nil
	}

	// Basic validation - ensure the string has at least 2 characters and only printable ASCII
	if len(polyline) < 2 || !isPrintableASCII(polyline) {
		logger.Error("failed to decode polyline", "polyline", polyline)
		return ErrorResponse("Failed to decode polyline: malformed input"), nil
	}

	// Decode the polyline
	locations := osm.DecodePolyline(polyline)
	if len(locations) == 0 {
		logger.Error("failed to decode polyline", "polyline", polyline)
		return ErrorResponse("Failed to decode polyline: malformed input"), nil
	}

	// Create output
	output := PolylineDecodeOutput{
		Points: locations,
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return ErrorResponse("Failed to generate result"), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}

// isPrintableASCII checks if a string contains only printable ASCII characters
func isPrintableASCII(s string) bool {
	for _, c := range s {
		if c < 32 || c > 126 {
			return false
		}
	}
	return true
}

// PolylineEncodeInput defines the input parameters for encoding points to a polyline
type PolylineEncodeInput struct {
	Points []geo.Location `json:"points"`
}

// PolylineEncodeOutput defines the output for an encoded polyline
type PolylineEncodeOutput struct {
	Polyline string `json:"polyline"`
}

// PolylineEncodeTool returns a tool definition for encoding points to a polyline
func PolylineEncodeTool() mcp.Tool {
	return mcp.NewTool("polyline_encode",
		mcp.WithDescription("Encode a series of geographic coordinates into a polyline string"),
		mcp.WithArray("points",
			mcp.Required(),
			mcp.Description("Array of latitude/longitude points to encode"),
		),
	)
}

// HandlePolylineEncode implements polyline encoding
func HandlePolylineEncode(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "polyline_encode")

	// Parse input
	var input PolylineEncodeInput
	inputJSON, err := json.Marshal(req.Params.Arguments)
	if err != nil {
		logger.Error("failed to marshal input", "error", err)
		return ErrorResponse("Invalid input format"), nil
	}

	if err := json.Unmarshal(inputJSON, &input); err != nil {
		logger.Error("failed to parse input", "error", err)
		return ErrorResponse("Invalid input format"), nil
	}

	// Validate input
	if len(input.Points) == 0 {
		logger.Error("empty points array")
		return ErrorResponse("At least one point is required"), nil
	}

	// Validate and convert to Location slice
	for i, p := range input.Points {
		// Validate zero coordinates
		if p.Latitude == 0 && p.Longitude == 0 {
			logger.Error("missing coordinates", "index", i)
			return ErrorResponse(fmt.Sprintf("Missing coordinates at index %d", i)), nil
		}

		// Validate coordinates range
		if err := osm.ValidateCoords(p.Latitude, p.Longitude); err != nil {
			logger.Error("invalid coordinates", "error", err, "index", i)
			return ErrorResponse(fmt.Sprintf("Invalid coordinates at index %d: %s", i, err)), nil
		}
	}

	// Encode to polyline
	polyline := osm.EncodePolyline(input.Points)

	// Create output
	output := PolylineEncodeOutput{
		Polyline: polyline,
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return ErrorResponse("Failed to generate result"), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}
