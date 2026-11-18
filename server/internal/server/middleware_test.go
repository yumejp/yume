package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestHostAllowlist(t *testing.T) {
	cases := []struct {
		name   string
		extra  []string
		host   string
		wantOK bool
	}{
		{"loopback always allowed", nil, "127.0.0.1:7654", true},
		{"localhost always allowed", nil, "localhost:7654", true},
		{"foreign host rejected by default", nil, "evil.example:7654", false},
		{"tailscale IP rejected without opt-in", nil, "100.101.102.103:7654", false},
		{"opted-in IP allowed", []string{"100.101.102.103"}, "100.101.102.103:7654", true},
		{"opted-in MagicDNS name allowed", []string{"pi.tail1234.ts.net"}, "pi.tail1234.ts.net:7654", true},
		{"opt-in is case-insensitive", []string{"Pi.Tail1234.ts.net"}, "pi.tail1234.ts.net:7654", true},
		{"opt-in doesn't open other hosts", []string{"pi.tail1234.ts.net"}, "evil.example:7654", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := securityMiddleware(c.extra, okHandler())
			req := httptest.NewRequest(http.MethodGet, "http://placeholder/health", nil)
			req.Host = c.host
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			gotOK := rec.Code == http.StatusOK
			if gotOK != c.wantOK {
				t.Fatalf("host %q extra %v: status %d, want ok=%v", c.host, c.extra, rec.Code, c.wantOK)
			}
		})
	}
}

// FOREIGN browser web Origins stay rejected even on an opted-in host — serving
// the tailnet must not weaken the anti-CSRF posture.
func TestWebOriginStillRejectedOnExtraHost(t *testing.T) {
	h := securityMiddleware([]string{"pi.tail1234.ts.net"}, okHandler())
	req := httptest.NewRequest(http.MethodGet, "http://placeholder/health", nil)
	req.Host = "pi.tail1234.ts.net:7654"
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("web origin on extra host: status %d, want 403", rec.Code)
	}
}

// Crafted origins that resolve to a trusted host via URL trickery (userinfo,
// non-http schemes) must NOT pass the allowlist.
func TestCraftedOriginRejected(t *testing.T) {
	h := securityMiddleware([]string{"100.101.102.103"}, okHandler())
	for _, origin := range []string{
		"http://evil.com@100.101.102.103",      // trusted host as authority, evil.com as userinfo
		"http://100.101.102.103.evil.com:8081", // subdomain spoof
		"file://100.101.102.103",               // non-http scheme
		"null",                                 // sandboxed iframe
	} {
		t.Run(origin, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://placeholder/health", nil)
			req.Host = "100.101.102.103:7654"
			req.Header.Set("Origin", origin)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("origin %q: status %d, want 403", origin, rec.Code)
			}
		})
	}
}

// A web Origin whose host IS the operator's opted-in deployment host is allowed
// (any port): the bundled app under another of the server's names, or the
// mobile web app served by Metro from the Pi over Tailscale. Only a page
// served from that private host can carry such an Origin, so this does not
// admit foreign sites (covered by TestWebOriginStillRejectedOnExtraHost).
func TestOwnHostWebOriginAllowedOnExtraHost(t *testing.T) {
	h := securityMiddleware([]string{"pi.tail1234.ts.net", "100.101.102.103"}, okHandler())
	cases := []string{
		"http://100.101.102.103:7654",     // bundled app, IP
		"http://pi.tail1234.ts.net:7654",  // bundled app, MagicDNS
		"http://100.101.102.103:8081",     // Metro web preview, IP
		"http://pi.tail1234.ts.net:8081",  // Metro web preview, MagicDNS
		"http://pi.tail1234.ts.net:19006", // Expo web default port
	}
	for _, origin := range cases {
		t.Run(origin, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://placeholder/health", nil)
			req.Host = "100.101.102.103:7654"
			req.Header.Set("Origin", origin)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("own-host web origin %q: status %d, want 200", origin, rec.Code)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
				t.Fatalf("ACAO = %q, want %q", got, origin)
			}
		})
	}
}

// The owner's Pi QA path: the Metro web app on :8081 of the Pi's Tailscale
// address calls the server on :7654 with X-Yume-User. Its preflight and its
// JSON write must both pass with only the bind address configured.
func TestMetroPreviewOnBindHostCanWrite(t *testing.T) {
	const origin = "http://100.101.102.103:8081"
	h := securityMiddleware([]string{"100.101.102.103"}, okHandler())

	pre := httptest.NewRequest(http.MethodOptions, "http://placeholder/list_all_tags", nil)
	pre.Host = "100.101.102.103:7654"
	pre.Header.Set("Origin", origin)
	pre.Header.Set("Access-Control-Request-Method", "POST")
	pre.Header.Set("Access-Control-Request-Headers", "content-type,x-yume-user")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, pre)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != origin {
		t.Fatalf("preflight: status %d ACAO %q, want 204 reflected", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-Yume-User") {
		t.Fatalf("Access-Control-Allow-Headers = %q, want X-Yume-User listed", got)
	}

	req := httptest.NewRequest(http.MethodPost, "http://placeholder/list_all_tags", strings.NewReader("{}"))
	req.Host = "100.101.102.103:7654"
	req.Header.Set("Origin", origin)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Yume-User", "test")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != origin {
		t.Fatalf("write: status %d ACAO %q, want 200 reflected", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

// LOOPBACK web origins must NOT ride the host allowlist: any local listener
// (a dev preview server, any app bound to 127.0.0.1) could otherwise serve a
// page with full deck access. Local web dev opts in via YUME_DEV_CORS instead.
func TestLoopbackWebOriginRejectedWithoutDevCORS(t *testing.T) {
	h := securityMiddleware([]string{"pi.tail1234.ts.net"}, okHandler())
	for _, origin := range []string{
		"http://127.0.0.1:8766", // preview server
		"http://localhost:8082", // local Expo web
		"http://[::1]:8080",
	} {
		t.Run(origin, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://placeholder/health", nil)
			req.Host = "pi.tail1234.ts.net:7654"
			req.Header.Set("Origin", origin)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("loopback origin %q: status %d, want 403 (devCORS off)", origin, rec.Code)
			}
		})
	}
}

// Extension requests pass (Yume's extension has host permissions, so the
// browser needs no CORS headers for it), but only extensions listed in
// YUME_EXTENSION_ORIGINS get them. An unlisted extension's preflight gets no
// Access-Control-Allow-Origin, so the browser never sends its JSON write.
func TestBrowserExtensionOrigins(t *testing.T) {
	const listed = "chrome-extension://abcdefghijklmnopabcdefghijklmnop"
	saved := extensionOrigins
	t.Cleanup(func() { extensionOrigins = saved })
	extensionOrigins = originSet(listed)
	h := securityMiddleware([]string{"pi.tail1234.ts.net"}, okHandler())
	send := func(method, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://placeholder/list_all_tags", strings.NewReader("{}"))
		req.Host = "pi.tail1234.ts.net:7654"
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	for _, origin := range []string{
		"chrome-extension://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"moz-extension://8f3c2c1e-1b2a-4c3d-9e8f-123456789abc",
	} {
		rec := send(http.MethodPost, origin)
		if rec.Code != http.StatusOK {
			t.Errorf("%s POST: got %d, want 200", origin, rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("%s POST: ACAO = %q, want none", origin, got)
		}
		rec = send(http.MethodOptions, origin)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("%s preflight: ACAO = %q, want none", origin, got)
		}
	}
	for _, method := range []string{http.MethodPost, http.MethodOptions} {
		rec := send(method, listed)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != listed {
			t.Errorf("listed %s: ACAO = %q, want %q", method, got, listed)
		}
	}
}

// Every response, refusals included, forbids framing, sniffing and referrers.
func TestSecurityHeaders(t *testing.T) {
	h := securityMiddleware(nil, okHandler())
	for _, host := range []string{"127.0.0.1:7654", "evil.example:7654"} {
		req := httptest.NewRequest(http.MethodGet, "http://placeholder/", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		for k, want := range map[string]string{
			"X-Frame-Options":         "DENY",
			"Content-Security-Policy": "frame-ancestors 'none'",
			"X-Content-Type-Options":  "nosniff",
			"Referrer-Policy":         "no-referrer",
		} {
			if got := rec.Header().Get(k); got != want {
				t.Errorf("host %s (%d): %s = %q, want %q", host, rec.Code, k, got, want)
			}
		}
	}
}
