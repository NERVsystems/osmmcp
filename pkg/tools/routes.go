package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/NERVsystems/osmmcp/pkg/cache"
	"github.com/NERVsystems/osmmcp/pkg/core"
	"github.com/NERVsystems/osmmcp/pkg/osm"
)

// GetRouteDirectionsTool returns a tool definition for getting route directions
func GetRouteDirectionsTool() mcp.Tool {
	return mcp.NewTool("get_route_directions",
		mcp.WithDescription("Get directions for a route between two locations"),
		mcp.WithNumber("start_lat",
			mcp.Required(),
			mcp.Description("The latitude of the starting point"),
		),
		mcp.WithNumber("start_lon",
			mcp.Required(),
			mcp.Description("The longitude of the starting point"),
		),
		mcp.WithNumber("end_lat",
			mcp.Required(),
			mcp.Description("The latitude of the destination"),
		),
		mcp.WithNumber("end_lon",
			mcp.Required(),
			mcp.Description("The longitude of the destination"),
		),
		mcp.WithString("mode",
			mcp.Description("Transportation mode: car, bike, foot"),
			mcp.DefaultString("car"),
		),
	)
}

// RouteDirections represents a calculated route between two points
type RouteDirections struct {
	Distance    float64     `json:"distance"`    // Total distance in meters
	Duration    float64     `json:"duration"`    // Total duration in seconds
	StartPoint  Location    `json:"start_point"` // Starting point
	EndPoint    Location    `json:"end_point"`   // Ending point
	Segments    []Segment   `json:"segments"`    // Route segments
	Coordinates [][]float64 `json:"coordinates"` // Route geometry as [lon, lat] pairs
}

// Segment represents a segment of a route with directions
type Segment struct {
	Distance    float64  `json:"distance"`    // Segment distance in meters
	Duration    float64  `json:"duration"`    // Segment duration in seconds
	Instruction string   `json:"instruction"` // Human-readable instruction
	Location    Location `json:"location"`    // Location of the maneuver
}

// HandleGetRouteDirections gets directions between two points
func HandleGetRouteDirections(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "get_route_directions")

	// Use the common validator for route parameters
	startLat, startLon, endLat, endLon, mode, errResult, err := ValidateRouteParameters(req, logger)
	if err != nil {
		return errResult, nil
	}

	// Map user-friendly mode to OSRM profile
	profile := mapModeToProfile(mode)

	// Check cache first
	cacheKey := fmt.Sprintf("route:%s:%f,%f:%f,%f", profile, startLat, startLon, endLat, endLon)
	if cachedData, found := cache.GetGlobalCache().Get(cacheKey); found {
		logger.Debug("route cache hit", "key", cacheKey)
		result, ok := cachedData.(*mcp.CallToolResult)
		if ok {
			return result, nil
		}
	}

	// Set up coordinates for the OSRM request
	coordinates := [][]float64{
		{startLon, startLat},
		{endLon, endLat},
	}

	// Set up OSRM options
	options := core.OSRMOptions{
		BaseURL:     osm.OSRMBaseURL,
		Profile:     profile,
		Overview:    "full",     // Include full geometry
		Steps:       true,       // Include turn-by-turn instructions
		Annotations: nil,        // No additional annotations
		Geometries:  "polyline", // Use polyline format
		Client:      osm.GetClient(ctx),
		RetryOptions: core.RetryOptions{
			MaxAttempts:  3,
			InitialDelay: 500 * time.Millisecond,
			MaxDelay:     5 * time.Second,
			Multiplier:   2.0,
		},
	}

	// Execute the route request
	route, err := core.GetRoute(ctx, coordinates, options)
	if err != nil {
		logger.Error("failed to get route", "error", err)
		if mcpErr, ok := err.(*core.MCPError); ok {
			return mcpErr.ToMCPResult(), nil
		}
		return core.ServiceError("OSRM", http.StatusServiceUnavailable,
			"Failed to communicate with routing service").ToMCPResult(), nil
	}

	// Check if we have valid route data
	if len(route.Routes) == 0 {
		return core.NewError("ROUTE_NOT_FOUND",
			"No route found between the specified points").ToMCPResult(), nil
	}

	// Get the first route (best match)
	bestRoute := route.Routes[0]

	// Extract and decode the polyline
	polylinePoints := osm.DecodePolyline(bestRoute.Geometry)
	if len(polylinePoints) == 0 {
		logger.Error("failed to decode polyline or empty result")
		return core.NewError("PARSE_ERROR",
			"Failed to decode route geometry").ToMCPResult(), nil
	}

	// Convert coordinates to the expected format
	coordinatesArrays := make([][]float64, len(polylinePoints))
	for i, point := range polylinePoints {
		coordinatesArrays[i] = []float64{point.Longitude, point.Latitude}
	}

	// Build the route segments
	segments := make([]Segment, 0)
	if len(bestRoute.Legs) > 0 {
		for _, step := range bestRoute.Legs[0].Steps {
			// Generate a human-readable instruction
			instruction := generateInstruction(
				step.Maneuver.Type,
				step.Maneuver.Modifier,
				step.Name,
			)

			// Create segment
			segment := Segment{
				Distance:    step.Distance,
				Duration:    step.Duration,
				Instruction: instruction,
				Location: Location{
					Latitude:  step.Maneuver.Location[1],
					Longitude: step.Maneuver.Location[0],
				},
			}

			segments = append(segments, segment)
		}
	}

	// Create route directions response
	directions := RouteDirections{
		Distance: bestRoute.Distance,
		Duration: bestRoute.Duration,
		StartPoint: Location{
			Latitude:  startLat,
			Longitude: startLon,
		},
		EndPoint: Location{
			Latitude:  endLat,
			Longitude: endLon,
		},
		Segments:    segments,
		Coordinates: coordinatesArrays,
	}

	// Create output
	output := struct {
		Directions RouteDirections `json:"directions"`
	}{
		Directions: directions,
	}

	// Marshal to JSON
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return core.NewError("INTERNAL_ERROR", "Failed to generate result").ToMCPResult(), nil
	}

	// Create tool result
	result := mcp.NewToolResultText(string(resultBytes))

	// Cache the result
	cache.GetGlobalCache().Set(cacheKey, result)

	return result, nil
}

// SuggestMeetingPointTool returns a tool definition for suggesting meeting points
func SuggestMeetingPointTool() mcp.Tool {
	return mcp.NewTool("suggest_meeting_point",
		mcp.WithDescription("Suggest optimal meeting points for multiple participants"),
		mcp.WithArray("locations",
			mcp.Required(),
			mcp.Description("Array of participant locations"),
		),
		mcp.WithString("category",
			mcp.Description("Type of meeting point to suggest (restaurant, cafe, etc.)"),
			mcp.DefaultString("restaurant"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of suggestions to return"),
			mcp.DefaultNumber(5),
		),
	)
}

// HandleSuggestMeetingPoint suggests meeting points for multiple participants
func HandleSuggestMeetingPoint(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "suggest_meeting_point")

	// Parse locations from the request using reflection since the structure might be complex
	var locations []struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	}

	// Get the locations parameter and try to extract the values
	locationsParam, err := extractLocations(req)
	if err != nil {
		logger.Error("failed to extract locations", "error", err)
		return ErrorResponse("Failed to parse locations: " + err.Error()), nil
	}
	locations = locationsParam

	// Check if we have at least two locations
	if len(locations) < 2 {
		return ErrorResponse("At least two locations are required"), nil
	}

	// Get other parameters
	category := mcp.ParseString(req, "category", "restaurant")
	limit := int(mcp.ParseFloat64(req, "limit", 5))

	// Calculate the center point (average of all locations)
	var centerLat, centerLon float64
	for _, loc := range locations {
		centerLat += loc.Latitude
		centerLon += loc.Longitude
	}
	centerLat /= float64(len(locations))
	centerLon /= float64(len(locations))

	// Calculate appropriate search radius based on distance between furthest points
	var maxDistance float64
	for _, loc := range locations {
		dist := osm.HaversineDistance(centerLat, centerLon, loc.Latitude, loc.Longitude)
		if dist > maxDistance {
			maxDistance = dist
		}
	}

	// If participants are extremely far apart (> 50km), return an error
	const maxAllowedDistance = 50000.0 // 50km
	if maxDistance > maxAllowedDistance {
		logger.Error("participants too far apart", "max_distance", maxDistance)
		return ErrorWithGuidance(&APIError{
			Service:     "Meeting Point",
			StatusCode:  http.StatusBadRequest,
			Message:     fmt.Sprintf("Participants are too far apart (%.1f km)", maxDistance/1000),
			Guidance:    "Meeting points can only be suggested when participants are within 50km of each other",
			Recoverable: false,
		}), nil
	}

	// Set radius to max distance + 1000m, with minimum of 1000m and maximum of 5000m
	radius := math.Min(math.Max(maxDistance+1000, 1000), 5000)

	// Create a simulated request to pass to FindNearbyPlaces
	// We're directly calling the function, so we create a new params object
	paramMap := make(map[string]interface{})
	paramMap["latitude"] = centerLat
	paramMap["longitude"] = centerLon
	paramMap["radius"] = radius
	paramMap["category"] = category
	paramMap["limit"] = float64(limit)

	// Use reflection to create a new CallToolRequest with our parameters
	simReq := mcp.CallToolRequest{}
	simReq.Params.Name = "find_nearby_places"
	simReq.Params.Arguments = paramMap

	// Call the HandleFindNearbyPlaces function directly
	result, err := HandleFindNearbyPlaces(ctx, simReq)
	if err != nil {
		logger.Error("failed to find nearby places", "error", err)
		return ErrorResponse("Failed to find meeting points"), nil
	}

	// Extract the text content from the result
	var contentText string
	for _, content := range result.Content {
		if text, ok := content.(mcp.TextContent); ok {
			contentText = text.Text
			break
		}
	}

	if contentText == "" {
		logger.Error("no text content in result")
		return ErrorResponse("Failed to process meeting points"), nil
	}

	// Parse the result to get the places
	var placesOutput struct {
		Places []Place `json:"places"`
	}

	if err := json.Unmarshal([]byte(contentText), &placesOutput); err != nil {
		logger.Error("failed to parse places result", "error", err)
		return ErrorResponse("Failed to process meeting points"), nil
	}

	// For each place, calculate the total distance from all participants
	type ScoredPlace struct {
		Place           Place   `json:"place"`
		TotalDistance   float64 `json:"total_distance"`
		AverageDistance float64 `json:"average_distance"`
	}

	scoredPlaces := make([]ScoredPlace, 0, len(placesOutput.Places))
	for _, place := range placesOutput.Places {
		var totalDistance float64
		for _, loc := range locations {
			dist := osm.HaversineDistance(
				place.Location.Latitude, place.Location.Longitude,
				loc.Latitude, loc.Longitude,
			)
			totalDistance += dist
		}

		scoredPlaces = append(scoredPlaces, ScoredPlace{
			Place:           place,
			TotalDistance:   totalDistance,
			AverageDistance: totalDistance / float64(len(locations)),
		})
	}

	// Sort by average distance (closest first)
	sort.Slice(scoredPlaces, func(i, j int) bool {
		return scoredPlaces[i].AverageDistance < scoredPlaces[j].AverageDistance
	})

	// Create output
	output := struct {
		MeetingPoints []struct {
			Place           Place   `json:"place"`
			AverageDistance float64 `json:"average_distance"`
		} `json:"meeting_points"`
		CenterPoint Location `json:"center_point"`
	}{
		CenterPoint: Location{
			Latitude:  centerLat,
			Longitude: centerLon,
		},
		MeetingPoints: make([]struct {
			Place           Place   `json:"place"`
			AverageDistance float64 `json:"average_distance"`
		}, 0, limit),
	}

	// Add meeting points to output
	maxResults := int(math.Min(float64(len(scoredPlaces)), float64(limit)))
	for i := 0; i < maxResults; i++ {
		output.MeetingPoints = append(output.MeetingPoints, struct {
			Place           Place   `json:"place"`
			AverageDistance float64 `json:"average_distance"`
		}{
			Place:           scoredPlaces[i].Place,
			AverageDistance: scoredPlaces[i].AverageDistance,
		})
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return ErrorResponse("Failed to generate result"), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}

// extractLocations extracts the location array from the CallToolRequest
func extractLocations(req mcp.CallToolRequest) ([]struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}, error) {
	var locations []struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	}

	// Get arguments using the SDK helper method
	args := req.GetArguments()
	if args == nil {
		return nil, core.NewError(core.ErrMissingParameter, "no arguments provided").
			WithGuidance("The locations parameter is required and must be an array of waypoints")
	}

	// Convert the locations parameter to JSON
	locationsRaw, ok := args["locations"]
	if !ok {
		return nil, core.NewError(core.ErrMissingParameter, "missing required locations parameter").
			WithGuidance("The locations parameter is required and must be an array of waypoints")
	}

	// Marshal and unmarshal to convert to our struct
	locationsJSON, err := json.Marshal(locationsRaw)
	if err != nil {
		return nil, core.NewError(core.ErrInternalError, "failed to marshal locations").
			WithGuidance("Unable to process the locations parameter")
	}

	if err := json.Unmarshal(locationsJSON, &locations); err != nil {
		return nil, core.NewError(core.ErrParseError, "failed to parse locations array").
			WithGuidance("The locations parameter must be a valid JSON array of waypoint objects")
	}

	return locations, nil
}

// mapModeToProfile maps a transportation mode to an OSRM profile
func mapModeToProfile(mode string) string {
	mode = strings.ToLower(mode)
	switch mode {
	case "bike", "bicycle":
		return "bike"
	case "foot", "walk", "walking":
		return "foot"
	default:
		return "car" // Default to car
	}
}

// generateInstruction creates a human-readable instruction from OSRM maneuver
func generateInstruction(maneuverType, modifier, roadName string) string {
	if roadName == "" {
		roadName = "the road"
	} else {
		roadName = "onto " + roadName
	}

	switch maneuverType {
	case "depart":
		return "Start your journey"
	case "arrive":
		return "You have arrived at your destination"
	case "turn":
		return fmt.Sprintf("Turn %s %s", modifier, roadName)
	case "continue":
		return fmt.Sprintf("Continue straight %s", roadName)
	case "roundabout":
		return fmt.Sprintf("Enter the roundabout and take the %s exit", modifier)
	case "merge":
		return fmt.Sprintf("Merge %s", roadName)
	case "fork":
		return fmt.Sprintf("Take the %s fork", modifier)
	default:
		if modifier != "" {
			return fmt.Sprintf("%s %s", modifier, roadName)
		}
		return fmt.Sprintf("Continue %s", roadName)
	}
}
