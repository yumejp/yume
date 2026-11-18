package setup

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func gzipped(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := io.WriteString(zw, s); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func noRetryWait(t *testing.T) {
	t.Helper()
	old := retryDelays
	retryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { retryDelays = old })
}

// serve answers every request with handle and counts the requests.
func serve(t *testing.T, handle http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handle(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func testFile(url string) File {
	return File{Name: "dict.xml.gz", URL: url, What: "test", Starts: "<?xml", MinSize: 16}
}

func TestFetchAllKeepsAValidFile(t *testing.T) {
	noRetryWait(t)
	body := gzipped(t, `<?xml version="1.0"?><JMdict>`+strings.Repeat("<entry/>", 50)+`</JMdict>`)
	srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write(body) })
	root := t.TempDir()
	if err := fetchAll(root, []File{testFile(srv.URL)}, io.Discard); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "dict.xml.gz"))
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("file not stored as served: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "dict.xml.gz.part")); !os.IsNotExist(err) {
		t.Fatalf(".part left behind: %v", err)
	}
}

func TestFetchAllRejectsBadDownloads(t *testing.T) {
	noRetryWait(t)
	valid := gzipped(t, `<?xml version="1.0"?>`+strings.Repeat("<entry/>", 50))
	cases := []struct {
		name     string
		handle   http.HandlerFunc
		want     string
		attempts int32
	}{
		{"not found", func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}, "404", 1},
		{"server error retried", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "down", http.StatusBadGateway)
		}, "502", 3},
		{"html page", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("<html><body>Please sign in to the Wi-Fi</body></html>"))
		}, "not a gzip file", 3},
		{"wrong content", func(w http.ResponseWriter, r *http.Request) {
			w.Write(gzipped(t, "<html>"+strings.Repeat("x", 100)+"</html>"))
		}, "does not start with", 3},
		{"too small", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("x"))
		}, "expected at least", 3},
		{"truncated gzip", func(w http.ResponseWriter, r *http.Request) {
			w.Write(valid[:len(valid)-8])
		}, "damaged gzip", 3},
		{"cut short", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100000")
			w.Write(valid)
		}, "", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, hits := serve(t, tc.handle)
			root := t.TempDir()
			err := fetchAll(root, []File{testFile(srv.URL)}, io.Discard)
			if err == nil {
				t.Fatal("bad download accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
			if got := hits.Load(); got != tc.attempts {
				t.Fatalf("attempts = %d, want %d", got, tc.attempts)
			}
			entries, _ := os.ReadDir(root)
			if len(entries) != 0 {
				t.Fatalf("rejected download left files: %v", entries)
			}
		})
	}
}

func TestFetchAllRetriesThenSucceeds(t *testing.T) {
	noRetryWait(t)
	body := gzipped(t, `<?xml version="1.0"?>`+strings.Repeat("<entry/>", 50))
	var n atomic.Int32
	srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		w.Write(body)
	})
	var out bytes.Buffer
	if err := fetchAll(t.TempDir(), []File{testFile(srv.URL)}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "retrying") {
		t.Fatalf("retry not reported: %q", out.String())
	}
}

func TestDownloadSizeLimits(t *testing.T) {
	noRetryWait(t)
	oldDL, oldUnpacked := maxDownload, maxUnpacked
	t.Cleanup(func() { maxDownload, maxUnpacked = oldDL, oldUnpacked })

	big := gzipped(t, `<?xml version="1.0"?>`+strings.Repeat("<entry/>", 5000))

	t.Run("declared too large", func(t *testing.T) {
		maxDownload, maxUnpacked = 64, oldUnpacked
		srv, hits := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write(big) })
		err := fetchAll(t.TempDir(), []File{testFile(srv.URL)}, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "limit") || hits.Load() != 1 {
			t.Fatalf("err = %v after %d attempts", err, hits.Load())
		}
	})
	t.Run("streamed too large", func(t *testing.T) {
		maxDownload, maxUnpacked = 64, oldUnpacked
		srv, hits := serve(t, func(w http.ResponseWriter, r *http.Request) {
			// No Content-Length: chunked, so only the copy can notice.
			w.(http.Flusher).Flush()
			w.Write(big)
		})
		err := fetchAll(t.TempDir(), []File{testFile(srv.URL)}, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "limit") || hits.Load() != 1 {
			t.Fatalf("err = %v after %d attempts", err, hits.Load())
		}
	})
	t.Run("unpacks too large", func(t *testing.T) {
		maxDownload, maxUnpacked = oldDL, 1000
		srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write(big) })
		err := fetchAll(t.TempDir(), []File{testFile(srv.URL)}, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "unpacks to more than") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestFetchAllKeepsExistingFiles(t *testing.T) {
	srv, hits := serve(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "dict.xml.gz"), []byte("kept"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fetchAll(root, []File{testFile(srv.URL)}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 0 {
		t.Fatal("an existing file was downloaded again")
	}
}

func TestSHA256Check(t *testing.T) {
	noRetryWait(t)
	payload := []byte("#!/bin/sh\necho downloader\n")
	sum := sha256.Sum256(payload)
	srv, hits := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write(payload) })
	dest := filepath.Join(t.TempDir(), "download")

	if _, err := download(srv.URL, dest, io.Discard, sha256Check(strings.Repeat("0", 64))); err == nil ||
		!strings.Contains(err.Error(), "checksum mismatch") || hits.Load() != 1 {
		t.Fatalf("tampered file: err = %v after %d attempts", err, hits.Load())
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("tampered file was kept")
	}
	if _, err := download(srv.URL, dest, io.Discard, sha256Check(hex.EncodeToString(sum[:]))); err != nil {
		t.Fatal(err)
	}
}

// The shipped URLs must stay on the host whose certificate matches.
func TestRequiredURLsUseHTTPS(t *testing.T) {
	for _, f := range Required {
		if !strings.HasPrefix(f.URL, "https://www.edrdg.org/") {
			t.Errorf("%s: %s is not https://www.edrdg.org", f.Name, f.URL)
		}
	}
	for _, d := range Dictionaries {
		if !strings.HasPrefix(d.URL, "https://") {
			t.Errorf("%s: %s is not https", d.Key, d.URL)
		}
	}
}

// The command VoiceVox prints must name the venv's python on every OS:
// Windows venvs keep it in Scripts, as python.exe.
func TestVenvTool(t *testing.T) {
	venv := filepath.Join("data", "voicevox", "venv")
	want := filepath.Join(venv, "bin", "python")
	if runtime.GOOS == "windows" {
		want = filepath.Join(venv, "Scripts", "python.exe")
	}
	if got := venvTool(venv, "python"); got != want {
		t.Errorf("venvTool(python) = %q, want %q", got, want)
	}
}
