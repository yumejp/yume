package auth

import (
	"errors"
	"testing"
)

// The operator path for a required-mode owner: give the passwordless owner an
// email and password, then sign in with them. Also covers the role-only
// header lookup and FindAccount's references.
func TestOwnerGetsSignIn(t *testing.T) {
	s, _ := openTestStore(t)
	ownerID, testID, _, err := s.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"owner": ownerID, " Test ": testID} {
		if id, ok := s.ResolveRoleUser(name); !ok || id != want {
			t.Errorf("ResolveRoleUser(%q) = %q %v, want %q", name, id, ok, want)
		}
	}
	for _, name := range []string{ownerID, "Owner account", "user", ""} {
		if id, ok := s.ResolveRoleUser(name); ok {
			t.Errorf("ResolveRoleUser(%q) resolved %q", name, id)
		}
	}

	if err := s.SetEmail(ownerID, "not-an-email"); !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("bad email: got %v", err)
	}
	if err := s.SetEmail(ownerID, " Me@Example.com "); err != nil {
		t.Fatalf("SetEmail: %v", err)
	}
	if err := s.SetPassword(ownerID, "short"); !errors.Is(err, ErrWeakPassword) {
		t.Errorf("weak password: got %v", err)
	}
	if err := s.SetPassword(ownerID, "owner-pass-1"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	u, _, err := s.Login("me@example.com", "owner-pass-1")
	if err != nil || u.ID != ownerID || u.Role != RoleOwner {
		t.Fatalf("owner login = %+v, %v", u, err)
	}
	for _, ref := range []string{"owner", "ME@example.com", ownerID} {
		if got, ok := s.FindAccount(ref); !ok || got.ID != ownerID {
			t.Errorf("FindAccount(%q) = %+v %v", ref, got, ok)
		}
	}
	if _, ok := s.FindAccount("Owner"); !ok {
		t.Error("FindAccount is case-insensitive for role names")
	}
	if got, ok := s.FindAccount("nobody@example.com"); ok {
		t.Errorf("FindAccount resolved an unknown email: %+v", got)
	}
}

// A reset signs the account out everywhere; delete removes the row and its
// sessions and refuses the singleton accounts.
func TestResetAndDeleteAccount(t *testing.T) {
	s, _ := openTestStore(t)
	ownerID, testID, _, err := s.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	u, tok, err := s.CreateUser("friend@example.com", "first-pass-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetEmail(ownerID, "friend@example.com"); !errors.Is(err, ErrEmailTaken) {
		t.Errorf("taken email: got %v", err)
	}
	if err := s.SetPassword(u.ID, "second-pass-2"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.UserForToken(tok); ok {
		t.Error("session survived a password reset")
	}
	if _, _, err := s.Login("friend@example.com", "first-pass-1"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("old password: got %v", err)
	}

	accounts, err := s.ListAccounts()
	if err != nil || len(accounts) != 3 {
		t.Fatalf("ListAccounts = %d accounts, %v", len(accounts), err)
	}
	if a := accounts[2]; a.ID != u.ID || !a.HasPassword || a.Sessions != 0 || a.Email != "friend@example.com" {
		t.Errorf("listed account = %+v", a)
	}
	if accounts[0].HasPassword {
		t.Error("the owner has no password yet")
	}

	if _, err := s.CreateSession(u.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{ownerID, testID} {
		if err := s.DeleteUser(id); !errors.Is(err, ErrProtectedAccount) {
			t.Errorf("DeleteUser(%s): got %v", id, err)
		}
	}
	if err := s.DeleteUser(u.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, ok := s.GetUser(u.ID); ok {
		t.Error("user row survived delete")
	}
	if n := sessionCount(t, s); n != 0 {
		t.Errorf("%d sessions survived delete", n)
	}
	if err := s.DeleteUser(u.ID); err == nil {
		t.Error("deleting a missing user succeeded")
	}
}
