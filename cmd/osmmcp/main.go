package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/NERVsystems/osmmcp/pkg/cache"
	"github.com/NERVsystems/osmmcp/pkg/monitoring"
	"github.com/NERVsystems/osmmcp/pkg/osm"
	"github.com/NERVsystems/osmmcp/pkg/registration"
	"github.com/NERVsystems/osmmcp/pkg/server"
	"github.com/NERVsystems/osmmcp/pkg/tools"
	"github.com/NERVsystems/osmmcp/pkg/tracing"
	ver "github.com/NERVsystems/osmmcp/pkg/version"
)

// Version information
var (
	showVersionFlag bool
	debug           bool
	generateConfig  string
	userAgent       string
	mergeOnly       bool

	// HTTP transport flags
	enableHTTP    bool
	httpOnly      bool
	httpAddr      string
	httpBaseURL   string
	httpAuthType  string
	httpAuthToken string

	// Monitoring flags
	enableMonitoring bool
	monitoringAddr   string

	// Registration flags
	enableRegistration bool
	registryURL        string
	serviceURL         string
	internalURL        string

	// Rate limits for each service
	nominatimRPS   float64
	nominatimBurst int
	overpassRPS    float64
	overpassBurst  int
	osrmRPS        float64
	osrmBurst      int
)

func init() {
	flag.BoolVar(&showVersionFlag, "version", false, "Display version information")
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")
	flag.StringVar(&generateConfig, "generate-config", "", "Generate a Claude Desktop Client config file at the specified path")
	flag.StringVar(&userAgent, "user-agent", osm.UserAgent, "User-Agent string for OSM API requests")
	flag.BoolVar(&mergeOnly, "merge-only", false, "Only merge new config, don't overwrite existing")

	// HTTP transport flags
	// TODO(NERV-MCP-STANDARD): This --enable-http flag is the standardized approach across all
	// NERV Systems MCP servers. Other servers (takmcp, aismcp, qgismcp) should adopt this pattern.
	// See MCP_SERVER_AUDIT.md for details.
	flag.BoolVar(&enableHTTP, "enable-http", false, "Enable HTTP+SSE transport (in addition to stdio)")
	flag.BoolVar(&httpOnly, "http-only", false, "Run HTTP transport only, skip stdio (requires --enable-http)")
	flag.StringVar(&httpAddr, "http-addr", ":7082", "HTTP server address")
	flag.StringVar(&httpBaseURL, "http-base-url", "", "Base URL for HTTP transport (auto-detected if empty)")
	flag.StringVar(&httpAuthType, "http-auth-type", "none", "HTTP authentication type: none, bearer, basic")
	flag.StringVar(&httpAuthToken, "http-auth-token", "", "HTTP authentication token")

	// Monitoring flags
	flag.BoolVar(&enableMonitoring, "enable-monitoring", true, "Enable Prometheus metrics and health endpoints")
	flag.StringVar(&monitoringAddr, "monitoring-addr", ":9090", "Monitoring server address")

	// Registration flags
	flag.BoolVar(&enableRegistration, "enable-registration", false, "Enable service registration with nerva-monitor")
	flag.StringVar(&registryURL, "registry-url", "", "nerva-monitor registry URL (e.g., http://nerva-monitor:7083)")
	flag.StringVar(&serviceURL, "service-url", "", "External URL where this service is accessible")
	flag.StringVar(&internalURL, "internal-url", "", "Internal URL for container environments")

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

	// Initialize OpenTelemetry tracing
	ctx := context.Background()
	shutdownTracing, err := tracing.InitTracing(ctx, ver.BuildVersion)
	if err != nil {
		logger.Error("failed to initialize tracing", "error", err)
		// Continue without tracing - it's not critical
	} else {
		// Ensure tracing is shut down on exit
		defer func() {
			if err := shutdownTracing(ctx); err != nil {
				logger.Error("error shutting down tracing", "error", err)
			}
		}()

		if endpoint := os.Getenv("OTLP_ENDPOINT"); endpoint != "" {
			logger.Info("OpenTelemetry tracing enabled", "endpoint", endpoint)
		}
	}

	// Show version and exit if requested
	if showVersionFlag {
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
		"version", ver.BuildVersion,
		"log_level", logLevel.String(),
		"user_agent", userAgent,
		"nominatim_rps", nominatimRPS,
		"nominatim_burst", nominatimBurst,
		"overpass_rps", overpassRPS,
		"overpass_burst", overpassBurst,
		"osrm_rps", osrmRPS,
		"osrm_burst", osrmBurst,
		"http_enabled", enableHTTP,
		"monitoring_enabled", enableMonitoring,
		"monitoring_addr", monitoringAddr)

	// Initialize health checker
	var healthChecker *monitoring.HealthChecker
	if enableMonitoring {
		healthChecker = monitoring.NewHealthChecker(monitoring.ServiceName, ver.BuildVersion)
		defer healthChecker.Shutdown()

		// Set up monitoring hooks for OSM client
		osm.SetMonitoringHooks(&osm.MonitoringHooks{
			OnRequest: func(service, operation string) {
				monitoring.RecordExternalServiceRequest(service, operation, 0, false) // Start request
			},
			OnResponse: func(service, operation string, duration time.Duration, success bool) {
				monitoring.RecordExternalServiceRequest(service, operation, duration, success)
			},
			OnRateLimit: func(service string, waitTime time.Duration) {
				monitoring.RecordRateLimitWait(service, waitTime)
				monitoring.RecordRateLimitExceeded(service)
			},
			OnError: func(service, errorType string) {
				monitoring.RecordError(service, errorType)
			},
		})
	}

	// Debug print to stderr to help diagnose MCP initialization issues
	fmt.Fprintf(os.Stderr, "DEBUG: Creating new server instance\n")

	// Create a new server instance
	s, err := server.NewServer()
	if err != nil {
		logger.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "DEBUG: Server instance created successfully\n")

	// Start monitoring external services if health checker is enabled
	if healthChecker != nil {
		startExternalServiceMonitoring(healthChecker, logger)
	}

	// Create context for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start monitoring server if enabled (Prometheus metrics only)
	var monitoringServer *http.Server
	if enableMonitoring {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())

		monitoringServer = &http.Server{
			Addr:              monitoringAddr,
			Handler:           mux,
			ReadHeaderTimeout: 30 * time.Second, // Prevent Slowloris attacks
		}

		go func() {
			logger.Info("starting Prometheus metrics server", "addr", monitoringAddr)
			if err := monitoringServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("monitoring server error", "error", err)
			}
		}()

		// Setup graceful shutdown for monitoring server
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := monitoringServer.Shutdown(shutdownCtx); err != nil {
				logger.Error("failed to shutdown monitoring server", "error", err)
			}
		}()
	}

	// Initialize registration client if enabled
	var regClient *registration.Client
	if enableRegistration {
		// Get tool names from registry
		toolRegistry := tools.NewRegistry(logger)
		toolNames := toolRegistry.GetToolNames()

		// Build service URL and health URL
		svcURL := serviceURL
		healthURL := serviceURL + "/health"
		if serviceURL == "" && enableHTTP {
			svcURL = fmt.Sprintf("http://localhost%s", httpAddr)
			healthURL = fmt.Sprintf("http://localhost%s/health", httpAddr)
		}

		regCfg := registration.Config{
			Enabled:           enableRegistration,
			RegistryURL:       registryURL,
			ServiceName:       "osmmcp",
			ServiceType:       "mcp",
			ServiceURL:        svcURL,
			HealthURL:         healthURL,
			InternalURL:       internalURL,
			InternalHealthURL: internalURL + "/health",
			Version:           ver.BuildVersion,
			Capabilities:      []string{"geocoding", "routing", "poi", "mapping"},
			Tools:             toolNames,
			Metadata: map[string]interface{}{
				"transport": map[string]bool{"stdio": true, "http": enableHTTP},
			},
		}
		regClient = registration.NewClient(regCfg, logger)
		regClient.Start(ctx)
		defer regClient.Stop()

		logger.Info("registration client initialized",
			"registry_url", registryURL,
			"service_url", svcURL,
			"tool_count", len(toolNames))
	}

	// Start HTTP transport in background if enabled (non-blocking)
	var httpTransport *server.HTTPTransport
	if enableHTTP {
		config := server.HTTPTransportConfig{
			Addr:        httpAddr,
			BaseURL:     httpBaseURL,
			AuthType:    httpAuthType,
			AuthToken:   httpAuthToken,
			MCPEndpoint: "/mcp",
		}

		httpTransport = server.NewHTTPTransport(s.GetMCPServer(), config, logger)

		// Set health checker if enabled
		if healthChecker != nil {
			httpTransport.SetHealthChecker(healthChecker)
		}

		// Start HTTP transport in goroutine (non-blocking)
		go func() {
			fmt.Fprintf(os.Stderr, "DEBUG: Starting Streamable HTTP transport in background\n")
			logger.Info("starting Streamable HTTP transport", "addr", httpAddr, "endpoint", "/mcp")
			if err := httpTransport.Start(); err != nil && err != http.ErrServerClosed {
				logger.Error("HTTP transport error", "error", err)
			}
		}()

		// Setup graceful shutdown for HTTP transport
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := httpTransport.Shutdown(shutdownCtx); err != nil {
				logger.Error("failed to shutdown HTTP transport", "error", err)
			}
		}()
	}

	// Transport startup logic:
	// - If HTTP is NOT enabled: Run stdio on main thread (blocking) - default behavior
	// - If HTTP IS enabled and httpOnly is false: Run stdio in goroutine (non-blocking), then wait for shutdown
	// - If HTTP IS enabled and httpOnly is true: Skip stdio, just wait for shutdown
	if !enableHTTP {
		// STDIO-only mode (default) - run blocking on main thread
		fmt.Fprintf(os.Stderr, "DEBUG: Starting stdio MCP server (blocking)\n")
		logger.Info("transport_enabled", "type", "stdio", "mode", "blocking")
		if err := s.RunWithContext(ctx); err != nil {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	} else if httpOnly {
		// HTTP-only mode - skip stdio transport entirely
		fmt.Fprintf(os.Stderr, "DEBUG: HTTP-only mode, skipping stdio transport\n")
		logger.Info("server_ready", "transports", []string{"http"}, "http_only", true)
		<-ctx.Done()
		logger.Info("shutdown signal received")
	} else {
		// HTTP enabled with stdio - run stdio in goroutine so both transports work
		go func() {
			fmt.Fprintf(os.Stderr, "DEBUG: Starting stdio MCP server (background)\n")
			logger.Info("transport_enabled", "type", "stdio", "mode", "background")
			if err := s.RunWithContext(ctx); err != nil {
				logger.Error("stdio transport error", "error", err)
				// Don't exit - HTTP transport may still be useful
			}
		}()

		// Wait for shutdown signal
		logger.Info("server_ready", "transports", []string{"stdio", "http"})
		<-ctx.Done()
		logger.Info("shutdown signal received")
	}

	// Server has shut down gracefully
	cache.StopGlobalCache()
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

	// Clean the path and validate it's safe
	cleanPath := filepath.Clean(path)
	if err := validateSafePath(cleanPath); err != nil {
		return fmt.Errorf("invalid config path: %w", err)
	}

	// Create config directory if it doesn't exist
	configDir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(configDir, 0750); err != nil { // More restrictive permissions
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
			"port": 7082,
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

// validateSafePath validates that a path is safe to write to within the current working directory
func validateSafePath(path string) error {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}

	// Resolve the absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// Check if the resolved path is within the current working directory
	relPath, err := filepath.Rel(cwd, absPath)
	if err != nil {
		return fmt.Errorf("failed to determine relative path: %w", err)
	}

	// Reject paths that go outside the working directory
	if strings.HasPrefix(relPath, "..") || strings.Contains(relPath, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path traversal detected: %s", relPath)
	}

	// Additional safety checks
	if filepath.IsAbs(path) {
		return fmt.Errorf("absolute paths are not allowed for security reasons")
	}

	return nil
}

// showVersion displays version information and exits
func showVersion() {
	fmt.Println(ver.String())
}

// startExternalServiceMonitoring starts monitoring external services
func startExternalServiceMonitoring(healthChecker *monitoring.HealthChecker, logger *slog.Logger) {
	// Monitor Nominatim service
	nominatimMonitor := monitoring.NewConnectionMonitor(
		"nominatim",
		healthChecker,
		func() error {
			return osm.CheckNominatimHealth()
		},
		30*time.Second,
	)
	nominatimMonitor.Start()

	// Monitor Overpass service
	overpassMonitor := monitoring.NewConnectionMonitor(
		"overpass",
		healthChecker,
		func() error {
			return osm.CheckOverpassHealth()
		},
		30*time.Second,
	)
	overpassMonitor.Start()

	// Monitor OSRM service
	osrmMonitor := monitoring.NewConnectionMonitor(
		"osrm",
		healthChecker,
		func() error {
			return osm.CheckOSRMHealth()
		},
		30*time.Second,
	)
	osrmMonitor.Start()

	logger.Info("started external service monitoring",
		"services", []string{"nominatim", "overpass", "osrm"},
		"check_interval", "30s")
}
