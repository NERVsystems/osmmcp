// Package tools provides the OpenStreetMap MCP tools implementations.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/NERVsystems/osmmcp/pkg/osm"
	"github.com/mark3labs/mcp-go/mcp"
)

// ExploreAreaTool returns a tool definition for exploring an area
func ExploreAreaTool() mcp.Tool {
	return mcp.NewTool("explore_area",
		mcp.WithDescription("Explore and describe an area based on its coordinates"),
		mcp.WithNumber("latitude",
			mcp.Required(),
			mcp.Description("The latitude coordinate of the area's center point"),
		),
		mcp.WithNumber("longitude",
			mcp.Required(),
			mcp.Description("The longitude coordinate of the area's center point"),
		),
		mcp.WithNumber("radius",
			mcp.Required(),
			mcp.Description("Search radius in meters (max 5000)"),
		),
	)
}

// AreaDescription represents a description of an area
type AreaDescription struct {
	Center       Location         `json:"center"`
	Radius       float64          `json:"radius"`
	Categories   map[string]int   `json:"categories"`
	PlaceCounts  map[string]int   `json:"place_counts"`
	KeyFeatures  []string         `json:"key_features"`
	TopPlaces    []Place          `json:"top_places"`
	Neighborhood NeighborhoodInfo `json:"neighborhood,omitempty"`
}

// NeighborhoodInfo contains information about a neighborhood
type NeighborhoodInfo struct {
	Name        string   `json:"name,omitempty"`
	Type        string   `json:"type,omitempty"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// HandleExploreArea implements area exploration functionality
func HandleExploreArea(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "explore_area")

	// Parse input parameters
	latitude := mcp.ParseFloat64(req, "latitude", 0)
	longitude := mcp.ParseFloat64(req, "longitude", 0)
	radius := mcp.ParseFloat64(req, "radius", 0)

	// Basic validation with descriptive error messages
	if latitude < -90 || latitude > 90 {
		return ErrorResponse("Invalid latitude: must be between -90 and 90 degrees"), nil
	}
	if longitude < -180 || longitude > 180 {
		return ErrorResponse("Invalid longitude: must be between -180 and 180 degrees"), nil
	}
	if radius <= 0 {
		return ErrorResponse("Radius is required and must be greater than 0 meters"), nil
	}
	if radius > 5000 {
		return ErrorResponse("Radius exceeds maximum allowed value of 5000 meters"), nil
	}

	// Build Overpass query to get area information
	var queryBuilder strings.Builder
	queryBuilder.WriteString("[out:json];")

	// Get general amenities
	queryBuilder.WriteString(fmt.Sprintf("(node(around:%f,%f,%f)[amenity];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("node(around:%f,%f,%f)[shop];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("node(around:%f,%f,%f)[tourism];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("node(around:%f,%f,%f)[leisure];", radius, latitude, longitude))

	// Add natural features
	queryBuilder.WriteString(fmt.Sprintf("node(around:%f,%f,%f)[natural];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[natural];", radius, latitude, longitude))

	// Add parks and public spaces
	queryBuilder.WriteString(fmt.Sprintf("node(around:%f,%f,%f)[landuse=park];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[landuse=park];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("node(around:%f,%f,%f)[leisure=park];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[leisure=park];", radius, latitude, longitude))

	// Add neighborhood/district information
	queryBuilder.WriteString(fmt.Sprintf("node(around:%f,%f,%f)[place];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[place];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("relation(around:%f,%f,%f)[place];", radius, latitude, longitude))

	// Complete the query
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

	// Execute request with timeout
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
		return ErrorResponse("Failed to parse area data"), nil
	}

	// Process the data to generate area description
	categories := make(map[string]int)
	placeCounts := make(map[string]int)
	keyFeatures := make([]string, 0)
	topPlaces := make([]Place, 0)

	// Track neighborhood information
	neighborhood := NeighborhoodInfo{}

	// Process all elements
	for _, element := range overpassResp.Elements {
		// Extract categories and count them
		if amenity, ok := element.Tags["amenity"]; ok {
			categories["amenity:"+amenity]++
			placeCounts["amenity"]++
		}
		if shop, ok := element.Tags["shop"]; ok {
			categories["shop:"+shop]++
			placeCounts["shop"]++
		}
		if tourism, ok := element.Tags["tourism"]; ok {
			categories["tourism:"+tourism]++
			placeCounts["tourism"]++
		}
		if leisure, ok := element.Tags["leisure"]; ok {
			categories["leisure:"+leisure]++
			placeCounts["leisure"]++
		}
		if natural, ok := element.Tags["natural"]; ok {
			categories["natural:"+natural]++
			placeCounts["natural"]++
		}

		// Look for neighborhood or district information
		if place, ok := element.Tags["place"]; ok {
			if place == "neighbourhood" || place == "suburb" || place == "quarter" || place == "district" {
				if name, ok := element.Tags["name"]; ok {
					// Only use the first neighborhood found for simplicity
					if neighborhood.Name == "" {
						neighborhood.Name = name
						neighborhood.Type = place
						// Try to get additional information
						if element.Tags["description"] != "" {
							neighborhood.Description = element.Tags["description"]
						}
						// Add any tags as features
						for k, v := range element.Tags {
							if k != "name" && k != "place" && k != "description" {
								neighborhood.Tags = append(neighborhood.Tags, fmt.Sprintf("%s=%s", k, v))
							}
						}
					}
				}
			}
		}

		// Add top places with high importance
		if element.Type == "node" && element.Tags["name"] != "" {
			// Consider parks, museums, important landmarks, etc.
			important := false
			if element.Tags["tourism"] == "museum" ||
				element.Tags["tourism"] == "attraction" ||
				element.Tags["amenity"] == "university" ||
				element.Tags["amenity"] == "hospital" ||
				element.Tags["leisure"] == "park" ||
				element.Tags["amenity"] == "theatre" ||
				element.Tags["amenity"] == "library" {
				important = true
			}

			if important {
				categories := []string{}
				for k, v := range element.Tags {
					if k != "name" && (k == "amenity" || k == "shop" || k == "tourism" || k == "leisure") {
						categories = append(categories, fmt.Sprintf("%s:%s", k, v))
					}
				}

				place := Place{
					ID:   fmt.Sprintf("%d", element.ID),
					Name: element.Tags["name"],
					Location: Location{
						Latitude:  element.Lat,
						Longitude: element.Lon,
					},
					Categories: categories,
				}

				topPlaces = append(topPlaces, place)
				if len(topPlaces) >= 10 {
					break
				}
			}
		}
	}

	// Determine key features
	if placeCounts["shop"] > 10 {
		keyFeatures = append(keyFeatures, "Commercial area with many shops")
	}
	if placeCounts["amenity"] > 10 {
		keyFeatures = append(keyFeatures, "Area with many amenities")
	}
	if placeCounts["tourism"] > 5 {
		keyFeatures = append(keyFeatures, "Tourist area")
	}
	if placeCounts["leisure"] > 5 || categories["leisure:park"] > 2 {
		keyFeatures = append(keyFeatures, "Recreational area with parks/leisure facilities")
	}
	if placeCounts["natural"] > 3 {
		keyFeatures = append(keyFeatures, "Area with natural features")
	}
	if categories["amenity:restaurant"] > 5 || categories["amenity:cafe"] > 5 {
		keyFeatures = append(keyFeatures, "Dining district with many restaurants/cafes")
	}
	if categories["amenity:school"] > 2 || categories["amenity:university"] > 0 {
		keyFeatures = append(keyFeatures, "Educational area")
	}
	if categories["amenity:hospital"] > 0 || categories["amenity:clinic"] > 2 {
		keyFeatures = append(keyFeatures, "Medical/healthcare area")
	}

	// If we have no key features, add a generic one
	if len(keyFeatures) == 0 {
		keyFeatures = append(keyFeatures, "Residential or low-density area")
	}

	// Create the area description
	areaDescription := AreaDescription{
		Center: Location{
			Latitude:  latitude,
			Longitude: longitude,
		},
		Radius:      radius,
		Categories:  categories,
		PlaceCounts: placeCounts,
		KeyFeatures: keyFeatures,
		TopPlaces:   topPlaces,
	}

	// Add neighborhood info if available
	if neighborhood.Name != "" {
		areaDescription.Neighborhood = neighborhood
	}

	// Create output
	output := struct {
		AreaDescription AreaDescription `json:"area_description"`
	}{
		AreaDescription: areaDescription,
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return ErrorResponse("Failed to generate result"), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}
