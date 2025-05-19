package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/NERVsystems/osmmcp/pkg/cache"
	"github.com/NERVsystems/osmmcp/pkg/osm"
	"github.com/NERVsystems/osmmcp/pkg/server"
	"github.com/NERVsystems/osmmcp/pkg/version"
)

// Version information
var (
	version        bool
	debug          bool
	generateConfig string
	userAgent      string
	mergeOnly      bool

	// Rate limits for each service
	nominatimRPS   float64
	nominatimBurst int
	overpassRPS    float64
	overpassBurst  int
	osrmRPS        float64
	osrmBurst      int
)

func init() {
	flag.BoolVar(&version, "version", false, "Display version information")
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")
	flag.StringVar(&generateConfig, "generate-config", "", "Generate a Claude Desktop Client config file at the specified path")
	flag.StringVar(&userAgent, "user-agent", osm.UserAgent, "User-Agent string for OSM API requests")
	flag.BoolVar(&mergeOnly, "merge-only", false, "Only merge new config, don't overwrite existing")

	// Nominatim rate limits
	flag.Float64Var(&nominatimRPS, "nominatim-rps", 1.0, "Nominatim rate limit in requests per second")
	flag.IntVar(&nominatimBurst, "nominatim-burst", 1, "Nominatim rate limit burst size")

	// Overpass rate limits
	flag.Float64Var(&overpassRPS, "overpass-rps", 1.0, "Overpass rate limit in requests per second")
	flag.IntVar(&overpassBurst, "overpass-burst", 1, "Overpass rate limit burst size")

	// OSRM rate limits
	flag.Float64Var(&osrmRPS, "osrm-rps", 1.0, "OSRM rate limit in requests per second")
	flag.IntVar(&osrmBurst, "osrm-burst", 1, "OSRM rate limit burst size")
}

func main() {
	flag.Parse()

	// Configure logging
	var logLevel slog.Level
	if debug {
		logLevel = slog.LevelDebug
	} else {
		logLevel = slog.LevelInfo
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	// Show version and exit if requested
	if version {
		showVersion()
		return
	}

	// Generate Claude Desktop config if requested
	if generateConfig != "" {
		if err := generateClientConfig(generateConfig, mergeOnly); err != nil {
			logger.Error("failed to generate config", "error", err)
			os.Exit(1)
		}
		logger.Info("successfully generated Claude Desktop Client config", "path", generateConfig)
		return
	}

	// Update global user agent if specified
	if userAgent != osm.UserAgent {
		osm.SetUserAgent(userAgent)
	}

	// Update rate limits if specified
	if nominatimRPS != 1.0 || nominatimBurst != 1 {
		osm.UpdateNominatimRateLimits(nominatimRPS, nominatimBurst)
	}
	if overpassRPS != 1.0 || overpassBurst != 1 {
		osm.UpdateOverpassRateLimits(overpassRPS, overpassBurst)
	}
	if osrmRPS != 1.0 || osrmBurst != 1 {
		osm.UpdateOSRMRateLimits(osrmRPS, osrmBurst)
	}

	logger.Info("starting OpenStreetMap MCP server",
		"version", version.BuildVersion,
		"log_level", logLevel.String(),
		"user_agent", userAgent,
		"nominatim_rps", nominatimRPS,
		"nominatim_burst", nominatimBurst,
		"overpass_rps", overpassRPS,
		"overpass_burst", overpassBurst,
		"osrm_rps", osrmRPS,
		"osrm_burst", osrmBurst)

	// Debug print to stderr to help diagnose MCP initialization issues
	fmt.Fprintf(os.Stderr, "DEBUG: Creating new server instance\n")

	// Create a new server instance
	s, err := server.NewServer()
	if err != nil {
		logger.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "DEBUG: Server instance created successfully\n")

	// Create context for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Run the MCP server with context
	fmt.Fprintf(os.Stderr, "DEBUG: Starting MCP server\n")
	if err := s.RunWithContext(ctx); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}

	// Server has shut down gracefully
	cache.GetGlobalCache().Stop()
	logger.Info("server stopped")
}

// generateClientConfig generates a configuration file for the Claude Desktop Client
func generateClientConfig(path string, mergeOnly bool) error {
	// Sanity check the path
	if path == "" {
		return fmt.Errorf("config path cannot be empty")
	}
	if !strings.HasSuffix(path, ".json") {
		return fmt.Errorf("config file must have .json extension")
	}

	// Clean the path and check for path traversal attempts
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") || filepath.IsAbs(cleanPath) {
		return fmt.Errorf("refusing to traverse outside workspace")
	}

	// Create config directory if it doesn't exist
	configDir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Read existing config if it exists and mergeOnly is true
	var existingConfig map[string]interface{}
	if mergeOnly {
		if data, err := os.ReadFile(cleanPath); err == nil {
			if err := json.Unmarshal(data, &existingConfig); err != nil {
				return fmt.Errorf("failed to parse existing config: %w", err)
			}
		}
	}

	// Create new config
	config := map[string]interface{}{
		"claude": map[string]interface{}{
			"api_key": os.Getenv("CLAUDE_API_KEY"),
			"model":   "claude-3-opus-20240229",
		},
		"server": map[string]interface{}{
			"host": "localhost",
			"port": 8080,
		},
	}

	// Merge with existing config if needed
	if mergeOnly && existingConfig != nil {
		for k, v := range existingConfig {
			if _, exists := config[k]; !exists {
				config[k] = v
			}
		}
	}

	// Write config file
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(cleanPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// showVersion displays version information and exits
func showVersion() {
	fmt.Println(version.String())
}
