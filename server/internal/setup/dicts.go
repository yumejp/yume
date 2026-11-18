package setup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// Optional dictionaries install like VoiceVox does: `yume-server dict ja-ja`
// (or YUME_DICTS=ja-ja when serving) downloads a Yomitan-format zip into
// <root>/dicts/ and imports it into the owner's deck. Each entry is one
// dictionary the server knows how to fetch.
type Dictionary struct {
	Key         string // what the user types
	Title       string // dict name as stored and listed
	URL         string // the Yomitan zip
	Author      string
	Homepage    string
	Description string
	GlossLang   string // language of the definitions
}

// Dictionaries the server can install by key.
var Dictionaries = map[string]Dictionary{
	"ja-ja": {
		Key:   "ja-ja",
		Title: "Wiktionary 日本語 (ja-ja)",
		// Pinned: the newest wiktionary-to-yomitan release ships only index
		// files for some pairs; this release carries the full zip.
		URL:         "https://github.com/yomidevs/wiktionary-to-yomitan/releases/download/v2025-04-08-10-54-36-00-00/kty-ja-ja.zip",
		Author:      "Kaikki-to-Yomitan contributors; Japanese Wiktionary (CC BY-SA 4.0)",
		Homepage:    "https://github.com/yomidevs/kaikki-to-yomitan",
		Description: "Japanese definitions from the Japanese Wiktionary. Powers the app's Japanese definitions setting.",
		GlossLang:   "ja",
	},
}

// DictionaryKeys lists the installable keys, sorted.
func DictionaryKeys() []string {
	keys := make([]string, 0, len(Dictionaries))
	for k := range Dictionaries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// DictionaryZip returns the local path of a dictionary's zip under root,
// downloading it first when missing.
func DictionaryZip(root, key string, out io.Writer) (string, error) {
	d, ok := Dictionaries[key]
	if !ok {
		return "", fmt.Errorf("unknown dictionary %q; available: %v", key, DictionaryKeys())
	}
	dir := filepath.Join(root, "dicts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, filepath.Base(d.URL))
	if st, err := os.Stat(dest); err == nil && st.Size() > 0 {
		fmt.Fprintf(out, "  %-18s already here (%s)\n", filepath.Base(dest), human(st.Size()))
		return dest, nil
	}
	fmt.Fprintf(out, "  %-18s %s… ", filepath.Base(dest), d.Title)
	n, err := download(d.URL, dest, out, zipCheck)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", d.Title, err)
	}
	fmt.Fprintf(out, " %s\n", human(n))
	return dest, nil
}
