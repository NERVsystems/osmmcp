package osm

import (
	"net/url"
	"testing"
)

// allowHostForTest temporarily adds the host of rawURL to the outbound
// allow-list so tests can target local httptest servers.
func allowHostForTest(tb testing.TB, rawURL string) {
	tb.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		tb.Fatalf("parse %q: %v", rawURL, err)
	}
	allowedHostsMu.Lock()
	_, existed := allowedOutboundHosts[u.Host]
	allowedOutboundHosts[u.Host] = struct{}{}
	allowedHostsMu.Unlock()
	if !existed {
		tb.Cleanup(func() {
			allowedHostsMu.Lock()
			delete(allowedOutboundHosts, u.Host)
			allowedHostsMu.Unlock()
		})
	}
}
