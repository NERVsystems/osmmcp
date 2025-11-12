package monitoring

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsInitialization(t *testing.T) {
	// Test that all metrics are properly registered
	metrics := []prometheus.Collector{
		MCPRequestsTotal,
		MCPRequestDuration,
		ExternalServiceRequestsTotal,
		ExternalServiceRequestDuration,
		RateLimitExceeded,
		RateLimitWaitTime,
		CacheHits,
		CacheMisses,
		CacheSize,
		ActiveConnections,
		ErrorsTotal,
		SystemInfo,
		GoRoutines,
		MemoryUsage,
		GCRuns,
	}

	for _, metric := range metrics {
		if metric == nil {
			t.Error("Metric is nil")
		}
	}
}

func TestRecordMCPRequest(t *testing.T) {
	// Clear any existing metrics
	MCPRequestsTotal.Reset()

	// Test successful request
	RecordMCPRequest("test_tool", 100*time.Millisecond, true)

	// Check counter
	if got := testutil.ToFloat64(MCPRequestsTotal.WithLabelValues("test_tool", "success")); got != 1 {
		t.Errorf("Expected 1 successful request, got %v", got)
	}

	// Test failed request
	RecordMCPRequest("test_tool", 200*time.Millisecond, false)

	// Check counter
	if got := testutil.ToFloat64(MCPRequestsTotal.WithLabelValues("test_tool", "error")); got != 1 {
		t.Errorf("Expected 1 failed request, got %v", got)
	}
}

func TestRecordExternalServiceRequest(t *testing.T) {
	// Clear any existing metrics
	ExternalServiceRequestsTotal.Reset()

	// Test successful request
	RecordExternalServiceRequest("nominatim", "geocode", 500*time.Millisecond, true)

	// Check counter
	if got := testutil.ToFloat64(ExternalServiceRequestsTotal.WithLabelValues("nominatim", "geocode", "success")); got != 1 {
		t.Errorf("Expected 1 successful external request, got %v", got)
	}

	// Test failed request
	RecordExternalServiceRequest("nominatim", "geocode", 300*time.Millisecond, false)

	// Check counter
	if got := testutil.ToFloat64(ExternalServiceRequestsTotal.WithLabelValues("nominatim", "geocode", "error")); got != 1 {
		t.Errorf("Expected 1 failed external request, got %v", got)
	}
}

func TestCacheMetrics(t *testing.T) {
	// Clear any existing metrics
	CacheHits.Reset()
	CacheMisses.Reset()
	CacheSize.Reset()

	// Test cache hit
	RecordCacheHit("test_cache")
	if got := testutil.ToFloat64(CacheHits.WithLabelValues("test_cache")); got != 1 {
		t.Errorf("Expected 1 cache hit, got %v", got)
	}

	// Test cache miss
	RecordCacheMiss("test_cache")
	if got := testutil.ToFloat64(CacheMisses.WithLabelValues("test_cache")); got != 1 {
		t.Errorf("Expected 1 cache miss, got %v", got)
	}

	// Test cache size update
	UpdateCacheSize("test_cache", 42)
	if got := testutil.ToFloat64(CacheSize.WithLabelValues("test_cache")); got != 42 {
		t.Errorf("Expected cache size 42, got %v", got)
	}
}

func TestRateLimitMetrics(t *testing.T) {
	// Clear any existing metrics
	RateLimitExceeded.Reset()
	RateLimitWaitTime.Reset()

	// Test rate limit exceeded
	RecordRateLimitExceeded("test_service")
	if got := testutil.ToFloat64(RateLimitExceeded.WithLabelValues("test_service")); got != 1 {
		t.Errorf("Expected 1 rate limit exceeded, got %v", got)
	}

	// Test rate limit wait time
	RecordRateLimitWait("test_service", 1*time.Second)
	// We can't easily test histogram values, but we can check that it doesn't panic
}

func TestErrorMetrics(t *testing.T) {
	// Clear any existing metrics
	ErrorsTotal.Reset()

	// Test error recording
	RecordError("test_component", "test_error")
	if got := testutil.ToFloat64(ErrorsTotal.WithLabelValues("test_component", "test_error")); got != 1 {
		t.Errorf("Expected 1 error, got %v", got)
	}
}

func TestUpdateActiveConnections(t *testing.T) {
	// Clear any existing metrics
	ActiveConnections.Reset()

	// Test connection update
	UpdateActiveConnections("http", "client", 5)
	if got := testutil.ToFloat64(ActiveConnections.WithLabelValues("http", "client")); got != 5 {
		t.Errorf("Expected 5 active connections, got %v", got)
	}
}

func BenchmarkRecordMCPRequest(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RecordMCPRequest("benchmark_tool", 100*time.Millisecond, true)
	}
}

func BenchmarkRecordExternalServiceRequest(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RecordExternalServiceRequest("benchmark_service", "benchmark_op", 100*time.Millisecond, true)
	}
}

func BenchmarkRecordCacheHit(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RecordCacheHit("benchmark_cache")
	}
}
