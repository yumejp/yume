// Package auth owns accounts and sessions in users.db. Card data remains in
// each user's store; package server resolves an authenticated user to a store.
//
// Accounts carry bcrypt-hashed passwords and are authenticated with opaque
// session tokens (UserForToken). The dev "open" auth mode instead resolves
// the owner or test account by role via the X-Yume-User header
// (ResolveRoleUser), bypassing tokens for local development. The operator
// commands (`yume-server users`) manage accounts directly: see accounts.go.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"

	"github.com/yumejp/yume/server/internal/dbsnap"
)

// Session lifetime. Tokens are opaque; clients re-login after expiry.
const sessionTTL = 30 * 24 * time.Hour

// bcryptCost ~12 is a good 2020s default — slow enough to blunt offline
// cracking, fast enough for interactive login.
const bcryptCost = 12

// Errors surfaced to the HTTP auth handlers, which map them to status codes.
var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidEmail       = errors.New("a valid email is required")
	ErrWeakPassword       = errors.New("password must be at least 8 characters")
)

// Role values. Owner and Test are singleton bootstrap accounts.
const (
	RoleOwner = "owner"
	RoleUser  = "user"
	RoleTest  = "test"
)

// User is one account row. Owner and test accounts may be passwordless.
type User struct {
	ID          string `json:"id"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	CreatedAt   int64  `json:"created_at"`
}

// Store wraps the users.db handle.
type Store struct {
	db *sql.DB

	ownerOnce sync.Once
	ownerID   string
	ownerErr  error
}

// Open initializes the registry database. A single connection keeps SQLite
// connection-scoped PRAGMAs consistent for this low-concurrency store.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open users.db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode = WAL; PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("users.db pragmas: %w", err)
	}
	if err := migrate(db, path); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Bulk-prune expired sessions. UserForToken deletes an expired session only
	// when its exact token is presented again, so abandoned tokens otherwise
	// accumulate forever.
	if _, err := db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().Unix()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("prune expired sessions: %w", err)
	}
	return &Store{db: db}, nil
}

// Close shuts down the registry DB. Idempotent.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// users.db schema, versioned in PRAGMA user_version like the decks
// (internal/srs/migrations.go): append-only, one transaction each, values
// frozen inside the migration. Version 1 is the shape every existing file
// already has, so a pre-versioning file moves to 1 without changing.
type migration struct {
	version int
	name    string
	stmts   []string
}

var migrations = []migration{
	{1, "users + sessions", []string{
		`CREATE TABLE IF NOT EXISTS users (
			id            TEXT PRIMARY KEY,
			email         TEXT UNIQUE,
			password_hash TEXT,
			display_name  TEXT NOT NULL,
			role          TEXT NOT NULL DEFAULT 'user',
			created_at    INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			token_hash TEXT PRIMARY KEY,
			user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id)`,
	}},
	{2, "updated_at on users and sessions, kept by the database", []string{
		`ALTER TABLE users ADD COLUMN updated_at INTEGER`,
		`UPDATE users SET updated_at = created_at`,
		`ALTER TABLE sessions ADD COLUMN updated_at INTEGER`,
		`UPDATE sessions SET updated_at = created_at`,
		`CREATE TRIGGER IF NOT EXISTS trg_users_stamp_insert AFTER INSERT ON users
			FOR EACH ROW WHEN NEW.updated_at IS NULL
			BEGIN UPDATE users SET updated_at = COALESCE(created_at, CAST(strftime('%s','now') AS INTEGER)) WHERE rowid = NEW.rowid; END`,
		`CREATE TRIGGER IF NOT EXISTS trg_users_stamp_update AFTER UPDATE ON users
			FOR EACH ROW WHEN NEW.updated_at IS OLD.updated_at
			BEGIN UPDATE users SET updated_at = CAST(strftime('%s','now') AS INTEGER) WHERE rowid = NEW.rowid; END`,
		`CREATE TRIGGER IF NOT EXISTS trg_sessions_stamp_insert AFTER INSERT ON sessions
			FOR EACH ROW WHEN NEW.updated_at IS NULL
			BEGIN UPDATE sessions SET updated_at = COALESCE(created_at, CAST(strftime('%s','now') AS INTEGER)) WHERE rowid = NEW.rowid; END`,
		`CREATE TRIGGER IF NOT EXISTS trg_sessions_stamp_update AFTER UPDATE ON sessions
			FOR EACH ROW WHEN NEW.updated_at IS OLD.updated_at
			BEGIN UPDATE sessions SET updated_at = CAST(strftime('%s','now') AS INTEGER) WHERE rowid = NEW.rowid; END`,
	}},
}

func init() {
	for i := 1; i < len(migrations); i++ {
		if migrations[i].version != migrations[i-1].version+1 {
			panic(fmt.Sprintf("users.db migrations out of order: %d follows %d", migrations[i].version, migrations[i-1].version))
		}
	}
}

// SchemaVersion is the latest users.db version this binary knows.
func SchemaVersion() int { return migrations[len(migrations)-1].version }

// migrate brings users.db up to date. A file that already holds a versioned
// schema is copied to backups/users-premigrate-* first, and a failed copy
// stops the upgrade.
func migrate(db *sql.DB, path string) error {
	var current int
	if err := db.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("users.db user_version: %w", err)
	}
	if latest := SchemaVersion(); current > latest {
		return fmt.Errorf("users.db schema %d is newer than supported schema %d: a newer Yume build wrote it; run that build, or restore the snapshot taken before the upgrade (docs/backup-and-restore.md)", current, latest)
	}
	if current > 0 && current < SchemaVersion() {
		if _, err := dbsnap.BeforeMigrate(db, path, current); err != nil {
			return fmt.Errorf("users.db snapshot before upgrading schema %d, so nothing was changed: %w", current, err)
		}
	}
	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("users.db begin migration %d: %w", m.version, err)
		}
		for _, q := range m.stmts {
			if _, err := tx.Exec(q); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("users.db migration %d (%s) %.40s...: %w", m.version, m.name, q, err)
			}
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("users.db set user_version %d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("users.db commit migration %d: %w", m.version, err)
		}
	}
	return nil
}

// newID returns a random 16-byte hex id. crypto/rand avoids a uuid dependency
// and the ids double as on-disk directory names (users/<id>/).
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// Bootstrap ensures the singleton owner and test accounts exist. It also
// reports whether the owner was created so main can initialize its deck.
func (s *Store) Bootstrap() (ownerID, testID string, ownerCreated bool, err error) {
	ownerID, ownerCreated, err = s.ensureRoleUser(RoleOwner, "Owner")
	if err != nil {
		return "", "", false, err
	}
	testID, _, err = s.ensureRoleUser(RoleTest, "Test")
	if err != nil {
		return "", "", false, err
	}
	return ownerID, testID, ownerCreated, nil
}

// ensureRoleUser finds the (singleton) user with the given role, creating one
// if absent. Used for owner/test only.
func (s *Store) ensureRoleUser(role, displayName string) (id string, created bool, err error) {
	err = s.db.QueryRow(`SELECT id FROM users WHERE role = ? LIMIT 1`, role).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if err != sql.ErrNoRows {
		return "", false, fmt.Errorf("lookup role %s: %w", role, err)
	}
	id, err = newID()
	if err != nil {
		return "", false, err
	}
	if _, err = s.db.Exec(
		`INSERT INTO users (id, email, password_hash, display_name, role, created_at)
		 VALUES (?, NULL, NULL, ?, ?, ?)`,
		id, displayName, role, time.Now().Unix(),
	); err != nil {
		return "", false, fmt.Errorf("create role %s: %w", role, err)
	}
	return id, true, nil
}

// OwnerID returns the owner account's id, cached after first resolution.
func (s *Store) OwnerID() (string, error) {
	s.ownerOnce.Do(func() {
		s.ownerErr = s.db.QueryRow(`SELECT id FROM users WHERE role = ? LIMIT 1`, RoleOwner).Scan(&s.ownerID)
	})
	return s.ownerID, s.ownerErr
}

// GetUser returns the account row for an id.
func (s *Store) GetUser(id string) (User, bool) {
	var u User
	var email sql.NullString
	err := s.db.QueryRow(
		`SELECT id, email, display_name, role, created_at FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &email, &u.DisplayName, &u.Role, &u.CreatedAt)
	if err != nil {
		return User{}, false
	}
	u.Email = email.String
	return u, true
}

// ResolveRoleUser maps an X-Yume-User header value to the singleton owner or
// test account, case-insensitively. Only those role names resolve: an email,
// id or display name never does, so the open-mode header cannot select an
// account that signed up with a password.
func (s *Store) ResolveRoleUser(name string) (string, bool) {
	role := strings.ToLower(strings.TrimSpace(name))
	if role != RoleOwner && role != RoleTest {
		return "", false
	}
	var id string
	if err := s.db.QueryRow(`SELECT id FROM users WHERE role = ? LIMIT 1`, role).Scan(&id); err != nil {
		return "", false
	}
	return id, true
}

// ResolveDevUser maps an operator's command-line user reference to a user
// id. Matches (in order) an exact id, a role name (e.g. "test"/"owner"), an
// email, or a display name — case-insensitive for the latter three. The
// HTTP X-Yume-User header uses the stricter ResolveRoleUser.
func (s *Store) ResolveDevUser(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	var id string
	// Exact id.
	if err := s.db.QueryRow(`SELECT id FROM users WHERE id = ?`, name).Scan(&id); err == nil {
		return id, true
	}
	// Role / email / display name, case-insensitive.
	lower := strings.ToLower(name)
	err := s.db.QueryRow(
		`SELECT id FROM users
		   WHERE lower(role) = ? OR lower(email) = ? OR lower(display_name) = ?
		   LIMIT 1`,
		lower, lower, lower,
	).Scan(&id)
	if err != nil {
		return "", false
	}
	return id, true
}

// CreateUser registers a new password account (role 'user') and returns the
// account plus a fresh session token. Email is normalized to lowercase and
// must be unique.
func (s *Store) CreateUser(email, password, displayName string) (User, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !strings.Contains(email, "@") {
		return User{}, "", ErrInvalidEmail
	}
	if len(password) < 8 {
		return User{}, "", ErrWeakPassword
	}
	if strings.TrimSpace(displayName) == "" {
		displayName = email
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return User{}, "", fmt.Errorf("hash password: %w", err)
	}
	id, err := newID()
	if err != nil {
		return User{}, "", err
	}
	now := time.Now().Unix()
	_, err = s.db.Exec(
		`INSERT INTO users (id, email, password_hash, display_name, role, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, email, string(hash), displayName, RoleUser, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return User{}, "", ErrEmailTaken
		}
		return User{}, "", fmt.Errorf("create user: %w", err)
	}
	u := User{ID: id, Email: email, DisplayName: displayName, Role: RoleUser, CreatedAt: now}
	tok, err := s.CreateSession(id)
	if err != nil {
		return User{}, "", err
	}
	return u, tok, nil
}

// Login verifies email + password and returns the account plus a fresh session
// token. Passwordless accounts (owner/test) cannot log in; they are reached
// via the dev X-Yume-User header in open mode until the operator gives them
// an email and password (`yume-server users owner`).
func (s *Store) Login(email, password string) (User, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var u User
	var em, hash sql.NullString
	err := s.db.QueryRow(
		`SELECT id, email, password_hash, display_name, role, created_at FROM users WHERE email = ?`, email,
	).Scan(&u.ID, &em, &hash, &u.DisplayName, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return User{}, "", ErrInvalidCredentials
	}
	if err != nil {
		return User{}, "", err
	}
	if !hash.Valid || bcrypt.CompareHashAndPassword([]byte(hash.String), []byte(password)) != nil {
		return User{}, "", ErrInvalidCredentials
	}
	u.Email = em.String
	tok, err := s.CreateSession(u.ID)
	if err != nil {
		return User{}, "", err
	}
	return u, tok, nil
}

// CreateSession issues an opaque 32-byte token, storing only its sha256 hash
// (so a leaked DB doesn't expose live tokens). The raw token is returned once.
func (s *Store) CreateSession(userID string) (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	now := time.Now()
	if _, err := s.db.Exec(
		`INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		hashToken(token), userID, now.Unix(), now.Add(sessionTTL).Unix(),
	); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return token, nil
}

// DeleteSession revokes one token (logout). No-op if the token is unknown.
func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, hashToken(token))
	return err
}

// UserForToken resolves a session bearer token to a user id, rejecting (and
// opportunistically deleting) expired sessions.
func (s *Store) UserForToken(token string) (string, bool) {
	if token == "" {
		return "", false
	}
	th := hashToken(token)
	var userID string
	var expires int64
	if err := s.db.QueryRow(
		`SELECT user_id, expires_at FROM sessions WHERE token_hash = ?`, th,
	).Scan(&userID, &expires); err != nil {
		return "", false
	}
	if time.Now().Unix() >= expires {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, th)
		return "", false
	}
	return userID, true
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
