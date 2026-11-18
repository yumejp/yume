package srs

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

// MigrateLegacyDeckTo moves the single-tenant deck (<dataRoot>/cards.db) to
// <dataRoot>/users/<owner>/cards.db. Idempotent: no legacy file or an existing
// owner deck → (false, nil), never overwrites. Backs up to
// <legacy>.premultiuser.bak first and refuses to proceed if that fails.
func MigrateLegacyDeckTo(ownerID string) (moved bool, err error) {
	legacy, err := resolveDBPath()
	if err != nil {
		return false, err
	}
	if !fileExists(legacy) {
		return false, nil
	}
	ownerPath, err := UserDBPath(ownerID)
	if err != nil {
		return false, err
	}
	if fileExists(ownerPath) {
		return false, nil
	}

	bak := legacy + ".premultiuser.bak"
	if err := copyFile(legacy, bak); err != nil {
		return false, fmt.Errorf("backup legacy deck before migration: %w", err)
	}

	// Checkpoint so the main file holds everything before the move.
	if err := checkpointTruncate(legacy); err != nil {
		return false, fmt.Errorf("checkpoint legacy deck: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(ownerPath), 0o755); err != nil {
		return false, fmt.Errorf("mkdir owner dir: %w", err)
	}
	if err := moveFile(legacy, ownerPath); err != nil {
		return false, fmt.Errorf("move legacy deck: %w", err)
	}

	_ = os.Remove(legacy + "-wal")
	_ = os.Remove(legacy + "-shm")

	// media/ and userdict.txt are derived from the store's path and must move
	// with it; shared assets (JMdict/examples gz, .bak files) stay at the root.
	legacyDir := filepath.Dir(legacy)
	ownerDir := filepath.Dir(ownerPath)
	if media := filepath.Join(legacyDir, "media"); dirExists(media) {
		if err := moveFile(media, filepath.Join(ownerDir, "media")); err != nil {
			return true, fmt.Errorf("move media dir (deck migrated, media left in place): %w", err)
		}
	}
	if ud := filepath.Join(legacyDir, "userdict.txt"); fileExists(ud) {
		if err := moveFile(ud, filepath.Join(ownerDir, "userdict.txt")); err != nil {
			return true, fmt.Errorf("move userdict (deck migrated, userdict left in place): %w", err)
		}
	}
	return true, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func checkpointTruncate(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return err
	}
	return nil
}

// moveFile renames, falling back to copy+remove across filesystems (EXDEV).
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}
