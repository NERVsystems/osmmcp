# OSM MCP Server Dockerfile
# OpenStreetMap geospatial services (geocoding, routing, POI search)
# Port: 7082

# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install git and ca-certificates for Go modules
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-w -s" -o osmmcp ./cmd/osmmcp

# Runtime stage
FROM alpine:3.23

# Install ca-certificates for HTTPS, curl for health checks
RUN apk --no-cache add ca-certificates curl && \
    addgroup -g 1000 osmmcp && \
    adduser -D -u 1000 -G osmmcp osmmcp

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/osmmcp /usr/local/bin/osmmcp

# Create working directory
RUN chown -R osmmcp:osmmcp /app

USER osmmcp

# OSM MCP port (matches NERVA port standard)
EXPOSE 7082

# Monitoring port
EXPOSE 9091

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -sf http://localhost:7082/ || exit 1

# Default command matching startup script:
# --enable-http --http-addr localhost:7082 --http-auth-type none --debug
# --user-agent "nerva-osm-mcp/1.0.0"
# --nominatim-rps 1.0 --nominatim-burst 2
# --overpass-rps 1.0 --overpass-burst 2
# --osrm-rps 1.0 --osrm-burst 2
# --monitoring-addr localhost:9091
ENTRYPOINT ["/usr/local/bin/osmmcp"]
CMD ["--enable-http", \
     "--http-addr", "0.0.0.0:7082", \
     "--http-auth-type", "none", \
     "--user-agent", "nerva-osm-mcp/1.0.0", \
     "--nominatim-rps", "1.0", \
     "--nominatim-burst", "2", \
     "--overpass-rps", "1.0", \
     "--overpass-burst", "2", \
     "--osrm-rps", "1.0", \
     "--osrm-burst", "2", \
     "--monitoring-addr", "0.0.0.0:9091"]
