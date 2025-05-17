// Package core provides shared utilities for the OpenStreetMap MCP tools.
package core

import (
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
)

// ValidationError represents a validation error for coordinates or other values
type ValidationError struct {
	Code     string
	Message  string
	Guidance string
}

// Error implements the error interface for ValidationError
func (e ValidationError) Error() string {
	if e.Guidance != "" {
		return fmt.Sprintf("%s: %s. %s", e.Code, e.Message, e.Guidance)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ValidateCoords checks if latitude and longitude are within valid ranges
func ValidateCoords(lat, lon float64) error {
	if lat < -90 || lat > 90 {
		return ValidationError{
			Code:     "INVALID_LATITUDE",
			Message:  fmt.Sprintf("Latitude must be between -90 and 90, got %f", lat),
			Guidance: "Ensure latitude is in decimal degrees",
		}
	}
	if lon < -180 || lon > 180 {
		return ValidationError{
			Code:     "INVALID_LONGITUDE",
			Message:  fmt.Sprintf("Longitude must be between -180 and 180, got %f", lon),
			Guidance: "Ensure longitude is in decimal degrees",
		}
	}
	return nil
}

// ValidateRadius checks if a radius is within the valid range
func ValidateRadius(radius, maxRadius float64) error {
	if radius <= 0 {
		return ValidationError{
			Code:     "INVALID_RADIUS",
			Message:  fmt.Sprintf("Radius must be greater than 0, got %f", radius),
			Guidance: "Specify a positive radius value",
		}
	}
	if maxRadius > 0 && radius > maxRadius {
		return ValidationError{
			Code:     "RADIUS_TOO_LARGE",
			Message:  fmt.Sprintf("Radius must be less than or equal to %f, got %f", maxRadius, radius),
			Guidance: fmt.Sprintf("Specify a radius less than %f", maxRadius),
		}
	}
	return nil
}

// ParseCoords extracts and validates latitude and longitude from a CallToolRequest
// It allows specifying alternative key names for latitude and longitude
func ParseCoords(req mcp.CallToolRequest, latKey, lonKey string) (float64, float64, error) {
	// Default keys if not specified
	if latKey == "" {
		latKey = "latitude"
	}
	if lonKey == "" {
		lonKey = "longitude"
	}

	// Extract values
	lat := mcp.ParseFloat64(req, latKey, 0)
	lon := mcp.ParseFloat64(req, lonKey, 0)

	// Validate coordinates
	if err := ValidateCoords(lat, lon); err != nil {
		return 0, 0, err
	}

	return lat, lon, nil
}

// ParseRadius extracts and validates a radius from a CallToolRequest
func ParseRadius(req mcp.CallToolRequest, key string, defaultRadius, maxRadius float64) (float64, error) {
	// Default key if not specified
	if key == "" {
		key = "radius"
	}

	// Extract value
	radius := mcp.ParseFloat64(req, key, defaultRadius)

	// Validate radius
	if err := ValidateRadius(radius, maxRadius); err != nil {
		return 0, err
	}

	return radius, nil
}

// ParseCoordsAndRadius combines coordinate and radius parsing with validation
func ParseCoordsAndRadius(req mcp.CallToolRequest, latKey, lonKey, radiusKey string, defaultRadius, maxRadius float64) (lat, lon, radius float64, err error) {
	lat, lon, err = ParseCoords(req, latKey, lonKey)
	if err != nil {
		return 0, 0, 0, err
	}

	radius, err = ParseRadius(req, radiusKey, defaultRadius, maxRadius)
	if err != nil {
		return lat, lon, 0, err
	}

	return lat, lon, radius, nil
}

// ParseCoordsWithLog parses coordinates and logs any errors
func ParseCoordsWithLog(req mcp.CallToolRequest, logger *slog.Logger, latKey, lonKey string) (float64, float64, error) {
	lat, lon, err := ParseCoords(req, latKey, lonKey)
	if err != nil {
		logger.Error("invalid coordinates", "error", err)
	}
	return lat, lon, err
}

// ParseRadiusWithLog parses radius and logs any errors
func ParseRadiusWithLog(req mcp.CallToolRequest, logger *slog.Logger, key string, defaultRadius, maxRadius float64) (float64, error) {
	radius, err := ParseRadius(req, key, defaultRadius, maxRadius)
	if err != nil {
		logger.Error("invalid radius", "error", err)
	}
	return radius, err
}

// ParseCoordsAndRadiusWithLog combines coordinate and radius parsing with logging
func ParseCoordsAndRadiusWithLog(req mcp.CallToolRequest, logger *slog.Logger, latKey, lonKey, radiusKey string, defaultRadius, maxRadius float64) (lat, lon, radius float64, err error) {
	lat, lon, err = ParseCoordsWithLog(req, logger, latKey, lonKey)
	if err != nil {
		return 0, 0, 0, err
	}

	radius, err = ParseRadiusWithLog(req, logger, radiusKey, defaultRadius, maxRadius)
	if err != nil {
		return lat, lon, 0, err
	}

	return lat, lon, radius, nil
}
