package core

import (
	"errors"
	"math"

	"github.com/NERVsystems/osmmcp/pkg/geo"
)

// EncodePolyline encodes a slice of geo.Location points into a polyline string.
// This implements Google's Polyline Algorithm Format (Polyline5).
// The algorithm uses 5 decimal places of precision (1e-5) for coordinates.
// See https://developers.google.com/maps/documentation/utilities/polylinealgorithm
func EncodePolyline(points []geo.Location) string {
	if len(points) == 0 {
		return ""
	}

	// Estimate result size (6 bytes per coordinate is common)
	result := make([]byte, 0, len(points)*12)

	// Initialize previous values
	prevLat := 0
	prevLon := 0

	// Encode each point
	for _, point := range points {
		// Convert to integer with 5 decimal precision
		lat := int(math.Round(point.Latitude * 1e5))
		lon := int(math.Round(point.Longitude * 1e5))

		// Encode differences from previous values
		result = append(result, encodeSigned(lat-prevLat)...)
		result = append(result, encodeSigned(lon-prevLon)...)

		// Update previous values
		prevLat = lat
		prevLon = lon
	}

	return string(result)
}

// DecodePolyline decodes a polyline string into a slice of geo.Location points.
// This implements Google's Polyline Algorithm Format (Polyline5).
// The algorithm uses 5 decimal places of precision (1e-5) for coordinates.
// See https://developers.google.com/maps/documentation/utilities/polylinealgorithm
func DecodePolyline(polyline string) ([]geo.Location, error) {
	if len(polyline) == 0 {
		return []geo.Location{}, nil
	}

	// Count number of backslashes to get a rough estimate of size
	count := len(polyline) / 8
	if count <= 0 {
		count = 1
	}

	// Allocate result slice with estimated capacity
	points := make([]geo.Location, 0, count)

	// Initialize variables
	index := 0
	prevLat := 0
	prevLon := 0
	strLen := len(polyline)

	// Iterate through the string
	for index < strLen {
		// Decode latitude
		lat, newIndex, err := decodeValue(polyline, index, prevLat)
		if err != nil {
			return nil, err
		}
		index = newIndex
		prevLat = lat

		// Decode longitude
		if index >= strLen {
			return nil, errors.New("invalid polyline: unexpected end of string")
		}
		lon, newIndex, err := decodeValue(polyline, index, prevLon)
		if err != nil {
			return nil, err
		}
		index = newIndex
		prevLon = lon

		// Add point to result
		points = append(points, geo.Location{
			Latitude:  float64(lat) * 1e-5,
			Longitude: float64(lon) * 1e-5,
		})
	}

	return points, nil
}

// decodeValue decodes a single value from a polyline string.
// This is an internal helper function that should not be exported.
func decodeValue(polyline string, index, prev int) (int, int, error) {
	strLen := len(polyline)
	result := 0
	shift := 0

	// Decode chunks until we hit a chunk that doesn't continue
	for {
		if index >= strLen {
			return 0, 0, errors.New("invalid polyline: unexpected end of string")
		}
		b := int(polyline[index]) - 63
		index++
		result |= (b & 0x1f) << shift
		shift += 5
		if b < 0x20 {
			break
		}
	}

	// Fix sign-bit inversion
	delta := (result >> 1) ^ (-(result & 1))
	value := prev + delta

	return value, index, nil
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
