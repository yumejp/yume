package server

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/yumejp/yume/server/internal/auth"
)

// authLimiter is a small dependency-free per-IP sliding-window rate limit for
// the password endpoints: bcrypt cost 12 burns ~0.5-1s of CPU per call on a
// Pi 4, so unthrottled signup/login is a trivial CPU DoS from anywhere on the
// bind network. Logout/me are cheap and stay unthrottled.
type authLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

const (
	authLimitWindow = time.Minute
	authLimitMax    = 10 // attempts per IP per window
)

func (l *authLimiter) allow(remoteAddr string, now time.Time) bool {
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		ip = remoteAddr
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hits == nil {
		l.hits = make(map[string][]time.Time)
	}
	cutoff := now.Add(-authLimitWindow)
	kept := l.hits[ip][:0]
	for _, t := range l.hits[ip] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= authLimitMax {
		l.hits[ip] = kept
		return false
	}
	l.hits[ip] = append(kept, now)
	// Opportunistically drop other stale IPs so the map can't grow unbounded.
	if len(l.hits) > 1024 {
		for k, ts := range l.hits {
			if len(ts) == 0 || !ts[len(ts)-1].After(cutoff) {
				delete(l.hits, k)
			}
		}
	}
	return true
}

// registerAuthRoutes wires the account endpoints. These live OUTSIDE the
// user-resolution gate (the middleware bypasses /auth/*): signup/login mint a
// session for an as-yet-unauthenticated caller; logout/me read the bearer
// token directly. securityMiddleware (Host/Origin/Content-Type) still applies.
// Every outcome is logged with the remote address (logAuthRoute), never the
// body or email. With signup closed, /auth/signup answers 403 signup_closed.
func registerAuthRoutes(mux *http.ServeMux, reg *auth.Store, signup bool) {
	if reg == nil {
		return
	}
	limiter := &authLimiter{}

	// POST /auth/signup { email, password, display_name? } → { token, user }
	mux.HandleFunc("/auth/signup", logAuthRoute("/auth/signup", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "use POST")
			return
		}
		if !signup {
			writeErrCode(w, http.StatusForbidden, "new accounts are turned off on this server", "signup_closed")
			return
		}
		if !limiter.allow(r.RemoteAddr, time.Now()) {
			writeErr(w, http.StatusTooManyRequests, "too many attempts — try again in a minute")
			return
		}
		var req struct {
			Email       string `json:"email"`
			Password    string `json:"password"`
			DisplayName string `json:"display_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		u, token, err := reg.CreateUser(req.Email, req.Password, req.DisplayName)
		if err != nil {
			writeErr(w, authErrStatus(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": u})
	}))

	// POST /auth/login { email, password } → { token, user }
	mux.HandleFunc("/auth/login", logAuthRoute("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "use POST")
			return
		}
		if !limiter.allow(r.RemoteAddr, time.Now()) {
			writeErr(w, http.StatusTooManyRequests, "too many attempts — try again in a minute")
			return
		}
		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		u, token, err := reg.Login(req.Email, req.Password)
		if err != nil {
			writeErr(w, authErrStatus(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": u})
	}))

	// POST /auth/logout (Bearer) → { ok: true }. Idempotent.
	mux.HandleFunc("/auth/logout", logAuthRoute("/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "use POST")
			return
		}
		if tok := bearerToken(r); tok != "" {
			if err := reg.DeleteSession(tok); err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}))

	// GET /auth/me (Bearer) → { user }
	mux.HandleFunc("/auth/me", logAuthRoute("/auth/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "use GET")
			return
		}
		tok := bearerToken(r)
		uid, ok := reg.UserForToken(tok)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "invalid or expired session")
			return
		}
		u, ok := reg.GetUser(uid)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "account no longer exists")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user": u})
	}))
}

// authErrStatus maps auth.Store errors to HTTP status codes.
func authErrStatus(err error) int {
	switch {
	case errors.Is(err, auth.ErrEmailTaken):
		return http.StatusConflict
	case errors.Is(err, auth.ErrInvalidCredentials):
		return http.StatusUnauthorized
	case errors.Is(err, auth.ErrInvalidEmail), errors.Is(err, auth.ErrWeakPassword):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
