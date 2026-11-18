package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestDeckStatsKeepsShapeWithoutCatchUp(t *testing.T) {
	h, _, _ := newAuthedMux(t, AuthModeOpen)
	rec := postJSON(h, "/deck_stats", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"total", "due", "due_reviews"} {
		if _, ok := body[k]; !ok {
			t.Errorf("deck_stats lost %q: %s", k, rec.Body.String())
		}
	}
	if _, ok := body["catch_up"]; ok {
		t.Errorf("empty deck should have no catch_up: %s", rec.Body.String())
	}
}
