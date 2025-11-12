package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// mock OSRM JSON response
const mockOSRMResponse = `{"code":"Ok","routes":[{"distance":100,"duration":10,"geometry":"mock","legs":[{"summary":"Main St","distance":100,"duration":10,"steps":[{"distance":50,"duration":5,"name":"Main St","mode":"driving","geometry":"","maneuver":{"type":"turn","modifier":"left","location":[0,0]}},{"distance":50,"duration":5,"name":"","mode":"driving","geometry":"","maneuver":{"type":"arrive","location":[0,0]}}]}]}],"waypoints":[]}`

func resetRouteCache() {
	initCache()
	routeCache.Purge()
}

func newMockServer() (*httptest.Server, *int) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(mockOSRMResponse))
	}))
	return server, &count
}

func newErrorServer(status int) (*httptest.Server, *int) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		w.WriteHeader(status)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":"Error","message":"bad"}`))
	}))
	return server, &count
}

type rewriteTransport struct {
	base   http.RoundTripper
	target *url.URL
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = t.target.Scheme
	req.URL.Host = t.target.Host
	return t.base.RoundTrip(req)
}

func TestGetRouteCache(t *testing.T) {
	server, count := newMockServer()
	defer server.Close()
	resetRouteCache()

	options := DefaultOSRMOptions()
	options.BaseURL = server.URL
	options.Client = server.Client()
	options.RetryOptions.MaxAttempts = 1

	coordsA := [][]float64{{0, 0}, {1, 1}}
	ctx := context.Background()

	r1, err := GetRoute(ctx, coordsA, options)
	if err != nil {
		t.Fatal(err)
	}
	if r1 == nil {
		t.Fatal("expected route result")
	}
	if *count != 1 {
		t.Fatalf("expected 1 request, got %d", *count)
	}

	r2, err := GetRoute(ctx, coordsA, options)
	if err != nil {
		t.Fatal(err)
	}
	if r2 == nil {
		t.Fatal("expected route result on second call")
	}
	if *count != 1 {
		t.Fatalf("expected cache hit on second call, requests=%d", *count)
	}
	if r1 != r2 {
		t.Errorf("expected cached result")
	}

	_, err = GetRoute(ctx, [][]float64{{1, 1}, {2, 2}}, options)
	if err != nil {
		t.Fatal(err)
	}
	if *count != 2 {
		t.Fatalf("expected cache miss for different coords, requests=%d", *count)
	}
}

func TestGetRouteNon200(t *testing.T) {
	server, _ := newErrorServer(http.StatusInternalServerError)
	defer server.Close()
	resetRouteCache()

	options := DefaultOSRMOptions()
	options.BaseURL = server.URL
	options.Client = server.Client()
	options.RetryOptions.MaxAttempts = 1

	_, err := GetRoute(context.Background(), [][]float64{{0, 0}, {1, 1}}, options)
	if err == nil {
		t.Fatal("expected error")
	}
	mcpErr, ok := err.(*MCPError)
	if !ok {
		t.Fatalf("expected *MCPError, got %T", err)
	}
	if mcpErr.Code != string(ErrInternalError) {
		t.Errorf("expected code %s, got %s", ErrInternalError, mcpErr.Code)
	}
}

func TestGetSimpleRouteFormatting(t *testing.T) {
	server, count := newMockServer()
	defer server.Close()
	resetRouteCache()

	orig := http.DefaultTransport
	target, _ := url.Parse(server.URL)
	http.DefaultTransport = &rewriteTransport{base: server.Client().Transport, target: target}
	defer func() { http.DefaultTransport = orig }()

	ctx := context.Background()
	from := []float64{0, 0}
	to := []float64{1, 1}

	route, err := GetSimpleRoute(ctx, from, to, "car")
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"Turn left onto Main St for 50 m", "Arrive at destination for 50 m"}
	if len(route.Instructions) != len(expected) {
		t.Fatalf("expected %d instructions, got %d", len(expected), len(route.Instructions))
	}
	for i, inst := range expected {
		if route.Instructions[i] != inst {
			t.Errorf("expected instruction %q, got %q", inst, route.Instructions[i])
		}
	}
	if *count != 1 {
		t.Fatalf("expected 1 request, got %d", *count)
	}

	_, err = GetSimpleRoute(ctx, from, to, "car")
	if err != nil {
		t.Fatal(err)
	}
	if *count != 1 {
		t.Fatalf("expected cached result on repeat call, requests=%d", *count)
	}
}
