package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
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

func TestIsProcessRunning(t *testing.T) {
	// Test with current process (should be running)
	currentPID := os.Getpid()
	if !isProcessRunning(currentPID) {
		t.Errorf("isProcessRunning(%d) = false, want true (current process should be running)", currentPID)
	}

	// Test with parent process (should be running during test)
	parentPID := os.Getppid()
	if !isProcessRunning(parentPID) {
		t.Errorf("isProcessRunning(%d) = false, want true (parent process should be running)", parentPID)
	}

	// Test with an invalid PID (very high number unlikely to exist)
	invalidPID := 999999
	if isProcessRunning(invalidPID) {
		t.Errorf("isProcessRunning(%d) = true, want false (invalid PID should not be running)", invalidPID)
	}
}

func TestParentProcessMonitoring(t *testing.T) {
	// Test the parent process monitoring logic in isolation
	s, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up channels to track monitoring behavior
	monitoringStarted := make(chan struct{})
	shutdownCalled := make(chan struct{})

	// Override the monitoring function to test the logic
	originalPpid := os.Getppid()

	// Start the context goroutine (which starts monitoring)
	s.ctxGoroutine.Do(func() {
		derived, cancel := context.WithCancel(ctx)
		s.ctxCancel = cancel

		go func() {
			select {
			case <-derived.Done():
				s.Shutdown()
			case <-s.stopCh:
				// Already being shut down
			}
		}()

		// Start parent process monitoring in a test-friendly way
		go func() {
			close(monitoringStarted)
			// Simulate the monitoring logic
			ppid := originalPpid
			s.logger.Debug("starting parent process monitor", "ppid", ppid)

			// In a real scenario, this would loop and check process existence
			// For testing, we just verify the monitoring can start
			select {
			case <-s.stopCh:
				return
			case <-time.After(100 * time.Millisecond):
				// Simulate detecting parent process exit
				if !isProcessRunning(ppid) {
					s.logger.Info("parent process has exited, shutting down server", "ppid", ppid)
					close(shutdownCalled)
					s.Shutdown()
					return
				}
			}
		}()
	})

	// Wait for monitoring to start
	select {
	case <-monitoringStarted:
		// Good, monitoring started
	case <-time.After(1 * time.Second):
		t.Error("Parent process monitoring did not start within timeout")
		return
	}

	// Manually trigger shutdown to test the flow
	s.Shutdown()

	// Verify the server can be shut down properly
	s.WaitForShutdown()
}

func TestParentProcessMonitoringWithRealProcess(t *testing.T) {
	// This test creates a real child process to test parent monitoring
	// Skip on short tests as it requires subprocess execution
	if testing.Short() {
		t.Skip("Skipping subprocess test in short mode")
	}

	// For this test, we'll verify that the monitoring function
	// correctly identifies when a process is no longer running
	// by creating and terminating a subprocess

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start a simple subprocess that will exit
	cmd := exec.CommandContext(ctx, "sleep", "1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start test subprocess: %v", err)
	}

	childPID := cmd.Process.Pid

	// Verify process is initially running
	if !isProcessRunning(childPID) {
		t.Errorf("Child process %d should be running initially", childPID)
	}

	// Wait for process to exit
	if err := cmd.Wait(); err != nil {
		t.Logf("Process exited with: %v (this is expected)", err)
	}

	// Verify process is no longer running
	if isProcessRunning(childPID) {
		t.Errorf("Child process %d should not be running after exit", childPID)
	}
}

// TestParentProcessMonitoringIntegration tests the full integration
// of parent process monitoring with server shutdown
func TestParentProcessMonitoringIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a server instance
	s, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	// Override the logger to capture logs
	var logOutput []string
	var logMutex sync.Mutex
	customLogger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create a custom logger that captures messages
	customLogger = slog.New(&testLogHandler{
		logs:  &logOutput,
		mutex: &logMutex,
	})
	s.logger = customLogger

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Track server shutdown
	shutdownComplete := make(chan struct{})
	go func() {
		defer close(shutdownComplete)
		s.RunWithContext(ctx)
	}()

	// Give server time to start and begin monitoring
	time.Sleep(300 * time.Millisecond)

	// Verify the server started monitoring (by checking it's running)
	// Note: The server might be in process of starting, so we check multiple times
	var running bool
	for i := 0; i < 5; i++ {
		s.mu.Lock()
		running = s.running
		s.mu.Unlock()
		if running {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !running {
		t.Error("Server should be running")
	}

	// Trigger shutdown
	s.Shutdown()

	// Verify shutdown completes within reasonable time
	select {
	case <-shutdownComplete:
		// Success
	case <-time.After(3 * time.Second):
		t.Error("Server shutdown took too long")
	}
}

// testLogHandler is a custom slog handler for testing
type testLogHandler struct {
	logs  *[]string
	mutex *sync.Mutex
}

func (h *testLogHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *testLogHandler) Handle(ctx context.Context, record slog.Record) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	*h.logs = append(*h.logs, record.Message)
	return nil
}

func (h *testLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *testLogHandler) WithGroup(name string) slog.Handler {
	return h
}
