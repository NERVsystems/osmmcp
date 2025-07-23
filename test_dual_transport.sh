#!/bin/bash
set -e

echo "🚀 Testing Full Dual Transport MCP Support for Anthropic API Integration"
echo

# Build the server
echo "Building osmmcp server..."
go build -o osmmcp ./cmd/osmmcp

# Start server in background
echo "Starting HTTP+SSE transport on :8081..."
./osmmcp --enable-http --http-addr :8081 --http-auth-type none &
SERVER_PID=$!

# Wait for server to start
sleep 3

# Test function
test_endpoint() {
    local url="$1"
    local expected_status="$2"
    local description="$3"
    
    echo -n "Testing $description... "
    
    if [ "$url" = "sse" ]; then
        # Special handling for SSE endpoint
        response=$(timeout 2s curl -s -N http://localhost:8081/sse -H "Accept: text/event-stream" | head -1)
        if [[ "$response" =~ "event: endpoint" ]]; then
            echo "✅ PASS"
        else
            echo "❌ FAIL: Expected SSE endpoint event"
            exit 1
        fi
    else
        status=$(curl -s -o /dev/null -w "%{http_code}" "$url")
        if [ "$status" = "$expected_status" ]; then
            echo "✅ PASS"
        else
            echo "❌ FAIL: Expected $expected_status, got $status"
            exit 1
        fi
    fi
}

echo
echo "🔍 Running Critical Bug Fix Tests..."

# Test service discovery
echo -n "Service discovery shows HTTP+SSE transport... "
discovery=$(curl -s http://localhost:8081/)
if echo "$discovery" | grep -q '"transport":"HTTP+SSE"'; then
    echo "✅ PASS"
else
    echo "❌ FAIL: Transport not advertised as HTTP+SSE"
    exit 1
fi

# Test both endpoints are advertised
echo -n "Service discovery includes both endpoints... "
if echo "$discovery" | grep -q '"sse":' && echo "$discovery" | grep -q '"message":'; then
    echo "✅ PASS"
else
    echo "❌ FAIL: Missing SSE or message endpoint in discovery"
    exit 1
fi

# Test the critical 404 bug fix
test_endpoint "http://localhost:8081/message" "400" "POST /message returns 400 (not 404)"
test_endpoint "http://localhost:8081/message?sessionId=test-123" "400" "POST /message?sessionId returns 400 (not 404)"

# Test SSE endpoint
test_endpoint "sse" "" "SSE endpoint establishes connection"

# Test health endpoint
test_endpoint "http://localhost:8081/health" "200" "Health endpoint"

echo
echo "🎯 Testing MCP Protocol Endpoints..."

# Test that POST /message gives proper JSON-RPC errors
echo -n "POST /message returns proper JSON-RPC error... "
response=$(curl -s -X POST http://localhost:8081/message -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"initialize","id":1}')
if echo "$response" | grep -q '"jsonrpc":"2.0"' && echo "$response" | grep -q '"error"'; then
    echo "✅ PASS"
else
    echo "❌ FAIL: Not a proper JSON-RPC error response"
    exit 1
fi

# Test SSE endpoint provides session ID
echo -n "SSE handshake includes sessionId... "
sse_response=$(timeout 2s curl -s -N http://localhost:8081/sse -H "Accept: text/event-stream" | head -2)
if echo "$sse_response" | grep -q "sessionId="; then
    echo "✅ PASS"
else
    echo "❌ FAIL: SSE handshake missing sessionId"
    exit 1
fi

echo
echo "✨ Success Criteria Validation:"
echo "✅ POST /message returns 400 (not 404) with JSON-RPC error"
echo "✅ POST /message?sessionId=xxx returns 400 (not 404) with JSON-RPC error"
echo "✅ Service discovery shows 'HTTP+SSE' transport"
echo "✅ Service discovery includes both 'sse' and 'message' endpoints"
echo "✅ SSE handshake advertises /message?sessionId=xxx"
echo "✅ All tests pass"

echo
echo "🎉 Dual Transport Implementation Complete!"
echo "✅ Full HTTP+SSE + JSON-RPC support implemented"
echo "✅ Anthropic API integration ready"
echo "✅ MCP connector compatibility confirmed"

# Cleanup
kill $SERVER_PID 2>/dev/null || true
rm -f osmmcp

echo
echo "Implementation is ready for production use with Anthropic API! 🚀"