package server

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// devCORS (YUME_DEV_CORS) additionally allows localhost web origins. Dev
// preview only; never set in a shipped build.
var devCORS = os.Getenv("YUME_DEV_CORS") != ""

// extensionOrigins (YUME_EXTENSION_ORIGINS, comma-separated exact origins
// such as chrome-extension://<id>) are the browser extensions that get CORS
// headers. Yume's own extension declares host permissions for its server, so
// its requests need none and it works without this; the list is for builds
// that lack host permissions.
var extensionOrigins = originSet(os.Getenv("YUME_EXTENSION_ORIGINS"))

// originSet parses a comma-separated origin list, normalized the way
// browsers send an Origin: lowercase, no trailing slash.
func originSet(list string) map[string]bool {
	set := map[string]bool{}
	for _, o := range strings.Split(list, ",") {
		if o = strings.TrimRight(strings.ToLower(strings.TrimSpace(o)), "/"); o != "" {
			set[o] = true
		}
	}
	return set
}

// maxBodyBytes bounds memory; base64 audio+image sidecars are the largest
// legitimate payload.
const maxBodyBytes = 64 << 20 // 64 MiB

// securityMiddleware is the security boundary against a malicious web page
// reaching a server on 127.0.0.1; per-handler checks are defense-in-depth.
//
//  1. Host allowlist (loopback + -bind/YUME_ALLOWED_HOSTS) — defeats DNS rebinding.
//  2. Origin allowlist — a web Origin is rejected unless it is the server's
//     own (the bundled web app) or on an allowed host. No Origin (curl, MCP,
//     native app) is allowed.
//     Browser extensions pass, but only those in YUME_EXTENSION_ORIGINS get
//     CORS headers: an extension without host permissions for this server
//     can neither read a response nor send a JSON write.
//  3. application/json required on mutating methods — not a CORS-simple type,
//     so a cross-origin POST needs a preflight, which layer 2 blocks (CSRF).
//
// Every response also forbids framing (clickjacking), MIME sniffing and
// referrers.
func securityMiddleware(extraHosts []string, next http.Handler) http.Handler {
	allowed := make(map[string]bool, len(extraHosts))
	for _, h := range extraHosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			allowed[h] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")

		// Layer 1: Host allowlist.
		if !allowedHost(r.Host, allowed) {
			logRejection(w, r, http.StatusForbidden, "forbidden_host")
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}

		// Layer 2: allow the bundled app, browser-extension clients, explicitly
		// configured hosts, and localhost origins when YUME_DEV_CORS is enabled.
		origin := r.Header.Get("Origin")
		cors := false
		switch {
		case origin == "":
		case isBrowserExtensionOrigin(origin):
			cors = extensionOrigins[strings.ToLower(origin)]
		case isSameOrigin(origin, r),
			isAllowedHostOrigin(origin, allowed),
			devCORS && isLocalhostOrigin(origin):
			cors = true
		default:
			logRejection(w, r, http.StatusForbidden, "forbidden_origin")
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}

		// Reflect the specific origin: "*" + credentials is invalid.
		if cors {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			// Authorization must be listed or bearer-token preflights fail.
			// X-Yume-Client names the calling client and version (e.g.
			// "extension/0.1.0") so a server can tell clients apart.
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Yume-User, Authorization, X-Yume-Client")
			w.Header().Set("Access-Control-Max-Age", "600")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

		// Layer 3. /mcp is exempt: the Streamable HTTP transport sends a bodyless
		// DELETE on session teardown.
		if r.URL.Path != "/mcp" && r.Method != http.MethodGet && r.Method != http.MethodHead {
			ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
			if !strings.HasPrefix(ct, "application/json") {
				http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// Browser extensions run under their own origin scheme, which no web page can
// forge. Their requests pass layer 2; CORS headers are reserved for
// extensionOrigins.
func isBrowserExtensionOrigin(origin string) bool {
	return strings.HasPrefix(origin, "chrome-extension://") ||
		strings.HasPrefix(origin, "moz-extension://") ||
		strings.HasPrefix(origin, "extension://")
}

func isLocalhostOrigin(origin string) bool {
	return strings.HasPrefix(origin, "http://localhost:") ||
		strings.HasPrefix(origin, "http://127.0.0.1:")
}

// isAllowedHostOrigin reports whether a web Origin's host is in the
// operator's explicit allowlist (bind IP / YUME_ALLOWED_HOSTS), on any port:
// the owner's Metro web preview on the server's machine is a trusted client.
// Loopback origins are rejected: any page served by any local
// listener would get full deck access. Local web dev opts in via YUME_DEV_CORS.
func isAllowedHostOrigin(origin string, allowed map[string]bool) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	// A browser Origin is exactly scheme://host[:port]; reject userinfo so
	// "http://evil.com@<trusted-host>" can't pass.
	if u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	hostname := u.Host
	if h, _, err := net.SplitHostPort(hostname); err == nil {
		hostname = h
	}
	switch hostname {
	case "127.0.0.1", "localhost", "::1", "[::1]":
		return false
	}
	return allowedHost(u.Host, allowed)
}

// allowedHost reports whether Host names loopback or a configured extra host.
// Port is ignored — the server binds exactly one.
func allowedHost(host string, extra map[string]bool) bool {
	if host == "" {
		return false
	}
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	switch hostname {
	case "127.0.0.1", "localhost", "::1", "[::1]":
		return true
	}
	return extra[strings.ToLower(hostname)]
}

// isSameOrigin accepts a web Origin that names this very listener: the
// embedded phone app is served from it, and only the server itself can put a
// page there. Loopback stays rejected otherwise (another local listener
// could serve a hostile page).
func isSameOrigin(origin string, r *http.Request) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}
