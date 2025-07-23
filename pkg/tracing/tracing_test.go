package tracing

import (
	"context"
	"os"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func TestInitTracing_NoEndpoint(t *testing.T) {
	// Ensure no OTLP endpoint is set
	oldEndpoint := os.Getenv("OTLP_ENDPOINT")
	os.Unsetenv("OTLP_ENDPOINT")
	defer func() {
		if oldEndpoint != "" {
			os.Setenv("OTLP_ENDPOINT", oldEndpoint)
		}
	}()

	ctx := context.Background()
	shutdown, err := InitTracing(ctx, "test-version")
	if err != nil {
		t.Fatalf("InitTracing failed: %v", err)
	}
	defer shutdown(ctx)

	// Verify we get a no-op tracer
	if Tracer == nil {
		t.Fatal("Tracer is nil")
	}

	// Test that operations work but are no-ops
	ctx, span := StartSpan(ctx, "test-span")
	if span == nil {
		t.Fatal("StartSpan returned nil span")
	}

	// These should not panic
	span.SetAttributes(attribute.String("test", "value"))
	span.RecordError(nil)
	span.SetStatus(codes.Ok, "test")
	span.End()
}

func TestInitTracing_WithEndpoint(t *testing.T) {
	// Skip if not in CI or if OTLP_ENDPOINT is not set for testing
	if os.Getenv("CI") == "" && os.Getenv("TEST_OTLP_ENDPOINT") == "" {
		t.Skip("Skipping OTLP test - set TEST_OTLP_ENDPOINT to run")
	}

	// Use test endpoint if available
	endpoint := os.Getenv("TEST_OTLP_ENDPOINT")
	if endpoint != "" {
		os.Setenv("OTLP_ENDPOINT", endpoint)
		defer os.Unsetenv("OTLP_ENDPOINT")
	}

	ctx := context.Background()
	shutdown, err := InitTracing(ctx, "test-version")
	if err != nil {
		t.Fatalf("InitTracing failed: %v", err)
	}
	defer shutdown(ctx)

	// Verify tracer is initialized
	if Tracer == nil {
		t.Fatal("Tracer is nil")
	}
}

func TestStartSpan(t *testing.T) {
	// Initialize with no-op tracer
	os.Unsetenv("OTLP_ENDPOINT")
	ctx := context.Background()
	shutdown, _ := InitTracing(ctx, "test")
	defer shutdown(ctx)

	// Test span creation
	ctx, span := StartSpan(ctx, "test-operation",
		trace.WithAttributes(
			attribute.String("test.key", "test-value"),
			attribute.Int("test.number", 42),
		),
	)

	if span == nil {
		t.Fatal("StartSpan returned nil span")
	}

	// Verify context has span
	ctxSpan := trace.SpanFromContext(ctx)
	if ctxSpan == nil {
		t.Fatal("No span in context")
	}

	span.End()
}

func TestRecordError(t *testing.T) {
	// Initialize with no-op tracer
	os.Unsetenv("OTLP_ENDPOINT")
	ctx := context.Background()
	shutdown, _ := InitTracing(ctx, "test")
	defer shutdown(ctx)

	ctx, span := StartSpan(ctx, "test-error")
	defer span.End()

	// Test recording an error - should not panic
	testErr := &testError{msg: "test error"}
	RecordError(ctx, testErr,
		trace.WithTimestamp(time.Now()),
		trace.WithAttributes(attribute.Bool("test", true)),
	)
}

func TestSetStatus(t *testing.T) {
	// Initialize with no-op tracer
	os.Unsetenv("OTLP_ENDPOINT")
	ctx := context.Background()
	shutdown, _ := InitTracing(ctx, "test")
	defer shutdown(ctx)

	ctx, span := StartSpan(ctx, "test-status")
	defer span.End()

	// Test setting status - should not panic
	SetStatus(ctx, codes.Error, "test error")
	SetStatus(ctx, codes.Ok, "test success")
}

func TestAddEvent(t *testing.T) {
	// Initialize with no-op tracer
	os.Unsetenv("OTLP_ENDPOINT")
	ctx := context.Background()
	shutdown, _ := InitTracing(ctx, "test")
	defer shutdown(ctx)

	ctx, span := StartSpan(ctx, "test-event")
	defer span.End()

	// Test adding events - should not panic
	AddEvent(ctx, "test-event-1",
		trace.WithAttributes(
			attribute.String("event.type", "test"),
			attribute.Int("event.value", 123),
		),
	)
	AddEvent(ctx, "test-event-2")
}

func TestSetAttributes(t *testing.T) {
	// Initialize with no-op tracer
	os.Unsetenv("OTLP_ENDPOINT")
	ctx := context.Background()
	shutdown, _ := InitTracing(ctx, "test")
	defer shutdown(ctx)

	ctx, span := StartSpan(ctx, "test-attributes")
	defer span.End()

	// Test setting attributes - should not panic
	SetAttributes(ctx,
		attribute.String("attr1", "value1"),
		attribute.Int("attr2", 42),
		attribute.Bool("attr3", true),
		attribute.Float64("attr4", 3.14),
		attribute.StringSlice("attr5", []string{"a", "b", "c"}),
	)
}

func TestAttributeHelpers(t *testing.T) {
	// Test MCPToolAttributes
	attrs := MCPToolAttributes("test-tool", "success", 123, 456)
	if len(attrs) != 4 {
		t.Errorf("MCPToolAttributes returned %d attributes, expected 4", len(attrs))
	}

	// Test ServiceAttributes
	attrs = ServiceAttributes("nominatim", "geocode", "https://example.com", 200)
	if len(attrs) != 4 {
		t.Errorf("ServiceAttributes returned %d attributes, expected 4", len(attrs))
	}

	// Test CacheAttributes
	attrs = CacheAttributes("osm", true, "test-key")
	if len(attrs) != 3 {
		t.Errorf("CacheAttributes returned %d attributes, expected 3", len(attrs))
	}

	// Test ErrorAttributes with nil error
	attrs = ErrorAttributes(nil)
	if len(attrs) != 0 {
		t.Errorf("ErrorAttributes with nil returned %d attributes, expected 0", len(attrs))
	}

	// Test ErrorAttributes with error
	attrs = ErrorAttributes(&testError{msg: "test error"})
	if len(attrs) != 2 {
		t.Errorf("ErrorAttributes returned %d attributes, expected 2", len(attrs))
	}
}

func TestEnvironmentDetection(t *testing.T) {
	// Test default environment
	oldEnv := os.Getenv("ENVIRONMENT")
	os.Unsetenv("ENVIRONMENT")
	env := getEnvironment()
	if env != "development" {
		t.Errorf("getEnvironment() = %s, expected 'development'", env)
	}

	// Test custom environment
	os.Setenv("ENVIRONMENT", "production")
	env = getEnvironment()
	if env != "production" {
		t.Errorf("getEnvironment() = %s, expected 'production'", env)
	}

	// Restore
	if oldEnv != "" {
		os.Setenv("ENVIRONMENT", oldEnv)
	} else {
		os.Unsetenv("ENVIRONMENT")
	}
}

// testError is a simple error type for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
