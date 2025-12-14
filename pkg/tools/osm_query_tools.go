package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/NERVsystems/osmmcp/pkg/geo"
	"github.com/NERVsystems/osmmcp/pkg/osm"
	"github.com/NERVsystems/osmmcp/pkg/osm/queries"
)

const (
	// Input validation limits to prevent DoS attacks
	maxTagCount       = 20  // Maximum number of tags per query
	maxTagKeyLength   = 100 // Maximum length of tag keys
	maxTagValueLength = 200 // Maximum length of tag values
)

// validateTags validates tag input to prevent DoS attacks and injection
func validateTags(tags map[string]string) error {
	// Check tag count limit
	if len(tags) == 0 {
		return fmt.Errorf("at least one tag is required")
	}
	if len(tags) > maxTagCount {
		return fmt.Errorf("too many tags: %d (maximum: %d)", len(tags), maxTagCount)
	}

	// Validate each tag
	for key, value := range tags {
		// Validate key length
		if len(key) == 0 {
			return fmt.Errorf("empty tag key")
		}
		if len(key) > maxTagKeyLength {
			return fmt.Errorf("tag key too long: %d characters (maximum: %d)", len(key), maxTagKeyLength)
		}

		// Validate value length
		if len(value) > maxTagValueLength {
			return fmt.Errorf("tag value too long: %d characters (maximum: %d)", len(value), maxTagValueLength)
		}

		// Check for potentially dangerous characters
		if strings.ContainsAny(key, "\x00\r\n\t") {
			return fmt.Errorf("tag key contains invalid characters")
		}
		if strings.ContainsAny(value, "\x00\r\n\t") {
			return fmt.Errorf("tag value contains invalid characters")
		}

		// Additional validation for injection prevention
		if strings.Contains(key, "..") || strings.Contains(value, "..") {
			return fmt.Errorf("tag contains potentially unsafe sequences")
		}
	}

	return nil
}

// OSMQueryBBoxInput defines the input parameters for querying OSM data by bounding box
type OSMQueryBBoxInput struct {
	BBox geo.BoundingBox   `json:"bbox"`
	Tags map[string]string `json:"tags"`
}

// OSMElement represents an OpenStreetMap element with tags
type OSMElement struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Location *geo.Location     `json:"location,omitempty"`
	Center   *geo.Location     `json:"center,omitempty"`
	Tags     map[string]string `json:"tags,omitempty"`
	Distance float64           `json:"distance,omitempty"`
}

// OSMQueryBBoxOutput defines the output for OSM query results
type OSMQueryBBoxOutput struct {
	Elements []OSMElement `json:"elements"`
}

// OSMQueryBBoxTool returns a tool definition for querying OSM data by bounding box
func OSMQueryBBoxTool() mcp.Tool {
	return mcp.NewTool("osm_query_bbox",
		mcp.WithDescription("Query OpenStreetMap data within a bounding box with tag filters. Requirements: (1) Use exact field names: minLat, minLon, maxLat, maxLon (case-sensitive), (2) Latitude range: -90 to 90, (3) Longitude range: -180 to 180, (4) minLat < maxLat, (5) minLon < maxLon. Example usage: bbox: {\"minLat\": 37.77, \"minLon\": -122.42, \"maxLat\": 37.78, \"maxLon\": -122.41}, tags: {\"amenity\": \"restaurant\", \"cuisine\": \"*\"}"),
		mcp.WithObject("bbox",
			mcp.Required(),
			mcp.Description("Bounding box object with required fields: minLat (number), minLon (number), maxLat (number), maxLon (number). Example: {\"minLat\": 37.77, \"minLon\": -122.42, \"maxLat\": 37.78, \"maxLon\": -122.41}"),
		),
		mcp.WithObject("tags",
			mcp.Required(),
			mcp.Description("Tags to filter by as key-value string pairs. Use '*' as value to match any value for a key. Example: {\"amenity\": \"restaurant\", \"cuisine\": \"*\", \"name\": \"Pizza\"}. Common keys: amenity, shop, leisure, highway, building, name, cuisine, brand"),
		),
	)
}

// HandleOSMQueryBBox implements OSM bbox querying functionality
func HandleOSMQueryBBox(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "osm_query_bbox")

	// Parse input
	var input OSMQueryBBoxInput
	inputJSON, err := json.Marshal(req.Params.Arguments)
	if err != nil {
		logger.Error("failed to marshal input", "error", err)
		return ErrorResponse(fmt.Sprintf("Failed to process input arguments: %v. Expected bbox object with minLat, minLon, maxLat, maxLon fields and tags object with key-value pairs.", err)), nil
	}

	if err := json.Unmarshal(inputJSON, &input); err != nil {
		logger.Error("failed to parse input", "error", err)
		return ErrorResponse(fmt.Sprintf("Invalid input format. Expected bbox object with minLat, minLon, maxLat, maxLon fields and tags object with key-value pairs. Error: %v. Example: {\"bbox\": {\"minLat\": 37.77, \"minLon\": -122.42, \"maxLat\": 37.78, \"maxLon\": -122.41}, \"tags\": {\"amenity\": \"restaurant\"}}", err)), nil
	}

	// Validate bounding box
	if input.BBox.MinLat < -90 || input.BBox.MinLat > 90 ||
		input.BBox.MaxLat < -90 || input.BBox.MaxLat > 90 ||
		input.BBox.MinLon < -180 || input.BBox.MinLon > 180 ||
		input.BBox.MaxLon < -180 || input.BBox.MaxLon > 180 ||
		input.BBox.MinLat >= input.BBox.MaxLat ||
		input.BBox.MinLon >= input.BBox.MaxLon {
		logger.Error("invalid bounding box",
			"minLat", input.BBox.MinLat,
			"minLon", input.BBox.MinLon,
			"maxLat", input.BBox.MaxLat,
			"maxLon", input.BBox.MaxLon)
		return ErrorResponse(fmt.Sprintf("Invalid bounding box: minLat=%.6f, minLon=%.6f, maxLat=%.6f, maxLon=%.6f. Requirements: (1) Use exact field names: minLat, minLon, maxLat, maxLon (case-sensitive), (2) Latitude range: -90 to 90, (3) Longitude range: -180 to 180, (4) minLat < maxLat, (5) minLon < maxLon. Example: {\"minLat\": 37.77, \"minLon\": -122.42, \"maxLat\": 37.78, \"maxLon\": -122.41}", input.BBox.MinLat, input.BBox.MinLon, input.BBox.MaxLat, input.BBox.MaxLon)), nil
	}

	// Validate tags with comprehensive bounds checking
	if err := validateTags(input.Tags); err != nil {
		logger.Error("invalid tags", "error", err)
		return ErrorResponse(fmt.Sprintf("Invalid tags: %v", err)), nil
	}

	// Build Overpass query using the query builder
	queryBuilder := queries.NewOverpassBuilder()
	queryBuilder.Begin()

	// Process tags to handle '*' wildcard properly
	for key, value := range input.Tags {
		if value == "*" {
			// For wildcard, we only need the key present, not a specific value
			input.Tags[key] = ""
		}
	}

	queryBuilder.WithNodeInBbox(
		input.BBox.MinLat, input.BBox.MinLon,
		input.BBox.MaxLat, input.BBox.MaxLon,
		input.Tags,
	)
	queryBuilder.WithWayInBbox(
		input.BBox.MinLat, input.BBox.MinLon,
		input.BBox.MaxLat, input.BBox.MaxLon,
		input.Tags,
	)
	// Also include relations
	queryBuilder.WithRelationInBbox(
		input.BBox.MinLat, input.BBox.MinLon,
		input.BBox.MaxLat, input.BBox.MaxLon,
		input.Tags,
	)
	queryBuilder.End().WithOutput("center")
	overpassQuery := queryBuilder.Build()

	// Log the generated query for debugging
	logger.Info("generated Overpass query", "query", overpassQuery)

	// Wait for rate limiting
	if err := osm.WaitForService(ctx, osm.ServiceOverpass); err != nil {
		logger.Error("rate limit exceeded", "error", err)
		return ErrorWithGuidance(&APIError{
			Service:     "Overpass",
			StatusCode:  http.StatusTooManyRequests,
			Message:     "Rate limit exceeded",
			Recoverable: true,
			Guidance:    GuidanceOverpassRateLimit,
		}), nil
	}

	// Build request
	reqURL, err := url.Parse(osm.OverpassBaseURL)
	if err != nil {
		logger.Error("failed to parse URL", "error", err)
		return ErrorResponse("Internal server error"), nil
	}

	// Make HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL.String(),
		strings.NewReader("data="+url.QueryEscape(overpassQuery)))
	if err != nil {
		logger.Error("failed to create request", "error", err)
		return ErrorResponse("Failed to create request"), nil
	}

	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("User-Agent", osm.GetUserAgent())

	// Execute request
	client := osm.GetClient(ctx)
	resp, err := client.Do(httpReq)
	if err != nil {
		logger.Error("failed to execute request", "error", err)
		return ErrorResponse("Failed to communicate with Overpass API"), nil
	}
	defer resp.Body.Close()

	// Process response
	if resp.StatusCode != http.StatusOK {
		logger.Error("Overpass API returned error", "status", resp.StatusCode)
		// Read error response body if available
		var errorMsg string
		if bodyBytes, readErr := io.ReadAll(resp.Body); readErr == nil {
			errorMsg = string(bodyBytes)
			if len(errorMsg) > 500 {
				errorMsg = errorMsg[:500] + "..."
			}
		}
		if errorMsg == "" {
			errorMsg = fmt.Sprintf("Overpass API returned status %d", resp.StatusCode)
		} else {
			errorMsg = fmt.Sprintf("Overpass API returned status %d: %s", resp.StatusCode, errorMsg)
		}
		return ErrorWithGuidance(NewAPIError("Overpass", resp.StatusCode, errorMsg, "")), nil
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
		return ErrorResponse("Failed to parse Overpass API response"), nil
	}

	// Convert to output format
	output := OSMQueryBBoxOutput{
		Elements: make([]OSMElement, len(overpassResp.Elements)),
	}

	for i, element := range overpassResp.Elements {
		// Convert ID to string
		output.Elements[i].ID = fmt.Sprintf("%d", element.ID)
		output.Elements[i].Type = element.Type
		output.Elements[i].Tags = element.Tags

		// Set location for nodes
		if element.Type == "node" {
			output.Elements[i].Location = &geo.Location{
				Latitude:  element.Lat,
				Longitude: element.Lon,
			}
		}

		// Set center for ways and relations
		if element.Center != nil {
			output.Elements[i].Center = &geo.Location{
				Latitude:  element.Center.Lat,
				Longitude: element.Center.Lon,
			}
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

// FilterTagsInput defines the input parameters for filtering OSM elements by tag
type FilterTagsInput struct {
	Elements []OSMElement        `json:"elements"`
	Tags     map[string][]string `json:"tags"`
}

// FilterTagsOutput defines the output for filtered OSM elements
type FilterTagsOutput struct {
	Elements []OSMElement `json:"elements"`
}

// FilterTagsTool returns a tool definition for filtering OSM elements by tag
func FilterTagsTool() mcp.Tool {
	return mcp.NewTool("filter_tags",
		mcp.WithDescription("Filter OSM elements by specified tags"),
		mcp.WithArray("elements",
			mcp.Required(),
			mcp.Description("Array of OSM elements to filter"),
		),
		mcp.WithObject("tags",
			mcp.Required(),
			mcp.Description("Tags to filter by, with key-value pairs where values are an array of acceptable values"),
		),
	)
}

// HandleFilterTags implements filtering OSM elements by tag
func HandleFilterTags(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "filter_tags")

	// Parse input
	var input FilterTagsInput
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
	if len(input.Elements) == 0 {
		logger.Error("empty elements array")
		return ErrorResponse("At least one element is required"), nil
	}

	if len(input.Tags) == 0 {
		logger.Error("empty tags object")
		return ErrorResponse("At least one tag is required"), nil
	}

	// Filter elements
	filteredElements := make([]OSMElement, 0)
	for _, element := range input.Elements {
		if elementMatchesTags(element, input.Tags) {
			filteredElements = append(filteredElements, element)
		}
	}

	// Create output
	output := FilterTagsOutput{
		Elements: filteredElements,
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return ErrorResponse("Failed to generate result"), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}

// elementMatchesTags checks if an element matches the specified tag criteria
func elementMatchesTags(element OSMElement, tagCriteria map[string][]string) bool {
	if element.Tags == nil {
		return false
	}

	// If no tag criteria provided, match all elements
	if len(tagCriteria) == 0 {
		return true
	}

	// All specified tags must match
	for key, allowedValues := range tagCriteria {
		elementValue, exists := element.Tags[key]
		if !exists {
			return false
		}

		// If no allowed values are specified, just checking for key presence is enough
		if len(allowedValues) == 0 {
			continue
		}

		// Special case: wildcard '*' means any value for this key is acceptable
		if len(allowedValues) == 1 && allowedValues[0] == "*" {
			continue
		}

		// Check if the element's value is in the allowed values
		valueMatches := false
		for _, allowedValue := range allowedValues {
			if elementValue == allowedValue {
				valueMatches = true
				break
			}
		}

		if !valueMatches {
			return false
		}
	}

	return true
}

// SortByDistanceInput defines the input parameters for sorting OSM elements by distance
type SortByDistanceInput struct {
	Elements []OSMElement `json:"elements"`
	Ref      geo.Location `json:"ref"`
}

// SortByDistanceOutput defines the output for sorted OSM elements
type SortByDistanceOutput struct {
	Elements []OSMElement `json:"elements"`
}

// SortByDistanceTool returns a tool definition for sorting OSM elements by distance
func SortByDistanceTool() mcp.Tool {
	return mcp.NewTool("sort_by_distance",
		mcp.WithDescription("Sort OSM elements by distance from a reference point"),
		mcp.WithArray("elements",
			mcp.Required(),
			mcp.Description("Array of OSM elements to sort"),
		),
		mcp.WithObject("ref",
			mcp.Required(),
			mcp.Description("Reference point to measure distances from"),
		),
	)
}

// HandleSortByDistance implements sorting OSM elements by distance
func HandleSortByDistance(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "sort_by_distance")

	// Parse input
	var input SortByDistanceInput
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
	if len(input.Elements) == 0 {
		logger.Error("empty elements array")
		return ErrorResponse("At least one element is required"), nil
	}

	// Validate reference point
	if input.Ref.Latitude == 0 && input.Ref.Longitude == 0 {
		logger.Error("missing 'ref' coordinates")
		return ErrorResponse("Missing 'ref' coordinates"), nil
	}

	if err := osm.ValidateCoords(input.Ref.Latitude, input.Ref.Longitude); err != nil {
		logger.Error("invalid 'ref' coordinates", "error", err)
		return ErrorResponse(fmt.Sprintf("Invalid 'ref' coordinates: %s", err)), nil
	}

	// Calculate distances and store in elements
	elements := make([]OSMElement, len(input.Elements))
	for i, element := range input.Elements {
		elements[i] = element

		// Determine point to use for distance calculation
		var pointLat, pointLon float64
		if element.Location != nil {
			pointLat = element.Location.Latitude
			pointLon = element.Location.Longitude
		} else if element.Center != nil {
			pointLat = element.Center.Latitude
			pointLon = element.Center.Longitude
		} else {
			// Skip elements without location information
			logger.Warn("element has no location or center", "id", element.ID, "type", element.Type)
			continue
		}

		// Calculate distance
		distance := geo.HaversineDistance(
			input.Ref.Latitude, input.Ref.Longitude,
			pointLat, pointLon,
		)
		elements[i].Distance = distance
	}

	// Sort elements by distance
	sort.Slice(elements, func(i, j int) bool {
		return elements[i].Distance < elements[j].Distance
	})

	// Create output
	output := SortByDistanceOutput{
		Elements: elements,
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return ErrorResponse("Failed to generate result"), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}
