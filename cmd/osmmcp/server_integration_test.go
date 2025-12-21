package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// waitForEndpoint polls the given URL until it returns 200 OK or timeout
func waitForEndpoint(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) // #nosec G107 -- test helper
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("endpoint %s did not become ready", url)
}

func TestServerMainHealth(t *testing.T) {
	// Pick an available TCP port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	// Build server binary inside temporary directory
	binDir := t.TempDir()
	binName := "osmmcp-test"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(binDir, binName)
	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	// Start the server process
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runCmd := exec.CommandContext(ctx, binPath,
		"--enable-http",
		"--http-only", // Skip stdio transport for testing
		"--http-addr", fmt.Sprintf("127.0.0.1:%d", port),
		"--enable-monitoring=false",
	)
	if err := runCmd.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer func() {
		// On Windows, Kill() is the only reliable way to terminate a process
		// On Unix, Kill() sends SIGKILL which works for cleanup
		_ = runCmd.Process.Kill()
		runCmd.Wait() // wait for shutdown
	}()

	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	if err := waitForEndpoint(healthURL, 15*time.Second); err != nil {
		t.Fatalf("server did not start: %v", err)
	}

	resp, err := http.Get(healthURL) // #nosec G107 -- test request
	if err != nil {
		t.Fatalf("failed GET /health: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}
