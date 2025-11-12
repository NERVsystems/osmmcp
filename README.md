# OpenStreetMap MCP Server (Go Implementation)

This is a Go implementation of the OpenStreetMap MCP server that enhances LLM capabilities with location-based services and geospatial data.

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
| `geocode_address` | Convert an address or place name to geographic coordinates | `{"address": "1600 Pennsylvania Ave, Washington DC"}` |
| `reverse_geocode` | Convert geographic coordinates to a human-readable address | `{"latitude": 38.8977, "longitude": -77.0365}` |
| `find_nearby_places` | Find points of interest near a specific location | `{"latitude": 37.7749, "longitude": -122.4194, "radius": 1000, "category": "restaurant", "limit": 5}` |
| `search_category` | Search for places by category within a rectangular area | `{"category": "cafe", "north_lat": 37.78, "south_lat": 37.77, "east_lon": -122.41, "west_lon": -122.42, "limit": 10}` |
| `get_route` | Calculate a route between two locations with distance and duration | `{"start_lat": 37.7749, "start_lon": -122.4194, "end_lat": 37.8043, "end_lon": -122.2711, "mode": "car"}` |
| `get_route_directions` | Get detailed turn-by-turn directions for a route between locations | `{"start_lat": 37.7749, "start_lon": -122.4194, "end_lat": 37.8043, "end_lon": -122.2711, "mode": "car"}` |
| `suggest_meeting_point` | Suggest an optimal meeting point for multiple people | `{"locations": [{"latitude": 37.7749, "longitude": -122.4194}, {"latitude": 37.8043, "longitude": -122.2711}], "category": "cafe", "limit": 3}` |
| `explore_area` | Explore an area and get comprehensive information about it | `{"latitude": 37.7749, "longitude": -122.4194, "radius": 1000}` |
| `find_charging_stations` | Find electric vehicle charging stations near a location | `{"latitude": 37.7749, "longitude": -122.4194, "radius": 5000, "limit": 10}` |
| `find_route_charging_stations` | Find electric vehicle charging stations along a route | `{"start_lat": 37.7749, "start_lon": -122.4194, "end_lat": 37.8043, "end_lon": -122.2711, "range": 300, "buffer": 5000}` |
| `analyze_commute` | Analyze transportation options between home and work locations | `{"home_latitude": 37.7749, "home_longitude": -122.4194, "work_latitude": 37.8043, "work_longitude": -122.2711, "transport_modes": ["car", "cycling", "walking"]}` |
| `analyze_neighborhood` | Evaluate neighborhood livability for real estate and relocation decisions | `{"latitude": 37.7749, "longitude": -122.4194, "radius": 1000, "include_price_data": true}` |
| `find_schools_nearby` | Find educational institutions near a specific location | `{"latitude": 37.7749, "longitude": -122.4194, "radius": 2000, "school_type": "elementary", "limit": 5}` |
| `find_parking_facilities` | Find parking facilities near a specific location | `{"latitude": 37.7749, "longitude": -122.4194, "radius": 1000, "type": "surface", "include_private": false, "limit": 5}` |

## Code Architecture and Design

The code follows software engineering best practices:

1. **High Cohesion, Low Coupling** - Each package has a clear, focused responsibility
2. **Separation of Concerns** - Tools, server logic, and utilities are cleanly separated
3. **DRY (Don't Repeat Yourself)** - Common utilities are extracted into the `pkg/osm` package
4. **Security First** - HTTP clients are properly configured with timeouts and connection limits
5. **Structured Logging** - All logging is done via `slog` with consistent levels and formats:
   - Debug: Developer detail, verbose or diagnostic messages
   - Info: Routine operational messages
   - Warn: Unexpected conditions that don't necessarily halt execution
   - Error: Critical problems, potential or actual failures
6. **SOLID Principles** - Particularly Single Responsibility and Interface Segregation
7. **Registry Pattern** - All tools are defined in a central registry for improved maintainability
8. **Google Polyline5 Format** - Standardized polyline encoding/decoding using Google's Polyline5 format
9. **Precise Geospatial Calculations** - Accurate Haversine distance calculations with appropriate tolerances
10. **Context-Aware Operations** - All operations properly handle context for cancellation and timeouts
11. **Response Caching** - TTL-based cache (5-minute default) with automatic cleanup to reduce API calls
12. **Rate Limiting** - Complies with OpenStreetMap API usage policies:
    - Nominatim: 1 request/second (geocoding policy)
    - Overpass: 2 requests/minute (API guidelines)
    - OSRM: 100 requests/minute (routing service)
13. **Connection Pooling** - Efficient HTTP client with 100 idle connections, 10 per host

## Usage

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
- `pkg/tools` - OpenStreetMap tool implementations and tool registry (14 tools)
- `pkg/osm` - OpenStreetMap API clients, rate limiting, polyline encoding, and utilities
- `pkg/geo` - Geographic types, bounding boxes, and Haversine distance calculations
- `pkg/cache` - TTL-based caching layer for API responses (5-minute default)
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

## License

MIT License 