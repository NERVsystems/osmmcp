package monitoring

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	// Service name for metrics
	ServiceName = "osmmcp"
)

var (
	// MCP request metrics
	MCPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "osmmcp_mcp_requests_total",
			Help: "Total number of MCP requests processed",
		},
		[]string{"tool", "status"},
	)

	MCPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "osmmcp_mcp_request_duration_seconds",
			Help:    "MCP request duration in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
		},
		[]string{"tool"},
	)

	// External service metrics
	ExternalServiceRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "osmmcp_external_service_requests_total",
			Help: "Total number of external service requests",
		},
		[]string{"service", "operation", "status"},
	)

	ExternalServiceRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "osmmcp_external_service_request_duration_seconds",
			Help:    "External service request duration in seconds",
			Buckets: []float64{0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0},
		},
		[]string{"service", "operation"},
	)

	// Rate limiting metrics
	RateLimitExceeded = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "osmmcp_rate_limit_exceeded_total",
			Help: "Total number of rate limit exceeded events",
		},
		[]string{"service"},
	)

	RateLimitWaitTime = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "osmmcp_rate_limit_wait_duration_seconds",
			Help:    "Time spent waiting for rate limits",
			Buckets: []float64{0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
		},
		[]string{"service"},
	)

	// Cache metrics
	CacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "osmmcp_cache_hits_total",
			Help: "Total number of cache hits",
		},
		[]string{"cache_type"},
	)

	CacheMisses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "osmmcp_cache_misses_total",
			Help: "Total number of cache misses",
		},
		[]string{"cache_type"},
	)

	CacheSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "osmmcp_cache_size",
			Help: "Current number of items in cache",
		},
		[]string{"cache_type"},
	)

	// Connection metrics
	ActiveConnections = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "osmmcp_active_connections",
			Help: "Number of active connections",
		},
		[]string{"transport", "type"},
	)

	// Error metrics
	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "osmmcp_errors_total",
			Help: "Total number of errors",
		},
		[]string{"component", "error_type"},
	)

	// System metrics
	SystemInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "osmmcp_system_info",
			Help: "System information",
		},
		[]string{"version", "go_version", "build_commit", "build_date"},
	)

	GoRoutines = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "osmmcp_goroutines",
			Help: "Number of goroutines",
		},
	)

	MemoryUsage = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "osmmcp_memory_usage_bytes",
			Help: "Memory usage in bytes",
		},
	)

	GCRuns = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "osmmcp_gc_runs_total",
			Help: "Total number of garbage collection runs",
		},
	)
)

// TransportInfo holds transport configuration and status
type TransportInfo struct {
	Type           string `json:"type"`                      // "http_streaming" or "stdio"
	HTTPAddr       string `json:"http_addr,omitempty"`       // HTTP address if enabled
	ActiveSessions int    `json:"active_sessions,omitempty"` // Active streaming sessions
}

// Service health and info structures
type ServiceHealth struct {
	Service       string                 `json:"service"`
	Version       string                 `json:"version"`
	Status        string                 `json:"status"` // "healthy", "degraded", "unhealthy"
	Uptime        time.Duration          `json:"uptime"`
	UptimeSeconds int64                  `json:"uptime_seconds"`       // Uptime in seconds for spec compliance
	StartTime     time.Time              `json:"start_time,omitempty"` // Optional field
	Connections   map[string]ConnStatus  `json:"connections"`
	Metrics       map[string]interface{} `json:"metrics,omitempty"`   // Optional field
	Transport     *TransportInfo         `json:"transport,omitempty"` // Transport info for monitoring
}

type ConnStatus struct {
	Status    string `json:"status"`               // "connected", "disconnected", "error"
	Latency   int64  `json:"latency_ms,omitempty"` // Optional latency in milliseconds
	LastError string `json:"last_error,omitempty"` // Last error message if any
}

// Helper functions for common metric updates
func RecordMCPRequest(tool string, duration time.Duration, success bool) {
	status := "success"
	if !success {
		status = "error"
	}
	MCPRequestsTotal.WithLabelValues(tool, status).Inc()
	MCPRequestDuration.WithLabelValues(tool).Observe(duration.Seconds())
}

func RecordExternalServiceRequest(service, operation string, duration time.Duration, success bool) {
	status := "success"
	if !success {
		status = "error"
	}
	ExternalServiceRequestsTotal.WithLabelValues(service, operation, status).Inc()
	ExternalServiceRequestDuration.WithLabelValues(service, operation).Observe(duration.Seconds())
}

func RecordCacheHit(cacheType string) {
	CacheHits.WithLabelValues(cacheType).Inc()
}

func RecordCacheMiss(cacheType string) {
	CacheMisses.WithLabelValues(cacheType).Inc()
}

func UpdateCacheSize(cacheType string, size int) {
	CacheSize.WithLabelValues(cacheType).Set(float64(size))
}

func RecordRateLimitExceeded(service string) {
	RateLimitExceeded.WithLabelValues(service).Inc()
}

func RecordRateLimitWait(service string, duration time.Duration) {
	RateLimitWaitTime.WithLabelValues(service).Observe(duration.Seconds())
}

func RecordError(component, errorType string) {
	ErrorsTotal.WithLabelValues(component, errorType).Inc()
}

func UpdateActiveConnections(transport, connType string, count int) {
	ActiveConnections.WithLabelValues(transport, connType).Set(float64(count))
}
