package tools

import (
	"context"
	"math"
	"testing"

	"github.com/NERVsystems/osmmcp/pkg/geo"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestHandleGeoDistance(t *testing.T) {
	tests := []struct {
		name        string
		from        geo.Location
		to          geo.Location
		expectError bool
		expected    float64
	}{
		{
			name:        "Valid coordinates",
			from:        geo.Location{Latitude: 40.7128, Longitude: -74.0060},  // New York
			to:          geo.Location{Latitude: 34.0522, Longitude: -118.2437}, // Los Angeles
			expectError: false,
			expected:    3935740.0, // ~3935km (allow small variation in floating point calc)
		},
		{
			name:        "Same point",
			from:        geo.Location{Latitude: 40.7128, Longitude: -74.0060},
			to:          geo.Location{Latitude: 40.7128, Longitude: -74.0060},
			expectError: false,
			expected:    0.0,
		},
		{
			name:        "Invalid from latitude",
			from:        geo.Location{Latitude: 91.0, Longitude: -74.0060},
			to:          geo.Location{Latitude: 34.0522, Longitude: -118.2437},
			expectError: true,
			expected:    0.0,
		},
		{
			name:        "Invalid to longitude",
			from:        geo.Location{Latitude: 40.7128, Longitude: -74.0060},
			to:          geo.Location{Latitude: 34.0522, Longitude: -181.0},
			expectError: true,
			expected:    0.0,
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
					Name: "geo_distance",
					Arguments: map[string]any{
						"from": tt.from,
						"to":   tt.to,
					},
				},
			}

			// Call handler
			result, err := HandleGeoDistance(context.Background(), req)

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
			var output GeoDistanceOutput
			if err := ParseResultJSON(result, &output); err != nil {
				t.Fatalf("Failed to unmarshal result: %v", err)
			}

			// Check distance with tolerance for floating-point differences
			tolerance := tt.expected * 0.01 // 1% tolerance
			if math.Abs(output.Distance-tt.expected) > tolerance && tt.expected > 0 {
				t.Errorf("Expected distance around %f, got %f", tt.expected, output.Distance)
			}
		})
	}
}

func TestHandleBBoxFromPoints(t *testing.T) {
	tests := []struct {
		name        string
		points      []geo.Location
		expectError bool
		expected    geo.BoundingBox
	}{
		{
			name: "Valid points",
			points: []geo.Location{
				{Latitude: 40.7128, Longitude: -74.0060},  // New York
				{Latitude: 34.0522, Longitude: -118.2437}, // Los Angeles
				{Latitude: 41.8781, Longitude: -87.6298},  // Chicago
			},
			expectError: false,
			expected: geo.BoundingBox{
				MinLat: 34.0522,
				MinLon: -118.2437,
				MaxLat: 41.8781,
				MaxLon: -74.0060,
			},
		},
		{
			name:        "Empty points array",
			points:      []geo.Location{},
			expectError: true,
		},
		{
			name: "Invalid point",
			points: []geo.Location{
				{Latitude: 40.7128, Longitude: -74.0060},
				{Latitude: 91.0, Longitude: -118.2437}, // Invalid latitude
			},
			expectError: true,
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
					Name: "bbox_from_points",
					Arguments: map[string]any{
						"points": tt.points,
					},
				},
			}

			// Call handler
			result, err := HandleBBoxFromPoints(context.Background(), req)

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
			var output BBoxFromPointsOutput
			if err := ParseResultJSON(result, &output); err != nil {
				t.Fatalf("Failed to unmarshal result: %v", err)
			}

			// Check bounding box values with small epsilon for floating point comparison
			const epsilon = 0.000001
			if math.Abs(output.BBox.MinLat-tt.expected.MinLat) > epsilon ||
				math.Abs(output.BBox.MinLon-tt.expected.MinLon) > epsilon ||
				math.Abs(output.BBox.MaxLat-tt.expected.MaxLat) > epsilon ||
				math.Abs(output.BBox.MaxLon-tt.expected.MaxLon) > epsilon {
				t.Errorf("Expected bbox %+v, got %+v", tt.expected, output.BBox)
			}
		})
	}
}

func TestHandleCentroidPoints(t *testing.T) {
	tests := []struct {
		name        string
		points      []geo.Location
		expectError bool
		expected    geo.Location
	}{
		{
			name: "Valid points",
			points: []geo.Location{
				{Latitude: 10.0, Longitude: 10.0},
				{Latitude: 20.0, Longitude: 20.0},
				{Latitude: 30.0, Longitude: 30.0},
			},
			expectError: false,
			expected:    geo.Location{Latitude: 20.0, Longitude: 20.0},
		},
		{
			name:        "Empty points array",
			points:      []geo.Location{},
			expectError: true,
		},
		{
			name: "Invalid point",
			points: []geo.Location{
				{Latitude: 10.0, Longitude: 10.0},
				{Latitude: 91.0, Longitude: 10.0}, // Invalid latitude
			},
			expectError: true,
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
					Name: "centroid_points",
					Arguments: map[string]any{
						"points": tt.points,
					},
				},
			}

			// Call handler
			result, err := HandleCentroidPoints(context.Background(), req)

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
			var output CentroidPointsOutput
			if err := ParseResultJSON(result, &output); err != nil {
				t.Fatalf("Failed to unmarshal result: %v", err)
			}

			// Check centroid with small epsilon for floating point comparison
			const epsilon = 0.000001
			if math.Abs(output.Centroid.Latitude-tt.expected.Latitude) > epsilon ||
				math.Abs(output.Centroid.Longitude-tt.expected.Longitude) > epsilon {
				t.Errorf("Expected centroid %+v, got %+v", tt.expected, output.Centroid)
			}
		})
	}
}
