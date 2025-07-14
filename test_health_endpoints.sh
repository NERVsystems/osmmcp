#!/bin/bash
# Test script to verify health endpoints are on the correct ports

echo "Starting OSM MCP server with HTTP transport..."
./osmmcp --enable-http --http-addr :7082 --enable-monitoring --monitoring-addr :9090 &
PID=$!

# Wait for server to start
sleep 3

echo -e "\nTesting health endpoints on port 7082 (MCP service port):"
echo "1. Testing /health endpoint:"
curl -s http://localhost:7082/health | jq . || echo "Failed"

echo -e "\n2. Testing /ready endpoint:"
curl -s http://localhost:7082/ready | jq . || echo "Failed"

echo -e "\n3. Testing /live endpoint:"
curl -s http://localhost:7082/live | jq . || echo "Failed"

echo -e "\n\nTesting Prometheus metrics on port 9090:"
echo "4. Testing /metrics endpoint:"
curl -s http://localhost:9090/metrics | head -10 || echo "Failed"

# Clean up
kill $PID 2>/dev/null
wait $PID 2>/dev/null

echo -e "\nTest complete!"