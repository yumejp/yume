package srs

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestYomitanZip(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kty-test.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	add := func(name string, v any) {
		b, _ := json.Marshal(v)
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	add("index.json", map[string]any{"format": 3, "revision": "2025.04.08", "title": "kty-test"})
	backlink := map[string]any{"tag": "div", "data": map[string]any{"content": "backlink"},
		"content": []any{map[string]any{"tag": "a", "href": "https://ja.wiktionary.org/wiki/x", "content": "Wiktionary"}}}
	structured := func(glosses ...string) []any {
		var lis []any
		for _, g := range glosses {
			lis = append(lis, map[string]any{"tag": "li", "content": []any{map[string]any{"tag": "div", "content": []any{g}}}})
		}
		return []any{map[string]any{"type": "structured-content", "content": []any{
			map[string]any{"tag": "ol", "data": map[string]any{"content": "glosses"}, "content": lis},
			backlink,
		}}}
	}
	add("term_bank_1.json", [][]any{
		{"なれる", "", "", "", 5, structured("繰り返し経験して、平気になる。", "親しくなる。"), 1, ""},
		{"慣れる", "", "", "", 3, structured("「なれる」を参照。"), 2, ""},
		{"納期", "", "", "", 4, structured("金銭や物品を相手先に納入する時期。"), 3, ""},
		{"空白", "", "", "", 1, []any{}, 4, ""},
	})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return path
}

func TestImportYomitanZipResolvesStubsAndIsIdempotent(t *testing.T) {
	s := openTestStore(t)
	path := writeTestYomitanZip(t)
	meta := CreateDictReq{Name: "Wiktionary test"}
	n, created, err := s.ImportYomitanZip(path, meta)
	if err != nil {
		t.Fatal(err)
	}
	if !created || n != 3 {
		t.Fatalf("created=%v imported=%d, want true 3 (the empty row is skipped)", created, n)
	}
	got, err := s.LookupImported([]string{"慣れる", "納期"})
	if err != nil {
		t.Fatal(err)
	}
	def := strings.Join(got["慣れる"][0].Senses[0].Glosses, "|")
	if def != "繰り返し経験して、平気になる。|親しくなる。" {
		t.Errorf("stub not resolved: %q", def)
	}
	if strings.Contains(strings.Join(got["納期"][0].Senses[0].Glosses, ""), "Wiktionary") {
		t.Errorf("backlink leaked into glosses: %+v", got["納期"][0].Senses[0].Glosses)
	}
	dicts, _ := s.ListDicts()
	if len(dicts) != 1 || dicts[0].Revision != "2025.04.08" {
		t.Errorf("dicts = %+v", dicts)
	}
	n, created, err = s.ImportYomitanZip(path, meta)
	if err != nil || created || n != 0 {
		t.Errorf("second import should be a no-op: n=%d created=%v err=%v", n, created, err)
	}
}
