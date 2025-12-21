package tools

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/NERVsystems/osmmcp/pkg/geo"
	"github.com/NERVsystems/osmmcp/pkg/osm"
)

func TestHandleRouteSample(t *testing.T) {
	// Setup test data
	// Use a simple polyline that represents a straight line path
	simplePolyline := osm.EncodePolyline([]geo.Location{
		{Latitude: 40.0, Longitude: -74.0},
		{Latitude: 42.0, Longitude: -74.0},
	})

	tests := []struct {
		name         string
		polyline     string
		interval     float64
		expectError  bool
		expectedSize int
	}{
		{
			name:         "Valid sampling",
			polyline:     simplePolyline,
			interval:     50000, // 50km, should get 5-6 points for ~222km route
			expectError:  false,
			expectedSize: 6,
		},
		{
			name:         "Empty polyline",
			polyline:     "",
			interval:     1000,
			expectError:  true,
			expectedSize: 0,
		},
		{
			name:         "Zero interval",
			polyline:     simplePolyline,
			interval:     0,
			expectError:  true,
			expectedSize: 0,
		},
		{
			name:         "Negative interval",
			polyline:     simplePolyline,
			interval:     -1000,
			expectError:  true,
			expectedSize: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test request
			req := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name: "route_sample",
					Arguments: map[string]any{
						"polyline": tt.polyline,
						"interval": tt.interval,
					},
				},
			}

			// Call handler
			result, err := HandleRouteSample(context.Background(), req)

			// Check for unexpected errors
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// For expected error case
			if tt.expectError {
				AssertErrorResult(t, result, "Expected error result, but got success")
				return
			}

			// Should not be an error result
			AssertSuccessResult(t, result, "Expected success result, but got error")

			// Unmarshal result
			var output RouteSampleOutput
			if err := ParseResultJSON(result, &output); err != nil {
				t.Fatalf("Failed to unmarshal result: %v", err)
			}

			// Check point count
			if tt.expectedSize > 0 && len(output.Points) != tt.expectedSize {
				t.Errorf("Expected around %d points, got %d", tt.expectedSize, len(output.Points))
			}

			// Verify points are in order along the route
			if len(output.Points) > 1 {
				for i := 1; i < len(output.Points); i++ {
					// For this simple north-south test route, latitude should be increasing
					if output.Points[i].Latitude < output.Points[i-1].Latitude {
						t.Errorf("Points not in correct order: point %d lat %f < point %d lat %f",
							i, output.Points[i].Latitude, i-1, output.Points[i-1].Latitude)
					}
				}
			}
		})
	}
}

func TestHandleEnrichEmissions(t *testing.T) {
	tests := []struct {
		name    string
		options []struct {
			Mode     string  `json:"mode"`
			Distance float64 `json:"distance"`
			Duration float64 `json:"duration,omitempty"`
		}
		expectError     bool
		expectedResults int
		expectedFields  []string // Which fields should be present in the result
	}{
		{
			name: "Multiple transport modes",
			options: []struct {
				Mode     string  `json:"mode"`
				Distance float64 `json:"distance"`
				Duration float64 `json:"duration,omitempty"`
			}{
				{
					Mode:     "car",
					Distance: 10000, // 10km
					Duration: 900,   // 15 minutes
				},
				{
					Mode:     "walking",
					Distance: 5000, // 5km
					Duration: 3600, // 1 hour
				},
				{
					Mode:     "cycling",
					Distance: 8000, // 8km
					Duration: 1800, // 30 minutes
				},
			},
			expectError:     false,
			expectedResults: 3,
			expectedFields:  []string{"co2_kg", "calories_kcal", "cost_local"},
		},
		{
			name: "Empty options",
			options: []struct {
				Mode     string  `json:"mode"`
				Distance float64 `json:"distance"`
				Duration float64 `json:"duration,omitempty"`
			}{},
			expectError:     true,
			expectedResults: 0,
		},
		{
			name: "Invalid mode",
			options: []struct {
				Mode     string  `json:"mode"`
				Distance float64 `json:"distance"`
				Duration float64 `json:"duration,omitempty"`
			}{
				{
					Mode:     "invalid",
					Distance: 10000,
					Duration: 900,
				},
			},
			expectError:     false, // Should handle invalid mode gracefully
			expectedResults: 1,
			expectedFields:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test request
			req := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name: "enrich_emissions",
					Arguments: map[string]any{
						"options": tt.options,
					},
				},
			}

			// Call handler
			result, err := HandleEnrichEmissions(context.Background(), req)

			// Check for unexpected errors
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// For expected error case
			if tt.expectError {
				AssertErrorResult(t, result, "Expected error result, but got success")
				return
			}

			// Should not be an error result
			AssertSuccessResult(t, result, "Expected success result, but got error")

			// Unmarshal result
			var output EnrichEmissionsOutput
			if err := ParseResultJSON(result, &output); err != nil {
				t.Fatalf("Failed to unmarshal result: %v", err)
			}

			// Check option count
			if len(output.Options) != tt.expectedResults {
				t.Errorf("Expected %d options, got %d", tt.expectedResults, len(output.Options))
			}

			// Skip further checks if no options expected
			if tt.expectedResults == 0 {
				return
			}

			// Check for car mode emission data
			for _, option := range output.Options {
				if option.Mode == "car" && tt.expectedFields != nil {
					// Car should have CO2 data
					for _, field := range tt.expectedFields {
						switch field {
						case "co2_kg":
							if option.CO2Kg <= 0 {
								t.Errorf("Expected CO2 emissions for car, got %f", option.CO2Kg)
							}
						case "cost_local":
							// Cost might be 0 in some cases, but should be present
							t.Logf("Car cost: %f", option.CostLocal)
						}
					}
				}

				if option.Mode == "walking" && tt.expectedFields != nil {
					// Walking should have calorie data
					for _, field := range tt.expectedFields {
						if field == "calories_kcal" && option.CaloriesKcal <= 0 {
							t.Errorf("Expected calories for walking, got %f", option.CaloriesKcal)
						}
					}
				}

				if option.Mode == "cycling" && tt.expectedFields != nil {
					// Cycling should have calorie data
					for _, field := range tt.expectedFields {
						if field == "calories_kcal" && option.CaloriesKcal <= 0 {
							t.Errorf("Expected calories for cycling, got %f", option.CaloriesKcal)
						}
					}
				}
			}
		})
	}
}

// Note: TestHandleRouteFetch is omitted because it would require mocking the OSRM API
