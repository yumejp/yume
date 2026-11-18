package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yumejp/yume/server/internal/srs"
)

// A deck this build refuses answers with the refusal's code, and the log
// carries only its class: the message names files.
func TestRefusedDeckLogsItsKind(t *testing.T) {
	_, reg, ownerID := newAuthedMux(t, AuthModeOpen)
	path, err := srs.UserDBPath(ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 20`); err != nil { // a pre-release deck
		t.Fatal(err)
	}
	db.Close()

	var buf bytes.Buffer
	h := NewMux(Config{AuthMode: AuthModeOpen, AuthStore: reg, Logger: slog.New(slog.NewJSONHandler(&buf, nil))})
	req := httptest.NewRequest("POST", "http://127.0.0.1/deck_stats", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Yume-User", "owner")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 500 {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Error, Code string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != srs.ErrKindSchemaPreRelease || !strings.Contains(body.Error, "pre-release") {
		t.Fatalf("response = %+v", body)
	}
	records := decodeLog(t, &buf)
	if len(records) != 2 || records[0]["event"] != "store.open_failed" || records[0]["error_kind"] != srs.ErrKindSchemaPreRelease {
		t.Fatalf("want a store.open_failed record with its kind: %s", &buf)
	}
	if strings.Contains(buf.String(), filepath.Base(filepath.Dir(path))) || strings.Contains(buf.String(), "pre-release Yume") {
		t.Fatalf("the log must not carry the message or paths: %s", &buf)
	}
}
