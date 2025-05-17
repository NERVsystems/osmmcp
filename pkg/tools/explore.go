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

	"github.com/NERVsystems/osmmcp/pkg/core"
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

	// Parse and validate input parameters
	lat, lon, radius, err := core.ParseCoordsAndRadiusWithLog(req, logger, "", "", "", 0, 5000)
	if err != nil {
		return core.NewError(core.ErrInvalidInput, err.Error()).ToMCPResult(), nil
	}

	// Build Overpass query using the fluent builder
	queryBuilder := core.NewOverpassBuilder().
		WithTimeout(25).
		WithCenter(lat, lon, radius)

	// Add various amenities to search for
	queryBuilder.WithTag("amenity", "")
	queryBuilder.WithTag("shop", "")
	queryBuilder.WithTag("tourism", "")
	queryBuilder.WithTag("leisure", "")
	queryBuilder.WithTag("natural", "")
	queryBuilder.WithTag("landuse", "park")
	queryBuilder.WithTag("leisure", "park")
	queryBuilder.WithTag("place", "")

	// Execute the query
	elements, err := executeOverpassQuery(ctx, queryBuilder.Build())
	if err != nil {
		logger.Error("failed to execute Overpass query", "error", err)
		return err.(*core.MCPError).ToMCPResult(), nil
	}

	// Process the data to generate area description
	categories := make(map[string]int)
	placeCounts := make(map[string]int)
	keyFeatures := make([]string, 0)
	topPlaces := make([]Place, 0)

	// Track neighborhood information
	neighborhood := NeighborhoodInfo{}

	// Process all elements
	for _, element := range elements {
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

				var lat, lon float64
				if element.Type == "node" {
					lat = element.Lat
					lon = element.Lon
				} else if element.Center != nil {
					lat = element.Center.Lat
					lon = element.Center.Lon
				}

				place := Place{
					ID:   fmt.Sprintf("%d", element.ID),
					Name: element.Tags["name"],
					Location: Location{
						Latitude:  lat,
						Longitude: lon,
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
			Latitude:  lat,
			Longitude: lon,
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
		return core.NewError(core.ErrInternalError, "Failed to generate result").ToMCPResult(), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}

// executeOverpassQuery executes an Overpass API query and returns the elements
func executeOverpassQuery(ctx context.Context, query string) ([]osm.OverpassElement, error) {
	// Build request
	reqURL, err := url.Parse(osm.OverpassBaseURL)
	if err != nil {
		return nil, core.NewError(core.ErrInternalError, "Internal server error")
	}

	// Create HTTP request factory for retries
	requestFactory := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(
			ctx,
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
		return nil, core.NewError(core.ErrParseError, "Failed to parse area data")
	}

	return overpassResp.Elements, nil
}
