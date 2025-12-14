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

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/NERVsystems/osmmcp/pkg/osm"
)

// School represents an educational institution
type School struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Location    Location `json:"location"`
	Distance    float64  `json:"distance,omitempty"`     // in meters
	Type        string   `json:"type,omitempty"`         // e.g., elementary, secondary, university
	Rating      float64  `json:"rating,omitempty"`       // if available
	IsPublic    bool     `json:"is_public,omitempty"`    // true for public schools
	Website     string   `json:"website,omitempty"`      // school website if available
	PhoneNumber string   `json:"phone_number,omitempty"` // contact number if available
}

// FindSchoolsNearbyTool returns a tool definition for finding schools near a location
func FindSchoolsNearbyTool() mcp.Tool {
	return mcp.NewTool("find_schools_nearby",
		mcp.WithDescription("Find educational institutions near a specific location"),
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
			mcp.DefaultNumber(2000),
		),
		mcp.WithString("school_type",
			mcp.Description("Optional school type filter (e.g., elementary, secondary, university, college)"),
			mcp.DefaultString(""),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results to return"),
			mcp.DefaultNumber(10),
		),
	)
}

// HandleFindSchoolsNearby implements finding schools functionality
func HandleFindSchoolsNearby(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "find_schools_nearby")

	// Parse input parameters
	latitude := mcp.ParseFloat64(req, "latitude", 0)
	longitude := mcp.ParseFloat64(req, "longitude", 0)
	radius := mcp.ParseFloat64(req, "radius", 2000)
	schoolType := mcp.ParseString(req, "school_type", "")
	limit := int(mcp.ParseFloat64(req, "limit", 10))

	// Basic validation
	if latitude < -90 || latitude > 90 {
		return ErrorResponse("Latitude must be between -90 and 90"), nil
	}
	if longitude < -180 || longitude > 180 {
		return ErrorResponse("Longitude must be between -180 and 180"), nil
	}
	if radius <= 0 || radius > 5000 {
		return ErrorResponse("Radius must be between 1 and 5000 meters"), nil
	}
	if limit <= 0 {
		limit = 10 // Default limit
	}
	if limit > 50 {
		limit = 50 // Max limit
	}

	// Build Overpass query for schools
	var queryBuilder strings.Builder
	queryBuilder.WriteString("[out:json];")
	queryBuilder.WriteString(fmt.Sprintf("(node(around:%f,%f,%f)[amenity=school];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("node(around:%f,%f,%f)[amenity=university];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("node(around:%f,%f,%f)[amenity=college];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("node(around:%f,%f,%f)[amenity=kindergarten];", radius, latitude, longitude))

	// Also search for ways (buildings)
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[amenity=school];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[amenity=university];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[amenity=college];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[amenity=kindergarten];", radius, latitude, longitude))

	// Complete the query
	queryBuilder.WriteString(");out center;")

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
			ID     int     `json:"id"`
			Type   string  `json:"type"`
			Lat    float64 `json:"lat,omitempty"`
			Lon    float64 `json:"lon,omitempty"`
			Center *struct {
				Lat float64 `json:"lat"`
				Lon float64 `json:"lon"`
			} `json:"center,omitempty"`
			Tags map[string]string `json:"tags"`
		} `json:"elements"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&overpassResp); err != nil {
		logger.Error("failed to decode response", "error", err)
		return ErrorResponse("Failed to parse schools data"), nil
	}

	// Convert to School objects and calculate distances
	schools := make([]School, 0)
	for _, element := range overpassResp.Elements {
		// Get coordinates (handling both nodes and ways)
		var lat, lon float64
		if element.Type == "node" {
			lat = element.Lat
			lon = element.Lon
		} else if element.Type == "way" && element.Center != nil {
			lat = element.Center.Lat
			lon = element.Center.Lon
		} else {
			continue // Skip elements without coordinates
		}

		// Skip elements without a name
		if element.Tags["name"] == "" {
			continue
		}

		// Apply school type filter if specified
		if schoolType != "" {
			// Convert both to lowercase for case-insensitive comparison
			schoolTypeLC := strings.ToLower(schoolType)

			// Check if the school type matches any of the possible fields
			amenity := strings.ToLower(element.Tags["amenity"])
			ischemType := strings.ToLower(element.Tags["isced:level"])
			schoolTypeTag := strings.ToLower(element.Tags["school:type"])

			if !(strings.Contains(amenity, schoolTypeLC) ||
				strings.Contains(ischemType, schoolTypeLC) ||
				strings.Contains(schoolTypeTag, schoolTypeLC)) {
				continue
			}
		}

		// Calculate distance
		distance := osm.HaversineDistance(
			latitude, longitude,
			lat, lon,
		)

		// Determine school type
		schoolTypeValue := ""
		if element.Tags["amenity"] == "university" {
			schoolTypeValue = "university"
		} else if element.Tags["amenity"] == "college" {
			schoolTypeValue = "college"
		} else if element.Tags["amenity"] == "kindergarten" {
			schoolTypeValue = "kindergarten"
		} else if element.Tags["isced:level"] != "" {
			// ISCED classification if available
			schoolTypeValue = element.Tags["isced:level"]
		} else if element.Tags["school:type"] != "" {
			schoolTypeValue = element.Tags["school:type"]
		} else {
			schoolTypeValue = "school"
		}

		// Create school object
		school := School{
			ID:   fmt.Sprintf("%d", element.ID),
			Name: element.Tags["name"],
			Location: Location{
				Latitude:  lat,
				Longitude: lon,
			},
			Distance:    distance,
			Type:        schoolTypeValue,
			IsPublic:    element.Tags["school:type"] == "public" || element.Tags["operator:type"] == "public",
			Website:     element.Tags["website"] + element.Tags["contact:website"],
			PhoneNumber: element.Tags["phone"] + element.Tags["contact:phone"],
		}

		schools = append(schools, school)
	}

	// Sort schools by distance (closest first)
	sort.Slice(schools, func(i, j int) bool {
		return schools[i].Distance < schools[j].Distance
	})

	// Limit results
	if len(schools) > limit {
		schools = schools[:limit]
	}

	// Create output
	output := struct {
		Schools []School `json:"schools"`
	}{
		Schools: schools,
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return ErrorResponse("Failed to generate result"), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}
