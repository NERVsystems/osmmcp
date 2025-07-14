package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/NERVsystems/osmmcp/pkg/version"
)

// HealthChecker manages service health monitoring
type HealthChecker struct {
	serviceName string
	version     string
	startTime   time.Time
	mu          sync.RWMutex
	connections map[string]*ConnStatus
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewHealthChecker creates a new health checker instance
func NewHealthChecker(serviceName, version string) *HealthChecker {
	ctx, cancel := context.WithCancel(context.Background())
	
	hc := &HealthChecker{
		serviceName: serviceName,
		version:     version,
		startTime:   time.Now(),
		connections: make(map[string]*ConnStatus),
		ctx:         ctx,
		cancel:      cancel,
	}
	
	// Start system metrics collection
	go hc.collectSystemMetrics()
	
	return hc
}

// UpdateConnection updates the status of a connection
func (h *HealthChecker) UpdateConnection(name, status string, latencyMs int64, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	
	h.connections[name] = &ConnStatus{
		Name:    name,
		Status:  status,
		Latency: latencyMs,
		Error:   errStr,
	}
}

// RemoveConnection removes a connection from monitoring
func (h *HealthChecker) RemoveConnection(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.connections, name)
}

// GetHealth returns the current health status
func (h *HealthChecker) GetHealth() ServiceHealth {
	h.mu.RLock()
	defer h.mu.RUnlock()
	
	// Determine overall status
	status := "healthy"
	degradedCount := 0
	errorCount := 0
	
	for _, conn := range h.connections {
		switch conn.Status {
		case "error", "disconnected":
			errorCount++
		case "degraded":
			degradedCount++
		}
	}
	
	// Status logic: healthy -> degraded -> unhealthy
	if errorCount > 0 {
		if errorCount > len(h.connections)/2 { // More than half are in error
			status = "unhealthy"
		} else {
			status = "degraded"
		}
	} else if degradedCount > 0 {
		status = "degraded"
	}
	
	// Copy connections to avoid race conditions
	connections := make(map[string]ConnStatus)
	for k, v := range h.connections {
		connections[k] = *v
	}
	
	// Gather runtime metrics
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	versionInfo := version.Info()
	
	return ServiceHealth{
		Service:   h.serviceName,
		Version:   h.version,
		Status:    status,
		Uptime:    time.Since(h.startTime),
		StartTime: h.startTime,
		Connections: connections,
		Metrics: map[string]interface{}{
			"goroutines":       runtime.NumGoroutine(),
			"memory_alloc_mb":  m.Alloc / 1024 / 1024,
			"memory_sys_mb":    m.Sys / 1024 / 1024,
			"gc_runs":          m.NumGC,
			"cpu_count":        runtime.NumCPU(),
			"version_info":     versionInfo,
			"total_connections": len(h.connections),
			"error_connections": errorCount,
			"degraded_connections": degradedCount,
		},
	}
}

// HealthHandler returns an HTTP handler for health checks
func (h *HealthChecker) HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		health := h.GetHealth()
		
		w.Header().Set("Content-Type", "application/json")
		
		// Set appropriate HTTP status based on health
		switch health.Status {
		case "healthy":
			w.WriteHeader(http.StatusOK)
		case "degraded":
			w.WriteHeader(http.StatusOK) // Still serving but with warnings
		case "unhealthy":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
		
		if err := json.NewEncoder(w).Encode(health); err != nil {
			http.Error(w, fmt.Sprintf("Failed to encode health response: %v", err), http.StatusInternalServerError)
		}
	}
}

// ReadinessHandler returns a simple readiness check
func (h *HealthChecker) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		health := h.GetHealth()
		
		w.Header().Set("Content-Type", "application/json")
		
		if health.Status == "unhealthy" {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		
		response := map[string]interface{}{
			"ready":  health.Status != "unhealthy",
			"status": health.Status,
		}
		
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, fmt.Sprintf("Failed to encode readiness response: %v", err), http.StatusInternalServerError)
		}
	}
}

// LivenessHandler returns a simple liveness check
func (h *HealthChecker) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		
		response := map[string]interface{}{
			"alive":  true,
			"uptime": time.Since(h.startTime).String(),
		}
		
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, fmt.Sprintf("Failed to encode liveness response: %v", err), http.StatusInternalServerError)
		}
	}
}

// collectSystemMetrics periodically collects and updates system metrics
func (h *HealthChecker) collectSystemMetrics() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			h.updateSystemMetrics()
		}
	}
}

// updateSystemMetrics updates Prometheus metrics with current system state
func (h *HealthChecker) updateSystemMetrics() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	// Update system metrics
	GoRoutines.Set(float64(runtime.NumGoroutine()))
	MemoryUsage.Set(float64(m.Alloc))
	GCRuns.Set(float64(m.NumGC))
	
	// Update version info if not already set
	versionInfo := version.Info()
	SystemInfo.WithLabelValues(
		versionInfo["version"],
		versionInfo["go_version"],
		versionInfo["commit"],
		versionInfo["build_date"],
	).Set(1)
}

// Shutdown gracefully shuts down the health checker
func (h *HealthChecker) Shutdown() {
	h.cancel()
}

// ConnectionMonitor helps monitor external service connections
type ConnectionMonitor struct {
	name         string
	healthChecker *HealthChecker
	checkFunc    func() error
	interval     time.Duration
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewConnectionMonitor creates a new connection monitor
func NewConnectionMonitor(name string, hc *HealthChecker, checkFunc func() error, interval time.Duration) *ConnectionMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &ConnectionMonitor{
		name:         name,
		healthChecker: hc,
		checkFunc:    checkFunc,
		interval:     interval,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start begins monitoring the connection
func (cm *ConnectionMonitor) Start() {
	go cm.monitor()
}

// Stop stops monitoring the connection
func (cm *ConnectionMonitor) Stop() {
	cm.cancel()
}

// monitor runs the connection monitoring loop
func (cm *ConnectionMonitor) monitor() {
	// Initial check
	cm.performCheck()
	
	ticker := time.NewTicker(cm.interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-cm.ctx.Done():
			return
		case <-ticker.C:
			cm.performCheck()
		}
	}
}

// performCheck executes the health check and updates status
func (cm *ConnectionMonitor) performCheck() {
	start := time.Now()
	err := cm.checkFunc()
	latency := time.Since(start).Milliseconds()
	
	status := "connected"
	if err != nil {
		status = "error"
	}
	
	cm.healthChecker.UpdateConnection(cm.name, status, latency, err)
}