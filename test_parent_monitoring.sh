#!/bin/bash

# Test script to verify parent process monitoring works correctly
# This simulates Claude Desktop starting and then exiting

echo "Testing parent process monitoring..."

# Start a parent process that will exit after a few seconds
(
    echo "Parent process ($$) starting MCP server..."
    
    # Start the MCP server as a child process
    ./osmmcp --debug &
    SERVER_PID=$!
    
    echo "Started MCP server with PID: $SERVER_PID"
    echo "Parent process will exit in 3 seconds..."
    
    # Wait 3 seconds then exit (simulating Claude Desktop exit)
    sleep 3
    echo "Parent process exiting..."
    
    # Check if server is still running after parent exits
    sleep 2
    if kill -0 $SERVER_PID 2>/dev/null; then
        echo "ERROR: MCP server is still running after parent exit"
        kill $SERVER_PID
        exit 1
    else
        echo "SUCCESS: MCP server properly shut down when parent exited"
        exit 0
    fi
) &

PARENT_PID=$!
echo "Started parent process with PID: $PARENT_PID"

# Wait for the test to complete
wait $PARENT_PID
TEST_RESULT=$?

echo "Test completed with result: $TEST_RESULT"
exit $TEST_RESULT