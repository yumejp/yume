package srs

import (
	"database/sql"
	"fmt"
	"time"
)

// Furigana mode is for words known by sound whose kanji reading won't stick:
// the card front shows the reading, so review tests the meaning. The word
// stays Learning while the tag is on. A card switches itself on at its
// furiganaAfterLapses-th lapse (again on any later miss, so trying it without
// furigana and failing puts it back) and is offered back once it has held
// for furiganaSteadyDays.
const (
	furiganaAfterLapses = 8
	furiganaSteadyDays  = 30
)

// switchToFurigana tags lemma and reports whether it was newly tagged.
func switchToFurigana(tx *sql.Tx, lang, lemma string, now time.Time) (bool, error) {
	res, err := tx.Exec(`INSERT OR IGNORE INTO word_tags (lang, lemma, tag, kind, created_at, updated_at)
		VALUES (?, ?, ?, 'system', ?, ?)`, lang, lemma, TagFurigana, now.Unix(), now.Unix())
	if err != nil {
		return false, fmt.Errorf("furigana: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// demoteKnown moves a Known lemma back to Learning.
func demoteKnown(q execer, lang, lemma string, now time.Time) (bool, error) {
	res, err := q.Exec(`UPDATE word_status SET status = 1, updated_at = ? WHERE lang = ? AND lemma = ? AND status = 2`, now.Unix(), lang, lemma)
	if err != nil {
		return false, fmt.Errorf("furigana demote: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
