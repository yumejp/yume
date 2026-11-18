package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSettingsRoundTrip pins the /save_settings + /get_settings wire shape the
// mobile app codes against: the value is an opaque JSON object echoed back
// verbatim, updated_at moves on save, and a never-saved key returns no value.
func TestSettingsRoundTrip(t *testing.T) {
	h, _, _ := newAuthedMux(t, AuthModeOpen)

	// Never saved: no value field, zero updated_at.
	rec := postJSON(h, "/get_settings", `{"key":"mobile"}`)
	if rec.Code != 200 {
		t.Fatalf("get before save: %d %s", rec.Code, rec.Body)
	}
	var got struct {
		Value     json.RawMessage `json:"value"`
		UpdatedAt int64           `json:"updated_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Value != nil || got.UpdatedAt != 0 {
		t.Fatalf("expected empty first read, got %s", rec.Body)
	}

	rec = postJSON(h, "/save_settings", `{"key":"mobile","value":{"autoPlayAudio":false,"newCardsPerDay":5}}`)
	if rec.Code != 200 {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}

	rec = postJSON(h, "/get_settings", `{}`) // key defaults to "mobile"
	if rec.Code != 200 {
		t.Fatalf("get after save: %d %s", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.UpdatedAt == 0 {
		t.Fatalf("expected updated_at after save, got %s", rec.Body)
	}
	var blob map[string]any
	if err := json.Unmarshal(got.Value, &blob); err != nil {
		t.Fatalf("value not an object: %v (%s)", err, rec.Body)
	}
	if blob["autoPlayAudio"] != false || blob["newCardsPerDay"] != float64(5) {
		t.Fatalf("blob not echoed verbatim: %s", got.Value)
	}

	// Guard: a null/non-object value must be rejected, not stored.
	for _, bad := range []string{`{"value":null}`, `{}`, `{"value":[1]}`, `{"value":"x"}`} {
		rec = postJSON(h, "/save_settings", bad)
		if rec.Code != 400 {
			t.Fatalf("save %s: expected 400, got %d %s", bad, rec.Code, rec.Body)
		}
	}
}

// TestSettingsPerUser pins that settings are per-account: a second user's
// save must not leak into the owner's read.
func TestSettingsPerUser(t *testing.T) {
	h, reg, _ := newAuthedMux(t, AuthModeOpen)
	_ = reg

	rec := postJSON(h, "/save_settings", `{"value":{"autoPlayAudio":false}}`)
	if rec.Code != 200 {
		t.Fatalf("owner save: %d %s", rec.Code, rec.Body)
	}

	// The test/role header resolves a different user in open mode.
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/save_settings",
		strings.NewReader(`{"value":{"autoPlayAudio":true}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Yume-User", "test")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("test-user save: %d %s", rec.Code, rec.Body)
	}

	rec = postJSON(h, "/get_settings", `{}`)
	var got struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var blob map[string]any
	if err := json.Unmarshal(got.Value, &blob); err != nil {
		t.Fatal(err)
	}
	if blob["autoPlayAudio"] != false {
		t.Fatalf("owner settings clobbered by test user: %s", got.Value)
	}
}
