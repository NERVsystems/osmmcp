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
