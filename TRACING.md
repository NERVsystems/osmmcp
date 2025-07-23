# OpenTelemetry Tracing

OSMMCP supports distributed tracing via OpenTelemetry (OTLP) for enhanced observability and debugging.

## Configuration

### Environment Variable
```bash
export OTLP_ENDPOINT=localhost:4317
```

## What's Traced

### MCP Tool Execution
Each tool call creates a span with:
- **Tool name**: The MCP tool being executed
- **Execution duration**: Total time for tool execution
- **Success/failure status**: Whether the tool succeeded or failed
- **Error details**: Stack traces and error messages (if applicable)
- **Result size**: Size of the JSON response

**Span Attributes**:
- `mcp.tool.name`: Name of the executed tool
- `mcp.tool.status`: success or error
- `mcp.tool.duration_ms`: Execution time in milliseconds
- `mcp.tool.result_size`: Size of the JSON result

### External Service Calls
All HTTP requests to OSM services are traced:
- **Nominatim**: Geocoding and reverse geocoding
- **Overpass API**: OSM data queries
- **OSRM**: Routing calculations

**Span Attributes**:
- `http.method`: GET, POST, etc.
- `http.url`: Full request URL
- `http.status_code`: Response status code
- `http.retry.attempts`: Number of attempts made
- `http.retry.final_error`: Final error if all retries failed

**Rate Limiting**:
- `osm.ratelimit.service`: Which service was rate limited
- `osm.ratelimit.wait_ms`: Time spent waiting for rate limit

### Cache Operations
Cache interactions are traced for both OSM and tile caches:

**Span Attributes**:
- `osm.cache.type`: "osm" or "tile"
- `osm.cache.hit`: Boolean indicating cache hit/miss
- `osm.cache.key`: The cache key
- `cache.expired`: Whether item was expired (for misses)
- `cache.eviction_triggered`: Whether eviction occurred

### HTTP Transport (when enabled)
All HTTP requests to the MCP server are traced:

**Span Attributes**:
- `http.method`: Request method
- `http.path`: Request path
- `http.status_code`: Response status
- `http.session_id`: Session ID for correlation
- `http.response.size`: Response body size

## Trace Propagation

OSMMCP uses W3C Trace Context for trace propagation. When integrated with other OpenTelemetry-enabled services, traces will be automatically correlated across service boundaries.

## Integration with Observability Platforms

### Jaeger
```bash
# Run Jaeger with OTLP support
docker run -d --name jaeger \
  -e COLLECTOR_OTLP_ENABLED=true \
  -p 16686:16686 \
  -p 4317:4317 \
  jaegertracing/all-in-one:latest

# Start OSMMCP with tracing
export OTLP_ENDPOINT=localhost:4317
./osmmcp

# View traces at http://localhost:16686
```

### Grafana Tempo
```bash
# Run Tempo
docker run -d --name tempo \
  -p 3200:3200 \
  -p 4317:4317 \
  -v $(pwd)/tempo.yaml:/etc/tempo.yaml \
  grafana/tempo:latest \
  -config.file=/etc/tempo.yaml

# Configure Grafana to use Tempo as a data source
```

### Example tempo.yaml
```yaml
server:
  http_listen_port: 3200

distributor:
  receivers:
    otlp:
      protocols:
        grpc:
          endpoint: 0.0.0.0:4317

storage:
  trace:
    backend: local
    local:
      path: /tmp/tempo/traces
```

## Example Usage

1. Start your OTLP collector (Jaeger, Tempo, etc.)
2. Set the OTLP endpoint:
   ```bash
   export OTLP_ENDPOINT=localhost:4317
   ```
3. Start OSMMCP:
   ```bash
   ./osmmcp
   ```
4. Use the MCP tools - traces will be automatically generated
5. View traces in your observability platform

## Understanding Traces

### Example: Route Finding
When using `osm_find_route`, you'll see:
1. **Parent span**: `mcp.tool.osm_find_route`
2. **Child spans**:
   - `http.request GET nominatim.openstreetmap.org` (geocoding start)
   - `http.request GET nominatim.openstreetmap.org` (geocoding end)
   - `http.request GET router.project-osrm.org` (routing)
   - `cache.get` operations for checking cached results
   - `cache.set` operations for storing results

### Example: Cache Hit
For cached operations:
1. **Parent span**: `mcp.tool.geocode_address`
2. **Child span**: `cache.get` with `osm.cache.hit=true`
3. No external HTTP requests (data served from cache)

## Performance Impact

Tracing has minimal performance impact:
- **No-op tracer** used when OTLP endpoint not configured
- **Asynchronous span export** doesn't block operations
- **Sampling**: Currently set to AlwaysSample for development
  - For production, consider using a probabilistic sampler

## Troubleshooting

### No traces appearing
1. Verify OTLP endpoint is set correctly:
   ```bash
   echo $OTLP_ENDPOINT
   ```
2. Check collector is running and accessible:
   ```bash
   telnet localhost 4317
   ```
3. Look for initialization messages in OSMMCP logs:
   ```
   OpenTelemetry tracing enabled endpoint=localhost:4317
   ```

### Incomplete traces
- Ensure all services use the same trace propagation format (W3C Trace Context)
- Check that context is properly passed through all function calls
- Verify no goroutines are created without propagating context

### High memory usage
- Consider implementing sampling to reduce trace volume
- Configure span limits in the SDK
- Use a remote collector instead of direct export

## Future Enhancements

- Configurable sampling strategies
- Trace-based testing for integration tests
- Custom span processors for sensitive data filtering
- Baggage propagation for request metadata
- Exemplar support linking metrics to traces