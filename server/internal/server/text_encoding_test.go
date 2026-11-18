package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Japanese that a shell lost on the way in is refused with a 400 that
// names the cause, instead of being saved with a 200: '?' from a
// Western-locale Windows shell, and Shift-JIS bytes (invalid UTF-8) from a
// Japanese-locale one.
func TestContentWritesRefuseLostJapanese(t *testing.T) {
	h, _, _ := newAuthedMux(t, AuthModeOpen)
	rec := postJSON(h, "/save_card", `{"lemma":"意見","sentence":"意見が分かれる。"}`)
	if rec.Code != 200 {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	var saved struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil || saved.ID == 0 {
		t.Fatalf("save response %s: %v", rec.Body, err)
	}
	sjis := string([]byte{0x88, 0xd3, 0x8c, 0xa9}) // 意見 in Shift-JIS
	for _, tc := range []struct{ path, body, want string }{
		{"/update_card", fmt.Sprintf(`{"card_id":%d,"sentence":"??????"}`, saved.ID), "encoding"},
		{"/add_card_note", fmt.Sprintf(`{"card_id":%d,"kind":"tutor","note":"%s"}`, saved.ID, sjis), "UTF-8"},
		{"/import_csv", `{"tag":"t","rows":[{"lemma":"??"}]}`, "row 1 lemma"},
	} {
		rec := postJSON(h, tc.path, tc.body)
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%s %s: got %d %s, want 400 %q", tc.path, tc.body, rec.Code, rec.Body, tc.want)
		}
	}
	rec = postJSON(h, "/add_card_note", fmt.Sprintf(`{"card_id":%d,"kind":"example","note":"意見を言う。"}`, saved.ID))
	if rec.Code != 200 {
		t.Errorf("example without translation: %d %s", rec.Code, rec.Body)
	}
}
