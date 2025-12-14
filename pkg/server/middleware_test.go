package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/NERVsystems/osmmcp/pkg/tracing"
)

func TestTracingMiddleware(t *testing.T) {
	// Initialize tracing with no-op tracer
	os.Unsetenv("OTLP_ENDPOINT")
	ctx := context.Background()
	shutdown, _ := tracing.InitTracing(ctx, "test")
	defer shutdown(ctx)

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify context has a span
		span := trace.SpanFromContext(r.Context())
		if span == nil {
			t.Error("No span in request context")
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	})

	// Wrap with tracing middleware
	handler := TracingMiddleware()(testHandler)

	// Test successful request
	t.Run("Success", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test/path?sessionId=123", nil)
		req.Header.Set("User-Agent", "test-agent")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rec.Code)
		}
	})

	// Test error response
	t.Run("Error", func(t *testing.T) {
		errorHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("error"))
		})

		handler := TracingMiddleware()(errorHandler)

		req := httptest.NewRequest("POST", "/error", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("Expected status 500, got %d", rec.Code)
		}
	})

	// Test session ID extraction
	t.Run("SessionID", func(t *testing.T) {
		// Test query parameter
		req := httptest.NewRequest("GET", "/test?sessionId=query-123", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		// Test header
		req = httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Session-ID", "header-456")
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	})
}
