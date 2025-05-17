// Package tools provides the OpenStreetMap MCP tools implementations.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
)

// InputParser is a generic function to parse request arguments into a strongly typed struct
func InputParser[T any](req mcp.CallToolRequest) (T, *mcp.CallToolResult, error) {
	var input T

	// Convert the arguments to JSON
	inputJSON, err := json.Marshal(req.Params.Arguments)
	if err != nil {
		return input, ErrorResponse(fmt.Sprintf("Invalid input format: %v", err)), err
	}

	// Parse into the specified type
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return input, ErrorResponse(fmt.Sprintf("Failed to parse input: %v", err)), err
	}

	return input, nil, nil
}

// WithParsedInput is a higher-order function that handles request parsing and error handling
func WithParsedInput[T any](
	handlerName string,
	handler func(ctx context.Context, input T, logger *slog.Logger) (interface{}, error),
) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		logger := slog.Default().With("tool", handlerName)

		// Parse the input
		input, errResult, err := InputParser[T](req)
		if err != nil {
			logger.Error("failed to parse input", "error", err)
			return errResult, nil
		}

		// Call the handler with the parsed input
		result, err := handler(ctx, input, logger)
		if err != nil {
			logger.Error("handler error", "error", err)
			return ErrorResponse(fmt.Sprintf("Failed to process request: %v", err)), nil
		}

		// Marshal the result
		resultBytes, err := json.Marshal(result)
		if err != nil {
			logger.Error("failed to marshal result", "error", err)
			return ErrorResponse("Failed to generate result"), nil
		}

		return mcp.NewToolResultText(string(resultBytes)), nil
	}
}

// ValidateCoordinates validates latitude and longitude are within valid ranges
func ValidateCoordinates(lat, lon float64) error {
	if lat < -90 || lat > 90 {
		return fmt.Errorf("latitude must be between -90 and 90, got %f", lat)
	}
	if lon < -180 || lon > 180 {
		return fmt.Errorf("longitude must be between -180 and 180, got %f", lon)
	}
	return nil
}

// ValidateRadius validates that a radius is positive and within the specified maximum
func ValidateRadius(radius, maxRadius float64) error {
	if radius <= 0 {
		return fmt.Errorf("radius must be greater than 0, got %f", radius)
	}
	if maxRadius > 0 && radius > maxRadius {
		return fmt.Errorf("radius must be less than or equal to %f, got %f", maxRadius, radius)
	}
	return nil
}

// WithValidatedCoordinates combines coordinate validation with input parsing
func WithValidatedCoordinates[T any](
	handlerName string,
	getCoords func(T) (lat, lon float64),
	handler func(ctx context.Context, input T, logger *slog.Logger) (interface{}, error),
) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return WithParsedInput(handlerName, func(ctx context.Context, input T, logger *slog.Logger) (interface{}, error) {
		// Extract and validate coordinates
		lat, lon := getCoords(input)
		if err := ValidateCoordinates(lat, lon); err != nil {
			return nil, err
		}

		// Call the handler with validated input
		return handler(ctx, input, logger)
	})
}

// Example usage:
/*
type FindNearbyPlacesInput struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Radius    float64 `json:"radius"`
	Category  string  `json:"category,omitempty"`
	Limit     int     `json:"limit,omitempty"`
}

// HandleFindNearbyPlaces would be implemented as:
func HandleFindNearbyPlaces(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return WithValidatedCoordinates[FindNearbyPlacesInput](
		"find_nearby_places",
		func(input FindNearbyPlacesInput) (float64, float64) {
			return input.Latitude, input.Longitude
		},
		func(ctx context.Context, input FindNearbyPlacesInput, logger *slog.Logger) (interface{}, error) {
			// Validate radius
			if err := ValidateRadius(input.Radius, 5000); err != nil {
				return nil, err
			}

			// Actual implementation logic...
			// ...

			return resultObject, nil
		},
	)(ctx, req)
}
*/
