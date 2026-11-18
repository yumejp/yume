package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRequiredModeServesAppShellOnly: the app must load before anyone can
// sign in, so GET of a bundled file skips the gate. API routes, single-page
// fallbacks and non-GET methods on the same paths stay behind it.
func TestRequiredModeServesAppShellOnly(t *testing.T) {
	h, _, _ := newAuthedMux(t, AuthModeRequired)
	prev := staticAsset
	staticAsset = func(p string) bool { return p == "/" || p == "/index.html" || p == "/_expo/static/js/app.js" }
	t.Cleanup(func() { staticAsset = prev })

	for _, tc := range []struct {
		method, path string
		gated        bool
	}{
		{http.MethodGet, "/", false},
		{http.MethodHead, "/index.html", false},
		{http.MethodGet, "/_expo/static/js/app.js", false},
		{http.MethodGet, "/deep/link", true},
		{http.MethodGet, "/deck_stats", true},
		{http.MethodPost, "/deck_stats", true},
		{http.MethodPost, "/", true},
	} {
		req := httptest.NewRequest(tc.method, "http://127.0.0.1"+tc.path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if gated := rec.Code == http.StatusUnauthorized; gated != tc.gated {
			t.Errorf("%s %s: status %d, gated=%v want %v", tc.method, tc.path, rec.Code, gated, tc.gated)
		}
	}
}

func TestSignupFollowsAuthMode(t *testing.T) {
	for _, tc := range []struct {
		name, mode, signup string
		open               bool
	}{
		{"open mode default", AuthModeOpen, "", true},
		{"required mode default", AuthModeRequired, "", false},
		{"required mode opened", AuthModeRequired, SignupOpen, true},
		{"open mode closed", AuthModeOpen, SignupClosed, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, reg, _ := newAuthedMux(t, tc.mode)
			h := NewMux(Config{Version: "test", AuthMode: tc.mode, Signup: tc.signup, AuthStore: reg})

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/health", nil))
			var health struct {
				Auth struct {
					Mode   string `json:"mode"`
					Signup bool   `json:"signup"`
				} `json:"auth"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
				t.Fatal(err)
			}
			if health.Auth.Mode != tc.mode || health.Auth.Signup != tc.open {
				t.Fatalf("health auth = %+v, want mode %s signup %v", health.Auth, tc.mode, tc.open)
			}

			rec = postJSON(h, "/auth/signup", `{"email":"new@example.com","password":"hunter2!!"}`)
			if tc.open {
				if rec.Code != http.StatusOK {
					t.Fatalf("signup: %d %s", rec.Code, rec.Body.String())
				}
				return
			}
			var body struct {
				Code string `json:"code"`
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			if rec.Code != http.StatusForbidden || body.Code != "signup_closed" {
				t.Fatalf("closed signup: %d %s", rec.Code, rec.Body.String())
			}
			if _, _, err := reg.Login("new@example.com", "hunter2!!"); err == nil {
				t.Fatal("closed signup created an account")
			}
		})
	}
}

// TestAuthRoutesLogOutcomes: /auth/* outcomes, including failed logins and
// the rate limit, are logged with route, status and remote address, and
// never with the email, password or token.
func TestAuthRoutesLogOutcomes(t *testing.T) {
	_, reg, _ := newAuthedMux(t, AuthModeRequired)
	if _, _, err := reg.CreateUser("private-user@example.com", "private-pass-1", ""); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	h := NewMux(Config{AuthMode: AuthModeRequired, AuthStore: reg, Logger: slog.New(slog.NewJSONHandler(&buf, nil))})
	login := func(password string) (int, string) {
		buf.Reset()
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/auth/login",
			strings.NewReader(`{"email":"private-user@example.com","password":"`+password+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.0.2.9:5000"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	check := func(status int, level string) {
		t.Helper()
		records := decodeLog(t, &buf)
		if len(records) != 1 {
			t.Fatalf("want one record: %s", &buf)
		}
		r := records[0]
		if r["event"] != "http.request" || r["route"] != "/auth/login" || r["status"] != float64(status) ||
			r["level"] != level || r["remote_ip"] != "192.0.2.9" {
			t.Fatalf("unexpected auth record: %v", r)
		}
		for _, secret := range []string{"private-user", "private-pass", "wrong-pass", "token"} {
			if strings.Contains(buf.String(), secret) {
				t.Fatalf("sensitive value leaked: %q in %s", secret, &buf)
			}
		}
	}

	if code, body := login("private-pass-1"); code != http.StatusOK {
		t.Fatalf("login: %d %s", code, body)
	}
	check(http.StatusOK, "INFO")
	if code, _ := login("wrong-pass"); code != http.StatusUnauthorized {
		t.Fatalf("bad login: %d", code)
	}
	check(http.StatusUnauthorized, "WARN")
	for i := 0; i < authLimitMax; i++ {
		login("wrong-pass")
	}
	if code, _ := login("wrong-pass"); code != http.StatusTooManyRequests {
		t.Fatalf("rate limit: %d", code)
	}
	check(http.StatusTooManyRequests, "WARN")
}
