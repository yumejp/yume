package auth

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "users.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func sessionCount(t *testing.T, s *Store) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	return n
}

func expireSession(t *testing.T, s *Store, token string) {
	t.Helper()
	if _, err := s.db.Exec(`UPDATE sessions SET expires_at = ? WHERE token_hash = ?`,
		time.Now().Add(-time.Minute).Unix(), hashToken(token)); err != nil {
		t.Fatalf("expire session: %v", err)
	}
}

// Signup → token → login → logout, plus the credential error paths. One
// password account only: bcrypt cost 12 is slow on the Pi.
func TestSignupLoginLogout(t *testing.T) {
	s, _ := openTestStore(t)

	if _, _, err := s.CreateUser("not-an-email", "hunter2!!", ""); !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("bad email: got %v, want ErrInvalidEmail", err)
	}
	if _, _, err := s.CreateUser("a@b.com", "short", ""); !errors.Is(err, ErrWeakPassword) {
		t.Errorf("weak password: got %v, want ErrWeakPassword", err)
	}

	u, token, err := s.CreateUser("  Aki@B.com ", "hunter2!!", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.Email != "aki@b.com" || u.DisplayName != "aki@b.com" || u.Role != RoleUser || u.ID == "" {
		t.Fatalf("unexpected user: %+v", u)
	}
	if got, ok := s.UserForToken(token); !ok || got != u.ID {
		t.Fatalf("signup token resolves to %q/%v, want %q", got, ok, u.ID)
	}
	if got, ok := s.GetUser(u.ID); !ok || got != u {
		t.Errorf("GetUser = %+v/%v, want %+v", got, ok, u)
	}

	if _, _, err := s.CreateUser("AKI@b.com", "another1!", "Dup"); !errors.Is(err, ErrEmailTaken) {
		t.Errorf("duplicate email: got %v, want ErrEmailTaken", err)
	}

	lu, ltoken, err := s.Login("Aki@B.com", "hunter2!!")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if lu.ID != u.ID || ltoken == token {
		t.Fatalf("login: user %q token reused=%v", lu.ID, ltoken == token)
	}
	if got, ok := s.UserForToken(ltoken); !ok || got != u.ID {
		t.Fatalf("login token resolves to %q/%v", got, ok)
	}
	if _, _, err := s.Login("aki@b.com", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("wrong password: got %v, want ErrInvalidCredentials", err)
	}
	if _, _, err := s.Login("nobody@b.com", "hunter2!!"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("unknown email: got %v, want ErrInvalidCredentials", err)
	}

	if err := s.DeleteSession(ltoken); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, ok := s.UserForToken(ltoken); ok {
		t.Error("token still valid after logout")
	}
	if got, ok := s.UserForToken(token); !ok || got != u.ID {
		t.Error("logout revoked the other session too")
	}
	if err := s.DeleteSession(ltoken); err != nil {
		t.Errorf("second DeleteSession: %v", err)
	}
}

// Passwordless owner/test accounts never log in; they are reached by role.
func TestBootstrapAndDevResolve(t *testing.T) {
	s, _ := openTestStore(t)
	ownerID, testID, created, err := s.Bootstrap()
	if err != nil || !created || ownerID == "" || testID == "" || ownerID == testID {
		t.Fatalf("Bootstrap = %q %q %v %v", ownerID, testID, created, err)
	}
	if id, _, created, err := s.Bootstrap(); err != nil || created || id != ownerID {
		t.Fatalf("second Bootstrap = %q %v %v", id, created, err)
	}
	if id, err := s.OwnerID(); err != nil || id != ownerID {
		t.Errorf("OwnerID = %q %v", id, err)
	}
	for _, name := range []string{"owner", "OWNER", ownerID, "Owner"} {
		if id, ok := s.ResolveDevUser(name); !ok || id != ownerID {
			t.Errorf("ResolveDevUser(%q) = %q %v", name, id, ok)
		}
	}
	if _, ok := s.ResolveDevUser("nobody"); ok {
		t.Error("ResolveDevUser resolved an unknown name")
	}
	if _, _, err := s.Login("", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("passwordless login: got %v", err)
	}
}

func TestExpiredSessionRejectedAndDeleted(t *testing.T) {
	s, _ := openTestStore(t)
	ownerID, _, _, err := s.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.CreateSession(ownerID)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if id, ok := s.UserForToken(token); !ok || id != ownerID {
		t.Fatalf("fresh token: %q %v", id, ok)
	}
	expireSession(t, s, token)
	if _, ok := s.UserForToken(token); ok {
		t.Fatal("expired token accepted")
	}
	if n := sessionCount(t, s); n != 0 {
		t.Errorf("expired session not deleted on use: %d rows", n)
	}
	if _, ok := s.UserForToken(""); ok {
		t.Error("empty token accepted")
	}
}

func TestOpenPrunesExpiredSessions(t *testing.T) {
	s, path := openTestStore(t)
	ownerID, _, _, err := s.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	live, err := s.CreateSession(ownerID)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := s.CreateSession(ownerID)
	if err != nil {
		t.Fatal(err)
	}
	expireSession(t, s, stale)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	if n := sessionCount(t, s2); n != 1 {
		t.Fatalf("sessions after reopen = %d, want 1", n)
	}
	if id, ok := s2.UserForToken(live); !ok || id != ownerID {
		t.Errorf("live token lost on reopen: %q %v", id, ok)
	}
}
