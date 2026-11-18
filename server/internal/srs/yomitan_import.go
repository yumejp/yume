package srs

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// Server-side import of a Yomitan term dictionary (index.json plus
// term_bank_N.json files), the same format clients upload through
// /create_dict + /import_dict_chunk. Used by `yume-server dict <key>` so a
// self-hoster gets Japanese definitions with one command.

const yomitanChunk = 5000

// Wiktionary-derived banks point kanji headwords at their kana entry
// ("「なれる」を参照。", "たべるの漢字表記。"). The kanji form is what a card
// carries, so those stubs are resolved to the target's definitions at import.
var yomitanStub = regexp.MustCompile(`^「?([^「」]+?)」?(?:を参照|の漢字表記)[。.]?$`)

// ImportYomitanZip imports the term banks in a Yomitan zip under meta.Name.
// It is idempotent: a dictionary of that name already in the store is left
// alone (created = false, imported = 0).
func (s *Store) ImportYomitanZip(path string, meta CreateDictReq) (imported int, created bool, err error) {
	existing, err := s.ListDicts()
	if err != nil {
		return 0, false, err
	}
	for _, d := range existing {
		if d.Name == meta.Name {
			return 0, false, nil
		}
	}
	r, err := zip.OpenReader(path)
	if err != nil {
		return 0, false, fmt.Errorf("open %s: %w", path, err)
	}
	defer r.Close()

	type row struct {
		term, reading string
		popularity    float64
		defs          []string
	}
	var rows []row
	byTerm := map[string][]string{}
	for _, f := range r.File {
		name := f.Name
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		if name == "index.json" && meta.Revision == "" {
			var idx struct {
				Revision string `json:"revision"`
			}
			if b, err := readZipFile(f); err == nil {
				_ = json.Unmarshal(b, &idx)
				meta.Revision = idx.Revision
			}
			continue
		}
		if !strings.HasPrefix(name, "term_bank_") || !strings.HasSuffix(name, ".json") {
			continue
		}
		b, err := readZipFile(f)
		if err != nil {
			return 0, false, fmt.Errorf("read %s: %w", f.Name, err)
		}
		var bank [][]any
		if err := json.Unmarshal(b, &bank); err != nil {
			return 0, false, fmt.Errorf("parse %s: %w", f.Name, err)
		}
		for _, raw := range bank {
			if len(raw) < 6 {
				continue
			}
			term, _ := raw[0].(string)
			reading, _ := raw[1].(string)
			pop, _ := raw[4].(float64)
			defs := yomitanDefinitions(raw[5])
			if term == "" || len(defs) == 0 {
				continue
			}
			rows = append(rows, row{term, reading, pop, defs})
			if _, isStub := stubTarget(defs); !isStub {
				byTerm[term] = append(byTerm[term], defs...)
			}
		}
	}
	if len(rows) == 0 {
		return 0, false, fmt.Errorf("%s: no term_bank_*.json entries", path)
	}

	id, err := s.CreateDict(meta)
	if err != nil {
		return 0, false, err
	}
	created = true
	batch := make([]NormalizedTerm, 0, yomitanChunk)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		n, err := s.ImportTermChunk(id, batch)
		imported += n
		batch = batch[:0]
		return err
	}
	for _, rw := range rows {
		defs := rw.defs
		if target, isStub := stubTarget(defs); isStub {
			if resolved := byTerm[target]; len(resolved) > 0 {
				defs = resolved
			}
		}
		js, _ := json.Marshal(defs)
		batch = append(batch, NormalizedTerm{Term: rw.term, Reading: rw.reading, DefinitionsJSON: string(js), Popularity: rw.popularity})
		if len(batch) >= yomitanChunk {
			if err := flush(); err != nil {
				return imported, created, err
			}
		}
	}
	return imported, created, flush()
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// stubTarget reports whether a definition list is only a pointer to another
// headword, and which one.
func stubTarget(defs []string) (string, bool) {
	if len(defs) != 1 {
		return "", false
	}
	m := yomitanStub.FindStringSubmatch(strings.TrimSpace(defs[0]))
	if m == nil {
		return "", false
	}
	return m[1], true
}

// yomitanDefinitions flattens one row's definitions to plain strings, one per
// sense, dropping the dictionary's own backlink ("Wiktionary") chrome.
func yomitanDefinitions(v any) []string {
	var out []string
	for _, s := range extractText(stripBacklinks(v)) {
		s = strings.TrimSpace(s)
		if s == "" || s == "Wiktionary" {
			continue
		}
		out = append(out, s)
	}
	// A structured entry yields one string per <li>; a flat entry one per
	// item. Either way, merge fragments that are not sentences on their own.
	return mergeFragments(out)
}

// stripBacklinks removes structured-content nodes tagged as backlinks or
// link elements, which carry attribution rather than meaning.
func stripBacklinks(v any) any {
	switch t := v.(type) {
	case []any:
		out := make([]any, 0, len(t))
		for _, e := range t {
			if e = stripBacklinks(e); e != nil {
				out = append(out, e)
			}
		}
		return out
	case map[string]any:
		if data, ok := t["data"].(map[string]any); ok {
			if c, _ := data["content"].(string); c == "backlink" {
				return nil
			}
		}
		if tag, _ := t["tag"].(string); tag == "a" {
			return nil
		}
		cp := map[string]any{}
		for k, val := range t {
			cp[k] = val
		}
		if c, ok := t["content"]; ok {
			cp["content"] = stripBacklinks(c)
		}
		return cp
	default:
		return v
	}
}

// mergeFragments joins pieces that a structured entry split mid-sentence
// (inline spans) while keeping each sense on its own string. A fragment
// that does not end a sentence is glued onto the next one.
func mergeFragments(parts []string) []string {
	var out []string
	var cur strings.Builder
	for _, p := range parts {
		cur.WriteString(p)
		if strings.HasSuffix(p, "。") || strings.HasSuffix(p, ".") || strings.HasSuffix(p, "）") {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
