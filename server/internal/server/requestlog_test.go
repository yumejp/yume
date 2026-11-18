package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yumejp/yume/server/internal/srs"
)

// decodeLog parses the JSON lines a test logger collected.
func decodeLog(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("log line is not JSON: %q", line)
		}
		records = append(records, record)
	}
	return records
}

// serveLogged runs one request through logged() the way the mux does and
// returns the response and the records it produced.
func serveLogged(t *testing.T, route string, handler http.HandlerFunc) (*httptest.ResponseRecorder, []map[string]any, string) {
	t.Helper()
	var buf bytes.Buffer
	h := attachLogger(slog.New(slog.NewJSONHandler(&buf, nil)), logged(route, handler))
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+route+"?secret=private-query",
		strings.NewReader(`{"lemma":"private-word"}`))
	req.Header.Set("Authorization", "Bearer private-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, decodeLog(t, &buf), buf.String()
}

func TestLoggedWritesOneRecordWhenTheHandlerReturns(t *testing.T) {
	rec, records, raw := serveLogged(t, "/thing", func(w http.ResponseWriter, r *http.Request) {
		reqLog(r).InfoContext(r.Context(), "from the handler", "event", "test.inside")
		writeErr(w, http.StatusNotFound, "no such thing")
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(records) != 2 {
		t.Fatalf("want the handler's record and the request record: %s", raw)
	}
	inside, request := records[0], records[1]
	id := rec.Header().Get("X-Request-ID")
	if id == "" || inside["request_id"] != id || request["request_id"] != id {
		t.Fatalf("records and header do not share a request id: %s", raw)
	}
	if inside["event"] != "test.inside" || inside["route"] != "/thing" {
		t.Fatalf("handler record = %v", inside)
	}
	if request["event"] != "http.request" || request["route"] != "/thing" || request["method"] != "POST" ||
		request["status"] != float64(http.StatusNotFound) || request["level"] != "WARN" {
		t.Fatalf("request record = %v", request)
	}
	if ms, ok := request["duration_ms"].(float64); !ok || ms < 0 {
		t.Fatalf("duration_ms = %v", request["duration_ms"])
	}
	for _, secret := range []string{"private-query", "private-word", "private-token"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("sensitive value leaked: %q in %s", secret, raw)
		}
	}
}

func TestLoggedLevelFollowsTheStatus(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		status  int
		level   string
	}{
		{"explicit 200", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, map[string]any{"ok": true}) }, 200, "INFO"},
		{"body without a status", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }, 200, "INFO"},
		{"nothing written", func(http.ResponseWriter, *http.Request) {}, 200, "INFO"},
		{"client error", func(w http.ResponseWriter, _ *http.Request) { writeErr(w, http.StatusBadRequest, "bad") }, 400, "WARN"},
		{"server error", func(w http.ResponseWriter, _ *http.Request) { writeErr(w, http.StatusInternalServerError, "broken") }, 500, "ERROR"},
	} {
		_, records, raw := serveLogged(t, "/thing", tc.handler)
		if len(records) != 1 {
			t.Fatalf("%s: want one record: %s", tc.name, raw)
		}
		if r := records[0]; r["status"] != float64(tc.status) || r["level"] != tc.level {
			t.Fatalf("%s: record = %v", tc.name, r)
		}
	}
}

func TestLoggedAnswersAPanicWithoutLoggingItsText(t *testing.T) {
	rec, records, raw := serveLogged(t, "/thing", func(http.ResponseWriter, *http.Request) {
		panic("private-panic")
	})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(records) != 1 {
		t.Fatalf("want one record: %s", raw)
	}
	r := records[0]
	if r["event"] != "http.request" || r["status"] != float64(500) || r["level"] != "ERROR" || r["panic"] != true {
		t.Fatalf("record = %v", r)
	}
	if strings.Contains(raw, "private-panic") || strings.Contains(rec.Body.String(), "private-panic") {
		t.Fatalf("panic text leaked: %s / %s", raw, rec.Body.String())
	}
}

func TestNoteCardSaveOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		saved   srs.SaveCardResp
		err     error
		outcome string
	}{
		{"new card", srs.SaveCardResp{ID: 7, Created: true}, nil, "created"},
		{"word already saved", srs.SaveCardResp{ID: 7}, nil, "existing"},
		{"refused", srs.SaveCardResp{}, errors.New("private-word is not a word"), "failed"},
	} {
		var buf bytes.Buffer
		l := slog.New(slog.NewJSONHandler(&buf, nil))
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/save_card", nil)
		req = req.WithContext(context.WithValue(req.Context(), logCtxKey{}, l))
		noteCardSave(req, tc.saved, tc.err)

		records := decodeLog(t, &buf)
		if len(records) != 1 {
			t.Fatalf("%s: want one record: %s", tc.name, &buf)
		}
		r := records[0]
		if r["event"] != "card.save" || r["outcome"] != tc.outcome {
			t.Fatalf("%s: record = %v", tc.name, r)
		}
		if tc.err == nil {
			if r["card_id"] != float64(7) {
				t.Fatalf("%s: card_id = %v", tc.name, r["card_id"])
			}
		} else if _, ok := r["status"].(float64); !ok {
			t.Fatalf("%s: a failed save records its status: %v", tc.name, r)
		}
		if strings.Contains(buf.String(), "private-word") {
			t.Fatalf("%s: error text leaked: %s", tc.name, &buf)
		}
	}
}

// A real save through the mux: the card.save record and the request record
// share an id, and neither carries the word.
func TestSaveCardIsLoggedWithoutTheWord(t *testing.T) {
	_, reg, _ := newAuthedMux(t, AuthModeOpen)
	var buf bytes.Buffer
	h := NewMux(Config{AuthMode: AuthModeOpen, AuthStore: reg, Logger: slog.New(slog.NewJSONHandler(&buf, nil))})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/save_card", strings.NewReader(`{"lemma":"供給"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body.String())
	}

	records := decodeLog(t, &buf)
	if len(records) != 2 {
		t.Fatalf("want a card.save record and a request record: %s", &buf)
	}
	save, request := records[0], records[1]
	if save["event"] != "card.save" || save["outcome"] != "created" || save["route"] != "/save_card" {
		t.Fatalf("save record = %v", save)
	}
	if request["event"] != "http.request" || request["status"] != float64(http.StatusOK) || request["level"] != "INFO" {
		t.Fatalf("request record = %v", request)
	}
	if id := rec.Header().Get("X-Request-ID"); id == "" || save["request_id"] != id || request["request_id"] != id {
		t.Fatalf("records and header do not share a request id: %s", &buf)
	}
	if strings.Contains(buf.String(), "供給") {
		t.Fatalf("the word reached the log: %s", &buf)
	}
}

// Requests the security or auth middleware refuses leave one http.rejected
// record with a fixed reason, and nothing the client sent.
func TestRejectionsAreLoggedWithAReason(t *testing.T) {
	_, reg, _ := newAuthedMux(t, AuthModeRequired)
	var buf bytes.Buffer
	h := NewMux(Config{AuthMode: AuthModeRequired, AuthStore: reg, Logger: slog.New(slog.NewJSONHandler(&buf, nil))})

	for _, tc := range []struct {
		host, origin, token, reason string
		status                      int
	}{
		{"evil.example", "", "", "forbidden_host", http.StatusForbidden},
		{"127.0.0.1", "https://evil.example", "", "forbidden_origin", http.StatusForbidden},
		{"127.0.0.1", "", "", "no_token", http.StatusUnauthorized},
		{"127.0.0.1", "", "private-bad-token", "invalid_token", http.StatusUnauthorized},
	} {
		buf.Reset()
		req := httptest.NewRequest("POST", "http://"+tc.host+"/save_card?secret=private-query", strings.NewReader(`{"lemma":"private-word"}`))
		req.Header.Set("Content-Type", "application/json")
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		if tc.token != "" {
			req.Header.Set("Authorization", "Bearer "+tc.token)
		}
		req.RemoteAddr = "192.0.2.7:4242"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("%s: status = %d: %s", tc.reason, rec.Code, rec.Body.String())
		}

		// One rejection record, never a route record: the handler did not run.
		records := decodeLog(t, &buf)
		if len(records) != 1 {
			t.Fatalf("want one rejection record: %s", &buf)
		}
		r := records[0]
		if r["event"] != "http.rejected" || r["level"] != "WARN" || r["status"] != float64(tc.status) ||
			r["reason"] != tc.reason || r["route"] != "/save_card" || r["remote_ip"] != "192.0.2.7" {
			t.Fatalf("unexpected rejection record: %v", r)
		}
		if id := rec.Header().Get("X-Request-ID"); id == "" || r["request_id"] != id {
			t.Fatalf("rejection not correlated: %v", r)
		}
		for _, secret := range []string{"private-query", "private-word", "private-bad-token", "evil.example"} {
			if strings.Contains(buf.String(), secret) {
				t.Fatalf("sensitive value leaked: %q", secret)
			}
		}
	}
}
