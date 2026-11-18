package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yumejp/yume/server/internal/dict"
	"github.com/yumejp/yume/server/internal/language"
)

// TestMain points the runtime-loaded JMdict at the repo's data file BEFORE
// any test triggers dict.Default()'s sync.Once (middleware_test's /health
// call would otherwise cache a load failure). Dict-dependent tests skip when
// the data file isn't present (CI without the ~10MB dict).
func TestMain(m *testing.M) {
	if p, err := filepath.Abs(filepath.Join("..", "..", "data", "JMdict_e.gz")); err == nil {
		if _, statErr := os.Stat(p); statErr == nil {
			os.Setenv("YUME_JMDICT", p)
		}
	}
	os.Exit(m.Run())
}

// The /lookup deconjugation fallback: conjugated raw strings resolve through
// the tokenizer's lemmas; auxiliary lemmas are filtered by lang.IsContent;
// exact hits are never disturbed.
func TestDeconjugateMisses(t *testing.T) {
	lang, ok := language.For("ja")
	if !ok || lang.Dict == nil || lang.Tokenize == nil {
		t.Fatal("ja language capabilities missing")
	}
	d, err := lang.Dict()
	if err != nil {
		t.Skipf("JMdict not available: %v", err)
	}

	results := d.Lookup([]string{"ぶつけた", "食べさせられた", "開く", "xyzzyplugh"})
	deconjugateMisses(lang, d, results)

	byQ := map[string]dict.LookupResult{}
	for _, r := range results {
		byQ[r.Query] = r
	}

	// Plain past-tense resolves to the base form.
	if len(byQ["ぶつけた"].Entries) == 0 {
		t.Error("ぶつけた: want entries via deconjugation, got none")
	}

	// Causative-passive chain resolves to the stem verb only — the aux lemmas
	// (させる/られる/た all have JMdict entries) must be filtered out, so every
	// returned entry is a 食べる entry.
	ents := byQ["食べさせられた"].Entries
	if len(ents) == 0 {
		t.Error("食べさせられた: want entries via deconjugation, got none")
	}
	for _, e := range ents {
		related := false
		for _, form := range append(append([]string{}, e.Kanji...), e.Readings...) {
			if form == "食べる" || form == "たべる" {
				related = true
			}
		}
		if !related {
			t.Errorf("食べさせられた: aux-lemma pollution: entry id=%d kanji=%v readings=%v", e.ID, e.Kanji, e.Readings)
		}
	}

	// Exact hits bypass the fallback entirely.
	direct := d.Lookup([]string{"開く"})
	if len(byQ["開く"].Entries) != len(direct[0].Entries) {
		t.Errorf("開く: exact hit disturbed: %d entries vs direct %d", len(byQ["開く"].Entries), len(direct[0].Entries))
	}

	// Garbage stays empty — no panic, no junk entries.
	if len(byQ["xyzzyplugh"].Entries) != 0 {
		t.Errorf("xyzzyplugh: want 0 entries, got %d", len(byQ["xyzzyplugh"].Entries))
	}
}
