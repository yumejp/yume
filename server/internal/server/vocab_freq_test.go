package server

import (
	"testing"

	"github.com/yumejp/yume/server/internal/dict"
	"github.com/yumejp/yume/server/internal/srs"
)

func TestBandsAndRankOrder(t *testing.T) {
	if bandFor(rankFromPriority(1)) != "1k" || bandFor(rankFromPriority(5)) != "3k" || bandFor(rankFromPriority(60)) != "12k" || bandFor(0) != "unranked" || bandFor(rankFromPriority(48)) != "beyond" {
		t.Fatalf("band mapping off: %s %s %s %s", bandFor(500), bandFor(2500), bandFor(7000), bandFor(24000))
	}
	rows := []srs.VocabRow{{Lemma: "b"}, {Lemma: "a"}}
	rows[0].Rank, rows[1].Rank = 0, 3000
	// annotateRanks re-derives ranks from the dictionary, so test the
	// counting on pre-banded rows instead.
	rows[0].Band, rows[1].Band = "unranked", "3k"
	rows[1].Status = 2
	if got := filterBand(append([]srs.VocabRow(nil), rows...), "3k"); len(got) != 1 || got[0].Lemma != "a" {
		t.Errorf("filterBand = %+v", got)
	}
	c := countBands(rows)
	if c[1].Total != 1 || c[1].Known != 1 || c[5].Total != 1 || c[5].Unknown != 1 {
		t.Errorf("counts = %+v", c)
	}
}

// An ichi1/news1 word is capped at the 6k-12k band whatever its nf band says:
// 食べる is ichi1 + nf25, which alone ranked it beyond 12,000. news1 words
// keep their nf rank (慰霊祭 is news1 + nf20, about rank 10,000).
func TestCommonWordsCapTheirRank(t *testing.T) {
	cases := []struct {
		name string
		e    dict.Entry
		want int
	}{
		{"食べる ichi1 nf25", dict.Entry{Priority: 25, Common: true}, 12000},
		{"図書館 ichi1 nf05", dict.Entry{Priority: 5, Common: true}, 2500},
		{"慰霊祭 news1 nf20", dict.Entry{Priority: 20, Common: true}, 10000},
		{"ichi1 without nf", dict.Entry{Priority: 60, Common: true}, 7000},
		{"nf25 alone", dict.Entry{Priority: 25}, 12500},
		{"spec1 alone", dict.Entry{Priority: 60}, 7000},
	}
	for _, c := range cases {
		if got := entryRankOf(c.e); got != c.want {
			t.Errorf("%s: rank %d, want %d", c.name, got, c.want)
		}
	}
}

// The cap may only move a word from "Beyond 12,000" into "6,000 to 12,000".
// Any other move would change the smaller bands, whose fixed Size is the
// coverage denominator, and lift rarer words over better-ranked ones.
func TestCommonCapOnlyLeavesBeyond(t *testing.T) {
	prios := []int{60, 80}
	for p := 1; p <= 48; p++ {
		prios = append(prios, p)
	}
	for _, p := range prios {
		plain := bandFor(rankFromPriority(p))
		capped := bandFor(entryRankOf(dict.Entry{Priority: p, Common: true}))
		if capped != plain && !(plain == "beyond" && capped == "12k") {
			t.Errorf("priority %d: common moves %q to %q", p, plain, capped)
		}
	}
}
