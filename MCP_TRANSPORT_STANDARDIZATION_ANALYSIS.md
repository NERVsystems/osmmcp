# OSM MCP Transport Standardization Analysis

---
## ⚠️ DOCUMENT STATUS: RESOLVED / ARCHIVED

**Original Analysis Date:** 2025-11-12
**Resolution Date:** 2025-11-14
**Current Status:** ✅ **CRITICAL ISSUE FIXED**

The critical dual transport issue identified in this analysis has been **RESOLVED**. The code in `cmd/osmmcp/main.go` (lines 242-286) now correctly implements dual transport support:
- HTTP transport runs in a goroutine (non-blocking) when `--enable-http` is set
- stdio transport ALWAYS runs on the main thread (blocking)
- Both transports operate simultaneously as required

This document is retained for historical reference only.

---

# [Original Analysis Below]

**Date:** 2025-11-12
**Branch:** `claude/mcp-transport-standardization-011CV3e4NuwUjWpRbnz42NvL`
**Status:** Analysis Complete

## Executive Summary

The OSM MCP server is **80% compliant** with the proposed transport standardization requirements. The server has most infrastructure in place but requires **one critical fix** and **minor adjustments** to achieve full compliance.

### Critical Issue 🚨

**Mutually Exclusive Transports:** The server currently runs EITHER stdio OR HTTP transport, not both simultaneously. This violates the core requirement for dual transport support.

### Current State vs Requirements

| Requirement | Status | Notes |
|-------------|--------|-------|
| `--enable-http` flag | ✅ Compliant | Already implemented |
| `--http-addr` flag | ✅ Compliant | Already implemented, default `:7082` |
| `--http-auth-type` flag | ✅ Compliant | Already implemented (none/bearer/basic) |
| Dual Transport (stdio + HTTP) | ❌ **BROKEN** | Currently mutually exclusive |
| HTTP Endpoints | ✅ Compliant | All required endpoints present |
| Health Check Format | ⚠️ Mostly Compliant | Minor field name differences |
| Degraded Mode | ✅ Compliant | Full implementation with connection monitoring |
| Prometheus Metrics | ✅ Compliant | Comprehensive metrics on separate port |

---

## Detailed Analysis

### 1. Flag Interface ✅

**Current Implementation:**
```go
// cmd/osmmcp/main.go:62-67
flag.BoolVar(&enableHTTP, "enable-http", false, "Enable HTTP+SSE transport (in addition to stdio)")
flag.StringVar(&httpAddr, "http-addr", ":7082", "HTTP server address")
flag.StringVar(&httpBaseURL, "http-base-url", "", "Base URL for HTTP transport (auto-detected if empty)")
flag.StringVar(&httpAuthType, "http-auth-type", "none", "HTTP authentication type: none, bearer, basic")
flag.StringVar(&httpAuthToken, "http-auth-token", "", "HTTP authentication token")
```

**Verdict:** ✅ Fully compliant - All required flags present with correct naming and defaults.

---

### 2. Dual Transport Support ❌

**Current Implementation (main.go:242-285):**
```go
// Start HTTP transport if enabled
var httpTransport *server.HTTPTransport
if enableHTTP {
    // ... HTTP transport setup ...
    if err := httpTransport.Start(); err != nil {
        os.Exit(1)  // BLOCKS HERE
    }
} else {
    // Run the MCP server with context (stdio transport)
    if err := s.RunWithContext(ctx); err != nil {
        os.Exit(1)
    }
}
```

**Problem:** This is an **if/else** condition - only ONE transport runs at a time.

**Required Behavior:**
- Stdio transport should ALWAYS run (main thread)
- HTTP transport should run in a goroutine when `--enable-http` is set
- Both must operate simultaneously

**Fix Required:**
```go
// Start HTTP transport in background if enabled
var httpTransport *server.HTTPTransport
if enableHTTP {
    // ... HTTP transport setup ...
    go func() {
        if err := httpTransport.Start(); err != nil && err != http.ErrServerClosed {
            logger.Error("HTTP transport error", "error", err)
        }
    }()
}

// ALWAYS run stdio transport on main thread (blocks)
if err := s.RunWithContext(ctx); err != nil {
    logger.Error("server error", "error", err)
    os.Exit(1)
}
```

**Impact:** This is the **single most critical change** needed for compliance.

---

### 3. HTTP Endpoints ✅

**Current Implementation (pkg/server/http_transport.go:109-129):**
```go
// Root endpoint for service discovery
t.mux.HandleFunc("/", t.handleServiceDiscovery)

// Health check endpoints (no auth required)
t.mux.HandleFunc("/health", t.handleHealth)
t.mux.HandleFunc("/ready", t.handleReady)
t.mux.HandleFunc("/live", t.handleLive)

// Debug endpoints
t.mux.HandleFunc("/sse/debug", t.handleSSEDebug)
t.mux.HandleFunc("/message/debug", t.handleMessageDebug)

// MCP endpoints (with auth)
t.mux.Handle("/sse", t.authMiddleware(t.sseServer.SSEHandler()))
t.mux.Handle("/message", t.authMiddleware(t.sseServer.MessageHandler()))
```

**Verdict:** ✅ All required endpoints present and properly implemented.

| Endpoint | Method | Purpose | Auth Required |
|----------|--------|---------|---------------|
| `/` | GET | Service discovery | No |
| `/health` | GET | Comprehensive health check | No |
| `/ready` | GET | Kubernetes readiness probe | No |
| `/live` | GET | Kubernetes liveness probe | No |
| `/sse` | GET | Server-Sent Events for MCP | Yes |
| `/message` | POST | JSON-RPC message endpoint | Yes |

---

### 4. Health Check Response Format ⚠️

**Required Format (from spec):**
```json
{
  "status": "healthy",
  "service": "osmmcp",
  "version": "1.0.0",
  "uptime_seconds": 12345,
  "connections": {
    "nominatim": {
      "status": "connected",
      "latency_ms": 15,
      "last_error": null
    }
  }
}
```

**Current Format (pkg/monitoring/health.go:112-130):**
```json
{
  "service": "osmmcp",
  "version": "1.0.0",
  "status": "healthy",
  "uptime": "2h30m15s",           // ⚠️ Duration string, not integer seconds
  "start_time": "2025-01-01...",  // ✅ Extra field (OK)
  "connections": {
    "nominatim": {
      "name": "nominatim",        // ⚠️ Redundant field
      "status": "connected",
      "latency_ms": 15,
      "error": "..."              // ⚠️ Should be "last_error"
    }
  },
  "metrics": { ... }              // ✅ Extra field (OK)
}
```

**Issues:**
1. **`uptime` field:** Currently a `time.Duration` (string like "2h30m"). Spec requires `uptime_seconds` as an integer.
2. **Connection `name` field:** Redundant (key already contains the name).
3. **Connection `error` field:** Should be `last_error` for consistency.

**Recommendation:** Add `uptime_seconds` field as an alias to maintain compatibility:

```go
// pkg/monitoring/metrics.go
type ServiceHealth struct {
    Service       string                 `json:"service"`
    Version       string                 `json:"version"`
    Status        string                 `json:"status"`
    Uptime        time.Duration          `json:"uptime"`              // Keep for backward compat
    UptimeSeconds int64                  `json:"uptime_seconds"`      // Add for spec compliance
    StartTime     time.Time              `json:"start_time,omitempty"`
    Connections   map[string]ConnStatus  `json:"connections"`
    Metrics       map[string]interface{} `json:"metrics,omitempty"`
}

type ConnStatus struct {
    Status    string `json:"status"`
    Latency   int64  `json:"latency_ms,omitempty"`
    LastError string `json:"last_error,omitempty"`  // Rename from "error"
}
```

**Impact:** Low - These are minor field adjustments that don't affect functionality.

---

### 5. Degraded Mode Support ✅

**Current Implementation:**

The OSM MCP server **already implements full degraded mode** support:

**Health Status Logic (pkg/monitoring/health.go:70-98):**
```go
func (h *HealthChecker) GetHealth() ServiceHealth {
    // ... determine overall status ...
    status := "healthy"

    if errorCount > len(h.connections)/2 {
        status = "unhealthy"  // More than half failing
    } else if errorCount > 0 {
        status = "degraded"   // Some failing
    } else if degradedCount > 0 {
        status = "degraded"   // Some degraded
    }

    return ServiceHealth{
        Status:      status,
        Connections: connections,
        // ...
    }
}
```

**External Service Monitoring (cmd/osmmcp/main.go:396-429):**
```go
// Monitor Nominatim service
nominatimMonitor := monitoring.NewConnectionMonitor(
    "nominatim",
    healthChecker,
    func() error {
        return osm.CheckNominatimHealth()
    },
    30*time.Second,
)
nominatimMonitor.Start()

// Monitor Overpass service
overpassMonitor := monitoring.NewConnectionMonitor(
    "overpass", healthChecker,
    func() error { return osm.CheckOverpassHealth() },
    30*time.Second,
)
overpassMonitor.Start()

// Monitor OSRM service
osrmMonitor := monitoring.NewConnectionMonitor(
    "osrm", healthChecker,
    func() error { return osm.CheckOSRMHealth() },
    30*time.Second,
)
osrmMonitor.Start()
```

**Verdict:** ✅ Fully compliant - Server starts successfully even if external services are unavailable. Health endpoint returns appropriate status based on dependency health.

**Example Degraded Response:**
```bash
$ curl http://localhost:7082/health
{
  "service": "osmmcp",
  "version": "0.1.0",
  "status": "degraded",
  "connections": {
    "nominatim": {
      "status": "connected",
      "latency_ms": 45
    },
    "overpass": {
      "status": "error",
      "error": "connection refused"
    },
    "osrm": {
      "status": "connected",
      "latency_ms": 23
    }
  }
}
```

---

### 6. Prometheus Metrics ✅

**Current Implementation:**

Metrics are served on a **separate monitoring port** (default: 9090):

**Monitoring Server (cmd/osmmcp/main.go:211-240):**
```go
if enableMonitoring {
    mux := http.NewServeMux()
    mux.Handle("/metrics", promhttp.Handler())

    monitoringServer = &http.Server{
        Addr:    monitoringAddr,  // Default: ":9090"
        Handler: mux,
    }

    go func() {
        logger.Info("starting Prometheus metrics server", "addr", monitoringAddr)
        if err := monitoringServer.ListenAndServe(); err != nil {
            logger.Error("monitoring server error", "error", err)
        }
    }()
}
```

**Available Metrics (pkg/monitoring/metrics.go):**
- `osmmcp_mcp_requests_total` - MCP requests by tool and status
- `osmmcp_mcp_request_duration_seconds` - Request duration histograms
- `osmmcp_external_service_requests_total` - External API requests
- `osmmcp_external_service_request_duration_seconds` - External API latency
- `osmmcp_rate_limit_exceeded_total` - Rate limit violations
- `osmmcp_rate_limit_wait_duration_seconds` - Rate limit wait time
- `osmmcp_cache_hits_total` / `osmmcp_cache_misses_total` - Cache statistics
- `osmmcp_cache_size` - Current cache size
- `osmmcp_active_connections` - Active connections by transport
- `osmmcp_errors_total` - Errors by component
- `osmmcp_goroutines` - Current goroutine count
- `osmmcp_memory_usage_bytes` - Memory usage
- `osmmcp_system_info` - Build and system information

**Verdict:** ✅ Comprehensive metrics implementation exceeds requirements.

---

## Implementation Plan

### Priority 1: Critical Fix 🚨

**Task:** Enable true dual transport support

**File:** `cmd/osmmcp/main.go`

**Current Code (lines 242-285):**
```go
if enableHTTP {
    // ... setup ...
    if err := httpTransport.Start(); err != nil {
        os.Exit(1)  // BLOCKS
    }
} else {
    if err := s.RunWithContext(ctx); err != nil {
        os.Exit(1)
    }
}
```

**Replace With:**
```go
// Start HTTP transport in background if enabled
var httpTransport *server.HTTPTransport
if enableHTTP {
    config := server.HTTPTransportConfig{
        Addr:        httpAddr,
        BaseURL:     httpBaseURL,
        AuthType:    httpAuthType,
        AuthToken:   httpAuthToken,
        SSEEndpoint: "/sse",
        MsgEndpoint: "/message",
    }

    httpTransport = server.NewHTTPTransport(s.GetMCPServer(), config, logger)

    if healthChecker != nil {
        httpTransport.SetHealthChecker(healthChecker)
    }

    // Start HTTP transport in goroutine (non-blocking)
    go func() {
        logger.Info("starting HTTP+SSE transport in background")
        if err := httpTransport.Start(); err != nil && err != http.ErrServerClosed {
            logger.Error("HTTP transport error", "error", err)
        }
    }()

    // Setup graceful shutdown for HTTP transport
    go func() {
        <-ctx.Done()
        shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()

        if err := httpTransport.Shutdown(shutdownCtx); err != nil {
            logger.Error("failed to shutdown HTTP transport", "error", err)
        }
    }()
}

// ALWAYS run stdio transport on main thread (blocking)
logger.Info("starting stdio MCP transport")
if err := s.RunWithContext(ctx); err != nil {
    logger.Error("server error", "error", err)
    os.Exit(1)
}
```

**Testing:**
```bash
# Test 1: Stdio only (default)
./osmmcp
# Should: Start successfully, stdio works, no HTTP port open

# Test 2: Dual transport mode
./osmmcp --enable-http --http-addr :7082
# Should: Start successfully, both stdio and HTTP work simultaneously

# Test 3: Verify both transports active
# Terminal 1:
./osmmcp --enable-http --http-addr :7082

# Terminal 2: Test HTTP health endpoint
curl http://localhost:7082/health

# Terminal 3: Test stdio transport (via Claude Desktop or MCP inspector)
# Send MCP protocol messages via stdin
```

---

### Priority 2: Health Check Format Adjustments

**Task:** Align health check response with spec format

**File:** `pkg/monitoring/metrics.go`

**Changes:**
1. Add `UptimeSeconds` field to `ServiceHealth` struct
2. Rename `Error` to `LastError` in `ConnStatus` struct
3. Remove redundant `Name` field from `ConnStatus`

**Updated Structs:**
```go
type ServiceHealth struct {
    Service       string                 `json:"service"`
    Version       string                 `json:"version"`
    Status        string                 `json:"status"`
    Uptime        time.Duration          `json:"uptime"`              // Keep for backward compatibility
    UptimeSeconds int64                  `json:"uptime_seconds"`      // Add for spec compliance
    StartTime     time.Time              `json:"start_time,omitempty"`
    Connections   map[string]ConnStatus  `json:"connections"`
    Metrics       map[string]interface{} `json:"metrics,omitempty"`
}

type ConnStatus struct {
    Status    string `json:"status"`
    Latency   int64  `json:"latency_ms,omitempty"`
    LastError string `json:"last_error,omitempty"`  // Renamed from "error"
}
```

**File:** `pkg/monitoring/health.go`

**Update GetHealth() method:**
```go
func (h *HealthChecker) GetHealth() ServiceHealth {
    // ... existing logic ...

    uptime := time.Since(h.startTime)

    return ServiceHealth{
        Service:       h.serviceName,
        Version:       h.version,
        Status:        status,
        Uptime:        uptime,                    // Keep for backward compat
        UptimeSeconds: int64(uptime.Seconds()),   // Add for spec compliance
        StartTime:     h.startTime,
        Connections:   connections,
        Metrics:       metricsData,
    }
}
```

**Update UpdateConnection() method:**
```go
func (h *HealthChecker) UpdateConnection(name, status string, latencyMs int64, err error) {
    h.mu.Lock()
    defer h.mu.Unlock()

    lastError := ""
    if err != nil {
        lastError = err.Error()
    }

    h.connections[name] = &ConnStatus{
        Status:    status,
        Latency:   latencyMs,
        LastError: lastError,  // Use "last_error" field
    }
}
```

**Testing:**
```bash
# Test health endpoint format
./osmmcp --enable-http --http-addr :7082

# In another terminal:
curl http://localhost:7082/health | jq .

# Expected output:
# {
#   "service": "osmmcp",
#   "version": "0.1.0",
#   "status": "healthy",
#   "uptime": "5m30s",
#   "uptime_seconds": 330,
#   "connections": {
#     "nominatim": {
#       "status": "connected",
#       "latency_ms": 45,
#       "last_error": null
#     }
#   }
# }
```

---

### Priority 3: Testing and Validation

**Test Suite:**

```bash
# Test 1: Stdio-only mode (default)
./osmmcp
# Expected: Start successfully, stdio works, no HTTP port open

# Test 2: Dual transport mode
./osmmcp --enable-http --http-addr :7082
# Expected: Start successfully, stdio works, HTTP health check responds

# Test 3: Health check format
curl http://localhost:7082/health | jq .
# Expected: JSON with required fields (uptime_seconds, last_error)

# Test 4: Degraded mode
# (Disable network to external services)
sudo iptables -A OUTPUT -d nominatim.openstreetmap.org -j DROP
curl http://localhost:7082/health | jq .
# Expected: status "degraded", connection errors shown

# Test 5: Concurrent requests
# Send stdio MCP requests while hitting HTTP health endpoint
# Expected: Both work simultaneously without interference

# Test 6: Monitoring metrics
curl http://localhost:9090/metrics | grep osmmcp_
# Expected: Prometheus metrics available
```

---

## Summary

### What's Already Compliant ✅

1. **Flag Interface** - All required flags present (`--enable-http`, `--http-addr`, `--http-auth-type`)
2. **HTTP Endpoints** - All required endpoints implemented (`/health`, `/ready`, `/live`, `/sse`, `/message`)
3. **Degraded Mode** - Full support with connection monitoring for external services
4. **Prometheus Metrics** - Comprehensive metrics on separate monitoring port
5. **Service Discovery** - Root endpoint provides transport capabilities

### What Needs Fixing ❌

1. **Dual Transport Support** - Critical issue: Currently mutually exclusive, need goroutine for HTTP
2. **Health Check Format** - Minor adjustments needed (`uptime_seconds`, `last_error` field names)

### Compliance Score

**Overall: 80% Compliant**

- Flags: 100% ✅
- Dual Transport: 0% ❌ (critical blocker)
- Endpoints: 100% ✅
- Health Format: 70% ⚠️ (minor issues)
- Degraded Mode: 100% ✅
- Metrics: 100% ✅

### Estimated Effort

- **Priority 1 (Dual Transport):** 2-4 hours (critical)
- **Priority 2 (Health Format):** 1-2 hours (low priority)
- **Priority 3 (Testing):** 2-3 hours (validation)

**Total:** 5-9 hours of development work

---

## Next Steps

1. **Review this analysis** with the team
2. **Implement Priority 1** (dual transport fix) - this is the blocker
3. **Implement Priority 2** (health format adjustments) - nice to have
4. **Run comprehensive test suite** to validate changes
5. **Update documentation** (CLAUDE.md) with new dual transport behavior
6. **Commit and push** to the feature branch
7. **Create PR** with changes and test results

---

## References

- **Main Implementation:** `cmd/osmmcp/main.go`
- **HTTP Transport:** `pkg/server/http_transport.go`
- **Health Monitoring:** `pkg/monitoring/health.go`
- **Metrics:** `pkg/monitoring/metrics.go`
- **Server Core:** `pkg/server/server.go`
