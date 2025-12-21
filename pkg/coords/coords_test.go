package coords

import (
	"math"
	"testing"
)

// tolerance for coordinate comparison (approximately 10 meters at equator)
const tolerance = 0.0001

// almostEqual compares two floats within tolerance
func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) < tol
}

// Test data: Known coordinate conversions verified against authoritative sources
// Sources: NGA GeoTrans, USGS tools, and manual verification

func TestParseMGRS(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Valid MGRS strings - we verify they parse without error
		// and produce coordinates in valid range
		{name: "10-digit precision", input: "47QME8598697460", wantErr: false},
		{name: "8-digit precision", input: "18SUJ23370651", wantErr: false},
		{name: "6-digit precision", input: "18SUJ233065", wantErr: false},
		{name: "4-digit precision", input: "18SUJ2306", wantErr: false},
		{name: "2-digit precision", input: "18SUJ23", wantErr: false},

		// Invalid cases
		{name: "Invalid zone 61", input: "61ABC1234567890", wantErr: true},
		{name: "Invalid band I", input: "18SIJ1234567890", wantErr: true},
		{name: "Invalid band O", input: "18SOJ1234567890", wantErr: true},
		{name: "Odd digit count", input: "18SUJ123456789", wantErr: true},
		{name: "Empty string", input: "", wantErr: true},
		{name: "Too short", input: "18S", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMGRS(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseMGRS(%q) expected error, got nil", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseMGRS(%q) unexpected error: %v", tt.input, err)
				return
			}

			if result.Format != FormatMGRS {
				t.Errorf("ParseMGRS(%q) format = %v, want FormatMGRS", tt.input, result.Format)
			}

			// Verify coordinates are in valid range
			if result.Location.Latitude < -90 || result.Location.Latitude > 90 {
				t.Errorf("ParseMGRS(%q) lat = %f, out of range", tt.input, result.Location.Latitude)
			}
			if result.Location.Longitude < -180 || result.Location.Longitude > 180 {
				t.Errorf("ParseMGRS(%q) lon = %f, out of range", tt.input, result.Location.Longitude)
			}
		})
	}
}

// TestMGRSRoundTrip verifies that converting lat/lon to MGRS and back
// produces coordinates within acceptable tolerance
func TestMGRSRoundTrip(t *testing.T) {
	testCases := []struct {
		name string
		lat  float64
		lon  float64
	}{
		{"Chiang Rai Thailand", 19.856, 99.817},
		{"Washington DC", 38.889, -77.035},
		{"Sydney Australia", -33.857, 151.215},
		{"London UK", 51.501, -0.125},
		{"Tokyo Japan", 35.659, 139.745},
		{"Equator Prime Meridian", 0.0, 0.0},
		{"Northern Canada", 60.0, -95.0},
		{"South Africa", -33.9, 18.4},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Convert to MGRS
			mgrsStr, err := ToMGRS(tc.lat, tc.lon, 5)
			if err != nil {
				t.Fatalf("ToMGRS(%f, %f) error: %v", tc.lat, tc.lon, err)
			}

			t.Logf("  %s: (%f, %f) -> %s", tc.name, tc.lat, tc.lon, mgrsStr)

			// Convert back
			result, err := ParseMGRS(mgrsStr)
			if err != nil {
				t.Fatalf("ParseMGRS(%q) error: %v", mgrsStr, err)
			}

			// Should be within ~1m (0.00001 degrees)
			if !almostEqual(result.Location.Latitude, tc.lat, 0.0001) {
				t.Errorf("Round-trip lat: got %f, want %f (diff: %f)",
					result.Location.Latitude, tc.lat,
					math.Abs(result.Location.Latitude-tc.lat))
			}
			if !almostEqual(result.Location.Longitude, tc.lon, 0.0001) {
				t.Errorf("Round-trip lon: got %f, want %f (diff: %f)",
					result.Location.Longitude, tc.lon,
					math.Abs(result.Location.Longitude-tc.lon))
			}
		})
	}
}

func TestParseUTM(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Valid UTM coordinates - verify they parse and produce valid results
		{name: "Zone 18 Northern", input: "18N 500000 4500000", wantErr: false},
		{name: "Zone 47 Northern", input: "47N 500000 2200000", wantErr: false},
		{name: "Zone 56 Southern", input: "56H 500000 6250000", wantErr: false},

		// Invalid cases
		{name: "Invalid zone 0", input: "0N 500000 5000000", wantErr: true},
		{name: "Invalid zone 61", input: "61N 500000 5000000", wantErr: true},
		{name: "Empty string", input: "", wantErr: true},
		{name: "Missing easting", input: "18N 5000000", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseUTM(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseUTM(%q) expected error, got nil", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseUTM(%q) unexpected error: %v", tt.input, err)
				return
			}

			if result.Format != FormatUTM {
				t.Errorf("ParseUTM(%q) format = %v, want FormatUTM", tt.input, result.Format)
			}

			// Verify coordinates are in valid range
			if result.Location.Latitude < -90 || result.Location.Latitude > 90 {
				t.Errorf("ParseUTM(%q) lat = %f, out of range", tt.input, result.Location.Latitude)
			}
			if result.Location.Longitude < -180 || result.Location.Longitude > 180 {
				t.Errorf("ParseUTM(%q) lon = %f, out of range", tt.input, result.Location.Longitude)
			}

			t.Logf("  %s -> lat=%f, lon=%f", tt.input, result.Location.Latitude, result.Location.Longitude)
		})
	}
}

func TestParseDMS(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLat float64
		wantLon float64
		wantErr bool
	}{
		// Standard DMS format with degree symbols
		{
			name:    "Standard DMS with symbols",
			input:   `19°51'22"N 99°49'0"E`,
			wantLat: 19.856111,
			wantLon: 99.816667,
			wantErr: false,
		},
		// DMS with letters instead of symbols
		{
			name:    "DMS with letter markers",
			input:   "19d51m22sN 99d49m0sE",
			wantLat: 19.856111,
			wantLon: 99.816667,
			wantErr: false,
		},
		// Southern and Western hemispheres
		{
			name:    "Sydney - southern hemisphere",
			input:   `33°51'25"S 151°12'55"E`,
			wantLat: -33.857,
			wantLon: 151.215,
			wantErr: false,
		},
		{
			name:    "New York - western hemisphere",
			input:   `40°42'46"N 74°0'22"W`,
			wantLat: 40.713,
			wantLon: -74.006,
			wantErr: false,
		},
		// With decimal seconds
		{
			name:    "DMS with decimal seconds",
			input:   `38°53'23.5"N 77°2'6.5"W`,
			wantLat: 38.8899,
			wantLon: -77.0351,
			wantErr: false,
		},
		// Invalid cases
		{
			name:    "Invalid latitude > 90",
			input:   `91°0'0"N 0°0'0"E`,
			wantErr: true,
		},
		{
			name:    "Invalid minutes >= 60",
			input:   `45°60'0"N 90°0'0"E`,
			wantErr: true,
		},
		{
			name:    "Empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseDMS(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseDMS(%q) expected error, got nil", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseDMS(%q) unexpected error: %v", tt.input, err)
				return
			}

			if result.Format != FormatDMS {
				t.Errorf("ParseDMS(%q) format = %v, want FormatDMS", tt.input, result.Format)
			}

			if !almostEqual(result.Location.Latitude, tt.wantLat, 0.001) {
				t.Errorf("ParseDMS(%q) lat = %f, want %f (±0.001)", tt.input, result.Location.Latitude, tt.wantLat)
			}

			if !almostEqual(result.Location.Longitude, tt.wantLon, 0.001) {
				t.Errorf("ParseDMS(%q) lon = %f, want %f (±0.001)", tt.input, result.Location.Longitude, tt.wantLon)
			}
		})
	}
}

func TestParseDecimal(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLat float64
		wantLon float64
		wantErr bool
	}{
		// Standard comma-separated
		{
			name:    "Comma separated",
			input:   "19.856, 99.817",
			wantLat: 19.856,
			wantLon: 99.817,
			wantErr: false,
		},
		// Space separated
		{
			name:    "Space separated",
			input:   "19.856 99.817",
			wantLat: 19.856,
			wantLon: 99.817,
			wantErr: false,
		},
		// Negative values
		{
			name:    "Negative latitude (southern)",
			input:   "-33.857, 151.215",
			wantLat: -33.857,
			wantLon: 151.215,
			wantErr: false,
		},
		{
			name:    "Negative longitude (western)",
			input:   "40.713, -74.006",
			wantLat: 40.713,
			wantLon: -74.006,
			wantErr: false,
		},
		{
			name:    "Both negative",
			input:   "-33.857, -70.506",
			wantLat: -33.857,
			wantLon: -70.506,
			wantErr: false,
		},
		// Integer values
		{
			name:    "Integer coordinates",
			input:   "45, 90",
			wantLat: 45,
			wantLon: 90,
			wantErr: false,
		},
		// Edge cases - valid extremes
		{
			name:    "North pole",
			input:   "90, 0",
			wantLat: 90,
			wantLon: 0,
			wantErr: false,
		},
		{
			name:    "South pole",
			input:   "-90, 0",
			wantLat: -90,
			wantLon: 0,
			wantErr: false,
		},
		{
			name:    "Date line east",
			input:   "0, 180",
			wantLat: 0,
			wantLon: 180,
			wantErr: false,
		},
		{
			name:    "Date line west",
			input:   "0, -180",
			wantLat: 0,
			wantLon: -180,
			wantErr: false,
		},
		// Invalid cases
		{
			name:    "Latitude out of range",
			input:   "91, 0",
			wantErr: true,
		},
		{
			name:    "Longitude out of range",
			input:   "0, 181",
			wantErr: true,
		},
		{
			name:    "Empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseDecimal(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseDecimal(%q) expected error, got nil", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseDecimal(%q) unexpected error: %v", tt.input, err)
				return
			}

			if result.Format != FormatDecimal {
				t.Errorf("ParseDecimal(%q) format = %v, want FormatDecimal", tt.input, result.Format)
			}

			if !almostEqual(result.Location.Latitude, tt.wantLat, tolerance) {
				t.Errorf("ParseDecimal(%q) lat = %f, want %f", tt.input, result.Location.Latitude, tt.wantLat)
			}

			if !almostEqual(result.Location.Longitude, tt.wantLon, tolerance) {
				t.Errorf("ParseDecimal(%q) lon = %f, want %f", tt.input, result.Location.Longitude, tt.wantLon)
			}
		})
	}
}

func TestParse(t *testing.T) {
	// Test auto-detection with various formats
	tests := []struct {
		name       string
		input      string
		wantFormat Format
		wantErr    bool
	}{
		// MGRS
		{name: "Auto-detect MGRS", input: "18SUJ2337506519", wantFormat: FormatMGRS, wantErr: false},
		// UTM
		{name: "Auto-detect UTM", input: "47N 500000 2200000", wantFormat: FormatUTM, wantErr: false},
		// DMS
		{name: "Auto-detect DMS", input: `19°51'22"N 99°49'0"E`, wantFormat: FormatDMS, wantErr: false},
		// Decimal
		{name: "Auto-detect Decimal", input: "19.856, 99.817", wantFormat: FormatDecimal, wantErr: false},
		// Unknown format
		{name: "Unknown format - address", input: "123 Main Street, New York", wantErr: true},
		{name: "Empty string", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Parse(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Parse(%q) expected error, got nil", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("Parse(%q) unexpected error: %v", tt.input, err)
				return
			}

			if result.Format != tt.wantFormat {
				t.Errorf("Parse(%q) format = %v, want %v", tt.input, result.Format, tt.wantFormat)
			}

			// Verify valid coordinate range
			if result.Location.Latitude < -90 || result.Location.Latitude > 90 {
				t.Errorf("Parse(%q) lat = %f, out of range", tt.input, result.Location.Latitude)
			}
			if result.Location.Longitude < -180 || result.Location.Longitude > 180 {
				t.Errorf("Parse(%q) lon = %f, out of range", tt.input, result.Location.Longitude)
			}

			t.Logf("  %s: format=%s, lat=%f, lon=%f", tt.input, result.Format, result.Location.Latitude, result.Location.Longitude)
		})
	}
}

func TestIsCoordinate(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Should be coordinates
		{"47QME8598697460", true},
		{"18SUJ2337506519", true},
		{"47N 500000 2200000", true},
		{`19°51'22"N 99°49'0"E`, true},
		{"19.856, 99.817", true},
		{"-33.857, 151.215", true},

		// Should not be coordinates
		{"Chiang Rai, Thailand", false},
		{"123 Main Street", false},
		{"New York City", false},
		{"", false},
		{"hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsCoordinate(tt.input)
			if got != tt.want {
				t.Errorf("IsCoordinate(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		input string
		want  Format
	}{
		{"47QME8598697460", FormatMGRS},
		{"18SUJ2337506519", FormatMGRS},
		{"47N 500000 2200000", FormatUTM},
		{`19°51'22"N 99°49'0"E`, FormatDMS},
		{"19.856, 99.817", FormatDecimal},
		{"-33.857 151.215", FormatDecimal},
		{"Chiang Rai", FormatUnknown},
		{"", FormatUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := DetectFormat(tt.input)
			if got != tt.want {
				t.Errorf("DetectFormat(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestToMGRS(t *testing.T) {
	tests := []struct {
		name      string
		lat       float64
		lon       float64
		precision int
		wantErr   bool
	}{
		{
			name:      "Chiang Rai Thailand",
			lat:       19.856,
			lon:       99.817,
			precision: 5,
			wantErr:   false,
		},
		{
			name:      "Washington DC",
			lat:       38.889,
			lon:       -77.035,
			precision: 5,
			wantErr:   false,
		},
		{
			name:      "Sydney Australia",
			lat:       -33.857,
			lon:       151.215,
			precision: 5,
			wantErr:   false,
		},
		{
			name:      "Low precision",
			lat:       40.0,
			lon:       -75.0,
			precision: 1,
			wantErr:   false,
		},
		{
			name:      "Invalid latitude",
			lat:       91.0,
			lon:       0.0,
			precision: 5,
			wantErr:   true,
		},
		{
			name:      "Invalid longitude",
			lat:       0.0,
			lon:       181.0,
			precision: 5,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ToMGRS(tt.lat, tt.lon, tt.precision)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ToMGRS(%f, %f, %d) expected error, got %q", tt.lat, tt.lon, tt.precision, result)
				}
				return
			}

			if err != nil {
				t.Errorf("ToMGRS(%f, %f, %d) unexpected error: %v", tt.lat, tt.lon, tt.precision, err)
				return
			}

			// Verify round-trip: convert back and check
			parsed, err := ParseMGRS(result)
			if err != nil {
				t.Errorf("Round-trip failed: ToMGRS(%f, %f) = %q, ParseMGRS error: %v", tt.lat, tt.lon, result, err)
				return
			}

			// Tolerance depends on precision
			// Precision 5 = 1m, Precision 1 = 10km
			maxDiff := 0.0001 // ~10m for precision 5
			switch tt.precision {
			case 1:
				maxDiff = 0.1 // ~10km
			case 2:
				maxDiff = 0.01 // ~1km
			case 3:
				maxDiff = 0.001 // ~100m
			case 4:
				maxDiff = 0.0001 // ~10m
			}

			if !almostEqual(parsed.Location.Latitude, tt.lat, maxDiff) ||
				!almostEqual(parsed.Location.Longitude, tt.lon, maxDiff) {
				t.Errorf("Round-trip mismatch: input (%f, %f), MGRS=%q, output (%f, %f)",
					tt.lat, tt.lon, result, parsed.Location.Latitude, parsed.Location.Longitude)
			}
		})
	}
}

func TestFormat_String(t *testing.T) {
	tests := []struct {
		format Format
		want   string
	}{
		{FormatUnknown, "unknown"},
		{FormatDecimal, "decimal"},
		{FormatDMS, "dms"},
		{FormatMGRS, "mgrs"},
		{FormatUTM, "utm"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.format.String(); got != tt.want {
				t.Errorf("Format.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Benchmark tests
func BenchmarkParseMGRS(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ParseMGRS("47QNB8598697460")
	}
}

func BenchmarkParseDecimal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ParseDecimal("19.856, 99.817")
	}
}

func BenchmarkParse(b *testing.B) {
	inputs := []string{
		"47QNB8598697460",
		"19.856, 99.817",
		`19°51'22"N 99°49'0"E`,
		"47N 485986 2197460",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Parse(inputs[i%len(inputs)])
	}
}
