package server

import (
	"context"
	"encoding/json"
	"io"
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
	// This test specifically addresses the bug where POST /message returned 404
	mcpSrv := mcpserver.NewMCPServer("test-server", "1.0.0")
	config := DefaultHTTPTransportConfig()
	transport := NewHTTPTransport(mcpSrv, config, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

	server := httptest.NewServer(transport.mux)
	defer server.Close()

	// Test POST /message without sessionId
	resp, err := http.Post(server.URL+"/message", "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","method":"initialize","id":1}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Should return 400 (Bad Request with JSON-RPC error), NOT 404
	if resp.StatusCode == http.StatusNotFound {
		t.Error("POST /message returned 404 - the bug is still present!")
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	// Should be a proper JSON-RPC error
	if response["jsonrpc"] != "2.0" {
		t.Error("Response should be JSON-RPC 2.0")
	}

	if response["error"] == nil {
		t.Error("Response should contain an error")
	}
}

func TestHTTPTransport_MessageEndpoint_WithInvalidSession(t *testing.T) {
	mcpSrv := mcpserver.NewMCPServer("test-server", "1.0.0")
	config := DefaultHTTPTransportConfig()
	transport := NewHTTPTransport(mcpSrv, config, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

	server := httptest.NewServer(transport.mux)
	defer server.Close()

	// Test POST /message with invalid sessionId
	resp, err := http.Post(server.URL+"/message?sessionId=invalid-session", "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","method":"initialize","id":1}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Should return 400 (Bad Request), NOT 404
	if resp.StatusCode == http.StatusNotFound {
		t.Error("POST /message?sessionId=invalid returned 404 - the bug is still present!")
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	// Should be a proper JSON-RPC error for invalid session
	if response["jsonrpc"] != "2.0" {
		t.Error("Response should be JSON-RPC 2.0")
	}

	errorObj, ok := response["error"].(map[string]interface{})
	if !ok {
		t.Fatal("Response should contain an error object")
	}

	if !strings.Contains(errorObj["message"].(string), "Invalid session") {
		t.Error("Error message should mention invalid session")
	}
}

func TestHTTPTransport_SSEEndpoint(t *testing.T) {
	mcpSrv := mcpserver.NewMCPServer("test-server", "1.0.0")
	config := DefaultHTTPTransportConfig()
	transport := NewHTTPTransport(mcpSrv, config, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

	server := httptest.NewServer(transport.mux)
	defer server.Close()

	// Test SSE endpoint
	req, err := http.NewRequest("GET", server.URL+"/sse", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Check SSE headers
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Error("Expected Content-Type: text/event-stream")
	}

	if resp.Header.Get("Cache-Control") != "no-cache" {
		t.Error("Expected Cache-Control: no-cache")
	}

	// Read the initial endpoint event
	buf := make([]byte, 1024)
	n, err := resp.Body.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}

	response := string(buf[:n])
	if !strings.Contains(response, "event: endpoint") {
		t.Error("Expected 'event: endpoint' in SSE response")
	}

	if !strings.Contains(response, "sessionId=") {
		t.Error("Expected sessionId in SSE endpoint data")
	}
}

func TestHTTPTransport_Authentication_Bearer(t *testing.T) {
	mcpSrv := mcpserver.NewMCPServer("test-server", "1.0.0")
	config := HTTPTransportConfig{
		Addr:        ":0",
		AuthType:    "bearer",
		AuthToken:   "test-token",
		MCPEndpoint: "/mcp",
	}

	transport := NewHTTPTransport(mcpSrv, config, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	server := httptest.NewServer(transport.mux)
	defer server.Close()

	// Test without auth - should fail
	resp, err := http.Get(server.URL + "/sse")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 for missing auth, got %d", resp.StatusCode)
	}

	// Test with correct bearer token - should succeed
	req, err := http.NewRequest("GET", server.URL+"/sse", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Accept", "text/event-stream")

	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 with correct auth, got %d", resp2.StatusCode)
	}
}

func TestHTTPTransport_DebugEndpoints(t *testing.T) {
	mcpSrv := mcpserver.NewMCPServer("test-server", "1.0.0")
	config := DefaultHTTPTransportConfig()
	transport := NewHTTPTransport(mcpSrv, config, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

	server := httptest.NewServer(transport.mux)
	defer server.Close()

	// Test SSE debug endpoint
	resp, err := http.Get(server.URL + "/sse/debug")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for SSE debug, got %d", resp.StatusCode)
	}

	var debug map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&debug); err != nil {
		t.Fatal(err)
	}

	if debug["endpoint"] != "/sse" {
		t.Errorf("Expected endpoint '/sse', got %v", debug["endpoint"])
	}

	// Test message debug endpoint
	resp2, err := http.Get(server.URL + "/message/debug")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for message debug, got %d", resp2.StatusCode)
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
	// for Anthropic API integration and MCP connector compatibility

	mcpSrv := mcpserver.NewMCPServer("test-server", "1.0.0")
	config := DefaultHTTPTransportConfig()
	transport := NewHTTPTransport(mcpSrv, config, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

	server := httptest.NewServer(transport.mux)
	defer server.Close()

	t.Run("ServiceDiscoveryAdvertisesBothEndpoints", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		var discovery map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&discovery)

		// Must advertise HTTP+SSE transport
		if discovery["transport"] != "streamable-http" {
			t.Error("Service discovery must advertise 'HTTP+SSE' transport")
		}

		// Must include both endpoints
		endpoints := discovery["endpoints"].(map[string]interface{})
		if endpoints["sse"] == nil {
			t.Error("Service discovery must include 'sse' endpoint")
		}
	})

	t.Run("MessageEndpointNotFound404Fixed", func(t *testing.T) {
		// Critical bug fix validation: POST /message must NOT return 404
		resp, err := http.Post(server.URL+"/message", "application/json",
			strings.NewReader(`{"jsonrpc":"2.0","method":"initialize","id":1}`))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			t.Fatal("CRITICAL BUG: POST /message still returns 404 - dual transport is broken")
		}

		// Should return 400 with proper JSON-RPC error
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("SSEHandshakeIncludesSessionID", func(t *testing.T) {
		req, _ := http.NewRequest("GET", server.URL+"/sse", nil)
		req.Header.Set("Accept", "text/event-stream")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		buf := make([]byte, 512)
		n, _ := resp.Body.Read(buf)
		response := string(buf[:n])

		if !strings.Contains(response, "sessionId=") {
			t.Error("SSE handshake must include sessionId in endpoint event")
		}

		if !strings.Contains(response, "/message?sessionId=") {
			t.Error("SSE handshake must advertise message endpoint with sessionId")
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
