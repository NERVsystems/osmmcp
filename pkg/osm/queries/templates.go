// Package queries provides utilities for building OpenStreetMap API queries.
package queries

import (
	"fmt"
	"strings"
)

// OverpassBuilder provides a fluent interface for building Overpass API queries.
// It allows for composing complex queries with proper syntax and formatting.
type OverpassBuilder struct {
	buf        strings.Builder
	elements   []string
	hasElement bool
}

// NewOverpassBuilder creates a new Overpass query builder with initial settings.
// All queries start with [out:json] to request JSON output format.
func NewOverpassBuilder() *OverpassBuilder {
	b := &OverpassBuilder{
		elements: make([]string, 0),
	}
	b.buf.WriteString("[out:json];")
	return b
}

// WithNode adds a node query around a point with specified radius and tags.
func (b *OverpassBuilder) WithNode(lat, lon, radius float64, tags map[string]string) *OverpassBuilder {
	query := fmt.Sprintf("node(around:%f,%f,%f)", radius, lat, lon)
	b.addElement(query, tags)
	return b
}

// WithWay adds a way query around a point with specified radius and tags.
func (b *OverpassBuilder) WithWay(lat, lon, radius float64, tags map[string]string) *OverpassBuilder {
	query := fmt.Sprintf("way(around:%f,%f,%f)", radius, lat, lon)
	b.addElement(query, tags)
	return b
}

// WithRelation adds a relation query around a point with specified radius and tags.
func (b *OverpassBuilder) WithRelation(lat, lon, radius float64, tags map[string]string) *OverpassBuilder {
	query := fmt.Sprintf("relation(around:%f,%f,%f)", radius, lat, lon)
	b.addElement(query, tags)
	return b
}

// WithArea adds an area query with the specified ID and tags.
func (b *OverpassBuilder) WithArea(areaId string, tags map[string]string) *OverpassBuilder {
	query := fmt.Sprintf("node(area:%s)", areaId)
	b.addElement(query, tags)
	return b
}

// WithNodeInBbox adds a node query within a bounding box and with specified tags.
func (b *OverpassBuilder) WithNodeInBbox(minLat, minLon, maxLat, maxLon float64, tags map[string]string) *OverpassBuilder {
	query := fmt.Sprintf("node(%f,%f,%f,%f)", minLat, minLon, maxLat, maxLon)
	b.addElement(query, tags)
	return b
}

// WithWayInBbox adds a way query within a bounding box and with specified tags.
func (b *OverpassBuilder) WithWayInBbox(minLat, minLon, maxLat, maxLon float64, tags map[string]string) *OverpassBuilder {
	query := fmt.Sprintf("way(%f,%f,%f,%f)", minLat, minLon, maxLat, maxLon)
	b.addElement(query, tags)
	return b
}

// WithRelationInBbox adds a relation query within a bounding box and with specified tags.
func (b *OverpassBuilder) WithRelationInBbox(minLat, minLon, maxLat, maxLon float64, tags map[string]string) *OverpassBuilder {
	query := fmt.Sprintf("relation(%f,%f,%f,%f)", minLat, minLon, maxLat, maxLon)
	b.addElement(query, tags)
	return b
}

// WithBbox adds both node and way queries within a bounding box with specified tags.
func (b *OverpassBuilder) WithBbox(minLat, minLon, maxLat, maxLon float64, tags map[string]string) *OverpassBuilder {
	return b.WithNodeInBbox(minLat, minLon, maxLat, maxLon, tags).
		WithWayInBbox(minLat, minLon, maxLat, maxLon, tags)
}

// WithKey adds a query for elements with the specified key around a location.
func (b *OverpassBuilder) WithKey(key string, lat, lon, radius float64) *OverpassBuilder {
	tags := map[string]string{
		key: "",
	}
	return b.WithNode(lat, lon, radius, tags).
		WithWay(lat, lon, radius, tags)
}

// WithAmenity is a convenience method to search for elements with the amenity tag.
func (b *OverpassBuilder) WithAmenity(value string, lat, lon, radius float64) *OverpassBuilder {
	tags := map[string]string{
		"amenity": value,
	}
	return b.WithNode(lat, lon, radius, tags).
		WithWay(lat, lon, radius, tags)
}

// Begin starts a group of queries with parentheses.
// This is required when using multiple element filters.
func (b *OverpassBuilder) Begin() *OverpassBuilder {
	if !b.hasElement {
		b.buf.WriteString("(")
		b.hasElement = true
	}
	return b
}

// End ends a group of queries with parentheses and adds the output statement.
// By default, it uses 'out body;' to include tag information in the results.
func (b *OverpassBuilder) End() *OverpassBuilder {
	if b.hasElement {
		b.buf.WriteString(");out body;")
	}
	return b
}

// WithOutput specifies a custom output format (default is 'body').
// Common options include 'body', 'center', 'geom', etc.
func (b *OverpassBuilder) WithOutput(outputType string) *OverpassBuilder {
	if b.hasElement {
		b.buf.WriteString(fmt.Sprintf(");out %s;", outputType))
	}
	return b
}

// Build returns the complete Overpass query string.
// This should be called after all query elements have been added
// and End() or WithOutput() has been called.
func (b *OverpassBuilder) Build() string {
	return b.buf.String()
}

// addElement adds a query element with tags to the builder.
// This is an internal helper method used by the public With* methods.
func (b *OverpassBuilder) addElement(baseQuery string, tags map[string]string) {
	// Ensure we're in a group
	if !b.hasElement {
		b.Begin()
	}

	// Build the element query with all tags
	var query strings.Builder
	query.WriteString(baseQuery)

	// Add tags as filters
	for key, value := range tags {
		if value == "" {
			// Just check for the presence of the key
			query.WriteString(fmt.Sprintf("[%s]", key))
		} else {
			// Check for specific key=value
			query.WriteString(fmt.Sprintf("[%s=%s]", key, value))
		}
	}

	// Add semicolon
	query.WriteString(";")

	// Add to the main query
	b.buf.WriteString(query.String())
}

// Examples of use:
//
// Find restaurants near a location:
//
//   query := NewOverpassBuilder().
//     WithAmenity("restaurant", lat, lon, 1000).
//     End().
//     Build()
//
// Find multiple amenities:
//
//   query := NewOverpassBuilder().
//     Begin().
//     WithAmenity("restaurant", lat, lon, 1000).
//     WithAmenity("cafe", lat, lon, 1000).
//     WithAmenity("bar", lat, lon, 1000).
//     End().
//     Build()
//
// Search in a bounding box:
//
//   query := NewOverpassBuilder().
//     WithBbox(minLat, minLon, maxLat, maxLon, map[string]string{"amenity": "school"}).
//     WithOutput("center").
//     Build()

// StandardQueries contains common query templates
var StandardQueries = struct {
	ChargingStations   func(lat, lon, radius float64) string
	Restaurants        func(lat, lon, radius float64) string
	Parks              func(lat, lon, radius float64) string
	Schools            func(lat, lon, radius float64) string
	PublicTransport    func(lat, lon, radius float64) string
	NeighborhoodSearch func(lat, lon, radius float64) string
}{
	ChargingStations: func(lat, lon, radius float64) string {
		return NewOverpassBuilder().
			Begin().
			WithAmenity("charging_station", lat, lon, radius).
			End().
			WithOutput("body").
			Build()
	},

	Restaurants: func(lat, lon, radius float64) string {
		return NewOverpassBuilder().
			Begin().
			WithNode(lat, lon, radius, map[string]string{"amenity": "restaurant"}).
			WithNode(lat, lon, radius, map[string]string{"amenity": "fast_food"}).
			WithNode(lat, lon, radius, map[string]string{"amenity": "cafe"}).
			WithNode(lat, lon, radius, map[string]string{"amenity": "bar"}).
			End().
			WithOutput("body").
			Build()
	},

	Parks: func(lat, lon, radius float64) string {
		return NewOverpassBuilder().
			Begin().
			WithNode(lat, lon, radius, map[string]string{"leisure": "park"}).
			WithWay(lat, lon, radius, map[string]string{"leisure": "park"}).
			WithNode(lat, lon, radius, map[string]string{"leisure": "garden"}).
			WithWay(lat, lon, radius, map[string]string{"leisure": "garden"}).
			End().
			WithOutput("body").
			Build()
	},

	Schools: func(lat, lon, radius float64) string {
		return NewOverpassBuilder().
			Begin().
			WithAmenity("school", lat, lon, radius).
			WithAmenity("university", lat, lon, radius).
			WithAmenity("kindergarten", lat, lon, radius).
			End().
			WithOutput("body").
			Build()
	},

	PublicTransport: func(lat, lon, radius float64) string {
		return NewOverpassBuilder().
			Begin().
			WithNode(lat, lon, radius, map[string]string{"public_transport": ""}).
			WithNode(lat, lon, radius, map[string]string{"highway": "bus_stop"}).
			WithNode(lat, lon, radius, map[string]string{"railway": "station"}).
			WithNode(lat, lon, radius, map[string]string{"railway": "tram_stop"}).
			End().
			WithOutput("body").
			Build()
	},

	NeighborhoodSearch: func(lat, lon, radius float64) string {
		return NewOverpassBuilder().
			Begin().
			WithNode(lat, lon, radius, map[string]string{"place": "neighbourhood"}).
			WithNode(lat, lon, radius, map[string]string{"place": "suburb"}).
			WithNode(lat, lon, radius, map[string]string{"place": "quarter"}).
			WithNode(lat, lon, radius, map[string]string{"place": "district"}).
			End().
			WithOutput("body").
			Build()
	},
}
