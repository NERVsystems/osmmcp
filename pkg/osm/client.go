// Package osm provides utilities for working with OpenStreetMap data.
package osm

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/time/rate"

	"github.com/NERVsystems/osmmcp/pkg/tracing"
)

const (
	// DefaultUserAgent is the default User-Agent string
	DefaultUserAgent = "OSMMCP/0.1.0"
)

var (
	// Global HTTP client with connection pooling
	httpClient *http.Client

	// Rate limiters for each service
	nominatimLimiter *rate.Limiter
	overpassLimiter  *rate.Limiter
	osrmLimiter      *rate.Limiter

	// User agent string
	userAgent     string
	userAgentLock sync.RWMutex
)

// init initializes the global HTTP client and rate limiters
func init() {
	// Initialize HTTP client with connection pooling
	httpClient = &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
		Timeout: 30 * time.Second,
	}

	// Initialize rate limiters with default values
	initRateLimiters()

	// Set default user agent
	SetUserAgent(DefaultUserAgent)
}

// initRateLimiters initializes the rate limiters with default values
func initRateLimiters() {
	// Default to 1 request per second with burst of 1
	nominatimLimiter = rate.NewLimiter(rate.Limit(1), 1)
	overpassLimiter = rate.NewLimiter(rate.Limit(1), 1)
	osrmLimiter = rate.NewLimiter(rate.Limit(1), 1)
}

// UpdateNominatimRateLimits updates the Nominatim rate limiter
func UpdateNominatimRateLimits(rps float64, burst int) {
	nominatimLimiter = rate.NewLimiter(rate.Limit(rps), burst)
}

// UpdateOverpassRateLimits updates the Overpass rate limiter
func UpdateOverpassRateLimits(rps float64, burst int) {
	overpassLimiter = rate.NewLimiter(rate.Limit(rps), burst)
}

// UpdateOSRMRateLimits updates the OSRM rate limiter
func UpdateOSRMRateLimits(rps float64, burst int) {
	osrmLimiter = rate.NewLimiter(rate.Limit(rps), burst)
}

// SetUserAgent sets the User-Agent string
func SetUserAgent(ua string) {
	userAgentLock.Lock()
	defer userAgentLock.Unlock()
	userAgent = ua
}

// GetUserAgent returns the current User-Agent string
func GetUserAgent() string {
	userAgentLock.RLock()
	defer userAgentLock.RUnlock()
	return userAgent
}

// GetClient returns the global HTTP client
func GetClient(ctx context.Context) *http.Client {
	return httpClient
}

// hostFromURL extracts the host from a URL string
func hostFromURL(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}
	return u.Host
}

// waitForRateLimit waits for the appropriate rate limiter based on the request URL
func waitForRateLimit(ctx context.Context, req *http.Request) error {
	host := hostFromURL(req.URL.String())

	var service string
	var limiter *rate.Limiter

	switch host {
	case hostFromURL(NominatimBaseURL):
		service = tracing.ServiceNominatim
		limiter = nominatimLimiter
	case hostFromURL(OverpassBaseURL):
		service = tracing.ServiceOverpass
		limiter = overpassLimiter
	case hostFromURL(OSRMBaseURL):
		service = tracing.ServiceOSRM
		limiter = osrmLimiter
	default:
		return nil // No rate limiting for unknown hosts
	}

	// Check if we need to wait
	if !limiter.Allow() {
		// Record rate limit wait in current span
		startWait := time.Now()

		// Add event about rate limiting
		tracing.AddEvent(ctx, "rate_limit_wait",
			trace.WithAttributes(
				attribute.String(tracing.AttrRateLimitService, service),
			),
		)

		// Wait for rate limit
		err := limiter.Wait(ctx)

		// Record wait duration
		waitDuration := time.Since(startWait)
		tracing.SetAttributes(ctx,
			attribute.String(tracing.AttrRateLimitService, service),
			attribute.Int64(tracing.AttrRateLimitWaitMs, waitDuration.Milliseconds()),
		)

		if err != nil {
			return err
		}
	}

	return nil
}

// DoRequest performs an HTTP request with rate limiting
func DoRequest(ctx context.Context, req *http.Request) (*http.Response, error) {
	// Set User-Agent header
	req.Header.Set("User-Agent", GetUserAgent())

	// Wait for rate limit
	if err := waitForRateLimit(ctx, req); err != nil {
		return nil, err
	}

	// Perform request
	return httpClient.Do(req)
}

// NewRequestWithUserAgent creates a new HTTP request with proper User-Agent header
// This simplifies creating requests with the correct header throughout the codebase
func NewRequestWithUserAgent(ctx context.Context, method, url string, body interface{}) (*http.Request, error) {
	var req *http.Request
	var err error

	if body != nil {
		bodyReader, ok := body.(io.Reader)
		if !ok {
			return nil, fmt.Errorf("body must implement io.Reader")
		}
		req, err = http.NewRequestWithContext(ctx, method, url, bodyReader)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}

	if err != nil {
		return nil, err
	}

	// Set required User-Agent for Nominatim's usage policy
	req.Header.Set("User-Agent", GetUserAgent())

	return req, nil
}

// Client represents an OSM API client
type Client struct {
	logger *slog.Logger
}

// NewOSMClient creates a new OSM API client
func NewOSMClient() *Client {
	return &Client{
		logger: slog.Default(),
	}
}

// SetLogger sets the logger for the client
func (c *Client) SetLogger(logger *slog.Logger) {
	c.logger = logger
}

// Health check functions for external services
// CheckNominatimHealth checks if Nominatim service is available
func CheckNominatimHealth() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Make a simple status request to Nominatim
	req, err := http.NewRequestWithContext(ctx, "GET", NominatimBaseURL+"/status", nil)
	if err != nil {
		return fmt.Errorf("failed to create nominatim health check request: %w", err)
	}

	resp, err := DoRequest(ctx, req)
	if err != nil {
		return fmt.Errorf("nominatim health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("nominatim health check returned status %d", resp.StatusCode)
	}

	return nil
}

// CheckOverpassHealth checks if Overpass API is available
func CheckOverpassHealth() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Make a simple status request to Overpass
	req, err := http.NewRequestWithContext(ctx, "GET", OverpassBaseURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create overpass health check request: %w", err)
	}

	// Add a simple query to check if the service is responsive
	req.URL.RawQuery = "data=[out:json];out meta;"

	resp, err := DoRequest(ctx, req)
	if err != nil {
		return fmt.Errorf("overpass health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("overpass health check returned status %d", resp.StatusCode)
	}

	return nil
}

// CheckOSRMHealth checks if OSRM service is available
func CheckOSRMHealth() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Make a simple status request to OSRM
	req, err := http.NewRequestWithContext(ctx, "GET", OSRMBaseURL+"/nearest/v1/driving/0,0", nil)
	if err != nil {
		return fmt.Errorf("failed to create osrm health check request: %w", err)
	}

	resp, err := DoRequest(ctx, req)
	if err != nil {
		return fmt.Errorf("osrm health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("osrm health check returned status %d", resp.StatusCode)
	}

	return nil
}
