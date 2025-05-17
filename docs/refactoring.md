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

The `pkg/core` package will be expanded to include:

#### 1.1 HTTP Utilities (`http.go`) ✅
- Generic retry mechanism with exponential backoff
- Request factory pattern for POST requests
- Standardized timeout and error handling

#### 1.2 Validation Utilities (`validation.go`) ✅
- Coordinate validation (latitude/longitude)
- Radius validation with min/max bounds
- Parameter extraction helpers with logging

#### 1.3 Error Handling (`errors.go`) ✅
- Standardized error response format
- Error codes with guidance messages
- Service-specific error handling (Nominatim, Overpass, OSRM)

#### 1.4 Scoring Utilities (`scoring.go`) ✅
- Weighted scoring functions for metrics
- Threshold-based categorization
- Distance-biased scoring

#### 1.5 Overpass Query Builder (`overpass.go`) ✅
- Fluent interface for building queries
- Support for various query patterns (bbox, center+radius)
- Tag filtering utilities

#### 1.6 OSRM Route Service (`osrm.go`) ✅
- Unified interface for routing operations
- Caching support
- Configuration options

### 2. Implementation Plan

#### 2.1 Refactor Common Patterns
1. Replace direct HTTP calls with core.WithRetry/WithRetryFactory
2. Replace manual validation with core.ValidateCoords/ValidateRadius
3. Replace string building with OverpassBuilder
4. Standardize error responses using core.NewError/ServiceError

#### 2.2 Create Service Abstractions
1. Create OSMService for Overpass API interactions
2. Create GeocodingService for Nominatim interactions
3. Create RoutingService for OSRM interactions

#### 2.3 Implement Tool Factory Pattern
1. Create a tool factory to reduce boilerplate
2. Standardize parameter parsing and validation
3. Implement common error handling patterns

### 3. Example Implementations

#### Example 1: Refactored HandleRouteFetch
```go
// HandleRouteFetch implements route fetching functionality
func HandleRouteFetch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "route_fetch")

	// Parse input
	var input RouteFetchInput
	inputJSON, err := json.Marshal(req.Params.Arguments)
	if err != nil {
		logger.Error("failed to marshal input", "error", err)
		return core.NewError(core.ErrInvalidInput, "Invalid input format").ToMCPResult(), nil
	}

	if err := json.Unmarshal(inputJSON, &input); err != nil {
		logger.Error("failed to parse input", "error", err)
		return core.NewError(core.ErrInvalidInput, "Invalid input format").ToMCPResult(), nil
	}

	// Validate input coordinates using core validation
	if err := core.ValidateCoords(input.Start.Latitude, input.Start.Longitude); err != nil {
		logger.Error("invalid 'start' coordinates", "error", err)
		return core.NewError(core.ErrInvalidLatitude, fmt.Sprintf("Invalid start coordinates: %s", err)).ToMCPResult(), nil
	}

	if err := core.ValidateCoords(input.End.Latitude, input.End.Longitude); err != nil {
		logger.Error("invalid 'end' coordinates", "error", err)
		return core.NewError(core.ErrInvalidLongitude, fmt.Sprintf("Invalid end coordinates: %s", err)).ToMCPResult(), nil
	}

	// Validate mode
	profile := convertModeToProfile(input.Mode)
	if profile == "" {
		logger.Error("invalid mode", "mode", input.Mode)
		errResult := core.NewError(core.ErrInvalidParameter, fmt.Sprintf("Invalid mode: %s", input.Mode))
		errResult = errResult.WithGuidance("Use 'car', 'bike', or 'foot'")
		return errResult.ToMCPResult(), nil
	}

	// Setup the coordinates (longitude first, latitude second, as expected by OSRM)
	startCoord := []float64{input.Start.Longitude, input.Start.Latitude}
	endCoord := []float64{input.End.Longitude, input.End.Latitude}

	// Use the simpler core.GetSimpleRoute helper
	route, err := core.GetSimpleRoute(ctx, startCoord, endCoord, profile)
	if err != nil {
		logger.Error("failed to get route", "error", err)
		if mcpErr, ok := err.(*core.MCPError); ok {
			return mcpErr.ToMCPResult(), nil
		}
		// Fallback for other errors
		return core.ServiceError("OSRM", http.StatusServiceUnavailable, "Failed to get route").
			WithGuidance("Try again later or check if the locations are reachable").
			ToMCPResult(), nil
	}

	// Create output from route result
	output := RouteFetchOutput{
		Polyline: route.Polyline,
		Distance: route.Distance,
		Duration: route.Duration,
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

#### Example 2: Refactored HandleFindParkingFacilities
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

### 4. Migration Strategy

1. **Phase 1**: Enhance core utilities (completed)
2. **Phase 2**: Refactor one tool from each category
   - Geographic tool (HandleGeoDistance)
   - Routing tool (HandleRouteFetch) ✅
   - Search tool (HandleFindParkingFacilities) ✅
3. **Phase 3**: Gradually migrate all other tools
4. **Phase 4**: Add comprehensive testing
5. **Phase 5**: Documentation updates

## Expected Benefits

1. **Reduced Code Duplication**: 60-70% reduction in duplicated patterns
2. **Improved Error Handling**: Consistent, user-friendly error messages
3. **Better Maintainability**: Separation of concerns, modular design
4. **Enhanced Performance**: Proper caching, efficient retry handling
5. **Lower Cognitive Load**: Developers can focus on tool-specific logic 