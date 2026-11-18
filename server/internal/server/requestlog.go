package server

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/yumejp/yume/server/internal/srs"
)

// Request records. Each request through jsonPost or an /auth/* route emits
// one JSON line when it ends, and anything a handler logs on the way carries
// the same request_id. The records name the route, the outcome and the time
// taken; they never carry bodies, query strings, credentials or user text.

// logCtxKey is the context key a request's logger travels under.
type logCtxKey struct{}

// fallbackLog is used when Config.Logger is nil, and by handlers called
// without the mux (as some tests do).
var fallbackLog = slog.New(slog.NewJSONHandler(os.Stderr, nil))

// attachLogger is the outermost handler: it puts base on every request so
// the layers inside, including the ones that refuse a request, can log.
func attachLogger(base *slog.Logger, next http.Handler) http.Handler {
	if base == nil {
		base = fallbackLog
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), logCtxKey{}, base)))
	})
}

// reqLog returns the logger this request carries.
func reqLog(r *http.Request) *slog.Logger {
	if l, ok := r.Context().Value(logCtxKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return fallbackLog
}

// statusWriter remembers the status a handler answered with.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the writer underneath.
func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// logged wraps one route. It names the request (request_id, also returned
// in X-Request-ID), hands the handler a logger that carries the id and the
// route, and when the handler returns writes the http.request record:
// method, status and duration_ms, at INFO, WARN for 4xx, ERROR for 5xx.
//
// route is the registered path, never the requested URL. A handler that
// panics is answered with a 500 and recorded with panic=true; what it
// panicked with is not logged.
func logged(route string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := rand.Text()
		w.Header().Set("X-Request-ID", id)
		l := reqLog(r).With("request_id", id, "route", route)
		r = r.WithContext(context.WithValue(r.Context(), logCtxKey{}, l))
		sw := &statusWriter{ResponseWriter: w}
		began := time.Now()

		defer func() {
			crash := recover()
			if crash == http.ErrAbortHandler {
				panic(crash) // net/http's own signal to drop the connection
			}
			if crash != nil && sw.status == 0 {
				writeErr(sw, http.StatusInternalServerError, "internal error")
			}
			status := sw.status
			if status == 0 {
				status = http.StatusOK
			}
			level := levelForStatus(status)
			attrs := []any{
				"event", "http.request",
				"method", r.Method,
				"status", status,
				"duration_ms", time.Since(began).Milliseconds(),
			}
			if crash != nil {
				level = slog.LevelError
				attrs = append(attrs, "panic", true)
			}
			l.Log(r.Context(), level, "HTTP request", attrs...)
		}()

		next(sw, r)
	}
}

func levelForStatus(status int) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	}
	return slog.LevelInfo
}

// noteCardSave records how a /save_card call ended as one card.save record:
// outcome is "created", "existing" (the word already had a card) or
// "failed". It carries the card id or the failure's status, never the word,
// its sentence or the error text.
func noteCardSave(r *http.Request, saved srs.SaveCardResp, err error) {
	l := reqLog(r)
	if err != nil {
		status := errStatus(err)
		l.Log(r.Context(), levelForStatus(status), "Card not saved",
			"event", "card.save", "outcome", "failed", "status", status)
		return
	}
	outcome := "existing"
	if saved.Created {
		outcome = "created"
	}
	l.InfoContext(r.Context(), "Card saved",
		"event", "card.save", "outcome", outcome, "card_id", saved.ID)
}

// remoteIP is the connection's address without the port. Behind a reverse
// proxy it is the proxy's address.
func remoteIP(r *http.Request) string {
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return ip
	}
	return r.RemoteAddr
}

// logRejection records a request the security or auth middleware refused,
// so brute-force attempts and 401 storms are visible. Only the path, a fixed
// reason and the remote address are logged: never bodies, tokens, emails or
// client-controlled headers. The caller writes the response.
func logRejection(w http.ResponseWriter, r *http.Request, status int, reason string) {
	id := rand.Text()
	w.Header().Set("X-Request-ID", id)
	reqLog(r).Log(r.Context(), slog.LevelWarn, "HTTP request rejected",
		"event", "http.rejected", "request_id", id, "route", routeLabel(r.URL.Path),
		"method", r.Method, "status", status, "reason", reason, "remote_ip", remoteIP(r))
}

// routeLabel bounds what a rejected path can put in the log.
func routeLabel(p string) string {
	const max = 64
	if len(p) > max {
		return p[:max] + "..."
	}
	return p
}

// logAuthRoute is logged plus the remote address, for the /auth/*
// endpoints: their outcomes (including 401 and 429) are the security record.
func logAuthRoute(route string, next http.HandlerFunc) http.HandlerFunc {
	inner := logged(route, next)
	return func(w http.ResponseWriter, r *http.Request) {
		l := reqLog(r).With("remote_ip", remoteIP(r))
		inner(w, r.WithContext(context.WithValue(r.Context(), logCtxKey{}, l)))
	}
}
