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

* `NewClient()` - Returns a pre-configured HTTP client for OSM API requests with appropriate timeouts and connection pooling
* `HaversineDistance()` - Calculates distances between geographic coordinates using the Haversine formula
* `NewBoundingBox()` - Creates a new bounding box for geographic queries
* `BoundingBox.ExtendWithPoint()` - Extends a bounding box to include a point
* `BoundingBox.Buffer()` - Adds a buffer around a bounding box
* `BoundingBox.String()` - Returns a formatted string representation of a bounding box for use in Overpass queries


## Usage

```go
import (
    "github.com/NERVsystems/osmmcp/pkg/osm"
    "github.com/NERVsystems/osmmcp/pkg/geo"
)

// Create an HTTP client with connection pooling
client := osm.NewClient()

// Initialize rate limiters with default or custom settings
osm.InitRateLimiters(1.0, 1, 0.033, 2, 1.67, 5)

// Create and use a bounding box
bbox := osm.NewBoundingBox()
bbox.ExtendWithPoint(lat1, lon1)
bbox.ExtendWithPoint(lat2, lon2)
bbox.Buffer(1000) // Add 1000 meter buffer
```

## Design Principles

The package follows these design principles:

1. **Single Responsibility**: Each file and component has a clear, focused purpose
2. **Reusability**: Components are designed to be reused across all 25 tools
3. **API Compliance**: Strict adherence to OpenStreetMap API usage policies via rate limiting
4. **Performance**: Connection pooling, response caching, and efficient polyline encoding
5. **Security**: Properly configured timeouts, connection limits, and coordinate validation
6. **Separation of Concerns**: OSM-specific utilities in this package, generic geographic calculations in `pkg/geo`

## Related Packages

* **`pkg/geo`** - Geographic types and calculations (Location, BoundingBox, HaversineDistance)
* **`pkg/cache`** - TTL-based caching layer used for response caching
* **`pkg/tools`** - Tool implementations that use this package's utilities
* **`pkg/server`** - MCP server that orchestrates tool execution 