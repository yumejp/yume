package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/yumejp/yume/server/internal/auth"
	"github.com/yumejp/yume/server/internal/srs"
)

// mcpInitialize is a minimal JSON-RPC initialize request — enough for the
// Streamable HTTP handler to answer, which proves the middleware actually
// forwarded the request rather than short-circuiting it.
const mcpInitialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":` +
	`{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`

// postMCP sends an MCP initialize through the full middleware stack.
func postMCP(h http.Handler, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/mcp", bytes.NewReader([]byte(mcpInitialize)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// newAuthedMux boots an owner + registry in a temp dir and builds a mux in the
// given auth mode. Mirrors the setup in auth_test.go.
func newAuthedMux(t *testing.T, mode string) (http.Handler, *auth.Store, string) {
	t.Helper()
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
	return NewMux(Config{Version: "test", AuthMode: mode, AuthStore: reg}), reg, ownerID
}

// TestMCPAuthRequiredMode: in required mode the MCP transport binds the
// OWNER's deck, so it must demand the owner's own session token — anonymous
// callers get 401, other accounts 403.
func TestMCPAuthRequiredMode(t *testing.T) {
	h, reg, ownerID := newAuthedMux(t, AuthModeRequired)

	ownerTok, err := reg.CreateSession(ownerID)
	if err != nil {
		t.Fatalf("owner session: %v", err)
	}
	_, userTok, err := reg.CreateUser("mallory@example.com", "hunter2!!", "Mallory")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// No token → 401 (must not fall through to the owner's deck).
	if rec := postMCP(h, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("no token: got %d, want 401 (%s)", rec.Code, rec.Body.String())
	}
	// Garbage token → 401.
	if rec := postMCP(h, "not-a-real-token"); rec.Code != http.StatusUnauthorized {
		t.Errorf("bad token: got %d, want 401 (%s)", rec.Code, rec.Body.String())
	}
	// A valid session that isn't the owner → 403 (real account, wrong deck).
	if rec := postMCP(h, userTok); rec.Code != http.StatusForbidden {
		t.Errorf("non-owner token: got %d, want 403 (%s)", rec.Code, rec.Body.String())
	}
	// The owner's token passes through to the MCP handler proper.
	if rec := postMCP(h, ownerTok); rec.Code != http.StatusOK {
		t.Errorf("owner token: got %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
}

// Open mode keeps the historical behavior: /mcp answers unauthenticated local
// callers (the middleware bypass), bound to the owner's deck.
func TestMCPOpenModeUnauthenticated(t *testing.T) {
	h, _, _ := newAuthedMux(t, AuthModeOpen)
	if rec := postMCP(h, ""); rec.Code != http.StatusOK {
		t.Errorf("open mode, no token: got %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
}
