// Package tools provides the OpenStreetMap MCP tools implementations.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/NERVsystems/osmmcp/pkg/osm"
	"github.com/mark3labs/mcp-go/mcp"
)

// ChargingStation represents an EV charging station
type ChargingStation struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Location    Location `json:"location"`
	Distance    float64  `json:"distance,omitempty"` // in meters
	Operator    string   `json:"operator,omitempty"`
	SocketTypes []string `json:"socket_types,omitempty"`
	Power       string   `json:"power,omitempty"` // max power in kW
	Access      string   `json:"access,omitempty"`
	Fee         bool     `json:"fee,omitempty"`
}

// RouteChargingStation extends ChargingStation with route-specific information
type RouteChargingStation struct {
	ChargingStation
	DistanceFromStart float64 `json:"distance_from_start"` // in meters
	PercentAlongRoute float64 `json:"percent_along_route"` // 0-100
}

// FindChargingStationsTool returns a tool definition for finding EV charging stations
func FindChargingStationsTool() mcp.Tool {
	return mcp.NewTool("find_charging_stations",
		mcp.WithDescription("Find electric vehicle charging stations near a specific location"),
		mcp.WithNumber("latitude",
			mcp.Required(),
			mcp.Description("The latitude coordinate of the center point"),
		),
		mcp.WithNumber("longitude",
			mcp.Required(),
			mcp.Description("The longitude coordinate of the center point"),
		),
		mcp.WithNumber("radius",
			mcp.Description("Search radius in meters (max 10000)"),
			mcp.DefaultNumber(5000),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results to return"),
			mcp.DefaultNumber(10),
		),
	)
}

// HandleFindChargingStations implements finding charging stations
func HandleFindChargingStations(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "find_charging_stations")

	// Parse and validate coordinates
	latStr := mcp.ParseString(req, "latitude", "")
	lonStr := mcp.ParseString(req, "longitude", "")
	radiusStr := mcp.ParseString(req, "radius", "")
	limitStr := mcp.ParseString(req, "limit", "")

	if latStr == "" || lonStr == "" {
		logger.Error("missing required coordinates", "latitude", latStr, "longitude", lonStr)
		return NewGeocodeDetailedError(
			"MISSING_COORDINATES",
			"Missing required coordinates",
			"",
			"The find_charging_stations tool requires both latitude and longitude parameters",
			"Example format: {\"latitude\": 40.7128, \"longitude\": -74.0060, \"radius\": 1000}",
		), nil
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		logger.Error("invalid latitude", "input", latStr, "error", err)
		return NewGeocodeDetailedError(
			"INVALID_LATITUDE",
			fmt.Sprintf("Invalid latitude value: %s", latStr),
			"",
			"Latitude must be a valid number between -90 and 90",
			"Example: 40.7128 (numeric, no quotes)",
		), nil
	}

	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		logger.Error("invalid longitude", "input", lonStr, "error", err)
		return NewGeocodeDetailedError(
			"INVALID_LONGITUDE",
			fmt.Sprintf("Invalid longitude value: %s", lonStr),
			"",
			"Longitude must be a valid number between -180 and 180",
			"Example: -74.0060 (numeric, no quotes)",
		), nil
	}

	if err := ValidateCoordinates(lat, lon); err != nil {
		logger.Error("coordinate validation failed", "error", err)
		return NewGeocodeDetailedError(
			"INVALID_COORDINATES",
			err.Error(),
			"",
			"Latitude must be between -90 and 90, longitude between -180 and 180",
		), nil
	}

	var radius float64 = 1000 // Default radius
	if radiusStr != "" {
		radius, err = strconv.ParseFloat(radiusStr, 64)
		if err != nil {
			logger.Error("invalid radius", "input", radiusStr, "error", err)
			return NewGeocodeDetailedError(
				"INVALID_RADIUS",
				fmt.Sprintf("Invalid radius value: %s", radiusStr),
				"",
				"Radius must be a valid positive number",
				"Example: 1000 (numeric, no quotes)",
			), nil
		}
	}

	if err := ValidateRadius(radius, 5000); err != nil {
		logger.Error("radius validation failed", "radius", radius, "error", err)
		return NewGeocodeDetailedError(
			"INVALID_RADIUS",
			err.Error(),
			"",
			"Radius must be positive and less than 5000 meters",
		), nil
	}

	var limit int = 10 // Default limit
	if limitStr != "" {
		limitFloat, err := strconv.ParseFloat(limitStr, 64)
		if err != nil {
			logger.Error("invalid limit", "input", limitStr, "error", err)
			return NewGeocodeDetailedError(
				"INVALID_LIMIT",
				fmt.Sprintf("Invalid limit value: %s", limitStr),
				"",
				"Limit must be a valid positive number",
				"Example: 10 (numeric, no quotes)",
			), nil
		}
		limit = int(limitFloat)
	}

	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	// Build Overpass query for charging stations
	var queryBuilder strings.Builder
	queryBuilder.WriteString("[out:json];")
	queryBuilder.WriteString(fmt.Sprintf("(node(around:%f,%f,%f)[amenity=charging_station];", radius, lat, lon))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[amenity=charging_station];", radius, lat, lon))
	queryBuilder.WriteString(");out body;")

	// Build request
	reqURL, err := url.Parse(osm.OverpassBaseURL)
	if err != nil {
		logger.Error("failed to parse URL", "error", err)
		return ErrorResponse("Internal server error"), nil
	}

	// Make HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL.String(), strings.NewReader("data="+url.QueryEscape(queryBuilder.String())))
	if err != nil {
		logger.Error("failed to create request", "error", err)
		return ErrorResponse("Failed to create request"), nil
	}

	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("User-Agent", osm.UserAgent)

	// Execute request
	client := osm.GetClient(ctx)
	resp, err := client.Do(httpReq)
	if err != nil {
		logger.Error("failed to execute request", "error", err)
		return ErrorResponse("Failed to communicate with OSM service"), nil
	}
	defer resp.Body.Close()

	// Process response
	if resp.StatusCode != http.StatusOK {
		logger.Error("OSM service returned error", "status", resp.StatusCode)
		return ErrorResponse(fmt.Sprintf("OSM service error: %d", resp.StatusCode)), nil
	}

	// Parse response
	var overpassResp struct {
		Elements []struct {
			ID   int               `json:"id"`
			Type string            `json:"type"`
			Lat  float64           `json:"lat,omitempty"`
			Lon  float64           `json:"lon,omitempty"`
			Tags map[string]string `json:"tags"`
		} `json:"elements"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&overpassResp); err != nil {
		logger.Error("failed to decode response", "error", err)
		return ErrorResponse("Failed to parse charging stations data"), nil
	}

	// Convert to ChargingStation objects and calculate distances
	stations := make([]ChargingStation, 0)
	for _, element := range overpassResp.Elements {
		// Skip elements without proper coordinates
		if element.Lat == 0 && element.Lon == 0 {
			continue
		}

		// Calculate distance
		distance := osm.HaversineDistance(
			lat, lon,
			element.Lat, element.Lon,
		)

		// Extract socket types
		socketTypes := make([]string, 0)
		for key, value := range element.Tags {
			if strings.HasPrefix(key, "socket:") {
				if value == "yes" {
					socketType := strings.TrimPrefix(key, "socket:")
					socketTypes = append(socketTypes, socketType)
				}
			}
		}

		// Create station object
		station := ChargingStation{
			ID:   fmt.Sprintf("%d", element.ID),
			Name: getStationName(element.Tags),
			Location: Location{
				Latitude:  element.Lat,
				Longitude: element.Lon,
			},
			Distance:    distance,
			Operator:    element.Tags["operator"],
			SocketTypes: socketTypes,
			Power:       element.Tags["maxpower"],
			Access:      element.Tags["access"],
			Fee:         element.Tags["fee"] == "yes",
		}

		stations = append(stations, station)
	}

	// Sort stations by distance (closest first)
	sort.Slice(stations, func(i, j int) bool {
		return stations[i].Distance < stations[j].Distance
	})

	// Limit results
	if len(stations) > limit {
		stations = stations[:limit]
	}

	// Create output
	output := struct {
		ChargingStations []ChargingStation `json:"charging_stations"`
	}{
		ChargingStations: stations,
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return ErrorResponse("Failed to generate result"), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}

// getStationName returns a name for the charging station
func getStationName(tags map[string]string) string {
	// Use name tag if available
	if name, ok := tags["name"]; ok && name != "" {
		return name
	}

	// Use operator tag as fallback
	if operator, ok := tags["operator"]; ok && operator != "" {
		return fmt.Sprintf("%s Charging Station", operator)
	}

	// Default generic name
	return "EV Charging Station"
}

// FindRouteChargingStationsTool returns a tool definition for finding charging stations along a route
func FindRouteChargingStationsTool() mcp.Tool {
	return mcp.NewTool("find_route_charging_stations",
		mcp.WithDescription("Find electric vehicle charging stations along a route between two locations"),
		mcp.WithNumber("start_latitude",
			mcp.Required(),
			mcp.Description("The latitude coordinate of the starting point"),
		),
		mcp.WithNumber("start_longitude",
			mcp.Required(),
			mcp.Description("The longitude coordinate of the starting point"),
		),
		mcp.WithNumber("end_latitude",
			mcp.Required(),
			mcp.Description("The latitude coordinate of the destination"),
		),
		mcp.WithNumber("end_longitude",
			mcp.Required(),
			mcp.Description("The longitude coordinate of the destination"),
		),
		mcp.WithNumber("buffer_distance",
			mcp.Description("Distance in meters to search on either side of the route (max 5000)"),
			mcp.DefaultNumber(2000),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results to return"),
			mcp.DefaultNumber(10),
		),
	)
}

// HandleFindRouteChargingStations implements finding charging stations along a route
func HandleFindRouteChargingStations(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "find_route_charging_stations")

	// Parse input parameters
	startLat := mcp.ParseFloat64(req, "start_latitude", 0)
	startLon := mcp.ParseFloat64(req, "start_longitude", 0)
	endLat := mcp.ParseFloat64(req, "end_latitude", 0)
	endLon := mcp.ParseFloat64(req, "end_longitude", 0)
	bufferDistance := mcp.ParseFloat64(req, "buffer_distance", 2000)
	limit := int(mcp.ParseFloat64(req, "limit", 10))

	// Basic validation
	if startLat < -90 || startLat > 90 || endLat < -90 || endLat > 90 {
		return ErrorResponse("Latitude must be between -90 and 90"), nil
	}
	if startLon < -180 || startLon > 180 || endLon < -180 || endLon > 180 {
		return ErrorResponse("Longitude must be between -180 and 180"), nil
	}
	if bufferDistance <= 0 || bufferDistance > 5000 {
		return ErrorResponse("Buffer distance must be between 1 and 5000 meters"), nil
	}
	if limit <= 0 {
		limit = 10 // Default limit
	}
	if limit > 50 {
		limit = 50 // Max limit
	}

	// First, get the route between the two points using OSRM
	osrmURL := fmt.Sprintf("%s/route/v1/driving/%f,%f;%f,%f",
		osm.OSRMBaseURL, startLon, startLat, endLon, endLat)
	reqURL, err := url.Parse(osrmURL)
	if err != nil {
		logger.Error("failed to parse URL", "error", err)
		return ErrorResponse("Internal server error"), nil
	}

	// Add query parameters for OSRM
	q := reqURL.Query()
	q.Add("overview", "full")
	q.Add("geometries", "geojson")
	reqURL.RawQuery = q.Encode()

	// Make HTTP request to OSRM
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		logger.Error("failed to create request", "error", err)
		return ErrorResponse("Failed to create route request"), nil
	}

	httpReq.Header.Set("User-Agent", osm.UserAgent)

	// Execute request
	client := osm.GetClient(ctx)
	resp, err := client.Do(httpReq)
	if err != nil {
		logger.Error("failed to execute request", "error", err)
		return ErrorResponse("Failed to communicate with routing service"), nil
	}
	defer resp.Body.Close()

	// Process response
	if resp.StatusCode != http.StatusOK {
		logger.Error("routing service returned error", "status", resp.StatusCode)
		return ErrorResponse(fmt.Sprintf("Routing service error: %d", resp.StatusCode)), nil
	}

	// Parse OSRM response
	var osrmResp struct {
		Routes []struct {
			Distance float64 `json:"distance"`
			Duration float64 `json:"duration"`
			Geometry struct {
				Coordinates [][]float64 `json:"coordinates"` // [lon, lat] format in GeoJSON
			} `json:"geometry"`
		} `json:"routes"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&osrmResp); err != nil {
		logger.Error("failed to decode response", "error", err)
		return ErrorResponse("Failed to parse routing data"), nil
	}

	// Check if we have any routes
	if len(osrmResp.Routes) == 0 {
		return ErrorResponse("No route found between the specified points"), nil
	}

	// Get the first route
	route := osrmResp.Routes[0]

	// Convert route coordinates to [lat, lon] format (OSRM returns [lon, lat])
	routeCoords := make([]Location, 0, len(route.Geometry.Coordinates))
	for _, coord := range route.Geometry.Coordinates {
		if len(coord) >= 2 {
			routeCoords = append(routeCoords, Location{
				Latitude:  coord[1],
				Longitude: coord[0],
			})
		}
	}

	// Create a bounding box for the route
	bbox := osm.NewBoundingBox()
	for _, coord := range routeCoords {
		bbox.ExtendWithPoint(coord.Latitude, coord.Longitude)
	}

	// Add buffer to bounding box
	bbox.Buffer(bufferDistance)

	// Build Overpass query for charging stations in bounding box
	var queryBuilder strings.Builder
	queryBuilder.WriteString("[out:json];")
	queryBuilder.WriteString(fmt.Sprintf("(node%s[amenity=charging_station];", bbox.String()))
	queryBuilder.WriteString(fmt.Sprintf("way%s[amenity=charging_station];", bbox.String()))
	queryBuilder.WriteString(");out body;")

	// Build request for Overpass
	reqURL, err = url.Parse(osm.OverpassBaseURL)
	if err != nil {
		logger.Error("failed to parse URL", "error", err)
		return ErrorResponse("Internal server error"), nil
	}

	// Make HTTP request to Overpass
	httpReq, err = http.NewRequestWithContext(ctx, http.MethodPost, reqURL.String(),
		strings.NewReader("data="+url.QueryEscape(queryBuilder.String())))
	if err != nil {
		logger.Error("failed to create request", "error", err)
		return ErrorResponse("Failed to create request"), nil
	}

	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("User-Agent", osm.UserAgent)

	// Execute request
	resp, err = client.Do(httpReq)
	if err != nil {
		logger.Error("failed to execute request", "error", err)
		return ErrorResponse("Failed to communicate with OSM service"), nil
	}
	defer resp.Body.Close()

	// Process response
	if resp.StatusCode != http.StatusOK {
		logger.Error("OSM service returned error", "status", resp.StatusCode)
		return ErrorResponse(fmt.Sprintf("OSM service error: %d", resp.StatusCode)), nil
	}

	// Parse Overpass response
	var overpassResp struct {
		Elements []struct {
			ID   int               `json:"id"`
			Type string            `json:"type"`
			Lat  float64           `json:"lat,omitempty"`
			Lon  float64           `json:"lon,omitempty"`
			Tags map[string]string `json:"tags"`
		} `json:"elements"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&overpassResp); err != nil {
		logger.Error("failed to decode response", "error", err)
		return ErrorResponse("Failed to parse charging stations data"), nil
	}

	// Process charging stations
	routeStations := make([]RouteChargingStation, 0)
	totalRouteDistance := route.Distance // meters

	for _, element := range overpassResp.Elements {
		// Skip elements without proper coordinates
		if element.Lat == 0 && element.Lon == 0 {
			continue
		}

		// Find distance to closest point on route
		minDistToRoute := math.MaxFloat64
		distFromStart := 0.0

		// For each station, find its closest point on the route
		stationLoc := Location{Latitude: element.Lat, Longitude: element.Lon}

		// Simple but not super efficient algorithm to find closest point on route
		for i := 0; i < len(routeCoords); i++ {
			dist := osm.HaversineDistance(stationLoc.Latitude, stationLoc.Longitude,
				routeCoords[i].Latitude, routeCoords[i].Longitude)

			if dist < minDistToRoute {
				minDistToRoute = dist

				// Calculate approximate distance from start to this point on route
				if i > 0 {
					for j := 0; j < i; j++ {
						distFromStart += osm.HaversineDistance(
							routeCoords[j].Latitude, routeCoords[j].Longitude,
							routeCoords[j+1].Latitude, routeCoords[j+1].Longitude)
					}
				}
			}
		}

		// Skip stations too far from route
		if minDistToRoute > bufferDistance {
			continue
		}

		// Extract socket types
		socketTypes := make([]string, 0)
		for key, value := range element.Tags {
			if strings.HasPrefix(key, "socket:") {
				if value == "yes" {
					socketType := strings.TrimPrefix(key, "socket:")
					socketTypes = append(socketTypes, socketType)
				}
			}
		}

		// Create station object
		routeStation := RouteChargingStation{
			ChargingStation: ChargingStation{
				ID:   fmt.Sprintf("%d", element.ID),
				Name: getStationName(element.Tags),
				Location: Location{
					Latitude:  element.Lat,
					Longitude: element.Lon,
				},
				Distance:    minDistToRoute,
				Operator:    element.Tags["operator"],
				SocketTypes: socketTypes,
				Power:       element.Tags["maxpower"],
				Access:      element.Tags["access"],
				Fee:         element.Tags["fee"] == "yes",
			},
			DistanceFromStart: distFromStart,
			PercentAlongRoute: (distFromStart / totalRouteDistance) * 100,
		}

		routeStations = append(routeStations, routeStation)
	}

	// Sort stations by distance along route
	sort.Slice(routeStations, func(i, j int) bool {
		return routeStations[i].DistanceFromStart < routeStations[j].DistanceFromStart
	})

	// Limit results
	if len(routeStations) > limit {
		routeStations = routeStations[:limit]
	}

	// Create output
	output := struct {
		RouteDistance    float64                `json:"route_distance"`
		RouteDuration    float64                `json:"route_duration"`
		ChargingStations []RouteChargingStation `json:"charging_stations"`
	}{
		RouteDistance:    route.Distance,
		RouteDuration:    route.Duration,
		ChargingStations: routeStations,
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return ErrorResponse("Failed to generate result"), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}
