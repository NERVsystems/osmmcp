// Package osm provides utilities for working with OpenStreetMap data.
package osm

import (
	"math"

	"github.com/NERVsystems/osmmcp/pkg/geo"
)

// DecodePolyline decodes an encoded polyline string to a slice of locations.
// This implements Google's Polyline Algorithm Format (Polyline5) which is used by OSRM.
// The algorithm uses 5 decimal places of precision (1e-5) for coordinates.
// See https://developers.google.com/maps/documentation/utilities/polylinealgorithm
func DecodePolyline(encoded string) []geo.Location {
	if len(encoded) == 0 {
		return []geo.Location{}
	}

	// Count number of backslashes to get a rough estimate of size
	count := len(encoded) / 4
	if count <= 0 {
		count = 1
	}

	// Allocate result slice with estimated capacity
	points := make([]geo.Location, 0, count)

	// Initialize variables
	index := 0
	lat := 0
	lng := 0
	strLen := len(encoded)

	// Iterate through the string
	for index < strLen {
		// Decode latitude
		result := 0
		shift := 0
		for {
			if index >= strLen {
				break
			}
			b := int(encoded[index]) - 63
			index++
			result |= (b & 0x1f) << shift
			shift += 5
			if b < 0x20 {
				break
			}
		}
		// Fix sign-bit inversion
		deltaLat := (result >> 1) ^ (-(result & 1))
		lat += deltaLat

		// Decode longitude
		result = 0
		shift = 0
		for {
			if index >= strLen {
				break
			}
			b := int(encoded[index]) - 63
			index++
			result |= (b & 0x1f) << shift
			shift += 5
			if b < 0x20 {
				break
			}
		}
		// Fix sign-bit inversion
		deltaLng := (result >> 1) ^ (-(result & 1))
		lng += deltaLng

		// Convert to floating point and add to result
		points = append(points, geo.Location{
			Latitude:  float64(lat) * 1e-5,
			Longitude: float64(lng) * 1e-5,
		})
	}

	return points
}

// EncodePolyline encodes a slice of locations into a polyline string.
// This implements Google's Polyline Algorithm Format (Polyline5) which is used by OSRM.
// The algorithm uses 5 decimal places of precision (1e-5) for coordinates.
// See https://developers.google.com/maps/documentation/utilities/polylinealgorithm
func EncodePolyline(points []geo.Location) string {
	if len(points) == 0 {
		return ""
	}

	// Check for potential overflow in size calculation
	const maxPoints = (1 << 30) / 6 // ~178 million points max
	if len(points) > maxPoints {
		points = points[:maxPoints] // Truncate to safe size
	}

	// Estimate result size (6 bytes per point is common)
	result := make([]byte, 0, len(points)*6)

	// Initialize previous values
	prevLat := 0
	prevLng := 0

	// Encode each point
	for _, point := range points {
		// Convert to integers with 5 decimal precision
		lat := int(math.Round(point.Latitude * 1e5))
		lng := int(math.Round(point.Longitude * 1e5))

		// Encode differences from previous values
		deltaLat := lat - prevLat
		deltaLng := lng - prevLng
		result = append(result, encodeSigned(deltaLat)...)
		result = append(result, encodeSigned(deltaLng)...)

		// Update previous values
		prevLat = lat
		prevLng = lng
	}

	return string(result)
}

// encodeSigned encodes a signed value using the Google Polyline Algorithm.
// This is an internal helper function that should not be exported.
func encodeSigned(value int) []byte {
	// Convert to zigzag encoding
	s := value << 1
	if value < 0 {
		s = ^s
	}

	// Encode the value
	var buf []byte
	for s >= 0x20 {
		buf = append(buf, byte((0x20|(s&0x1f))+63))
		s >>= 5
	}
	buf = append(buf, byte(s+63))
	return buf
}
