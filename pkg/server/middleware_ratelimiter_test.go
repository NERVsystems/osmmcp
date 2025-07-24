package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestRateLimiterMiddleware_TooManyRequests(t *testing.T) {
	rl := NewRateLimiter(rate.Every(time.Second), 1)
	t.Cleanup(rl.Stop)

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:1234"

	// First request should pass
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec1.Code)
	}

	// Second immediate request should be rate limited
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got %d", rec2.Code)
	}
}

func TestRateLimiterEvictOldestVisitor(t *testing.T) {
	rl := NewRateLimiter(rate.Every(time.Minute), 1)
	rl.maxVisitors = 2
	t.Cleanup(rl.Stop)

	rl.getVisitor("1.1.1.1")
	time.Sleep(1 * time.Millisecond)
	rl.getVisitor("2.2.2.2")
	time.Sleep(1 * time.Millisecond)
	rl.getVisitor("3.3.3.3") // should evict 1.1.1.1

	rl.mu.RLock()
	_, ok1 := rl.visitors["1.1.1.1"]
	_, ok2 := rl.visitors["2.2.2.2"]
	_, ok3 := rl.visitors["3.3.3.3"]
	count := len(rl.visitors)
	rl.mu.RUnlock()

	if ok1 {
		t.Error("oldest visitor was not evicted")
	}
	if !ok2 || !ok3 {
		t.Error("expected newer visitors to remain")
	}
	if count != 2 {
		t.Errorf("expected 2 visitors, got %d", count)
	}
}
