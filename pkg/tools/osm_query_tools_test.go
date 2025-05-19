package tools

import (
	"context"
	"testing"

	"github.com/NERVsystems/osmmcp/pkg/geo"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestHandleFilterTags(t *testing.T) {
	tests := []struct {
		name           string
		elements       []OSMElement
		tags           map[string][]string
		expectError    bool
		expectedCount  int
		expectedResult []string // Expected element IDs in result
	}{
		{
			name: "Filter by single tag",
			elements: []OSMElement{
				{
					ID: "1",
					Tags: map[string]string{
						"amenity": "restaurant",
						"cuisine": "italian",
					},
				},
				{
					ID: "2",
					Tags: map[string]string{
						"amenity": "cafe",
					},
				},
				{
					ID: "3",
					Tags: map[string]string{
						"shop": "bakery",
					},
				},
			},
			tags: map[string][]string{
				"amenity": {"restaurant", "cafe"},
			},
			expectError:    false,
			expectedCount:  2,
			expectedResult: []string{"1", "2"},
		},
		{
			name: "Filter by multiple tags",
			elements: []OSMElement{
				{
					ID: "1",
					Tags: map[string]string{
						"amenity": "restaurant",
						"cuisine": "italian",
					},
				},
				{
					ID: "2",
					Tags: map[string]string{
						"amenity": "restaurant",
						"cuisine": "chinese",
					},
				},
				{
					ID: "3",
					Tags: map[string]string{
						"amenity": "cafe",
					},
				},
			},
			tags: map[string][]string{
				"amenity": {"restaurant"},
				"cuisine": {"italian"},
			},
			expectError:    false,
			expectedCount:  1,
			expectedResult: []string{"1"},
		},
		{
			name: "Wildcard tag filter",
			elements: []OSMElement{
				{
					ID: "1",
					Tags: map[string]string{
						"amenity": "restaurant",
						"cuisine": "italian",
					},
				},
				{
					ID: "2",
					Tags: map[string]string{
						"amenity": "restaurant",
						"cuisine": "chinese",
					},
				},
				{
					ID: "3",
					Tags: map[string]string{
						"amenity": "cafe",
					},
				},
			},
			tags: map[string][]string{
				"amenity": {"restaurant"},
				"cuisine": {"*"}, // Any cuisine value
			},
			expectError:    false,
			expectedCount:  2,
			expectedResult: []string{"1", "2"},
		},
		{
			name: "No matching elements",
			elements: []OSMElement{
				{
					ID: "1",
					Tags: map[string]string{
						"amenity": "restaurant",
					},
				},
			},
			tags: map[string][]string{
				"shop": {"bakery"},
			},
			expectError:    false,
			expectedCount:  0,
			expectedResult: []string{},
		},
		{
			name:          "Empty elements array",
			elements:      []OSMElement{},
			tags:          map[string][]string{"amenity": {"restaurant"}},
			expectError:   true,
			expectedCount: 0,
		},
		{
			name: "Empty tags",
			elements: []OSMElement{
				{
					ID: "1",
					Tags: map[string]string{
						"amenity": "restaurant",
					},
				},
			},
			tags:          map[string][]string{},
			expectError:   true,
			expectedCount: 0,
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
					Name: "filter_tags",
					Arguments: map[string]any{
						"elements": tt.elements,
						"tags":     tt.tags,
					},
				},
			}

			// Call handler
			result, err := HandleFilterTags(context.Background(), req)

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
			var output FilterTagsOutput
			if err := ParseResultJSON(result, &output); err != nil {
				t.Fatalf("Failed to unmarshal result: %v", err)
			}

			// Check element count
			if len(output.Elements) != tt.expectedCount {
				t.Errorf("Expected %d elements, got %d", tt.expectedCount, len(output.Elements))
			}

			// Check that the expected elements are present
			foundIDs := make(map[string]bool)
			for _, element := range output.Elements {
				foundIDs[element.ID] = true
			}

			for _, expectedID := range tt.expectedResult {
				if !foundIDs[expectedID] {
					t.Errorf("Expected element with ID %s not found in result", expectedID)
				}
			}
		})
	}
}

func TestHandleSortByDistance(t *testing.T) {
	tests := []struct {
		name          string
		elements      []OSMElement
		ref           geo.Location
		expectError   bool
		expectedOrder []string // Expected order of element IDs after sorting
	}{
		{
			name: "Sort elements by distance",
			elements: []OSMElement{
				{
					ID: "1",
					Location: &geo.Location{
						Latitude:  40.7128,
						Longitude: -74.0060, // New York
					},
				},
				{
					ID: "2",
					Location: &geo.Location{
						Latitude:  34.0522,
						Longitude: -118.2437, // Los Angeles
					},
				},
				{
					ID: "3",
					Location: &geo.Location{
						Latitude:  41.8781,
						Longitude: -87.6298, // Chicago
					},
				},
			},
			ref: geo.Location{
				Latitude:  38.9072,
				Longitude: -77.0369, // Washington DC
			},
			expectError:   false,
			expectedOrder: []string{"1", "3", "2"}, // Order from closest to farthest from DC
		},
		{
			name:          "Empty elements array",
			elements:      []OSMElement{},
			ref:           geo.Location{Latitude: 38.9072, Longitude: -77.0369},
			expectError:   true,
			expectedOrder: []string{},
		},
		{
			name: "Elements with center points",
			elements: []OSMElement{
				{
					ID:   "1",
					Type: "way",
					Center: &geo.Location{
						Latitude:  40.7128,
						Longitude: -74.0060, // New York
					},
				},
				{
					ID:   "2",
					Type: "way",
					Center: &geo.Location{
						Latitude:  34.0522,
						Longitude: -118.2437, // Los Angeles
					},
				},
			},
			ref: geo.Location{
				Latitude:  38.9072,
				Longitude: -77.0369, // Washington DC
			},
			expectError:   false,
			expectedOrder: []string{"1", "2"},
		},
		{
			name: "Invalid reference point",
			elements: []OSMElement{
				{
					ID: "1",
					Location: &geo.Location{
						Latitude:  40.7128,
						Longitude: -74.0060,
					},
				},
			},
			ref: geo.Location{
				Latitude:  91.0, // Invalid latitude
				Longitude: -77.0369,
			},
			expectError:   true,
			expectedOrder: []string{},
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
					Name: "sort_by_distance",
					Arguments: map[string]any{
						"elements": tt.elements,
						"ref":      tt.ref,
					},
				},
			}

			// Call handler
			result, err := HandleSortByDistance(context.Background(), req)

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
			var output SortByDistanceOutput
			if err := ParseResultJSON(result, &output); err != nil {
				t.Fatalf("Failed to unmarshal result: %v", err)
			}

			// Check element count
			if len(output.Elements) != len(tt.expectedOrder) {
				t.Errorf("Expected %d elements, got %d", len(tt.expectedOrder), len(output.Elements))
				return
			}

			// Check that the order matches expected
			for i, expectedID := range tt.expectedOrder {
				if i >= len(output.Elements) {
					break
				}
				actualID := output.Elements[i].ID
				if actualID != expectedID {
					t.Errorf("Expected element at position %d to have ID %s, got %s", i, expectedID, actualID)
				}
			}

			// Verify that distances are in ascending order
			for i := 1; i < len(output.Elements); i++ {
				if output.Elements[i-1].Distance > output.Elements[i].Distance {
					t.Errorf("Elements not sorted by distance: %f > %f",
						output.Elements[i-1].Distance, output.Elements[i].Distance)
				}
			}
		})
	}
}

// Note: TestHandleOSMQueryBBox is omitted because it would require mocking an external OSM API call
