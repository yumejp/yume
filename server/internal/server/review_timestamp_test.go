package server

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestReviewTimestampRequiresOfflineGuard(t *testing.T) {
	h, _, _ := newAuthedMux(t, AuthModeOpen)
	rec := postJSON(h, "/review_card", fmt.Sprintf(
		`{"card_id":1,"rating":3,"review_id":"mobile-1","reviewed_at":%d}`,
		time.Now().Unix(),
	))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "requires prev_review_id") {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestReviewTimestampRejectsImplausibleAge(t *testing.T) {
	h, _, _ := newAuthedMux(t, AuthModeOpen)
	rec := postJSON(h, "/review_card", fmt.Sprintf(
		`{"card_id":1,"rating":3,"review_id":"mobile-1","prev_review_id":"","reviewed_at":%d}`,
		time.Now().Add(-maxOfflineReviewAge-time.Hour).Unix(),
	))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "outside the accepted range") {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}
