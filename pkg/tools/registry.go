// Package tools provides the OpenStreetMap MCP tools implementations.
package tools

import (
	"context"
	"log/slog"

	"github.com/NERVsystems/osmmcp/pkg/core"
	"github.com/NERVsystems/osmmcp/pkg/tools/prompts"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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
			Description: "Convert a text address to geographic coordinates. Parameters: address (string), region (optional string)",
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
			Description: "Query OpenStreetMap data within a bounding box. Parameters: bbox (object with minLat, minLon, maxLat, maxLon), tags (object)",
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
		mcpServer.AddTool(def.Tool, def.Handler)
	}
}

// RegisterPrompts registers all prompts with the MCP server.
func (r *Registry) RegisterPrompts(mcpServer *server.MCPServer) {
	r.logger.Info("registering geocoding prompts")
	prompts.RegisterGeocodingPrompts(mcpServer)
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
