package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// Operator account management, for the `yume-server users` commands. None of
// this is reachable over HTTP: whoever runs these owns the data folder.

// ErrProtectedAccount refuses to delete the singleton owner or test account;
// the server recreates them on start and the owner's deck is the default.
var ErrProtectedAccount = errors.New("the owner and test accounts cannot be deleted")

// Account is a User plus the facts an operator needs when listing them.
type Account struct {
	User
	HasPassword bool
	Sessions    int
}

// ListAccounts returns every account, oldest first.
func (s *Store) ListAccounts() ([]Account, error) {
	rows, err := s.db.Query(`
		SELECT u.id, u.email, u.display_name, u.role, u.created_at,
		       u.password_hash IS NOT NULL,
		       (SELECT COUNT(*) FROM sessions WHERE user_id = u.id)
		  FROM users u ORDER BY u.created_at, u.rowid`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		var a Account
		var email sql.NullString
		if err := rows.Scan(&a.ID, &email, &a.DisplayName, &a.Role, &a.CreatedAt, &a.HasPassword, &a.Sessions); err != nil {
			return nil, err
		}
		a.Email = email.String
		out = append(out, a)
	}
	return out, rows.Err()
}

// FindAccount resolves an operator's reference: an email, an exact id, or
// the role name owner or test. Display names are not unique, so they never
// match.
func (s *Store) FindAccount(ref string) (User, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return User{}, false
	}
	var id string
	lower := strings.ToLower(ref)
	switch {
	case lower == RoleOwner || lower == RoleTest:
		if err := s.db.QueryRow(`SELECT id FROM users WHERE role = ? LIMIT 1`, lower).Scan(&id); err != nil {
			return User{}, false
		}
	default:
		if err := s.db.QueryRow(`SELECT id FROM users WHERE id = ? OR email = ?`, ref, lower).Scan(&id); err != nil {
			return User{}, false
		}
	}
	return s.GetUser(id)
}

// SetPassword replaces an account's password and signs it out everywhere, so
// a reset also ends any session someone else might hold.
func (s *Store) SetPassword(userID, password string) error {
	if len(password) < 8 {
		return ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, string(hash), userID)
	if err != nil {
		return fmt.Errorf("set password: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("user %s: not found", userID)
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("sign out sessions: %w", err)
	}
	return tx.Commit()
}

// SetEmail gives an account its sign-in email (the owner has none until the
// operator sets one). The email is normalized like signup's.
func (s *Store) SetEmail(userID, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !strings.Contains(email, "@") {
		return ErrInvalidEmail
	}
	var other string
	err := s.db.QueryRow(`SELECT id FROM users WHERE email = ?`, email).Scan(&other)
	switch {
	case err == nil && other != userID:
		return ErrEmailTaken
	case err != nil && err != sql.ErrNoRows:
		return fmt.Errorf("look up email: %w", err)
	}
	res, err := s.db.Exec(`UPDATE users SET email = ? WHERE id = ?`, email, userID)
	if err != nil {
		return fmt.Errorf("set email: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("user %s: not found", userID)
	}
	return nil
}

// DeleteUser removes an account and its sessions from the registry. The
// caller removes the deck folder (users/<id>/). Owner and test are refused.
func (s *Store) DeleteUser(userID string) error {
	u, ok := s.GetUser(userID)
	if !ok {
		return fmt.Errorf("user %s: not found", userID)
	}
	if u.Role == RoleOwner || u.Role == RoleTest {
		return ErrProtectedAccount
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("delete sessions: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM users WHERE id = ?`, userID); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return tx.Commit()
}
