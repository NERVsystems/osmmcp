package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// TestSSEEndpointBasic tests basic SSE functionality
func TestSSEEndpointBasic(t *testing.T) {
	// Create a simple MCP server for testing
	mcpServer := mcpserver.NewMCPServer("test-server", "1.0.0")

	// Create HTTP transport with default config
	config := DefaultHTTPTransportConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	transport := NewHTTPTransport(mcpServer, config, logger)

	// Create test server
	server := httptest.NewServer(transport.mux)
	defer server.Close()

	// Test SSE endpoint
	resp, err := http.Get(server.URL + "/sse")
	if err != nil {
		t.Fatalf("Failed to connect to SSE endpoint: %v", err)
	}
	defer resp.Body.Close()

	// Check headers
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d. Body: %s", resp.StatusCode, string(body))
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/event-stream") {
		t.Errorf("Expected Content-Type to start with 'text/event-stream', got %s", contentType)
	}

	if resp.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("Expected Cache-Control: no-cache, got %s", resp.Header.Get("Cache-Control"))
	}
}

// TestSSEEndpointWithMiddleware tests SSE with all middleware applied
func TestSSEEndpointWithMiddleware(t *testing.T) {
	// Create a simple MCP server for testing
	mcpServer := mcpserver.NewMCPServer("test-server", "1.0.0")

	// Create HTTP transport with default config
	config := DefaultHTTPTransportConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	transport := NewHTTPTransport(mcpServer, config, logger)

	// Apply middleware stack similar to production
	handler := http.Handler(transport.mux)
	handler = TracingMiddleware()(handler)
	handler = LoggingMiddleware(logger)(handler)
	handler = SecurityHeaders(handler)
	handler = RequestSizeLimiter(10 * 1024 * 1024)(handler)

	// Create test server with middleware
	server := httptest.NewServer(handler)
	defer server.Close()

	// Test SSE endpoint through middleware
	req, err := http.NewRequest("GET", server.URL+"/sse", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to connect to SSE endpoint: %v", err)
	}
	defer resp.Body.Close()

	// Should work with middleware
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d. Body: %s", resp.StatusCode, string(body))
	}

	// Read initial events
	reader := bufio.NewReader(resp.Body)
	eventCount := 0
	timeout := time.After(3 * time.Second)

	for eventCount < 2 {
		select {
		case <-timeout:
			if eventCount == 0 {
				t.Fatal("Timeout waiting for SSE events")
			}
			return // Got at least one event
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF && eventCount > 0 {
					return // Connection closed after receiving events
				}
				t.Fatalf("Error reading SSE stream: %v", err)
			}

			// Count actual event lines (starting with "event:" or "data:")
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "event:") || strings.HasPrefix(trimmed, "data:") {
				eventCount++
				t.Logf("Received SSE line: %s", trimmed)
			}
		}
	}
}

// TestSSEEndpointAuthentication tests SSE with authentication
func TestSSEEndpointAuthentication(t *testing.T) {
	tests := []struct {
		name       string
		authType   string
		authToken  string
		authHeader string
		expectCode int
	}{
		{
			name:       "No auth required",
			authType:   "none",
			authToken:  "",
			authHeader: "",
			expectCode: http.StatusOK,
		},
		{
			name:       "Bearer auth success",
			authType:   "bearer",
			authToken:  "test-token-123",
			authHeader: "Bearer test-token-123",
			expectCode: http.StatusOK,
		},
		{
			name:       "Bearer auth failure",
			authType:   "bearer",
			authToken:  "test-token-123",
			authHeader: "Bearer wrong-token",
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "Bearer auth missing",
			authType:   "bearer",
			authToken:  "test-token-123",
			authHeader: "",
			expectCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create MCP server
			mcpServer := mcpserver.NewMCPServer("test-server", "1.0.0")

			// Create HTTP transport with auth config
			config := DefaultHTTPTransportConfig()
			config.AuthType = tt.authType
			config.AuthToken = tt.authToken

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			transport := NewHTTPTransport(mcpServer, config, logger)

			// Create test server
			server := httptest.NewServer(transport.mux)
			defer server.Close()

			// Create request
			req, err := http.NewRequest("GET", server.URL+"/sse", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			req.Header.Set("Accept", "text/event-stream")

			// Send request
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Failed to send request: %v", err)
			}
			defer resp.Body.Close()

			// Check status code
			if resp.StatusCode != tt.expectCode {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectCode, resp.StatusCode, string(body))
			}
		})
	}
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
	// This test simulates the MCP protocol SSE session
	mcpServer := mcpserver.NewMCPServer("test-server", "1.0.0")

	config := DefaultHTTPTransportConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	transport := NewHTTPTransport(mcpServer, config, logger)

	server := httptest.NewServer(transport.mux)
	defer server.Close()

	// Connect to SSE
	req, _ := http.NewRequest("GET", server.URL+"/sse", nil)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 200, got %d. Body: %s", resp.StatusCode, string(body))
	}

	// Parse SSE events
	reader := bufio.NewReader(resp.Body)
	events := make([]string, 0)
	
	// Read a few events with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				line, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "data:") {
					events = append(events, strings.TrimSpace(line))
				}
			}
		}
	}()

	// Wait for timeout
	<-ctx.Done()

	// Should have received some events
	if len(events) == 0 {
		t.Fatal("No SSE events received")
	}

	t.Logf("Received %d SSE events", len(events))
	for i, event := range events {
		t.Logf("Event %d: %s", i, event)
	}
}

// TestSSEEndpointConcurrency tests concurrent SSE connections
func TestSSEEndpointConcurrency(t *testing.T) {
	mcpServer := mcpserver.NewMCPServer("test-server", "1.0.0")
	config := DefaultHTTPTransportConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	transport := NewHTTPTransport(mcpServer, config, logger)

	server := httptest.NewServer(transport.mux)
	defer server.Close()

	// Start multiple concurrent SSE connections
	numConnections := 5
	errChan := make(chan error, numConnections)

	for i := 0; i < numConnections; i++ {
		go func(id int) {
			req, _ := http.NewRequest("GET", server.URL+"/sse", nil)
			req.Header.Set("Accept", "text/event-stream")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				errChan <- fmt.Errorf("connection %d failed: %v", id, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				errChan <- fmt.Errorf("connection %d got status %d: %s", id, resp.StatusCode, string(body))
				return
			}

			// Successfully connected
			errChan <- nil
		}(i)
	}

	// Wait for all connections
	for i := 0; i < numConnections; i++ {
		if err := <-errChan; err != nil {
			t.Error(err)
		}
	}
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

	if discovery["transport"] != "HTTP+SSE" {
		t.Errorf("Expected transport=HTTP+SSE, got %v", discovery["transport"])
	}

	endpoints, ok := discovery["endpoints"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected endpoints to be a map")
	}

	if !strings.HasSuffix(endpoints["sse"].(string), "/sse") {
		t.Errorf("Expected SSE endpoint to end with /sse, got %s", endpoints["sse"])
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