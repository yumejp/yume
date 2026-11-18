// Package webapp serves the phone app (an Expo web export) from the server
// itself, so one binary is the whole install. The build lands in dist/ via
// scripts/build-web.sh; an empty dist/ serves a short notice instead.
package webapp

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

//go:embed all:dist
var dist embed.FS

// IsAsset reports whether urlPath names a real bundled file ("/" is
// index.html). It never matches API routes or single-page fallbacks, so
// required auth mode can serve the app shell and nothing else unauthenticated.
func IsAsset(urlPath string) bool {
	p := strings.TrimPrefix(path.Clean("/"+urlPath), "/")
	if p == "" {
		p = "index.html"
	}
	if strings.HasPrefix(p, ".") {
		return false
	}
	info, err := fs.Stat(dist, "dist/"+p)
	return err == nil && !info.IsDir()
}

// Handler serves dist/ with a single-page fallback: unknown paths without a
// file extension get index.html so client-side routes deep-link.
func Handler() http.Handler {
	sub, _ := fs.Sub(dist, "dist")
	return handler(sub)
}

func handler(sub fs.FS) http.Handler {
	files := http.FS(sub)
	fileServer := http.FileServer(files)
	worker := sync.OnceValue(func() []byte { return serviceWorker(sub) })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		if _, err := fs.Stat(sub, "index.html"); err != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			http.Error(w, "Yume server is running. The phone app was not bundled into this build; see README.", http.StatusNotFound)
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if p == "sw.js" {
			if js := worker(); js != nil {
				// Root scope, and revalidated on every check so a server
				// upgrade replaces the cached app shell.
				w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("Service-Worker-Allowed", "/")
				http.ServeContent(w, r, "sw.js", time.Time{}, bytes.NewReader(js))
				return
			}
		}
		if _, err := fs.Stat(sub, p); err != nil && path.Ext(p) == "" {
			r.URL.Path = "/"
		}
		// Hashed bundles may cache forever; index.html must not.
		if strings.Contains(p, "/static/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})
}

// shellMarker is the placeholder in mobile/public/sw.js that the build's
// app-shell list replaces.
var shellMarker = []byte("/* yume:shell */ null")

// serviceWorker returns the export's sw.js with its app-shell list filled in:
// every bundled file the app needs to start (index.html as "/"), and a version
// hashed from the whole export so each build gets a fresh cache. Nil when the
// export has no sw.js with the marker; it is then served as a plain file.
func serviceWorker(sub fs.FS) []byte {
	src, err := fs.ReadFile(sub, "sw.js")
	if err != nil || !bytes.Contains(src, shellMarker) {
		return nil
	}
	h := sha256.New()
	shell := []string{"/"}
	err = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(sub, p)
		if err != nil {
			return err
		}
		h.Write([]byte(p))
		h.Write([]byte{0})
		h.Write(data)
		if shellFile(p) {
			shell = append(shell, "/"+p)
		}
		return nil
	})
	if err != nil {
		return nil
	}
	list, _ := json.Marshal(struct {
		Version string   `json:"version"`
		Files   []string `json:"files"`
	}{hex.EncodeToString(h.Sum(nil))[:16], shell})
	return bytes.Replace(src, shellMarker, list, 1)
}

// shellFile reports whether an exported file belongs in the offline app
// shell: the bundle, its fonts and images, the manifest and icons. Hidden
// files, source maps, export metadata, the worker itself and index.html
// (cached as "/") stay out.
func shellFile(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if strings.HasPrefix(seg, ".") {
			return false
		}
	}
	switch p {
	case "index.html", "sw.js", "metadata.json":
		return false
	}
	return !strings.HasSuffix(p, ".map")
}
