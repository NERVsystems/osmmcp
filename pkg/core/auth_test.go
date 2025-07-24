package core

import "testing"

// Test ValidateAuthToken with a strong token
func TestValidateAuthTokenStrong(t *testing.T) {
	token := "a1b2c3d4e5f6g7h8"
	if err := ValidateAuthToken(token); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

// Test ValidateAuthToken with empty token
func TestValidateAuthTokenEmpty(t *testing.T) {
	if err := ValidateAuthToken(""); err == nil {
		t.Fatal("expected error for empty token")
	}
}

// Test ValidateAuthToken with weak token
func TestValidateAuthTokenWeak(t *testing.T) {
	if err := ValidateAuthToken("password12345678"); err == nil {
		t.Fatal("expected error for weak token")
	}
}

// Test AuthenticateBearer with valid header
func TestAuthenticateBearerValid(t *testing.T) {
	expected := "validtokensecret"
	result := AuthenticateBearer("Bearer "+expected, expected)
	if !result.Authorized {
		t.Fatalf("expected authorized true, got false: %s", result.Error)
	}
}

// Test AuthenticateBearer with invalid headers and token
func TestAuthenticateBearerInvalid(t *testing.T) {
	result := AuthenticateBearer("", "token")
	if result.Authorized || result.Error != "Missing Authorization header" {
		t.Fatalf("expected missing header error, got %+v", result)
	}

	result = AuthenticateBearer("Token token", "token")
	if result.Authorized || result.Error != "Invalid Authorization header format" {
		t.Fatalf("expected invalid format error, got %+v", result)
	}

	result = AuthenticateBearer("Bearer wrong", "token")
	if result.Authorized || result.Error != "Invalid bearer token" {
		t.Fatalf("expected invalid token error, got %+v", result)
	}
}

// Test AuthenticateBasic with matching credentials
func TestAuthenticateBasicMatch(t *testing.T) {
	result := AuthenticateBasic("user", "pass", "user:pass")
	if !result.Authorized {
		t.Fatalf("expected authorized, got error: %s", result.Error)
	}
}

// Test AuthenticateBasic with missing or mismatching credentials
func TestAuthenticateBasicMismatch(t *testing.T) {
	result := AuthenticateBasic("", "", "user:pass")
	if result.Authorized || result.Error != "Missing basic auth credentials" {
		t.Fatalf("expected missing credentials, got %+v", result)
	}

	result = AuthenticateBasic("user", "wrong", "user:pass")
	if result.Authorized || result.Error != "Invalid basic auth credentials" {
		t.Fatalf("expected invalid credentials, got %+v", result)
	}
}
