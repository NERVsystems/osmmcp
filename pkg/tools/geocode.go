package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/NERVsystems/osmmcp/pkg/core"
	"github.com/NERVsystems/osmmcp/pkg/osm"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/sync/singleflight"
)

const (
	// Nominatim is OSM's geocoding service
	nominatimBaseURL = "https://nominatim.openstreetmap.org"

	// UserAgent identifies our application to Nominatim
	userAgent = "NERV-MCP-Geocoder/1.0 (contact: ops@nerv.systems)"

	// Search parameters
	maxResults    = 3   // Maximum number of results to return
	minImportance = 0.4 // Minimum importance threshold for result selection

	// Cache configuration
	cacheSize = 512            // Maximum number of entries in the LRU cache
	cacheTTL  = 24 * time.Hour // Cache entries valid for 24 hours

	// Retry configuration
	maxRetries     = 3                      // Maximum number of retries for failed requests
	initialBackoff = 500 * time.Millisecond // Initial backoff delay
)

// Default region to append for single-token or landmark queries
// defaultRegion specifies the region appended to single token queries. It can
// be overridden via the OSMMCP_DEFAULT_REGION environment variable to make the
// server behavior configurable in different deployments.
var defaultRegion = func() string {
	if env := os.Getenv("OSMMCP_DEFAULT_REGION"); env != "" {
		return env
	}
	return "Singapore"
}()

// Global cache and request group to deduplicate in-flight requests
var (
	// geocodeCache is an LRU cache for geocoding results
	geocodeCache *lru.Cache[string, []byte]

	// reverseGeocodeCache is an LRU cache for reverse geocoding results
	reverseGeocodeCache *lru.Cache[string, []byte]

	// requestGroup deduplicates in-flight requests
	requestGroup singleflight.Group

	// Once ensures caches are initialized only once
	initOnce sync.Once
)

// initCaches initializes the LRU caches
func initCaches() {
	initOnce.Do(func() {
		var err error

		// Initialize geocoding cache
		geocodeCache, err = lru.New[string, []byte](cacheSize)
		if err != nil {
			slog.Error("failed to create geocode cache", "error", err)
			// Create a minimal cache as fallback
			geocodeCache, _ = lru.New[string, []byte](10)
		}

		// Initialize reverse geocoding cache
		reverseGeocodeCache, err = lru.New[string, []byte](cacheSize)
		if err != nil {
			slog.Error("failed to create reverse geocode cache", "error", err)
			// Create a minimal cache as fallback
			reverseGeocodeCache, _ = lru.New[string, []byte](10)
		}

		// Set the user agent for all OSM requests
		osm.SetUserAgent(userAgent)
	})
}

// GeocodeAddressInput defines the input parameters for geocoding an address
type GeocodeAddressInput struct {
	Address string `json:"address"`
	Region  string `json:"region,omitempty"` // Optional region for context
}

// GeocodeAddressOutput defines the output format for geocoded addresses
type GeocodeAddressOutput struct {
	Place      Place   `json:"place"`
	Candidates []Place `json:"candidates,omitempty"`
}

// GeocodeDetailedError provides detailed error information with suggestions
type GeocodeDetailedError struct {
	Code        string   `json:"code"`
	Message     string   `json:"message"`
	Query       string   `json:"query,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
}

// NewGeocodeDetailedError creates a detailed error response with JSON format
func NewGeocodeDetailedError(code, message string, query string, suggestions ...string) *mcp.CallToolResult {
	// Create structured error
	errorObj := GeocodeDetailedError{
		Code:        code,
		Message:     message,
		Query:       query,
		Suggestions: suggestions,
	}

	// Marshal to JSON
	errorJSON, err := json.Marshal(errorObj)
	if err != nil {
		// Fallback if marshaling fails
		return mcp.NewToolResultError(fmt.Sprintf("ERROR: %s - %s", code, message))
	}

	return mcp.NewToolResultError(string(errorJSON))
}

// GeocodeAddressTool returns a tool definition for geocoding addresses
func GeocodeAddressTool() mcp.Tool {
	return mcp.NewTool("geocode_address",
		mcp.WithDescription("Convert an address or place name to geographic coordinates"),
		mcp.WithString("address",
			mcp.Required(),
			mcp.Description("The address or place name to geocode. For best results, format addresses clearly without parentheses and include city/country information for locations outside the US. For international or tourist sites, include the region or country name. Example: 'Merlion Park Singapore' instead of 'Merlion Park (Singapore)'."),
		),
		mcp.WithString("region",
			mcp.Description("Optional region context to improve results for ambiguous queries (e.g., 'Singapore'). Will be automatically appended to short queries."),
			mcp.DefaultString(""),
		),
	)
}

// sanitizeAddress cleans the address query for better geocoding results
// and returns both with and without parentheses versions
func sanitizeAddress(address string) (string, string) {
	// Remove extra whitespace
	address = strings.TrimSpace(address)

	// Replace multiple spaces with a single space
	re := regexp.MustCompile(`\s+`)
	address = re.ReplaceAllString(address, " ")

	// Extract content inside parentheses
	reParens := regexp.MustCompile(`\(([^)]*)\)`)
	matches := reParens.FindStringSubmatch(address)

	// Default values
	withoutParens := address
	parensContent := ""

	// If we found parentheses content
	if len(matches) > 1 {
		// Content inside parentheses
		parensContent = strings.TrimSpace(matches[1])

		// Content without parentheses - replace with a single space to avoid double spaces
		withoutParens = strings.TrimSpace(reParens.ReplaceAllString(address, " "))

		// Replace any doubled spaces that might have been created
		withoutParens = re.ReplaceAllString(withoutParens, " ")
	}

	return withoutParens, parensContent
}

// ensureRegion appends region information to short queries if needed
func ensureRegion(query, region string) string {
	// If region is empty or query already has region info, return as is
	if region == "" || strings.Contains(query, region) {
		return query
	}

	// If query is a short phrase (fewer than three words) and has no commas
	if !strings.Contains(query, ",") {
		words := strings.Fields(query)
		if len(words) < 3 {
			return query + " " + region
		}
	}

	return query
}

// cacheKey generates a consistent cache key for a query
func cacheKey(query string) string {
	// Normalize query for caching
	return strings.ToLower(strings.TrimSpace(query))
}

// reverseGeoCacheKey generates a cache key for reverse geocoding
func reverseGeoCacheKey(lat, lon float64) string {
	// Round coordinates to 5 decimal places for caching
	roundedLat := math.Round(lat*100000) / 100000
	roundedLon := math.Round(lon*100000) / 100000
	return fmt.Sprintf("%.5f,%.5f", roundedLat, roundedLon)
}

// NominatimResult represents a result from the Nominatim geocoding service
type NominatimResult struct {
	PlaceID     json.Number `json:"place_id"` // Using json.Number to handle both string and numeric IDs
	DisplayName string      `json:"display_name"`
	Lat         string      `json:"lat"`
	Lon         string      `json:"lon"`
	Type        string      `json:"type"`
	Importance  float64     `json:"importance"`
	Address     struct {
		Road        string `json:"road"`
		HouseNumber string `json:"house_number"`
		City        string `json:"city"`
		Town        string `json:"town"`
		State       string `json:"state"`
		Country     string `json:"country"`
		PostCode    string `json:"postcode"`
	} `json:"address"`
}

// geocodeQuery performs a single geocoding request with caching
func geocodeQuery(ctx context.Context, query string) ([]NominatimResult, error) {
	logger := slog.Default().With("query", query)

	// Initialize caches if needed
	initCaches()

	// Create a normalized key for caching
	key := cacheKey(query)

	// Check cache first
	if cachedData, found := geocodeCache.Get(key); found {
		logger.Info("cache hit", "query", query)

		var results []NominatimResult
		if err := json.Unmarshal(cachedData, &results); err != nil {
			logger.Error("failed to unmarshal cached results", "error", err)
		} else {
			return results, nil
		}
	}

	// Use singleflight to deduplicate in-flight requests for the same query
	result, err, _ := requestGroup.Do(key, func() (interface{}, error) {
		// Build request URL
		reqURL, err := url.Parse(fmt.Sprintf("%s/search", nominatimBaseURL))
		if err != nil {
			return nil, core.NewError(core.ErrInternalError, "Failed to parse URL for geocoding service")
		}

		// Add query parameters
		q := reqURL.Query()
		q.Add("q", query)
		q.Add("format", "json")
		q.Add("limit", fmt.Sprintf("%d", maxResults)) // Increased limit
		q.Add("addressdetails", "1")                  // Get detailed address info
		reqURL.RawQuery = q.Encode()

		// Create HTTP request factory for retries
		requestFactory := func() (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("User-Agent", userAgent)
			return req, nil
		}

		// Execute request with retries
		client := osm.GetClient(ctx)
		resp, err := core.WithRetryFactory(ctx, requestFactory, client, core.DefaultRetryOptions)
		if err != nil {
			return nil, core.ServiceError("Nominatim", http.StatusServiceUnavailable, "Failed to communicate with geocoding service")
		}
		defer resp.Body.Close()

		// Handle error response
		if resp.StatusCode != http.StatusOK {
			return nil, core.ServiceError("Nominatim", resp.StatusCode, fmt.Sprintf("Geocoding service error: %d", resp.StatusCode))
		}

		// Parse response
		var results []NominatimResult
		if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
			return nil, core.NewError(core.ErrParseError, "Failed to decode geocoding response")
		}

		// Cache the results
		resultsJSON, err := json.Marshal(results)
		if err == nil {
			geocodeCache.Add(key, resultsJSON)
		}

		return results, nil
	})

	if err != nil {
		return nil, err
	}

	return result.([]NominatimResult), nil
}

// resultToPlace converts a Nominatim result to a Place object
func resultToPlace(result NominatimResult) (Place, error) {
	// Convert lat/lon to float64
	var lat, lon float64
	if _, err := fmt.Sscanf(result.Lat, "%f", &lat); err != nil {
		return Place{}, fmt.Errorf("failed to parse latitude: %w", err)
	}

	if _, err := fmt.Sscanf(result.Lon, "%f", &lon); err != nil {
		return Place{}, fmt.Errorf("failed to parse longitude: %w", err)
	}

	// Get city (could be in city or town field)
	city := result.Address.City
	if city == "" {
		city = result.Address.Town
	}

	// Create output
	place := Place{
		ID:   result.PlaceID.String(),
		Name: result.DisplayName,
		Location: Location{
			Latitude:  lat,
			Longitude: lon,
		},
		Address: Address{
			Formatted:   result.DisplayName,
			Street:      result.Address.Road,
			HouseNumber: result.Address.HouseNumber,
			City:        city,
			State:       result.Address.State,
			Country:     result.Address.Country,
			PostalCode:  result.Address.PostCode,
		},
		Importance: result.Importance,
	}

	return place, nil
}

// resultsToPlaces converts a slice of Nominatim results to Places
func resultsToPlaces(results []NominatimResult) ([]Place, error) {
	places := make([]Place, 0, len(results))

	for _, result := range results {
		place, err := resultToPlace(result)
		if err != nil {
			continue // Skip invalid results
		}
		places = append(places, place)
	}

	return places, nil
}

// HandleGeocodeAddress implements the geocoding functionality
//
// Side-effects: performs up to four HTTP GET requests (first + three retries),
// respects a 512-entry shared LRU cache, and annotates each outbound request
// with a descriptive User-Agent header.
func HandleGeocodeAddress(ctx context.Context, rawInput mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "geocode_address")

	// Parse input
	address := mcp.ParseString(rawInput, "address", "")
	region := mcp.ParseString(rawInput, "region", defaultRegion)

	// Log the original query for diagnostics
	logger.Info("geocoding address", "original_query", address, "region", region)

	if address == "" {
		return NewGeocodeDetailedError(
			"EMPTY_ADDRESS",
			"Address must not be empty",
			address,
			"Provide a specific address or place name",
			"Include city/region for better results",
		), nil
	}

	// Sanitize the address to improve search results
	withoutParens, parensContent := sanitizeAddress(address)
	logger.Info("sanitized query",
		"original", address,
		"without_parens", withoutParens,
		"parens_content", parensContent)

	// Keep track of the queries we'll try in order
	querySequence := []string{}

	// First query: If we have content outside parentheses, use it with region context
	if withoutParens != "" && withoutParens != address {
		querySequence = append(querySequence, ensureRegion(withoutParens, region))
	}

	// Second query: If we have content inside parentheses, use it with region context
	if parensContent != "" {
		querySequence = append(querySequence, ensureRegion(parensContent, region))
	}

	// Always include the full original query with region context
	querySequence = append(querySequence, ensureRegion(address, region))

	// Ensure we have unique queries
	seen := make(map[string]bool)
	uniqueQueries := []string{}

	for _, q := range querySequence {
		if !seen[q] {
			seen[q] = true
			uniqueQueries = append(uniqueQueries, q)
		}
	}

	// Try each query in sequence until we get results
	var allResults []NominatimResult
	var firstSuccess string
	var queryErr error

	for _, query := range uniqueQueries {
		logger.Info("trying query", "query", query)

		results, err := geocodeQuery(ctx, query)
		if err != nil {
			logger.Error("query failed", "query", query, "error", err)
			queryErr = err
			continue
		}

		if len(results) > 0 {
			allResults = results
			firstSuccess = query
			logger.Info("query succeeded", "query", query, "results", len(results))
			break
		}

		logger.Info("query returned no results", "query", query)
	}

	// Handle no results from any query
	if len(allResults) == 0 {
		logger.Info("all queries failed", "address", address)

		// Check if there was a specific error
		if queryErr != nil {
			if mcpErr, ok := queryErr.(*core.MCPError); ok {
				return NewGeocodeDetailedError(
					mcpErr.Code,
					mcpErr.Message,
					address,
					"Try again in a few moments",
				), nil
			}
		}

		// Generate helpful suggestions
		suggestions := []string{
			"Try a simpler query without special characters",
			"Include the city or country name",
		}

		// Add specific suggestions based on the query
		if strings.Contains(address, "(") && strings.Contains(address, ")") {
			suggestions = append(suggestions, "Remove content in parentheses")
		}

		if strings.Contains(address, ",") {
			suggestions = append(suggestions, "Try without commas")
		}

		// If it has multiple words and might be in a non-English language
		words := strings.Fields(address)
		if len(words) >= 2 {
			suggestions = append(suggestions, "For international locations, try official or local name")
			suggestions = append(suggestions, "For tourist sites, add the region or country name")
		}

		return NewGeocodeDetailedError(
			"NO_RESULTS",
			"No results found for the address",
			address,
			suggestions...,
		), nil
	}

	// Sort results by importance
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Importance > allResults[j].Importance
	})

	// Find the best result - either first result with importance > threshold or the top result
	bestResultIndex := 0
	for i, result := range allResults {
		if result.Importance >= minImportance {
			bestResultIndex = i
			break
		}
	}

	bestResult := allResults[bestResultIndex]
	logger.Info("selected best result",
		"importance", bestResult.Importance,
		"name", bestResult.DisplayName,
		"successful_query", firstSuccess)

	// Convert all results to places
	places, err := resultsToPlaces(allResults)
	if err != nil {
		logger.Error("failed to convert results to places", "error", err)
		return NewGeocodeDetailedError(
			"PARSE_ERROR",
			"Failed to process geocoding results",
			address,
		), nil
	}

	if len(places) == 0 {
		logger.Error("no valid places after conversion", "results", len(allResults))
		return NewGeocodeDetailedError(
			"PARSE_ERROR",
			"Failed to convert results to valid places",
			address,
		), nil
	}

	// Create output with best place and all candidates
	output := GeocodeAddressOutput{
		Place:      places[bestResultIndex],
		Candidates: places,
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return NewGeocodeDetailedError(
			"RESULT_ERROR",
			"Failed to generate result",
			address,
		), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}

// ReverseGeocodeInput defines the input parameters for reverse geocoding
type ReverseGeocodeInput struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// ReverseGeocodeOutput defines the output format for reverse geocoded coordinates
type ReverseGeocodeOutput struct {
	Place Place `json:"place"`
}

// ReverseGeocodeTool returns a tool definition for reverse geocoding
func ReverseGeocodeTool() mcp.Tool {
	return mcp.NewTool("reverse_geocode",
		mcp.WithDescription("Convert geographic coordinates to a human-readable address"),
		mcp.WithNumber("latitude",
			mcp.Required(),
			mcp.Description("The latitude coordinate as a decimal between -90 and 90"),
		),
		mcp.WithNumber("longitude",
			mcp.Required(),
			mcp.Description("The longitude coordinate as a decimal between -180 and 180"),
		),
	)
}

// HandleReverseGeocode implements the reverse geocoding functionality
//
// Side-effects: performs up to four HTTP GET requests (first + three retries),
// respects a 512-entry shared LRU cache, and annotates each outbound request
// with a descriptive User-Agent header.
func HandleReverseGeocode(ctx context.Context, rawInput mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "reverse_geocode")

	// Initialize caches if needed
	initCaches()

	// Parse and validate input coordinates
	latitude, longitude, err := core.ParseCoordsWithLog(rawInput, logger, "latitude", "longitude")
	if err != nil {
		// Extract the specific error code from the validation error
		errorCode := "INVALID_COORDINATES"
		if valErr, ok := err.(core.ValidationError); ok {
			errorCode = valErr.Code
		}

		return NewGeocodeDetailedError(
			errorCode,
			err.Error(),
			fmt.Sprintf("lat: %f, lon: %f", latitude, longitude),
			"Ensure coordinates are in decimal degrees",
		), nil
	}

	// Create a cache key
	key := reverseGeoCacheKey(latitude, longitude)

	// Check cache first
	if cachedData, found := reverseGeocodeCache.Get(key); found {
		logger.Info("cache hit", "key", key)

		var result struct {
			Place Place `json:"place"`
		}

		if err := json.Unmarshal(cachedData, &result); err != nil {
			logger.Error("failed to unmarshal cached results", "error", err)
		} else {
			resultBytes, err := json.Marshal(result)
			if err != nil {
				logger.Error("failed to marshal cached result", "error", err)
			} else {
				return mcp.NewToolResultText(string(resultBytes)), nil
			}
		}
	}

	// Use singleflight to deduplicate in-flight requests
	responseData, err, _ := requestGroup.Do(key, func() (interface{}, error) {
		// Build request URL
		reqURL, err := url.Parse(fmt.Sprintf("%s/reverse", nominatimBaseURL))
		if err != nil {
			return nil, core.NewError(core.ErrInternalError, "Failed to parse URL for geocoding service")
		}

		// Add query parameters
		q := reqURL.Query()
		q.Add("lat", fmt.Sprintf("%f", latitude))
		q.Add("lon", fmt.Sprintf("%f", longitude))
		q.Add("format", "json")
		q.Add("addressdetails", "1")
		reqURL.RawQuery = q.Encode()

		// Create HTTP request factory for retries
		requestFactory := func() (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("User-Agent", userAgent)
			return req, nil
		}

		// Execute request with retries
		client := osm.GetClient(ctx)
		resp, err := core.WithRetryFactory(ctx, requestFactory, client, core.DefaultRetryOptions)
		if err != nil {
			return nil, core.ServiceError("Nominatim", http.StatusServiceUnavailable, "Failed to communicate with geocoding service")
		}
		defer resp.Body.Close()

		// Handle error response
		if resp.StatusCode != http.StatusOK {
			return nil, core.ServiceError("Nominatim", resp.StatusCode, fmt.Sprintf("Geocoding service error: %d", resp.StatusCode))
		}

		// Parse response
		var result NominatimResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, core.NewError(core.ErrParseError, "Failed to decode geocoding response")
		}

		return result, nil
	})

	if err != nil {
		logger.Error("request failed", "error", err)
		if mcpErr, ok := err.(*core.MCPError); ok {
			return NewGeocodeDetailedError(
				mcpErr.Code,
				mcpErr.Message,
				fmt.Sprintf("lat: %f, lon: %f", latitude, longitude),
				"Try again in a few moments",
			), nil
		}
		return NewGeocodeDetailedError(
			"SERVICE_ERROR",
			"Failed to communicate with geocoding service",
			fmt.Sprintf("lat: %f, lon: %f", latitude, longitude),
			"Try again in a few moments",
		), nil
	}

	result := responseData.(NominatimResult)

	// Convert to Place
	place, err := resultToPlace(result)
	if err != nil {
		logger.Error("failed to convert result to place", "error", err)
		return NewGeocodeDetailedError(
			"PARSE_ERROR",
			"Failed to parse geocoding response",
			fmt.Sprintf("lat: %f, lon: %f", latitude, longitude),
		), nil
	}

	// Create output
	output := ReverseGeocodeOutput{
		Place: place,
	}

	// Cache the result
	outputJSON, err := json.Marshal(output)
	if err == nil {
		reverseGeocodeCache.Add(key, outputJSON)
	}

	return mcp.NewToolResultText(string(outputJSON)), nil
}

// Example end-to-end flow for "Merlion Park (Singapore)"
// 1. sanitizeAddress returns two siblings:
//    • "Merlion Park"
//    • "Singapore"
// 2. Engine sends first query with appended region, gets results.
// 3. Engine sorts results by importance and selects the best match.
// 4. Response cached and returned as structured JSON.
