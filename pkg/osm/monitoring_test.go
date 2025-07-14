package osm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	
	"golang.org/x/time/rate"
)

func TestSetAndGetMonitoringHooks(t *testing.T) {
	// Clear any existing hooks
	SetMonitoringHooks(nil)
	
	// Create test hooks
	var requestCalled, responseCalled, rateLimitCalled, errorCalled bool
	
	hooks := &MonitoringHooks{
		OnRequest: func(service, operation string) {
			requestCalled = true
		},
		OnResponse: func(service, operation string, duration time.Duration, success bool) {
			responseCalled = true
		},
		OnRateLimit: func(service string, waitTime time.Duration) {
			rateLimitCalled = true
		},
		OnError: func(service, errorType string) {
			errorCalled = true
		},
	}
	
	// Set hooks
	SetMonitoringHooks(hooks)
	
	// Get hooks and verify
	retrieved := getMonitoringHooks()
	if retrieved == nil {
		t.Error("Expected hooks to be set")
	}
	
	// Test that hooks are called
	if retrieved.OnRequest != nil {
		retrieved.OnRequest("test", "test")
		if !requestCalled {
			t.Error("OnRequest should have been called")
		}
	}
	
	if retrieved.OnResponse != nil {
		retrieved.OnResponse("test", "test", 100*time.Millisecond, true)
		if !responseCalled {
			t.Error("OnResponse should have been called")
		}
	}
	
	if retrieved.OnRateLimit != nil {
		retrieved.OnRateLimit("test", 100*time.Millisecond)
		if !rateLimitCalled {
			t.Error("OnRateLimit should have been called")
		}
	}
	
	if retrieved.OnError != nil {
		retrieved.OnError("test", "test")
		if !errorCalled {
			t.Error("OnError should have been called")
		}
	}
}

func TestGetServiceFromRequest(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "Nominatim URL",
			url:      "https://nominatim.openstreetmap.org/search",
			expected: "nominatim",
		},
		{
			name:     "Overpass URL",
			url:      "https://overpass-api.de/api/interpreter",
			expected: "overpass",
		},
		{
			name:     "OSRM URL",
			url:      "https://router.project-osrm.org/route/v1/driving/0,0;1,1",
			expected: "osrm",
		},
		{
			name:     "Unknown URL",
			url:      "https://example.com/api",
			expected: "unknown",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", tt.url, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			
			service := getServiceFromRequest(req)
			if service != tt.expected {
				t.Errorf("Expected service %s, got %s", tt.expected, service)
			}
		})
	}
}

func TestMonitoredDoRequestSuccess(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()
	
	// Set up monitoring hooks
	var requestCalled, responseCalled bool
	var capturedService, capturedOperation string
	var capturedDuration time.Duration
	var capturedSuccess bool
	
	hooks := &MonitoringHooks{
		OnRequest: func(service, operation string) {
			requestCalled = true
			capturedService = service
			capturedOperation = operation
		},
		OnResponse: func(service, operation string, duration time.Duration, success bool) {
			responseCalled = true
			capturedDuration = duration
			capturedSuccess = success
		},
	}
	
	SetMonitoringHooks(hooks)
	defer SetMonitoringHooks(nil) // Clean up
	
	// Make request
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	
	resp, err := MonitoredDoRequest(context.Background(), req, "test_operation")
	if err != nil {
		t.Errorf("MonitoredDoRequest failed: %v", err)
	}
	defer resp.Body.Close()
	
	// Verify hooks were called
	if !requestCalled {
		t.Error("OnRequest should have been called")
	}
	
	if !responseCalled {
		t.Error("OnResponse should have been called")
	}
	
	if capturedService != "unknown" { // Test server URL is not a known service
		t.Errorf("Expected service 'unknown', got %s", capturedService)
	}
	
	if capturedOperation != "test_operation" {
		t.Errorf("Expected operation 'test_operation', got %s", capturedOperation)
	}
	
	if capturedDuration <= 0 {
		t.Error("Duration should be greater than 0")
	}
	
	if !capturedSuccess {
		t.Error("Request should have been successful")
	}
}

func TestMonitoredDoRequestError(t *testing.T) {
	// Create a test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()
	
	// Set up monitoring hooks
	var errorCalled bool
	var capturedSuccess bool
	
	hooks := &MonitoringHooks{
		OnResponse: func(service, operation string, duration time.Duration, success bool) {
			capturedSuccess = success
		},
		OnError: func(service, errorType string) {
			errorCalled = true
		},
	}
	
	SetMonitoringHooks(hooks)
	defer SetMonitoringHooks(nil) // Clean up
	
	// Make request
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	
	resp, err := MonitoredDoRequest(context.Background(), req, "test_operation")
	if err != nil {
		t.Errorf("MonitoredDoRequest failed: %v", err)
	}
	defer resp.Body.Close()
	
	// Verify success was false due to 500 status
	if capturedSuccess {
		t.Error("Request should not have been successful")
	}
	
	// Error hook should not be called for HTTP errors, only for network errors
	if errorCalled {
		t.Error("OnError should not have been called for HTTP error status")
	}
}

func TestMonitoredDoRequestNetworkError(t *testing.T) {
	// Set up monitoring hooks
	var errorCalled bool
	var capturedErrorType string
	
	hooks := &MonitoringHooks{
		OnError: func(service, errorType string) {
			errorCalled = true
			capturedErrorType = errorType
		},
	}
	
	SetMonitoringHooks(hooks)
	defer SetMonitoringHooks(nil) // Clean up
	
	// Make request to invalid URL (using a blocked port)
	req, err := http.NewRequest("GET", "http://127.0.0.1:1", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	
	_, err = MonitoredDoRequest(context.Background(), req, "test_operation")
	if err == nil {
		t.Error("Expected network error")
	}
	
	// Verify error hook was called
	if !errorCalled {
		t.Error("OnError should have been called for network error")
	}
	
	if capturedErrorType != "request_error" {
		t.Errorf("Expected error type 'request_error', got %s", capturedErrorType)
	}
}

func TestMonitoredDoRequestWithoutHooks(t *testing.T) {
	// Clear hooks
	SetMonitoringHooks(nil)
	
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()
	
	// Make request without hooks (should not panic)
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	
	resp, err := MonitoredDoRequest(context.Background(), req, "test_operation")
	if err != nil {
		t.Errorf("MonitoredDoRequest failed: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestMonitoredDoRequestRateLimit(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()
	
	// Set up monitoring hooks
	var rateLimitCalled bool
	var capturedWaitTime time.Duration
	
	hooks := &MonitoringHooks{
		OnRateLimit: func(service string, waitTime time.Duration) {
			rateLimitCalled = true
			capturedWaitTime = waitTime
		},
	}
	
	SetMonitoringHooks(hooks)
	defer SetMonitoringHooks(nil) // Clean up
	
	// Set very restrictive rate limit to trigger rate limiting
	oldLimiter := nominatimLimiter
	nominatimLimiter = rate.NewLimiter(rate.Limit(0.1), 1) // Very slow rate
	defer func() { nominatimLimiter = oldLimiter }()
	
	// Make request to nominatim-like URL
	req, err := http.NewRequest("GET", "https://nominatim.openstreetmap.org/search", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	
	// Make two requests quickly to trigger rate limiting
	_, err = MonitoredDoRequest(context.Background(), req, "test_operation")
	if err != nil {
		t.Errorf("First request failed: %v", err)
	}
	
	_, err = MonitoredDoRequest(context.Background(), req, "test_operation")
	if err != nil {
		t.Errorf("Second request failed: %v", err)
	}
	
	// The second request should have triggered rate limiting
	if !rateLimitCalled {
		t.Error("OnRateLimit should have been called")
	}
	
	if capturedWaitTime <= 100*time.Millisecond {
		t.Error("Wait time should be significant for rate limiting")
	}
}

func BenchmarkMonitoredDoRequest(b *testing.B) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()
	
	// Set up minimal monitoring hooks
	hooks := &MonitoringHooks{
		OnRequest:  func(service, operation string) {},
		OnResponse: func(service, operation string, duration time.Duration, success bool) {},
	}
	
	SetMonitoringHooks(hooks)
	defer SetMonitoringHooks(nil)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", server.URL, nil)
		resp, _ := MonitoredDoRequest(context.Background(), req, "benchmark")
		if resp != nil {
			resp.Body.Close()
		}
	}
}

func BenchmarkGetServiceFromRequest(b *testing.B) {
	req, _ := http.NewRequest("GET", "https://nominatim.openstreetmap.org/search", nil)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getServiceFromRequest(req)
	}
}