// Package registration provides service registration with nerva-monitor.
// Registration is optional and fails gracefully - the server will always function
// even if the registry is unavailable.
package registration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// DefaultHeartbeatInterval is the default interval between heartbeats.
const DefaultHeartbeatInterval = 30 * time.Second

// DefaultTimeout is the default timeout for HTTP requests.
const DefaultTimeout = 5 * time.Second

// Config holds the configuration for service registration.
type Config struct {
	// Enabled controls whether registration is active (default: false)
	Enabled bool

	// RegistryURL is the URL of the nerva-monitor registry endpoint
	// e.g., "http://nerva-monitor:7083"
	RegistryURL string

	// ServiceName is the unique name of this service
	ServiceName string

	// ServiceType is the type of service (usually "mcp")
	ServiceType string

	// ServiceURL is the external URL where this service is accessible
	ServiceURL string

	// HealthURL is the URL for health checks
	HealthURL string

	// InternalURL is the internal URL (optional, for container environments)
	InternalURL string

	// InternalHealthURL is the internal health URL (optional)
	InternalHealthURL string

	// Version is the service version
	Version string

	// Capabilities is a list of capabilities this service provides
	Capabilities []string

	// Tools is a list of MCP tools this service provides
	Tools []string

	// Metadata is additional metadata about the service
	Metadata map[string]interface{}

	// HeartbeatInterval is how often to send heartbeats (default: 30s)
	HeartbeatInterval time.Duration

	// Timeout is the HTTP request timeout (default: 5s)
	Timeout time.Duration
}

// RegistrationRequest is the request format for the registry API.
type RegistrationRequest struct {
	Name           string                 `json:"name"`
	Type           string                 `json:"type"`
	URL            string                 `json:"url"`
	HealthURL      string                 `json:"health_url"`
	InternalURL    string                 `json:"internal_url,omitempty"`
	InternalHealth string                 `json:"internal_health_url,omitempty"`
	Version        string                 `json:"version"`
	Capabilities   []string               `json:"capabilities,omitempty"`
	Tools          []string               `json:"tools,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// RegistrationResponse is the response from the registry.
type RegistrationResponse struct {
	Status          string    `json:"status"`
	Name            string    `json:"name"`
	TTLSeconds      int       `json:"ttl_seconds"`
	NextHeartbeatBy time.Time `json:"next_heartbeat_by"`
}

// Client handles registration with nerva-monitor.
type Client struct {
	cfg        Config
	logger     *slog.Logger
	httpClient *http.Client
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	registered bool
	mu         sync.RWMutex
}

// NewClient creates a new registration client.
// If cfg.Enabled is false, the client will be a no-op.
func NewClient(cfg Config, logger *slog.Logger) *Client {
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = DefaultHeartbeatInterval
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.ServiceType == "" {
		cfg.ServiceType = "mcp"
	}

	return &Client{
		cfg:    cfg,
		logger: logger,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// Start begins the registration and heartbeat loop.
// This method is non-blocking and returns immediately.
// If registration is disabled, this is a no-op.
func (c *Client) Start(ctx context.Context) {
	if !c.cfg.Enabled {
		c.logger.Info("service registration disabled")
		return
	}

	if c.cfg.RegistryURL == "" {
		c.logger.Warn("service registration enabled but no registry URL configured")
		return
	}

	ctx, c.cancel = context.WithCancel(ctx)
	c.wg.Add(1)
	go c.heartbeatLoop(ctx)
}

// Stop gracefully stops the registration client.
// It attempts to deregister from the registry before stopping.
func (c *Client) Stop() {
	if !c.cfg.Enabled || c.cancel == nil {
		return
	}

	// Try to deregister
	c.deregister()

	// Cancel the heartbeat loop
	c.cancel()
	c.wg.Wait()
}

// IsRegistered returns whether the service is currently registered.
func (c *Client) IsRegistered() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.registered
}

// heartbeatLoop sends periodic heartbeats to the registry.
func (c *Client) heartbeatLoop(ctx context.Context) {
	defer c.wg.Done()

	// Initial registration
	c.register()

	ticker := time.NewTicker(c.cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.register()
		case <-ctx.Done():
			return
		}
	}
}

// register sends a registration/heartbeat request to the registry.
func (c *Client) register() {
	req := RegistrationRequest{
		Name:           c.cfg.ServiceName,
		Type:           c.cfg.ServiceType,
		URL:            c.cfg.ServiceURL,
		HealthURL:      c.cfg.HealthURL,
		InternalURL:    c.cfg.InternalURL,
		InternalHealth: c.cfg.InternalHealthURL,
		Version:        c.cfg.Version,
		Capabilities:   c.cfg.Capabilities,
		Tools:          c.cfg.Tools,
		Metadata:       c.cfg.Metadata,
	}

	body, err := json.Marshal(req)
	if err != nil {
		c.logger.Error("failed to marshal registration request", "error", err)
		c.setRegistered(false)
		return
	}

	url := fmt.Sprintf("%s/api/register", c.cfg.RegistryURL)
	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		c.logger.Error("failed to create registration request", "error", err)
		c.setRegistered(false)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.logger.Debug("registration failed (registry may be unavailable)", "error", err)
		c.setRegistered(false)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		c.logger.Warn("registration failed", "status", resp.StatusCode, "body", string(bodyBytes))
		c.setRegistered(false)
		return
	}

	var regResp RegistrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&regResp); err != nil {
		c.logger.Warn("failed to decode registration response", "error", err)
		c.setRegistered(false)
		return
	}

	wasRegistered := c.IsRegistered()
	c.setRegistered(true)

	if !wasRegistered {
		c.logger.Info("registered with nerva-monitor",
			"name", c.cfg.ServiceName,
			"ttl_seconds", regResp.TTLSeconds,
		)
	}
}

// deregister sends a deregistration request to the registry.
func (c *Client) deregister() {
	if !c.IsRegistered() {
		return
	}

	url := fmt.Sprintf("%s/api/register/%s", c.cfg.RegistryURL, c.cfg.ServiceName)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, url, nil)
	if err != nil {
		c.logger.Debug("failed to create deregistration request", "error", err)
		return
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Debug("deregistration failed (registry may be unavailable)", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		c.logger.Info("deregistered from nerva-monitor", "name", c.cfg.ServiceName)
	}

	c.setRegistered(false)
}

// setRegistered updates the registration status.
func (c *Client) setRegistered(registered bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.registered = registered
}
