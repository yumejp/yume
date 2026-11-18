package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yumejp/yume/server/internal/auth"
	"github.com/yumejp/yume/server/internal/srs"
)

// /health carries the build identity and the API handshake numbers that the
// app and the extension read before calling anything else.
func TestHealthReportsBuildAndAPILevel(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(srs.CloseAll)
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
	h := NewMux(Config{Version: "0.1.0-alpha", Commit: "abc123", AuthMode: AuthModeOpen, AuthStore: reg})

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health: status %d: %s", rec.Code, rec.Body)
	}
	var got struct {
		OK           bool   `json:"ok"`
		Version      string `json:"version"`
		Commit       string `json:"commit"`
		APILevel     int    `json:"api_level"`
		MinClientAPI int    `json:"min_client_api"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.OK || got.Version != "0.1.0-alpha" || got.Commit != "abc123" {
		t.Fatalf("build identity: %+v", got)
	}
	if got.APILevel != APILevel || got.MinClientAPI != MinClientAPI {
		t.Fatalf("api levels: %+v, want %d/%d", got, APILevel, MinClientAPI)
	}
	if MinClientAPI < 1 || MinClientAPI > APILevel {
		t.Fatalf("MinClientAPI %d must be between 1 and APILevel %d", MinClientAPI, APILevel)
	}
}

// A browser extension that names itself with X-Yume-Client must pass the
// CORS preflight, or every call it makes fails before reaching a handler.
// CORS headers go only to extensions listed in YUME_EXTENSION_ORIGINS.
func TestPreflightAllowsClientHeader(t *testing.T) {
	const origin = "chrome-extension://abcdefghijklmnopabcdefghijklmnop"
	saved := extensionOrigins
	t.Cleanup(func() { extensionOrigins = saved })
	extensionOrigins = originSet(origin)
	h := securityMiddleware(nil, okHandler())
	req := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1:7654/health", nil)
	req.Host = "127.0.0.1:7654"
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Headers", "content-type, x-yume-client")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight: status %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !containsFold(got, "X-Yume-Client") {
		t.Fatalf("Access-Control-Allow-Headers %q lacks X-Yume-Client", got)
	}
}

func containsFold(list, name string) bool {
	for _, part := range strings.Split(list, ",") {
		if strings.EqualFold(strings.TrimSpace(part), name) {
			return true
		}
	}
	return false
}
