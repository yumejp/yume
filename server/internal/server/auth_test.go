package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/yumejp/yume/server/internal/auth"
	"github.com/yumejp/yume/server/internal/srs"
)

// TestAuthFlowRequiredMode exercises signup → authed call → me → logout →
// rejection, plus the guarantees of required mode (no token = 401, X-Yume-User
// ignored) and the credential error paths.
func TestAuthFlowRequiredMode(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(srs.CloseAll) // close per-user stores before TempDir removal (Windows)
	t.Setenv("YUME_DB", filepath.Join(dir, "cards.db"))

	reg, err := auth.Open(filepath.Join(dir, "users.db"))
	if err != nil {
		t.Fatalf("auth.Open: %v", err)
	}
	t.Cleanup(func() { reg.Close() })
	ownerID, _, _, err := reg.Bootstrap()
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	srs.SetOwner(ownerID)

	// Signup is closed by default in required mode; this flow opens it.
	h := NewMux(Config{Version: "test", AuthMode: AuthModeRequired, Signup: SignupOpen, AuthStore: reg})

	// do issues a request through the full middleware stack. Host is loopback
	// so securityMiddleware admits it; no Origin (native caller).
	do := func(method, path, token string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, "http://127.0.0.1"+path, &buf)
		if method != http.MethodGet {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// No token in required mode → 401.
	if rec := do(http.MethodPost, "/deck_stats", "", map[string]any{}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth deck_stats: got %d, want 401", rec.Code)
	}

	// X-Yume-User must be IGNORED in required mode (no impersonation).
	{
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/deck_stats", bytes.NewReader([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Yume-User", "owner")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("X-Yume-User in required mode: got %d, want 401", rec.Code)
		}
	}

	// Signup → token.
	rec := do(http.MethodPost, "/auth/signup", "", map[string]string{
		"email": "a@b.com", "password": "hunter2!!", "display_name": "Aki",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("signup: got %d (%s)", rec.Code, rec.Body.String())
	}
	var sresp struct {
		Token string    `json:"token"`
		User  auth.User `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sresp); err != nil {
		t.Fatalf("decode signup: %v", err)
	}
	if sresp.Token == "" || sresp.User.Role != auth.RoleUser {
		t.Fatalf("unexpected signup response: %+v", sresp)
	}
	token := sresp.Token

	// Authed deck_stats → the new user's own (empty) deck.
	rec = do(http.MethodPost, "/deck_stats", token, map[string]any{})
	if rec.Code != http.StatusOK {
		t.Fatalf("authed deck_stats: got %d (%s)", rec.Code, rec.Body.String())
	}
	var ds struct {
		Total  int    `json:"total"`
		DBPath string `json:"db_path"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &ds)
	if ds.Total != 0 {
		t.Errorf("new user deck total = %d, want 0", ds.Total)
	}
	if want := filepath.Join(dir, "users", sresp.User.ID); !contains(ds.DBPath, sresp.User.ID) {
		t.Errorf("deck path %q not under user dir %q", ds.DBPath, want)
	}

	// /auth/me echoes the account.
	rec = do(http.MethodGet, "/auth/me", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("me: got %d (%s)", rec.Code, rec.Body.String())
	}

	// Logout invalidates the token.
	if rec := do(http.MethodPost, "/auth/logout", token, map[string]any{}); rec.Code != http.StatusOK {
		t.Fatalf("logout: got %d", rec.Code)
	}
	if rec := do(http.MethodGet, "/auth/me", token, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout: got %d, want 401", rec.Code)
	}

	// Duplicate email → 409.
	if rec := do(http.MethodPost, "/auth/signup", "", map[string]string{"email": "a@b.com", "password": "another1!"}); rec.Code != http.StatusConflict {
		t.Errorf("duplicate signup: got %d, want 409", rec.Code)
	}
	// Wrong password → 401.
	if rec := do(http.MethodPost, "/auth/login", "", map[string]string{"email": "a@b.com", "password": "wrong"}); rec.Code != http.StatusUnauthorized {
		t.Errorf("bad login: got %d, want 401", rec.Code)
	}
	// Weak password → 400.
	if rec := do(http.MethodPost, "/auth/signup", "", map[string]string{"email": "c@d.com", "password": "short"}); rec.Code != http.StatusBadRequest {
		t.Errorf("weak password: got %d, want 400", rec.Code)
	}
}

func contains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }

// A YUME_AUTH_MODE that is not exactly a mode must fail, never serve open.
func TestParseAuthMode(t *testing.T) {
	for in, want := range map[string]string{
		"":            AuthModeOpen,
		"open":        AuthModeOpen,
		" Open ":      AuthModeOpen,
		"required":    AuthModeRequired,
		"Required":    AuthModeRequired,
		"REQUIRED\n":  AuthModeRequired,
		" required  ": AuthModeRequired,
	} {
		if got, err := ParseAuthMode(in); err != nil || got != want {
			t.Errorf("ParseAuthMode(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"true", "1", "yes", "require", "requried", "closed", "none", "off"} {
		if got, err := ParseAuthMode(in); err == nil {
			t.Errorf("ParseAuthMode(%q) = %q, want an error", in, got)
		}
	}
}

// In open mode X-Yume-User takes role names only: it must not reach a
// password account by email, id or display name.
func TestDevUserHeaderRolesOnly(t *testing.T) {
	h, reg, _ := newAuthedMux(t, AuthModeOpen)
	u, _, err := reg.CreateUser("friend@example.com", "long-enough-1", "Friend")
	if err != nil {
		t.Fatal(err)
	}
	send := func(user string) int {
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/deck_stats", bytes.NewReader([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Yume-User", user)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	for _, user := range []string{"owner", "test", "Test", " OWNER "} {
		if code := send(user); code != http.StatusOK {
			t.Errorf("X-Yume-User %q: got %d, want 200", user, code)
		}
	}
	for _, user := range []string{"friend@example.com", u.ID, "Friend", "user", "nobody"} {
		if code := send(user); code != http.StatusBadRequest {
			t.Errorf("X-Yume-User %q: got %d, want 400", user, code)
		}
	}
}
