package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/yumejp/yume/server/internal/auth"
	"github.com/yumejp/yume/server/internal/srs"
	"github.com/yumejp/yume/server/internal/webapp"
)

// Auth modes (Config.AuthMode). Empty string is treated as open.
const (
	AuthModeOpen     = "open"     // dev: unauthenticated → owner; X-Yume-User picks a role
	AuthModeRequired = "required" // prod: a valid session bearer token is mandatory
)

// ParseAuthMode reads a YUME_AUTH_MODE value: unset means open, otherwise
// only "open" or "required" (any case, surrounding space ignored). Anything
// else is an error, never a silent fall-through to open: "Required" or
// "true" must not leave a deck unprotected.
func ParseAuthMode(v string) (string, error) {
	switch mode := strings.ToLower(strings.TrimSpace(v)); mode {
	case "":
		return AuthModeOpen, nil
	case AuthModeOpen, AuthModeRequired:
		return mode, nil
	default:
		return "", fmt.Errorf("YUME_AUTH_MODE=%q is not a mode; use %q or %q", v, AuthModeOpen, AuthModeRequired)
	}
}

// Signup settings (Config.Signup, YUME_SIGNUP).
const (
	SignupOpen   = "open"
	SignupClosed = "closed"
)

// authMode is the effective mode: anything but required behaves as open.
func authMode(cfg Config) string {
	if cfg.AuthMode == AuthModeRequired {
		return AuthModeRequired
	}
	return AuthModeOpen
}

// signupOpen reports whether /auth/signup creates accounts. Unset, required
// mode is closed: the owner opens it (YUME_SIGNUP=open) while people join.
func signupOpen(cfg Config) bool {
	if cfg.Signup == "" {
		return authMode(cfg) == AuthModeOpen
	}
	return cfg.Signup == SignupOpen
}

type ctxKey int

const userIDKey ctxKey = 0

func withUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

func userIDFrom(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey).(string)
	return id, ok && id != ""
}

// storeFor resolves the request user's store. Fails closed: no resolved user
// is a bug, never a silent write to someone else's deck.
func storeFor(r *http.Request) (*srs.Store, error) {
	if id, ok := userIDFrom(r.Context()); ok {
		return srs.ForUser(id)
	}
	return nil, fmt.Errorf("no user resolved for request")
}

// requireOwner gates writes to shared assets (the tokenizer user dictionary).
func requireOwner(r *http.Request, reg *auth.Store) error {
	uid, ok := userIDFrom(r.Context())
	if !ok {
		return fmt.Errorf("no user resolved for request")
	}
	owner, err := reg.OwnerID()
	if err != nil || owner == "" {
		return fmt.Errorf("owner account not resolved")
	}
	if uid != owner {
		return fmt.Errorf("only the owner may modify the shared tokenizer dictionary")
	}
	return nil
}

// staticAsset reports a real file of the bundled app; tests swap it because
// the embedded bundle is empty in a plain `go test` build.
var staticAsset = webapp.IsAsset

// reject logs a middleware refusal and writes the JSON error.
func reject(w http.ResponseWriter, r *http.Request, status int, reason, msg string) {
	logRejection(w, r, status, reason)
	writeErr(w, status, msg)
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// userResolveMiddleware attaches a user id to each data request; it runs
// inside securityMiddleware.
//
//  1. A valid bearer token → that user (both modes); an invalid one is always 401.
//  2. open mode, no token: X-Yume-User names a role (owner, test), else the
//     owner. Emails, ids and display names are refused, so the header cannot
//     reach an account that signed up with a password.
//  3. required mode, no token → 401.
//
// /health, /auth/* and GET/HEAD of the app's own bundled files skip
// resolution: the app shell must load before anyone can sign in, while every
// API route and single-page fallback stays behind the gate.
//
// /mcp binds the owner's store internally, so in required mode it demands
// the owner's own token (non-owner → 403) rather than any resolved user.
func userResolveMiddleware(mode string, reg *auth.Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.Path; {
		case path == "/health", path == "/licenses", strings.HasPrefix(path, "/auth/"):
			next.ServeHTTP(w, r)
			return
		case (r.Method == http.MethodGet || r.Method == http.MethodHead) && staticAsset(path):
			next.ServeHTTP(w, r)
			return
		case path == "/mcp":
			if mode != AuthModeRequired {
				next.ServeHTTP(w, r)
				return
			}
			tok := bearerToken(r)
			if tok == "" {
				reject(w, r, http.StatusUnauthorized, "no_token", "authentication required")
				return
			}
			uid, ok := reg.UserForToken(tok)
			if !ok {
				reject(w, r, http.StatusUnauthorized, "invalid_token", "invalid or expired session")
				return
			}
			owner, err := reg.OwnerID()
			if err != nil || owner == "" {
				writeErr(w, http.StatusInternalServerError, "owner account not resolved")
				return
			}
			if uid != owner {
				reject(w, r, http.StatusForbidden, "not_owner", "MCP is restricted to the owner account")
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		if tok := bearerToken(r); tok != "" {
			if uid, ok := reg.UserForToken(tok); ok {
				next.ServeHTTP(w, r.WithContext(withUserID(r.Context(), uid)))
				return
			}
			reject(w, r, http.StatusUnauthorized, "invalid_token", "invalid or expired session")
			return
		}

		if mode != AuthModeRequired {
			if h := strings.TrimSpace(r.Header.Get("X-Yume-User")); h != "" {
				uid, ok := reg.ResolveRoleUser(h)
				if !ok {
					writeErr(w, http.StatusBadRequest, "unknown X-Yume-User: "+h+" (use a role name: owner or test)")
					return
				}
				next.ServeHTTP(w, r.WithContext(withUserID(r.Context(), uid)))
				return
			}
			uid, err := reg.OwnerID()
			if err != nil || uid == "" {
				writeErr(w, http.StatusInternalServerError, "owner account not resolved")
				return
			}
			next.ServeHTTP(w, r.WithContext(withUserID(r.Context(), uid)))
			return
		}

		reject(w, r, http.StatusUnauthorized, "no_token", "authentication required")
	})
}
