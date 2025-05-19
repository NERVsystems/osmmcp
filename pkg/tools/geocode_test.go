package tools

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestSanitizeAddress(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		expectedWithout string
		expectedParens  string
	}{
		{
			name:            "Simple address",
			input:           "1600 Amphitheatre Parkway",
			expectedWithout: "1600 Amphitheatre Parkway",
			expectedParens:  "",
		},
		{
			name:            "With parentheses",
			input:           "Blue Temple (Wat Rong Suea Ten) in Chiang Rai",
			expectedWithout: "Blue Temple in Chiang Rai",
			expectedParens:  "Wat Rong Suea Ten",
		},
		{
			name:            "With extra spaces",
			input:           "  New   York   City  ",
			expectedWithout: "New York City",
			expectedParens:  "",
		},
		{
			name:            "With small content in parentheses",
			input:           "Empire State Building (NY)",
			expectedWithout: "Empire State Building",
			expectedParens:  "NY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withoutParens, parensContent := sanitizeAddress(tt.input)
			if withoutParens != tt.expectedWithout {
				t.Errorf("sanitizeAddress(%q) returned withoutParens = %q, want %q", tt.input, withoutParens, tt.expectedWithout)
			}
			if parensContent != tt.expectedParens {
				t.Errorf("sanitizeAddress(%q) returned parensContent = %q, want %q", tt.input, parensContent, tt.expectedParens)
			}
		})
	}
}

func TestHandleGeocodeAddress(t *testing.T) {
	tests := []struct {
		name        string
		address     string
		expectError bool
		errorCode   string // Added for testing error codes
	}{
		{
			name:        "Valid address",
			address:     "1600 Amphitheatre Parkway, Mountain View, CA",
			expectError: false,
		},
		{
			name:        "Empty address",
			address:     "",
			expectError: true,
			errorCode:   "EMPTY_ADDRESS",
		},
		{
			name:        "No results",
			address:     "NonexistentPlace123456789",
			expectError: true,
			errorCode:   "NO_RESULTS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test request
			req := mcp.CallToolRequest{
				Params: struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments,omitempty"`
					Meta      *mcp.Meta `json:"_meta,omitempty"`
				}{
					Name: "geocode_address",
					Arguments: map[string]any{
						"address": tt.address,
					},
				},
			}

			// Call handler
			result, err := HandleGeocodeAddress(context.Background(), req)

			// Check error cases
			if tt.expectError {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}
				if result == nil {
					t.Error("Expected error response, got nil")
					return
				}
				// Extract text content
				var contentText string
				for _, content := range result.Content {
					if text, ok := content.(mcp.TextContent); ok {
						contentText = text.Text
						break
					}
				}
				if contentText == "" {
					t.Error("Expected error message, got empty content")
					return
				}

				// Check for JSON in error response
				if contentText[0] == '{' {
					// Parse detailed error
					var detailedError GeocodeDetailedError
					if err := json.Unmarshal([]byte(contentText), &detailedError); err != nil {
						t.Errorf("Failed to parse detailed error: %v", err)
						return
					}

					// Verify error code if expected
					if tt.errorCode != "" && detailedError.Code != tt.errorCode {
						t.Errorf("Expected error code %q, got %q", tt.errorCode, detailedError.Code)
					}

					// Verify error has message
					if detailedError.Message == "" {
						t.Error("Expected non-empty error message")
					}

					// Check that query is included
					if detailedError.Query != tt.address {
						t.Errorf("Expected query %q in error, got %q", tt.address, detailedError.Query)
					}
				}

				return
			}

			// For successful cases, verify the response
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Error("Expected result, got nil")
				return
			}

			// Extract text content
			var contentText string
			for _, content := range result.Content {
				if text, ok := content.(mcp.TextContent); ok {
					contentText = text.Text
					break
				}
			}

			if contentText == "" {
				t.Error("No text content in result")
				return
			}

			// Parse the result
			var output struct {
				Place struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					Location struct {
						Latitude  float64 `json:"latitude"`
						Longitude float64 `json:"longitude"`
					} `json:"location"`
					Address struct {
						Street      string `json:"street"`
						HouseNumber string `json:"house_number"`
						City        string `json:"city"`
						State       string `json:"state"`
						Country     string `json:"country"`
						PostalCode  string `json:"postal_code"`
						Formatted   string `json:"formatted"`
					} `json:"address"`
				} `json:"place"`
			}

			if err := json.Unmarshal([]byte(contentText), &output); err != nil {
				t.Errorf("Failed to parse result: %v", err)
				return
			}

			// Verify the response structure
			if output.Place.ID == "" {
				t.Error("Expected non-empty place ID")
			}
			if output.Place.Name == "" {
				t.Error("Expected non-empty place name")
			}
			if output.Place.Location.Latitude == 0 || output.Place.Location.Longitude == 0 {
				t.Error("Expected non-zero coordinates")
			}
			if output.Place.Address.Formatted == "" {
				t.Error("Expected non-empty formatted address")
			}
		})
	}
}

func TestHandleReverseGeocode(t *testing.T) {
	tests := []struct {
		name        string
		latitude    float64
		longitude   float64
		expectError bool
		errorCode   string // Added for testing error codes
	}{
		{
			name:        "Valid coordinates",
			latitude:    37.7749,
			longitude:   -122.4194,
			expectError: false,
		},
		{
			name:        "Invalid latitude",
			latitude:    91.0,
			longitude:   -122.4194,
			expectError: true,
			errorCode:   "INVALID_LATITUDE",
		},
		{
			name:        "Invalid longitude",
			latitude:    37.7749,
			longitude:   181.0,
			expectError: true,
			errorCode:   "INVALID_LONGITUDE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test request
			req := mcp.CallToolRequest{
				Params: struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments,omitempty"`
					Meta      *mcp.Meta `json:"_meta,omitempty"`
				}{
					Name: "reverse_geocode",
					Arguments: map[string]any{
						"latitude":  tt.latitude,
						"longitude": tt.longitude,
					},
				},
			}

			// Call handler
			result, err := HandleReverseGeocode(context.Background(), req)

			// Check error cases
			if tt.expectError {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}
				if result == nil {
					t.Error("Expected error response, got nil")
					return
				}
				// Extract text content
				var contentText string
				for _, content := range result.Content {
					if text, ok := content.(mcp.TextContent); ok {
						contentText = text.Text
						break
					}
				}
				if contentText == "" {
					t.Error("Expected error message, got empty content")
					return
				}

				// Check for JSON in error response
				if contentText[0] == '{' {
					// Parse detailed error
					var detailedError GeocodeDetailedError
					if err := json.Unmarshal([]byte(contentText), &detailedError); err != nil {
						t.Errorf("Failed to parse detailed error: %v", err)
						return
					}

					// Verify error code if expected
					if tt.errorCode != "" && detailedError.Code != tt.errorCode {
						t.Errorf("Expected error code %q, got %q", tt.errorCode, detailedError.Code)
					}

					// Verify error has message
					if detailedError.Message == "" {
						t.Error("Expected non-empty error message")
					}
				}

				return
			}

			// For successful cases, verify the response
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Error("Expected result, got nil")
				return
			}

			// Extract text content
			var contentText string
			for _, content := range result.Content {
				if text, ok := content.(mcp.TextContent); ok {
					contentText = text.Text
					break
				}
			}

			if contentText == "" {
				t.Error("No text content in result")
				return
			}

			// Parse the result
			var output struct {
				Place struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					Location struct {
						Latitude  float64 `json:"latitude"`
						Longitude float64 `json:"longitude"`
					} `json:"location"`
					Address struct {
						Street      string `json:"street"`
						HouseNumber string `json:"house_number"`
						City        string `json:"city"`
						State       string `json:"state"`
						Country     string `json:"country"`
						PostalCode  string `json:"postal_code"`
						Formatted   string `json:"formatted"`
					} `json:"address"`
				} `json:"place"`
			}

			if err := json.Unmarshal([]byte(contentText), &output); err != nil {
				t.Errorf("Failed to parse result: %v", err)
				return
			}

			// Verify the response structure
			if output.Place.ID == "" {
				t.Error("Expected non-empty place ID")
			}
			if output.Place.Name == "" {
				t.Error("Expected non-empty place name")
			}
			if output.Place.Location.Latitude == 0 || output.Place.Location.Longitude == 0 {
				t.Error("Expected non-zero coordinates")
			}
			if output.Place.Address.Formatted == "" {
				t.Error("Expected non-empty formatted address")
			}
		})
	}
}

func TestParentheticalHandling(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	// Create test request with the test query for Merlion Park
	req := mcp.CallToolRequest{
		Params: struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments,omitempty"`
			Meta      *mcp.Meta `json:"_meta,omitempty"`
		}{
			Name: "geocode_address",
			Arguments: map[string]any{
				"address": "Merlion Park (Singapore)",
				"region":  "Singapore",
			},
		},
	}

	// Call handler
	result, err := HandleGeocodeAddress(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	// Extract text content
	var contentText string
	for _, content := range result.Content {
		if text, ok := content.(mcp.TextContent); ok {
			contentText = text.Text
			break
		}
	}

	if contentText == "" {
		t.Fatal("No text content in result")
	}

	// Parse the result
	var output GeocodeAddressOutput
	if err := json.Unmarshal([]byte(contentText), &output); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}

	// Check for expected values
	t.Logf("Found place: %s at coordinates: %f, %f",
		output.Place.Name,
		output.Place.Location.Latitude,
		output.Place.Location.Longitude)

	// The Merlion Park in Singapore is located approximately at these coordinates
	// We allow for some variance since the exact coordinates might change with data updates
	expectedLat := 1.2868
	expectedLon := 103.8545

	latDiff := math.Abs(output.Place.Location.Latitude - expectedLat)
	lonDiff := math.Abs(output.Place.Location.Longitude - expectedLon)

	// Check if the coordinates are within 0.1 degree (rough approximation)
	if latDiff > 0.1 || lonDiff > 0.1 {
		t.Errorf("Coordinates too far from expected: got (%f, %f), want near (%f, %f)",
			output.Place.Location.Latitude, output.Place.Location.Longitude,
			expectedLat, expectedLon)
	}
}
