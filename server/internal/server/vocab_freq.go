package server

import (
	"sort"

	"github.com/yumejp/yume/server/internal/dict"
	"github.com/yumejp/yume/server/internal/srs"
)

// Frequency bands for the Library's tile view. Ranks come from JMdict's
// priority tags (nf band × 500 ≈ rank; the ichi/news/spec classes are
// placed after the banded words), so they are coarse and newspaper-biased,
// but free and always present. An ichi1/news1 word never ranks below
// commonRankCap: the nf band alone put 食べる (ichi1, nf25) beyond 12,000.
type vocabBand struct {
	Key     string `json:"band"`
	Label   string `json:"label"`
	RankMax int    `json:"rank_max,omitempty"`
	// Size is how many ranked words the band holds in the dictionary, so
	// clients can show coverage (known ÷ size) rather than known ÷ tracked.
	// 0 = open-ended (no coverage figure).
	Size int `json:"size,omitempty"`
}

var vocabBands = []vocabBand{
	{"1k", "Top 1,000", 1000, 1000},
	{"3k", "1,000 to 3,000", 3000, 2000},
	{"6k", "3,000 to 6,000", 6000, 3000},
	{"12k", "6,000 to 12,000", 12000, 6000},
	{"beyond", "Beyond 12,000", 1 << 30, 12000},
	{"unranked", "Rare", 0, 0},
}

// commonRankCap is the worst rank an ichi1/news1 word gets: the end of the
// "6,000 to 12,000" band. That is all the tags claim: news1 is the first
// 12,000 of the newspaper list (it always comes with nf01-24, so it is never
// capped) and ichi1 is a list of about 10,000 words. A lower cap would lift
// rarer news1 words over better-ranked ones and overfill the smaller bands,
// whose Size is the coverage denominator. The cap only moves words out of
// "Beyond 12,000".
const commonRankCap = 12000

func rankFromPriority(p int) int {
	switch {
	case p <= 0:
		return 0
	case p <= 48:
		return p * 500
	case p == 60:
		return 7000
	default:
		return 10000
	}
}

// entryRankOf is one entry's rank: its priority rank, capped for words
// JMdict marks common (ichi1/news1).
func entryRankOf(e dict.Entry) int {
	r := rankFromPriority(e.Priority)
	if e.Common && (r == 0 || r > commonRankCap) {
		r = commonRankCap
	}
	return r
}

func bandFor(rank int) string {
	if rank <= 0 {
		return "unranked"
	}
	for _, b := range vocabBands {
		if b.RankMax > 0 && rank <= b.RankMax {
			return b.Key
		}
	}
	return "beyond"
}

// entryRank finds the JMdict entry for a lemma (matching the written form,
// then the reading) and returns its rank; 0 when unknown or unranked.
func entryRank(d *dict.Dict, lemma string) int {
	if d == nil {
		return 0
	}
	res := d.Lookup([]string{lemma})
	if len(res) == 0 || len(res[0].Entries) == 0 {
		return 0
	}
	best := 0
	for _, e := range res[0].Entries {
		matches := false
		for _, k := range e.Kanji {
			if k == lemma {
				matches = true
			}
		}
		for _, r := range e.Readings {
			if r == lemma {
				matches = true
			}
		}
		if !matches {
			continue
		}
		if r := entryRankOf(e); r > 0 && (best == 0 || r < best) {
			best = r
		}
	}
	if best == 0 {
		best = entryRankOf(res[0].Entries[0])
	}
	return best
}

// annotateRanks fills Rank/Band and sorts most common first (ties A→Z,
// unranked last).
func annotateRanks(rows []srs.VocabRow) {
	d, _ := dict.Default()
	for i := range rows {
		rows[i].Rank = entryRank(d, rows[i].Lemma)
		rows[i].Band = bandFor(rows[i].Rank)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ri, rj := rows[i].Rank, rows[j].Rank
		if ri == 0 {
			ri = 1 << 30
		}
		if rj == 0 {
			rj = 1 << 30
		}
		if ri != rj {
			return ri < rj
		}
		return rows[i].Lemma < rows[j].Lemma
	})
}

// filterBand keeps rows in one band; "" keeps everything.
func filterBand(rows []srs.VocabRow, band string) []srs.VocabRow {
	if band == "" {
		return rows
	}
	out := rows[:0]
	for _, r := range rows {
		if r.Band == band {
			out = append(out, r)
		}
	}
	return out
}

type vocabBandCount struct {
	vocabBand
	Total    int `json:"total"`
	Known    int `json:"known"`
	Learning int `json:"learning"`
	Unknown  int `json:"unknown"`
	Ignored  int `json:"ignored"`
}

func countBands(rows []srs.VocabRow) []vocabBandCount {
	byKey := map[string]*vocabBandCount{}
	out := make([]vocabBandCount, len(vocabBands))
	for i, b := range vocabBands {
		out[i] = vocabBandCount{vocabBand: b}
		byKey[b.Key] = &out[i]
	}
	for _, r := range rows {
		c := byKey[r.Band]
		c.Total++
		switch r.Status {
		case 2:
			c.Known++
		case 1:
			c.Learning++
		case 3:
			c.Ignored++
		default:
			c.Unknown++
		}
	}
	return out
}
