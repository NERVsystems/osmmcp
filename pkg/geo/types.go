// Package geo provides common geographic types and calculations.
// It centralizes location-based data structures and algorithms to ensure
// consistency across the codebase.
package geo

import (
	"fmt"
	"math"
)

// EarthRadius is the mean radius of Earth according to WGS-84 in meters
const EarthRadius = 6371000.0

// Location represents a geographic coordinate (latitude and longitude)
// with standardized JSON field names.
//
// Example:
//
//	loc := geo.Location{Latitude: 37.7749, Longitude: -122.4194}
//	dist := geo.HaversineDistance(loc.Latitude, loc.Longitude, 34.0522, -118.2437)
type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// Address represents a structured address
type Address struct {
	Street      string `json:"street,omitempty"`
	HouseNumber string `json:"house_number,omitempty"`
	City        string `json:"city,omitempty"`
	State       string `json:"state,omitempty"`
	Country     string `json:"country,omitempty"`
	PostalCode  string `json:"postal_code,omitempty"`
	Formatted   string `json:"formatted,omitempty"`
}

// BoundingBox represents a geographic bounding box with southwest and northeast corners
// When used in API requests and responses, the field names must be lowercase with camelCase:
// minLat, minLon, maxLat, maxLon. Using different capitalization (e.g., MinLat) will cause
// parsing errors in API calls.
type BoundingBox struct {
	MinLat float64 `json:"minLat"` // Southern edge (minimum latitude)
	MinLon float64 `json:"minLon"` // Western edge (minimum longitude)
	MaxLat float64 `json:"maxLat"` // Northern edge (maximum latitude)
	MaxLon float64 `json:"maxLon"` // Eastern edge (maximum longitude)
}

// NewBoundingBox creates a new empty bounding box
func NewBoundingBox() *BoundingBox {
	return &BoundingBox{
		MinLat: 90.0, // Start with inverted min/max so any point extends correctly
		MinLon: 180.0,
		MaxLat: -90.0,
		MaxLon: -180.0,
	}
}

// ExtendWithPoint extends the bounding box to include the specified point
func (bb *BoundingBox) ExtendWithPoint(lat, lon float64) {
	if lat < bb.MinLat {
		bb.MinLat = lat
	}
	if lat > bb.MaxLat {
		bb.MaxLat = lat
	}
	if lon < bb.MinLon {
		bb.MinLon = lon
	}
	if lon > bb.MaxLon {
		bb.MaxLon = lon
	}
}

// Buffer adds a buffer around the bounding box in meters
// This is a rough approximation as it converts meters to degrees using
// a simple factor that's reasonably accurate near the equator.
func (bb *BoundingBox) Buffer(bufferMeters float64) {
	// Convert meters to approximate degrees (crude approximation)
	// 0.01 degrees ≈ 1.11 km at the equator
	bufferDegrees := bufferMeters / 111000
	bb.MinLat -= bufferDegrees
	bb.MaxLat += bufferDegrees
	bb.MinLon -= bufferDegrees
	bb.MaxLon += bufferDegrees

	// Ensure coordinates are within valid ranges
	if bb.MinLat < -90 {
		bb.MinLat = -90
	}
	if bb.MaxLat > 90 {
		bb.MaxLat = 90
	}
	if bb.MinLon < -180 {
		bb.MinLon = -180
	}
	if bb.MaxLon > 180 {
		bb.MaxLon = 180
	}
}

// String returns a string representation of the bounding box for use in Overpass queries
func (bb *BoundingBox) String() string {
	return fmt.Sprintf("(%f,%f,%f,%f)", bb.MinLat, bb.MinLon, bb.MaxLat, bb.MaxLon)
}

// HaversineDistance calculates the great-circle distance between two points
// on the Earth's surface given their latitude and longitude in degrees.
// The result is returned in meters.
func HaversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Convert degrees to radians
	lat1Rad := lat1 * math.Pi / 180.0
	lon1Rad := lon1 * math.Pi / 180.0
	lat2Rad := lat2 * math.Pi / 180.0
	lon2Rad := lon2 * math.Pi / 180.0

	// Haversine formula
	dlat := lat2Rad - lat1Rad
	dlon := lon2Rad - lon1Rad
	a := math.Sin(dlat/2)*math.Sin(dlat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dlon/2)*math.Sin(dlon/2)
	c := 2 * math.Asin(math.Sqrt(a))

	// Calculate distance in meters
	return EarthRadius * c
}
