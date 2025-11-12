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

// NeighborhoodAnalysis represents the analysis of a neighborhood for livability
type NeighborhoodAnalysis struct {
	Name            string   `json:"name,omitempty"`
	Location        Location `json:"location"`
	WalkScore       int      `json:"walk_score"`       // 0-100 walkability score
	BikeScore       int      `json:"bike_score"`       // 0-100 biking score
	TransitScore    int      `json:"transit_score"`    // 0-100 public transit score
	EducationScore  int      `json:"education_score"`  // 0-100 education facilities score
	ShoppingScore   int      `json:"shopping_score"`   // 0-100 shopping amenities score
	DiningScore     int      `json:"dining_score"`     // 0-100 dining options score
	RecreationScore int      `json:"recreation_score"` // 0-100 recreation options score
	SafetyScore     int      `json:"safety_score"`     // 0-100 safety score
	HealthcareScore int      `json:"healthcare_score"` // 0-100 healthcare facilities score
	OverallScore    int      `json:"overall_score"`    // 0-100 overall livability score
	PriceIndex      int      `json:"price_index"`      // 0-100 relative price index (higher is more expensive)
	Summary         string   `json:"summary"`          // Textual summary of the analysis
	KeyAmenities    []string `json:"key_amenities"`    // List of notable amenities nearby
	KeyIssues       []string `json:"key_issues"`       // List of notable issues or drawbacks
}

// AnalyzeNeighborhoodTool returns a tool definition for analyzing neighborhood livability
func AnalyzeNeighborhoodTool() mcp.Tool {
	return mcp.NewTool("analyze_neighborhood",
		mcp.WithDescription("Evaluate neighborhood livability for real estate and relocation decisions"),
		mcp.WithNumber("latitude",
			mcp.Required(),
			mcp.Description("The latitude coordinate of the neighborhood center"),
		),
		mcp.WithNumber("longitude",
			mcp.Required(),
			mcp.Description("The longitude coordinate of the neighborhood center"),
		),
		mcp.WithString("neighborhood_name",
			mcp.Description("Optional name of the neighborhood (if known)"),
			mcp.DefaultString(""),
		),
		mcp.WithNumber("radius",
			mcp.Description("Search radius in meters (max 2000)"),
			mcp.DefaultNumber(1000),
		),
		mcp.WithBoolean("include_price_data",
			mcp.Description("Whether to include pricing and real estate data in the analysis"),
			mcp.DefaultBool(true),
		),
	)
}

// HandleAnalyzeNeighborhood implements neighborhood livability analysis functionality
func HandleAnalyzeNeighborhood(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "analyze_neighborhood")

	// Parse input parameters
	latitude := mcp.ParseFloat64(req, "latitude", 0)
	longitude := mcp.ParseFloat64(req, "longitude", 0)
	neighborhoodName := mcp.ParseString(req, "neighborhood_name", "")
	radius := mcp.ParseFloat64(req, "radius", 1000)
	includePriceData := mcp.ParseBoolean(req, "include_price_data", true)

	// Basic validation
	if latitude < -90 || latitude > 90 {
		return ErrorResponse("Latitude must be between -90 and 90"), nil
	}
	if longitude < -180 || longitude > 180 {
		return ErrorResponse("Longitude must be between -180 and 180"), nil
	}
	if radius <= 0 || radius > 2000 {
		return ErrorResponse("Radius must be between 1 and 2000 meters"), nil
	}

	// If neighborhood name not provided, attempt to get it via reverse geocoding
	if neighborhoodName == "" {
		neighborhoodName = getNeighborhoodName(ctx, latitude, longitude)
	}

	// Build Overpass query for amenities in the area
	var queryBuilder strings.Builder
	queryBuilder.WriteString("[out:json];")

	// Shopping amenities
	queryBuilder.WriteString(fmt.Sprintf("(node(around:%f,%f,%f)[shop];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[shop];", radius, latitude, longitude))

	// Food and dining amenities
	queryBuilder.WriteString(fmt.Sprintf("node(around:%f,%f,%f)[amenity=restaurant];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[amenity=restaurant];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("node(around:%f,%f,%f)[amenity=cafe];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[amenity=cafe];", radius, latitude, longitude))

	// Education amenities
	queryBuilder.WriteString(fmt.Sprintf("node(around:%f,%f,%f)[amenity=school];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[amenity=school];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("node(around:%f,%f,%f)[amenity=university];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[amenity=university];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("node(around:%f,%f,%f)[amenity=kindergarten];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[amenity=kindergarten];", radius, latitude, longitude))

	// Healthcare amenities
	queryBuilder.WriteString(fmt.Sprintf("node(around:%f,%f,%f)[amenity=hospital];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[amenity=hospital];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("node(around:%f,%f,%f)[amenity=clinic];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[amenity=clinic];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("node(around:%f,%f,%f)[amenity=pharmacy];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[amenity=pharmacy];", radius, latitude, longitude))

	// Recreation amenities
	queryBuilder.WriteString(fmt.Sprintf("node(around:%f,%f,%f)[leisure];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[leisure];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("relation(around:%f,%f,%f)[leisure];", radius, latitude, longitude))

	// Transportation
	queryBuilder.WriteString(fmt.Sprintf("node(around:%f,%f,%f)[public_transport];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[highway=primary];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[highway=secondary];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[highway=cycleway];", radius, latitude, longitude))
	queryBuilder.WriteString(fmt.Sprintf("way(around:%f,%f,%f)[highway=footway];", radius, latitude, longitude))

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
		return ErrorWithGuidance(NewAPIError("Overpass", http.StatusServiceUnavailable,
			fmt.Sprintf("Failed to communicate with Overpass API: %s", err.Error()),
			GuidanceOverpassGeneral)), nil
	}
	defer resp.Body.Close()

	// Process response
	if resp.StatusCode != http.StatusOK {
		logger.Error("Overpass API returned error", "status", resp.StatusCode)
		return ErrorWithGuidance(NewAPIError("Overpass", resp.StatusCode,
			fmt.Sprintf("Overpass API returned status %d", resp.StatusCode),
			"")), nil
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
		return ErrorResponse("Failed to parse neighborhood data"), nil
	}

	// Process and categorize elements
	var (
		shops        = 0
		restaurants  = 0
		cafes        = 0
		schools      = 0
		universities = 0
		hospitals    = 0
		clinics      = 0
		pharmacies   = 0
		parks        = 0
		sportsVenues = 0
		transitStops = 0
		cycleways    = 0
		footpaths    = 0
	)

	keyAmenities := make([]string, 0)

	for _, element := range overpassResp.Elements {
		// Skip elements without a name or relevant tags
		if element.Tags == nil || (element.Tags["name"] == "" && element.Type != "way") {
			continue
		}

		// Count by category
		if element.Tags["shop"] != "" {
			shops++
			// Add notable shops to key amenities
			if shop := element.Tags["shop"]; shop == "supermarket" || shop == "mall" || shop == "department_store" {
				if name := element.Tags["name"]; name != "" {
					keyAmenities = append(keyAmenities, fmt.Sprintf("%s (%s)", name, shop))
				}
			}
		}

		if element.Tags["amenity"] == "restaurant" {
			restaurants++
			// Add notable restaurants to key amenities
			if name := element.Tags["name"]; name != "" && len(keyAmenities) < 15 {
				keyAmenities = append(keyAmenities, fmt.Sprintf("%s (restaurant)", name))
			}
		}

		if element.Tags["amenity"] == "cafe" {
			cafes++
		}

		if element.Tags["amenity"] == "school" {
			schools++
			// Add schools to key amenities
			if name := element.Tags["name"]; name != "" {
				keyAmenities = append(keyAmenities, fmt.Sprintf("%s (school)", name))
			}
		}

		if element.Tags["amenity"] == "university" {
			universities++
			// Add universities to key amenities
			if name := element.Tags["name"]; name != "" {
				keyAmenities = append(keyAmenities, fmt.Sprintf("%s (university)", name))
			}
		}

		if element.Tags["amenity"] == "hospital" {
			hospitals++
			// Add hospitals to key amenities
			if name := element.Tags["name"]; name != "" {
				keyAmenities = append(keyAmenities, fmt.Sprintf("%s (hospital)", name))
			}
		}

		if element.Tags["amenity"] == "clinic" {
			clinics++
		}

		if element.Tags["amenity"] == "pharmacy" {
			pharmacies++
		}

		if element.Tags["leisure"] == "park" {
			parks++
			// Add parks to key amenities
			if name := element.Tags["name"]; name != "" {
				keyAmenities = append(keyAmenities, fmt.Sprintf("%s (park)", name))
			}
		}

		if element.Tags["leisure"] == "sports_centre" || element.Tags["leisure"] == "stadium" {
			sportsVenues++
		}

		if element.Tags["public_transport"] != "" {
			transitStops++
		}

		if element.Tags["highway"] == "cycleway" {
			cycleways++
		}

		if element.Tags["highway"] == "footway" {
			footpaths++
		}
	}

	// Calculate component scores (0-100)
	walkScore := calculateWalkScore(shops, restaurants, cafes, parks, pharmacies, footpaths)
	bikeScore := calculateBikeScore(cycleways, shops, schools, parks)
	transitScore := calculateTransitScore(transitStops)
	educationScore := calculateEducationScore(schools, universities)
	shoppingScore := calculateShoppingScore(shops)
	diningScore := calculateDiningScore(restaurants, cafes)
	recreationScore := calculateRecreationScore(parks, sportsVenues)
	healthcareScore := calculateHealthcareScore(hospitals, clinics, pharmacies)

	// Safety score is a placeholder - would need crime data
	safetyScore := 60

	// Calculate overall score as weighted average
	overallScore := calculateOverallScore(
		walkScore, bikeScore, transitScore, educationScore,
		shoppingScore, diningScore, recreationScore, safetyScore, healthcareScore,
	)

	// Get price index - in a real implementation, this would come from an external API
	priceIndex := 50
	if !includePriceData {
		priceIndex = 0
	}

	// Key issues based on low scores
	keyIssues := identifyKeyIssues(
		walkScore, bikeScore, transitScore, educationScore,
		shoppingScore, diningScore, recreationScore, safetyScore, healthcareScore,
	)

	// Generate textual summary
	summary := generateNeighborhoodSummary(
		neighborhoodName, overallScore, keyAmenities, keyIssues,
		walkScore, transitScore, diningScore, recreationScore,
	)

	// Create the analysis result
	analysis := NeighborhoodAnalysis{
		Name:            neighborhoodName,
		Location:        Location{Latitude: latitude, Longitude: longitude},
		WalkScore:       walkScore,
		BikeScore:       bikeScore,
		TransitScore:    transitScore,
		EducationScore:  educationScore,
		ShoppingScore:   shoppingScore,
		DiningScore:     diningScore,
		RecreationScore: recreationScore,
		SafetyScore:     safetyScore,
		HealthcareScore: healthcareScore,
		OverallScore:    overallScore,
		PriceIndex:      priceIndex,
		Summary:         summary,
		KeyAmenities:    keyAmenities,
		KeyIssues:       keyIssues,
	}

	// Convert to JSON and return
	jsonResult, err := json.Marshal(analysis)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return ErrorResponse("Failed to generate analysis"), nil
	}

	return mcp.NewToolResultText(string(jsonResult)), nil
}

// Helper functions for calculating scores

func calculateWalkScore(shops, restaurants, cafes, parks, pharmacies, footpaths int) int {
	// Simple algorithm - would be more complex in production
	score := shops*2 + restaurants*2 + cafes + parks*3 + pharmacies*2 + footpaths
	return boundScore(score / 3)
}

func calculateBikeScore(cycleways, shops, schools, parks int) int {
	score := cycleways*4 + shops + schools + parks
	return boundScore(score / 2)
}

func calculateTransitScore(transitStops int) int {
	score := transitStops * 10
	return boundScore(score)
}

func calculateEducationScore(schools, universities int) int {
	score := schools*10 + universities*20
	return boundScore(score)
}

func calculateShoppingScore(shops int) int {
	score := shops * 5
	return boundScore(score)
}

func calculateDiningScore(restaurants, cafes int) int {
	score := restaurants*5 + cafes*3
	return boundScore(score)
}

func calculateRecreationScore(parks, sportsVenues int) int {
	score := parks*10 + sportsVenues*5
	return boundScore(score)
}

func calculateHealthcareScore(hospitals, clinics, pharmacies int) int {
	score := hospitals*20 + clinics*10 + pharmacies*5
	return boundScore(score)
}

func calculateOverallScore(scores ...int) int {
	if len(scores) == 0 {
		return 0
	}

	sum := 0
	for _, score := range scores {
		sum += score
	}

	return sum / len(scores)
}

func boundScore(score int) int {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

// identifyKeyIssues identifies key issues based on low scores
func identifyKeyIssues(scores ...int) []string {
	issues := make([]string, 0)

	categories := []string{
		"walkability", "biking infrastructure", "public transit",
		"education options", "shopping amenities", "dining options",
		"recreation facilities", "safety", "healthcare access",
	}

	for i, score := range scores {
		if score < 30 && i < len(categories) {
			issues = append(issues, fmt.Sprintf("Limited %s", categories[i]))
		}
	}

	return issues
}

// generateNeighborhoodSummary creates a textual summary of the neighborhood
func generateNeighborhoodSummary(name string, overallScore int, amenities, issues []string, walkScore, transitScore, diningScore, recreationScore int) string {
	var builder strings.Builder

	// Neighborhood name and overall assessment
	if name != "" {
		builder.WriteString(fmt.Sprintf("%s is ", name))
	} else {
		builder.WriteString("This area is ")
	}

	// Overall score description
	if overallScore >= 80 {
		builder.WriteString("an excellent neighborhood ")
	} else if overallScore >= 60 {
		builder.WriteString("a good neighborhood ")
	} else if overallScore >= 40 {
		builder.WriteString("an average neighborhood ")
	} else {
		builder.WriteString("a neighborhood with improvement opportunities ")
	}

	builder.WriteString(fmt.Sprintf("with an overall livability score of %d/100. ", overallScore))

	// Add walkability info
	if walkScore >= 70 {
		builder.WriteString("The area is very walkable ")
	} else if walkScore >= 50 {
		builder.WriteString("The area is somewhat walkable ")
	} else {
		builder.WriteString("The area is car-dependent ")
	}

	// Add transit info
	if transitScore >= 70 {
		builder.WriteString("with excellent public transportation. ")
	} else if transitScore >= 50 {
		builder.WriteString("with decent public transportation options. ")
	} else if transitScore >= 30 {
		builder.WriteString("with limited public transportation. ")
	} else {
		builder.WriteString("with very few public transportation options. ")
	}

	// Mention key amenities
	if len(amenities) > 0 {
		builder.WriteString("Notable amenities include ")
		for i, amenity := range amenities {
			if i > 0 {
				if i == len(amenities)-1 {
					builder.WriteString(" and ")
				} else {
					builder.WriteString(", ")
				}
			}
			builder.WriteString(amenity)

			// Limit to 5 amenities to keep summary concise
			if i >= 4 {
				if len(amenities) > 5 {
					builder.WriteString(fmt.Sprintf(", and %d more", len(amenities)-5))
				}
				break
			}
		}
		builder.WriteString(". ")
	}

	// Mention key issues
	if len(issues) > 0 {
		builder.WriteString("Areas for improvement include ")
		for i, issue := range issues {
			if i > 0 {
				if i == len(issues)-1 {
					builder.WriteString(" and ")
				} else {
					builder.WriteString(", ")
				}
			}
			builder.WriteString(issue)

			// Limit to 3 issues to keep summary concise
			if i >= 2 {
				if len(issues) > 3 {
					builder.WriteString(fmt.Sprintf(", and %d more", len(issues)-3))
				}
				break
			}
		}
		builder.WriteString(".")
	}

	return builder.String()
}

// getNeighborhoodName attempts to get a neighborhood name from coordinates via reverse geocoding
func getNeighborhoodName(ctx context.Context, lat, lon float64) string {
	// Initialize with default name
	neighborhoodName := "This area"

	// Build Nominatim request URL
	reqURL, err := url.Parse(osm.NominatimBaseURL + "/reverse")
	if err != nil {
		return neighborhoodName
	}

	// Add query parameters
	q := reqURL.Query()
	q.Add("format", "json")
	q.Add("lat", fmt.Sprintf("%f", lat))
	q.Add("lon", fmt.Sprintf("%f", lon))
	q.Add("zoom", "16") // Higher zoom level for more specific location info
	q.Add("addressdetails", "1")
	reqURL.RawQuery = q.Encode()

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return neighborhoodName
	}

	httpReq.Header.Set("User-Agent", osm.UserAgent)

	// Execute request
	client := osm.GetClient(ctx)
	resp, err := client.Do(httpReq)
	if err != nil {
		return neighborhoodName
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return neighborhoodName
	}

	// Parse response
	var result struct {
		Address struct {
			Neighbourhood string `json:"neighbourhood"`
			Suburb        string `json:"suburb"`
			City          string `json:"city"`
			Town          string `json:"town"`
			Village       string `json:"village"`
		} `json:"address"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return neighborhoodName
	}

	// Try to get the most specific location name
	if result.Address.Neighbourhood != "" {
		neighborhoodName = result.Address.Neighbourhood
	} else if result.Address.Suburb != "" {
		neighborhoodName = result.Address.Suburb
	} else if result.Address.Town != "" {
		neighborhoodName = result.Address.Town
	} else if result.Address.Village != "" {
		neighborhoodName = result.Address.Village
	} else if result.Address.City != "" {
		neighborhoodName = result.Address.City
	}

	return neighborhoodName
}
