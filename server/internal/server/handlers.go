// Package server wires the HTTP endpoints that Yume clients call.
//
// Each RPC method M is a `POST /M` with the params as the JSON body:
//
//	fetch('http://127.0.0.1:7654/M', { method:'POST', body: JSON.stringify(params) })
//
// Request and response shapes are the RPC `result` payloads; clients wrap
// them 1:1.
package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/yumejp/yume/server/internal/auth"
	"github.com/yumejp/yume/server/internal/srs"
	"github.com/yumejp/yume/server/internal/webapp"
)

// Config controls the HTTP server and account resolution.
type Config struct {
	// Version and Commit identify the build in /health; Commit may be empty.
	Version string
	Commit  string
	// Logger receives the request and card-save records (requestlog.go). Nil
	// writes JSON lines to stderr.
	Logger *slog.Logger
	// ExtraAllowedHosts extends the middleware's Host allowlist beyond
	// loopback — set when serving a private interface (Pi over Tailscale).
	ExtraAllowedHosts []string
	// AuthMode is "open" (dev: unauthenticated → owner, X-Yume-User picks a
	// user) or "required" (prod: a valid session bearer token is mandatory).
	// Empty is treated as open. See auth.go.
	AuthMode string
	// Signup is "open" (anyone who reaches /auth/signup gets an account) or
	// "closed". Empty follows the auth mode: open mode stays open, required
	// mode is closed, so a shared server never hands out decks by default.
	Signup string
	// AuthStore is the user/session registry the resolution middleware reads.
	AuthStore *auth.Store
}

// NewMux builds the routed handler. We use the stdlib mux because nothing
// here needs path params or middleware chains complex enough to justify a
// router dep.
func NewMux(cfg Config) http.Handler {
	mux := http.NewServeMux()

	// Account endpoints (/auth/signup|login|logout|me). Registered outside the
	// user-resolution gate — see authhandlers.go.
	registerAuthRoutes(mux, cfg.AuthStore, signupOpen(cfg))

	registerParseRoutes(mux, cfg)
	registerDeckRoutes(mux, cfg)
	registerDictRoutes(mux, cfg)
	registerMaintenanceRoutes(mux, cfg)
	registerMiningRoutes(mux, cfg)
	registerSettingsRoutes(mux, cfg)

	// The phone app, served from the same origin as the API. Registered last:
	// "/" is the catch-all and every API route above is more specific.
	mux.Handle("/", webapp.Handler())

	// Wrap the mux in two layers. Inner: user-resolution (attaches the
	// per-request user id that storeFor reads). Outer: the security
	// middleware — Host allowlist, Origin allowlist, Content-Type
	// requirement, body cap — which is the real network security boundary and
	// must run first. See middleware.go and auth.go.
	resolved := userResolveMiddleware(cfg.AuthMode, cfg.AuthStore, mux)
	return attachLogger(cfg.Logger, securityMiddleware(cfg.ExtraAllowedHosts, resolved))
}

// --- Request / response types ---

type tokenizeReq struct {
	Text string `json:"text"`
	Lang string `json:"lang"`
}

type tokenizeBatchReq struct {
	Texts []string `json:"texts"`
	Lang  string   `json:"lang"`
}

type lookupReq struct {
	Queries []string `json:"queries"`
	// Optional; empty defaults to "ja".
	Lang string `json:"lang"`
}

type examplesReq struct {
	Queries []string `json:"queries"`
	Lang    string   `json:"lang"`
	// Optional kana reading; sentences the corpus reads differently rank last.
	Reading string `json:"reading"`
	// Optional; server defaults to 5.
	Limit int `json:"limit"`
}

// --- Small helpers ---

// routeOpts are the deviations from the plain POST-JSON contract.
type routeOpts struct {
	allowGET     bool // also answer GET; a POST body is then optional too
	optionalBody bool // an empty POST body means defaults
	noStore      bool // fn runs without a per-user store (nil)
}

// httpErr is a handler-raised error that carries its own status.
type httpErr struct {
	status int
	msg    string
}

func (e httpErr) Error() string { return e.msg }

func badRequest(msg string) error { return httpErr{http.StatusBadRequest, msg} }

// errStatus is the one place errors become statuses. 400 only for errors the
// request caused: the offline replayer drops a rating on 4xx but retries on
// 5xx, so a transient failure (db locked, disk) must never be 400.
func errStatus(err error) int {
	var he httpErr
	switch {
	case errors.As(err, &he):
		return he.status
	case srs.IsClientErr(err):
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

// jsonPost registers POST /path: decode Req from the body, resolve the
// request user's store, call fn, encode its result. Req = struct{} reads no
// body at all.
func jsonPost[Req any](mux *http.ServeMux, path string, fn func(r *http.Request, store *srs.Store, req Req) (any, error), opts ...routeOpts) {
	var o routeOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	mux.HandleFunc(path, logged(path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && !(o.allowGET && r.Method == http.MethodGet) {
			writeErr(w, http.StatusMethodNotAllowed, "use POST")
			return
		}
		var req Req
		// Every request may name a language ("lang" in the body or the query);
		// the store is bound to it, default ja. Read the body once so both the
		// typed request and the language peek see it.
		lang := r.URL.Query().Get("lang")
		_, noBody := any(&req).(*struct{})
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "read body: "+err.Error())
				return
			}
			if len(body) > 0 {
				if !noBody {
					if err := json.Unmarshal(body, &req); err != nil {
						writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
						return
					}
				}
				var peek struct {
					Lang string `json:"lang"`
				}
				if json.Unmarshal(body, &peek) == nil && peek.Lang != "" {
					lang = peek.Lang
				}
			} else if !noBody && !(o.optionalBody || o.allowGET) {
				writeErr(w, http.StatusBadRequest, "invalid JSON: EOF")
				return
			}
		}
		var store *srs.Store
		if !o.noStore {
			var err error
			if store, err = storeFor(r); err != nil {
				// The message names files; the log carries only its class.
				kind := srs.SchemaErrKind(err)
				logKind := kind
				if logKind == "" {
					logKind = "store_open"
				}
				reqLog(r).ErrorContext(r.Context(), "Deck unavailable",
					"event", "store.open_failed", "error_kind", logKind)
				writeErrCode(w, http.StatusInternalServerError, err.Error(), kind)
				return
			}
			if lang != "" {
				store = store.WithLang(lang)
			}
		}
		resp, err := fn(r, store, req)
		if err != nil {
			writeErrCode(w, errStatus(err), err.Error(), srs.ErrCode(err))
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeErrCode(w, status, msg, "")
}

// writeErrCode adds a stable `code` when the error carries one; clients
// branch on the code, never on the message.
func writeErrCode(w http.ResponseWriter, status int, msg, code string) {
	body := map[string]any{"ok": false, "error": msg}
	if code != "" {
		body["code"] = code
	}
	writeJSON(w, status, body)
}
