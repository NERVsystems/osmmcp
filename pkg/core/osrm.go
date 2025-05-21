// Package core provides shared utilities for the OpenStreetMap MCP tools.
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/NERVsystems/osmmcp/pkg/geo"
	"github.com/NERVsystems/osmmcp/pkg/osm"
	lru "github.com/hashicorp/golang-lru/v2"
)

const (
	// Default OSRM service base URL
	defaultOSRMBaseURL = "https://router.project-osrm.org"

	// Default cache size for route results
	defaultRouteCacheSize = 256

	// Default cache TTL for route results
	defaultRouteCacheTTL = 24 * time.Hour
)

var (
	// Global route cache
	routeCache     *lru.Cache[string, *OSRMResult]
	routeCacheOnce sync.Once
)

// OSRMOptions defines options for OSRM route requests
type OSRMOptions struct {
	// Base URL for the OSRM service
	BaseURL string

	// Profile to use (car, bike, foot)
	Profile string

	// Overview determines the geometry precision
	// "simplified", "full", "false"
	Overview string

	// Steps controls whether to return step-by-step instructions
	Steps bool

	// Annotations adds additional metadata to the route
	// "duration", "nodes", "distance", "speed", "weight", etc.
	Annotations []string

	// Alternatives controls whether to return alternative routes
	Alternatives int

	// Geometries controls the format of the returned geometry
	// "polyline", "polyline6", "geojson"
	Geometries string

	// Waypoints indices of waypoints to use (0-based)
	Waypoints []int

	// SampleInterval distance between samples in meters (if > 0, samples the route)
	SampleInterval float64

	// MaxAlternatives maximum number of alternatives to return
	MaxAlternatives int

	// Client is the HTTP client to use for requests
	Client *http.Client

	// RetryOptions controls retry behavior
	RetryOptions RetryOptions
}

// DefaultOSRMOptions returns reasonable defaults for OSRM requests
func DefaultOSRMOptions() OSRMOptions {
	return OSRMOptions{
		BaseURL:         defaultOSRMBaseURL,
		Profile:         "car",
		Overview:        "simplified",
		Steps:           false,
		Annotations:     nil,
		Alternatives:    0,
		Geometries:      "polyline",
		Waypoints:       nil,
		SampleInterval:  0,
		MaxAlternatives: 3,
		Client:          &http.Client{Timeout: 10 * time.Second},
		RetryOptions:    DefaultRetryOptions,
	}
}

// OSRMRoute represents a route returned by the OSRM service
type OSRMRoute struct {
	Duration   float64   `json:"duration"`    // Duration in seconds
	Distance   float64   `json:"distance"`    // Distance in meters
	Geometry   string    `json:"geometry"`    // Encoded polyline or GeoJSON
	Weight     float64   `json:"weight"`      // Weight value (typically duration)
	WeightName string    `json:"weight_name"` // Name of the weight metric
	Legs       []OSRMLeg `json:"legs"`        // Route legs between waypoints
}

// OSRMLeg represents a leg of a route between two waypoints
type OSRMLeg struct {
	Duration    float64    `json:"duration"` // Duration in seconds
	Distance    float64    `json:"distance"` // Distance in meters
	Summary     string     `json:"summary"`  // Summary of the leg
	Weight      float64    `json:"weight"`   // Weight value
	Steps       []OSRMStep `json:"steps"`    // Step-by-step instructions
	Annotations struct {
		Duration []float64 `json:"duration,omitempty"` // Duration per segment
		Distance []float64 `json:"distance,omitempty"` // Distance per segment
		Speed    []float64 `json:"speed,omitempty"`    // Speed per segment
		Weight   []float64 `json:"weight,omitempty"`   // Weight per segment
	} `json:"annotations,omitempty"`
}

// OSRMStep represents a single step in a route leg
type OSRMStep struct {
	Duration float64      `json:"duration"` // Duration in seconds
	Distance float64      `json:"distance"` // Distance in meters
	Name     string       `json:"name"`     // Road name
	Mode     string       `json:"mode"`     // Transport mode
	Geometry string       `json:"geometry"` // Encoded polyline
	Maneuver OSRMManeuver `json:"maneuver"` // Maneuver instructions
}

// OSRMManeuver represents a maneuver instruction
type OSRMManeuver struct {
	Type        string    `json:"type"`                  // Type of maneuver
	Modifier    string    `json:"modifier,omitempty"`    // Direction modifier
	Location    []float64 `json:"location"`              // Coordinates [lon, lat]
	Instruction string    `json:"instruction,omitempty"` // Human-readable instruction
}

// OSRMWaypoint represents a waypoint in the route
type OSRMWaypoint struct {
	Name     string    `json:"name"`     // Street name
	Location []float64 `json:"location"` // Coordinates [lon, lat]
	Distance float64   `json:"distance"` // Distance from requested coordinate
}

// OSRMResult represents the complete response from the OSRM service
type OSRMResult struct {
	Code      string         `json:"code"`      // Status code
	Message   string         `json:"message"`   // Error message if applicable
	Routes    []OSRMRoute    `json:"routes"`    // Array of routes
	Waypoints []OSRMWaypoint `json:"waypoints"` // Array of waypoints
}

// initCache initializes the route cache
func initCache() {
	routeCacheOnce.Do(func() {
		var err error
		routeCache, err = lru.New[string, *OSRMResult](defaultRouteCacheSize)
		if err != nil {
			routeCache, _ = lru.New[string, *OSRMResult](16) // Fallback to smaller cache
		}
	})
}

// cacheKey generates a cache key for a route request
func cacheKey(coordinates [][]float64, options OSRMOptions) string {
	// Build a string representing coordinates
	var coordsStr strings.Builder
	for i, coord := range coordinates {
		if i > 0 {
			coordsStr.WriteString(";")
		}
		coordsStr.WriteString(fmt.Sprintf("%.6f,%.6f", coord[0], coord[1]))
	}

	// Add key options that affect the route
	optStr := fmt.Sprintf("%s;%s;%v;%s;%d",
		options.Profile,
		options.Overview,
		options.Steps,
		options.Geometries,
		options.Alternatives)

	return coordsStr.String() + "|" + optStr
}

// GetRoute fetches a route from the OSRM service
func GetRoute(ctx context.Context, coordinates [][]float64, options OSRMOptions) (*OSRMResult, error) {
	logger := slog.Default().With("service", "osrm")

	// Initialize cache if needed
	initCache()

	// Create cache key
	key := cacheKey(coordinates, options)

	// Check cache first
	if cached, found := routeCache.Get(key); found {
		logger.Debug("route cache hit", "key", key)
		return cached, nil
	}

	logger.Debug("route cache miss", "key", key)

	// Build the coordinate string
	var coordStr strings.Builder
	for i, coord := range coordinates {
		if i > 0 {
			coordStr.WriteString(";")
		}
		// OSRM expects coordinates as longitude,latitude
		coordStr.WriteString(fmt.Sprintf("%.6f,%.6f", coord[0], coord[1]))
	}

	// Default BaseURL if not provided
	if options.BaseURL == "" {
		options.BaseURL = defaultOSRMBaseURL
	}

	// Default Client if not provided
	if options.Client == nil {
		options.Client = &http.Client{Timeout: 10 * time.Second}
	}

	// Build the request URL
	baseURL := fmt.Sprintf("%s/route/v1/%s/%s",
		strings.TrimRight(options.BaseURL, "/"),
		options.Profile,
		coordStr.String())

	reqURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	// Add query parameters
	query := reqURL.Query()
	query.Add("overview", options.Overview)
	query.Add("steps", fmt.Sprintf("%v", options.Steps))
	query.Add("geometries", options.Geometries)
	query.Add("alternatives", fmt.Sprintf("%d", options.Alternatives))

	// Add annotations if specified
	if len(options.Annotations) > 0 {
		query.Add("annotations", strings.Join(options.Annotations, ","))
	}

	// Add waypoints if specified
	if len(options.Waypoints) > 0 {
		var waypoints strings.Builder
		for i, wp := range options.Waypoints {
			if i > 0 {
				waypoints.WriteString(";")
			}
			waypoints.WriteString(fmt.Sprintf("%d", wp))
		}
		query.Add("waypoints", waypoints.String())
	}

	reqURL.RawQuery = query.Encode()

	// Create the request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, err
	}

	// Set User-Agent
	req.Header.Set("User-Agent", "OSM-MCP-Client/1.0")

	// Execute the request with retries
	resp, err := WithRetry(ctx, req, options.Client, options.RetryOptions)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Parse the response
	result := &OSRMResult{}
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return nil, err
	}

	// Check for OSRM error
	if result.Code != "Ok" {
		return nil, fmt.Errorf("OSRM error: %s", result.Message)
	}

	// Sample routes if requested
	if options.SampleInterval > 0 && len(result.Routes) > 0 {
		for i, rt := range result.Routes {
			pts := osm.DecodePolyline(rt.Geometry)
			sampled := samplePolyline(pts, options.SampleInterval)
			result.Routes[i].Geometry = osm.EncodePolyline(sampled)
		}
	}

	// Cache the result
	routeCache.Add(key, result)

	return result, nil
}

// Point represents a geographic point
type Point struct {
	Longitude float64
	Latitude  float64
}

// SimpleRoute represents a simplified route
type SimpleRoute struct {
	Distance     float64  `json:"distance"`               // Distance in meters
	Duration     float64  `json:"duration"`               // Duration in seconds
	Polyline     string   `json:"polyline"`               // Encoded polyline
	Points       []Point  `json:"points,omitempty"`       // Decoded points
	Summary      string   `json:"summary"`                // Route summary
	Instructions []string `json:"instructions,omitempty"` // Turn-by-turn instructions
}

// GetSimpleRoute is a convenience wrapper that returns a simplified route
func GetSimpleRoute(ctx context.Context, from, to []float64, mode string) (*SimpleRoute, error) {
	// Create options based on mode
	options := DefaultOSRMOptions()
	options.Profile = mode
	options.Steps = true // For turn-by-turn instructions

	// Get the route
	result, err := GetRoute(ctx, [][]float64{from, to}, options)
	if err != nil {
		return nil, err
	}

	// Check if we have any routes
	if len(result.Routes) == 0 {
		return nil, fmt.Errorf("no routes found")
	}

	// Use the first (best) route
	route := result.Routes[0]

	// Extract instructions
	instructions := make([]string, 0)
	if len(route.Legs) > 0 {
		for _, step := range route.Legs[0].Steps {
			instruction := formatInstruction(step)
			if instruction != "" {
				instructions = append(instructions, instruction)
			}
		}
	}

	// Create a summary from the first leg
	summary := ""
	if len(route.Legs) > 0 {
		summary = route.Legs[0].Summary
	}

	// Create the simple route
	simpleRoute := &SimpleRoute{
		Distance:     route.Distance,
		Duration:     route.Duration,
		Polyline:     route.Geometry,
		Summary:      summary,
		Instructions: instructions,
	}

	return simpleRoute, nil
}

// formatInstruction creates a human-readable instruction from a step
func formatInstruction(step OSRMStep) string {
	maneuver := step.Maneuver

	// Basic format: "{Type} {modifier} onto {name}"
	var instruction strings.Builder

	switch maneuver.Type {
	case "depart":
		instruction.WriteString("Start")
	case "arrive":
		instruction.WriteString("Arrive at destination")
	case "turn":
		instruction.WriteString("Turn")
		if maneuver.Modifier != "" {
			instruction.WriteString(" ")
			instruction.WriteString(maneuver.Modifier)
		}
	case "continue":
		instruction.WriteString("Continue")
		if maneuver.Modifier != "" {
			instruction.WriteString(" ")
			instruction.WriteString(maneuver.Modifier)
		}
	case "roundabout":
		instruction.WriteString("Enter roundabout")
	case "exit roundabout":
		instruction.WriteString("Exit roundabout")
	case "merge":
		instruction.WriteString("Merge")
		if maneuver.Modifier != "" {
			instruction.WriteString(" ")
			instruction.WriteString(maneuver.Modifier)
		}
	case "fork":
		instruction.WriteString("Keep")
		if maneuver.Modifier != "" {
			instruction.WriteString(" ")
			instruction.WriteString(maneuver.Modifier)
		}
		instruction.WriteString(" at fork")
	default:
		// If type doesn't match any of our cases, use the instruction directly
		if maneuver.Instruction != "" {
			return maneuver.Instruction
		}
		instruction.WriteString(maneuver.Type)
	}

	// Add the road name if available
	if step.Name != "" && step.Name != "-" {
		instruction.WriteString(" onto ")
		instruction.WriteString(step.Name)
	}

	// Add distance information
	if step.Distance > 0 {
		distanceStr := ""
		if step.Distance >= 1000 {
			distanceStr = fmt.Sprintf(" for %.1f km", step.Distance/1000)
		} else {
			distanceStr = fmt.Sprintf(" for %d m", int(step.Distance))
		}
		instruction.WriteString(distanceStr)
	}

	return instruction.String()
}

// samplePolyline resamples a slice of geographic points at the given interval.
// This mirrors the logic used by the route_sample tool and is used when
// SampleInterval is specified in OSRMOptions.
func samplePolyline(points []geo.Location, interval float64) []geo.Location {
	if len(points) < 2 || interval <= 0 {
		return points
	}

	result := []geo.Location{points[0]}
	current := points[0]
	remaining := interval

	for i := 0; i < len(points)-1; i++ {
		start := current
		end := points[i+1]
		for {
			segment := geo.HaversineDistance(start.Latitude, start.Longitude, end.Latitude, end.Longitude)
			if segment < remaining {
				remaining -= segment
				current = end
				break
			}

			frac := remaining / segment
			newPoint := geo.Location{
				Latitude:  start.Latitude + (end.Latitude-start.Latitude)*frac,
				Longitude: start.Longitude + (end.Longitude-start.Longitude)*frac,
			}
			result = append(result, newPoint)
			start = newPoint
			current = newPoint
			remaining = interval
		}
	}

	last := points[len(points)-1]
	if result[len(result)-1] != last {
		result = append(result, last)
	}

	return result
}
