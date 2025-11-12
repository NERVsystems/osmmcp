package tools

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// IsErrorResult checks if a CallToolResult represents an error
func IsErrorResult(result *mcp.CallToolResult) bool {
	if result == nil {
		return false
	}

	// Check if the isError flag is set
	return result.IsError
}

// AssertErrorResult checks that a result is an error result and fails the test if not
func AssertErrorResult(t *testing.T, result *mcp.CallToolResult, message string) {
	if !IsErrorResult(result) {
		t.Error(message)
	}
}

// AssertSuccessResult checks that a result is a success result and fails the test if not
func AssertSuccessResult(t *testing.T, result *mcp.CallToolResult, message string) {
	if IsErrorResult(result) {
		// Extract the error text for better test output
		var errorText string
		for _, content := range result.Content {
			if text, ok := content.(mcp.TextContent); ok {
				errorText = text.Text
				break
			}
		}
		t.Errorf("%s. Got error: %s", message, errorText)
	}
}

// ParseResultJSON parses the JSON content from a CallToolResult
func ParseResultJSON(result *mcp.CallToolResult, out interface{}) error {
	var content string
	for _, c := range result.Content {
		if text, ok := c.(mcp.TextContent); ok {
			content = text.Text
			break
		}
	}

	return json.Unmarshal([]byte(content), out)
}
