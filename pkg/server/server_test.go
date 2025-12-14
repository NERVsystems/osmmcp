package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
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
	// Skip on Windows - process checking uses Unix-specific signal 0
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - uses Unix-specific signal handling")
	}

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
	// Skip on Windows - process checking uses Unix-specific signal 0
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - uses Unix-specific signal handling")
	}

	// Test the parent process monitoring logic in isolation
	s, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	// Context not needed for this test, but kept for consistency
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up channels to track monitoring behavior
	monitoringStarted := make(chan struct{})

	// Test the monitoring function directly without running the full server
	go func() {
		defer close(monitoringStarted)

		ppid := os.Getppid()
		s.logger.Debug("testing parent process monitor", "ppid", ppid)

		// Verify the process monitoring logic works
		if !isProcessRunning(ppid) {
			t.Errorf("Parent process %d should be running during test", ppid)
		}

		// Test with an invalid PID
		if isProcessRunning(999999) {
			t.Error("Invalid PID should not be detected as running")
		}
	}()

	// Wait for monitoring test to complete
	select {
	case <-monitoringStarted:
		// Good, monitoring test completed
	case <-time.After(5 * time.Second):
		t.Error("Parent process monitoring test did not complete within timeout")
	}

	// Test shutdown mechanism works (don't wait since server wasn't actually started)
	s.Shutdown()
}

func TestParentProcessMonitoringWithRealProcess(t *testing.T) {
	// Skip on Windows - uses Unix-specific signal handling and 'sleep' command
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - uses Unix-specific signal handling")
	}

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

// TestParentProcessMonitoringIntegration tests the integration without blocking on stdin
func TestParentProcessMonitoringIntegration(t *testing.T) {
	// Skip on Windows - uses Unix-specific signal handling
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - uses Unix-specific signal handling")
	}

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a server instance
	s, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test the monitoring setup without running the blocking server
	monitoringSetup := make(chan struct{})

	// Start the context goroutine (which would start monitoring)
	s.ctxGoroutine.Do(func() {
		derived, cancelDerived := context.WithCancel(ctx)
		s.ctxCancel = cancelDerived

		go func() {
			select {
			case <-derived.Done():
				s.Shutdown()
			case <-s.stopCh:
				// Already being shut down
			}
		}()

		// Simulate monitoring startup (without the infinite loop)
		go func() {
			ppid := os.Getppid()
			s.logger.Debug("integration test: parent process monitor setup", "ppid", ppid)

			// Verify process monitoring works
			if !isProcessRunning(ppid) {
				t.Errorf("Parent process %d should be running during integration test", ppid)
			}

			close(monitoringSetup)
		}()
	})

	// Wait for monitoring setup
	select {
	case <-monitoringSetup:
		// Good, monitoring was set up
	case <-time.After(2 * time.Second):
		t.Error("Parent process monitoring setup did not complete within timeout")
	}

	// Test shutdown mechanism (don't wait since server wasn't actually started)
	s.Shutdown()
}
