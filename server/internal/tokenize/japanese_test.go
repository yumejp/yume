package tokenize

import (
	"os"
	"path/filepath"
	"testing"
)

// A kanji user-dict word must carry its yomi as the token reading — user
// entries exist precisely to correct IPADIC's preferred reading (蒸す is
// ふかす in IPADIC; the deck teaches むす), and without the reading the
// furigana surfaces render nothing over the corrected word.
func TestUserDictTokenCarriesReading(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "userdict.txt")
	if err := os.WriteFile(path, []byte("蒸す,蒸す,ムス,カスタム語\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetUserDictPath(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = SetUserDictPath("") })

	toks, err := Japanese("野菜は蒸すと甘くなる。")
	if err != nil {
		t.Fatal(err)
	}
	var got *Token
	for i := range toks {
		if toks[i].Surface == "蒸す" {
			got = &toks[i]
		}
	}
	if got == nil {
		t.Fatalf("no 蒸す token in %+v", toks)
	}
	if got.POS != "カスタム語" {
		t.Errorf("POS = %q, want カスタム語 (user entry should preempt IPADIC)", got.POS)
	}
	if got.Reading != "ムス" {
		t.Errorf("Reading = %q, want ムス", got.Reading)
	}
}
