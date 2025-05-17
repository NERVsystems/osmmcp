// Package core provides shared utilities for the OpenStreetMap MCP tools.
package core

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
)

// ErrorCode defines standard error codes for MCP tools
type ErrorCode string

// Standard error codes
const (
	// Input validation errors
	ErrInvalidInput     ErrorCode = "INVALID_INPUT"
	ErrInvalidLatitude  ErrorCode = "INVALID_LATITUDE"
	ErrInvalidLongitude ErrorCode = "INVALID_LONGITUDE"
	ErrInvalidRadius    ErrorCode = "INVALID_RADIUS"
	ErrRadiusTooLarge   ErrorCode = "RADIUS_TOO_LARGE"
	ErrEmptyParameter   ErrorCode = "EMPTY_PARAMETER"
	ErrMissingParameter ErrorCode = "MISSING_PARAMETER"
	ErrInvalidParameter ErrorCode = "INVALID_PARAMETER"

	// Service errors
	ErrServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	ErrServiceTimeout     ErrorCode = "SERVICE_TIMEOUT"
	ErrRateLimit          ErrorCode = "RATE_LIMIT"
	ErrNetworkError       ErrorCode = "NETWORK_ERROR"

	// Data errors
	ErrNoResults     ErrorCode = "NO_RESULTS"
	ErrParseError    ErrorCode = "PARSE_ERROR"
	ErrInternalError ErrorCode = "INTERNAL_ERROR"
)

// MCPError represents a detailed error structure for MCP tool responses
type MCPError struct {
	Code        string   `json:"code"`
	Message     string   `json:"message"`
	Query       string   `json:"query,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
	Guidance    string   `json:"guidance,omitempty"`
}

// Error implements the error interface
func (e MCPError) Error() string {
	if e.Guidance != "" {
		return fmt.Sprintf("%s: %s. %s", e.Code, e.Message, e.Guidance)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NewError creates a new MCPError with the given code and message
func NewError(code ErrorCode, message string) *MCPError {
	return &MCPError{
		Code:    string(code),
		Message: message,
	}
}

// WithQuery adds query information to the error
func (e *MCPError) WithQuery(query string) *MCPError {
	e.Query = query
	return e
}

// WithGuidance adds guidance information to the error
func (e *MCPError) WithGuidance(guidance string) *MCPError {
	e.Guidance = guidance
	return e
}

// WithSuggestions adds suggestions to the error
func (e *MCPError) WithSuggestions(suggestions ...string) *MCPError {
	e.Suggestions = append(e.Suggestions, suggestions...)
	return e
}

// ToMCPResult converts the error to an MCP tool result
func (e *MCPError) ToMCPResult() *mcp.CallToolResult {
	// Marshal to JSON
	errorJSON, err := json.Marshal(e)
	if err != nil {
		// Fallback if marshaling fails
		return mcp.NewToolResultError(fmt.Sprintf("ERROR: %s - %s", e.Code, e.Message))
	}

	return mcp.NewToolResultError(string(errorJSON))
}

// ServiceError creates an error for external service failures
func ServiceError(service string, statusCode int, message string) *MCPError {
	var code ErrorCode
	var guidance string

	switch statusCode {
	case http.StatusTooManyRequests:
		code = ErrRateLimit
		guidance = "The service is rate-limited. Please try again in a few moments."
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		code = ErrServiceTimeout
		guidance = "The request timed out. Try reducing the search area or simplifying the query."
	case http.StatusBadRequest:
		code = ErrInvalidInput
		guidance = "The request was invalid. Check your parameters and try again."
	case http.StatusInternalServerError:
		code = ErrInternalError
		guidance = "The server encountered an error. This is likely temporary, please try again later."
	case http.StatusServiceUnavailable:
		code = ErrServiceUnavailable
		guidance = "The service is temporarily unavailable. Please try again later."
	default:
		code = ErrServiceUnavailable
		guidance = "Please try again later or modify your request parameters."
	}

	return NewError(code, fmt.Sprintf("%s service error: %s", service, message)).
		WithGuidance(guidance)
}

// NewValidationError creates an error for validation failures
func NewValidationError(code ErrorCode, message string) *MCPError {
	return NewError(code, message).
		WithGuidance("Please correct the parameters and try again.")
}
