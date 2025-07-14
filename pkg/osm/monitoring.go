package osm

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// MonitoringHooks defines hooks for monitoring HTTP requests
type MonitoringHooks struct {
	// OnRequest is called before making an HTTP request
	OnRequest func(service, operation string)
	
	// OnResponse is called after receiving an HTTP response
	OnResponse func(service, operation string, duration time.Duration, success bool)
	
	// OnRateLimit is called when a rate limit is encountered
	OnRateLimit func(service string, waitTime time.Duration)
	
	// OnError is called when an error occurs
	OnError func(service, errorType string)
}

var (
	// Global monitoring hooks
	globalHooks *MonitoringHooks
	hooksMutex  sync.RWMutex
)

// SetMonitoringHooks sets global monitoring hooks
func SetMonitoringHooks(hooks *MonitoringHooks) {
	hooksMutex.Lock()
	defer hooksMutex.Unlock()
	globalHooks = hooks
}

// getMonitoringHooks returns the current monitoring hooks
func getMonitoringHooks() *MonitoringHooks {
	hooksMutex.RLock()
	defer hooksMutex.RUnlock()
	return globalHooks
}

// MonitoredDoRequest performs an HTTP request with monitoring
func MonitoredDoRequest(ctx context.Context, req *http.Request, operation string) (*http.Response, error) {
	service := getServiceFromRequest(req)
	
	hooks := getMonitoringHooks()
	if hooks != nil && hooks.OnRequest != nil {
		hooks.OnRequest(service, operation)
	}
	
	start := time.Now()
	
	// Check for rate limiting
	if err := waitForRateLimit(ctx, req); err != nil {
		if hooks != nil && hooks.OnError != nil {
			hooks.OnError(service, "rate_limit_wait_error")
		}
		return nil, err
	}
	
	// Track rate limit wait time
	waitTime := time.Since(start)
	if waitTime > 100*time.Millisecond { // Only track significant waits
		if hooks != nil && hooks.OnRateLimit != nil {
			hooks.OnRateLimit(service, waitTime)
		}
	}
	
	// Reset timer for actual request
	requestStart := time.Now()
	resp, err := httpClient.Do(req)
	duration := time.Since(requestStart)
	
	success := err == nil && resp != nil && resp.StatusCode < 400
	
	if hooks != nil && hooks.OnResponse != nil {
		hooks.OnResponse(service, operation, duration, success)
	}
	
	if err != nil && hooks != nil && hooks.OnError != nil {
		hooks.OnError(service, "request_error")
	}
	
	return resp, err
}

// getServiceFromRequest determines which service is being called based on the request URL
func getServiceFromRequest(req *http.Request) string {
	host := req.URL.Host
	
	switch host {
	case hostFromURL(NominatimBaseURL):
		return "nominatim"
	case hostFromURL(OverpassBaseURL):
		return "overpass"
	case hostFromURL(OSRMBaseURL):
		return "osrm"
	default:
		return "unknown"
	}
}