package tools

import (
	"strings"
	"testing"

	"github.com/NERVsystems/osmmcp/pkg/core"
)

// buildCategoryQuery mirrors how HandleFindNearbyPlaces assembles its query.
func buildCategoryQuery(category string) string {
	b := core.NewOverpassBuilder().
		WithTimeout(25).
		WithCenter(54.3486, 18.6536, 800)
	applyCategoryTags(b, mapCategoryToOSMTags(category))
	return b.Build()
}

// TestCategoryQueryORsValuesWithinAKey guards the original find_nearby_places
// bug: values for one key were appended as separate filters and therefore
// AND-ed, so no element could ever match.
func TestCategoryQueryORsValuesWithinAKey(t *testing.T) {
	query := buildCategoryQuery("restaurant")

	want := `[amenity~"^(restaurant|cafe|fast_food|bar|pub|food_court)$"]`
	if !strings.Contains(query, want) {
		t.Errorf("expected OR-ed amenity filter %q in query:\n%s", want, query)
	}
	if strings.Contains(query, "[amenity=restaurant][amenity=cafe]") {
		t.Errorf("values for a single key are AND-ed:\n%s", query)
	}
}

// TestCategoryQueryORsAcrossKeys guards the follow-up bug: categories mapping
// to several tag keys must union the keys, not require all of them at once.
func TestCategoryQueryORsAcrossKeys(t *testing.T) {
	for _, category := range []string{"cafe", "pharmacy", "park", "museum", "transport", "sasquatch"} {
		t.Run(category, func(t *testing.T) {
			query := buildCategoryQuery(category)

			for _, statement := range strings.Split(query, ";") {
				statement = strings.TrimPrefix(statement, "(")
				if !strings.HasPrefix(statement, "node(") &&
					!strings.HasPrefix(statement, "way(") &&
					!strings.HasPrefix(statement, "relation(") {
					continue // settings header or output directive
				}
				if n := strings.Count(statement, "["); n > 1 {
					t.Errorf("statement %q requires %d tag keys at once; keys must be unioned\nfull query:\n%s",
						statement, n, query)
				}
			}

			// Every key must still appear somewhere in the union.
			for key := range mapCategoryToOSMTags(category) {
				if !strings.Contains(query, "["+key) {
					t.Errorf("key %q missing from query:\n%s", key, query)
				}
			}
		})
	}
}

// TestCategoryQueryCoversAllElementTypes ensures ways and relations are still
// searched, since many parks, buildings and stations are not nodes.
func TestCategoryQueryCoversAllElementTypes(t *testing.T) {
	query := buildCategoryQuery("cafe")

	for _, elementType := range []string{"node", "way", "relation"} {
		if !strings.Contains(query, elementType+"(around:") {
			t.Errorf("query is missing %s statements:\n%s", elementType, query)
		}
	}
}

// TestCategoryQueryIsDeterministic ensures map iteration order does not leak
// into the generated query, which would make it uncacheable and untestable.
func TestCategoryQueryIsDeterministic(t *testing.T) {
	first := buildCategoryQuery("transport")
	for i := 0; i < 20; i++ {
		if got := buildCategoryQuery("transport"); got != first {
			t.Fatalf("query is not deterministic:\n%s\nvs\n%s", first, got)
		}
	}
}

// TestCategoryQueryWildcardShop checks the "any shop" mapping degrades to an
// existence check rather than a literal [shop=*].
func TestCategoryQueryWildcardShop(t *testing.T) {
	query := buildCategoryQuery("shopping")

	if !strings.Contains(query, "[shop]") {
		t.Errorf("expected bare [shop] existence filter in query:\n%s", query)
	}
	if strings.Contains(query, "shop=*") {
		t.Errorf("wildcard leaked into query as a literal value:\n%s", query)
	}
}

// TestBuildOutputsCentre ensures ways and relations come back with coordinates.
func TestBuildOutputsCentre(t *testing.T) {
	query := buildCategoryQuery("park")

	if !strings.HasSuffix(query, ");out center;") {
		t.Errorf("query must end with an out center directive:\n%s", query)
	}
	if strings.Contains(query, ">;") {
		t.Errorf("recurse-down makes out center apply to child nodes:\n%s", query)
	}
}
