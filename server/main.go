// Yume server: the HTTP API every Yume client builds on. Security
// model: internal/server/middleware.go.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/yumejp/yume/server/internal/auth"
	"github.com/yumejp/yume/server/internal/dict"
	"github.com/yumejp/yume/server/internal/server"
	"github.com/yumejp/yume/server/internal/setup"
	"github.com/yumejp/yume/server/internal/srs"
	"github.com/yumejp/yume/server/internal/tokenize"
)

// Version and Commit identify the build in /health, the MCP handshake and
// the startup log. Release builds stamp both:
//
//	go build -ldflags "-X main.Version=0.1.0-alpha -X main.Commit=$(git rev-parse HEAD)"
//
// An unstamped build reports "dev", and its commit from the Go toolchain's
// VCS stamp when it was built inside a git checkout.
var (
	Version = "dev"
	Commit  = ""
)

// buildCommit is the stamped commit, else the toolchain's VCS revision
// (suffixed "-dirty" for uncommitted changes), else "".
func buildCommit() string {
	if Commit != "" {
		return Commit
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var rev, dirty string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if rev == "" {
		return ""
	}
	return rev + dirty
}

const DefaultPort = 7654

// usage prints the subcommands; plain `yume-server` serves.
func usage() {
	fmt.Fprintln(os.Stderr, `Yume server

  yume-server setup [-data DIR]     download the dictionary files into the data folder
  yume-server voicevox [-data DIR]  install the optional VoiceVox engine for card audio
  yume-server dict ja-ja [-data DIR]  install an optional dictionary (ja-ja: Japanese definitions)
  yume-server mnemonics share|export|import [FILE] [-data DIR] [-user ID]
                                    the shared mnemonic library: move a deck's mnemonics into it,
                                    write it to a JSON file, or load one
  yume-server users list|owner|set-password|delete|export [EMAIL] [-data DIR]
                                    manage accounts; "yume-server users" lists the commands
  yume-server [-bind ADDR] [-port N] [-data DIR]   serve

Data folder: -data, else $YUME_DB's folder, else the OS default
(~/.local/share/yume, ~/Library/Application Support/Yume, %LOCALAPPDATA%\Yume).
YUME_DICTS=ja-ja while serving installs that dictionary on start if missing.

YUME_AUTH_MODE is open (the default: every request acts as the owner) or
required (every request needs a signed-in account). Open mode refuses a
non-loopback -bind unless YUME_ALLOW_OPEN_NETWORK=1 says the address is
private (Tailscale, WireGuard): anyone who can reach it acts as the owner.`)
}

// openOwnerStore bootstraps the user registry and returns the owner's deck,
// the way serving does, for subcommands that write to it.
func openOwnerStore() (*srs.Store, func(), error) {
	root, err := srs.DataRoot()
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, nil, err
	}
	authStore, err := auth.Open(filepath.Join(root, "users.db"))
	if err != nil {
		return nil, nil, err
	}
	ownerID, _, _, err := authStore.Bootstrap()
	if err != nil {
		authStore.Close()
		return nil, nil, err
	}
	srs.SetOwner(ownerID)
	store, err := srs.Owner()
	if err != nil {
		authStore.Close()
		return nil, nil, err
	}
	return store, func() { authStore.Close() }, nil
}

// installDict downloads (if needed) and imports one optional dictionary into
// the owner's deck. Safe to repeat: an installed dictionary is skipped.
func installDict(store *srs.Store, key string, out io.Writer) error {
	d, ok := setup.Dictionaries[key]
	if !ok {
		return fmt.Errorf("unknown dictionary %q; available: %v", key, setup.DictionaryKeys())
	}
	root, err := srs.DataRoot()
	if err != nil {
		return err
	}
	zipPath, err := setup.DictionaryZip(root, key, out)
	if err != nil {
		return err
	}
	n, created, err := store.ImportYomitanZip(zipPath, srs.CreateDictReq{
		Name: d.Title, Author: d.Author, URL: d.Homepage, Description: d.Description, GlossLang: d.GlossLang,
	})
	if err != nil {
		return err
	}
	if !created {
		fmt.Fprintf(out, "  %s is already installed\n", d.Title)
		return nil
	}
	fmt.Fprintf(out, "  %s: %d entries imported\n", d.Title, n)
	return nil
}

// runMnemonics manages the shared library (content.db):
//
//	share   copy a deck's own mnemonics and kanji stories into the library
//	        where it has none and clear the copies that now match it, so the
//	        deck keeps only overrides
//	export  write the library as JSON (stdout, or FILE)
//	import  add a JSON file's entries the library lacks (-overwrite replaces)
func runMnemonics(args []string) {
	fs := flag.NewFlagSet("mnemonics", flag.ExitOnError)
	data := fs.String("data", "", "data folder")
	user := fs.String("user", "owner", "deck to share from: a user id, role or email")
	lang := fs.String("lang", "", "language to export (default: all)")
	overwrite := fs.Bool("overwrite", false, "import: replace entries the library already has")
	// Words may come before or after the flags: `export FILE -data X` and
	// `-data X export FILE` both read naturally, and flag.Parse stops at
	// the first bare word.
	var words []string
	for len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		words, args = append(words, args[0]), args[1:]
	}
	fs.Parse(args)
	words = append(words, fs.Args()...)
	verb, file := "", ""
	if len(words) > 0 {
		verb, words = words[0], words[1:]
	}
	if len(words) == 1 {
		file = words[0]
	}
	if verb == "" || len(words) > 1 || (verb == "import" && file == "") {
		fmt.Fprintln(os.Stderr, "usage: yume-server mnemonics share [-user ID] | export [FILE] [-lang ja] | import FILE [-overwrite]   [-data DIR]")
		os.Exit(2)
	}
	applyDataDir(*data)
	root, err := srs.DataRoot()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		log.Fatal(err)
	}
	lib, err := srs.OpenLibrary(filepath.Join(root, "content.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer lib.Close()
	switch verb {
	case "share":
		authStore, err := auth.Open(filepath.Join(root, "users.db"))
		if err != nil {
			log.Fatal(err)
		}
		defer authStore.Close()
		if _, _, _, err := authStore.Bootstrap(); err != nil {
			log.Fatal(err)
		}
		uid, ok := authStore.ResolveDevUser(*user)
		if !ok {
			log.Fatalf("user %q: not found", *user)
		}
		store, err := srs.ForUser(uid)
		if err != nil {
			log.Fatal(err)
		}
		defer srs.CloseAll()
		shared, cleared, err := store.ShareMnemonics(lib)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("shared %d mnemonics into the library; %d cards now read the shared one\n", shared, cleared)
		shared, cleared, err = store.ShareKanjiNotes(lib)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("shared %d kanji stories into the library; %d now read the shared one\n", shared, cleared)
	case "export":
		content, err := lib.ExportAll(*lang)
		if err != nil {
			log.Fatal(err)
		}
		b, _ := json.MarshalIndent(content, "", "  ")
		b = append(b, '\n')
		if file == "" {
			os.Stdout.Write(b)
		} else if err := os.WriteFile(file, b, 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Fprintf(os.Stderr, "%d mnemonics, %d kanji stories\n", len(content.Mnemonics), len(content.KanjiNotes))
	case "import":
		b, err := os.ReadFile(file)
		if err != nil {
			log.Fatal(err)
		}
		// A file is the export object, or (older exports) a bare mnemonic array.
		var content srs.LibraryContent
		if err := json.Unmarshal(b, &content); err != nil {
			if err2 := json.Unmarshal(b, &content.Mnemonics); err2 != nil {
				log.Fatalf("%s: %v", file, err)
			}
		}
		n, err := lib.ImportAll(content, *overwrite)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("added %d, replaced %d, skipped %d\n", n.Added, n.Replaced, n.Skipped)
	default:
		log.Fatalf("unknown verb %q (share, export, import)", verb)
	}
}

func runDict(args []string) {
	fs := flag.NewFlagSet("dict", flag.ExitOnError)
	data := fs.String("data", "", "data folder")
	// The name may come before or after the flags: `dict ja-ja -data X` reads
	// as naturally as `dict -data X ja-ja`, and flag.Parse stops at the first
	// bare word.
	name := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name, args = args[0], args[1:]
	}
	fs.Parse(args)
	if name == "" && fs.NArg() == 1 {
		name = fs.Arg(0)
	}
	if name == "" || fs.NArg() > 1 {
		fmt.Fprintf(os.Stderr, "usage: yume-server dict <name> [-data DIR]\navailable: %v\n", setup.DictionaryKeys())
		os.Exit(2)
	}
	applyDataDir(*data)
	store, closeFn, err := openOwnerStore()
	if err != nil {
		log.Fatal(err)
	}
	defer closeFn()
	if err := installDict(store, name, os.Stdout); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Done. In the app, Settings › Definitions › Japanese uses it.")
}

// applyDataDir makes -data the root for the database, media and dictionary
// files by setting the environment the loaders already read.
func applyDataDir(dir string) {
	if dir != "" {
		os.Setenv("YUME_DB", filepath.Join(dir, "cards.db"))
	}
	root, err := srs.DataRoot()
	if err != nil {
		return
	}
	for env, name := range map[string]string{
		"YUME_JMDICT":   "JMdict_e.gz",
		"YUME_EXAMPLES": "examples.utf.gz",
		"YUME_KANJIDIC": "kanjidic2.xml.gz",
	} {
		if os.Getenv(env) != "" {
			continue
		}
		if p := filepath.Join(root, name); fileExists(p) {
			os.Setenv(env, p)
		}
	}
	tuning, legacy := srs.TuningPathIn(root)
	if legacy && os.Getenv("YUME_SRS_TUNING") == "" {
		from, to := tuning, filepath.Join(root, "srs-tuning.json")
		if abs, err := filepath.Abs(from); err == nil {
			from = abs
		}
		if abs, err := filepath.Abs(to); err == nil {
			to = abs
		}
		log.Printf("srs tuning: reading %s from the working directory; move it to %s so it follows the data folder", from, to)
	}
	srs.SetTuningDefault(tuning)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// isLoopbackBind reports whether a -bind address only accepts connections
// from this machine. An empty address listens on every interface.
func isLoopbackBind(addr string) bool {
	if strings.EqualFold(addr, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(addr, "[]"))
	return ip != nil && ip.IsLoopback()
}

// checkOpenBind refuses open auth mode on an address other machines can
// reach, unless the operator states it is private with
// YUME_ALLOW_OPEN_NETWORK=1 (the Docker image and the Pi recipe do).
func checkOpenBind(mode, bind, allowOpenNetwork string) error {
	if mode != server.AuthModeOpen || isLoopbackBind(bind) {
		return nil
	}
	if ok, _ := strconv.ParseBool(strings.TrimSpace(allowOpenNetwork)); ok {
		return nil
	}
	return fmt.Errorf("open auth mode on -bind %q would let anyone who can reach that address act as the owner, "+
		"with every deck and the tutor tools. If the address is private (Tailscale, WireGuard), set "+
		"YUME_ALLOW_OPEN_NETWORK=1; otherwise bind 127.0.0.1 or set YUME_AUTH_MODE=required", bind)
}

func runSetup(args []string) {
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	data := fs.String("data", "", "data folder")
	fs.Parse(args)
	applyDataDir(*data)
	root, err := srs.DataRoot()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Data folder: %s\n", root)
	if err := setup.Data(root, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "\nsetup: %v\n"+
			"The dictionary files come from EDRDG (https://www.edrdg.org). Check that this\n"+
			"machine can reach it, then run setup again; files already here are kept.\n", err)
		os.Exit(1)
	}
	fmt.Printf("\nReady. Start with:\n  yume-server")
	if *data != "" {
		fmt.Printf(" -data %s", *data)
	}
	fmt.Printf("\nthen open http://127.0.0.1:%d on this machine, or bind a private address for your phone (see README).\n", DefaultPort)
}

func runVoiceVox(args []string) {
	fs := flag.NewFlagSet("voicevox", flag.ExitOnError)
	data := fs.String("data", "", "data folder")
	fs.Parse(args)
	applyDataDir(*data)
	root, err := srs.DataRoot()
	if err != nil {
		log.Fatal(err)
	}
	exe, _ := os.Executable()
	tools := filepath.Join(filepath.Dir(exe), "tools", "voicevox")
	if !fileExists(filepath.Join(tools, "batch-audio.py")) {
		tools = filepath.Join("tools", "voicevox")
	}
	if !fileExists(filepath.Join(tools, "batch-audio.py")) {
		// The engine still installs, but nothing would make the clips.
		fmt.Println("Warning: tools/voicevox/batch-audio.py was not found next to yume-server or in this folder,\n" +
			"so the audio batch tool will not be installed. Put the repository's server/tools/voicevox\n" +
			"folder at tools/voicevox beside the binary, then run this again.")
	}
	fmt.Printf("Installing VoiceVox under %s\n", filepath.Join(root, "voicevox"))
	if err := setup.VoiceVox(root, tools, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "setup":
			runSetup(os.Args[2:])
			return
		case "voicevox":
			runVoiceVox(os.Args[2:])
			return
		case "dict":
			runDict(os.Args[2:])
			return
		case "mnemonics":
			runMnemonics(os.Args[2:])
			return
		case "users":
			runUsers(os.Args[2:])
			return
		case "help", "-h", "--help":
			usage()
			return
		}
	}
	port := flag.Int("port", DefaultPort, "TCP port to bind")
	dataDir := flag.String("data", "", "Data folder (database, media, dictionaries). Default: see `yume-server help`.")
	bind := flag.String("bind", "127.0.0.1",
		"Address to bind. Default loopback (recommended). For the headless/Pi\n"+
			"deployment, bind a PRIVATE interface (your Tailscale IP, 100.x.y.z):\n"+
			"never a public one. Non-loopback binds are auto-added to the Host\n"+
			"allowlist; add extra names (MagicDNS) via YUME_ALLOWED_HOSTS.")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Usage = usage
	flag.Parse()

	commit := buildCommit()
	if *showVersion {
		fmt.Println(strings.TrimSpace(Version + " " + commit))
		return
	}
	applyDataDir(*dataDir)

	// The auth mode is settled before anything opens: a mistyped value or an
	// open deck on a reachable address stops the server instead of serving.
	authMode, err := server.ParseAuthMode(os.Getenv("YUME_AUTH_MODE"))
	if err != nil {
		log.Fatal(err)
	}
	if err := checkOpenBind(authMode, *bind, os.Getenv("YUME_ALLOW_OPEN_NETWORK")); err != nil {
		log.Fatal(err)
	}
	log.Printf("auth mode: %s", authMode)

	// YUME_SIGNUP=open|closed; unset follows the auth mode (closed when required).
	signup := strings.ToLower(strings.TrimSpace(os.Getenv("YUME_SIGNUP")))
	if signup != "" && signup != server.SignupOpen && signup != server.SignupClosed {
		log.Fatalf("YUME_SIGNUP must be open or closed, not %q", signup)
	}

	// Extra allowed Hosts: the bind address plus YUME_ALLOWED_HOSTS (comma-separated).
	var extraHosts []string
	if *bind != "127.0.0.1" && *bind != "localhost" && *bind != "::1" {
		extraHosts = append(extraHosts, *bind)
	}
	for _, h := range strings.Split(os.Getenv("YUME_ALLOWED_HOSTS"), ",") {
		if h = strings.TrimSpace(h); h != "" {
			extraHosts = append(extraHosts, h)
		}
	}

	// Initialize account stores and import an existing root deck when needed.
	root, err := srs.DataRoot()
	if err != nil {
		log.Fatalf("resolve data root: %v", err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		log.Fatalf("mkdir data root: %v", err)
	}
	dict.SetDataRoot(root)
	if abs, err := filepath.Abs(root); err == nil {
		log.Printf("data folder: %s", abs)
	}
	authStore, err := auth.Open(filepath.Join(root, "users.db"))
	if err != nil {
		log.Fatalf("open user registry: %v", err)
	}
	defer authStore.Close()
	ownerID, _, ownerCreated, err := authStore.Bootstrap()
	if err != nil {
		log.Fatalf("user bootstrap: %v", err)
	}
	srs.SetOwner(ownerID)
	lib, err := srs.OpenLibrary(filepath.Join(root, "content.db"))
	if err != nil {
		log.Fatalf("open shared library: %v", err)
	}
	defer lib.Close()
	srs.SetLibrary(lib)
	if ownerCreated {
		if moved, merr := srs.MigrateLegacyDeckTo(ownerID); merr != nil {
			log.Fatalf("migrate legacy deck: %v", merr)
		} else if moved {
			log.Printf("migrated existing single-tenant deck to owner account")
		}
	}
	// Open the owner's deck before serving: a deck this build cannot open (a
	// newer or pre-release schema) stops the server here with the reason,
	// instead of every deck call failing behind a healthy /health.
	if _, err := srs.Owner(); err != nil {
		log.Fatalf("open owner deck: %v", err)
	}

	// Optional dictionaries named in YUME_DICTS install on first start (the
	// Docker path: YUME_DICTS=ja-ja). Runs in the background so serving is
	// not held up by a download; repeats are no-ops.
	if names := strings.TrimSpace(os.Getenv("YUME_DICTS")); names != "" {
		if store, err := srs.Owner(); err == nil {
			go func() {
				for _, key := range strings.Split(names, ",") {
					if key = strings.TrimSpace(key); key == "" {
						continue
					}
					log.Printf("dictionary %s: checking", key)
					if err := installDict(store, key, log.Writer()); err != nil {
						log.Printf("dictionary %s: %v", key, err)
					}
				}
			}()
		}
	}

	// The tokenizer is a shared asset; its user dictionary lives next to the
	// owner's DB. Failure here just means plain IPADIC.
	if store, err := srs.Owner(); err == nil {
		udPath := filepath.Join(store.DataDir(), "userdict.txt")
		if err := tokenize.SeedUserDictIfMissing(udPath); err != nil {
			log.Printf("user dict seed: %v", err)
		}
		if err := tokenize.SetUserDictPath(udPath); err != nil {
			log.Printf("user dict load: %v (continuing with plain IPADIC)", err)
		}
	}

	mux := server.NewMux(server.Config{
		Version:           Version,
		Commit:            commit,
		ExtraAllowedHosts: extraHosts,
		AuthMode:          authMode,
		Signup:            signup,
		AuthStore:         authStore,
	})

	if authMode == server.AuthModeRequired {
		state := "closed (set YUME_SIGNUP=open while people join)"
		if signup == server.SignupOpen {
			state = "open (YUME_SIGNUP=open)"
		}
		log.Printf("auth mode required: sign-in needed for every deck; new accounts %s", state)
	}

	addr := fmt.Sprintf("%s:%d", *bind, *port)
	if len(extraHosts) > 0 {
		log.Printf("NOTE: serving beyond loopback (bind=%s, extra allowed hosts: %s).", *bind, strings.Join(extraHosts, ", "))
		if authMode == server.AuthModeOpen {
			log.Printf("      Auth mode is open: anyone who can reach this port, including devices shared into your tailnet, acts as the owner with full access to every deck.")
		} else {
			log.Printf("      Auth mode is required: every data request needs a signed-in account; /mcp needs the owner's.")
		}
	}
	// Load the dictionary before listening. A missing one is the likeliest
	// first-run mistake (setup skipped, or its download blocked), so it gets
	// a loud line of its own instead of surfacing as failed lookups.
	if d, err := dict.Default(); err != nil {
		log.Printf("WARNING: %v", err)
		log.Printf("WARNING: the server is running, but dictionary search fails until this is fixed.")
	} else {
		log.Printf("dictionary: %d entries", d.Size())
	}

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("Yume server %s listening on http://%s", strings.TrimSpace(Version+" "+commit), addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	<-sigs

	log.Println("shutdown requested...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Println("bye")
}
