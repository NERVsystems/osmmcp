package tools

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHandleAnalyzeNeighborhood_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		latitude    float64
		longitude   float64
		radius      float64
		expectError bool
		description string
	}{
		{
			name:        "Invalid latitude - too low",
			latitude:    -91.0,
			longitude:   -74.0060,
			radius:      1000.0,
			expectError: true,
			description: "Should reject latitude below -90",
		},
		{
			name:        "Invalid latitude - too high",
			latitude:    91.0,
			longitude:   -74.0060,
			radius:      1000.0,
			expectError: true,
			description: "Should reject latitude above 90",
		},
		{
			name:        "Invalid longitude - too low",
			latitude:    40.7128,
			longitude:   -181.0,
			radius:      1000.0,
			expectError: true,
			description: "Should reject longitude below -180",
		},
		{
			name:        "Invalid longitude - too high",
			latitude:    40.7128,
			longitude:   181.0,
			radius:      1000.0,
			expectError: true,
			description: "Should reject longitude above 180",
		},
		{
			name:        "Invalid radius - zero",
			latitude:    40.7128,
			longitude:   -74.0060,
			radius:      0.0,
			expectError: true,
			description: "Should reject radius of 0",
		},
		{
			name:        "Invalid radius - negative",
			latitude:    40.7128,
			longitude:   -74.0060,
			radius:      -100.0,
			expectError: true,
			description: "Should reject negative radius",
		},
		{
			name:        "Invalid radius - too large",
			latitude:    40.7128,
			longitude:   -74.0060,
			radius:      3000.0,
			expectError: true,
			description: "Should reject radius above 2000",
		},
		{
			name:        "Valid parameters",
			latitude:    40.7128,
			longitude:   -74.0060,
			radius:      1000.0,
			expectError: false,
			description: "Should accept valid parameters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test request
			req := mcp.CallToolRequest{
				Params: struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments,omitempty"`
					Meta      *mcp.Meta      `json:"_meta,omitempty"`
				}{
					Name: "analyze_neighborhood",
					Arguments: map[string]any{
						"latitude":           tt.latitude,
						"longitude":          tt.longitude,
						"radius":             tt.radius,
						"neighborhood_name":  "Test Area",
						"include_price_data": true,
					},
				},
			}

			// Call handler
			result, err := HandleAnalyzeNeighborhood(context.Background(), req)

			// Check for unexpected errors
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// For expected error case
			if tt.expectError {
				AssertErrorResult(t, result, tt.description)
				// Verify the error message contains useful information
				if result.Content != nil && len(result.Content) > 0 {
					if text, ok := result.Content[0].(mcp.TextContent); ok {
						errorText := text.Text
						// Check that the error message is not generic
						if errorText == "Failed to communicate with OSM service" {
							t.Errorf("Error message is too generic: %s", errorText)
						}
						// Check that the error message contains helpful guidance
						if len(errorText) < 10 {
							t.Errorf("Error message is too short to be helpful: %s", errorText)
						}
					}
				}
				return
			}

			// Should not be an error result for valid parameters
			// Note: This may still fail due to network issues or API unavailability
			// In a real test environment, we would mock the HTTP client
			if IsErrorResult(result) {
				// For valid parameters, we expect either success or a meaningful error
				if result.Content != nil && len(result.Content) > 0 {
					if text, ok := result.Content[0].(mcp.TextContent); ok {
						errorText := text.Text
						// Verify the error has proper structure if it's an API error
						if errorText == "Failed to communicate with OSM service" {
							t.Errorf("Got generic error message for valid parameters: %s", errorText)
						}
						// If it's a network error, it should contain guidance
						if len(errorText) < 50 {
							t.Errorf("Error message for valid parameters lacks detail: %s", errorText)
						}
					}
				}
			}
		})
	}
}

func TestAnalyzeNeighborhood_ScoreCalculation(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		expected int
		function func(...int) int
	}{
		{
			name:     "Walk score calculation",
			input:    []int{5, 3, 2, 1, 2, 10}, // shops, restaurants, cafes, parks, pharmacies, footpaths
			expected: 11,                       // (5*2 + 3*2 + 2 + 1*3 + 2*2 + 10) / 3 = 35/3 = 11
			function: func(args ...int) int {
				return calculateWalkScore(args[0], args[1], args[2], args[3], args[4], args[5])
			},
		},
		{
			name:     "Transit score calculation",
			input:    []int{5}, // transitStops
			expected: 50,       // 5 * 10 = 50
			function: func(args ...int) int {
				return calculateTransitScore(args[0])
			},
		},
		{
			name:     "Score boundary test - over 100",
			input:    []int{50}, // Large number of transit stops
			expected: 100,       // Should be capped at 100
			function: func(args ...int) int {
				return calculateTransitScore(args[0])
			},
		},
		{
			name:     "Score boundary test - under 0",
			input:    []int{0, 0, 0, 0, 0, 0}, // No amenities
			expected: 0,                       // Should be at least 0
			function: func(args ...int) int {
				return calculateWalkScore(args[0], args[1], args[2], args[3], args[4], args[5])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.function(tt.input...)
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestIdentifyKeyIssues(t *testing.T) {
	tests := []struct {
		name     string
		scores   []int
		expected []string
	}{
		{
			name:     "No issues - all scores good",
			scores:   []int{80, 70, 60, 50, 40, 60, 70, 80, 90},
			expected: []string{},
		},
		{
			name:     "Multiple issues - low scores",
			scores:   []int{20, 25, 15, 40, 50, 60, 10, 80, 90},
			expected: []string{"Limited walkability", "Limited biking infrastructure", "Limited public transit", "Limited recreation facilities"},
		},
		{
			name:     "Single issue - transit",
			scores:   []int{80, 70, 20, 50, 40, 60, 70, 80, 90},
			expected: []string{"Limited public transit"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := identifyKeyIssues(tt.scores...)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d issues, got %d", len(tt.expected), len(result))
			}
			for i, expectedIssue := range tt.expected {
				if i >= len(result) {
					t.Errorf("Missing expected issue: %s", expectedIssue)
					continue
				}
				if result[i] != expectedIssue {
					t.Errorf("Expected issue %s, got %s", expectedIssue, result[i])
				}
			}
		})
	}
}
