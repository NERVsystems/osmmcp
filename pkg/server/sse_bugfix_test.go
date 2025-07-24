package server

import (
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

// TestSSEBugFix_StreamingUnsupported verifies the critical SSE bug is fixed
// Bug: SSE endpoint returned "Streaming unsupported" with HTTP 500
// Root cause: responseWriter wrapper didn't implement http.Flusher interface
func TestSSEBugFix_StreamingUnsupported(t *testing.T) {
	// Create MCP server
	mcpServer := mcpserver.NewMCPServer("test-server", "1.0.0")

	// Create HTTP transport
	config := DefaultHTTPTransportConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	transport := NewHTTPTransport(mcpServer, config, logger)

	// Apply the same middleware stack as production
	handler := http.Handler(transport.mux)
	handler = TracingMiddleware()(handler)
	handler = LoggingMiddleware(logger)(handler)
	handler = SecurityHeaders(handler)
	handler = RequestSizeLimiter(10 * 1024 * 1024)(handler)

	// Create test server
	server := httptest.NewServer(handler)
	defer server.Close()

	// Make SSE request
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

	// Check that we don't get the bug symptoms
	if resp.StatusCode == http.StatusInternalServerError {
		body, _ := io.ReadAll(resp.Body)
		if strings.Contains(string(body), "Streaming unsupported") {
			t.Fatal("BUG NOT FIXED: SSE endpoint still returns 'Streaming unsupported' with HTTP 500")
		}
		t.Fatalf("SSE endpoint returned HTTP 500: %s", string(body))
	}

	// Verify successful SSE response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected HTTP 200, got %d: %s", resp.StatusCode, string(body))
	}

	// Verify SSE headers
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/event-stream") {
		t.Errorf("Expected Content-Type: text/event-stream, got: %s", contentType)
	}

	if resp.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("Expected Cache-Control: no-cache, got: %s", resp.Header.Get("Cache-Control"))
	}

	t.Log("✓ SSE bug is fixed: endpoint returns proper SSE response")
}

// TestResponseWriterPreservesInterfaces verifies our fix preserves all necessary interfaces
func TestResponseWriterPreservesInterfaces(t *testing.T) {
	// Create a test handler that checks for Flusher
	var flusherAvailable bool
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, flusherAvailable = w.(http.Flusher)
		w.WriteHeader(http.StatusOK)
	})

	// Apply our middleware
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := http.Handler(testHandler)
	handler = LoggingMiddleware(logger)(handler)

	// Create test request
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	// Execute request
	handler.ServeHTTP(rec, req)

	// Verify Flusher was available
	if !flusherAvailable {
		t.Fatal("http.Flusher interface not preserved through middleware")
	}
}

// TestSSEEndpointStressTest performs a stress test on the SSE endpoint
func TestSSEEndpointStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	// Create MCP server
	mcpServer := mcpserver.NewMCPServer("test-server", "1.0.0")

	// Create HTTP transport
	config := DefaultHTTPTransportConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	transport := NewHTTPTransport(mcpServer, config, logger)

	// Apply full middleware stack
	handler := http.Handler(transport.mux)
	handler = TracingMiddleware()(handler)
	handler = LoggingMiddleware(logger)(handler)
	handler = SecurityHeaders(handler)
	handler = RequestSizeLimiter(10 * 1024 * 1024)(handler)

	// Create test server
	server := httptest.NewServer(handler)
	defer server.Close()

	// Number of concurrent connections
	numConnections := 50
	results := make(chan error, numConnections)

	// Start concurrent SSE connections
	for i := 0; i < numConnections; i++ {
		go func(id int) {
			req, err := http.NewRequest("GET", server.URL+"/sse", nil)
			if err != nil {
				results <- err
				return
			}
			req.Header.Set("Accept", "text/event-stream")

			client := &http.Client{
				Timeout: 10 * time.Second,
			}
			resp, err := client.Do(req)
			if err != nil {
				results <- err
				return
			}
			defer resp.Body.Close()

			// Check for the bug
			if resp.StatusCode == http.StatusInternalServerError {
				body, _ := io.ReadAll(resp.Body)
				if strings.Contains(string(body), "Streaming unsupported") {
					results <- fmt.Errorf("Connection %d: BUG - Streaming unsupported", id)
					return
				}
			}

			if resp.StatusCode != http.StatusOK {
				results <- fmt.Errorf("Connection %d: Expected 200, got %d", id, resp.StatusCode)
				return
			}

			results <- nil
		}(i)
	}

	// Collect results
	failures := 0
	for i := 0; i < numConnections; i++ {
		if err := <-results; err != nil {
			t.Error(err)
			failures++
		}
	}

	if failures > 0 {
		t.Errorf("Stress test failed: %d/%d connections failed", failures, numConnections)
	} else {
		t.Logf("✓ Stress test passed: all %d concurrent SSE connections succeeded", numConnections)
	}
}