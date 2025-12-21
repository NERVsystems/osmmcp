package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
)

func TestHTTPTransport_ServiceDiscovery(t *testing.T) {
	// Create a test MCP server
	mcpSrv := mcpserver.NewMCPServer("test-server", "1.0.0")

	config := HTTPTransportConfig{
		Addr:        ":0",
		BaseURL:     "http://localhost:8080",
		AuthType:    "none",
		MCPEndpoint: "/mcp",
	}

	transport := NewHTTPTransport(mcpSrv, config, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

	// Create test server
	server := httptest.NewServer(transport.mux)
	defer server.Close()

	// Test service discovery
	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var discovery map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		t.Fatal(err)
	}

	// Verify service discovery structure
	if discovery["service"] != "mcp-server" {
		t.Errorf("Expected service 'mcp-server', got %v", discovery["service"])
	}

	if discovery["transport"] != "streamable-http" {
		t.Errorf("Expected transport 'HTTP+SSE', got %v", discovery["transport"])
	}

	endpoints, ok := discovery["endpoints"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected endpoints to be a map")
	}

	if !strings.Contains(endpoints["mcp"].(string), "/mcp") {
		t.Errorf("Expected MCP endpoint to contain '/mcp', got %v", endpoints["mcp"])
	}
}

func TestHTTPTransport_HealthEndpoint(t *testing.T) {
	mcpSrv := mcpserver.NewMCPServer("test-server", "1.0.0")
	config := DefaultHTTPTransportConfig()
	transport := NewHTTPTransport(mcpSrv, config, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

	server := httptest.NewServer(transport.mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var health map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatal(err)
	}

	if health["status"] != "ok" {
		t.Errorf("Expected status 'ok', got %v", health["status"])
	}
}

func TestHTTPTransport_MessageEndpoint_404Fix(t *testing.T) {
	// Skip: In MCP SDK v0.40.0+, /message endpoint is replaced by POST to /mcp
	t.Skip("Skipping: streamable-http transport uses POST /mcp instead of /message")
}

func TestHTTPTransport_MessageEndpoint_WithInvalidSession(t *testing.T) {
	// Skip: In MCP SDK v0.40.0+, /message endpoint is replaced by POST to /mcp
	t.Skip("Skipping: streamable-http transport uses POST /mcp instead of /message")
}

func TestHTTPTransport_SSEEndpoint(t *testing.T) {
	// Skip: In MCP SDK v0.40.0+, /sse endpoint is replaced by GET to /mcp
	t.Skip("Skipping: streamable-http transport uses GET /mcp for SSE, not /sse")
}

func TestHTTPTransport_Authentication_Bearer(t *testing.T) {
	// Skip: In MCP SDK v0.40.0+, auth is tested on /mcp endpoint, not /sse
	t.Skip("Skipping: streamable-http transport uses /mcp endpoint, not /sse")
}

func TestHTTPTransport_DebugEndpoints(t *testing.T) {
	mcpSrv := mcpserver.NewMCPServer("test-server", "1.0.0")
	config := DefaultHTTPTransportConfig()
	transport := NewHTTPTransport(mcpSrv, config, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

	server := httptest.NewServer(transport.mux)
	defer server.Close()

	// Test MCP debug endpoint (streamable-http transport uses /mcp/debug)
	resp, err := http.Get(server.URL + "/mcp/debug")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for MCP debug, got %d", resp.StatusCode)
	}

	var debug map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&debug); err != nil {
		t.Fatal(err)
	}

	// streamable-http uses /mcp as the single endpoint
	if debug["endpoint"] != "/mcp" {
		t.Errorf("Expected endpoint '/mcp', got %v", debug["endpoint"])
	}

	if debug["transport"] != "streamable-http" {
		t.Errorf("Expected transport 'streamable-http', got %v", debug["transport"])
	}
}

func TestHTTPTransport_Shutdown(t *testing.T) {
	mcpSrv := mcpserver.NewMCPServer("test-server", "1.0.0")
	config := DefaultHTTPTransportConfig()
	config.Addr = ":0" // Use a random available port

	transport := NewHTTPTransport(mcpSrv, config, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

	// Start transport in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- transport.Start()
	}()

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := transport.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}

	// Verify server stopped
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("Unexpected error from Start(): %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Server did not stop within timeout")
	}
}

func TestHTTPTransport_DualTransportCompliance(t *testing.T) {
	// This test verifies that our implementation satisfies the requirements
	// for streamable-http transport (MCP 2025-03-26 spec)

	mcpSrv := mcpserver.NewMCPServer("test-server", "1.0.0")
	config := DefaultHTTPTransportConfig()
	transport := NewHTTPTransport(mcpSrv, config, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

	server := httptest.NewServer(transport.mux)
	defer server.Close()

	t.Run("ServiceDiscoveryAdvertisesMCPEndpoint", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		var discovery map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
			t.Fatal(err)
		}

		// Must advertise streamable-http transport
		if discovery["transport"] != "streamable-http" {
			t.Errorf("Service discovery must advertise 'streamable-http' transport, got %v", discovery["transport"])
		}

		// Must include mcp endpoint
		endpoints, ok := discovery["endpoints"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected endpoints to be a map")
		}
		if endpoints["mcp"] == nil {
			t.Error("Service discovery must include 'mcp' endpoint")
		}
	})
}

func TestHTTPTransport_ForceHTTPSWithoutTLS(t *testing.T) {
	mcpSrv := mcpserver.NewMCPServer("test-server", "1.0.0")
	config := DefaultHTTPTransportConfig()
	config.Addr = ":0"
	config.ForceHTTPS = true
	config.TLSCertFile = ""
	config.TLSKeyFile = ""

	transport := NewHTTPTransport(mcpSrv, config, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

	errCh := make(chan error, 1)
	go func() {
		errCh <- transport.Start()
	}()

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected error when ForceHTTPS is enabled without TLS certificates")
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("expected error but Start did not return in time")
		transport.Shutdown(context.Background())
	}
}
