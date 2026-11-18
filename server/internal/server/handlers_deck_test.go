package server

import (
	"strings"
	"testing"
)

// A lemma the store refuses (empty, or punctuation from degraded
// tokenization) is the client's fault: 400, not 500.
func TestSaveCardRejectsBadLemma(t *testing.T) {
	h, _, _ := newAuthedMux(t, AuthModeOpen)
	for _, tc := range []struct{ body, want string }{
		{`{"lemma":""}`, "lemma is required"},
		{`{"lemma":"？"}`, "punctuation"},
	} {
		rec := postJSON(h, "/save_card", tc.body)
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%s: got %d %s, want 400 %q", tc.body, rec.Code, rec.Body, tc.want)
		}
	}
	if rec := postJSON(h, "/save_card", `{"lemma":"供給"}`); rec.Code != 200 {
		t.Fatalf("valid save: %d %s", rec.Code, rec.Body)
	}
}
