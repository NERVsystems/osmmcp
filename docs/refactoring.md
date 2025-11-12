# OpenStreetMap MCP Refactoring Plan

## Issues Identified

1. **Code Duplication**:
   - Multiple implementations of HTTP clients with retry logic
   - Repetitive validation for coordinates and radius parameters
   - Duplicated error handling and response formatting
   - Multiple implementations of Overpass query construction
   - Repetitive tool definition boilerplate

2. **Inconsistent Error Handling**:
   - Some tools return detailed error responses, others use simple messages
   - Inconsistent error codes and guidance
   - No standardized approach to retry failures

3. **Maintainability Challenges**:
   - Tight coupling between HTTP client logic and tool implementations
   - No separation between OSM API interaction and business logic
   - Repetitive transformation of response data

## Refactoring Approach

### 1. Core Package Enhancements (`pkg/core`)

We've created a new `pkg/core` package with modular components that encapsulate common functionality:

- **HTTP Utilities** (`http.go`):
  - Generic retry mechanisms with exponential backoff
  - Unified client configuration and error handling
  - Request factory pattern to allow retrying requests with bodies

- **Validation Utilities** (`validation.go`):
  - Standardized coordinate validation
  - Parameter parsing with logging
  - Radius validation and bounding

- **Error Handling** (`errors.go`):
  - Consistent error format with codes, messages, and guidance
  - Standard error types for common scenarios
  - Contextual error creation for different API services

- **Scoring Utilities** (`scoring.go`):
  - Generic weighted scoring functions
  - Normalization utilities

- **Overpass Query Builder** (`overpass.go`):
  - Fluent interface for building Overpass queries
  - Type-safe parameter handling
  - Proper escaping and formatting

- **OSRM Route Service** (`osrm.go`):
  - Unified interface for various routing operations
  - Response parsing and transformation
  - Proper caching implementation

- **Tool Factory** (`tool_factory.go`):
  - Simplified creation of common tool patterns
  - Standardized parameter definitions
  - Consistent tool formatting

### 2. Refactored Tools

We've applied these core utilities to refactor several key tool implementations:

1. **Routing Tools**:
   - `HandleRouteFetch` - Now uses the OSRM core utilities
   - `HandleGetRouteDirections` - Simplified with unified retry logic and error handling

2. **Geocoding Tools**:
   - `HandleGeocodeAddress` - Improved caching and error handling
   - `HandleReverseGeocode` - Now uses standardized HTTP retry logic

3. **Exploration Tools**:
   - `HandleExploreArea` - Uses the Overpass builder for cleaner query construction

4. **Parking Tools**:
   - `HandleFindParkingFacilities` - Complete refactoring with new Overpass query builder

### 3. Benefits Achieved

The refactoring has delivered several key benefits:

1. **Reduced Code Duplication**:
   - HTTP client logic is now defined once
   - Validation is centralized and consistent
   - Query building follows a standard pattern

2. **Improved Error Handling**:
   - Consistent error format across all tools
   - Better user guidance for error resolution
   - Proper context propagation

3. **Enhanced Maintainability**:
   - Clear separation of concerns
   - Easier to understand and modify
   - Single responsibility for each module

4. **Better Performance**:
   - Proper caching implementation
   - Optimized retry handling
   - Reduced memory allocations

5. **Increased Robustness**:
   - Better handling of edge cases
   - More graceful failure modes
   - Improved logging and diagnostics

## Implementation Examples

### Example 1: Refactored Parking Facilities Search

```go
// HandleFindParkingFacilities implements finding parking facilities functionality
func HandleFindParkingFacilities(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "find_parking_facilities")

	// Parse and validate input parameters
	lat, lon, radius, err := core.ParseCoordsAndRadiusWithLog(req, logger, "", "", "", 1000, 5000)
	if err != nil {
		return core.NewError(core.ErrInvalidInput, err.Error()).ToMCPResult(), nil
	}

	// Parse additional parameters with defaults
	facilityType := mcp.ParseString(req, "type", "")
	includePrivate := mcp.ParseBoolean(req, "include_private", false)
	limit := int(mcp.ParseFloat64(req, "limit", 10))

	// Validate and cap limit
	if limit <= 0 {
		limit = 10 // Default limit
	}
	if limit > 50 {
		limit = 50 // Max limit
	}

	// Build Overpass query using the fluent builder
	queryBuilder := core.NewOverpassBuilder().
		WithTimeout(25).
		WithCenter(lat, lon, radius).
		WithTag("amenity", "parking")

	// Add additional type filter if specified
	if facilityType != "" {
		queryBuilder.WithTag("parking", facilityType)
	}

	// Execute the query
	results, err := fetchParkingFacilities(ctx, queryBuilder.Build())
	if err != nil {
		logger.Error("failed to fetch parking facilities", "error", err)
		return err.(*core.MCPError).ToMCPResult(), nil
	}

	// Process results
	facilities, err := processParkingFacilities(results, lat, lon, includePrivate, facilityType)
	if err != nil {
		logger.Error("failed to process parking facilities", "error", err)
		return core.NewError(core.ErrParseError, "Failed to process parking data").ToMCPResult(), nil
	}

	// Sort facilities by distance (closest first)
	sort.Slice(facilities, func(i, j int) bool {
		return facilities[i].Distance < facilities[j].Distance
	})

	// Limit results
	if len(facilities) > limit {
		facilities = facilities[:limit]
	}

	// Create output
	output := struct {
		Facilities []ParkingArea `json:"facilities"`
	}{
		Facilities: facilities,
	}

	// Return result
	resultBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("failed to marshal result", "error", err)
		return core.NewError(core.ErrInternalError, "Failed to generate result").ToMCPResult(), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}
```

### Example 2: Refactored Route Directions

```go
// HandleGetRouteDirections gets directions between two points
func HandleGetRouteDirections(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "get_route_directions")

	// Parse and validate start coordinates
	startLat, startLon, err := core.ParseCoordsWithLog(req, logger, "start_", "")
	if err != nil {
		return core.NewError(core.ErrInvalidInput, fmt.Sprintf("Invalid start coordinates: %s", err)).ToMCPResult(), nil
	}

	// Parse and validate end coordinates
	endLat, endLon, err := core.ParseCoordsWithLog(req, logger, "end_", "")
	if err != nil {
		return core.NewError(core.ErrInvalidInput, fmt.Sprintf("Invalid end coordinates: %s", err)).ToMCPResult(), nil
	}

	// Parse transportation mode
	mode := mcp.ParseString(req, "mode", "car")
	profile := mapModeToProfile(mode)

	// Set up coordinates for the OSRM request
	coordinates := [][]float64{
		{startLon, startLat},
		{endLon, endLat},
	}

	// Set up OSRM options
	options := core.OSRMOptions{
		BaseURL:     osm.OSRMBaseURL,
		Profile:     profile,
		Overview:    "full",       // Include full geometry
		Steps:       true,         // Include turn-by-turn instructions
		Annotations: nil,          // No additional annotations
		Geometries:  "polyline",   // Use polyline format
		Client:      osm.GetClient(ctx),
		RetryOptions: core.RetryOptions{
			MaxAttempts:  3,
			InitialDelay: 500 * time.Millisecond,
			MaxDelay:     5 * time.Second,
			Multiplier:   2.0,
		},
	}

	// Execute the route request
	route, err := core.GetRoute(ctx, coordinates, options)
	if err != nil {
		logger.Error("failed to get route", "error", err)
		if mcpErr, ok := err.(*core.MCPError); ok {
			return mcpErr.ToMCPResult(), nil
		}
		return core.ServiceError("OSRM", http.StatusServiceUnavailable, 
			"Failed to communicate with routing service").ToMCPResult(), nil
	}

	// Process results and return
	// ...
}
```

## Next Steps

1. **Complete Tool Refactoring**:
   - Apply similar patterns to remaining tools
   - Standardize all tool definitions

2. **Testing Improvements**:
   - Update tests to use mock responses
   - Add integration tests for core utilities
   - Increase test coverage

3. **Documentation**:
   - Document core utilities
   - Update tool documentation
   - Add usage examples

4. **Performance Optimization**:
   - Optimize validation for hot paths
   - Improve cache hit rates
   - Consider batch processing for certain operations

5. **Code Generation**:
   - Investigate generating tool definitions from templates
   - Automate documentation generation 