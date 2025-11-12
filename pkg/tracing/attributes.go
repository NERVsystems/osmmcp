package tracing

import "go.opentelemetry.io/otel/attribute"

// Attribute keys for MCP operations
const (
	// MCP tool attributes
	AttrMCPToolName     = "mcp.tool.name"
	AttrMCPToolStatus   = "mcp.tool.status"
	AttrMCPToolDuration = "mcp.tool.duration_ms"
	AttrMCPResultSize   = "mcp.tool.result_size"

	// External service attributes
	AttrServiceName      = "osm.service.name"
	AttrServiceOperation = "osm.service.operation"
	AttrServiceURL       = "osm.service.url"
	AttrServiceStatus    = "osm.service.status"

	// Cache attributes
	AttrCacheType = "osm.cache.type"
	AttrCacheHit  = "osm.cache.hit"
	AttrCacheKey  = "osm.cache.key"

	// Rate limiting attributes
	AttrRateLimitService = "osm.ratelimit.service"
	AttrRateLimitWaitMs  = "osm.ratelimit.wait_ms"

	// HTTP transport attributes
	AttrHTTPMethod     = "http.method"
	AttrHTTPStatusCode = "http.status_code"
	AttrHTTPPath       = "http.path"
	AttrHTTPSessionID  = "http.session_id"

	// Error attributes
	AttrErrorType    = "error.type"
	AttrErrorMessage = "error.message"
)

// Status values
const (
	StatusSuccess     = "success"
	StatusError       = "error"
	StatusTimeout     = "timeout"
	StatusRateLimited = "rate_limited"
)

// Service names
const (
	ServiceNominatim = "nominatim"
	ServiceOverpass  = "overpass"
	ServiceOSRM      = "osrm"
	ServiceTiles     = "tiles"
)

// Cache types
const (
	CacheTypeOSM  = "osm"
	CacheTypeTile = "tile"
)

// Helper functions for common attributes

// MCPToolAttributes returns attributes for MCP tool execution
func MCPToolAttributes(toolName string, status string, durationMs int64, resultSize int) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(AttrMCPToolName, toolName),
		attribute.String(AttrMCPToolStatus, status),
		attribute.Int64(AttrMCPToolDuration, durationMs),
		attribute.Int(AttrMCPResultSize, resultSize),
	}
}

// ServiceAttributes returns attributes for external service calls
func ServiceAttributes(service, operation, url string, status int) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(AttrServiceName, service),
		attribute.String(AttrServiceOperation, operation),
		attribute.String(AttrServiceURL, url),
		attribute.Int(AttrServiceStatus, status),
	}
}

// CacheAttributes returns attributes for cache operations
func CacheAttributes(cacheType string, hit bool, key string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(AttrCacheType, cacheType),
		attribute.Bool(AttrCacheHit, hit),
		attribute.String(AttrCacheKey, key),
	}
}

// ErrorAttributes returns attributes for errors
func ErrorAttributes(err error) []attribute.KeyValue {
	if err == nil {
		return nil
	}
	return []attribute.KeyValue{
		attribute.String(AttrErrorType, "error"),
		attribute.String(AttrErrorMessage, err.Error()),
	}
}
