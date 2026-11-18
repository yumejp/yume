package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yumejp/yume/server/internal/srs"
)

// postJSON sends a JSON POST through the full middleware stack (loopback
// Host, no Origin — the local-caller class the middleware admits).
func postJSON(h http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestGetVideoVocabEndpoint pins the /get_video_vocab wire shape clients
// codes against: {words:[{lemma,status,in_video_cues,library_videos,...}]},
// sorted by library reach, defaulting to unknown+learning, [] never null.
// Readings/glosses are best-effort (no JMdict in the test env) and asserted
// only structurally at the srs/mcpserver layers.
func TestGetVideoVocabEndpoint(t *testing.T) {
	h, _, ownerID := newAuthedMux(t, AuthModeOpen)
	store, err := srs.ForUser(ownerID)
	if err != nil {
		t.Fatalf("owner store: %v", err)
	}
	if err := store.SetWordStatus("known1", 2); err != nil {
		t.Fatal(err)
	}
	for _, v := range []srs.VideoSnapshotReq{
		{VideoID: "site:A", URL: "https://example.com/a", Title: "A", Platform: "site",
			CueLemmas: [][]string{{"unkA", "known1"}, {"unkA"}, {"unkB"}}},
		{VideoID: "site:B", URL: "https://example.com/b", Title: "B", Platform: "site",
			CueLemmas: [][]string{{"unkA"}}},
	} {
		if _, _, err := store.SaveVideoSnapshot(v); err != nil {
			t.Fatal(err)
		}
	}

	rec := postJSON(h, "/get_video_vocab", `{"video_id":"site:A"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Words []struct {
			Lemma         string `json:"lemma"`
			Status        int    `json:"status"`
			InVideoCues   int    `json:"in_video_cues"`
			LibraryVideos int    `json:"library_videos"`
		} `json:"words"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	// Default statuses exclude known1; sort puts unkA (reach 2) first.
	if len(resp.Words) != 2 {
		t.Fatalf("words = %+v, want [unkA unkB]", resp.Words)
	}
	if w := resp.Words[0]; w.Lemma != "unkA" || w.Status != 0 || w.InVideoCues != 2 || w.LibraryVideos != 2 {
		t.Errorf("words[0] = %+v, want unkA/0/2/2", w)
	}
	if w := resp.Words[1]; w.Lemma != "unkB" || w.InVideoCues != 1 || w.LibraryVideos != 1 {
		t.Errorf("words[1] = %+v, want unkB/1/1", w)
	}

	// Explicit statuses reach the store: [2] returns only the known word.
	rec = postJSON(h, "/get_video_vocab", `{"video_id":"site:A","statuses":[2]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("statuses [2]: status %d: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Words) != 1 || resp.Words[0].Lemma != "known1" || resp.Words[0].Status != 2 {
		t.Errorf("statuses [2]: words = %+v, want only known1", resp.Words)
	}

	// No matching words → "words":[] literally, never null.
	rec = postJSON(h, "/get_video_vocab", `{"video_id":"site:A","statuses":[3]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("statuses [3]: status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"words":[]`) {
		t.Errorf("empty result must serialize as [], got: %s", rec.Body.String())
	}

	// Unknown video → 400 with the store's "video not found" message.
	rec = postJSON(h, "/get_video_vocab", `{"video_id":"site:nope"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "video not found") {
		t.Errorf("missing video: status %d body %s, want 400 'video not found'", rec.Code, rec.Body.String())
	}
}
