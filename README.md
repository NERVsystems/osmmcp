# OpenStreetMap MCP Server

[![Go](https://github.com/NERVsystems/osmmcp/actions/workflows/go.yml/badge.svg)](https://github.com/NERVsystems/osmmcp/actions/workflows/go.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/NERVsystems/osmmcp)](https://goreportcard.com/report/github.com/NERVsystems/osmmcp)

## Overview

This is a Go OpenStreetMap MCP server. It implements the [Model Context Protocol](https://github.com/mark3labs/mcp-go) to enable LLMs to interact with geospatial data.


Our focus is on precision, performance, maintainability, and ease of integration with MCP desktop clients.

## Features

The server provides LLMs with tools to interact with OpenStreetMap data, including:

* Geocoding addresses and place names to coordinates
* Reverse geocoding coordinates to addresses
* Finding nearby points of interest
* Calculating routes and getting directions between locations
* Searching for places by category within a bounding box
* Suggesting optimal meeting points for multiple people
* Exploring areas and getting comprehensive location information
* Finding EV charging stations near a location
* Finding EV charging stations along a route
* Analyzing commute options between home and work
* Performing neighborhood livability analysis
* Finding schools near a location
* Finding parking facilities

## Implemented Tools

| Tool Name | Description | Example Parameters |
|-----------|-------------|-------------------|
| `bbox_from_points` | Create a bounding box that encompasses all given geographic coordinates | `{"points": [{"latitude": 37.7749, "longitude": -122.4194}, {"latitude": 37.8043, "longitude": -122.2711}]}` |
| `centroid_points` | Calculate the geographic centroid (mean center) of a set of coordinates | `{"points": [{"latitude": 37.7749, "longitude": -122.4194}, {"latitude": 37.8043, "longitude": -122.2711}]}` |
| `enrich_emissions` | Enrich route options with CO2 emissions, calorie burn, and cost estimates | `{"options": [{"mode": "car", "distance": 5000}, {"mode": "bike", "distance": 4500}]}` |
| `filter_tags` | Filter OSM elements by specified tags | `{"elements": [...], "tags": {"amenity": ["restaurant", "cafe"]}}` |
| `geocode_address` | Convert an address or place name to geographic coordinates | `{"address": "1600 Pennsylvania Ave, Washington DC"}` |
| `geo_distance` | Calculate the distance between two geographic coordinates | `{"from": {"latitude": 37.7749, "longitude": -122.4194}, "to": {"latitude": 37.8043, "longitude": -122.2711}}` |
| `get_map_image` | Retrieve and display an OpenStreetMap image for analysis | `{"latitude": 37.7749, "longitude": -122.4194, "zoom": 14}` |
| `osm_query_bbox` | Query OpenStreetMap data within a bounding box with tag filters | `{"bbox": {"minLat": 37.77, "minLon": -122.42, "maxLat": 37.78, "maxLon": -122.41}, "tags": {"amenity": "restaurant"}}` |
| `polyline_decode` | Decode an encoded polyline string into a series of geographic coordinates | `{"polyline": "a~l~FfynpOnlB_pDhgEhjD"}` |
| `polyline_encode` | Encode a series of geographic coordinates into a polyline string | `{"points": [{"latitude": 37.7749, "longitude": -122.4194}, {"latitude": 37.8043, "longitude": -122.2711}]}` |
| `reverse_geocode` | Convert geographic coordinates to a human-readable address | `{"latitude": 38.8977, "longitude": -77.0365}` |
| `route_fetch` | Fetch a route between two points using OSRM routing service | `{"start": {"latitude": 37.7749, "longitude": -122.4194}, "end": {"latitude": 37.8043, "longitude": -122.2711}, "mode": "car"}` |
| `route_sample` | Sample points along a route at specified intervals | `{"polyline": "a~l~FfynpOnlB_pDhgEhjD", "interval": 100}` |
| `sort_by_distance` | Sort OSM elements by distance from a reference point | `{"elements": [...], "ref": {"latitude": 37.7749, "longitude": -122.4194}}` |
| `find_nearby_places` | Find points of interest near a specific location | `{"latitude": 37.7749, "longitude": -122.4194, "radius": 1000, "category": "restaurant", "limit": 5}` |
| `get_route_directions` | Get detailed turn-by-turn directions for a route between locations | `{"start_lat": 37.7749, "start_lon": -122.4194, "end_lat": 37.8043, "end_lon": -122.2711, "mode": "car"}` |
| `suggest_meeting_point` | Suggest an optimal meeting point for multiple people | `{"locations": [{"latitude": 37.7749, "longitude": -122.4194}, {"latitude": 37.8043, "longitude": -122.2711}], "category": "cafe", "limit": 3}` |
| `explore_area` | Explore an area and get comprehensive information about it | `{"latitude": 37.7749, "longitude": -122.4194, "radius": 1000}` |
| `find_charging_stations` | Find electric vehicle charging stations near a location | `{"latitude": 37.7749, "longitude": -122.4194, "radius": 5000, "limit": 10}` |
| `analyze_commute` | Analyze transportation options between home and work locations | `{"home_latitude": 37.7749, "home_longitude": -122.4194, "work_latitude": 37.8043, "work_longitude": -122.2711, "transport_modes": ["car", "cycling", "walking"]}` |
| `analyze_neighborhood` | Evaluate neighborhood livability for real estate and relocation decisions | `{"latitude": 37.7749, "longitude": -122.4194, "radius": 1000, "include_price_data": true}` |
| `find_schools_nearby` | Find educational institutions near a specific location | `{"latitude": 37.7749, "longitude": -122.4194, "radius": 2000, "school_type": "elementary", "limit": 5}` |
| `find_parking_facilities` | Find parking facilities near a specific location | `{"latitude": 37.7749, "longitude": -122.4194, "radius": 1000, "type": "surface", "include_private": false, "limit": 5}` |

## New Geographic and Routing Tools

The v0.1.1 release includes enhanced geographic and routing capabilities:

### Geographic Tools

- **Bounding Box Generation**: Create geographic bounding boxes that encompass multiple points.
- **Centroid Calculation**: Find the mean center of a set of geographic coordinates.
- **Distance Calculation**: Calculate precise distances between geographic points using the Haversine formula.
- **OSM Element Filtering**: Filter and sort OpenStreetMap elements by tags and distance.

### Polyline Tools

- **Polyline Encoding/Decoding**: Convert between geographic coordinates and Google's Polyline5 format.
- **Route Sampling**: Sample points along routes at specific intervals for detailed analysis.

### Route Tools

- **Route Fetching**: Obtain routes between points using the OSRM routing service.
- **Emissions Enrichment**: Enhance route options with estimated CO2 emissions, calorie burn, and cost data.

These tools provide LLMs with foundational geographic capabilities for building complex location-based applications.

## Composable Tool Design

Many of our MCP tools are designed as composable primitives, enabling novel workflows that might not have been foreseen during development.  Composite tools exist to efficiently perform common complex operations.

### Composition Principles

- **Uniform Interfaces**: All tools use consistent parameter naming (e.g., `minLat`, `maxLon`) and data structures
- **Single Responsibility**: Each tool does one thing and does it well
- **Output/Input Compatibility**: The output of one tool can be directly used as input to another
- **Functional Independence**: Tools operate without side effects or hidden dependencies
- **Precise Error Messages**: When issues occur, detailed feedback indicates exactly what went wrong

### Example Workflows

1. **Find Points of Interest Along a Path**:
   ```
   bbox_from_points → osm_query_bbox → filter_tags → sort_by_distance
   ```

2. **Find Optimal Meeting Points**:
   ```
   centroid_points → find_nearby_places → filter_tags
   ```

3. **Analyze Route Characteristics**:
   ```
   route_fetch → polyline_decode → route_sample → filter_tags
   ```

This compositional approach empowers LLMs to create emergent capabilities beyond what any individual tool provides. For example, an LLM can easily create queries like "show the five closest wheelchair-accessible cafés that are open past 22:00 along my route" by combining the appropriate primitive tools, without requiring custom server-side endpoints.

## Visual Mapping Capabilities

The MCP server provides visual mapping capabilities through one tool:

1. `get_map_image` - Returns map images in SVG format for improved visualization and analysis

### Map Images for Analysis

The `get_map_image` tool provides mapping capabilities that deliver both visual map representation and detailed metadata. This approach offers several advantages:

- **Visual Display**: Returns the actual map tile image that displays inline in the conversation
- **Direct Visualization**: Enables Claude to visually analyze the map features and geography
- **Comprehensive Metadata**: Includes precise coordinates, bounds, and scale information
- **Rich Context**: Provides a text description with a direct link to OpenStreetMap for further exploration

The tool returns both the map tile image and a structured text description containing the location coordinates, a direct link to view the map online, and detailed metadata about the map area, making it ideal for both visual analysis and comprehensive location understanding.

### Example Usage

To retrieve a map image of San Francisco at zoom level 14:

```json
{
  "name": "get_map_image",
  "arguments": {
    "latitude": 37.7749,
    "longitude": -122.4194,
    "zoom": 14
  }
}
```

The response from the tool includes:
- Visual map tile image displayed inline in the conversation
- Text description of the location with a direct OpenStreetMap link
- Precise coordinate information for the map
- Geographic bounds of the visible area
- Scale information (meters per pixel)

## Improved Geocoding Tools

The geocoding tools have been enhanced to provide more reliable results and better error handling:

### Key Improvements

- **Smart Address Preprocessing**: Automatically sanitizes inputs to improve success rates
- **Detailed Error Reporting**: Returns structured error responses with error codes and helpful suggestions
- **Better Diagnostics**: Provides detailed logging to track geocoding issues
- **Improved Formatting Guide**: Documentation with specific examples of what works well

### Best Practices for Geocoding

For optimal results when using the geocoding tools:

1. **Simplify complex queries**: 
   - Bad: "Golden Temple (Harmandir Sahib) in Amritsar"
   - Good: "Golden Temple Amritsar India"

2. **Add geographic context**: 
   - Bad: "Eiffel Tower"
   - Good: "Eiffel Tower, Paris, France"

3. **Read error suggestions**: 
   - Our enhanced error responses include specific suggestions for fixing failed queries

See the [Geocoding Tools Guide](pkg/tools/docs/geocoding.md) for comprehensive documentation and [AI Prompts for Geocoding](pkg/tools/docs/ai_prompts.md) for examples of how to guide AI systems in using these tools effectively.

## Code Architecture and Design

The code follows software engineering best practices:

1. **High Cohesion, Low Coupling** - Each package has a clear, focused responsibility
2. **Separation of Concerns** - Tools, server logic, and utilities are cleanly separated
3. **DRY (Don't Repeat Yourself)** - Common utilities are extracted into the `pkg/osm` package
4. **Security First** - HTTP clients are properly configured with timeouts and connection limits
5. **UNIX-like Composability** - Small, focused tools that can be combined in powerful ways
6. **Structured Logging** - All logging is done via `slog` with consistent levels and formats:
   - Debug: Developer detail, verbose or diagnostic messages
   - Info: Routine operational messages
   - Warn: Unexpected conditions that don't necessarily halt execution
   - Error: Critical problems, potential or actual failures
7. **SOLID Principles** - Particularly Single Responsibility and Interface Segregation
8. **Registry Pattern** - All tools are defined in a central registry for improved maintainability
9. **Google Polyline5 Format** - Standardized polyline encoding/decoding using Google's Polyline5 format
10. **Precise Geospatial Calculations** - Accurate Haversine distance calculations with appropriate tolerances
11. **Context-Aware Operations** - All operations properly handle context for cancellation and timeouts

## Usage

### Installation

To install the OpenStreetMap MCP server:

#### Option 1: Download Pre-built Binaries

The easiest way to get started is to download the latest release from our [releases page](https://github.com/NERVsystems/osmmcp/releases). Choose the appropriate binary for your operating system and architecture.

#### Option 2: Build from Source

If you prefer to build from source:
```

### Requirements

- Go 1.24 or higher
- OpenStreetMap API access (no API key required, but rate limits apply)

### Building the server

```bash
go build -o osmmcp ./cmd/osmmcp
```

### Running the server

```bash
./osmmcp
```

The server supports several command-line flags:

```bash
# Show version information
./osmmcp --version
# The version may differ if built with custom -ldflags

# Enable debug logging
./osmmcp --debug

# Generate a Claude Desktop Client configuration file
./osmmcp --generate-config /path/to/config.json

# Customize rate limits (requests per second and burst)
# Defaults: Nominatim=1 rps/burst 1, Overpass=2 per min/burst 2, OSRM=100 per min/burst 5
./osmmcp --nominatim-rps 1.0 --nominatim-burst 1
./osmmcp --overpass-rps 0.033 --overpass-burst 2
./osmmcp --osrm-rps 1.67 --osrm-burst 5

# Set custom User-Agent string
./osmmcp --user-agent "MyApp/1.0"
```

### Logging Configuration

The server uses structured logging via `slog` with the following configuration:

- Debug level: Enabled with `--debug` flag
- Default level: Info
- Format: Text-based with key-value pairs
- Output: Standard error (stderr)

Example log output:
```
2024-03-14T10:15:30.123Z INFO starting OpenStreetMap MCP server version=0.1.0 log_level=info user_agent=osm-mcp-server/0.1.0
2024-03-14T10:15:30.124Z DEBUG rate limiter initialized service=nominatim rps=1.0 burst=1
```

The server will start and listen for MCP requests on the standard input/output. You can use it with any MCP-compatible client or LLM integration.

### Using with Claude Desktop Client

This MCP server is designed to work with Claude Desktop Client. You can set it up easily with the following steps:

1. Build the server:
   ```bash
   go build -o osmmcp ./cmd/osmmcp
   ```

2. Generate or update the Claude Desktop Client configuration:
   ```bash
   ./osmmcp --generate-config ~/Library/Application\ Support/Anthropic/Claude/config.json
   ```

   This will add an `OSM` entry to the `mcpServers` configuration in Claude Desktop Client. The configuration system intelligently:
   
   - Creates the file if it doesn't exist
   - Preserves existing tools when updating the configuration
   - Uses absolute paths to ensure Claude can find the executable
   - Validates JSON output to prevent corruption

3. Restart Claude Desktop Client to load the updated configuration.

4. In a conversation with Claude, you can now use the OpenStreetMap MCP tools.

The configuration file will look similar to this:

```json
{
  "mcpServers": {
    "OSM": {
      "command": "/path/to/osmmcp",
      "args": []
    },
  }
}
```

### API Dependencies

The server relies on these external APIs:

- **Nominatim** - For geocoding operations
- **Overpass API** - For OpenStreetMap data queries
- **OSRM** - For routing calculations

No API keys are required as these are open public APIs, but the server follows usage policies including proper user agent identification and request rate limiting.

## Development

### Project Structure

- `cmd/osmmcp` - Main application entry point
- `pkg/server` - MCP server implementation
- `pkg/tools` - OpenStreetMap tool implementations and tool registry (25 tools)
- `pkg/osm` - OpenStreetMap API clients, rate limiting, polyline encoding, and utilities
- `pkg/geo` - Geographic types, bounding boxes, and Haversine distance calculations
- `pkg/core` - Core utilities including HTTP retry logic, validation, error handling, Overpass query builder, and OSRM service client
- `pkg/cache` - TTL-based caching layer for API responses (5-minute default)
- `pkg/monitoring` - Prometheus metrics, health checking, connection monitoring, and observability
- `pkg/tracing` - OpenTelemetry tracing support for distributed tracing and debugging
- `pkg/testutil` - Testing utilities and helpers
- `pkg/version` - Build metadata and version information

### Adding New Tools

To add a new tool:

1. Implement the tool functions in a new or existing file in `pkg/tools`
2. Add the tool definition to the registry in `pkg/tools/registry.go`

The registry-based design makes it easy to add new tools without modifying multiple files. All tool definitions are centralized in one place, making the codebase more maintainable.

### Testing

Run tests with:
```bash
go test ./...
```

The test suite includes:
- Unit tests for polyline encoding/decoding
- Server integration tests
- Geographic calculation tests
- Logging utility tests

## Acknowledgments

This implementation is based on two excellent sources:
- [jagan-shanmugam/open-streetmap-mcp](https://github.com/jagan-shanmugam/open-streetmap-mcp) - The original Python implementation
- [MCPLink OSM MCP Server](https://www.mcplink.ai/mcp/jagan-shanmugam/osm-mcp-server) - The MCPLink version with additional features

## Project History

Originally created by [@pdfinn](https://github.com/pdfinn).  
All core functionality and initial versions developed prior to organisational transfer.

## Documentation

- [GoDoc](https://pkg.go.dev/github.com/NERVsystems/osmmcp) - Full API documentation
- [Geocoding Tools Guide](pkg/tools/docs/geocoding.md) - Detailed guide for geocoding features and best practices
- [AI Prompts for Geocoding](pkg/tools/docs/ai_prompts.md) - System prompts and examples for AI integration with geocoding tools

## License

MIT License 
