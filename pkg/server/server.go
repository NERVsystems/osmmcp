// Package server provides the MCP server implementation for the OpenStreetMap integration.
package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/NERVsystems/osmmcp/pkg/osm"
	"github.com/NERVsystems/osmmcp/pkg/tools"
	"github.com/NERVsystems/osmmcp/pkg/tools/prompts"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	// ServerName is the name of the MCP server
	ServerName = "osm-mcp-server"

	// ServerVersion is the version of the MCP server
	ServerVersion = "0.1.0"
)

// Server encapsulates the MCP server with OpenStreetMap tools.
type Server struct {
	srv     *server.MCPServer
	logger  *slog.Logger
	stopCh  chan struct{}
	doneCh  chan struct{}
	running bool
	mu      sync.Mutex
	once    sync.Once // Ensure we only close stopCh once
}

// NewServer creates a new OpenStreetMap MCP server with all tools registered.
func NewServer() (*Server, error) {
	logger := slog.Default()
	logger.Info("initializing OpenStreetMap MCP server",
		"name", ServerName,
		"version", ServerVersion)

	// Create MCP server with options
	srv := server.NewMCPServer(
		ServerName,
		ServerVersion,
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	// Create tool registry and register all tools and prompts
	registry := tools.NewRegistry(logger)
	registry.RegisterAll(srv)

	// Register prompt templates so the AI knows how to use the tools
	geocodingTemplate := mcp.NewPromptTemplate("geocoding",
		[]mcp.PromptMessage{
			mcp.NewPromptMessage(
				mcp.RoleSystem,
				mcp.NewTextContent(prompts.GeocodingSystemPrompt()),
			),
		},
	)
	srv.AddPromptTemplate(geocodingTemplate)

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
		err := server.ServeStdio(s.srv)
		if err != nil && err != io.EOF {
			s.logger.Error("server error", "error", err)
		}
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
	go func() {
		select {
		case <-ctx.Done():
			s.Shutdown()
		case <-s.stopCh:
			// Already being shut down
		}
	}()

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
}

// WaitForShutdown blocks until the server has fully shut down.
func (s *Server) WaitForShutdown() {
	<-s.doneCh
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
	w.Write([]byte(`{"status":"ok"}`))
	return http.StatusOK, nil
}

// handleGeocode handles geocoding requests
func (h *Handler) handleGeocode(w http.ResponseWriter, r *http.Request) (int, error) {
	// TODO: Implement geocoding handler
	return http.StatusNotImplemented, nil
}

// handlePlaces handles places search requests
func (h *Handler) handlePlaces(w http.ResponseWriter, r *http.Request) (int, error) {
	// TODO: Implement places handler
	return http.StatusNotImplemented, nil
}

// handleRoute handles routing requests
func (h *Handler) handleRoute(w http.ResponseWriter, r *http.Request) (int, error) {
	// TODO: Implement route handler
	return http.StatusNotImplemented, nil
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	return time.Now().Format("20060102150405.000000000")
}
