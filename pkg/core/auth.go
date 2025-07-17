package core

import (
	"crypto/subtle"
	"strings"
	"time"
)

// SecureCompareString performs constant-time string comparison to prevent timing attacks
func SecureCompareString(a, b string) bool {
	// Convert strings to byte slices for constant-time comparison
	aBytes := []byte(a)
	bBytes := []byte(b)

	// If lengths are different, the strings are not equal
	if len(aBytes) != len(bBytes) {
		return false
	}

	// Use constant-time comparison
	return subtle.ConstantTimeCompare(aBytes, bBytes) == 1
}

// ValidateAuthToken validates an authentication token with security best practices
func ValidateAuthToken(token string) error {
	if token == "" {
		return NewError(ErrInvalidParameter, "Authentication token cannot be empty").
			WithGuidance("Provide a valid authentication token for security.")
	}

	// Minimum length requirement for security
	if len(token) < 16 {
		return NewError(ErrInvalidParameter, "Authentication token is too short").
			WithGuidance("Use a token with at least 16 characters for security.")
	}

	// Check for common weak tokens
	weakTokens := []string{
		"password", "secret", "token", "admin", "test", "default",
		"12345", "123456", "password123", "secret123", "admin123",
	}

	lowerToken := strings.ToLower(token)
	for _, weak := range weakTokens {
		if strings.Contains(lowerToken, weak) {
			return NewError(ErrInvalidParameter, "Authentication token appears to be weak").
				WithGuidance("Use a randomly generated, strong authentication token.")
		}
	}

	return nil
}

// AuthResult represents the result of authentication
type AuthResult struct {
	Authorized bool
	Error      string
	Duration   time.Duration
}

// AuthenticateBearer performs secure bearer token authentication
func AuthenticateBearer(authHeader, expectedToken string) AuthResult {
	start := time.Now()
	defer func() {
		// Add small delay to prevent timing attacks
		time.Sleep(1 * time.Millisecond)
	}()

	if authHeader == "" {
		return AuthResult{
			Authorized: false,
			Error:      "Missing Authorization header",
			Duration:   time.Since(start),
		}
	}

	// Extract token from "Bearer <token>" format
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return AuthResult{
			Authorized: false,
			Error:      "Invalid Authorization header format",
			Duration:   time.Since(start),
		}
	}

	token := parts[1]

	// Secure comparison
	if !SecureCompareString(token, expectedToken) {
		return AuthResult{
			Authorized: false,
			Error:      "Invalid bearer token",
			Duration:   time.Since(start),
		}
	}

	return AuthResult{
		Authorized: true,
		Duration:   time.Since(start),
	}
}

// AuthenticateBasic performs secure basic authentication
func AuthenticateBasic(username, password, expectedCredentials string) AuthResult {
	start := time.Now()
	defer func() {
		// Add small delay to prevent timing attacks
		time.Sleep(1 * time.Millisecond)
	}()

	if username == "" || password == "" {
		return AuthResult{
			Authorized: false,
			Error:      "Missing basic auth credentials",
			Duration:   time.Since(start),
		}
	}

	// Construct credentials string
	credentials := username + ":" + password

	// Secure comparison
	if !SecureCompareString(credentials, expectedCredentials) {
		return AuthResult{
			Authorized: false,
			Error:      "Invalid basic auth credentials",
			Duration:   time.Since(start),
		}
	}

	return AuthResult{
		Authorized: true,
		Duration:   time.Since(start),
	}
}
