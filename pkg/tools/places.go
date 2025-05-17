// Package tools provides the OpenStreetMap MCP tools implementations.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/NERVsystems/osmmcp/pkg/osm"
	"github.com/mark3labs/mcp-go/mcp"
)

// FindNearbyPlacesTool returns a tool definition for finding nearby places
func FindNearbyPlacesTool() mcp.Tool {
	return mcp.NewTool("find_nearby_places",
		mcp.WithDescription("Find points of interest near a specific location"),
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
			mcp.DefaultNumber(1000),
		),
		mcp.WithString("category",
			mcp.Description("Optional category filter (e.g., restaurant, hotel, park)"),
			mcp.DefaultString(""),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results to return"),
			mcp.DefaultNumber(10),
		),
	)
}

// HandleFindNearbyPlaces implements finding nearby POIs
func HandleFindNearbyPlaces(ctx context.Context, rawInput mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "find_nearby_places")

	// Parse input parameters
	latitude := mcp.ParseFloat64(rawInput, "latitude", 0)
	longitude := mcp.ParseFloat64(rawInput, "longitude", 0)
	radius := mcp.ParseFloat64(rawInput, "radius", 1000)
	category := mcp.ParseString(rawInput, "category", "")
	limit := int(mcp.ParseFloat64(rawInput, "limit", 10))

	// Basic validation
	if latitude < -90 || latitude > 90 {
		return ErrorResponse("Latitude must be between -90 and 90"), nil
	}
	if longitude < -180 || longitude > 180 {
		return ErrorResponse("Longitude must be between -180 and 180"), nil
	}
	if radius <= 0 || radius > 10000 {
		return ErrorResponse("Radius must be between 1 and 10000 meters"), nil
	}
	if limit <= 0 {
		limit = 10 // Default limit
	}
	if limit > 50 {
		limit = 50 // Max limit
	}

	// Map generic categories to OSM tags
	osmTags := mapCategoryToOSMTags(category)

	// Build Overpass query
	var queryBuilder strings.Builder
	queryBuilder.WriteString("[out:json];")
	queryBuilder.WriteString(fmt.Sprintf("(node(around:%f,%f,%f)", radius, latitude, longitude))

	// Add tag filters if category specified
	if len(osmTags) > 0 {
		for key, values := range osmTags {
			for _, value := range values {
				queryBuilder.WriteString(fmt.Sprintf("[%s=%s]", key, value))
			}
		}
	}

	// Complete the query
	queryBuilder.WriteString(";);out body;")

	// Build request
	reqURL, err := url.Parse(osm.OverpassBaseURL)
	if err != nil {
		logger.Error("failed to parse URL", "error", err)
		return ErrorResponse("Internal server error"), nil
	}

	// Make HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL.String(), strings.NewReader("data="+url.QueryEscape(queryBuilder.String())))
	if err != nil {
		logger.Error("failed to create request", "error", err)
		return ErrorResponse("Failed to create request"), nil
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Execute request with rate limiting
	resp, err := osm.DoRequest(ctx, req)
	if err != nil {
		logger.Error("failed to execute request", "error", err)
		return ErrorResponse("Failed to communicate with places service"), nil
	}
	defer resp.Body.Close()

	// Process response
	if resp.StatusCode != http.StatusOK {
		logger.Error("places service returned error", "status", resp.StatusCode)
		return ErrorResponse(fmt.Sprintf("Places service error: %d", resp.StatusCode)), nil
	}

	// Parse response
	var overpassResp struct {
		Elements []struct {
			ID   int     `json:"id"`
			Type string  `json:"type"`
			Lat  float64 `json:"lat"`
			Lon  float64 `json:"lon"`
			Tags struct {
				Name     string `json:"name"`
				Amenity  string `json:"amenity"`
				Shop     string `json:"shop"`
				Tourism  string `json:"tourism"`
				Leisure  string `json:"leisure"`
				Highway  string `json:"highway"`
				Building string `json:"building"`
			} `json:"tags"`
		} `json:"elements"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&overpassResp); err != nil {
		logger.Error("failed to decode response", "error", err)
		return ErrorResponse("Failed to parse places response"), nil
	}

	// Convert to Place objects and calculate distances
	places := make([]Place, 0)
	for _, element := range overpassResp.Elements {
		// Skip elements without a name
		if element.Tags.Name == "" {
			continue
		}

		// Calculate distance
		distance := osm.HaversineDistance(
			latitude, longitude,
			element.Lat, element.Lon,
		)

		// Determine place category
		categories := []string{}
		if element.Tags.Amenity != "" {
			categories = append(categories, element.Tags.Amenity)
		}
		if element.Tags.Shop != "" {
			categories = append(categories, "shop:"+element.Tags.Shop)
		}
		if element.Tags.Tourism != "" {
			categories = append(categories, "tourism:"+element.Tags.Tourism)
		}
		if element.Tags.Leisure != "" {
			categories = append(categories, "leisure:"+element.Tags.Leisure)
		}

		// Create place object
		place := Place{
			ID:   strconv.Itoa(element.ID),
			Name: element.Tags.Name,
			Location: Location{
				Latitude:  element.Lat,
				Longitude: element.Lon,
			},
			Categories: categories,
			Distance:   distance,
		}

		places = append(places, place)
	}

	// Sort places by distance (closest first)
	sortPlacesByDistance(places)

	// Limit results
	if len(places) > limit {
		places = places[:limit]
	}

	// Create output
	output := struct {
		Places []Place `json:"places"`
	}{
		Places: places,
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return ErrorResponse("Failed to generate result"), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}

// mapCategoryToOSMTags maps generic category names to OSM tag combinations
func mapCategoryToOSMTags(category string) map[string][]string {
	// Convert to lowercase for case-insensitive matching
	category = strings.ToLower(category)

	// Common category mappings
	switch category {
	case "restaurant", "restaurants", "dining":
		return map[string][]string{
			"amenity": {"restaurant", "cafe", "fast_food", "bar", "pub", "food_court"},
		}
	case "park", "parks", "recreation":
		return map[string][]string{
			"leisure": {"park", "garden", "playground", "nature_reserve"},
			"landuse": {"recreation_ground", "park", "greenfield"},
			"natural": {"wood", "grassland", "meadow"},
		}
	case "hotel", "hotels", "lodging", "accommodation":
		return map[string][]string{
			"tourism": {"hotel", "hostel", "guest_house", "motel", "apartment", "resort"},
		}
	case "school", "schools", "education":
		return map[string][]string{
			"amenity": {"school", "kindergarten", "university", "college", "language_school", "music_school", "driving_school"},
		}
	case "bank", "banks", "atm", "atms", "finance":
		return map[string][]string{
			"amenity": {"bank", "atm", "bureau_de_change", "money_transfer"},
		}
	case "shop", "shops", "store", "stores", "shopping":
		return map[string][]string{
			"shop": {"*"}, // Match any shop type
		}
	case "cafe", "cafes", "coffee", "tea":
		return map[string][]string{
			"amenity": {"cafe", "ice_cream"},
			"shop":    {"coffee", "tea"},
		}
	case "hospital", "hospitals", "medical", "healthcare":
		return map[string][]string{
			"amenity": {"hospital", "clinic", "doctors", "dentist", "pharmacy", "healthcare"},
		}
	case "pharmacy", "pharmacies", "drugstore", "chemist":
		return map[string][]string{
			"amenity": {"pharmacy"},
			"shop":    {"chemist", "drugstore", "medical_supply"},
		}
	case "supermarket", "grocery", "market", "food_shop":
		return map[string][]string{
			"shop": {"supermarket", "convenience", "grocery", "greengrocer", "butcher", "bakery", "deli"},
		}
	case "museum", "museums", "gallery", "galleries", "art":
		return map[string][]string{
			"tourism": {"museum", "gallery", "artwork"},
			"amenity": {"arts_centre"},
		}
	case "attraction", "attractions", "tourist", "tourism", "sightseeing":
		return map[string][]string{
			"tourism": {"attraction", "viewpoint", "information", "museum", "gallery", "theme_park", "zoo"},
		}
	case "transport", "transportation", "transit", "bus", "train", "station":
		return map[string][]string{
			"public_transport": {"station", "stop_position", "platform"},
			"railway":          {"station", "halt", "tram_stop", "subway_entrance"},
			"amenity":          {"bus_station", "ferry_terminal", "taxi"},
			"highway":          {"bus_stop"},
		}
	default:
		// Split the category by spaces to handle multi-word categories
		parts := strings.Fields(category)
		if len(parts) > 1 {
			// Try matching a more specific multi-word category
			compound := strings.Join(parts, "_")
			return map[string][]string{
				"amenity":  {compound, category},
				"shop":     {compound, category},
				"tourism":  {compound, category},
				"leisure":  {compound, category},
				"natural":  {compound, category},
				"historic": {compound, category},
			}
		}

		// For unknown categories, try multiple tag combinations
		return map[string][]string{
			"amenity":  {category},
			"shop":     {category},
			"tourism":  {category},
			"leisure":  {category},
			"natural":  {category},
			"historic": {category},
		}
	}
}

// sortPlacesByDistance sorts places by distance (closest first)
func sortPlacesByDistance(places []Place) {
	// Replace bubble sort with sort.Slice for better performance
	sort.Slice(places, func(i, j int) bool {
		return places[i].Distance < places[j].Distance
	})
}

// SearchCategoryTool returns a tool definition for searching places by category
func SearchCategoryTool() mcp.Tool {
	return mcp.NewTool("search_category",
		mcp.WithDescription("Find places of a specific category within a bounding box"),
		mcp.WithString("category",
			mcp.Required(),
			mcp.Description("Category to search for (e.g., restaurant, hotel, park)"),
		),
		mcp.WithNumber("north_lat",
			mcp.Required(),
			mcp.Description("Northern boundary latitude"),
		),
		mcp.WithNumber("south_lat",
			mcp.Required(),
			mcp.Description("Southern boundary latitude"),
		),
		mcp.WithNumber("east_lon",
			mcp.Required(),
			mcp.Description("Eastern boundary longitude"),
		),
		mcp.WithNumber("west_lon",
			mcp.Required(),
			mcp.Description("Western boundary longitude"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results to return"),
			mcp.DefaultNumber(20),
		),
	)
}

// HandleSearchCategory implements category search functionality
func HandleSearchCategory(ctx context.Context, rawInput mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "search_category")

	// Parse input parameters
	category := mcp.ParseString(rawInput, "category", "")
	northLat := mcp.ParseFloat64(rawInput, "north_lat", 0)
	southLat := mcp.ParseFloat64(rawInput, "south_lat", 0)
	eastLon := mcp.ParseFloat64(rawInput, "east_lon", 0)
	westLon := mcp.ParseFloat64(rawInput, "west_lon", 0)
	limit := int(mcp.ParseFloat64(rawInput, "limit", 20))

	// Basic validation
	if category == "" {
		return ErrorResponse("Category must not be empty"), nil
	}
	if northLat < southLat {
		return ErrorResponse("North latitude must be greater than south latitude"), nil
	}
	if northLat < -90 || northLat > 90 || southLat < -90 || southLat > 90 {
		return ErrorResponse("Latitude must be between -90 and 90"), nil
	}
	if eastLon < -180 || eastLon > 180 || westLon < -180 || westLon > 180 {
		return ErrorResponse("Longitude must be between -180 and 180"), nil
	}
	if limit <= 0 {
		limit = 20 // Default limit
	}
	if limit > 100 {
		limit = 100 // Max limit
	}

	// Map generic categories to OSM tags
	osmTags := mapCategoryToOSMTags(category)

	// Build Overpass query
	var queryBuilder strings.Builder
	queryBuilder.WriteString("[out:json];")
	queryBuilder.WriteString("(")

	// Include nodes, ways, and relations in the bounding box
	queryBuilder.WriteString(fmt.Sprintf("node(%f,%f,%f,%f)", southLat, westLon, northLat, eastLon))

	// Add tag filters for nodes
	for key, values := range osmTags {
		for _, value := range values {
			if value == "*" {
				// Special case: use any value for this key
				queryBuilder.WriteString(fmt.Sprintf("[%s]", key))
			} else {
				queryBuilder.WriteString(fmt.Sprintf("[%s=%s]", key, value))
			}
		}
	}
	queryBuilder.WriteString(";")

	// Add ways with the same tags
	queryBuilder.WriteString(fmt.Sprintf("way(%f,%f,%f,%f)", southLat, westLon, northLat, eastLon))
	for key, values := range osmTags {
		for _, value := range values {
			if value == "*" {
				queryBuilder.WriteString(fmt.Sprintf("[%s]", key))
			} else {
				queryBuilder.WriteString(fmt.Sprintf("[%s=%s]", key, value))
			}
		}
	}
	queryBuilder.WriteString(";")

	// Add relations with the same tags
	queryBuilder.WriteString(fmt.Sprintf("relation(%f,%f,%f,%f)", southLat, westLon, northLat, eastLon))
	for key, values := range osmTags {
		for _, value := range values {
			if value == "*" {
				queryBuilder.WriteString(fmt.Sprintf("[%s]", key))
			} else {
				queryBuilder.WriteString(fmt.Sprintf("[%s=%s]", key, value))
			}
		}
	}
	queryBuilder.WriteString(";")

	// Complete the query
	queryBuilder.WriteString(");out center;")

	// Log the generated query
	overpassQuery := queryBuilder.String()
	logger.Info("generated Overpass query", "query", overpassQuery)

	// Build request
	reqURL, err := url.Parse(osm.OverpassBaseURL)
	if err != nil {
		logger.Error("failed to parse URL", "error", err)
		return ErrorResponse("Internal server error"), nil
	}

	// Make HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL.String(), strings.NewReader("data="+url.QueryEscape(overpassQuery)))
	if err != nil {
		logger.Error("failed to create request", "error", err)
		return ErrorResponse("Failed to create request"), nil
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", osm.GetUserAgent()) // Add User-Agent header

	// Execute request with rate limiting
	resp, err := osm.DoRequest(ctx, req)
	if err != nil {
		logger.Error("failed to execute request", "error", err)
		return ErrorResponse("Failed to communicate with places service"), nil
	}
	defer resp.Body.Close()

	// Process response
	if resp.StatusCode != http.StatusOK {
		logger.Error("places service returned error", "status", resp.StatusCode)
		return ErrorResponse(fmt.Sprintf("Places service error: %d", resp.StatusCode)), nil
	}

	// Parse response
	var overpassResp struct {
		Elements []struct {
			ID     int     `json:"id"`
			Type   string  `json:"type"`
			Lat    float64 `json:"lat,omitempty"`
			Lon    float64 `json:"lon,omitempty"`
			Center *struct {
				Lat float64 `json:"lat"`
				Lon float64 `json:"lon"`
			} `json:"center,omitempty"`
			Tags map[string]string `json:"tags,omitempty"`
		} `json:"elements"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&overpassResp); err != nil {
		logger.Error("failed to decode response", "error", err)
		return ErrorResponse("Failed to parse places response"), nil
	}

	// Log response size
	logger.Info("received elements from Overpass API", "count", len(overpassResp.Elements))

	// Convert to Place objects
	places := make([]Place, 0)
	for _, element := range overpassResp.Elements {
		// Determine coordinates based on element type
		var lat, lon float64
		if element.Type == "node" {
			lat = element.Lat
			lon = element.Lon
		} else if element.Center != nil {
			// Ways and relations use center coordinates
			lat = element.Center.Lat
			lon = element.Center.Lon
		} else {
			// Skip elements without coordinates
			continue
		}

		// Skip elements without tags
		if element.Tags == nil {
			continue
		}

		// Skip elements without a name (unless we want to include unnamed places)
		name := element.Tags["name"]
		if name == "" {
			// For unnamed elements, try to generate a descriptive name
			if element.Tags["amenity"] != "" {
				name = strings.Title(element.Tags["amenity"])
			} else if element.Tags["shop"] != "" {
				name = strings.Title(element.Tags["shop"])
			} else if element.Tags["tourism"] != "" {
				name = strings.Title(element.Tags["tourism"])
			} else if element.Tags["leisure"] != "" {
				name = strings.Title(element.Tags["leisure"])
			} else {
				// Skip elements without any meaningful naming possibility
				continue
			}
		}

		// Determine place category
		categories := []string{}
		if element.Tags["amenity"] != "" {
			categories = append(categories, element.Tags["amenity"])
		}
		if element.Tags["shop"] != "" {
			categories = append(categories, "shop:"+element.Tags["shop"])
		}
		if element.Tags["tourism"] != "" {
			categories = append(categories, "tourism:"+element.Tags["tourism"])
		}
		if element.Tags["leisure"] != "" {
			categories = append(categories, "leisure:"+element.Tags["leisure"])
		}

		// Create place object
		place := Place{
			ID:   strconv.Itoa(element.ID),
			Name: name,
			Location: Location{
				Latitude:  lat,
				Longitude: lon,
			},
			Categories: categories,
		}

		places = append(places, place)
	}

	// Limit results
	if len(places) > limit {
		places = places[:limit]
	}

	// Create output
	output := struct {
		Places []Place `json:"places"`
	}{
		Places: places,
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return ErrorResponse("Failed to generate result"), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}
