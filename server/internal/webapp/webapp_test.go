package webapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

const workerSrc = "const SHELL = /* yume:shell */ null;\n"

func export(bundle string) fstest.MapFS {
	return fstest.MapFS{
		".gitkeep":      {},
		"index.html":    {Data: []byte(`<script src="/_expo/static/js/web/index-` + bundle + `.js"></script>`)},
		"sw.js":         {Data: []byte(workerSrc)},
		"manifest.json": {Data: []byte(`{}`)},
		"metadata.json": {Data: []byte(`{}`)},
		"favicon.ico":   {Data: []byte("ico")},
		"_expo/static/js/web/index-" + bundle + ".js":     {Data: []byte("app " + bundle)},
		"_expo/static/js/web/index-" + bundle + ".js.map": {Data: []byte("map")},
		"assets/assets/moon.abc.png":                      {Data: []byte("png")},
	}
}

type shell struct {
	Version string   `json:"version"`
	Files   []string `json:"files"`
}

func getWorker(t *testing.T, h http.Handler) (*httptest.ResponseRecorder, shell) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sw.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /sw.js = %d", rec.Code)
	}
	m := regexp.MustCompile(`const SHELL = (\{.*\});`).FindStringSubmatch(rec.Body.String())
	if m == nil {
		t.Fatalf("shell list not filled in: %q", rec.Body.String())
	}
	var s shell
	if err := json.Unmarshal([]byte(m[1]), &s); err != nil {
		t.Fatal(err)
	}
	return rec, s
}

func TestServiceWorkerHeadersAndShell(t *testing.T) {
	rec, s := getWorker(t, handler(export("aaa")))
	for k, want := range map[string]string{
		"Cache-Control":          "no-cache",
		"Service-Worker-Allowed": "/",
		"Content-Type":           "text/javascript; charset=utf-8",
	} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	want := []string{"/", "/_expo/static/js/web/index-aaa.js", "/assets/assets/moon.abc.png", "/favicon.ico", "/manifest.json"}
	if strings.Join(s.Files, " ") != strings.Join(want, " ") {
		t.Errorf("files = %v, want %v", s.Files, want)
	}
	if len(s.Version) != 16 {
		t.Errorf("version = %q", s.Version)
	}
}

func TestServiceWorkerVersionFollowsTheBuild(t *testing.T) {
	_, a := getWorker(t, handler(export("aaa")))
	_, again := getWorker(t, handler(export("aaa")))
	_, b := getWorker(t, handler(export("bbb")))
	if a.Version != again.Version {
		t.Errorf("same export gave versions %s and %s", a.Version, again.Version)
	}
	if a.Version == b.Version {
		t.Errorf("a new bundle kept version %s", a.Version)
	}
}

func TestServiceWorkerWithoutMarkerIsAPlainFile(t *testing.T) {
	fsys := export("aaa")
	fsys["sw.js"] = &fstest.MapFile{Data: []byte("// someone else's worker\n")}
	rec := httptest.NewRecorder()
	handler(fsys).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sw.js", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "// someone else's worker\n" {
		t.Fatalf("GET /sw.js = %d %q", rec.Code, rec.Body.String())
	}

	delete(fsys, "sw.js")
	rec = httptest.NewRecorder()
	handler(fsys).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sw.js", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /sw.js without a worker = %d, want 404", rec.Code)
	}
}

func TestCachingHeaders(t *testing.T) {
	h := handler(export("aaa"))
	for path, want := range map[string]string{
		"/":                                 "no-cache",
		"/review":                           "no-cache",
		"/manifest.json":                    "no-cache",
		"/_expo/static/js/web/index-aaa.js": "public, max-age=31536000, immutable",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != want {
			t.Errorf("GET %s = %d, Cache-Control %q, want %q", path, rec.Code, rec.Header().Get("Cache-Control"), want)
		}
	}
}

// Hidden files, API routes, traversal and directories are never bundled
// assets, whether or not the app was built into dist/.
func TestIsAssetRejectsNonFiles(t *testing.T) {
	for _, p := range []string{"/.gitkeep", "/deck_stats", "/../webapp.go", "/_expo/", "/_expo/static"} {
		if IsAsset(p) {
			t.Errorf("IsAsset(%q) = true", p)
		}
	}
}
