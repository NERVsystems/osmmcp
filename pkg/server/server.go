// Package server provides the MCP server implementation for the OpenStreetMap integration.
package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/NERVsystems/osmmcp/pkg/core"
	"github.com/NERVsystems/osmmcp/pkg/osm"
	"github.com/NERVsystems/osmmcp/pkg/tools"
	"github.com/NERVsystems/osmmcp/pkg/tools/prompts"
)

const (
	// ServerName is the name of the MCP server
	ServerName = "osm-mcp-server"

	// ServerVersion is the version of the MCP server
	ServerVersion = "0.1.0"
)

// Server encapsulates the MCP server with OpenStreetMap tools.
type Server struct {
	srv          *mcpserver.MCPServer
	logger       *slog.Logger
	stopCh       chan struct{}
	doneCh       chan struct{}
	running      bool
	mu           sync.Mutex
	once         sync.Once // Ensure we only close stopCh once
	ctxCancel    context.CancelFunc
	ctxGoroutine sync.Once // Ensure we only start one context goroutine
}

// NewServer creates a new OpenStreetMap MCP server with all tools registered.
func NewServer() (*Server, error) {
	logger := slog.Default()
	logger.Info("initializing OpenStreetMap MCP server",
		"name", ServerName,
		"version", ServerVersion)

	// Initialize tile resource manager
	core.InitTileResourceManager(logger)

	// Create MCP server with options
	srv := mcpserver.NewMCPServer(
		ServerName,
		ServerVersion,
		mcpserver.WithToolCapabilities(false),
		mcpserver.WithRecovery(),
	)

	// Create tool registry and register all tools and prompts
	registry := tools.NewRegistry(logger)
	registry.RegisterAll(srv)

	// Register the geocoding system prompt using the v0.28.0+ API
	geocodingPrompt := mcp.NewPrompt("geocoding_system",
		mcp.WithPromptDescription("System prompt with geocoding instructions"),
	)

	// Add the prompt with its handler function
	srv.AddPrompt(geocodingPrompt, func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return mcp.NewGetPromptResult(
			"Geocoding System Instructions",
			[]mcp.PromptMessage{
				mcp.NewPromptMessage(
					mcp.RoleAssistant,
					mcp.NewTextContent(prompts.GeocodingSystemPrompt()),
				),
			},
		), nil
	})

	return &Server{
		srv:    srv,
		logger: logger,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}, nil
}

// Run starts the MCP server using stdin/stdout for communication.
// This method blocks until the server is stopped or an error occurs.
func (s *Server) Run() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()

	// Run the server in a goroutine
	go func() {
		defer close(s.doneCh)
		err := mcpserver.ServeStdio(s.srv)
		if err != nil && err != io.EOF {
			s.logger.Error("MCP server error", "error", err)
		} else if err == io.EOF {
			s.logger.Info("stdin closed, shutting down server gracefully")
		}

		// Ensure the main Run loop is notified that the
		// server has finished processing.
		s.Shutdown()
	}()

	// Wait for stop signal
	<-s.stopCh

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	// Wait for server to finish before returning
	<-s.doneCh
	return nil
}

// RunWithContext starts the MCP server and allows for graceful shutdown via context.
// This method blocks until the context is canceled or an error occurs.
func (s *Server) RunWithContext(ctx context.Context) error {
	// Create a goroutine to watch the context for cancellation
	s.ctxGoroutine.Do(func() {
		// Create a derived context that we can cancel
		derived, cancel := context.WithCancel(ctx)
		s.ctxCancel = cancel

		go func() {
			select {
			case <-derived.Done():
				s.Shutdown()
			case <-s.stopCh:
				// Already being shut down
			}
		}()

		// Start parent process monitoring as a fallback for stdio transport
		// This ensures the server shuts down if the parent process exits unexpectedly
		go s.monitorParentProcess()
	})

	return s.Run()
}

// Shutdown initiates a graceful shutdown of the server.
// It does not block and returns immediately.
// Using sync.Once to ensure we don't close an already closed channel.
func (s *Server) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	// Signal the server to stop using sync.Once to avoid panics
	// on double close of the channel
	s.once.Do(func() {
		close(s.stopCh)
	})

	// Cancel the context if we have one
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
}

// WaitForShutdown blocks until the server has fully shut down.
func (s *Server) WaitForShutdown() {
	<-s.doneCh
}

// GetMCPServer returns the underlying MCP server instance for HTTP transport
func (s *Server) GetMCPServer() *mcpserver.MCPServer {
	return s.srv
}

// Handler represents the HTTP server handler
type Handler struct {
	logger *slog.Logger
	osm    *osm.Client
}

// NewHandler creates a new server handler
func NewHandler(logger *slog.Logger) *Handler {
	return &Handler{
		logger: logger,
		osm:    osm.NewOSMClient(),
	}
}

// ServeHTTP implements the http.Handler interface
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	path := r.URL.Path
	method := r.Method

	// Add request ID to context
	reqID := r.Header.Get("X-Request-ID")
	if reqID == "" {
		reqID = generateRequestID()
	}
	// Not using ctx here, so no need to create and update it
	// Directly use the reqID for logging

	// Log request
	h.logger.Info("request started",
		"request_id", reqID,
		"method", method,
		"path", path,
		"remote_addr", r.RemoteAddr,
		"user_agent", r.UserAgent())

	// Handle request
	var status int
	var err error

	switch {
	case path == "/health":
		status, err = h.handleHealth(w, r)
	case path == "/geocode":
		status, err = h.handleGeocode(w, r)
	case path == "/places":
		status, err = h.handlePlaces(w, r)
	case path == "/route":
		status, err = h.handleRoute(w, r)
	default:
		status = http.StatusNotFound
		err = nil
	}

	// Log response
	duration := time.Since(start)
	if err != nil {
		h.logger.Error("request failed",
			"request_id", reqID,
			"method", method,
			"path", path,
			"status", status,
			"duration", duration,
			"error", err)
	} else {
		h.logger.Info("request completed",
			"request_id", reqID,
			"method", method,
			"path", path,
			"status", status,
			"duration", duration)
	}
}

// handleHealth handles health check requests
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) (int, error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
		h.logger.Error("failed to write health response", "error", err)
		return http.StatusOK, err // Status already written, but return error for logging
	}

	return http.StatusOK, nil
}

// handleGeocode handles geocoding requests
func (h *Handler) handleGeocode(w http.ResponseWriter, r *http.Request) (int, error) {
	q := r.URL.Query()
	address := q.Get("address")
	region := q.Get("region")

	req := mcp.CallToolRequest{
		Params: struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments,omitempty"`
			Meta      *mcp.Meta      `json:"_meta,omitempty"`
		}{
			Name: "geocode_address",
			Arguments: map[string]any{
				"address": address,
			},
		},
	}
	if region != "" {
		req.Params.Arguments["region"] = region
	}

	result, err := tools.HandleGeocodeAddress(r.Context(), req)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	var content string
	for _, c := range result.Content {
		if t, ok := c.(mcp.TextContent); ok {
			content = t.Text
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	status := http.StatusOK
	if result.IsError {
		status = http.StatusBadRequest
	}
	w.WriteHeader(status)

	if _, err := w.Write([]byte(content)); err != nil {
		h.logger.Error("failed to write geocode response", "error", err)
		return status, err
	}

	return status, nil
}

// handlePlaces handles places search requests
func (h *Handler) handlePlaces(w http.ResponseWriter, r *http.Request) (int, error) {
	q := r.URL.Query()
	req := mcp.CallToolRequest{
		Params: struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments,omitempty"`
			Meta      *mcp.Meta      `json:"_meta,omitempty"`
		}{
			Name: "find_nearby_places",
			Arguments: map[string]any{
				"latitude":  q.Get("latitude"),
				"longitude": q.Get("longitude"),
				"radius":    q.Get("radius"),
				"category":  q.Get("category"),
				"limit":     q.Get("limit"),
			},
		},
	}

	result, err := tools.HandleFindNearbyPlaces(r.Context(), req)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	var content string
	for _, c := range result.Content {
		if t, ok := c.(mcp.TextContent); ok {
			content = t.Text
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	status := http.StatusOK
	if result.IsError {
		status = http.StatusBadRequest
	}
	w.WriteHeader(status)

	if _, err := w.Write([]byte(content)); err != nil {
		h.logger.Error("failed to write places response", "error", err)
		return status, err
	}

	return status, nil
}

// handleRoute handles routing requests
func (h *Handler) handleRoute(w http.ResponseWriter, r *http.Request) (int, error) {
	q := r.URL.Query()
	req := mcp.CallToolRequest{
		Params: struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments,omitempty"`
			Meta      *mcp.Meta      `json:"_meta,omitempty"`
		}{
			Name: "route_fetch",
			Arguments: map[string]any{
				"start": map[string]any{
					"latitude":  q.Get("start_lat"),
					"longitude": q.Get("start_lon"),
				},
				"end": map[string]any{
					"latitude":  q.Get("end_lat"),
					"longitude": q.Get("end_lon"),
				},
				"mode": q.Get("mode"),
			},
		},
	}

	result, err := tools.HandleRouteFetch(r.Context(), req)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	var content string
	for _, c := range result.Content {
		if t, ok := c.(mcp.TextContent); ok {
			content = t.Text
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	status := http.StatusOK
	if result.IsError {
		status = http.StatusBadRequest
	}
	w.WriteHeader(status)

	if _, err := w.Write([]byte(content)); err != nil {
		h.logger.Error("failed to write route response", "error", err)
		return status, err
	}

	return status, nil
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	return time.Now().Format("20060102150405.000000000")
}

// monitorParentProcess monitors the parent process and shuts down the server
// when the parent process exits. This serves as a fallback mechanism in case
// stdin EOF detection fails. The primary shutdown mechanism should be EOF on stdin.
func (s *Server) monitorParentProcess() {
	ppid := os.Getppid()
	s.logger.Debug("starting parent process monitor as fallback", "ppid", ppid)

	// Check parent process every 30 seconds (less aggressive than primary EOF detection)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			// Server is already shutting down
			return
		case <-ticker.C:
			// Check if parent process still exists
			if !isProcessRunning(ppid) {
				s.logger.Info("parent process has exited (fallback detection), shutting down server", "ppid", ppid)
				s.Shutdown()
				return
			}
		}
	}
}

// isProcessRunning checks if a process with the given PID is still running
func isProcessRunning(pid int) bool {
	// On Unix systems, sending signal 0 to a process checks if it exists
	// without actually sending a signal
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Send signal 0 (syscall.Signal(0)) to check if process exists
	// This is a Unix convention - signal 0 checks process existence without sending a real signal
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		// Process doesn't exist or we don't have permission
		return false
	}

	return true
}
