package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yumejp/yume/server/internal/auth"
	"github.com/yumejp/yume/server/internal/srs"
)

// TestLicensesPublic: the notices reach every user, signed in or not, and
// carry the credits the licenses require.
func TestLicensesPublic(t *testing.T) {
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

	for _, mode := range []string{AuthModeOpen, AuthModeRequired} {
		h := NewMux(Config{Version: "test", AuthMode: mode, AuthStore: reg})

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/licenses", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: GET /licenses = %d, want 200", mode, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
			t.Errorf("%s: Content-Type %q, want text/plain", mode, ct)
		}
		body := rec.Body.String()
		for _, want := range []string{"Electronic Dictionary Research and Development Group", "NO WARRANTY", "modernc.org/sqlite", "react-native-web"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: /licenses lacks %q", mode, want)
			}
		}

		rec = httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/licenses", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: POST /licenses = %d, want 405", mode, rec.Code)
		}
	}
}
