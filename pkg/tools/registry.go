// Package tools provides the OpenStreetMap MCP tools implementations.
package tools

import (
	"context"
	"log/slog"

	"github.com/NERVsystems/osmmcp/pkg/tools/prompts"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Registry holds all MCP tool registrations for the OpenStreetMap service.
type Registry struct {
	logger *slog.Logger
}

// NewRegistry creates a new MCP tool registry.
func NewRegistry(logger *slog.Logger) *Registry {
	return &Registry{
		logger: logger,
	}
}

// ToolDefinition represents an OpenStreetMap MCP tool definition.
type ToolDefinition struct {
	Name        string
	Description string
	Tool        mcp.Tool
	Handler     func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

// GetToolDefinitions returns all OpenStreetMap MCP tool definitions.
func (r *Registry) GetToolDefinitions() []ToolDefinition {
	return []ToolDefinition{
		// Bbox Tools
		{
			Name:        "bbox_from_points",
			Description: "Create a bounding box that encompasses all given geographic coordinates",
			Tool:        BBoxFromPointsTool(),
			Handler:     HandleBBoxFromPoints,
		},

		// Centroid Tools
		{
			Name:        "centroid_points",
			Description: "Calculate the geographic centroid (mean center) of a set of coordinates",
			Tool:        CentroidPointsTool(),
			Handler:     HandleCentroidPoints,
		},

		// Emissions Tools
		{
			Name:        "enrich_emissions",
			Description: "Enrich route options with CO2 emissions, calorie burn, and cost estimates",
			Tool:        EnrichEmissionsTool(),
			Handler:     HandleEnrichEmissions,
		},

		// EV Tools
		{
			Name:        "find_charging_stations",
			Description: "Find electric vehicle charging stations near a location",
			Tool:        FindChargingStationsTool(),
			Handler:     HandleFindChargingStations,
		},
		{
			Name:        "find_route_charging_stations",
			Description: "Find electric vehicle charging stations along a route",
			Tool:        FindRouteChargingStationsTool(),
			Handler:     HandleFindRouteChargingStations,
		},

		// Exploration Tools
		{
			Name:        "explore_area",
			Description: "Explore an area and get comprehensive information about it",
			Tool:        ExploreAreaTool(),
			Handler:     HandleExploreArea,
		},

		// Filter Tools
		{
			Name:        "filter_tags",
			Description: "Filter OSM elements by specified tags",
			Tool:        FilterTagsTool(),
			Handler:     HandleFilterTags,
		},

		// Geocoding Tools
		{
			Name:        "geocode_address",
			Description: "Convert an address or place name to geographic coordinates",
			Tool:        GeocodeAddressTool(),
			Handler:     HandleGeocodeAddress,
		},
		{
			Name:        "geo_distance",
			Description: "Calculate the distance between two geographic coordinates using the Haversine formula",
			Tool:        GeoDistanceTool(),
			Handler:     HandleGeoDistance,
		},
		{
			Name:        "reverse_geocode",
			Description: "Convert geographic coordinates to a human-readable address",
			Tool:        ReverseGeocodeTool(),
			Handler:     HandleReverseGeocode,
		},

		// Neighborhood Analysis Tools
		{
			Name:        "analyze_neighborhood",
			Description: "Evaluate neighborhood livability for real estate and relocation decisions",
			Tool:        AnalyzeNeighborhoodTool(),
			Handler:     HandleAnalyzeNeighborhood,
		},

		// OSM Query Tools
		{
			Name:        "osm_query_bbox",
			Description: "Query OpenStreetMap data within a bounding box with tag filters",
			Tool:        OSMQueryBBoxTool(),
			Handler:     HandleOSMQueryBBox,
		},

		// Parking Tools
		{
			Name:        "find_parking_facilities",
			Description: "Find parking facilities near a specific location",
			Tool:        FindParkingAreasTool(),
			Handler:     HandleFindParkingFacilities,
		},

		// Place Search Tools
		{
			Name:        "find_nearby_places",
			Description: "Find points of interest near a specific location",
			Tool:        FindNearbyPlacesTool(),
			Handler:     HandleFindNearbyPlaces,
		},
		{
			Name:        "search_category",
			Description: "Search for places by category within a rectangular area",
			Tool:        SearchCategoryTool(),
			Handler:     HandleSearchCategory,
		},

		// Polyline Tools
		{
			Name:        "polyline_decode",
			Description: "Decode an encoded polyline string into a series of geographic coordinates",
			Tool:        PolylineDecodeTool(),
			Handler:     HandlePolylineDecode,
		},
		{
			Name:        "polyline_encode",
			Description: "Encode a series of geographic coordinates into a polyline string",
			Tool:        PolylineEncodeTool(),
			Handler:     HandlePolylineEncode,
		},

		// Routing Tools
		{
			Name:        "get_route_directions",
			Description: "Get directions for a route between two locations",
			Tool:        GetRouteDirectionsTool(),
			Handler:     HandleGetRouteDirections,
		},
		{
			Name:        "route_fetch",
			Description: "Fetch a route between two points using OSRM routing service",
			Tool:        RouteFetchTool(),
			Handler:     HandleRouteFetch,
		},
		{
			Name:        "route_sample",
			Description: "Sample points along a route at specified intervals",
			Tool:        RouteSampleTool(),
			Handler:     HandleRouteSample,
		},
		{
			Name:        "suggest_meeting_point",
			Description: "Suggest an optimal meeting point for multiple people",
			Tool:        SuggestMeetingPointTool(),
			Handler:     HandleSuggestMeetingPoint,
		},

		// School Tools
		{
			Name:        "find_schools_nearby",
			Description: "Find educational institutions near a specific location",
			Tool:        FindSchoolsNearbyTool(),
			Handler:     HandleFindSchoolsNearby,
		},

		// Sorting Tools
		{
			Name:        "sort_by_distance",
			Description: "Sort OSM elements by distance from a reference point",
			Tool:        SortByDistanceTool(),
			Handler:     HandleSortByDistance,
		},

		// Transportation Tools
		{
			Name:        "analyze_commute",
			Description: "Analyze transportation options between home and work locations",
			Tool:        AnalyzeCommuteTool(),
			Handler:     HandleAnalyzeCommute,
		},
	}
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
	r.RegisterTools(mcpServer)
	r.RegisterPrompts(mcpServer)
}
