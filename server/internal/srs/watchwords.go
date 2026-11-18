package srs

import (
	"fmt"
	"strings"
	"time"
)

// Watch words — "look out for these": words an AI tutor suggested the
// learner should acquire, pushed in through the MCP endpoint (or any HTTP
// client). Clients list them under Suggested; marking a word
// Known or Ignored retires the suggestion automatically (see SetWordStatus).

// WatchWord is one suggestion: the lemma plus the tutor's one-line reason.
type WatchWord struct {
	Lemma     string `json:"lemma"`
	Note      string `json:"note,omitempty"`
	Source    string `json:"source,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	// Learner's current word status (0..3) — lets a tutor see which of its
	// suggestions have since been learned.
	Status int `json:"status"`
}

// AddWatchWords upserts a batch of suggestions (idempotent per lemma; a
// re-suggest refreshes the note). Lemmas are trimmed; empties skipped.
// Returns the number actually written.
func (s *Store) AddWatchWords(words []WatchWord, source string) (int, error) {
	if source == "" {
		source = "mcp"
	}
	tx, err := s.q().Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().Unix()
	n := 0
	for _, w := range words {
		lemma := strings.TrimSpace(w.Lemma)
		if lemma == "" || len(lemma) > 64 || isJunkLemma(lemma) {
			continue
		}
		note := strings.TrimSpace(w.Note)
		if len(note) > 500 {
			note = note[:500]
		}
		if _, err := tx.Exec(
			`INSERT INTO watch_words (lang, lemma, note, source, created_at) VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(lang, lemma) DO UPDATE SET note = excluded.note, source = excluded.source`,
			s.lang, lemma, note, source, now,
		); err != nil {
			return 0, fmt.Errorf("add watch word %q: %w", lemma, err)
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

// ListWatchWords returns all suggestions joined with the learner's current
// word status (absent row = 0/unknown), newest first.
func (s *Store) ListWatchWords() ([]WatchWord, error) {
	rows, err := s.q().Query(
		`SELECT w.lemma, w.note, w.source, w.created_at, COALESCE(ws.status, 0)
		   FROM watch_words w
		   LEFT JOIN word_status ws ON ws.lang = w.lang AND ws.lemma = w.lemma
		  WHERE w.lang = ?
		  ORDER BY w.created_at DESC, w.lemma`, s.lang,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WatchWord{}
	for rows.Next() {
		var w WatchWord
		var created int64
		if err := rows.Scan(&w.Lemma, &w.Note, &w.Source, &created, &w.Status); err != nil {
			return nil, err
		}
		w.CreatedAt = time.Unix(created, 0).UTC().Format(time.RFC3339)
		out = append(out, w)
	}
	return out, rows.Err()
}

// RemoveWatchWord deletes one suggestion; reports whether it existed.
func (s *Store) RemoveWatchWord(lemma string) (bool, error) {
	res, err := s.q().Exec(`DELETE FROM watch_words WHERE lang = ? AND lemma = ?`, s.lang, strings.TrimSpace(lemma))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ClearWatchWords removes every suggestion; returns how many were removed.
func (s *Store) ClearWatchWords() (int, error) {
	res, err := s.q().Exec(`DELETE FROM watch_words WHERE lang = ?`, s.lang)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// retireWatchWord drops a suggestion once the learner has dealt with the
// word — called from SetWordStatus on Known/Ignored.
func (s *Store) retireWatchWord(lemma string) {
	_, _ = s.q().Exec(`DELETE FROM watch_words WHERE lang = ? AND lemma = ?`, s.lang, lemma)
}
