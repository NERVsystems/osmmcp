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

// GeoDistanceInput defines the input parameters for calculating distance
type GeoDistanceInput struct {
	From geo.Location `json:"from"`
	To   geo.Location `json:"to"`
}

// GeoDistanceOutput defines the output for distance calculation
type GeoDistanceOutput struct {
	Distance float64 `json:"distance"` // in meters
}

// GeoDistanceTool returns a tool definition for calculating geographic distance
func GeoDistanceTool() mcp.Tool {
	return mcp.NewTool("geo_distance",
		mcp.WithDescription("Calculate the distance between two geographic coordinates using the Haversine formula"),
		mcp.WithObject("from",
			mcp.Required(),
			mcp.Description("The starting point as {latitude, longitude}"),
		),
		mcp.WithObject("to",
			mcp.Required(),
			mcp.Description("The ending point as {latitude, longitude}"),
		),
	)
}

// HandleGeoDistance implements geographic distance calculation
func HandleGeoDistance(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "geo_distance")

	// Parse input
	var input GeoDistanceInput
	inputJSON, err := json.Marshal(req.Params.Arguments)
	if err != nil {
		logger.Error("failed to marshal input", "error", err)
		return ErrorResponse("Invalid input format"), nil
	}

	if err := json.Unmarshal(inputJSON, &input); err != nil {
		logger.Error("failed to parse input", "error", err)
		return ErrorResponse("Invalid input format"), nil
	}

	// Validate input coordinates
	if input.From.Latitude == 0 && input.From.Longitude == 0 {
		logger.Error("missing 'from' coordinates")
		return ErrorResponse("Missing 'from' coordinates"), nil
	}

	if input.To.Latitude == 0 && input.To.Longitude == 0 {
		logger.Error("missing 'to' coordinates")
		return ErrorResponse("Missing 'to' coordinates"), nil
	}

	if err := osm.ValidateCoords(input.From.Latitude, input.From.Longitude); err != nil {
		logger.Error("invalid 'from' coordinates", "error", err)
		return ErrorResponse(fmt.Sprintf("Invalid 'from' coordinates: %s", err)), nil
	}

	if err := osm.ValidateCoords(input.To.Latitude, input.To.Longitude); err != nil {
		logger.Error("invalid 'to' coordinates", "error", err)
		return ErrorResponse(fmt.Sprintf("Invalid 'to' coordinates: %s", err)), nil
	}

	// Calculate distance using Haversine formula
	distance := geo.HaversineDistance(
		input.From.Latitude, input.From.Longitude,
		input.To.Latitude, input.To.Longitude,
	)

	// Create output
	output := GeoDistanceOutput{
		Distance: distance,
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return ErrorResponse("Failed to generate result"), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}

// BBoxFromPointsInput defines the input parameters for creating a bounding box
type BBoxFromPointsInput struct {
	Points []geo.Location `json:"points"`
}

// BBoxFromPointsOutput defines the output for bounding box creation
type BBoxFromPointsOutput struct {
	BBox geo.BoundingBox `json:"bbox"`
}

// BBoxFromPointsTool returns a tool definition for creating a bounding box from points
func BBoxFromPointsTool() mcp.Tool {
	return mcp.NewTool("bbox_from_points",
		mcp.WithDescription("Create a bounding box that encompasses all given geographic coordinates"),
		mcp.WithArray("points",
			mcp.Required(),
			mcp.Description("Array of latitude/longitude points to include in the bounding box"),
		),
	)
}

// HandleBBoxFromPoints implements bounding box creation from points
func HandleBBoxFromPoints(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "bbox_from_points")

	// Parse input
	var input BBoxFromPointsInput
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

	// Create and extend bounding box
	bbox := geo.NewBoundingBox()
	for i, p := range input.Points {
		// Validate coordinates
		if p.Latitude == 0 && p.Longitude == 0 {
			logger.Error("missing coordinates", "index", i)
			return ErrorResponse(fmt.Sprintf("Missing coordinates at index %d", i)), nil
		}

		if err := osm.ValidateCoords(p.Latitude, p.Longitude); err != nil {
			logger.Error("invalid coordinates", "error", err, "index", i)
			return ErrorResponse(fmt.Sprintf("Invalid coordinates at index %d: %s", i, err)), nil
		}
		bbox.ExtendWithPoint(p.Latitude, p.Longitude)
	}

	// Create output
	output := BBoxFromPointsOutput{
		BBox: *bbox,
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return ErrorResponse("Failed to generate result"), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}

// CentroidPointsInput defines the input parameters for calculating the centroid
type CentroidPointsInput struct {
	Points []geo.Location `json:"points"`
}

// CentroidPointsOutput defines the output for centroid calculation
type CentroidPointsOutput struct {
	Centroid geo.Location `json:"centroid"`
}

// CentroidPointsTool returns a tool definition for calculating the centroid of points
func CentroidPointsTool() mcp.Tool {
	return mcp.NewTool("centroid_points",
		mcp.WithDescription("Calculate the geographic centroid (mean center) of a set of coordinates"),
		mcp.WithArray("points",
			mcp.Required(),
			mcp.Description("Array of latitude/longitude points to calculate centroid from"),
		),
	)
}

// HandleCentroidPoints implements point centroid calculation
func HandleCentroidPoints(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "centroid_points")

	// Parse input
	var input CentroidPointsInput
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

	// Calculate centroid (simple mean of lat/lon values)
	var sumLat, sumLon float64
	for i, p := range input.Points {
		// Validate coordinates
		if p.Latitude == 0 && p.Longitude == 0 {
			logger.Error("missing coordinates", "index", i)
			return ErrorResponse(fmt.Sprintf("Missing coordinates at index %d", i)), nil
		}

		if err := osm.ValidateCoords(p.Latitude, p.Longitude); err != nil {
			logger.Error("invalid coordinates", "error", err, "index", i)
			return ErrorResponse(fmt.Sprintf("Invalid coordinates at index %d: %s", i, err)), nil
		}
		sumLat += p.Latitude
		sumLon += p.Longitude
	}

	centroid := geo.Location{
		Latitude:  sumLat / float64(len(input.Points)),
		Longitude: sumLon / float64(len(input.Points)),
	}

	// Create output
	output := CentroidPointsOutput{
		Centroid: centroid,
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return ErrorResponse("Failed to generate result"), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}
