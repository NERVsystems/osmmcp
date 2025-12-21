package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"log/slog"
)

// TestSSEBugFix_StreamingUnsupported verifies the critical SSE bug is fixed
// Bug: SSE endpoint returned "Streaming unsupported" with HTTP 500
// Root cause: responseWriter wrapper didn't implement http.Flusher interface
// Note: In MCP SDK v0.40.0+, SSE is now part of the /mcp endpoint (streamable-http transport)
func TestSSEBugFix_StreamingUnsupported(t *testing.T) {
	// Skip: In MCP SDK v0.40.0+ with streamable-http transport, SSE is part of /mcp endpoint
	// The /sse endpoint no longer exists as a separate route
	t.Skip("Skipping: streamable-http transport uses /mcp endpoint for SSE, not /sse")
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
	// Skip: In MCP SDK v0.40.0+ with streamable-http transport, SSE is part of /mcp endpoint
	t.Skip("Skipping: streamable-http transport uses /mcp endpoint for SSE, not /sse")
}
