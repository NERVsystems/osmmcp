package tools

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// ValidateOSMParameters validates common parameters used in OSM tools
// and returns detailed error messages to help users correct their input
func ValidateOSMParameters(req mcp.CallToolRequest, toolName string, logger *slog.Logger) (float64, float64, float64, int, *mcp.CallToolResult, error) {
	// Parse parameters as strings first to detect missing/malformed values
	latStr := mcp.ParseString(req, "latitude", "")
	lonStr := mcp.ParseString(req, "longitude", "")
	radiusStr := mcp.ParseString(req, "radius", "")
	limitStr := mcp.ParseString(req, "limit", "")

	// Validate required parameters
	if latStr == "" || lonStr == "" {
		missingParams := []string{}
		if latStr == "" {
			missingParams = append(missingParams, "latitude")
		}
		if lonStr == "" {
			missingParams = append(missingParams, "longitude")
		}

		logger.Error("missing required parameters", "missing", strings.Join(missingParams, ", "))

		// Return a detailed error with example
		return 0, 0, 0, 0, NewGeocodeDetailedError(
			"MISSING_PARAMETERS",
			fmt.Sprintf("Missing required parameters: %s", strings.Join(missingParams, ", ")),
			"",
			fmt.Sprintf("The %s tool requires both latitude and longitude parameters", toolName),
			fmt.Sprintf("Example: %s", GetToolUsageExample(toolName)),
		), fmt.Errorf("missing parameters")
	}

	// Parse and validate latitude
	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		logger.Error("invalid latitude", "input", latStr, "error", err)
		return 0, 0, 0, 0, NewGeocodeDetailedError(
			"INVALID_LATITUDE",
			fmt.Sprintf("Invalid latitude value: %s", latStr),
			"",
			"Latitude must be a valid number between -90 and 90",
			"Example: 40.7128 (numeric, no quotes)",
		), fmt.Errorf("invalid latitude")
	}

	// Parse and validate longitude
	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		logger.Error("invalid longitude", "input", lonStr, "error", err)
		return 0, 0, 0, 0, NewGeocodeDetailedError(
			"INVALID_LONGITUDE",
			fmt.Sprintf("Invalid longitude value: %s", lonStr),
			"",
			"Longitude must be a valid number between -180 and 180",
			"Example: -74.0060 (numeric, no quotes)",
		), fmt.Errorf("invalid longitude")
	}

	// Validate coordinates range
	if err := ValidateCoordinates(lat, lon); err != nil {
		logger.Error("coordinate validation failed", "error", err)
		return 0, 0, 0, 0, NewGeocodeDetailedError(
			"INVALID_COORDINATES",
			err.Error(),
			"",
			"Latitude must be between -90 and 90, longitude between -180 and 180",
		), fmt.Errorf("invalid coordinates")
	}

	// Parse radius with default
	var radius float64 = 1000 // Default radius
	if radiusStr != "" {
		radius, err = strconv.ParseFloat(radiusStr, 64)
		if err != nil {
			logger.Error("invalid radius", "input", radiusStr, "error", err)
			return 0, 0, 0, 0, NewGeocodeDetailedError(
				"INVALID_RADIUS",
				fmt.Sprintf("Invalid radius value: %s", radiusStr),
				"",
				"Radius must be a valid positive number",
				"Example: 1000 (numeric, no quotes)",
			), fmt.Errorf("invalid radius")
		}
	}

	// Validate radius range
	if err := ValidateRadius(radius, 5000); err != nil {
		logger.Error("radius validation failed", "radius", radius, "error", err)
		return 0, 0, 0, 0, NewGeocodeDetailedError(
			"INVALID_RADIUS",
			err.Error(),
			"",
			"Radius must be positive and less than 5000 meters",
		), fmt.Errorf("invalid radius range")
	}

	// Parse limit with default
	var limit int = 10 // Default limit
	if limitStr != "" {
		limitFloat, err := strconv.ParseFloat(limitStr, 64)
		if err != nil {
			logger.Error("invalid limit", "input", limitStr, "error", err)
			return 0, 0, 0, 0, NewGeocodeDetailedError(
				"INVALID_LIMIT",
				fmt.Sprintf("Invalid limit value: %s", limitStr),
				"",
				"Limit must be a valid positive number",
				"Example: 10 (numeric, no quotes)",
			), fmt.Errorf("invalid limit")
		}
		limit = int(limitFloat)
	}

	// Cap limit to reasonable range
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	return lat, lon, radius, limit, nil, nil
}

// ValidateRouteParameters validates parameters for routing functions
func ValidateRouteParameters(req mcp.CallToolRequest, logger *slog.Logger) (startLat, startLon, endLat, endLon float64, mode string, result *mcp.CallToolResult, err error) {
	// Parse start coordinates
	startLatStr := mcp.ParseString(req, "start_lat", "")
	startLonStr := mcp.ParseString(req, "start_lon", "")

	// Parse end coordinates
	endLatStr := mcp.ParseString(req, "end_lat", "")
	endLonStr := mcp.ParseString(req, "end_lon", "")

	// Parse transportation mode
	mode = mcp.ParseString(req, "mode", "car")

	// Validate required parameters
	if startLatStr == "" || startLonStr == "" || endLatStr == "" || endLonStr == "" {
		missingParams := []string{}
		if startLatStr == "" {
			missingParams = append(missingParams, "start_lat")
		}
		if startLonStr == "" {
			missingParams = append(missingParams, "start_lon")
		}
		if endLatStr == "" {
			missingParams = append(missingParams, "end_lat")
		}
		if endLonStr == "" {
			missingParams = append(missingParams, "end_lon")
		}

		logger.Error("missing required parameters", "missing", strings.Join(missingParams, ", "))
		result = NewGeocodeDetailedError(
			"MISSING_PARAMETERS",
			fmt.Sprintf("Missing required parameters: %s", strings.Join(missingParams, ", ")),
			"",
			"The get_route_directions tool requires start_lat, start_lon, end_lat, and end_lon parameters",
			"Example format: {\"start_lat\": 40.7128, \"start_lon\": -74.0060, \"end_lat\": 40.7580, \"end_lon\": -73.9855, \"mode\": \"car\"}",
		)
		return 0, 0, 0, 0, "", result, fmt.Errorf("missing parameters")
	}

	// Parse and validate start latitude
	startLat, err = strconv.ParseFloat(startLatStr, 64)
	if err != nil {
		logger.Error("invalid start latitude", "input", startLatStr, "error", err)
		result = NewGeocodeDetailedError(
			"INVALID_START_LATITUDE",
			fmt.Sprintf("Invalid start latitude value: %s", startLatStr),
			"",
			"start_lat must be a valid number between -90 and 90",
			"Example: 40.7128 (numeric, no quotes)",
		)
		return 0, 0, 0, 0, "", result, fmt.Errorf("invalid start latitude")
	}

	// Parse and validate start longitude
	startLon, err = strconv.ParseFloat(startLonStr, 64)
	if err != nil {
		logger.Error("invalid start longitude", "input", startLonStr, "error", err)
		result = NewGeocodeDetailedError(
			"INVALID_START_LONGITUDE",
			fmt.Sprintf("Invalid start longitude value: %s", startLonStr),
			"",
			"start_lon must be a valid number between -180 and 180",
			"Example: -74.0060 (numeric, no quotes)",
		)
		return 0, 0, 0, 0, "", result, fmt.Errorf("invalid start longitude")
	}

	// Validate start coordinates range
	if err = ValidateCoordinates(startLat, startLon); err != nil {
		logger.Error("start coordinate validation failed", "error", err)
		result = NewGeocodeDetailedError(
			"INVALID_START_COORDINATES",
			err.Error(),
			"",
			"start_lat must be between -90 and 90, start_lon between -180 and 180",
		)
		return 0, 0, 0, 0, "", result, fmt.Errorf("invalid start coordinates")
	}

	// Parse and validate end latitude
	endLat, err = strconv.ParseFloat(endLatStr, 64)
	if err != nil {
		logger.Error("invalid end latitude", "input", endLatStr, "error", err)
		result = NewGeocodeDetailedError(
			"INVALID_END_LATITUDE",
			fmt.Sprintf("Invalid end latitude value: %s", endLatStr),
			"",
			"end_lat must be a valid number between -90 and 90",
			"Example: 40.7580 (numeric, no quotes)",
		)
		return 0, 0, 0, 0, "", result, fmt.Errorf("invalid end latitude")
	}

	// Parse and validate end longitude
	endLon, err = strconv.ParseFloat(endLonStr, 64)
	if err != nil {
		logger.Error("invalid end longitude", "input", endLonStr, "error", err)
		result = NewGeocodeDetailedError(
			"INVALID_END_LONGITUDE",
			fmt.Sprintf("Invalid end longitude value: %s", endLonStr),
			"",
			"end_lon must be a valid number between -180 and 180",
			"Example: -73.9855 (numeric, no quotes)",
		)
		return 0, 0, 0, 0, "", result, fmt.Errorf("invalid end longitude")
	}

	// Validate end coordinates range
	if err = ValidateCoordinates(endLat, endLon); err != nil {
		logger.Error("end coordinate validation failed", "error", err)
		result = NewGeocodeDetailedError(
			"INVALID_END_COORDINATES",
			err.Error(),
			"",
			"end_lat must be between -90 and 90, end_lon between -180 and 180",
		)
		return 0, 0, 0, 0, "", result, fmt.Errorf("invalid end coordinates")
	}

	// Validate mode
	validModes := map[string]bool{"car": true, "bike": true, "foot": true}
	if !validModes[mode] {
		logger.Error("invalid mode", "mode", mode)
		result = NewGeocodeDetailedError(
			"INVALID_MODE",
			fmt.Sprintf("Invalid transportation mode: %s", mode),
			"",
			"mode must be one of: car, bike, foot",
			"Example: \"mode\": \"car\"",
		)
		return 0, 0, 0, 0, "", result, fmt.Errorf("invalid mode")
	}

	return startLat, startLon, endLat, endLon, mode, nil, nil
}
