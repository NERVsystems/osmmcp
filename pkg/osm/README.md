# OSM Package

This package provides common utilities for working with OpenStreetMap data in the osmmcp project.

## Overview

The `osm` package contains reusable components for interacting with OpenStreetMap services and data. It centralizes various constants, data structures, and utility functions to ensure consistency across the codebase and reduce duplication.

## Components

### Files and Modules

#### `client.go`
* `NewClient()` - Returns a pre-configured HTTP client for OSM API requests
  - Connection pooling (100 idle connections, 10 per host)
  - 30-second timeout for requests
  - Proper transport configuration

#### `util.go`
* **Constants:**
  - `NominatimBaseURL` - Base URL for Nominatim geocoding service
  - `OverpassBaseURL` - Base URL for Overpass API to query OSM data
  - `OSRMBaseURL` - Base URL for OSRM routing service
  - `UserAgent` - User agent string for API requests (compliance with OSM policies)
* **Data:**
  - `CategoryMap` - Maps common category names (restaurant, park, cafe, etc.) to OSM tags
* **Functions:**
  - `ValidateCoordinates()` - Validates latitude and longitude ranges

#### `ratelimit.go`
* **Rate Limiters:**
  - `NominatimLimiter` - 1 request/second with burst of 1 (Nominatim policy compliance)
  - `OverpassLimiter` - 2 requests/minute with burst of 2 (Overpass API guidelines)
  - `OSRMLimiter` - 100 requests/minute with burst of 5 (OSRM routing service)
* **Functions:**
  - `InitRateLimiters()` - Initialize rate limiters with custom or default settings

#### `polyline.go`
* `EncodePolyline()` - Encodes coordinates to Google Polyline5 format
* `DecodePolyline()` - Decodes Google Polyline5 format to coordinates
* Used for efficient route geometry transmission (reduces payload size)

#### `cache.go`
* `GetCachedResponse()` - Retrieve cached API responses
* `CacheResponse()` - Store API responses with TTL
* Integrates with `pkg/cache` TTL cache for response caching

#### `queries/templates.go`
* Overpass query templates for various search operations
* Pre-formatted QL queries for common use cases

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

// Validate coordinates
if err := osm.ValidateCoordinates(lat, lon); err != nil {
    // Handle invalid coordinates
}

// Get category-specific OSM tags
restaurantTags := osm.CategoryMap["restaurant"]

// Encode/decode polylines for route geometry
polyline := osm.EncodePolyline(coordinates)
coords := osm.DecodePolyline(polyline)

// For geographic calculations, use pkg/geo:
// - geo.HaversineDistance() for distance calculations
// - geo.NewBoundingBox() for bounding boxes
// - geo.Location for coordinate representation
```

## Design Principles

The package follows these design principles:

1. **Single Responsibility**: Each file and component has a clear, focused purpose
2. **Reusability**: Components are designed to be reused across all 14 tools
3. **API Compliance**: Strict adherence to OpenStreetMap API usage policies via rate limiting
4. **Performance**: Connection pooling, response caching, and efficient polyline encoding
5. **Security**: Properly configured timeouts, connection limits, and coordinate validation
6. **Separation of Concerns**: OSM-specific utilities in this package, generic geographic calculations in `pkg/geo`

## Related Packages

* **`pkg/geo`** - Geographic types and calculations (Location, BoundingBox, HaversineDistance)
* **`pkg/cache`** - TTL-based caching layer used for response caching
* **`pkg/tools`** - Tool implementations that use this package's utilities
* **`pkg/server`** - MCP server that orchestrates tool execution 