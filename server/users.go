package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yumejp/yume/server/internal/auth"
	"github.com/yumejp/yume/server/internal/srs"
)

const usersUsage = `usage: yume-server users <command> [-data DIR]

  list                          every account: email, role, password set, sessions
  owner EMAIL                   give the owner account this sign-in email and a new
                                password, so it can sign in when YUME_AUTH_MODE=required
  set-password EMAIL            replace an account's password and sign it out everywhere
  delete EMAIL -yes             remove an account, its sessions and its deck folder
  export EMAIL [FILE]           write the account's deck as an Anki .apkg

EMAIL may also be "owner" or "test". A new password is generated and printed
once; pass -password-stdin to read one from standard input instead.`

// runUsers is the operator's account tooling, so nobody edits users.db by
// hand. It works on the data folder directly and never over HTTP.
func runUsers(args []string) {
	fs := flag.NewFlagSet("users", flag.ExitOnError)
	data := fs.String("data", "", "data folder")
	yes := fs.Bool("yes", false, "delete: confirm")
	fromStdin := fs.Bool("password-stdin", false, "owner, set-password: read the new password from standard input")
	fs.Usage = func() { fmt.Fprintln(os.Stderr, usersUsage) }
	// Words may come before or after the flags, as in `mnemonics`.
	var words []string
	for len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		words, args = append(words, args[0]), args[1:]
	}
	fs.Parse(args)
	words = append(words, fs.Args()...)
	if len(words) == 0 {
		fs.Usage()
		os.Exit(2)
	}
	verb, words := words[0], words[1:]
	want := map[string][2]int{"list": {0, 0}, "owner": {1, 1}, "set-password": {1, 1}, "delete": {1, 1}, "export": {1, 2}}
	n, ok := want[verb]
	if !ok || len(words) < n[0] || len(words) > n[1] {
		fs.Usage()
		os.Exit(2)
	}

	applyDataDir(*data)
	root, err := srs.DataRoot()
	if err != nil {
		log.Fatal(err)
	}
	// Never create a registry here: the server's first start does that and
	// moves a single-user deck under the owner, which a registry created
	// early would skip.
	usersDB := filepath.Join(root, "users.db")
	if !fileExists(usersDB) {
		log.Fatalf("no accounts in %s yet: start the server once with this data folder, or pass -data", root)
	}
	reg, err := auth.Open(usersDB)
	if err != nil {
		log.Fatal(err)
	}
	defer reg.Close()

	find := func(ref string) auth.User {
		u, ok := reg.FindAccount(ref)
		if !ok {
			log.Fatalf("no account %q: run `yume-server users list` to see them", ref)
		}
		return u
	}

	switch verb {
	case "list":
		accounts, err := reg.ListAccounts()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%-32s  %-6s  %-8s  %-8s  %-10s  %s\n", "EMAIL", "ROLE", "PASSWORD", "SESSIONS", "CREATED", "ID")
		for _, a := range accounts {
			email := a.Email
			if email == "" {
				email = "-"
			}
			pw := "no"
			if a.HasPassword {
				pw = "yes"
			}
			fmt.Printf("%-32s  %-6s  %-8s  %-8d  %-10s  %s\n", email, a.Role, pw, a.Sessions,
				time.Unix(a.CreatedAt, 0).Format("2006-01-02"), a.ID)
		}
	case "owner":
		owner := find(auth.RoleOwner)
		pw := newPassword(*fromStdin)
		if err := reg.SetEmail(owner.ID, words[0]); err != nil {
			log.Fatal(err)
		}
		if err := reg.SetPassword(owner.ID, pw); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("The owner account signs in as %s.\n", strings.ToLower(strings.TrimSpace(words[0])))
		printPassword(pw, *fromStdin)
		fmt.Println("Open mode still works as before; with YUME_AUTH_MODE=required, sign in with these in the app.")
	case "set-password":
		u := find(words[0])
		if u.Email == "" {
			log.Fatalf("%s has no sign-in email: for the owner, use `yume-server users owner EMAIL`", words[0])
		}
		pw := newPassword(*fromStdin)
		if err := reg.SetPassword(u.ID, pw); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("New password for %s; its other sessions are signed out.\n", u.Email)
		printPassword(pw, *fromStdin)
	case "delete":
		u := find(words[0])
		if u.Role == auth.RoleOwner || u.Role == auth.RoleTest {
			log.Fatal(auth.ErrProtectedAccount)
		}
		dbPath, err := srs.UserDBPath(u.ID)
		if err != nil {
			log.Fatal(err)
		}
		deckDir := filepath.Dir(dbPath)
		if !*yes {
			fmt.Fprintf(os.Stderr, "This removes %s, its sessions and its deck folder %s, including cards, reviews and media.\n"+
				"Export it first if you may need it (`yume-server users export %s`). Run again with -yes to delete.\n", u.Email, deckDir, u.Email)
			os.Exit(2)
		}
		if err := reg.DeleteUser(u.ID); err != nil {
			log.Fatal(err)
		}
		if err := os.RemoveAll(deckDir); err != nil {
			log.Fatalf("deleted the account, but its deck folder could not be removed (%v): stop the server and delete %s", err, deckDir)
		}
		fmt.Printf("Deleted %s and %s.\n", u.Email, deckDir)
	case "export":
		u := find(words[0])
		file := ""
		if len(words) == 2 {
			file = words[1]
		} else {
			name := u.Email
			if name == "" {
				name = u.Role
			}
			file = fmt.Sprintf("yume-%s-%s.apkg", strings.NewReplacer("@", "-at-", "/", "-", `\`, "-").Replace(name), time.Now().Format("2006-01-02"))
		}
		store, err := srs.ForUser(u.ID)
		if err != nil {
			log.Fatal(err)
		}
		defer srs.CloseAll()
		b, err := store.ExportApkg("")
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(file, b, 0o600); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Wrote %s (%d bytes). It imports into Anki with scheduling, review history and media.\n", file, len(b))
	}
}

// newPassword reads one line from standard input, or generates a password
// the operator hands over privately.
func newPassword(fromStdin bool) string {
	if fromStdin {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			log.Fatal(err)
		}
		pw := strings.TrimRight(line, "\r\n")
		if len(pw) < 8 {
			log.Fatal(auth.ErrWeakPassword)
		}
		return pw
	}
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		log.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func printPassword(pw string, fromStdin bool) {
	if fromStdin {
		fmt.Println("Password: the one read from standard input.")
		return
	}
	fmt.Printf("Password: %s\nIt is shown only now; share it privately.\n", pw)
}
