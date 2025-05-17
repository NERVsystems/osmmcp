// Package core provides shared utilities for the OpenStreetMap MCP tools.
package core

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// ToolFactory provides a simplified way to create new tool definitions
// with standardized parameters
type ToolFactory struct {
	// Factory configuration options could be added here
}

// NewToolFactory creates a new tool factory
func NewToolFactory() *ToolFactory {
	return &ToolFactory{}
}

// CreateBasicTool creates a new tool with the specified name and description
func (f *ToolFactory) CreateBasicTool(name, description string) mcp.Tool {
	return mcp.NewTool(name, mcp.WithDescription(description))
}

// CreateLocationTool creates a tool with coordinates and radius parameters
func (f *ToolFactory) CreateLocationTool(name, description string, defaultRadius, maxRadius float64) mcp.Tool {
	radiusDesc := "Search radius in meters"
	if maxRadius > 0 {
		radiusDesc += fmt.Sprintf(" (max %.0f)", maxRadius)
	}

	return mcp.NewTool(name,
		mcp.WithDescription(description),
		mcp.WithNumber("latitude",
			mcp.Required(),
			mcp.Description("The latitude coordinate of the center point"),
		),
		mcp.WithNumber("longitude",
			mcp.Required(),
			mcp.Description("The longitude coordinate of the center point"),
		),
		mcp.WithNumber("radius",
			mcp.Description(radiusDesc),
			mcp.DefaultNumber(defaultRadius),
		),
	)
}

// CreateSearchTool creates a tool with coordinates, radius, category and limit parameters
func (f *ToolFactory) CreateSearchTool(name, description string, defaultRadius, maxRadius float64, defaultLimit, maxLimit int) mcp.Tool {
	radiusDesc := "Search radius in meters"
	if maxRadius > 0 {
		radiusDesc += fmt.Sprintf(" (max %.0f)", maxRadius)
	}

	limitDesc := "Maximum number of results to return"
	if maxLimit > 0 {
		limitDesc += fmt.Sprintf(" (max %d)", maxLimit)
	}

	return mcp.NewTool(name,
		mcp.WithDescription(description),
		mcp.WithNumber("latitude",
			mcp.Required(),
			mcp.Description("The latitude coordinate of the center point"),
		),
		mcp.WithNumber("longitude",
			mcp.Required(),
			mcp.Description("The longitude coordinate of the center point"),
		),
		mcp.WithNumber("radius",
			mcp.Description(radiusDesc),
			mcp.DefaultNumber(defaultRadius),
		),
		mcp.WithString("category",
			mcp.Description("Category or type of place to search for"),
		),
		mcp.WithNumber("limit",
			mcp.Description(limitDesc),
			mcp.DefaultNumber(float64(defaultLimit)),
		),
	)
}

// CreateRouteTool creates a tool for routing between two points
func (f *ToolFactory) CreateRouteTool(name, description string) mcp.Tool {
	return mcp.NewTool(name,
		mcp.WithDescription(description),
		mcp.WithNumber("start_lat",
			mcp.Required(),
			mcp.Description("The latitude coordinate of the starting point"),
		),
		mcp.WithNumber("start_lon",
			mcp.Required(),
			mcp.Description("The longitude coordinate of the starting point"),
		),
		mcp.WithNumber("end_lat",
			mcp.Required(),
			mcp.Description("The latitude coordinate of the destination"),
		),
		mcp.WithNumber("end_lon",
			mcp.Required(),
			mcp.Description("The longitude coordinate of the destination"),
		),
		mcp.WithString("mode",
			mcp.Description("Transportation mode: car, bike, or foot"),
			mcp.DefaultString("car"),
		),
	)
}
