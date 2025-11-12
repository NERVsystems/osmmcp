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

// CommuteOption represents a transportation option for commuting
type CommuteOption struct {
	Mode           string   `json:"mode"`                      // car, transit, walking, cycling
	Distance       float64  `json:"distance"`                  // in meters
	Duration       float64  `json:"duration"`                  // in seconds
	Summary        string   `json:"summary"`                   // brief description of the route
	Instructions   []string `json:"instructions,omitempty"`    // turn-by-turn directions
	CO2Emission    float64  `json:"co2_emission,omitempty"`    // in kg, if available
	CaloriesBurned float64  `json:"calories_burned,omitempty"` // if applicable (walking, cycling)
	Cost           float64  `json:"cost,omitempty"`            // estimated cost in local currency, if available
}

// CommuteAnalysis represents the full analysis of commute options
type CommuteAnalysis struct {
	HomeLocation      Location        `json:"home_location"`
	WorkLocation      Location        `json:"work_location"`
	CommuteOptions    []CommuteOption `json:"commute_options"`
	RecommendedOption string          `json:"recommended_option"` // e.g., "car", "transit", "cycling"
	Factors           []string        `json:"factors,omitempty"`  // factors considered in recommendation
}

// AnalyzeCommuteTool returns a tool definition for analyzing commute options
func AnalyzeCommuteTool() mcp.Tool {
	return mcp.NewTool("analyze_commute",
		mcp.WithDescription("Analyze transportation options between home and work locations"),
		mcp.WithNumber("home_latitude",
			mcp.Required(),
			mcp.Description("The latitude coordinate of the home location"),
		),
		mcp.WithNumber("home_longitude",
			mcp.Required(),
			mcp.Description("The longitude coordinate of the home location"),
		),
		mcp.WithNumber("work_latitude",
			mcp.Required(),
			mcp.Description("The latitude coordinate of the work location"),
		),
		mcp.WithNumber("work_longitude",
			mcp.Required(),
			mcp.Description("The longitude coordinate of the work location"),
		),
		mcp.WithArray("transport_modes",
			mcp.Description("Transport modes to analyze (car, cycling, walking)"),
			mcp.DefaultArray([]interface{}{"car", "cycling", "walking"}),
		),
	)
}

// ParseArray extracts an array parameter from a CallToolRequest
func ParseArray(req mcp.CallToolRequest, paramName string) ([]interface{}, error) {
	// Check if parameter exists
	param, ok := req.Params.Arguments[paramName]
	if !ok {
		return nil, core.NewError(core.ErrMissingParameter, fmt.Sprintf("parameter %s not found", paramName)).
			WithGuidance("Ensure all required parameters are provided in the request")
	}

	// Check if it's already an array
	if arr, ok := param.([]interface{}); ok {
		return arr, nil
	}

	// Try to convert from JSON
	jsonBytes, err := json.Marshal(param)
	if err != nil {
		return nil, core.NewError(core.ErrInternalError, "failed to marshal parameter").
			WithGuidance("The parameter could not be processed. Check the parameter format")
	}

	var result []interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, core.NewError(core.ErrParseError, "failed to parse array parameter").
			WithGuidance("The parameter must be a valid JSON array")
	}

	return result, nil
}

// HandleAnalyzeCommute implements commute analysis functionality
func HandleAnalyzeCommute(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "analyze_commute")

	// Parse input parameters
	homeLat := mcp.ParseFloat64(req, "home_latitude", 0)
	homeLon := mcp.ParseFloat64(req, "home_longitude", 0)
	workLat := mcp.ParseFloat64(req, "work_latitude", 0)
	workLon := mcp.ParseFloat64(req, "work_longitude", 0)

	// Get transport modes from request
	modesRaw, err := ParseArray(req, "transport_modes")
	if err != nil {
		modesRaw = []interface{}{"car", "cycling", "walking"}
	}

	// Convert modes to strings
	modes := make([]string, 0, len(modesRaw))
	for _, m := range modesRaw {
		if mode, ok := m.(string); ok {
			modes = append(modes, mode)
		}
	}

	// Add default modes if none are specified
	if len(modes) == 0 {
		modes = []string{"car", "cycling", "walking"}
	}

	// Basic validation
	if homeLat < -90 || homeLat > 90 || workLat < -90 || workLat > 90 {
		return ErrorResponse("Latitude must be between -90 and 90"), nil
	}
	if homeLon < -180 || homeLon > 180 || workLon < -180 || workLon > 180 {
		return ErrorResponse("Longitude must be between -180 and 180"), nil
	}

	// Initialize analysis result
	analysis := CommuteAnalysis{
		HomeLocation: Location{
			Latitude:  homeLat,
			Longitude: homeLon,
		},
		WorkLocation: Location{
			Latitude:  workLat,
			Longitude: workLon,
		},
		CommuteOptions: make([]CommuteOption, 0, len(modes)),
	}

	// Get routes for each mode
	for _, mode := range modes {
		// Map mode to OSRM profile
		profile := mapModeToProfile(mode)

		// Build OSRM request URL
		baseURL := fmt.Sprintf("%s/route/v1/%s", osm.OSRMBaseURL, profile)
		coordinates := fmt.Sprintf("%f,%f;%f,%f", homeLon, homeLat, workLon, workLat)

		reqURL, err := url.Parse(baseURL + "/" + coordinates)
		if err != nil {
			logger.Error("failed to parse URL", "error", err)
			continue
		}

		// Add query parameters
		q := reqURL.Query()
		q.Add("overview", "simplified") // Simplified geometry
		q.Add("steps", "true")          // Include turn-by-turn instructions
		reqURL.RawQuery = q.Encode()

		// Make HTTP request
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
		if err != nil {
			logger.Error("failed to create request", "error", err)
			continue
		}

		httpReq.Header.Set("User-Agent", osm.UserAgent)

		// Execute request
		client := osm.GetClient(ctx)
		resp, err := client.Do(httpReq)
		if err != nil {
			logger.Error("failed to execute request", "error", err)
			continue
		}

		// Process response
		if resp.StatusCode != http.StatusOK {
			logger.Error("routing service returned error", "status", resp.StatusCode)
			if err := resp.Body.Close(); err != nil {
				logger.Warn("failed to close response body", "error", err)
			}
			continue
		}

		// Parse OSRM response
		var osrmResp struct {
			Code   string `json:"code"`
			Routes []struct {
				Distance float64 `json:"distance"`
				Duration float64 `json:"duration"`
				Legs     []struct {
					Steps []struct {
						Distance float64 `json:"distance"`
						Duration float64 `json:"duration"`
						Name     string  `json:"name"`
						Maneuver struct {
							Type     string `json:"type"`
							Modifier string `json:"modifier,omitempty"`
						} `json:"maneuver"`
					} `json:"steps"`
				} `json:"legs"`
			} `json:"routes"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&osrmResp); err != nil {
			logger.Error("failed to decode response", "error", err)
			if err := resp.Body.Close(); err != nil {
				logger.Warn("failed to close response body", "error", err)
			}
			continue
		}
		if err := resp.Body.Close(); err != nil {
			logger.Warn("failed to close response body", "error", err)
		}

		// Check if any routes were found
		if len(osrmResp.Routes) == 0 {
			continue
		}

		// Get the best route (first one)
		osrmRoute := osrmResp.Routes[0]

		// Extract instructions if available
		instructions := make([]string, 0)
		if len(osrmRoute.Legs) > 0 {
			for _, step := range osrmRoute.Legs[0].Steps {
				instruction := generateInstruction(step.Maneuver.Type, step.Maneuver.Modifier, step.Name)
				if instruction != "" {
					instructions = append(instructions, instruction)
				}
			}
		}

		// Create commute option
		option := CommuteOption{
			Mode:         mode,
			Distance:     osrmRoute.Distance,
			Duration:     osrmRoute.Duration,
			Instructions: instructions,
		}

		// Add estimated CO2 emissions
		if mode == "car" {
			option.CO2Emission = osrmRoute.Distance / 1000 * CarCO2PerKm
		} else if mode == "transit" {
			option.CO2Emission = osrmRoute.Distance / 1000 * TransitCO2PerKm
		}

		// Add calories burned
		if mode == "walking" {
			option.CaloriesBurned = (osrmRoute.Distance / 1000) * WalkingCaloriesPerKm
		} else if mode == "cycling" {
			option.CaloriesBurned = (osrmRoute.Distance / 1000) * BikeCaloriesPerKm
		}

		// Generate summary
		durationMinutes := int(osrmRoute.Duration / 60)
		durationHours := durationMinutes / 60
		durationMinutesRemainder := durationMinutes % 60

		if durationHours > 0 {
			option.Summary = fmt.Sprintf("%s: %.1f km, %dh %dmin",
				strings.Title(mode), osrmRoute.Distance/1000, durationHours, durationMinutesRemainder)
		} else {
			option.Summary = fmt.Sprintf("%s: %.1f km, %d min",
				strings.Title(mode), osrmRoute.Distance/1000, durationMinutes)
		}

		// Add to options
		analysis.CommuteOptions = append(analysis.CommuteOptions, option)
	}

	// Determine recommended option based on simple heuristics
	if len(analysis.CommuteOptions) > 0 {
		// Sort options by different priorities
		fastestOption := ""
		fastestTime := float64(24 * 60 * 60) // 24 hours in seconds
		greenestOption := ""
		lowestEmission := float64(1000) // 1000kg CO2 as starting point
		healthiestOption := ""
		mostCalories := float64(0)

		for _, option := range analysis.CommuteOptions {
			// Find fastest option
			if option.Duration < fastestTime {
				fastestTime = option.Duration
				fastestOption = option.Mode
			}

			// Find greenest option (lowest CO2)
			if option.CO2Emission >= 0 && option.CO2Emission < lowestEmission {
				lowestEmission = option.CO2Emission
				greenestOption = option.Mode
			}

			// Find healthiest option (most calories)
			if option.CaloriesBurned > mostCalories {
				mostCalories = option.CaloriesBurned
				healthiestOption = option.Mode
			}
		}

		// Simple decision logic
		// If commute is under 3km, recommend walking or cycling (if available)
		if analysis.CommuteOptions[0].Distance < 3000 {
			if healthiestOption != "" {
				analysis.RecommendedOption = healthiestOption
				analysis.Factors = append(analysis.Factors, "Short distance ideal for active transportation")
				analysis.Factors = append(analysis.Factors, "Health benefits from physical activity")
			} else {
				analysis.RecommendedOption = fastestOption
				analysis.Factors = append(analysis.Factors, "Fastest commute time")
			}
		} else if analysis.CommuteOptions[0].Distance < 10000 {
			// For 3-10km, prefer cycling if available, otherwise fastest
			if healthiestOption == "cycling" {
				analysis.RecommendedOption = "cycling"
				analysis.Factors = append(analysis.Factors, "Medium distance ideal for cycling")
				analysis.Factors = append(analysis.Factors, "Health benefits from physical activity")
				analysis.Factors = append(analysis.Factors, "Lower environmental impact")
			} else {
				analysis.RecommendedOption = fastestOption
				analysis.Factors = append(analysis.Factors, "Fastest commute time")
			}
		} else {
			// For longer distances, prefer fastest option
			analysis.RecommendedOption = fastestOption
			analysis.Factors = append(analysis.Factors, "Fastest commute time for longer distance")

			// If fastest is car, mention environmental impact
			if fastestOption == "car" && greenestOption != "" && greenestOption != "car" {
				analysis.Factors = append(analysis.Factors, fmt.Sprintf("Consider %s for lower environmental impact", greenestOption))
			}
		}
	}

	// Create output
	output := struct {
		CommuteAnalysis CommuteAnalysis `json:"commute_analysis"`
	}{
		CommuteAnalysis: analysis,
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return ErrorResponse("Failed to generate result"), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}
