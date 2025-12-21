// Package coords provides unified coordinate parsing and conversion.
//
// It automatically detects and converts various coordinate formats to
// decimal degrees (WGS84), enabling tools to accept coordinates in any
// common format without requiring the caller to pre-convert.
//
// Supported formats:
//   - MGRS: Military Grid Reference System (e.g., "47QNB8598697460")
//   - UTM: Universal Transverse Mercator (e.g., "47N 485986 2197460")
//   - DMS: Degrees Minutes Seconds (e.g., "19°51'22"N 99°48'59"E")
//   - Decimal Degrees: Standard lat/lon (e.g., "19.856, 99.816")
package coords

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/NERVsystems/osmmcp/pkg/geo"
	"github.com/akhenakh/mgrs"
)

// Format represents a coordinate format type
type Format int

const (
	FormatUnknown Format = iota
	FormatDecimal        // Decimal degrees (lat, lon)
	FormatDMS            // Degrees Minutes Seconds
	FormatMGRS           // Military Grid Reference System
	FormatUTM            // Universal Transverse Mercator
)

// String returns the format name
func (f Format) String() string {
	switch f {
	case FormatDecimal:
		return "decimal"
	case FormatDMS:
		return "dms"
	case FormatMGRS:
		return "mgrs"
	case FormatUTM:
		return "utm"
	default:
		return "unknown"
	}
}

// ParseResult contains the parsed coordinate and metadata
type ParseResult struct {
	Location geo.Location // Converted lat/lon
	Format   Format       // Detected format
	Original string       // Original input string
}

// Regular expressions for format detection
var (
	// MGRS: Grid Zone Designator (1-60 + latitude band A-Z except I,O) + 100km square ID (2 letters) + numeric location
	// Examples: 47QNB8598697460, 18SUJ2337506519, 4QFJ12345678
	mgrsRegex = regexp.MustCompile(`(?i)^(\d{1,2})([C-HJ-NP-X])([A-HJ-NP-Z]{2})(\d{2,10})$`)

	// UTM: Zone + hemisphere + easting + northing
	// Examples: "47N 485986 2197460", "18T 234567 4567890"
	utmRegex = regexp.MustCompile(`(?i)^(\d{1,2})([A-Z])\s+(\d+)\s+(\d+)$`)

	// DMS: Degrees Minutes Seconds with direction
	// Examples: "19°51'22"N 99°48'59"E", "19d51m22sN 99d48m59sE"
	// Also handles: 19 51 22 N 99 48 59 E
	dmsRegex = regexp.MustCompile(`(?i)^(-?\d+)[°d\s]+(\d+)[′'m\s]+(\d+(?:\.\d+)?)[″"s]?\s*([NS])[\s,]+(-?\d+)[°d\s]+(\d+)[′'m\s]+(\d+(?:\.\d+)?)[″"s]?\s*([EW])$`)

	// Decimal degrees: lat, lon or lat lon
	// Examples: "19.856, 99.816", "19.856 99.816", "-33.8688, 151.2093"
	decimalRegex = regexp.MustCompile(`^(-?\d+\.?\d*)[,\s]+(-?\d+\.?\d*)$`)
)

// Parse attempts to detect the coordinate format and convert to decimal degrees.
// It returns an error if the input cannot be parsed as any known format.
func Parse(input string) (*ParseResult, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("empty coordinate string")
	}

	// Try each format in order of specificity
	// MGRS first (most specific pattern)
	if result, err := ParseMGRS(input); err == nil {
		return result, nil
	}

	// UTM second
	if result, err := ParseUTM(input); err == nil {
		return result, nil
	}

	// DMS third
	if result, err := ParseDMS(input); err == nil {
		return result, nil
	}

	// Decimal degrees last (most general)
	if result, err := ParseDecimal(input); err == nil {
		return result, nil
	}

	return nil, fmt.Errorf("unrecognized coordinate format: %q", input)
}

// IsCoordinate returns true if the input appears to be a coordinate in any supported format.
// This is a quick check that doesn't perform full parsing.
func IsCoordinate(input string) bool {
	input = strings.TrimSpace(input)
	if input == "" {
		return false
	}

	// Check each pattern
	if mgrsRegex.MatchString(input) {
		return true
	}
	if utmRegex.MatchString(input) {
		return true
	}
	if dmsRegex.MatchString(input) {
		return true
	}
	if decimalRegex.MatchString(input) {
		return true
	}

	return false
}

// DetectFormat returns the detected coordinate format without full parsing.
func DetectFormat(input string) Format {
	input = strings.TrimSpace(input)
	if input == "" {
		return FormatUnknown
	}

	if mgrsRegex.MatchString(input) {
		return FormatMGRS
	}
	if utmRegex.MatchString(input) {
		return FormatUTM
	}
	if dmsRegex.MatchString(input) {
		return FormatDMS
	}
	if decimalRegex.MatchString(input) {
		return FormatDecimal
	}

	return FormatUnknown
}

// ParseMGRS parses an MGRS coordinate string.
// MGRS format: Grid Zone (1-60) + Latitude Band (C-X, excluding I and O) +
// 100km Square ID (2 letters) + Numeric Location (2-10 digits, even count)
//
// Examples:
//   - 47QNB8598697460 (10-digit: 1m precision)
//   - 18SUJ23370651 (8-digit: 10m precision)
//   - 4QFJ1234 (4-digit: 1km precision)
func ParseMGRS(input string) (*ParseResult, error) {
	input = strings.TrimSpace(strings.ToUpper(input))

	if !mgrsRegex.MatchString(input) {
		return nil, fmt.Errorf("invalid MGRS format: %q", input)
	}

	// Use the mgrs library for conversion
	lat, lon, err := mgrs.MGRSToLatLng(input)
	if err != nil {
		return nil, fmt.Errorf("MGRS conversion failed: %w", err)
	}

	// Validate the result is in valid range
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return nil, fmt.Errorf("MGRS conversion produced invalid coordinates: lat=%f, lon=%f", lat, lon)
	}

	return &ParseResult{
		Location: geo.Location{
			Latitude:  lat,
			Longitude: lon,
		},
		Format:   FormatMGRS,
		Original: input,
	}, nil
}

// ParseUTM parses a UTM coordinate string.
// UTM format: Zone (1-60) + Hemisphere letter + Easting + Northing
//
// Examples:
//   - 47N 485986 2197460
//   - 18T 234567 4567890
func ParseUTM(input string) (*ParseResult, error) {
	input = strings.TrimSpace(strings.ToUpper(input))

	matches := utmRegex.FindStringSubmatch(input)
	if matches == nil {
		return nil, fmt.Errorf("invalid UTM format: %q", input)
	}

	zone, err := strconv.Atoi(matches[1])
	if err != nil || zone < 1 || zone > 60 {
		return nil, fmt.Errorf("invalid UTM zone: %s", matches[1])
	}

	band := matches[2][0]
	easting, err := strconv.ParseFloat(matches[3], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid UTM easting: %s", matches[3])
	}

	northing, err := strconv.ParseFloat(matches[4], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid UTM northing: %s", matches[4])
	}

	// Determine if northern or southern hemisphere based on band letter
	// Bands C-M are southern hemisphere, N-X are northern
	isNorthern := band >= 'N'

	// Convert UTM to lat/lon
	lat, lon := utmToLatLon(zone, easting, northing, isNorthern)

	// Validate
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return nil, fmt.Errorf("UTM conversion produced invalid coordinates: lat=%f, lon=%f", lat, lon)
	}

	return &ParseResult{
		Location: geo.Location{
			Latitude:  lat,
			Longitude: lon,
		},
		Format:   FormatUTM,
		Original: input,
	}, nil
}

// ParseDMS parses a Degrees Minutes Seconds coordinate string.
//
// Examples:
//   - 19°51'22"N 99°48'59"E
//   - 19d51m22sN 99d48m59sE
//   - 19 51 22 N 99 48 59 E
func ParseDMS(input string) (*ParseResult, error) {
	input = strings.TrimSpace(input)

	matches := dmsRegex.FindStringSubmatch(input)
	if matches == nil {
		return nil, fmt.Errorf("invalid DMS format: %q", input)
	}

	// Parse latitude components
	latDeg, _ := strconv.ParseFloat(matches[1], 64)
	latMin, _ := strconv.ParseFloat(matches[2], 64)
	latSec, _ := strconv.ParseFloat(matches[3], 64)
	latDir := strings.ToUpper(matches[4])

	// Parse longitude components
	lonDeg, _ := strconv.ParseFloat(matches[5], 64)
	lonMin, _ := strconv.ParseFloat(matches[6], 64)
	lonSec, _ := strconv.ParseFloat(matches[7], 64)
	lonDir := strings.ToUpper(matches[8])

	// Validate ranges
	if latDeg > 90 || latMin >= 60 || latSec >= 60 {
		return nil, fmt.Errorf("invalid latitude values: %s", input)
	}
	if lonDeg > 180 || lonMin >= 60 || lonSec >= 60 {
		return nil, fmt.Errorf("invalid longitude values: %s", input)
	}

	// Convert to decimal degrees
	lat := latDeg + latMin/60 + latSec/3600
	lon := lonDeg + lonMin/60 + lonSec/3600

	// Apply direction
	if latDir == "S" {
		lat = -lat
	}
	if lonDir == "W" {
		lon = -lon
	}

	return &ParseResult{
		Location: geo.Location{
			Latitude:  lat,
			Longitude: lon,
		},
		Format:   FormatDMS,
		Original: input,
	}, nil
}

// ParseDecimal parses a decimal degrees coordinate string.
//
// Examples:
//   - 19.856, 99.816
//   - 19.856 99.816
//   - -33.8688, 151.2093
func ParseDecimal(input string) (*ParseResult, error) {
	input = strings.TrimSpace(input)

	matches := decimalRegex.FindStringSubmatch(input)
	if matches == nil {
		return nil, fmt.Errorf("invalid decimal format: %q", input)
	}

	lat, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid latitude: %s", matches[1])
	}

	lon, err := strconv.ParseFloat(matches[2], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid longitude: %s", matches[2])
	}

	// Validate ranges
	if lat < -90 || lat > 90 {
		return nil, fmt.Errorf("latitude out of range: %f", lat)
	}
	if lon < -180 || lon > 180 {
		return nil, fmt.Errorf("longitude out of range: %f", lon)
	}

	return &ParseResult{
		Location: geo.Location{
			Latitude:  lat,
			Longitude: lon,
		},
		Format:   FormatDecimal,
		Original: input,
	}, nil
}

// ToMGRS converts a lat/lon to MGRS string with specified precision.
// Precision is 1-5 representing: 10km, 1km, 100m, 10m, 1m
func ToMGRS(lat, lon float64, precision int) (string, error) {
	if precision < 1 || precision > 5 {
		precision = 5 // Default to 1m precision
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return "", fmt.Errorf("coordinates out of range: lat=%f, lon=%f", lat, lon)
	}

	result, err := mgrs.LatLngToMGRS(lat, lon, precision)
	if err != nil {
		return "", fmt.Errorf("MGRS conversion failed: %w", err)
	}
	return result, nil
}

// utmToLatLon converts UTM coordinates to latitude/longitude (WGS84)
// Using the standard Karney algorithm
func utmToLatLon(zone int, easting, northing float64, isNorthern bool) (lat, lon float64) {
	// WGS84 ellipsoid parameters
	const (
		a  = 6378137.0         // Semi-major axis (meters)
		f  = 1 / 298.257223563 // Flattening
		k0 = 0.9996            // Scale factor
	)

	// Derived constants
	b := a * (1 - f)             // Semi-minor axis
	e2 := (a*a - b*b) / (a * a)  // First eccentricity squared
	ep2 := (a*a - b*b) / (b * b) // Second eccentricity squared
	e1 := (1 - math.Sqrt(1-e2)) / (1 + math.Sqrt(1-e2))

	// Remove false easting and northing
	x := easting - 500000.0
	y := northing
	if !isNorthern {
		y = y - 10000000.0
	}

	// Central meridian
	lon0 := float64((zone-1)*6-180+3) * math.Pi / 180.0

	// Footpoint latitude
	m := y / k0
	mu := m / (a * (1 - e2/4 - 3*e2*e2/64 - 5*e2*e2*e2/256))

	phi1 := mu +
		(3*e1/2-27*e1*e1*e1/32)*math.Sin(2*mu) +
		(21*e1*e1/16-55*e1*e1*e1*e1/32)*math.Sin(4*mu) +
		(151*e1*e1*e1/96)*math.Sin(6*mu) +
		(1097*e1*e1*e1*e1/512)*math.Sin(8*mu)

	// Calculate parameters at footpoint latitude
	sinPhi1 := math.Sin(phi1)
	cosPhi1 := math.Cos(phi1)
	tanPhi1 := math.Tan(phi1)

	n1 := a / math.Sqrt(1-e2*sinPhi1*sinPhi1)
	t1 := tanPhi1 * tanPhi1
	c1 := ep2 * cosPhi1 * cosPhi1
	r1 := a * (1 - e2) / math.Pow(1-e2*sinPhi1*sinPhi1, 1.5)
	d := x / (n1 * k0)

	// Calculate latitude (in radians)
	lat = phi1 - (n1*tanPhi1/r1)*(d*d/2-
		(5+3*t1+10*c1-4*c1*c1-9*ep2)*d*d*d*d/24+
		(61+90*t1+298*c1+45*t1*t1-252*ep2-3*c1*c1)*d*d*d*d*d*d/720)

	// Calculate longitude (in radians)
	lon = lon0 + (d-
		(1+2*t1+c1)*d*d*d/6+
		(5-2*c1+28*t1-3*c1*c1+8*ep2+24*t1*t1)*d*d*d*d*d/120)/cosPhi1

	// Convert to degrees
	lat = lat * 180 / math.Pi
	lon = lon * 180 / math.Pi

	return lat, lon
}
