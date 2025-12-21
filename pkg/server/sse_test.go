package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"log/slog"

	mcpserver "github.com/mark3labs/mcp-go/server"
)

// TestResponseWriterInterfaces tests that our responseWriter properly implements all interfaces
func TestResponseWriterInterfaces(t *testing.T) {
	// Test with a regular ResponseWriter
	recorder := httptest.NewRecorder()
	wrapped := newResponseWriter(recorder)

	// Test Flusher interface
	var _ http.Flusher = wrapped // Compile-time check

	// Test Hijacker interface
	var _ http.Hijacker = wrapped // Compile-time check

	// Test Pusher interface
	var _ http.Pusher = wrapped // Compile-time check

	// Test that Flush works
	wrapped.Flush() // Should not panic

	// Test that Hijack returns ErrNotSupported for regular ResponseWriter
	_, _, err := wrapped.Hijack()
	if err != http.ErrNotSupported {
		t.Errorf("Hijack should return ErrNotSupported, got %v", err)
	}

	// Test that Push returns ErrNotSupported for regular ResponseWriter
	err = wrapped.Push("/test", nil)
	if err != http.ErrNotSupported {
		t.Errorf("Push should return ErrNotSupported, got %v", err)
	}
}

// TestSSEEndpointBasic tests basic SSE functionality via the streamable-http /mcp endpoint
func TestSSEEndpointBasic(t *testing.T) {
	// Skip: In MCP SDK v0.40.0+ with streamable-http transport, SSE is part of /mcp endpoint
	// The /sse endpoint no longer exists as a separate route
	t.Skip("Skipping: streamable-http transport uses /mcp endpoint for SSE, not /sse")
}

// TestSSEEndpointWithMiddleware tests SSE with all middleware applied
func TestSSEEndpointWithMiddleware(t *testing.T) {
	// Skip: In MCP SDK v0.40.0+ with streamable-http transport, SSE is part of /mcp endpoint
	t.Skip("Skipping: streamable-http transport uses /mcp endpoint for SSE, not /sse")
}

// TestSSEEndpointAuthentication tests SSE with authentication
func TestSSEEndpointAuthentication(t *testing.T) {
	// Skip: In MCP SDK v0.40.0+ with streamable-http transport, SSE is part of /mcp endpoint
	// Authentication tests for /mcp endpoint are handled separately
	t.Skip("Skipping: streamable-http transport uses /mcp endpoint for SSE, not /sse")
}

// TestSSEEndpointHTTPSEnforcement tests HTTPS enforcement
func TestSSEEndpointHTTPSEnforcement(t *testing.T) {
	// Create MCP server
	mcpServer := mcpserver.NewMCPServer("test-server", "1.0.0")

	// Create HTTP transport with HTTPS enforcement but no TLS (should fail on Start)
	config := DefaultHTTPTransportConfig()
	config.ForceHTTPS = true
	// No TLS certificates provided

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	transport := NewHTTPTransport(mcpServer, config, logger)

	// Start should fail
	err := transport.Start()
	if err == nil {
		t.Fatal("Expected Start to fail with ForceHTTPS and no TLS certificates")
	}

	// Now test with ForceHTTPS and mock TLS
	config.ForceHTTPS = true
	config.TLSCertFile = "" // Still empty, but we'll test the redirect behavior
	config.TLSKeyFile = ""

	transport2 := NewHTTPTransport(mcpServer, config, logger)
	server := httptest.NewServer(transport2.mux)
	defer server.Close()

	// Test redirect behavior
	req, _ := http.NewRequest("GET", server.URL+"/sse", nil)
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Should get redirect
	if resp.StatusCode != http.StatusMovedPermanently {
		t.Errorf("Expected 301 redirect, got %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if !strings.HasPrefix(location, "https://") {
		t.Errorf("Expected HTTPS redirect, got %s", location)
	}
}

// TestSSEStreamingData tests actual SSE data streaming
func TestSSEStreamingData(t *testing.T) {
	// Skip: In MCP SDK v0.40.0+ with streamable-http transport, SSE is part of /mcp endpoint
	t.Skip("Skipping: streamable-http transport uses /mcp endpoint for SSE, not /sse")
}

// TestSSEEndpointConcurrency tests concurrent SSE connections
func TestSSEEndpointConcurrency(t *testing.T) {
	// Skip: In MCP SDK v0.40.0+ with streamable-http transport, SSE is part of /mcp endpoint
	t.Skip("Skipping: streamable-http transport uses /mcp endpoint for SSE, not /sse")
}

// TestServiceDiscoveryEndpoint tests the root endpoint
func TestServiceDiscoveryEndpoint(t *testing.T) {
	mcpServer := mcpserver.NewMCPServer("test-server", "1.0.0")
	config := DefaultHTTPTransportConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	transport := NewHTTPTransport(mcpServer, config, logger)

	server := httptest.NewServer(transport.mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("Failed to get service discovery: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	var discovery map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check required fields
	if discovery["service"] != "mcp-server" {
		t.Errorf("Expected service=mcp-server, got %v", discovery["service"])
	}

	// MCP SDK v0.40.0+ uses streamable-http transport
	if discovery["transport"] != "streamable-http" {
		t.Errorf("Expected transport=streamable-http, got %v", discovery["transport"])
	}

	endpoints, ok := discovery["endpoints"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected endpoints to be a map")
	}

	// MCP SDK v0.40.0+ uses /mcp endpoint for all MCP operations
	mcpEndpoint, ok := endpoints["mcp"].(string)
	if !ok {
		t.Fatal("Expected mcp endpoint to be a string")
	}
	if !strings.HasSuffix(mcpEndpoint, "/mcp") {
		t.Errorf("Expected MCP endpoint to end with /mcp, got %s", mcpEndpoint)
	}
}

// TestHealthEndpoints tests all health check endpoints
func TestHealthEndpoints(t *testing.T) {
	mcpServer := mcpserver.NewMCPServer("test-server", "1.0.0")
	config := DefaultHTTPTransportConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	transport := NewHTTPTransport(mcpServer, config, logger)

	server := httptest.NewServer(transport.mux)
	defer server.Close()

	endpoints := []string{"/health", "/ready", "/live"}

	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			resp, err := http.Get(server.URL + endpoint)
			if err != nil {
				t.Fatalf("Failed to get %s: %v", endpoint, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("Expected 200 for %s, got %d", endpoint, resp.StatusCode)
			}

			var body map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("Failed to decode %s response: %v", endpoint, err)
			}

			// Basic validation
			if endpoint == "/health" && body["status"] != "ok" {
				t.Errorf("Expected status=ok for health endpoint")
			}
		})
	}
}

// TestMiddlewareIntegration ensures all middleware work together
func TestMiddlewareIntegration(t *testing.T) {
	// Create a handler that checks if Flusher is available
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if Flusher is available after middleware wrapping
		if _, ok := w.(http.Flusher); !ok {
			http.Error(w, "Flusher not available", http.StatusInternalServerError)
			return
		}

		// Write response
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))

		// Try to flush
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})

	// Apply all middleware
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := http.Handler(testHandler)
	handler = TracingMiddleware()(handler)
	handler = LoggingMiddleware(logger)(handler)
	handler = SecurityHeaders(handler)
	handler = RequestSizeLimiter(1024)(handler)

	// Create test server
	server := httptest.NewServer(handler)
	defer server.Close()

	// Send request
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Should succeed
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 200, got %d. Body: %s", resp.StatusCode, string(body))
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "OK" {
		t.Errorf("Expected body 'OK', got %s", string(body))
	}
}
