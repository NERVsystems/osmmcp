package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/NERVsystems/osmmcp/pkg/core"
	"github.com/NERVsystems/osmmcp/pkg/monitoring"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// HTTPTransportConfig holds configuration for the HTTP transport
type HTTPTransportConfig struct {
	Addr           string  `json:"addr"`             // HTTP server address (e.g., ":8080")
	BaseURL        string  `json:"base_url"`         // Base URL for service discovery
	AuthType       string  `json:"auth_type"`        // Authentication type: "bearer", "basic", "none"
	AuthToken      string  `json:"auth_token"`       // Authentication token
	SSEEndpoint    string  `json:"sse_endpoint"`     // SSE endpoint path (default: "/sse")
	MsgEndpoint    string  `json:"msg_endpoint"`     // Message endpoint path (default: "/message")
	RateLimit      float64 `json:"rate_limit"`       // Requests per second per IP (0 = disabled)
	RateBurst      int     `json:"rate_burst"`       // Burst size for rate limiter
	MaxRequestSize int64   `json:"max_request_size"` // Maximum request body size in bytes
	MaxHeaderBytes int     `json:"max_header_bytes"` // Maximum header size in bytes
	TLSCertFile    string  `json:"tls_cert_file"`    // Path to TLS certificate file
	TLSKeyFile     string  `json:"tls_key_file"`     // Path to TLS private key file
	ForceHTTPS     bool    `json:"force_https"`      // Force HTTPS redirect for HTTP requests
}

// DefaultHTTPTransportConfig returns sensible defaults
func DefaultHTTPTransportConfig() HTTPTransportConfig {
	return HTTPTransportConfig{
		Addr:           ":7082",
		BaseURL:        "",
		AuthType:       "none",
		AuthToken:      "",
		SSEEndpoint:    "/sse",
		MsgEndpoint:    "/message",
		RateLimit:      10,       // 10 requests per second per IP
		RateBurst:      20,       // Allow bursts of 20
		MaxRequestSize: 10 << 20, // 10 MB
		MaxHeaderBytes: 1 << 20,  // 1 MB
		TLSCertFile:    "",       // No TLS by default
		TLSKeyFile:     "",       // No TLS by default
		ForceHTTPS:     false,    // No HTTPS enforcement by default
	}
}

// HTTPTransport implements HTTP+SSE dual transport for MCP
type HTTPTransport struct {
	config        HTTPTransportConfig
	logger        *slog.Logger
	sseServer     *mcpserver.SSEServer
	mux           *http.ServeMux
	httpSrv       *http.Server
	rateLimiter   *RateLimiter
	healthChecker *monitoring.HealthChecker
	mu            sync.RWMutex
}

// NewHTTPTransport creates a new HTTP transport instance
func NewHTTPTransport(mcpServer *mcpserver.MCPServer, config HTTPTransportConfig, logger *slog.Logger) *HTTPTransport {
	if logger == nil {
		logger = slog.Default()
	}

	// Validate authentication configuration
	if config.AuthType != "none" && config.AuthToken != "" {
		if err := core.ValidateAuthToken(config.AuthToken); err != nil {
			logger.Warn("weak authentication token detected", "error", err.Error())
		}
	}

	// Create SSE server with proper endpoint configuration
	sseServer := mcpserver.NewSSEServer(
		mcpServer,
		mcpserver.WithSSEEndpoint(config.SSEEndpoint),
		mcpserver.WithMessageEndpoint(config.MsgEndpoint),
		mcpserver.WithBaseURL(config.BaseURL),
	)

	// Create HTTP mux
	mux := http.NewServeMux()

	transport := &HTTPTransport{
		config:    config,
		logger:    logger,
		sseServer: sseServer,
		mux:       mux,
	}

	// Mount handlers with proper routing for dual transport support
	transport.setupRoutes()

	return transport
}

// SetHealthChecker sets the health checker for the HTTP transport
func (t *HTTPTransport) SetHealthChecker(hc *monitoring.HealthChecker) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.healthChecker = hc
}

// setupRoutes configures all HTTP routes
func (t *HTTPTransport) setupRoutes() {
	// Root endpoint for service discovery
	t.mux.HandleFunc("/", t.httpsEnforcement(t.handleServiceDiscovery))

	// Health check endpoints (no auth required)
	t.mux.HandleFunc("/health", t.handleHealth)
	t.mux.HandleFunc("/ready", t.handleReady)
	t.mux.HandleFunc("/live", t.handleLive)

	// Debug endpoints (no auth required)
	t.mux.HandleFunc(t.config.SSEEndpoint+"/debug", t.handleSSEDebug)
	t.mux.HandleFunc(t.config.MsgEndpoint+"/debug", t.handleMessageDebug)

	// Mount both SSE and Message handlers for dual transport support
	// This ensures both POST /message and SSE /sse work correctly
	t.mux.Handle(t.config.SSEEndpoint, t.httpsEnforcement(t.authMiddleware(t.sseServer.SSEHandler()).ServeHTTP))
	t.mux.Handle(t.config.SSEEndpoint+"/", t.httpsEnforcement(t.authMiddleware(t.sseServer.SSEHandler()).ServeHTTP))
	t.mux.Handle(t.config.MsgEndpoint, t.httpsEnforcement(t.authMiddleware(t.sseServer.MessageHandler()).ServeHTTP))
	t.mux.Handle(t.config.MsgEndpoint+"/", t.httpsEnforcement(t.authMiddleware(t.sseServer.MessageHandler()).ServeHTTP))
}

// httpsEnforcement redirects HTTP requests to HTTPS if ForceHTTPS is enabled
func (t *HTTPTransport) httpsEnforcement(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if t.config.ForceHTTPS && r.TLS == nil {
			// Redirect HTTP to HTTPS
			httpsURL := "https://" + r.Host + r.RequestURI

			// Log the redirect for security audit
			t.logger.Info("redirecting HTTP request to HTTPS",
				"client_ip", r.RemoteAddr,
				"original_url", r.URL.String(),
				"redirect_url", httpsURL)

			http.Redirect(w, r, httpsURL, http.StatusMovedPermanently)
			return
		}

		next(w, r)
	}
}

// authMiddleware provides authentication for MCP endpoints
func (t *HTTPTransport) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health/discovery/debug endpoints
		if r.URL.Path == "/health" || r.URL.Path == "/ready" || r.URL.Path == "/live" ||
			r.URL.Path == "/" ||
			r.URL.Path == t.config.SSEEndpoint+"/debug" || r.URL.Path == t.config.MsgEndpoint+"/debug" {
			next.ServeHTTP(w, r)
			return
		}

		if t.config.AuthType == "none" {
			next.ServeHTTP(w, r)
			return
		}

		var authResult core.AuthResult

		switch t.config.AuthType {
		case "bearer":
			authHeader := r.Header.Get("Authorization")
			authResult = core.AuthenticateBearer(authHeader, t.config.AuthToken)

		case "basic":
			username, password, ok := r.BasicAuth()
			if !ok {
				authResult = core.AuthResult{
					Authorized: false,
					Error:      "Missing basic auth credentials",
				}
			} else {
				authResult = core.AuthenticateBasic(username, password, t.config.AuthToken)
			}

		default:
			authResult = core.AuthResult{
				Authorized: false,
				Error:      "Unknown auth type",
			}
		}

		if !authResult.Authorized {
			t.logger.Warn("authentication failed",
				"remote_addr", r.RemoteAddr,
				"path", r.URL.Path,
				"auth_type", t.config.AuthType,
				"error", authResult.Error,
				"auth_duration", authResult.Duration)

			w.Header().Set("WWW-Authenticate", "Bearer")
			t.writeJSONRPCError(w, nil, -32602, "Authentication required")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleServiceDiscovery provides service discovery for MCP clients
func (t *HTTPTransport) handleServiceDiscovery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	baseURL := t.config.BaseURL
	if baseURL == "" {
		// Prefer HTTPS if TLS is configured or forced
		scheme := "http"
		if r.TLS != nil || t.config.ForceHTTPS || (t.config.TLSCertFile != "" && t.config.TLSKeyFile != "") {
			scheme = "https"
		}
		baseURL = fmt.Sprintf("%s://%s", scheme, r.Host)
	}

	// Minimal service discovery to avoid information disclosure
	discovery := map[string]interface{}{
		"service":   "mcp-server", // Generic service name
		"transport": "HTTP+SSE",
		"endpoints": map[string]string{
			"sse":     baseURL + t.config.SSEEndpoint,
			"message": baseURL + t.config.MsgEndpoint,
		},
		"capabilities": map[string]interface{}{
			"tools":   true,
			"prompts": true,
		},
		"auth": map[string]interface{}{
			"required": t.config.AuthType != "none",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(discovery); err != nil {
		t.logger.Error("failed to encode service discovery response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// handleHealth provides comprehensive health check endpoint
func (t *HTTPTransport) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	t.mu.RLock()
	hc := t.healthChecker
	t.mu.RUnlock()

	if hc != nil {
		// Use the health checker for comprehensive health status
		hc.HealthHandler()(w, r)
	} else {
		// Fallback to simple health check (minimal info)
		health := map[string]interface{}{
			"status": "ok",
			// Remove timestamp, service name, and version to prevent information disclosure
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(health); err != nil {
			t.logger.Error("failed to encode health response", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}
}

// handleReady provides Kubernetes-style readiness check
func (t *HTTPTransport) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	t.mu.RLock()
	hc := t.healthChecker
	t.mu.RUnlock()

	if hc != nil {
		hc.ReadinessHandler()(w, r)
	} else {
		// Fallback to simple ready response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"ready":  true,
			"status": "ok",
		}); err != nil {
			t.logger.Error("failed to encode ready response", "error", err)
		}
	}
}

// handleLive provides Kubernetes-style liveness check
func (t *HTTPTransport) handleLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	t.mu.RLock()
	hc := t.healthChecker
	t.mu.RUnlock()

	if hc != nil {
		hc.LivenessHandler()(w, r)
	} else {
		// Fallback to simple alive response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"alive": true,
			// Remove uptime to prevent timing information disclosure
		}); err != nil {
			t.logger.Error("failed to encode liveness response", "error", err)
		}
	}
}

// handleSSEDebug provides debug information for SSE endpoint
func (t *HTTPTransport) handleSSEDebug(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	debug := map[string]interface{}{
		"endpoint":    t.config.SSEEndpoint,
		"description": "Server-Sent Events endpoint for MCP communication",
		"usage":       "Connect with Accept: text/event-stream header",
		"transport":   "HTTP+SSE",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(debug); err != nil {
		t.logger.Error("failed to encode SSE debug response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// handleMessageDebug provides debug information for message endpoint
func (t *HTTPTransport) handleMessageDebug(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	debug := map[string]interface{}{
		"endpoint":    t.config.MsgEndpoint,
		"description": "JSON-RPC message endpoint for MCP communication",
		"usage":       "POST JSON-RPC messages with sessionId query parameter",
		"transport":   "HTTP+SSE",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(debug); err != nil {
		t.logger.Error("failed to encode message debug response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// writeJSONRPCError writes a JSON-RPC error response
func (t *HTTPTransport) writeJSONRPCError(w http.ResponseWriter, id interface{}, code int, message string) {
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		t.logger.Error("failed to encode JSON-RPC error", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// Start begins serving HTTP requests
func (t *HTTPTransport) Start() error {
	t.mu.Lock()

	if t.httpSrv != nil {
		t.mu.Unlock()
		return core.NewError(core.ErrInternalError, "HTTP transport already started").
			WithGuidance("The HTTP transport is already running. Stop it before starting again.")
	}

	// Apply middleware in the correct order
	handler := http.Handler(t.mux)
	handler = TracingMiddleware()(handler) // Add tracing first to capture all requests
	handler = LoggingMiddleware(t.logger)(handler)
	handler = SecurityHeaders(handler)
	handler = RequestSizeLimiter(10 * 1024 * 1024)(handler) // 10MB limit

	t.httpSrv = &http.Server{
		Addr:         t.config.Addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Check if TLS is configured
	if t.config.TLSCertFile != "" && t.config.TLSKeyFile != "" {
		t.logger.Info("starting HTTPS transport",
			"addr", t.config.Addr,
			"sse_endpoint", t.config.SSEEndpoint,
			"message_endpoint", t.config.MsgEndpoint,
			"auth_type", t.config.AuthType,
			"base_url", t.config.BaseURL,
			"tls_enabled", true,
			"force_https", t.config.ForceHTTPS)

		t.mu.Unlock() // Release lock before blocking call
		return t.httpSrv.ListenAndServeTLS(t.config.TLSCertFile, t.config.TLSKeyFile)
	}

	t.logger.Info("starting HTTP transport",
		"addr", t.config.Addr,
		"sse_endpoint", t.config.SSEEndpoint,
		"message_endpoint", t.config.MsgEndpoint,
		"auth_type", t.config.AuthType,
		"base_url", t.config.BaseURL,
		"tls_enabled", false,
		"force_https", t.config.ForceHTTPS)

	if t.config.ForceHTTPS {
		t.logger.Warn("HTTPS enforcement enabled but no TLS certificates provided - HTTP requests will be redirected")
	}

	t.mu.Unlock() // Release lock before blocking call
	return t.httpSrv.ListenAndServe()
}

// Shutdown gracefully stops the HTTP transport
func (t *HTTPTransport) Shutdown(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.httpSrv == nil {
		return nil
	}

	t.logger.Info("shutting down HTTP transport")

	// Shutdown SSE server first
	if err := t.sseServer.Shutdown(ctx); err != nil {
		t.logger.Error("failed to shutdown SSE server", "error", err)
	}

	// Then shutdown HTTP server
	err := t.httpSrv.Shutdown(ctx)
	t.httpSrv = nil
	return err
}

// GetBaseURL returns the configured base URL
func (t *HTTPTransport) GetBaseURL() string {
	return t.config.BaseURL
}

// GetConfig returns the transport configuration
func (t *HTTPTransport) GetConfig() HTTPTransportConfig {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.config
}
