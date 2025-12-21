// Package tools provides the OpenStreetMap MCP tools implementations.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/NERVsystems/osmmcp/pkg/core"
	"github.com/NERVsystems/osmmcp/pkg/tools/prompts"
	"github.com/NERVsystems/osmmcp/pkg/tracing"
)

// Registry contains all tool definitions and handlers
type Registry struct {
	logger  *slog.Logger
	factory *core.ToolFactory
}

// NewRegistry creates a new tool registry
func NewRegistry(logger *slog.Logger) *Registry {
	return &Registry{
		logger:  logger,
		factory: core.NewToolFactory(),
	}
}

// ToolDefinition represents an OpenStreetMap MCP tool definition.
type ToolDefinition struct {
	Name        string
	Description string
	Tool        mcp.Tool
	Handler     func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

// GetToolDefinitions returns the list of all available tools.
func (r *Registry) GetToolDefinitions() []ToolDefinition {
	defs := []ToolDefinition{
		// Version and capability tools
		{
			Name:        "get_version",
			Description: "Get the version information for this OpenStreetMap MCP",
			Tool:        GetVersionTool(),
			Handler:     HandleGetVersion,
		},

		// Geocoding tools
		{
			Name:        "geocode_address",
			Description: "Convert address, place name, or military coordinates (MGRS, UTM, DMS) to lat/lon. Essential for tactical coordinate handling.",
			Tool:        GeocodeAddressTool(),
			Handler:     HandleGeocodeAddress,
		},
		{
			Name:        "reverse_geocode",
			Description: "Convert geographic coordinates to a street address. Parameters: latitude (number), longitude (number)",
			Tool:        ReverseGeocodeTool(),
			Handler:     HandleReverseGeocode,
		},

		// Visualization tools
		{
			Name:        "get_map_image",
			Description: "Get a map image of a specified location. Parameters: latitude (number), longitude (number), zoom (number, 1-19)",
			Tool:        GetMapImageTool(),
			Handler:     HandleGetMapImage,
		},

		// Route and direction tools
		{
			Name:        "route_fetch",
			Description: "Fetch a route between two points. Parameters: start (object with latitude/longitude), end (object with latitude/longitude), mode (string: car, bike, foot)",
			Tool:        RouteFetchTool(),
			Handler:     HandleRouteFetch,
		},
		{
			Name:        "get_route_directions",
			Description: "Get turn-by-turn directions between two points. Parameters: start_lat (number), start_lon (number), end_lat (number), end_lon (number), mode (string: car, bike, foot)",
			Tool:        GetRouteDirectionsTool(),
			Handler:     HandleGetRouteDirections,
		},
		{
			Name:        "suggest_meeting_point",
			Description: "Suggest a meeting point for multiple locations. Parameters: locations (array of latitude/longitude objects), radius (number), category (string)",
			Tool:        SuggestMeetingPointTool(),
			Handler:     HandleSuggestMeetingPoint,
		},
		{
			Name:        "route_sample",
			Description: "Sample points along a route at regular intervals. Parameters: polyline (string), interval (number in meters)",
			Tool:        RouteSampleTool(),
			Handler:     HandleRouteSample,
		},
		{
			Name:        "analyze_commute",
			Description: "Analyze commute options between home and work locations. Parameters: home (object), work (object)",
			Tool:        AnalyzeCommuteTool(),
			Handler:     HandleAnalyzeCommute,
		},

		// POI and exploration tools
		{
			Name:        "find_nearby_places",
			Description: "Find places near a location. Parameters: latitude (number), longitude (number), radius (number in meters), category (string), limit (number)",
			Tool:        FindNearbyPlacesTool(),
			Handler:     HandleFindNearbyPlaces,
		},
		{
			Name:        "explore_area",
			Description: "Explore an area and get key features. Parameters: latitude (number), longitude (number), radius (number in meters)",
			Tool:        ExploreAreaTool(),
			Handler:     HandleExploreArea,
		},
		{
			Name:        "find_parking_facilities",
			Description: "Find parking facilities near a location. Parameters: latitude (number), longitude (number), radius (number in meters), type (string), include_private (boolean), limit (number)",
			Tool:        FindParkingAreasTool(),
			Handler:     HandleFindParkingFacilities,
		},
		{
			Name:        "find_charging_stations",
			Description: "Find EV charging stations near a location. Parameters: latitude (number), longitude (number), radius (number in meters), limit (number)",
			Tool:        FindChargingStationsTool(),
			Handler:     HandleFindChargingStations,
		},
		{
			Name:        "find_schools_nearby",
			Description: "Find schools near a location. Parameters: latitude (number), longitude (number), radius (number in meters), limit (number)",
			Tool:        FindSchoolsNearbyTool(),
			Handler:     HandleFindSchoolsNearby,
		},
		{
			Name:        "analyze_neighborhood",
			Description: "Analyze a neighborhood for livability. Parameters: latitude (number), longitude (number), name (string)",
			Tool:        AnalyzeNeighborhoodTool(),
			Handler:     HandleAnalyzeNeighborhood,
		},

		// Geo utility tools
		{
			Name:        "geo_distance",
			Description: "Calculate distance between two points. Parameters: from (object with latitude/longitude), to (object with latitude/longitude)",
			Tool:        GeoDistanceTool(),
			Handler:     HandleGeoDistance,
		},
		{
			Name:        "bbox_from_points",
			Description: "Create a bounding box from multiple points. Parameters: points (array of latitude/longitude objects)",
			Tool:        BBoxFromPointsTool(),
			Handler:     HandleBBoxFromPoints,
		},
		{
			Name:        "centroid_points",
			Description: "Calculate the centroid of multiple points. Parameters: points (array of latitude/longitude objects)",
			Tool:        CentroidPointsTool(),
			Handler:     HandleCentroidPoints,
		},

		// Polyline utilities
		{
			Name:        "polyline_decode",
			Description: "Decode a polyline string into a series of coordinates. Parameters: polyline (string)",
			Tool:        PolylineDecodeTool(),
			Handler:     HandlePolylineDecode,
		},
		{
			Name:        "polyline_encode",
			Description: "Encode a series of coordinates into a polyline string. Parameters: points (array of latitude/longitude objects)",
			Tool:        PolylineEncodeTool(),
			Handler:     HandlePolylineEncode,
		},
		{
			Name:        "enrich_emissions",
			Description: "Enrich transportation modes with emissions data. Parameters: options (array of mode objects)",
			Tool:        EnrichEmissionsTool(),
			Handler:     HandleEnrichEmissions,
		},

		// OSM query tools
		{
			Name:        "osm_query_bbox",
			Description: "Query OpenStreetMap data within a bounding box with tag filters. Parameters: bbox (object with minLat, minLon, maxLat, maxLon), tags (object with key-value string pairs, use '*' for wildcards). Example: bbox: {\"minLat\": 37.77, \"minLon\": -122.42, \"maxLat\": 37.78, \"maxLon\": -122.41}, tags: {\"amenity\": \"restaurant\", \"cuisine\": \"*\"}",
			Tool:        OSMQueryBBoxTool(),
			Handler:     HandleOSMQueryBBox,
		},
		{
			Name:        "filter_tags",
			Description: "Filter OSM elements by tags. Parameters: elements (array), tags (object of string arrays)",
			Tool:        FilterTagsTool(),
			Handler:     HandleFilterTags,
		},
		{
			Name:        "sort_by_distance",
			Description: "Sort OSM elements by distance from a reference point. Parameters: elements (array), ref (object with latitude/longitude)",
			Tool:        SortByDistanceTool(),
			Handler:     HandleSortByDistance,
		},

		// Tile cache management
		{
			Name:        "tile_cache",
			Description: "Manage and access cached map tiles. Parameters: action (string: list, get, stats), x (number), y (number), zoom (number)",
			Tool:        GetTileCacheTool(),
			Handler:     HandleTileCache,
		},
	}

	return defs
}

// RegisterTools registers all tools with the MCP server.
func (r *Registry) RegisterTools(mcpServer *server.MCPServer) {
	for _, def := range r.GetToolDefinitions() {
		r.logger.Info("registering tool", "name", def.Name)
		// Wrap handler with tracing
		tracedHandler := r.wrapWithTracing(def.Name, def.Handler)
		mcpServer.AddTool(def.Tool, tracedHandler)
	}
}

// wrapWithTracing wraps a tool handler with OpenTelemetry tracing
func (r *Registry) wrapWithTracing(toolName string, handler func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Start span
		spanName := fmt.Sprintf("mcp.tool.%s", toolName)
		ctx, span := tracing.StartSpan(ctx, spanName,
			trace.WithAttributes(
				attribute.String(tracing.AttrMCPToolName, toolName),
			),
		)
		defer span.End()

		// Record start time
		startTime := time.Now()

		// Execute handler
		result, err := handler(ctx, req)

		// Calculate duration
		duration := time.Since(startTime)
		durationMs := duration.Milliseconds()

		// Determine status
		status := tracing.StatusSuccess
		if err != nil {
			status = tracing.StatusError
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}

		// Calculate result size
		resultSize := 0
		if result != nil && result.Content != nil {
			if data, marshalErr := json.Marshal(result.Content); marshalErr == nil {
				resultSize = len(data)
			}
		}

		// Set final attributes
		span.SetAttributes(
			attribute.String(tracing.AttrMCPToolStatus, status),
			attribute.Int64(tracing.AttrMCPToolDuration, durationMs),
			attribute.Int(tracing.AttrMCPResultSize, resultSize),
		)

		// Log for debugging
		r.logger.Debug("tool execution traced",
			"tool", toolName,
			"duration_ms", durationMs,
			"status", status,
			"result_size", resultSize,
		)

		return result, err
	}
}

// RegisterPrompts registers all prompts with the MCP server.
func (r *Registry) RegisterPrompts(mcpServer *server.MCPServer) {
	r.logger.Info("registering geocoding prompts")
	prompts.RegisterGeocodingPrompts(mcpServer)
}

// GetToolNames returns a list of all tool names.
func (r *Registry) GetToolNames() []string {
	defs := r.GetToolDefinitions()
	names := make([]string, len(defs))
	for i, def := range defs {
		names[i] = def.Name
	}
	return names
}

// RegisterAll registers all tools and prompts with the MCP server.
func (r *Registry) RegisterAll(mcpServer *server.MCPServer) {
	// Create a context with the registry for capabilities lookup
	registryCtx := context.WithValue(context.Background(), "registry", r)
	mcpServer.WithContext(registryCtx, nil)

	// Register all tools and prompts
	r.RegisterTools(mcpServer)
	r.RegisterPrompts(mcpServer)
}
