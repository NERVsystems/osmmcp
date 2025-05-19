package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/NERVsystems/osmmcp/pkg/core"
	"github.com/NERVsystems/osmmcp/pkg/geo"
	"github.com/NERVsystems/osmmcp/pkg/osm"
	"github.com/mark3labs/mcp-go/mcp"
)

// RouteFetchInput defines the input parameters for fetching a route
type RouteFetchInput struct {
	Start geo.Location `json:"start"`
	End   geo.Location `json:"end"`
	Mode  string       `json:"mode"`
}

// RouteFetchOutput defines the output for a fetched route
type RouteFetchOutput struct {
	Polyline string  `json:"polyline"`
	Distance float64 `json:"distance"` // in meters
	Duration float64 `json:"duration"` // in seconds
}

// RouteFetchTool returns a tool definition for fetching routes
func RouteFetchTool() mcp.Tool {
	return mcp.NewTool("route_fetch",
		mcp.WithDescription("Fetch a route between two points using OSRM routing service"),
		mcp.WithObject("start",
			mcp.Required(),
			mcp.Description("The starting point as {latitude, longitude}"),
		),
		mcp.WithObject("end",
			mcp.Required(),
			mcp.Description("The ending point as {latitude, longitude}"),
		),
		mcp.WithString("mode",
			mcp.Description("Travel mode (car, bike, foot)"),
			mcp.DefaultString("car"),
		),
	)
}

// convertModeToProfile maps user-friendly travel modes to OSRM profile names
func convertModeToProfile(mode string) string {
	switch mode {
	case "car", "driving", "drive":
		return "car"
	case "bike", "bicycle", "cycling":
		return "bike"
	case "foot", "walk", "walking":
		return "foot"
	default:
		return ""
	}
}

// HandleRouteFetch implements route fetching functionality
func HandleRouteFetch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "route_fetch")

	// Parse input
	var input RouteFetchInput
	inputJSON, err := json.Marshal(req.Params.Arguments)
	if err != nil {
		logger.Error("failed to marshal input", "error", err)
		return core.NewError(core.ErrInvalidInput, "Invalid input format").ToMCPResult(), nil
	}

	if err := json.Unmarshal(inputJSON, &input); err != nil {
		logger.Error("failed to parse input", "error", err)
		return core.NewError(core.ErrInvalidInput, "Invalid input format").ToMCPResult(), nil
	}

	// Validate input coordinates using core validation
	if err := core.ValidateCoords(input.Start.Latitude, input.Start.Longitude); err != nil {
		logger.Error("invalid 'start' coordinates", "error", err)
		return core.NewError(core.ErrInvalidLatitude, fmt.Sprintf("Invalid start coordinates: %s", err)).ToMCPResult(), nil
	}

	if err := core.ValidateCoords(input.End.Latitude, input.End.Longitude); err != nil {
		logger.Error("invalid 'end' coordinates", "error", err)
		return core.NewError(core.ErrInvalidLongitude, fmt.Sprintf("Invalid end coordinates: %s", err)).ToMCPResult(), nil
	}

	// Validate mode
	profile := convertModeToProfile(input.Mode)
	if profile == "" {
		logger.Error("invalid mode", "mode", input.Mode)
		errResult := core.NewError(core.ErrInvalidParameter, fmt.Sprintf("Invalid mode: %s", input.Mode))
		errResult = errResult.WithGuidance("Use 'car', 'bike', or 'foot'")
		return errResult.ToMCPResult(), nil
	}

	// Setup the coordinates (longitude first, latitude second, as expected by OSRM)
	startCoord := []float64{input.Start.Longitude, input.Start.Latitude}
	endCoord := []float64{input.End.Longitude, input.End.Latitude}

	// Use the simpler core.GetSimpleRoute helper
	route, err := core.GetSimpleRoute(ctx, startCoord, endCoord, profile)
	if err != nil {
		logger.Error("failed to get route", "error", err)
		if mcpErr, ok := err.(*core.MCPError); ok {
			return mcpErr.ToMCPResult(), nil
		}
		// Fallback for other errors
		return core.ServiceError("OSRM", http.StatusServiceUnavailable, "Failed to get route").
			WithGuidance("Try again later or check if the locations are reachable").
			ToMCPResult(), nil
	}

	// Create output from route result
	output := RouteFetchOutput{
		Polyline: route.Polyline,
		Distance: route.Distance,
		Duration: route.Duration,
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return core.NewError(core.ErrInternalError, "Failed to generate result").ToMCPResult(), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}

// RouteSampleInput defines the input parameters for sampling points along a route
type RouteSampleInput struct {
	Polyline string  `json:"polyline"`
	Interval float64 `json:"interval"` // in meters
}

// RouteSampleOutput defines the output for sampled route points
type RouteSampleOutput struct {
	Points []geo.Location `json:"points"`
}

// RouteSampleTool returns a tool definition for sampling points along a route
func RouteSampleTool() mcp.Tool {
	return mcp.NewTool("route_sample",
		mcp.WithDescription("Sample points along a route at specified intervals"),
		mcp.WithString("polyline",
			mcp.Required(),
			mcp.Description("The encoded polyline string representing the route"),
		),
		mcp.WithNumber("interval",
			mcp.Required(),
			mcp.Description("Sampling interval in meters (must be > 0)"),
		),
	)
}

// HandleRouteSample implements route sampling functionality
func HandleRouteSample(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "route_sample")

	// Parse input
	var input RouteSampleInput
	inputJSON, err := json.Marshal(req.Params.Arguments)
	if err != nil {
		logger.Error("failed to marshal input", "error", err)
		return ErrorResponse("Invalid input format"), nil
	}

	if err := json.Unmarshal(inputJSON, &input); err != nil {
		logger.Error("failed to parse input", "error", err)
		return ErrorResponse("Invalid input format"), nil
	}

	// Validate polyline
	if input.Polyline == "" {
		logger.Error("empty polyline")
		return ErrorResponse("Polyline string is required"), nil
	}

	// Validate interval
	if input.Interval <= 0 {
		logger.Error("invalid interval", "interval", input.Interval)
		return ErrorResponse("Interval must be greater than 0"), nil
	}

	// Decode the polyline
	points := osm.DecodePolyline(input.Polyline)
	if len(points) < 2 {
		logger.Error("polyline has too few points", "count", len(points))
		return ErrorResponse("Polyline must contain at least 2 points"), nil
	}

	// Sample points along the route
	sampledPoints := samplePolylinePoints(points, input.Interval)

	// Create output
	output := RouteSampleOutput{
		Points: sampledPoints,
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return ErrorResponse("Failed to generate result"), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}

// samplePolylinePoints samples points along a polyline at the specified interval
// Optimized for better performance by avoiding redundant distance calculations
func samplePolylinePoints(points []geo.Location, interval float64) []geo.Location {
	if len(points) < 2 || interval <= 0 {
		return points
	}

	// Start with the first point
	result := []geo.Location{points[0]}

	// currentPoint tracks our last sampled position
	currentPoint := points[0]
	// remaining holds the distance left until the next sample
	remaining := interval

	for i := 0; i < len(points)-1; i++ {
		start := currentPoint
		end := points[i+1]

		for {
			// Distance from the current point to the end of the segment
			segmentDistance := geo.HaversineDistance(start.Latitude, start.Longitude, end.Latitude, end.Longitude)

			if segmentDistance < remaining {
				// Not enough distance left in this segment
				remaining -= segmentDistance
				currentPoint = end
				break
			}

			// Interpolate a new point at the required fraction
			fraction := remaining / segmentDistance
			newPoint := geo.Location{
				Latitude:  start.Latitude + (end.Latitude-start.Latitude)*fraction,
				Longitude: start.Longitude + (end.Longitude-start.Longitude)*fraction,
			}

			result = append(result, newPoint)

			// Prepare for the next sample
			start = newPoint
			currentPoint = newPoint
			remaining = interval

			// Continue sampling within the same segment if distance remains
		}
	}

	// Ensure the final point of the polyline is included
	if last := points[len(points)-1]; result[len(result)-1] != last {
		result = append(result, last)
	}

	return result
}

// EnrichEmissionsInput defines the input parameters for enriching route options with emissions data
type EnrichEmissionsInput struct {
	Options []struct {
		Mode     string  `json:"mode"`
		Distance float64 `json:"distance"`
		Duration float64 `json:"duration,omitempty"`
	} `json:"options"`
}

// EnrichEmissionsOutput defines the output for enriched route options
type EnrichEmissionsOutput struct {
	Options []struct {
		Mode         string  `json:"mode"`
		Distance     float64 `json:"distance"`
		Duration     float64 `json:"duration,omitempty"`
		CO2Kg        float64 `json:"co2_kg,omitempty"`
		CaloriesKcal float64 `json:"calories_kcal,omitempty"`
		CostLocal    float64 `json:"cost_local,omitempty"`
	} `json:"options"`
}

// EnrichEmissionsTool returns a tool definition for enriching route options with emissions data
func EnrichEmissionsTool() mcp.Tool {
	return mcp.NewTool("enrich_emissions",
		mcp.WithDescription("Enrich route options with CO2 emissions, calorie burn, and cost estimates"),
		mcp.WithArray("options",
			mcp.Required(),
			mcp.Description("Array of route options with mode and distance (and optional duration)"),
		),
	)
}

// HandleEnrichEmissions implements emissions enrichment functionality
func HandleEnrichEmissions(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "enrich_emissions")

	// Parse input
	var input EnrichEmissionsInput
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
	if len(input.Options) == 0 {
		logger.Error("empty options array")
		return ErrorResponse("At least one route option is required"), nil
	}

	// Enrich each option with emissions data
	var output EnrichEmissionsOutput
	output.Options = make([]struct {
		Mode         string  `json:"mode"`
		Distance     float64 `json:"distance"`
		Duration     float64 `json:"duration,omitempty"`
		CO2Kg        float64 `json:"co2_kg,omitempty"`
		CaloriesKcal float64 `json:"calories_kcal,omitempty"`
		CostLocal    float64 `json:"cost_local,omitempty"`
	}, len(input.Options))

	for i, option := range input.Options {
		// Validate mode and distance
		if option.Distance <= 0 {
			logger.Error("invalid distance", "distance", option.Distance, "index", i)
			return ErrorResponse(fmt.Sprintf("Invalid distance for option %d: must be greater than 0", i)), nil
		}

		// Copy basic fields
		output.Options[i].Mode = option.Mode
		output.Options[i].Distance = option.Distance
		output.Options[i].Duration = option.Duration

		// Calculate emissions based on mode
		distanceKm := option.Distance / 1000
		switch option.Mode {
		case "car", "driving", "drive":
			output.Options[i].CO2Kg = CarCO2PerKm * distanceKm
			output.Options[i].CostLocal = CarCostPerKm * distanceKm

		case "bike", "bicycle", "cycling":
			output.Options[i].CO2Kg = BikeCO2PerKm * distanceKm
			output.Options[i].CaloriesKcal = BikeCaloriesPerKm * distanceKm

		case "foot", "walk", "walking":
			output.Options[i].CO2Kg = WalkingCO2PerKm * distanceKm
			output.Options[i].CaloriesKcal = WalkingCaloriesPerKm * distanceKm

		case "transit", "public_transport", "public_transit", "bus":
			output.Options[i].CO2Kg = TransitCO2PerKm * distanceKm
			output.Options[i].CostLocal = TransitCostPerKm * distanceKm

		case "electric_car", "ev":
			output.Options[i].CO2Kg = ElectricCarCO2PerKm * distanceKm
			output.Options[i].CostLocal = ElectricCarCostPerKm * distanceKm

		default:
			// Unknown mode, skip enrichment
			logger.Warn("unknown mode, skipping enrichment", "mode", option.Mode, "index", i)
		}
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return ErrorResponse("Failed to generate result"), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}
