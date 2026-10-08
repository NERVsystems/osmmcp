package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPSEnforcementRedirect(t *testing.T) {
	next := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

	tests := []struct {
		name     string
		baseURL  string
		host     string
		target   string
		wantCode int
		wantLoc  string
	}{
		{"valid host", "", "mcp.example.com", "/mcp?x=1", http.StatusMovedPermanently, "https://mcp.example.com/mcp?x=1"},
		{"valid host with port", "", "mcp.example.com:7082", "/", http.StatusMovedPermanently, "https://mcp.example.com:7082/"},
		{"configured base wins", "https://canonical.example.org", "attacker.test", "/mcp", http.StatusMovedPermanently, "https://canonical.example.org/mcp"},
		{"host with path injection", "", "evil.test/x", "/", http.StatusBadRequest, ""},
		{"host with userinfo", "", "good.test@evil.test", "/", http.StatusBadRequest, ""},
		{"host with backslash", "", `evil.test\@good.test`, "/", http.StatusBadRequest, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &HTTPTransport{
				config: HTTPTransportConfig{ForceHTTPS: true, BaseURL: tt.baseURL},
				logger: slog.Default(),
			}
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			req.Host = tt.host
			rec := httptest.NewRecorder()
			tr.httpsEnforcement(next)(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if got := rec.Header().Get("Location"); got != tt.wantLoc {
				t.Errorf("Location = %q, want %q", got, tt.wantLoc)
			}
		})
	}
}
