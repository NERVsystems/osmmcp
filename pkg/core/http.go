// Package core provides shared utilities for the OpenStreetMap MCP tools.
package core

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// RetryOptions configures retry behavior for HTTP requests
type RetryOptions struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

// DefaultRetryOptions provides sensible defaults for retries
var DefaultRetryOptions = RetryOptions{
	MaxAttempts:  3,
	InitialDelay: 500 * time.Millisecond,
	MaxDelay:     10 * time.Second,
	Multiplier:   2.0,
}

// DefaultClient provides a pre-configured HTTP client with secure defaults
var DefaultClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	},
}

// secureHeaders adds security headers to the request
func secureHeaders(req *http.Request) {
	req.Header.Set("X-Content-Type-Options", "nosniff")
	req.Header.Set("X-Frame-Options", "DENY")
	req.Header.Set("X-XSS-Protection", "1; mode=block")
	req.Header.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
}

// WithRetry performs an HTTP request with exponential backoff retry logic
func WithRetry(ctx context.Context, req *http.Request, client *http.Client, options RetryOptions) (*http.Response, error) {
	logger := slog.Default().With(
		"url", req.URL.String(),
		"method", req.Method,
		"host", req.Host,
	)
	var lastErr error

	delay := options.InitialDelay

	for attempt := 0; attempt < options.MaxAttempts; attempt++ {
		// If not the first attempt, log and wait
		if attempt > 0 {
			logger.Info("retrying request",
				"attempt", attempt+1,
				"max_attempts", options.MaxAttempts,
				"delay", delay,
				"last_error", lastErr,
			)

			// Wait for backoff delay
			select {
			case <-time.After(delay):
				// Continue with retry
			case <-ctx.Done():
				return nil, ctx.Err()
			}

			// Calculate the next delay with exponential backoff
			delay = time.Duration(float64(delay) * options.Multiplier)
			if delay > options.MaxDelay {
				delay = options.MaxDelay
			}
		}

		// Make a new request for each attempt to avoid body already closed errors
		newReq := req.Clone(ctx)
		if req.Body != nil {
			logger.Error("request with body cannot be retried automatically, use a request factory function")
			return nil, fmt.Errorf("cannot retry request with non-nil body")
		}

		// Add security headers
		secureHeaders(newReq)

		// Execute the request
		resp, err := client.Do(newReq)
		if err == nil && resp.StatusCode == http.StatusOK {
			logger.Debug("request successful",
				"status", resp.StatusCode,
				"content_length", resp.ContentLength,
				"content_type", resp.Header.Get("Content-Type"),
			)
			return resp, nil
		}

		// Record the error
		if err != nil {
			lastErr = err
			logger.Error("request failed",
				"error", err,
				"attempt", attempt+1,
				"url", req.URL.String(),
			)
		} else {
			lastErr = fmt.Errorf("HTTP status %d", resp.StatusCode)
			logger.Error("request returned error status",
				"status", resp.StatusCode,
				"attempt", attempt+1,
				"url", req.URL.String(),
				"response_headers", resp.Header,
			)
			resp.Body.Close()
		}
	}

	return nil, fmt.Errorf("max retries reached: %w", lastErr)
}

// DoWithRetry performs an HTTP request with default retry options
func DoWithRetry(ctx context.Context, req *http.Request, client *http.Client) (*http.Response, error) {
	if client == nil {
		client = DefaultClient
	}
	return WithRetry(ctx, req, client, DefaultRetryOptions)
}

// RequestFactory is a function that creates a new HTTP request
// This approach allows for retrying requests with bodies
type RequestFactory func() (*http.Request, error)

// WithRetryFactory performs HTTP requests created by a factory with retry logic
func WithRetryFactory(ctx context.Context, factory RequestFactory, client *http.Client, options RetryOptions) (*http.Response, error) {
	var lastErr error
	delay := options.InitialDelay
	logger := slog.Default()

	if client == nil {
		client = DefaultClient
	}

	for attempt := 0; attempt < options.MaxAttempts; attempt++ {
		// If not the first attempt, log and wait
		if attempt > 0 {
			logger.Info("retrying request",
				"attempt", attempt+1,
				"max_attempts", options.MaxAttempts,
				"delay", delay,
				"last_error", lastErr,
			)

			// Wait for backoff delay
			select {
			case <-time.After(delay):
				// Continue with retry
			case <-ctx.Done():
				return nil, ctx.Err()
			}

			// Calculate the next delay with exponential backoff
			delay = time.Duration(float64(delay) * options.Multiplier)
			if delay > options.MaxDelay {
				delay = options.MaxDelay
			}
		}

		// Create a new request
		req, err := factory()
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %w", err)
			logger.Error("request creation failed",
				"error", err,
				"attempt", attempt+1,
			)
			continue
		}

		// Use the provided context
		req = req.WithContext(ctx)

		// Add security headers
		secureHeaders(req)

		// Execute the request
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			logger.Debug("request successful",
				"status", resp.StatusCode,
				"content_length", resp.ContentLength,
				"content_type", resp.Header.Get("Content-Type"),
				"url", req.URL.String(),
			)
			return resp, nil
		}

		// Record the error
		if err != nil {
			lastErr = err
			logger.Error("request failed",
				"error", err,
				"attempt", attempt+1,
				"url", req.URL.String(),
			)
		} else {
			lastErr = fmt.Errorf("HTTP status %d", resp.StatusCode)
			logger.Error("request returned error status",
				"status", resp.StatusCode,
				"attempt", attempt+1,
				"url", req.URL.String(),
				"response_headers", resp.Header,
			)
			resp.Body.Close()
		}
	}

	return nil, fmt.Errorf("max retries reached: %w", lastErr)
}
