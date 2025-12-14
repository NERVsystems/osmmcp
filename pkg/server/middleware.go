package server

import (
	"bufio"
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/time/rate"

	"github.com/NERVsystems/osmmcp/pkg/tracing"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

// Context keys
const (
	requestIDKey contextKey = "request_id"
)

// RateLimiter provides per-IP rate limiting
type RateLimiter struct {
	visitors    map[string]*visitor
	mu          sync.RWMutex
	rate        rate.Limit
	burst       int
	cleanup     chan struct{}
	maxVisitors int // Maximum number of visitor entries to prevent memory exhaustion
}

// visitor tracks rate limiter state for each visitor
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	rl := &RateLimiter{
		visitors:    make(map[string]*visitor),
		rate:        r,
		burst:       b,
		cleanup:     make(chan struct{}),
		maxVisitors: 10000, // Reasonable limit to prevent memory exhaustion
	}

	// Start cleanup goroutine
	go rl.cleanupVisitors()

	return rl
}

// cleanupVisitors removes old entries from the visitors map
func (rl *RateLimiter) cleanupVisitors() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	defer func() {
		if r := recover(); r != nil {
			// Log panic and restart cleanup goroutine
			// In a production system, you'd want to inject a logger

			// Restart the cleanup goroutine after a brief delay
			time.Sleep(time.Second)
			go rl.cleanupVisitors()
		}
	}()

	for {
		select {
		case <-ticker.C:
			// Add panic recovery around the cleanup operation
			func() {
				defer func() {
					if r := recover(); r != nil {
						// Log panic but don't stop the cleanup loop
						// The cleanup will continue on the next tick
					}
				}()

				rl.mu.Lock()
				for ip, v := range rl.visitors {
					if time.Since(v.lastSeen) > 3*time.Minute {
						delete(rl.visitors, ip)
					}
				}
				rl.mu.Unlock()
			}()
		case <-rl.cleanup:
			return
		}
	}
}

// Stop stops the cleanup goroutine
func (rl *RateLimiter) Stop() {
	close(rl.cleanup)
}

// getVisitor returns the rate limiter for the given IP
func (rl *RateLimiter) getVisitor(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	if !exists {
		// Check if we've hit the maximum number of visitors
		if len(rl.visitors) >= rl.maxVisitors {
			// Find and remove the oldest visitor to make room
			rl.evictOldestVisitor()
		}

		limiter := rate.NewLimiter(rl.rate, rl.burst)
		rl.visitors[ip] = &visitor{limiter, time.Now()}
		return limiter
	}

	v.lastSeen = time.Now()
	return v.limiter
}

// evictOldestVisitor removes the visitor with the oldest lastSeen time
// This method assumes the caller holds the write lock
func (rl *RateLimiter) evictOldestVisitor() {
	if len(rl.visitors) == 0 {
		return
	}

	var oldestIP string
	var oldestTime time.Time
	first := true

	for ip, visitor := range rl.visitors {
		if first || visitor.lastSeen.Before(oldestTime) {
			oldestIP = ip
			oldestTime = visitor.lastSeen
			first = false
		}
	}

	if oldestIP != "" {
		delete(rl.visitors, oldestIP)
	}
}

// Middleware returns an HTTP middleware that rate limits requests
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getIP(r)
		limiter := rl.getVisitor(ip)

		if !limiter.Allow() {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getIP extracts the client IP from the request
func getIP(r *http.Request) string {
	// Check X-Forwarded-For header
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// Take the first IP in the chain
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}

	// Check X-Real-IP header
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" && net.ParseIP(realIP) != nil {
		return realIP
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// RequestSizeLimiter returns middleware that limits request body size
func RequestSizeLimiter(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders adds security headers to responses
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")

		next.ServeHTTP(w, r)
	})
}

// LoggingMiddleware logs HTTP requests
func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap ResponseWriter to capture status code
			wrapped := newResponseWriter(w)

			// Add request ID to context
			reqID := r.Header.Get("X-Request-ID")
			if reqID == "" {
				reqID = generateRequestID()
			}
			ctx := context.WithValue(r.Context(), requestIDKey, reqID)
			r = r.WithContext(ctx)

			// Log request start
			logger.Info("http request",
				"request_id", reqID,
				"method", r.Method,
				"path", r.URL.Path,
				"remote_addr", getIP(r),
				"user_agent", r.UserAgent())

			// Process request
			next.ServeHTTP(wrapped, r)

			// Log request completion
			logger.Info("http response",
				"request_id", reqID,
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.statusCode,
				"duration", time.Since(start),
				"bytes", wrapped.bytesWritten)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code and bytes written
// It also preserves the optional interfaces that the underlying ResponseWriter might implement
type responseWriter struct {
	http.ResponseWriter
	statusCode    int
	bytesWritten  int64
	headerWritten bool
}

// newResponseWriter creates a new responseWriter that preserves optional interfaces
func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.headerWritten {
		rw.statusCode = code
		rw.headerWritten = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.headerWritten {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

// Flush implements the http.Flusher interface
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements the http.Hijacker interface
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// Push implements the http.Pusher interface (HTTP/2 Server Push)
func (rw *responseWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := rw.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

// TracingMiddleware adds OpenTelemetry tracing to HTTP requests
func TracingMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract session ID from query or header for correlation
			sessionID := r.URL.Query().Get("sessionId")
			if sessionID == "" {
				sessionID = r.Header.Get("X-Session-ID")
			}

			// Start tracing span
			spanName := r.Method + " " + r.URL.Path
			ctx, span := tracing.StartSpan(r.Context(), spanName,
				trace.WithAttributes(
					attribute.String(tracing.AttrHTTPMethod, r.Method),
					attribute.String(tracing.AttrHTTPPath, r.URL.Path),
					attribute.String("http.url", r.URL.String()),
					attribute.String("http.host", r.Host),
					attribute.String("http.user_agent", r.UserAgent()),
					attribute.String("http.remote_addr", r.RemoteAddr),
				),
			)
			defer span.End()

			// Add session ID if present
			if sessionID != "" {
				span.SetAttributes(attribute.String(tracing.AttrHTTPSessionID, sessionID))
			}

			// Wrap response writer to capture status code
			wrapped := newResponseWriter(w)

			// Process request with new context
			r = r.WithContext(ctx)
			next.ServeHTTP(wrapped, r)

			// Set final span attributes
			span.SetAttributes(
				attribute.Int(tracing.AttrHTTPStatusCode, wrapped.statusCode),
				attribute.Int64("http.response.size", wrapped.bytesWritten),
			)

			// Set span status based on HTTP status code
			if wrapped.statusCode >= 400 {
				span.SetStatus(codes.Error, http.StatusText(wrapped.statusCode))
			} else {
				span.SetStatus(codes.Ok, "")
			}
		})
	}
}
