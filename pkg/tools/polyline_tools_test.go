package tools

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/NERVsystems/osmmcp/pkg/geo"
)

func TestHandlePolylineDecode(t *testing.T) {
	tests := []struct {
		name        string
		polyline    string
		expectError bool
		pointCount  int
	}{
		{
			name:        "Valid polyline",
			polyline:    "_p~iF~ps|U_ulLnnqC_mqNvxq`@",
			expectError: false,
			pointCount:  3,
		},
		{
			name:        "Empty polyline",
			polyline:    "",
			expectError: true,
			pointCount:  0,
		},
		{
			name:        "Single character polyline",
			polyline:    "?",
			expectError: true,
			pointCount:  0,
		},
		{
			name:        "Non-printable ASCII characters",
			polyline:    string([]byte{0x01, 0x02, 0x03}),
			expectError: true,
			pointCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test request
			req := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name: "polyline_decode",
					Arguments: map[string]any{
						"polyline": tt.polyline,
					},
				},
			}

			// Call handler
			result, err := HandlePolylineDecode(context.Background(), req)

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
			var output PolylineDecodeOutput
			if err := ParseResultJSON(result, &output); err != nil {
				t.Fatalf("Failed to unmarshal result: %v", err)
			}

			// Check point count
			if len(output.Points) != tt.pointCount {
				t.Errorf("Expected %d points, got %d", tt.pointCount, len(output.Points))
			}
		})
	}
}

func TestHandlePolylineEncode(t *testing.T) {
	tests := []struct {
		name        string
		points      []geo.Location
		expectError bool
		expected    string
	}{
		{
			name: "Valid points",
			points: []geo.Location{
				{Latitude: 38.5, Longitude: -120.2},
				{Latitude: 40.7, Longitude: -120.95},
				{Latitude: 43.252, Longitude: -126.453},
			},
			expectError: false,
			expected:    "_p~iF~ps|U_ulLnnqC_mqNvxq`@",
		},
		{
			name:        "Empty points array",
			points:      []geo.Location{},
			expectError: true,
		},
		{
			name: "Invalid coordinates",
			points: []geo.Location{
				{Latitude: 138.5, Longitude: -120.2}, // Invalid latitude
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test request
			req := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name: "polyline_encode",
					Arguments: map[string]any{
						"points": tt.points,
					},
				},
			}

			// Call handler
			result, err := HandlePolylineEncode(context.Background(), req)

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
			var output PolylineEncodeOutput
			if err := ParseResultJSON(result, &output); err != nil {
				t.Fatalf("Failed to unmarshal result: %v", err)
			}

			// Check polyline
			if tt.expected != "" && output.Polyline != tt.expected {
				t.Errorf("Expected polyline %s, got %s", tt.expected, output.Polyline)
			}
		})
	}
}
