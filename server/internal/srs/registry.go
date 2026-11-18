package srs

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Per-user store registry: each user's deck is an ordinary Store at
// <dataRoot>/users/<id>/cards.db, opened lazily and cached for the process
// lifetime. Media and backups derive from the Store's path, so they are
// isolated per user for free.

var (
	userStoresMu sync.Mutex
	userStores   = map[string]*Store{}
)

// DataRoot is the directory holding the per-user stores (via resolveDBPath).
func DataRoot() (string, error) {
	p, err := resolveDBPath()
	if err != nil {
		return "", err
	}
	return filepath.Dir(p), nil
}

// UserDBPath is the absolute path to a user's card database.
func UserDBPath(userID string) (string, error) {
	root, err := DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "users", userID, "cards.db"), nil
}

// ForUser returns a user's Store, opening it on first use. The mutex is held
// across Open so migrations run once; no eviction.
func ForUser(userID string) (*Store, error) {
	if userID == "" {
		return nil, fmt.Errorf("srs.ForUser: empty user id")
	}
	userStoresMu.Lock()
	defer userStoresMu.Unlock()
	if s, ok := userStores[userID]; ok {
		return s, nil
	}
	path, err := UserDBPath(userID)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir user dir: %w", err)
	}
	s, err := Open(path)
	if err != nil {
		return nil, err
	}
	userStores[userID] = s
	return s, nil
}

// CloseAll closes and forgets every cached store (shutdown; tests on Windows).
func CloseAll() {
	userStoresMu.Lock()
	defer userStoresMu.Unlock()
	for id, s := range userStores {
		_ = s.Close()
		delete(userStores, id)
	}
}

// Owner identity, set by main after bootstrap; used where no request is in
// scope (userdict seeding, open-mode default, MCP).
var (
	ownerMu sync.RWMutex
	ownerID string
)

// SetOwner records the owner user id.
func SetOwner(id string) {
	ownerMu.Lock()
	ownerID = id
	ownerMu.Unlock()
}

// Owner returns the owner's store. Errors if SetOwner hasn't run yet.
func Owner() (*Store, error) {
	ownerMu.RLock()
	id := ownerID
	ownerMu.RUnlock()
	if id == "" {
		return nil, fmt.Errorf("srs.Owner: owner not set (bootstrap did not run)")
	}
	return ForUser(id)
}
