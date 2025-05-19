// Package core provides shared utilities for the OpenStreetMap MCP tools.
package core

import (
	"fmt"
	"strings"

	"github.com/NERVsystems/osmmcp/pkg/geo"
)

// OverpassBuilder provides a fluent interface for building Overpass API queries
type OverpassBuilder struct {
	outFormat      string
	timeout        int
	elements       []string
	bbox           *geo.BoundingBox
	center         *LocationRadius
	globalTags     []TagFilter
	elementFilters []ElementFilter
}

// LocationRadius represents a center point with a radius
type LocationRadius struct {
	Lat    float64
	Lon    float64
	Radius float64
}

// TagFilter represents a tag filter for Overpass queries
type TagFilter struct {
	Key     string
	Values  []string
	Exclude bool
}

// ElementFilter represents a filter with tags for a specific element type
type ElementFilter struct {
	ElementType string // "node", "way", "relation"
	Tags        []TagFilter
	BBox        *geo.BoundingBox // Optional bounding box
	Around      *LocationRadius  // Optional around filter
}

// NewOverpassBuilder creates a new builder with default settings
func NewOverpassBuilder() *OverpassBuilder {
	return &OverpassBuilder{
		outFormat: "json",
		timeout:   25, // Default timeout in seconds
		elements:  []string{},
	}
}

// WithTimeout sets the query timeout
func (b *OverpassBuilder) WithTimeout(seconds int) *OverpassBuilder {
	b.timeout = seconds
	return b
}

// WithOutputFormat sets the output format
func (b *OverpassBuilder) WithOutputFormat(format string) *OverpassBuilder {
	b.outFormat = format
	return b
}

// WithBoundingBox sets a bounding box filter
func (b *OverpassBuilder) WithBoundingBox(minLat, minLon, maxLat, maxLon float64) *OverpassBuilder {
	b.bbox = &geo.BoundingBox{
		MinLat: minLat,
		MinLon: minLon,
		MaxLat: maxLat,
		MaxLon: maxLon,
	}
	return b
}

// WithCenter sets a center point and radius
func (b *OverpassBuilder) WithCenter(lat, lon, radius float64) *OverpassBuilder {
	b.center = &LocationRadius{
		Lat:    lat,
		Lon:    lon,
		Radius: radius,
	}
	return b
}

// WithTag adds a global tag filter
func (b *OverpassBuilder) WithTag(key string, values ...string) *OverpassBuilder {
	b.globalTags = append(b.globalTags, TagFilter{
		Key:    key,
		Values: values,
	})
	return b
}

// WithExcludeTag adds a global exclude tag filter
func (b *OverpassBuilder) WithExcludeTag(key string, values ...string) *OverpassBuilder {
	b.globalTags = append(b.globalTags, TagFilter{
		Key:     key,
		Values:  values,
		Exclude: true,
	})
	return b
}

// WithNode adds a node filter
func (b *OverpassBuilder) WithNode(tags ...TagFilter) *OverpassBuilder {
	b.elementFilters = append(b.elementFilters, ElementFilter{
		ElementType: "node",
		Tags:        tags,
		BBox:        b.bbox,
		Around:      b.center,
	})
	return b
}

// WithWay adds a way filter
func (b *OverpassBuilder) WithWay(tags ...TagFilter) *OverpassBuilder {
	b.elementFilters = append(b.elementFilters, ElementFilter{
		ElementType: "way",
		Tags:        tags,
		BBox:        b.bbox,
		Around:      b.center,
	})
	return b
}

// WithRelation adds a relation filter
func (b *OverpassBuilder) WithRelation(tags ...TagFilter) *OverpassBuilder {
	b.elementFilters = append(b.elementFilters, ElementFilter{
		ElementType: "relation",
		Tags:        tags,
		BBox:        b.bbox,
		Around:      b.center,
	})
	return b
}

// Tag creates a TagFilter for a key with optional values
func Tag(key string, values ...string) TagFilter {
	return TagFilter{
		Key:    key,
		Values: values,
	}
}

// NotTag creates an excluding TagFilter
func NotTag(key string, values ...string) TagFilter {
	return TagFilter{
		Key:     key,
		Values:  values,
		Exclude: true,
	}
}

// Build generates the Overpass query string
func (b *OverpassBuilder) Build() string {
	var query strings.Builder

	// Add query format and timeout
	query.WriteString(fmt.Sprintf("[out:%s][timeout:%d];", b.outFormat, b.timeout))

	// Start element collection
	query.WriteString("(")

	// Process each element filter
	for _, filter := range b.elementFilters {
		query.WriteString(b.buildElementFilter(filter))
	}

	// If no explicit element filters, but we have global tags, add default filters for all element types
	if len(b.elementFilters) == 0 && len(b.globalTags) > 0 {
		// Add default filters for all three element types
		defaultTypes := []string{"node", "way", "relation"}
		for _, elementType := range defaultTypes {
			filter := ElementFilter{
				ElementType: elementType,
				Tags:        b.globalTags,
				BBox:        b.bbox,
				Around:      b.center,
			}
			query.WriteString(b.buildElementFilter(filter))
		}
	}

	// Close element collection and add output directive
	query.WriteString(");out body;")

	// Add center directive for ways and relations if needed
	if b.outFormat == "json" {
		query.WriteString(">;out center;")
	}

	return query.String()
}

// buildElementFilter generates the query part for a specific element filter
func (b *OverpassBuilder) buildElementFilter(filter ElementFilter) string {
	var elementQuery strings.Builder

	// Start with element type
	elementQuery.WriteString(filter.ElementType)

	// Add spatial filter (bbox or around)
	if filter.Around != nil {
		elementQuery.WriteString(fmt.Sprintf("(around:%.1f,%.6f,%.6f)",
			filter.Around.Radius, filter.Around.Lat, filter.Around.Lon))
	} else if filter.BBox != nil {
		elementQuery.WriteString(fmt.Sprintf("(%.6f,%.6f,%.6f,%.6f)",
			filter.BBox.MinLat, filter.BBox.MinLon, filter.BBox.MaxLat, filter.BBox.MaxLon))
	}

	// Add tag filters
	tagFilters := filter.Tags
	if len(tagFilters) == 0 {
		tagFilters = b.globalTags
	}

	for _, tag := range tagFilters {
		elementQuery.WriteString(b.buildTagFilter(tag))
	}

	elementQuery.WriteString(";")
	return elementQuery.String()
}

// buildTagFilter generates the query part for a tag filter
func (b *OverpassBuilder) buildTagFilter(filter TagFilter) string {
	// If no values provided, just check for the existence of the tag
	if len(filter.Values) == 0 {
		if filter.Exclude {
			return fmt.Sprintf("[!%s]", filter.Key)
		}
		return fmt.Sprintf("[%s]", filter.Key)
	}

	// Handle single value case
	if len(filter.Values) == 1 {
		// Special case for "*" meaning any value
		if filter.Values[0] == "*" {
			if filter.Exclude {
				return fmt.Sprintf("[!%s]", filter.Key)
			}
			return fmt.Sprintf("[%s]", filter.Key)
		}

		// Regular value
		if filter.Exclude {
			return fmt.Sprintf("[%s!=%s]", filter.Key, filter.Values[0])
		}
		return fmt.Sprintf("[%s=%s]", filter.Key, filter.Values[0])
	}

	// Multiple values using regex
	values := strings.Join(filter.Values, "|")
	if filter.Exclude {
		return fmt.Sprintf("[%s!~\"%s\"]", filter.Key, values)
	}
	return fmt.Sprintf("[%s~\"%s\"]", filter.Key, values)
}

// Example usage:
/*
query := NewOverpassBuilder().
	WithCenter(37.7749, -122.4194, 1000).
	WithTag("amenity", "restaurant", "cafe").
	WithTag("cuisine", "italian").
	Build()
*/
