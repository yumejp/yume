package dict

import (
	"strings"
	"testing"
)

// A miniature JMdict with the same structural quirks as the real file:
// DOCTYPE internal subset, &entity; POS references, multi-kanji /
// multi-reading entries, and a katakana-reading loanword.
const sampleXML = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE JMdict [
<!ENTITY n "noun (common) (futsuumeishi)">
<!ENTITY adj-na "adjectival nouns or quasi-adjectives (keiyodoshi)">
]>
<JMdict>
<entry>
<ent_seq>1244210</ent_seq>
<k_ele><keb>今日</keb></k_ele>
<r_ele><reb>きょう</reb></r_ele>
<r_ele><reb>こんにち</reb></r_ele>
<sense><pos>&n;</pos><gloss>today</gloss><gloss>this day</gloss></sense>
</entry>
<entry>
<ent_seq>1591900</ent_seq>
<k_ele><keb>綺麗</keb></k_ele>
<k_ele><keb>奇麗</keb></k_ele>
<r_ele><reb>きれい</reb></r_ele>
<sense><pos>&adj-na;</pos><gloss>pretty</gloss><gloss>clean</gloss></sense>
</entry>
<entry>
<ent_seq>1049180</ent_seq>
<r_ele><reb>コーヒー</reb></r_ele>
<sense><pos>&n;</pos><gloss>coffee</gloss></sense>
</entry>
</JMdict>`

func loadSample(t *testing.T) *Dict {
	t.Helper()
	d, err := Load(strings.NewReader(sampleXML))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return d
}

func TestLoadParsesEntriesAndEntities(t *testing.T) {
	d := loadSample(t)
	if d.Size() != 3 {
		t.Fatalf("Size = %d, want 3", d.Size())
	}
	res := d.Lookup([]string{"今日"})
	if len(res) != 1 || len(res[0].Entries) != 1 {
		t.Fatalf("lookup 今日: got %+v", res)
	}
	e := res[0].Entries[0]
	if e.ID != 1244210 {
		t.Errorf("ID = %d, want 1244210", e.ID)
	}
	if len(e.Readings) != 2 || e.Readings[0] != "きょう" {
		t.Errorf("Readings = %v", e.Readings)
	}
	if len(e.Senses) != 1 || e.Senses[0].POS[0] != "n" {
		t.Errorf("entity-derived POS = %+v, want [n]", e.Senses)
	}
	if got := e.Senses[0].Glosses; len(got) != 2 || got[0] != "today" {
		t.Errorf("Glosses = %v", got)
	}
}

func TestLookupByReadingAndKanjiVariants(t *testing.T) {
	d := loadSample(t)
	for _, q := range []string{"綺麗", "奇麗", "きれい"} {
		res := d.Lookup([]string{q})
		if len(res[0].Entries) != 1 || res[0].Entries[0].ID != 1591900 {
			t.Errorf("lookup %q: got %+v", q, res[0].Entries)
		}
	}
}

func TestLookupDedupesWithinQuery(t *testing.T) {
	d := loadSample(t)
	// 今日 has two readings pointing at the same entry; querying both forms
	// in one request must not duplicate the entry within a single query.
	res := d.Lookup([]string{"きょう", "こんにち"})
	if len(res) != 2 {
		t.Fatalf("want 2 results, got %d", len(res))
	}
	for _, r := range res {
		if len(r.Entries) != 1 {
			t.Errorf("query %q: %d entries, want 1", r.Query, len(r.Entries))
		}
	}
}

func TestLookupKatakanaFallback(t *testing.T) {
	d := loadSample(t)
	// kagome emits kana lemmas as katakana; JMdict stores the reading as
	// hiragana. The miss-only fallback must bridge them.
	res := d.Lookup([]string{"キレイ"})
	if len(res[0].Entries) != 1 || res[0].Entries[0].ID != 1591900 {
		t.Fatalf("katakana fallback failed: %+v", res[0].Entries)
	}
	// Loanwords stored as katakana in JMdict resolve directly (no fallback
	// needed, and the fallback must not break them).
	res = d.Lookup([]string{"コーヒー"})
	if len(res[0].Entries) != 1 || res[0].Entries[0].ID != 1049180 {
		t.Fatalf("katakana direct hit failed: %+v", res[0].Entries)
	}
	// A genuine miss stays a miss.
	res = d.Lookup([]string{"ゾンビ"})
	if len(res[0].Entries) != 0 {
		t.Fatalf("expected no entries for ゾンビ, got %+v", res[0].Entries)
	}
}

func TestKataToHira(t *testing.T) {
	cases := []struct{ in, want string }{
		{"キョウ", "きょう"},
		{"キレイ", "きれい"},
		{"ヴ", "ゔ"},             // edge of the mapped block
		{"ヶ", "ゖ"},             // last mapped rune
		{"コーヒー", "こーひー"},       // chōonpu passes through
		{"今日", "今日"},           // kanji untouched
		{"abc ABC", "abc ABC"}, // ASCII untouched
		{"半角ｶﾅ", "半角ｶﾅ"},       // halfwidth katakana deliberately unmapped
		{"", ""},
	}
	for _, c := range cases {
		if got := KataToHira(c.in); got != c.want {
			t.Errorf("KataToHira(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
