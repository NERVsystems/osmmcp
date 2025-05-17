// Package core provides shared utilities for the OpenStreetMap MCP tools.
package core

import (
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// ValidationError represents a validation error with a specific code and message
type ValidationError struct {
	Code    string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ValidateCoords validates latitude and longitude coordinates
func ValidateCoords(lat, lon float64) error {
	if math.IsNaN(lat) || math.IsNaN(lon) {
		return ValidationError{
			Code:    "INVALID_COORDINATES",
			Message: "Coordinates must be valid numbers",
		}
	}

	if lat < -90 || lat > 90 {
		return ValidationError{
			Code:    "INVALID_LATITUDE",
			Message: "Latitude must be between -90 and 90 degrees",
		}
	}

	if lon < -180 || lon > 180 {
		return ValidationError{
			Code:    "INVALID_LONGITUDE",
			Message: "Longitude must be between -180 and 180 degrees",
		}
	}

	return nil
}

// ValidateRadius validates a search radius
func ValidateRadius(radius, maxRadius float64) error {
	if math.IsNaN(radius) {
		return ValidationError{
			Code:    "INVALID_RADIUS",
			Message: "Radius must be a valid number",
		}
	}

	if radius <= 0 {
		return ValidationError{
			Code:    "INVALID_RADIUS",
			Message: "Radius must be greater than 0",
		}
	}

	if radius > maxRadius {
		return ValidationError{
			Code:    "RADIUS_TOO_LARGE",
			Message: fmt.Sprintf("Radius must not exceed %.0f meters", maxRadius),
		}
	}

	return nil
}

// ParseCoords parses and validates coordinates from a request
func ParseCoords(req mcp.CallToolRequest, latKey, lonKey string) (float64, float64, error) {
	latStr := mcp.ParseString(req, latKey, "")
	lonStr := mcp.ParseString(req, lonKey, "")

	if latStr == "" || lonStr == "" {
		return 0, 0, ValidationError{
			Code:    "MISSING_COORDINATES",
			Message: fmt.Sprintf("Both %s and %s are required", latKey, lonKey),
		}
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		return 0, 0, ValidationError{
			Code:    "INVALID_LATITUDE",
			Message: fmt.Sprintf("Invalid latitude value: %s", latStr),
		}
	}

	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		return 0, 0, ValidationError{
			Code:    "INVALID_LONGITUDE",
			Message: fmt.Sprintf("Invalid longitude value: %s", lonStr),
		}
	}

	if err := ValidateCoords(lat, lon); err != nil {
		return 0, 0, err
	}

	return lat, lon, nil
}

// ParseRadius parses and validates a radius parameter
func ParseRadius(req mcp.CallToolRequest, key string, defaultRadius, maxRadius float64) (float64, error) {
	radiusStr := mcp.ParseString(req, key, "")

	// Use default if not provided
	if radiusStr == "" {
		return defaultRadius, nil
	}

	radius, err := strconv.ParseFloat(radiusStr, 64)
	if err != nil {
		return 0, ValidationError{
			Code:    "INVALID_RADIUS",
			Message: fmt.Sprintf("Invalid radius value: %s", radiusStr),
		}
	}

	if err := ValidateRadius(radius, maxRadius); err != nil {
		return 0, err
	}

	return radius, nil
}

// ParseCoordsAndRadius parses and validates coordinates and radius
func ParseCoordsAndRadius(req mcp.CallToolRequest, latKey, lonKey, radiusKey string, defaultRadius, maxRadius float64) (lat, lon, radius float64, err error) {
	lat, lon, err = ParseCoords(req, latKey, lonKey)
	if err != nil {
		return 0, 0, 0, err
	}

	radius, err = ParseRadius(req, radiusKey, defaultRadius, maxRadius)
	if err != nil {
		return 0, 0, 0, err
	}

	return lat, lon, radius, nil
}

// ParseCoordsWithLog parses coordinates with logging
func ParseCoordsWithLog(req mcp.CallToolRequest, logger *slog.Logger, latKey, lonKey string) (float64, float64, error) {
	lat, lon, err := ParseCoords(req, latKey, lonKey)
	if err != nil {
		logger.Error("coordinate validation failed",
			"error", err,
			"lat_key", latKey,
			"lon_key", lonKey,
		)
		return 0, 0, err
	}

	logger.Debug("coordinates validated",
		"latitude", lat,
		"longitude", lon,
	)
	return lat, lon, nil
}

// ParseRadiusWithLog parses radius with logging
func ParseRadiusWithLog(req mcp.CallToolRequest, logger *slog.Logger, key string, defaultRadius, maxRadius float64) (float64, error) {
	radius, err := ParseRadius(req, key, defaultRadius, maxRadius)
	if err != nil {
		logger.Error("radius validation failed",
			"error", err,
			"key", key,
			"default", defaultRadius,
			"max", maxRadius,
		)
		return 0, err
	}

	logger.Debug("radius validated",
		"radius", radius,
		"key", key,
	)
	return radius, nil
}

// ParseCoordsAndRadiusWithLog parses coordinates and radius with logging
func ParseCoordsAndRadiusWithLog(req mcp.CallToolRequest, logger *slog.Logger, latKey, lonKey, radiusKey string, defaultRadius, maxRadius float64) (lat, lon, radius float64, err error) {
	lat, lon, radius, err = ParseCoordsAndRadius(req, latKey, lonKey, radiusKey, defaultRadius, maxRadius)
	if err != nil {
		logger.Error("coordinate and radius validation failed",
			"error", err,
			"lat_key", latKey,
			"lon_key", lonKey,
			"radius_key", radiusKey,
		)
		return 0, 0, 0, err
	}

	logger.Debug("coordinates and radius validated",
		"latitude", lat,
		"longitude", lon,
		"radius", radius,
	)
	return lat, lon, radius, nil
}

// SanitizeString removes potentially dangerous characters from a string
func SanitizeString(s string) string {
	// Remove control characters
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, s)

	// Trim whitespace
	return strings.TrimSpace(s)
}

// ValidateStringLength checks if a string is within acceptable length bounds
func ValidateStringLength(s string, min, max int) error {
	length := len(s)
	if length < min {
		return ValidationError{
			Code:    "STRING_TOO_SHORT",
			Message: fmt.Sprintf("String must be at least %d characters long", min),
		}
	}
	if length > max {
		return ValidationError{
			Code:    "STRING_TOO_LONG",
			Message: fmt.Sprintf("String must not exceed %d characters", max),
		}
	}
	return nil
}

// ValidateNumericRange checks if a number is within acceptable bounds
func ValidateNumericRange(n float64, min, max float64) error {
	if n < min {
		return ValidationError{
			Code:    "VALUE_TOO_SMALL",
			Message: fmt.Sprintf("Value must be at least %g", min),
		}
	}
	if n > max {
		return ValidationError{
			Code:    "VALUE_TOO_LARGE",
			Message: fmt.Sprintf("Value must not exceed %g", max),
		}
	}
	return nil
}
