// Package tracing provides OpenTelemetry tracing capabilities for osmmcp
package tracing

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const (
	// ServiceName is the name of the service in traces
	ServiceName = "osmmcp"
	// TracerName is the name of the tracer
	TracerName = "github.com/NERVsystems/osmmcp"
)

// Tracer is the global tracer instance
var Tracer trace.Tracer = noop.NewTracerProvider().Tracer(TracerName)

// InitTracing initializes OpenTelemetry tracing with OTLP exporter
func InitTracing(ctx context.Context, version string) (shutdown func(context.Context) error, err error) {
	// Check if OTLP endpoint is configured
	endpoint := os.Getenv("OTLP_ENDPOINT")
	if endpoint == "" {
		// Use no-op tracer if no endpoint configured
		Tracer = noop.NewTracerProvider().Tracer(TracerName)
		return func(ctx context.Context) error { return nil }, nil
	}

	// Create OTLP exporter
	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(), // TODO: Add TLS support
	)

	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP trace exporter: %w", err)
	}

	// Create resource with service information
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(ServiceName),
			semconv.ServiceVersion(version),
			attribute.String("service.environment", getEnvironment()),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	// Create tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()), // TODO: Make configurable
	)

	// Set global tracer provider
	otel.SetTracerProvider(tp)
	
	// Set global propagator
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// Get tracer
	Tracer = tp.Tracer(TracerName)

	// Return shutdown function
	return func(ctx context.Context) error {
		// Shutdown with 5 second timeout
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return tp.Shutdown(shutdownCtx)
	}, nil
}

// getEnvironment returns the environment name
func getEnvironment() string {
	if env := os.Getenv("ENVIRONMENT"); env != "" {
		return env
	}
	return "development"
}

// StartSpan starts a new span with common attributes
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return Tracer.Start(ctx, name, opts...)
}

// RecordError records an error on the span from context
func RecordError(ctx context.Context, err error, opts ...trace.EventOption) {
	span := trace.SpanFromContext(ctx)
	if span != nil && span.IsRecording() {
		span.RecordError(err, opts...)
	}
}

// SetStatus sets the status of the span from context
func SetStatus(ctx context.Context, code codes.Code, description string) {
	span := trace.SpanFromContext(ctx)
	if span != nil && span.IsRecording() {
		span.SetStatus(code, description)
	}
}

// AddEvent adds an event to the span from context
func AddEvent(ctx context.Context, name string, opts ...trace.EventOption) {
	span := trace.SpanFromContext(ctx)
	if span != nil && span.IsRecording() {
		span.AddEvent(name, opts...)
	}
}

// SetAttributes sets attributes on the span from context
func SetAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span != nil && span.IsRecording() {
		span.SetAttributes(attrs...)
	}
}