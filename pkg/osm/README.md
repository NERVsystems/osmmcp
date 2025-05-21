# OSM Package

This package provides common utilities for working with OpenStreetMap data in the osmmcp project.

## Overview

The `osm` package contains reusable components for interacting with OpenStreetMap services and data. It centralizes various constants, data structures, and utility functions to ensure consistency across the codebase and reduce duplication.

## Components

### Constants

* `NominatimBaseURL` - Base URL for Nominatim geocoding service
* `OverpassBaseURL` - Base URL for Overpass API to query OSM data
* `OSRMBaseURL` - Base URL for OSRM routing service
* `UserAgent` - User agent string to use for API requests
* `EarthRadius` - Earth radius in meters for distance calculations

### Functions

* `GetClient()` - Returns the global HTTP client for OSM API requests
* `HaversineDistance()` - Calculates distances between geographic coordinates using the Haversine formula (available in the `geo` package)
* `NewBoundingBox()` - Creates a new bounding box for geographic queries (available in the `geo` package)
* `BoundingBox.ExtendWithPoint()` - Extends a bounding box to include a point
* `BoundingBox.Buffer()` - Adds a buffer around a bounding box
* `BoundingBox.String()` - Returns a formatted string representation of a bounding box for use in Overpass queries

### Data

* `CategoryMap` - Maps common category names (restaurant, park, etc.) to OSM tags

## Usage

```go
import "github.com/NERVsystems/osmmcp/pkg/osm"

// Create an HTTP client
client := osm.GetClient(context.Background())

// Calculate distance between two points
distance := geo.HaversineDistance(lat1, lon1, lat2, lon2)

// Create and use a bounding box
bbox := geo.NewBoundingBox()
bbox.ExtendWithPoint(lat1, lon1)
bbox.ExtendWithPoint(lat2, lon2)
bbox.Buffer(1000) // Add 1000 meter buffer

// Get category-specific OSM tags
restaurantTags := osm.CategoryMap["restaurant"]
```

## Design Principles

The package follows these design principles:

1. **Single Responsibility**: Each component has a clear, focused purpose
2. **Reusability**: Components are designed to be reused across tools
3. **Abstraction**: Implementation details of OSM services are hidden behind clean interfaces
4. **Consistency**: Ensures consistent behavior across different API calls
5. **Security**: Properly configures timeouts and connection limits 