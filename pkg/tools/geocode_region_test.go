package tools

import (
	"os"
	"slices"
	"testing"
)

// The regression these tests pin down: defaultRegion used to be "Singapore",
// and ensureRegion appends the region to any query of fewer than three words
// with no comma. Every terse place name was therefore rewritten before it ever
// reached Nominatim ("Eiffel Tower" -> "Eiffel Tower Singapore") and came back
// NO_RESULTS. No test covered it because the handler tests either use verbose,
// comma-qualified addresses or pass region explicitly.

// A geocoder must not invent a region context the caller never asked for.
func TestDefaultRegionEmptyUnlessConfigured(t *testing.T) {
	if env := os.Getenv("OSMMCP_DEFAULT_REGION"); env != "" {
		t.Skipf("OSMMCP_DEFAULT_REGION is set to %q in this environment", env)
	}
	if defaultRegion != "" {
		t.Errorf("defaultRegion = %q, want \"\" — an implicit region silently rewrites every short query", defaultRegion)
	}
}

func TestEnsureRegion(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		region string
		want   string
	}{
		{"no region leaves a short query alone", "Eiffel Tower", "", "Eiffel Tower"},
		{"no region leaves a single word alone", "Denver", "", "Denver"},
		{"configured region is appended to a short query", "Merlion Park", "Singapore", "Merlion Park Singapore"},
		{"three or more words are left alone", "Golden Gate Bridge", "Singapore", "Golden Gate Bridge"},
		{"a comma means the caller qualified it", "Denver, Colorado", "Singapore", "Denver, Colorado"},
		{"region already present is not doubled", "Merlion Park Singapore", "Singapore", "Merlion Park Singapore"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ensureRegion(tt.query, tt.region); got != tt.want {
				t.Errorf("ensureRegion(%q, %q) = %q, want %q", tt.query, tt.region, got, tt.want)
			}
		})
	}
}

// Region augmentation must only ADD candidates. Whatever the region context,
// the caller's exact query has to be tried before the tool reports NO_RESULTS.
func TestGeocodeQuerySequenceAlwaysIncludesBareAddress(t *testing.T) {
	cases := []struct {
		address       string
		withoutParens string
		parensContent string
		region        string
	}{
		{"Eiffel Tower", "Eiffel Tower", "", ""},
		{"Eiffel Tower", "Eiffel Tower", "", "Singapore"},
		{"Fort Bragg", "Fort Bragg", "", "Singapore"},
		{"Hawksbill Mountain", "Hawksbill Mountain", "", "Afghanistan"},
		{"Merlion Park (Singapore)", "Merlion Park", "Singapore", "Singapore"},
		{"Blue Temple (Wat Rong Suea Ten) in Chiang Rai", "Blue Temple in Chiang Rai", "Wat Rong Suea Ten", "Thailand"},
	}

	for _, c := range cases {
		got := geocodeQuerySequence(c.address, c.withoutParens, c.parensContent, c.region)
		if !slices.Contains(got, c.address) {
			t.Errorf("geocodeQuerySequence(%q, region=%q) = %v; the caller's exact query is missing",
				c.address, c.region, got)
		}
		seen := map[string]bool{}
		for _, q := range got {
			if seen[q] {
				t.Errorf("geocodeQuerySequence(%q, region=%q) = %v; duplicate query %q wastes a rate-limited Nominatim call",
					c.address, c.region, got, q)
			}
			seen[q] = true
		}
	}
}

// With a region configured, the augmented form is tried FIRST (it disambiguates
// when the region is correct) and the bare form is the fallback.
func TestGeocodeQuerySequenceOrder(t *testing.T) {
	got := geocodeQuerySequence("Merlion Park", "Merlion Park", "", "Singapore")
	want := []string{"Merlion Park Singapore", "Merlion Park"}
	if !slices.Equal(got, want) {
		t.Errorf("geocodeQuerySequence = %v, want %v", got, want)
	}
}
