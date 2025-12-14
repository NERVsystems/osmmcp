// Package tools provides the OpenStreetMap MCP tools implementations.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/NERVsystems/osmmcp/pkg/osm"
)

// OSRMRouteResponse represents the response from the OSRM routing service
type OSRMRouteResponse struct {
	Code      string         `json:"code"`
	Message   string         `json:"message,omitempty"`
	Routes    []OSRMRoute    `json:"routes,omitempty"`
	Waypoints []OSRMWaypoint `json:"waypoints,omitempty"`
}

// OSRMRoute represents a single route in the OSRM response
type OSRMRoute struct {
	Distance   float64   `json:"distance"`
	Duration   float64   `json:"duration"`
	Geometry   string    `json:"geometry"`
	Legs       []OSRMLeg `json:"legs"`
	Weight     float64   `json:"weight"`
	WeightName string    `json:"weight_name"`
}

// OSRMLeg represents a leg of the OSRM route
type OSRMLeg struct {
	Distance float64    `json:"distance"`
	Duration float64    `json:"duration"`
	Steps    []OSRMStep `json:"steps"`
	Summary  string     `json:"summary"`
	Weight   float64    `json:"weight"`
}

// OSRMStep represents a step in an OSRM leg
type OSRMStep struct {
	Distance float64      `json:"distance"`
	Duration float64      `json:"duration"`
	Geometry string       `json:"geometry"`
	Maneuver OSRMManeuver `json:"maneuver"`
	Mode     string       `json:"mode"`
	Name     string       `json:"name"`
	Weight   float64      `json:"weight"`
}

// OSRMManeuver represents a maneuver in an OSRM step
type OSRMManeuver struct {
	BearingAfter  int       `json:"bearing_after"`
	BearingBefore int       `json:"bearing_before"`
	Location      []float64 `json:"location"`
	Type          string    `json:"type"`
}

// OSRMWaypoint represents a waypoint in the OSRM route
type OSRMWaypoint struct {
	Distance float64   `json:"distance"`
	Name     string    `json:"name"`
	Location []float64 `json:"location"`
}

// GetRouteTool returns a tool definition for route calculation
func GetRouteTool() mcp.Tool {
	return mcp.NewTool("get_route",
		mcp.WithDescription("Calculate a route between two points"),
		mcp.WithNumber("start_lat",
			mcp.Required(),
			mcp.Description("Starting point latitude"),
		),
		mcp.WithNumber("start_lon",
			mcp.Required(),
			mcp.Description("Starting point longitude"),
		),
		mcp.WithNumber("end_lat",
			mcp.Required(),
			mcp.Description("Ending point latitude"),
		),
		mcp.WithNumber("end_lon",
			mcp.Required(),
			mcp.Description("Ending point longitude"),
		),
		mcp.WithString("profile",
			mcp.Description("Routing profile (driving, walking, cycling)"),
		),
		mcp.WithBoolean("alternatives",
			mcp.Description("Whether to return alternative routes"),
		),
	)
}

// HandleGetRoute implements route calculation
func HandleGetRoute(ctx context.Context, rawInput mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "get_route")

	// Parse input parameters
	startLat := mcp.ParseFloat64(rawInput, "start_lat", 0)
	startLon := mcp.ParseFloat64(rawInput, "start_lon", 0)
	endLat := mcp.ParseFloat64(rawInput, "end_lat", 0)
	endLon := mcp.ParseFloat64(rawInput, "end_lon", 0)
	profile := mcp.ParseString(rawInput, "profile", "driving")
	alternatives := mcp.ParseBoolean(rawInput, "alternatives", false)

	// Validate parameters
	if startLat < -90 || startLat > 90 || endLat < -90 || endLat > 90 {
		return ErrorResponse("Invalid latitude values"), nil
	}
	if startLon < -180 || startLon > 180 || endLon < -180 || endLon > 180 {
		return ErrorResponse("Invalid longitude values"), nil
	}

	// Build request URL
	reqURL, err := url.Parse(osm.OSRMBaseURL)
	if err != nil {
		logger.Error("failed to parse URL", "error", err)
		return ErrorResponse("Internal server error"), nil
	}

	// Add coordinates to path
	reqURL.Path = fmt.Sprintf("/route/v1/%s/%f,%f;%f,%f",
		profile,
		startLon, startLat,
		endLon, endLat,
	)

	// Add query parameters
	q := reqURL.Query()
	q.Set("overview", "full")
	q.Set("geometries", "geojson")
	q.Set("alternatives", strconv.FormatBool(alternatives))
	q.Set("steps", "true")
	q.Set("annotations", "true")
	reqURL.RawQuery = q.Encode()

	// Make HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		logger.Error("failed to create request", "error", err)
		return ErrorResponse("Failed to create request"), nil
	}

	// Execute request with rate limiting
	resp, err := osm.DoRequest(ctx, req)
	if err != nil {
		logger.Error("failed to execute request", "error", err)
		return ErrorResponse("Failed to communicate with routing service"), nil
	}
	defer resp.Body.Close()

	// Parse response
	var osrmResp OSRMRouteResponse
	if err := json.NewDecoder(resp.Body).Decode(&osrmResp); err != nil {
		logger.Error("failed to decode response", "error", err)
		return ErrorResponse("Failed to parse routing response"), nil
	}

	// Check for errors
	if osrmResp.Code != "Ok" {
		logger.Error("routing service error", "code", osrmResp.Code, "message", osrmResp.Message)
		return ErrorResponse(fmt.Sprintf("Routing service error: %s", osrmResp.Message)), nil
	}

	// Convert OSRM response to our Route type
	if len(osrmResp.Routes) == 0 {
		return ErrorResponse("No route found"), nil
	}

	osrmRoute := osrmResp.Routes[0]
	route := Route{
		Distance: osrmRoute.Distance,
		Duration: osrmRoute.Duration,
		StartPoint: Location{
			Latitude:  startLat,
			Longitude: startLon,
		},
		EndPoint: Location{
			Latitude:  endLat,
			Longitude: endLon,
		},
	}

	// Extract instructions from steps
	for _, leg := range osrmRoute.Legs {
		for _, step := range leg.Steps {
			route.Instructions = append(route.Instructions, step.Name)
		}
	}

	// Return result
	resultBytes, err := json.Marshal(route)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return ErrorResponse("Failed to generate result"), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}
