package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/NERVsystems/osmmcp/pkg/geo"
)

func main() {
	// Create a bounding box with some test values
	bbox := geo.BoundingBox{
		MinLat: 37.7749,
		MinLon: -122.4194,
		MaxLat: 37.8049,
		MaxLon: -122.3894,
	}

	// Serialize to JSON
	bboxJSON, err := json.MarshalIndent(bbox, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal BoundingBox: %v", err)
	}
	fmt.Printf("BoundingBox JSON:\n%s\n\n", string(bboxJSON))

	// Verify that we can deserialize it back
	var deserializedBBox geo.BoundingBox
	err = json.Unmarshal(bboxJSON, &deserializedBBox)
	if err != nil {
		log.Fatalf("Failed to unmarshal BoundingBox: %v", err)
	}
	fmt.Printf("Deserialized BoundingBox: %+v\n\n", deserializedBBox)

	// Test with uppercase field names (which should fail)
	invalidJSON := `{
		"MinLat": 37.7749,
		"MinLon": -122.4194,
		"MaxLat": 37.8049,
		"MaxLon": -122.3894
	}`

	var uppercaseBBox geo.BoundingBox
	err = json.Unmarshal([]byte(invalidJSON), &uppercaseBBox)
	if err != nil {
		fmt.Printf("As expected, failed to unmarshal with uppercase field names: %v\n", err)
	} else {
		fmt.Printf("WARNING: Unexpectedly succeeded with uppercase field names: %+v\n", uppercaseBBox)
	}

	// Verify that lowercase camelCase field names work
	validJSON := `{
		"minLat": 37.7749,
		"minLon": -122.4194,
		"maxLat": 37.8049,
		"maxLon": -122.3894
	}`

	var lowercaseBBox geo.BoundingBox
	err = json.Unmarshal([]byte(validJSON), &lowercaseBBox)
	if err != nil {
		fmt.Printf("ERROR: Failed to unmarshal with lowercase field names: %v\n", err)
	} else {
		fmt.Printf("Successfully unmarshaled with lowercase field names: %+v\n", lowercaseBBox)
	}
}
