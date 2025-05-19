// Package tools provides the OpenStreetMap MCP tools implementations.
package tools

import (
	"context"
	"encoding/json"
	"log/slog"
	"runtime/debug"

	"github.com/mark3labs/mcp-go/mcp"
)

// Version information
var (
	// Version is the application version, set during build
	Version = "dev"

	// BuildInfo contains additional build information
	BuildInfo *debug.BuildInfo
)

func init() {
	// Attempt to get build info for detailed version reporting
	info, ok := debug.ReadBuildInfo()
	if ok {
		BuildInfo = info
	}
}

// VersionInfo represents version information for the service
type VersionInfo struct {
	Version     string            `json:"version"`
	GoVersion   string            `json:"go_version,omitempty"`
	BuildTime   string            `json:"build_time,omitempty"`
	VCSRevision string            `json:"vcs_revision,omitempty"`
	Settings    map[string]string `json:"settings,omitempty"`
}

// GetVersionTool returns a tool definition for retrieving version information
func GetVersionTool() mcp.Tool {
	return mcp.NewTool("get_version",
		mcp.WithDescription("Get the version and build information of the OSM MCP service"),
	)
}

// HandleGetVersion implements version information retrieval
func HandleGetVersion(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := slog.Default().With("tool", "get_version")

	// Create version info
	versionInfo := VersionInfo{
		Version:  Version,
		Settings: make(map[string]string),
	}

	// Add build info if available
	if BuildInfo != nil {
		versionInfo.GoVersion = BuildInfo.GoVersion

		// Extract build settings
		for _, setting := range BuildInfo.Settings {
			if setting.Key == "vcs.revision" {
				versionInfo.VCSRevision = setting.Value
			} else if setting.Key == "vcs.time" {
				versionInfo.BuildTime = setting.Value
			} else {
				versionInfo.Settings[setting.Key] = setting.Value
			}
		}
	}

	// Return result
	resultBytes, err := json.Marshal(versionInfo)
	if err != nil {
		logger.Error("failed to marshal version info", "error", err)
		return ErrorResponse("Failed to retrieve version information"), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}
