package monitoring

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewHealthChecker(t *testing.T) {
	hc := NewHealthChecker("test-service", "1.0.0")
	defer hc.Shutdown()

	if hc.serviceName != "test-service" {
		t.Errorf("Expected service name 'test-service', got %s", hc.serviceName)
	}

	if hc.version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got %s", hc.version)
	}

	if hc.connections == nil {
		t.Error("Connections map should be initialized")
	}

	if hc.ctx == nil {
		t.Error("Context should be initialized")
	}
}

func TestUpdateConnection(t *testing.T) {
	hc := NewHealthChecker("test-service", "1.0.0")
	defer hc.Shutdown()

	// Test updating a connection
	hc.UpdateConnection("test-conn", "connected", 100, nil)

	hc.mu.RLock()
	conn, exists := hc.connections["test-conn"]
	hc.mu.RUnlock()

	if !exists {
		t.Error("Connection should exist")
	}

	if conn.Status != "connected" {
		t.Errorf("Expected status 'connected', got %s", conn.Status)
	}

	if conn.Latency != 100 {
		t.Errorf("Expected latency 100, got %d", conn.Latency)
	}

	if conn.LastError != "" {
		t.Errorf("Expected no error, got %s", conn.LastError)
	}
}

func TestUpdateConnectionWithError(t *testing.T) {
	hc := NewHealthChecker("test-service", "1.0.0")
	defer hc.Shutdown()

	// Test updating a connection with error
	testErr := errors.New("test error")
	hc.UpdateConnection("test-conn", "error", 200, testErr)

	hc.mu.RLock()
	conn, exists := hc.connections["test-conn"]
	hc.mu.RUnlock()

	if !exists {
		t.Error("Connection should exist")
	}

	if conn.Status != "error" {
		t.Errorf("Expected status 'error', got %s", conn.Status)
	}

	if conn.LastError != "test error" {
		t.Errorf("Expected error 'test error', got %s", conn.LastError)
	}
}

func TestRemoveConnection(t *testing.T) {
	hc := NewHealthChecker("test-service", "1.0.0")
	defer hc.Shutdown()

	// Add a connection
	hc.UpdateConnection("test-conn", "connected", 100, nil)

	// Remove the connection
	hc.RemoveConnection("test-conn")

	hc.mu.RLock()
	_, exists := hc.connections["test-conn"]
	hc.mu.RUnlock()

	if exists {
		t.Error("Connection should not exist after removal")
	}
}

func TestGetHealthStatus(t *testing.T) {
	hc := NewHealthChecker("test-service", "1.0.0")
	defer hc.Shutdown()

	// Test healthy status with no connections
	health := hc.GetHealth()
	if health.Status != "healthy" {
		t.Errorf("Expected status 'healthy', got %s", health.Status)
	}

	// Add a healthy connection
	hc.UpdateConnection("conn1", "connected", 100, nil)
	health = hc.GetHealth()
	if health.Status != "healthy" {
		t.Errorf("Expected status 'healthy', got %s", health.Status)
	}

	// Add a degraded connection
	hc.UpdateConnection("conn2", "degraded", 200, nil)
	health = hc.GetHealth()
	if health.Status != "degraded" {
		t.Errorf("Expected status 'degraded', got %s", health.Status)
	}

	// Add an error connection
	hc.UpdateConnection("conn3", "error", 300, errors.New("test error"))
	health = hc.GetHealth()
	if health.Status != "degraded" {
		t.Errorf("Expected status 'degraded', got %s", health.Status)
	}

	// Add another error connection (majority in error = unhealthy)
	hc.UpdateConnection("conn4", "disconnected", 400, errors.New("disconnected"))
	hc.UpdateConnection("conn5", "error", 500, errors.New("another error")) // 3/5 in error
	health = hc.GetHealth()
	if health.Status != "unhealthy" {
		t.Errorf("Expected status 'unhealthy', got %s", health.Status)
	}
}

func TestGetHealthFields(t *testing.T) {
	hc := NewHealthChecker("test-service", "1.0.0")
	defer hc.Shutdown()

	health := hc.GetHealth()

	// Test required fields
	if health.Service != "test-service" {
		t.Errorf("Expected service 'test-service', got %s", health.Service)
	}

	if health.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got %s", health.Version)
	}

	if health.Uptime < 0 {
		t.Error("Uptime should not be negative")
	}

	if health.StartTime.IsZero() {
		t.Error("StartTime should not be zero")
	}

	if health.Connections == nil {
		t.Error("Connections should not be nil")
	}

	if health.Metrics == nil {
		t.Error("Metrics should not be nil")
	}

	// Test metrics fields
	if _, exists := health.Metrics["goroutines"]; !exists {
		t.Error("Metrics should contain goroutines")
	}

	if _, exists := health.Metrics["memory_alloc_mb"]; !exists {
		t.Error("Metrics should contain memory_alloc_mb")
	}

	if _, exists := health.Metrics["cpu_count"]; !exists {
		t.Error("Metrics should contain cpu_count")
	}
}

func TestHealthHandler(t *testing.T) {
	hc := NewHealthChecker("test-service", "1.0.0")
	defer hc.Shutdown()

	handler := hc.HealthHandler()

	// Test healthy status
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got %s", contentType)
	}

	var health ServiceHealth
	if err := json.NewDecoder(w.Body).Decode(&health); err != nil {
		t.Errorf("Failed to decode health response: %v", err)
	}

	if health.Status != "healthy" {
		t.Errorf("Expected status 'healthy', got %s", health.Status)
	}
}

func TestHealthHandlerUnhealthy(t *testing.T) {
	hc := NewHealthChecker("test-service", "1.0.0")
	defer hc.Shutdown()

	// Add connections to make it unhealthy
	hc.UpdateConnection("conn1", "error", 100, errors.New("test error"))
	hc.UpdateConnection("conn2", "disconnected", 200, errors.New("disconnected"))

	handler := hc.HealthHandler()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}

	var health ServiceHealth
	if err := json.NewDecoder(w.Body).Decode(&health); err != nil {
		t.Errorf("Failed to decode health response: %v", err)
	}

	if health.Status != "unhealthy" {
		t.Errorf("Expected status 'unhealthy', got %s", health.Status)
	}
}

func TestReadinessHandler(t *testing.T) {
	hc := NewHealthChecker("test-service", "1.0.0")
	defer hc.Shutdown()

	handler := hc.ReadinessHandler()

	// Test ready status
	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Errorf("Failed to decode readiness response: %v", err)
	}

	if ready, ok := response["ready"].(bool); !ok || !ready {
		t.Error("Expected ready to be true")
	}
}

func TestLivenessHandler(t *testing.T) {
	hc := NewHealthChecker("test-service", "1.0.0")
	defer hc.Shutdown()

	handler := hc.LivenessHandler()

	req := httptest.NewRequest("GET", "/live", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Errorf("Failed to decode liveness response: %v", err)
	}

	if alive, ok := response["alive"].(bool); !ok || !alive {
		t.Error("Expected alive to be true")
	}

	if _, exists := response["uptime"]; !exists {
		t.Error("Expected uptime field")
	}
}

func TestNewConnectionMonitor(t *testing.T) {
	hc := NewHealthChecker("test-service", "1.0.0")
	defer hc.Shutdown()

	checkFunc := func() error { return nil }

	monitor := NewConnectionMonitor("test-monitor", hc, checkFunc, 1*time.Second)
	defer monitor.Stop()

	if monitor.name != "test-monitor" {
		t.Errorf("Expected name 'test-monitor', got %s", monitor.name)
	}

	if monitor.healthChecker != hc {
		t.Error("Health checker should match")
	}

	if monitor.interval != 1*time.Second {
		t.Errorf("Expected interval 1s, got %v", monitor.interval)
	}
}

func TestConnectionMonitorSuccess(t *testing.T) {
	hc := NewHealthChecker("test-service", "1.0.0")
	defer hc.Shutdown()

	checkFunc := func() error { return nil }

	monitor := NewConnectionMonitor("test-monitor", hc, checkFunc, 100*time.Millisecond)
	defer monitor.Stop()

	monitor.Start()

	// Wait for at least one check
	time.Sleep(200 * time.Millisecond)

	hc.mu.RLock()
	conn, exists := hc.connections["test-monitor"]
	hc.mu.RUnlock()

	if !exists {
		t.Error("Connection should exist")
	}

	if conn.Status != "connected" {
		t.Errorf("Expected status 'connected', got %s", conn.Status)
	}
}

func TestConnectionMonitorError(t *testing.T) {
	hc := NewHealthChecker("test-service", "1.0.0")
	defer hc.Shutdown()

	testErr := errors.New("test error")
	checkFunc := func() error { return testErr }

	monitor := NewConnectionMonitor("test-monitor", hc, checkFunc, 100*time.Millisecond)
	defer monitor.Stop()

	monitor.Start()

	// Wait for at least one check
	time.Sleep(200 * time.Millisecond)

	hc.mu.RLock()
	conn, exists := hc.connections["test-monitor"]
	hc.mu.RUnlock()

	if !exists {
		t.Error("Connection should exist")
	}

	if conn.Status != "error" {
		t.Errorf("Expected status 'error', got %s", conn.Status)
	}

	if conn.LastError != "test error" {
		t.Errorf("Expected error 'test error', got %s", conn.LastError)
	}
}

func TestConnectionMonitorStop(t *testing.T) {
	hc := NewHealthChecker("test-service", "1.0.0")
	defer hc.Shutdown()

	checkFunc := func() error { return nil }

	monitor := NewConnectionMonitor("test-monitor", hc, checkFunc, 50*time.Millisecond)
	monitor.Start()

	// Wait for at least one check
	time.Sleep(100 * time.Millisecond)

	// Stop the monitor
	monitor.Stop()

	// Wait a bit more
	time.Sleep(100 * time.Millisecond)

	// The monitor should have stopped (this is hard to test directly,
	// but we can at least verify it doesn't panic)
}

func BenchmarkGetHealth(b *testing.B) {
	hc := NewHealthChecker("test-service", "1.0.0")
	defer hc.Shutdown()

	// Add some connections
	hc.UpdateConnection("conn1", "connected", 100, nil)
	hc.UpdateConnection("conn2", "connected", 200, nil)
	hc.UpdateConnection("conn3", "error", 300, errors.New("test error"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hc.GetHealth()
	}
}

func BenchmarkUpdateConnection(b *testing.B) {
	hc := NewHealthChecker("test-service", "1.0.0")
	defer hc.Shutdown()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hc.UpdateConnection("test-conn", "connected", 100, nil)
	}
}
