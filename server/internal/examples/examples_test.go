package examples

import (
	"strings"
	"testing"
)

const fixture = `A: 彼は忙しい生活の中で家族と会うことがない。	He doesn't see his family in his busy life.#ID=303697_100000
B: 彼(かれ)[01] は 忙しい(いそがしい) 生活 の 中(なか) で(#2028980) 家族 と 会う[01] 事(こと){こと} が 無い{ない}
A: 彼は忙しいので、君に会えない。	In as much as he is busy, he can't meet you.#ID=303695_100002
B: 彼(かれ)[01] は 忙しい(いそがしい) ので 君(きみ)[01] に 会う[01]{会えない}
A: 短い。	Short.#ID=1_1
B: 短い(みじかい)
`

func TestParseAndLookup(t *testing.T) {
	c, err := Parse(strings.NewReader(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(c.pairs); got != 3 {
		t.Fatalf("pairs = %d, want 3", got)
	}

	// Headword lookup hits both 会う sentences.
	hits := c.Lookup([]string{"会う"}, 5)
	if len(hits) != 2 {
		t.Fatalf("会う hits = %d, want 2", len(hits))
	}
	// Shorter sentence sorts first.
	if !strings.Contains(hits[0].Text, "会えない") {
		t.Fatalf("expected shorter 会えない sentence first, got %q", hits[0].Text)
	}
	if hits[0].Translation == "" || strings.Contains(hits[0].Translation, "#ID=") {
		t.Fatalf("translation not cleaned: %q", hits[0].Translation)
	}

	// Braced inflected-surface lookup.
	if got := c.Lookup([]string{"会えない"}, 5); len(got) != 1 {
		t.Fatalf("会えない hits = %d, want 1", len(got))
	}

	// [lemma, surface] fallthrough: first query with hits wins.
	if got := c.Lookup([]string{"存在しない語", "短い"}, 5); len(got) != 1 {
		t.Fatalf("fallthrough hits = %d, want 1", len(got))
	}

	// Limit respected.
	if got := c.Lookup([]string{"会う"}, 1); len(got) != 1 {
		t.Fatalf("limit ignored: %d", len(got))
	}

	// Misses return nil.
	if got := c.Lookup([]string{"ない語"}, 5); got != nil {
		t.Fatalf("expected nil for miss, got %v", got)
	}
}

// The real 高貴 cluster from the corpus: five near-identical "noble birth"
// drill sentences plus one genuinely different sentence, which
// shortest-first ranking alone always cut.
const cloneFixture = `A: 彼は高貴な生まれだ。	He is of noble ancestry.#ID=296389_107297
B: 彼(かれ)[01] は 高貴{高貴な} 生まれ だ
A: 彼女は高貴な生まれだ。	She was born with a silver spoon in her mouth.#ID=313622_90092
B: 彼女[01] は 高貴{高貴な} 生まれ だ
A: 彼は高貴な家柄の人です。	He is a man of noble blood.#ID=296388_107298
B: 彼(かれ)[01] は 高貴{高貴な} 家柄 の 人(ひと) です
A: 彼は高貴な生まれの人です。	He is a man of noble birth.#ID=296390_107296
B: 彼(かれ)[01] は 高貴{高貴な} 生まれ の 人(ひと) です
A: 彼女は高貴な生まれの女性です。	She is a woman of noble birth.#ID=313623_90091
B: 彼女[01] は 高貴{高貴な} 生まれ の 女性[01] です
A: 彼女はまるで王女のように高貴な顔立ちをしている。	She looks as noble as if she were a princess.#ID=312271_91442
B: 彼女[01] は 丸で{まるで} 王女 の様に[01]{のように} 高貴{高貴な}~ 顔立ち を 為る(する){している}
`

func TestLookupDiversifiesNearDuplicates(t *testing.T) {
	c, err := Parse(strings.NewReader(cloneFixture))
	if err != nil {
		t.Fatal(err)
	}

	hits := c.Lookup([]string{"高貴"}, 3)
	if len(hits) != 3 {
		t.Fatalf("hits = %d, want 3", len(hits))
	}
	// Shortest sentence still leads.
	if hits[0].Text != "彼は高貴な生まれだ。" {
		t.Fatalf("expected shortest sentence first, got %q", hits[0].Text)
	}
	// The he-said/she-said clone of it must not take a slot…
	for _, h := range hits {
		if h.Text == "彼女は高貴な生まれだ。" {
			t.Fatalf("near-duplicate clone survived: %v", hits)
		}
	}
	// …so the one genuinely different sentence makes the cut.
	found := false
	for _, h := range hits {
		if strings.Contains(h.Text, "王女") {
			found = true
		}
	}
	if !found {
		t.Fatalf("princess sentence should be picked over clones, got %v", hits)
	}
}

func TestLookupFillsFromClonesWhenCorpusHasNoVariety(t *testing.T) {
	c, err := Parse(strings.NewReader(cloneFixture))
	if err != nil {
		t.Fatal(err)
	}

	// Asking for more than the distinct sentences available must still
	// return limit pairs (clones fill the tail), never come up short.
	hits := c.Lookup([]string{"高貴"}, 5)
	if len(hits) != 5 {
		t.Fatalf("hits = %d, want 5 (clones should backfill)", len(hits))
	}
	seen := map[string]bool{}
	for _, h := range hits {
		if seen[h.Text] {
			t.Fatalf("same sentence returned twice: %q", h.Text)
		}
		seen[h.Text] = true
	}
}

func TestDiceSimilarity(t *testing.T) {
	a := bigrams("彼は高貴な生まれだ。")
	if got := diceSimilarity(a, a); got != 1 {
		t.Fatalf("self similarity = %v, want 1", got)
	}
	b := bigrams("完全に別の文です。")
	if got := diceSimilarity(a, b); got > 0.2 {
		t.Fatalf("unrelated similarity = %v, want ~0", got)
	}
	if got := diceSimilarity(a, map[string]bool{}); got != 0 {
		t.Fatalf("empty-set similarity = %v, want 0", got)
	}
}

// 解ける in the corpus: dozens of "solve" ([01]) sentences, a few "melt"
// ([04]) ones, and some read ほどける. A card is about one sense and one
// reading, so the picker should not lead with the short "snow melted" line.
func TestLookupPrefersDominantSenseAndReading(t *testing.T) {
	src := `A: 雪は解け去った。	The snow has melted away.#ID=1_1
B: 雪 は 解ける(とける)[04]{解け} 去る{去った}
A: 帯がほどけた。	The sash came undone.#ID=2_2
B: 帯 が 解ける(ほどける){ほどけた}
A: 楽にその問題が解けた。	It was easy for me to solve the problem.#ID=3_3
B: 楽{楽に} 其の[01]{その} 問題 が 解ける(とける)[01]{解けた}
A: 昨日の謎がやっと解けたようだ。	Yesterday's mystery seems solved at last.#ID=4_4
B: 昨日 の 謎 が やっと 解ける(とける)[01]{解けた} 様だ{ようだ}
A: 紐が解けている。	The string is untied.#ID=5_5
B: 紐 が 解ける{解けて} 居る{いる}
`
	c, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	got := c.LookupReading([]string{"解ける"}, "とける", 5)
	want := []string{"楽にその問題が解けた。", "昨日の謎がやっと解けたようだ。", "紐が解けている。", "雪は解け去った。", "帯がほどけた。"}
	if len(got) != len(want) {
		t.Fatalf("got %d pairs, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Text != want[i] {
			t.Fatalf("pos %d = %q, want %q (all: %v)", i, got[i].Text, want[i], got)
		}
	}
	// Without a reading, the ほどける line is just another non-dominant sense
	// (untagged), so it sorts by length among the untagged tier.
	if got := c.Lookup([]string{"解ける"}, 5); got[2].Text != "帯がほどけた。" {
		t.Fatalf("no-reading order: %v", got)
	}
}

func TestTagsFor(t *testing.T) {
	cases := []struct {
		tok     string
		sense   uint8
		reading string
	}{
		{"解ける(とける)[04]{解け}", 4, "とける"},
		{"解ける(#1198910)[02]{とけた}~", 2, ""},
		{"彼(かれ)[01]", 1, "かれ"},
		{"問題", 0, ""},
		{"其の[01]{その}", 1, ""},
	}
	for _, tc := range cases {
		s, r := tagsFor(tc.tok)
		if s != tc.sense || r != tc.reading {
			t.Errorf("tagsFor(%q) = %d,%q want %d,%q", tc.tok, s, r, tc.sense, tc.reading)
		}
	}
}
