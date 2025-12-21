package coords_test

import (
	"testing"

	"github.com/NERVsystems/osmmcp/pkg/coords"
)

// TestChiangRaiMGRS tests the specific MGRS coordinate from the CASEVAC incident
func TestChiangRaiMGRS(t *testing.T) {
	// This is the actual MGRS coordinate from the failed CASEVAC scenario
	// Zone 47Q is in Thailand/SE Asia, NOT Middle East

	// First, let's get the correct MGRS for Chiang Rai
	chiangRaiLat := 19.856
	chiangRaiLon := 99.817

	mgrsStr, err := coords.ToMGRS(chiangRaiLat, chiangRaiLon, 5)
	if err != nil {
		t.Fatalf("ToMGRS failed: %v", err)
	}

	t.Logf("Chiang Rai MGRS: %s", mgrsStr)

	// Verify the zone is 47Q (Thailand region)
	if mgrsStr[:3] != "47Q" {
		t.Errorf("Expected zone 47Q, got %s", mgrsStr[:3])
	}

	// Now parse it back
	result, err := coords.Parse(mgrsStr)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Verify it's in Thailand (roughly)
	if result.Location.Latitude < 15 || result.Location.Latitude > 25 {
		t.Errorf("Latitude %f not in Thailand range (15-25)", result.Location.Latitude)
	}
	if result.Location.Longitude < 95 || result.Location.Longitude > 105 {
		t.Errorf("Longitude %f not in Thailand range (95-105)", result.Location.Longitude)
	}

	t.Logf("Parsed back: lat=%f, lon=%f", result.Location.Latitude, result.Location.Longitude)
}

// TestMGRSZoneValidation ensures we correctly identify geographic zones
func TestMGRSZoneValidation(t *testing.T) {
	testCases := []struct {
		name        string
		lat         float64
		lon         float64
		expectZone  string
		description string
	}{
		{
			name:        "Chiang Rai Thailand",
			lat:         19.856,
			lon:         99.817,
			expectZone:  "47Q",
			description: "Northern Thailand",
		},
		{
			name:        "Bangkok Thailand",
			lat:         13.756,
			lon:         100.502,
			expectZone:  "47P",
			description: "Central Thailand",
		},
		{
			name:        "Washington DC",
			lat:         38.889,
			lon:         -77.035,
			expectZone:  "18S",
			description: "Eastern US",
		},
		{
			name:        "London UK",
			lat:         51.501,
			lon:         -0.125,
			expectZone:  "30U",
			description: "Western Europe",
		},
		{
			name:        "Sydney Australia",
			lat:         -33.857,
			lon:         151.215,
			expectZone:  "56H",
			description: "Eastern Australia",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mgrsStr, err := coords.ToMGRS(tc.lat, tc.lon, 5)
			if err != nil {
				t.Fatalf("ToMGRS failed: %v", err)
			}

			zone := mgrsStr[:3]
			if zone != tc.expectZone {
				t.Errorf("%s: expected zone %s, got %s (full: %s)",
					tc.description, tc.expectZone, zone, mgrsStr)
			} else {
				t.Logf("%s (%f, %f) -> %s ✓", tc.description, tc.lat, tc.lon, mgrsStr)
			}
		})
	}
}

// TestAutoDetectionDoesNotConfuseAddresses ensures place names aren't detected as coordinates
func TestAutoDetectionDoesNotConfuseAddresses(t *testing.T) {
	placenames := []string{
		"Chiang Rai, Thailand",
		"123 Main Street, New York",
		"Washington DC",
		"Tokyo, Japan",
		"Merlion Park Singapore",
		"Big Ben London",
		"some random text",
		"NERVA tactical AI",
	}

	for _, name := range placenames {
		t.Run(name, func(t *testing.T) {
			if coords.IsCoordinate(name) {
				t.Errorf("Place name %q incorrectly detected as coordinate", name)
			}
		})
	}
}
