// Package setup fetches the data files the server loads at runtime and
// installs the optional VoiceVox engine. Everything lands under the data
// root, so an install is one binary plus one folder.
package setup

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// File is one downloadable data file.
type File struct {
	Name string
	URL  string
	What string
	// Starts is how the unpacked file begins. A download that does not
	// (an HTML error page, a captive portal) is rejected.
	Starts string
	// MinSize is the smallest plausible download; each file is well over
	// twice its minimum today.
	MinSize int64
}

// Required data files. All three are EDRDG releases (CC BY-SA 4.0; see
// LICENSES.md); the server loads them lazily from the data root. They come
// from www.edrdg.org over HTTPS: ftp.edrdg.org serves the same files but
// its certificate names only www, so https://ftp.edrdg.org fails TLS.
var Required = []File{
	{"JMdict_e.gz", "https://www.edrdg.org/pub/Nihongo/JMdict_e.gz", "dictionary", "<?xml", 4 << 20},
	{"examples.utf.gz", "https://www.edrdg.org/pub/Nihongo/examples.utf.gz", "example sentences", "A: ", 4 << 20},
	{"kanjidic2.xml.gz", "https://www.edrdg.org/kanjidic/kanjidic2.xml.gz", "kanji dictionary", "<?xml", 512 << 10},
}

// Download limits. JMdict_e.gz is about 10 MB and unpacks to about 60 MB;
// the caps only stop a runaway or hostile response from filling the disk.
// Variables so tests can lower them.
var (
	maxDownload int64 = 200 << 20
	maxUnpacked int64 = 1 << 30
)

// retryDelays are the waits between attempts at one download.
var retryDelays = []time.Duration{2 * time.Second, 10 * time.Second}

var httpClient = &http.Client{Timeout: 30 * time.Minute}

// Data downloads any missing required file into root. Existing files are
// kept; delete one to refresh it.
func Data(root string, out io.Writer) error {
	return fetchAll(root, Required, out)
}

func fetchAll(root string, files []File, out io.Writer) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	for _, f := range files {
		dest := filepath.Join(root, f.Name)
		if st, err := os.Stat(dest); err == nil && st.Size() > 0 {
			fmt.Fprintf(out, "  %-18s already here (%s)\n", f.Name, human(st.Size()))
			continue
		}
		fmt.Fprintf(out, "  %-18s %s…", f.Name, f.What)
		n, err := download(f.URL, dest, out, f.check)
		if err != nil {
			fmt.Fprintln(out)
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		fmt.Fprintf(out, " %s\n", human(n))
	}
	return nil
}

// check accepts a downloaded file only if it is at least MinSize, is one
// complete gzip stream within maxUnpacked, and unpacks to text that begins
// with Starts.
func (f File) check(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.Size() < f.MinSize {
		return fmt.Errorf("only %s arrived, expected at least %s", human(st.Size()), human(f.MinSize))
	}
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	gz, err := gzip.NewReader(fh)
	if err != nil {
		return fmt.Errorf("not a gzip file: %w", err)
	}
	defer gz.Close()
	head := make([]byte, len(f.Starts))
	if _, err := io.ReadFull(gz, head); err != nil || !bytes.Equal(head, []byte(f.Starts)) {
		return fmt.Errorf("unexpected content: the file does not start with %q", f.Starts)
	}
	n, err := io.Copy(io.Discard, io.LimitReader(gz, maxUnpacked+1))
	if err != nil {
		return fmt.Errorf("damaged gzip file: %w", err)
	}
	if int64(len(head))+n > maxUnpacked {
		return fmt.Errorf("unpacks to more than %s", human(maxUnpacked))
	}
	return nil
}

// permanent marks a download failure that another attempt would repeat.
type permanent struct{ error }

func (p permanent) Unwrap() error { return p.error }

// download fetches url into dest through dest.part, renaming it into place
// only after check (when set) accepts it. Network errors, server errors and
// damaged files are retried after each of retryDelays.
func download(url, dest string, out io.Writer, check func(path string) error) (int64, error) {
	for attempt := 0; ; attempt++ {
		n, err := fetch(url, dest, out, check)
		if err == nil {
			return n, nil
		}
		var p permanent
		if errors.As(err, &p) || attempt >= len(retryDelays) {
			return 0, err
		}
		fmt.Fprintf(out, " failed (%v), retrying in %s…", err, retryDelays[attempt])
		time.Sleep(retryDelays[attempt])
	}
}

func fetch(url, dest string, out io.Writer, check func(path string) error) (int64, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("HTTP %s", resp.Status)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return 0, permanent{err}
		}
		return 0, err
	}
	if resp.ContentLength > maxDownload {
		return 0, permanent{fmt.Errorf("the server offers %s, over the %s limit", human(resp.ContentLength), human(maxDownload))}
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, &ticker{r: io.LimitReader(resp.Body, maxDownload+1), out: out})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	switch {
	case err != nil:
	case n > maxDownload:
		err = permanent{fmt.Errorf("download passed the %s limit", human(maxDownload))}
	case resp.ContentLength >= 0 && n != resp.ContentLength:
		err = fmt.Errorf("download cut short: %s of %s", human(n), human(resp.ContentLength))
	case check != nil:
		err = check(tmp)
	}
	if err != nil {
		os.Remove(tmp)
		return 0, err
	}
	return n, os.Rename(tmp, dest)
}

// sha256Check accepts a file only if its SHA-256 is want (hex).
func sha256Check(want string) func(path string) error {
	return func(path string) error {
		fh, err := os.Open(path)
		if err != nil {
			return err
		}
		defer fh.Close()
		h := sha256.New()
		if _, err := io.Copy(h, fh); err != nil {
			return err
		}
		if got := hex.EncodeToString(h.Sum(nil)); got != want {
			return permanent{fmt.Errorf("checksum mismatch: got sha256 %s, want %s", got, want)}
		}
		return nil
	}
}

// zipCheck accepts a file only if it opens as a zip archive.
func zipCheck(path string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("not a zip file: %w", err)
	}
	return zr.Close()
}

// ticker prints a dot per ~2 MB so a slow link shows life.
type ticker struct {
	r    io.Reader
	out  io.Writer
	seen int64
}

func (t *ticker) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	before := t.seen / (2 << 20)
	t.seen += int64(n)
	if t.seen/(2<<20) > before {
		fmt.Fprint(t.out, ".")
	}
	return n, err
}

func human(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d B", n)
}

// --- VoiceVox (optional) ------------------------------------------------

const voicevoxCoreVersion = "0.16.4"

// pinned is one release asset and its SHA-256, as GitHub lists it for the
// voicevoxCoreVersion release. Bumping the version means updating these.
type pinned struct{ name, sha256 string }

// voicevoxDownloader is VOICEVOX's own fetcher for the engine, models and
// the Open JTalk dictionary; it exists per platform. It is executed, so its
// hash is checked before it is kept.
func voicevoxDownloader() (url, sum string, err error) {
	key := runtime.GOOS + "/" + runtime.GOARCH
	assets := map[string]pinned{
		"linux/amd64":   {"download-linux-x64", "9d53fe39b3a6de7ebd10b779533ad8bd0ee09bc23ce8a9337eef23d281d28a1b"},
		"linux/arm64":   {"download-linux-arm64", "09dc3a69f5b7a902d62d491a584fd2b34cd177dcd03a316a789c12bf46595ace"},
		"darwin/amd64":  {"download-osx-x64", "4235cc4e6ecbe8a5d3a0326403115f8317ab46d429c8336f981b8793add4aa24"},
		"darwin/arm64":  {"download-osx-arm64", "1ec1db42766ba27bfb9a1c0d6186b1bbce40b1eae1d6f7069ac9a436bdc50786"},
		"windows/amd64": {"download-windows-x64.exe", "8293658d3af5a8cf753b292747110e46ca4351366dd9705172444160dbdfb3b9"},
	}
	a, ok := assets[key]
	if !ok {
		return "", "", fmt.Errorf("no VOICEVOX build for %s", key)
	}
	return "https://github.com/VOICEVOX/voicevox_core/releases/download/" + voicevoxCoreVersion + "/" + a.name, a.sha256, nil
}

// voicevoxWheel returns the wheel URL with its hash as a #sha256= fragment,
// which pip checks before installing.
func voicevoxWheel() (string, error) {
	key := runtime.GOOS + "/" + runtime.GOARCH
	tags := map[string]pinned{
		"linux/amd64":   {"manylinux_2_34_x86_64", "a9fdcffa2de4e365c1e0eda20d723ac87f66770d41c21299ca0e15b8267bd09d"},
		"linux/arm64":   {"manylinux_2_34_aarch64", "1c84bf510488e278bbee6f18f7a5e1d6a62bf122ec33e2b6ba586fd76213a165"},
		"darwin/amd64":  {"macosx_10_12_x86_64", "8cac0c052fb24ee030608e09a7e9ba0a0a07afd9b3e8b8d62dc54b600999a330"},
		"darwin/arm64":  {"macosx_11_0_arm64", "fa369bdd35214c166fcc6cf93ac5f6d3e88f568e72a7245e97b93fdbac7a5331"},
		"windows/amd64": {"win_amd64", "16f0f59e6175220162b709b5a24de9c3766e14b54ab71338c2b9484f1a8f1794"},
	}
	t, ok := tags[key]
	if !ok {
		return "", fmt.Errorf("no VOICEVOX wheel for %s", key)
	}
	return fmt.Sprintf("https://github.com/VOICEVOX/voicevox_core/releases/download/%s/voicevox_core-%s-cp310-abi3-%s.whl#sha256=%s", voicevoxCoreVersion, voicevoxCoreVersion, t.name, t.sha256), nil
}

// VoiceVox installs the engine under <root>/voicevox: the official
// downloader fetches ONNX Runtime, the voice models and the Open JTalk
// dictionary into voicevox/core; a Python venv gets the voicevox_core wheel;
// the batch script from tools/voicevox is copied alongside. Steps that need
// a tool the machine lacks are printed instead of run.
func VoiceVox(root string, toolsDir string, out io.Writer) error {
	dir := filepath.Join(root, "voicevox")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	dlURL, dlSum, err := voicevoxDownloader()
	if err != nil {
		return err
	}
	dl := filepath.Join(dir, "download")
	if runtime.GOOS == "windows" {
		dl += ".exe"
	}
	if _, err := os.Stat(dl); err != nil {
		fmt.Fprint(out, "  fetching the VOICEVOX downloader…")
		if _, err := download(dlURL, dl, out, sha256Check(dlSum)); err != nil {
			return err
		}
		os.Chmod(dl, 0o755)
		fmt.Fprintln(out)
	}
	core := filepath.Join(dir, "core")
	if _, err := os.Stat(filepath.Join(core, "models")); err != nil {
		fmt.Fprintln(out, "  downloading engine, models and dictionary (this is large; VOICEVOX will ask you to accept its terms):")
		cmd := exec.Command(dl, "--only", "onnxruntime", "models", "dict", "-o", core)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, out, out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("voicevox download: %w", err)
		}
	} else {
		fmt.Fprintln(out, "  engine already here")
	}
	// Batch script.
	for _, name := range []string{"batch-audio.py", "README.md"} {
		src := filepath.Join(toolsDir, name)
		b, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		_ = os.WriteFile(filepath.Join(dir, name), b, 0o644)
	}
	// Python side.
	wheel, err := voicevoxWheel()
	if err != nil {
		return err
	}
	venv := filepath.Join(dir, "venv")
	py, perr := exec.LookPath("python3")
	if perr != nil {
		py, perr = exec.LookPath("python")
	}
	pip := venvTool(venv, "pip")
	if perr == nil {
		if _, err := os.Stat(pip); err != nil {
			fmt.Fprintln(out, "  creating a Python environment…")
			if err := run(out, py, "-m", "venv", venv); err != nil {
				perr = err
			}
		}
	}
	if perr == nil {
		fmt.Fprintln(out, "  installing voicevox_core…")
		perr = run(out, pip, "install", "--quiet", wheel)
	}
	if perr != nil {
		python := "python3"
		if runtime.GOOS == "windows" {
			python = "python"
		}
		fmt.Fprintf(out, "\n  Python step skipped (%v). Run by hand:\n    %s -m venv %s\n    %s install %s\n", perr, python, venv, pip, wheel)
	}
	fmt.Fprintf(out, "\nVoiceVox is in %s.\nGenerate audio for cards that lack it:\n  %s %s\n", dir, venvTool(venv, "python"), filepath.Join(dir, "batch-audio.py"))
	return nil
}

// venvTool is the path of a program inside a Python venv: bin/<name> on
// macOS and Linux, Scripts\<name>.exe on Windows.
func venvTool(venv, name string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venv, "Scripts", name+".exe")
	}
	return filepath.Join(venv, "bin", name)
}

func run(out io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = out, out
	return cmd.Run()
}
