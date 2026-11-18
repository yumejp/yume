package srs

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// User prefs are opaque per-user client settings blobs (see migration 12).
// The key namespaces the client ("mobile"); the value is JSON the client
// owns the shape of — the store never inspects it.

// GetPref returns the stored blob and its updated_at unix time for a key.
// A key that was never saved returns ("", 0, nil) — absence isn't an error.
func (s *Store) GetPref(key string) (string, int64, error) {
	if key == "" {
		return "", 0, clientErrf("pref key is required")
	}
	var value string
	var updatedAt int64
	err := s.q().QueryRow(
		`SELECT value, updated_at FROM user_prefs WHERE key = ?`, key,
	).Scan(&value, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", 0, nil
		}
		return "", 0, fmt.Errorf("get pref %q: %w", key, err)
	}
	return value, updatedAt, nil
}

// SetPref upserts a key's blob and returns the new updated_at unix time.
func (s *Store) SetPref(key, value string) (int64, error) {
	if key == "" {
		return 0, clientErrf("pref key is required")
	}
	now := time.Now().Unix()
	_, err := s.q().Exec(
		`INSERT INTO user_prefs (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, now,
	)
	if err != nil {
		return 0, fmt.Errorf("set pref %q: %w", key, err)
	}
	return now, nil
}
