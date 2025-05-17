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
	"strings"

	"github.com/NERVsystems/osmmcp/pkg/core"
	"github.com/NERVsystems/osmmcp/pkg/osm"
	"github.com/mark3labs/mcp-go/mcp"
)

// ParkingArea represents a parking facility
type ParkingArea struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Location     Location `json:"location"`
	Distance     float64  `json:"distance,omitempty"`     // in meters
	Type         string   `json:"type,omitempty"`         // e.g., surface, underground, multi-storey
	Access       string   `json:"access,omitempty"`       // e.g., public, private, customers
	Capacity     int      `json:"capacity,omitempty"`     // number of parking spaces if available
	Fee          bool     `json:"fee,omitempty"`          // whether there's a parking fee
	MaxStay      string   `json:"max_stay,omitempty"`     // maximum parking duration if available
	Availability string   `json:"availability,omitempty"` // if real-time availability is known
	Wheelchair   bool     `json:"wheelchair,omitempty"`   // wheelchair accessibility
	Operator     string   `json:"operator,omitempty"`     // who operates the facility
}

// FindParkingAreasTool returns a tool definition for finding parking facilities
func FindParkingAreasTool() mcp.Tool {
	return mcp.NewTool("find_parking_facilities",
		mcp.WithDescription("Find parking facilities near a specific location"),
		mcp.WithNumber("latitude",
			mcp.Required(),
			mcp.Description("The latitude coordinate of the center point"),
		),
		mcp.WithNumber("longitude",
			mcp.Required(),
			mcp.Description("The longitude coordinate of the center point"),
		),
		mcp.WithNumber("radius",
			mcp.Description("Search radius in meters (max 5000)"),
			mcp.DefaultNumber(1000),
		),
		mcp.WithString("type",
			mcp.Description("Optional type filter (e.g., surface, underground, multi-storey)"),
			mcp.DefaultString(""),
		),
		mcp.WithBoolean("include_private",
			mcp.Description("Whether to include private parking facilities"),
			mcp.DefaultBool(false),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results to return (max 50)"),
			mcp.DefaultNumber(10),
		),
	)
}

// HandleFindParkingFacilities implements finding parking facilities functionality
func HandleFindParkingFacilities(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "find_parking_facilities")

	// Parse and validate input parameters
	lat, lon, radius, err := core.ParseCoordsAndRadiusWithLog(req, logger, "", "", "", 1000, 5000)
	if err != nil {
		return core.NewError(core.ErrInvalidInput, err.Error()).ToMCPResult(), nil
	}

	// Parse additional parameters with defaults
	facilityType := mcp.ParseString(req, "type", "")
	includePrivate := mcp.ParseBoolean(req, "include_private", false)
	limit := int(mcp.ParseFloat64(req, "limit", 10))

	// Validate and cap limit
	if limit <= 0 {
		limit = 10 // Default limit
	}
	if limit > 50 {
		limit = 50 // Max limit
	}

	// Build Overpass query using the fluent builder
	queryBuilder := core.NewOverpassBuilder().
		WithTimeout(25).
		WithCenter(lat, lon, radius).
		WithTag("amenity", "parking")

	// Add additional type filter if specified
	if facilityType != "" {
		queryBuilder.WithTag("parking", facilityType)
	}

	// Execute the query
	results, err := fetchParkingFacilities(ctx, queryBuilder.Build())
	if err != nil {
		logger.Error("failed to fetch parking facilities", "error", err)
		return err.(*core.MCPError).ToMCPResult(), nil
	}

	// Process results
	facilities, err := processParkingFacilities(results, lat, lon, includePrivate, facilityType)
	if err != nil {
		logger.Error("failed to process parking facilities", "error", err)
		return core.NewError(core.ErrParseError, "Failed to process parking data").ToMCPResult(), nil
	}

	// Sort facilities by distance (closest first)
	sort.Slice(facilities, func(i, j int) bool {
		return facilities[i].Distance < facilities[j].Distance
	})

	// Limit results
	if len(facilities) > limit {
		facilities = facilities[:limit]
	}

	// Create output
	output := struct {
		Facilities []ParkingArea `json:"facilities"`
	}{
		Facilities: facilities,
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return core.NewError(core.ErrInternalError, "Failed to generate result").ToMCPResult(), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}

// fetchParkingFacilities fetches parking facilities from the Overpass API
func fetchParkingFacilities(ctx context.Context, query string) ([]osm.OverpassElement, error) {
	// Build request
	reqURL, err := url.Parse(osm.OverpassBaseURL)
	if err != nil {
		return nil, core.NewError(core.ErrInternalError, "Internal server error")
	}

	// Create HTTP request factory for retries
	requestFactory := func() (*http.Request, error) {
		req, err := http.NewRequest(
			http.MethodPost,
			reqURL.String(),
			strings.NewReader("data="+url.QueryEscape(query)),
		)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", osm.UserAgent)
		return req, nil
	}

	// Execute request with retries
	client := osm.GetClient(ctx)
	resp, err := core.WithRetryFactory(ctx, requestFactory, client, core.DefaultRetryOptions)
	if err != nil {
		return nil, core.ServiceError("Overpass", http.StatusServiceUnavailable, "Failed to communicate with OSM service")
	}
	defer resp.Body.Close()

	// Process response
	if resp.StatusCode != http.StatusOK {
		return nil, core.ServiceError("Overpass", resp.StatusCode, fmt.Sprintf("OSM service error: %d", resp.StatusCode))
	}

	// Parse response
	var overpassResp struct {
		Elements []osm.OverpassElement `json:"elements"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&overpassResp); err != nil {
		return nil, core.NewError(core.ErrParseError, "Failed to parse parking facilities data")
	}

	return overpassResp.Elements, nil
}

// processParkingFacilities processes OSM elements into parking facilities
func processParkingFacilities(elements []osm.OverpassElement, lat, lon float64, includePrivate bool, facilityType string) ([]ParkingArea, error) {
	facilities := make([]ParkingArea, 0)

	for _, element := range elements {
		// Get coordinates (handling both nodes and ways/relations)
		var elemLat, elemLon float64
		if element.Type == "node" {
			elemLat = element.Lat
			elemLon = element.Lon
		} else if (element.Type == "way" || element.Type == "relation") && element.Center != nil {
			elemLat = element.Center.Lat
			elemLon = element.Center.Lon
		} else {
			continue // Skip elements without coordinates
		}

		// Skip private facilities if not requested
		if !includePrivate {
			access := strings.ToLower(element.Tags["access"])
			if access == "private" || access == "customers" || access == "permit" {
				continue
			}
		}

		// Apply facility type filter if specified
		if facilityType != "" {
			parkingType := strings.ToLower(element.Tags["parking"])
			if parkingType != "" && !strings.Contains(parkingType, strings.ToLower(facilityType)) {
				continue
			}
		}

		// Calculate distance
		distance := osm.HaversineDistance(
			lat, lon,
			elemLat, elemLon,
		)

		// Parse capacity if available
		capacity := 0
		if capacityStr := element.Tags["capacity"]; capacityStr != "" {
			_, _ = fmt.Sscanf(capacityStr, "%d", &capacity)
		} else if capacityStr := element.Tags["capacity:disabled"]; capacityStr != "" {
			_, _ = fmt.Sscanf(capacityStr, "%d", &capacity)
		}

		// Determine if there's a fee
		hasFee := false
		if feeStr := element.Tags["fee"]; feeStr == "yes" || feeStr == "true" {
			hasFee = true
		}

		// Determine wheelchair accessibility
		hasWheelchair := false
		if wheelchairStr := element.Tags["wheelchair"]; wheelchairStr == "yes" || wheelchairStr == "designated" {
			hasWheelchair = true
		}

		// Create facility object
		name := element.Tags["name"]
		if name == "" {
			// Generate a generic name if none exists
			parkingType := element.Tags["parking"]
			if parkingType == "" {
				parkingType = "parking"
			}
			name = fmt.Sprintf("%s parking", strings.Title(parkingType))
		}

		facility := ParkingArea{
			ID:   fmt.Sprintf("%d", element.ID),
			Name: name,
			Location: Location{
				Latitude:  elemLat,
				Longitude: elemLon,
			},
			Distance:   distance,
			Type:       element.Tags["parking"],
			Access:     element.Tags["access"],
			Capacity:   capacity,
			Fee:        hasFee,
			MaxStay:    element.Tags["maxstay"],
			Wheelchair: hasWheelchair,
			Operator:   element.Tags["operator"],
		}

		facilities = append(facilities, facility)
	}

	return facilities, nil
}
