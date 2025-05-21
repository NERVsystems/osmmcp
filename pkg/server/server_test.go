package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewServer(t *testing.T) {
	s, err := NewServer()
	if err != nil {
		t.Errorf("NewServer() error = %v", err)
	}
	if s == nil {
		t.Error("NewServer() returned nil server")
	}
}

func TestServer_Run(t *testing.T) {
	s, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the server in a goroutine
	go func() {
		if err := s.RunWithContext(ctx); err != nil {
			t.Errorf("RunWithContext() error = %v", err)
		}
	}()

	// Shutdown the server
	s.Shutdown()
	s.WaitForShutdown()
}

func TestHandler_Health(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(logger)
	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()
	status, err := h.handleHealth(rr, req)
	if err != nil {
		t.Fatalf("handleHealth returned error: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
}
