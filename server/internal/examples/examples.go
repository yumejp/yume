// Package examples serves Japanese example sentences from the Tanaka Corpus
// (the Tatoeba-maintained "examples.utf" file distributed by EDRDG alongside
// JMdict). The file is built for exactly this use: every sentence pair
// carries a B-line indexing the sentence by JMdict headword — so per-word
// example lookup is an index hit, not a substring search.
//
// Format (one pair per two lines):
//
//	A: 彼は忙しい生活の中で家族と会うことがない。\tHe doesn't see ...#ID=...
//	B: 彼(かれ)[01] は 忙しい(いそがしい) 生活 の 中(なか) ... 会う[01] 事(こと){こと} ...
//
// Each B token is `headword(reading)[sense]{inflected-surface}~priority`.
// We index both the headword (matches kagome lemmas: 会う) and the braced
// surface form (matches exact inflections: 会えない).
//
// Like JMdict, the corpus is loaded at runtime, not embedded:
// $YUME_EXAMPLES → ./data/examples.utf.gz → <binary dir>/data/examples.utf.gz.
// Download: https://www.edrdg.org/pub/Nihongo/examples.utf.gz (CC BY 2.0 FR,
// see LICENSES.md).
package examples

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Pair is one example sentence with its English translation.
type Pair struct {
	Text        string `json:"text"`
	Translation string `json:"translation"`
}

// Corpus is the in-memory sentence store + word index.
type Corpus struct {
	pairs []Pair
	index map[string][]hit // headword / surface form → tagged pair indices
}

// hit is one sentence under an index key, with the B-line's annotations for
// that token: the JMdict sense number ([01]…) and the reading in parens,
// each empty/zero when the corpus left it out.
type hit struct {
	id      int32
	sense   uint8
	reading string
}

var (
	defaultOnce   sync.Once
	defaultCorpus *Corpus
	defaultErr    error
)

// Default loads the corpus on first use and caches it for the process
// lifetime (same pattern as dict.Default).
func Default() (*Corpus, error) {
	defaultOnce.Do(func() {
		path, err := findFile()
		if err != nil {
			defaultErr = err
			return
		}
		defaultCorpus, defaultErr = Load(path)
	})
	return defaultCorpus, defaultErr
}

// Loaded reports whether the corpus has been (successfully) loaded, plus its
// size — for /health. Never triggers a load.
func Loaded() (bool, int) {
	if defaultCorpus == nil {
		return false, 0
	}
	return true, len(defaultCorpus.pairs)
}

func findFile() (string, error) {
	candidates := []string{}
	if env := os.Getenv("YUME_EXAMPLES"); env != "" {
		candidates = append(candidates, env)
	}
	candidates = append(candidates, filepath.Join("data", "examples.utf.gz"))
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "data", "examples.utf.gz"))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf(
		"examples.utf.gz not found. Run `yume-server setup`, set $YUME_EXAMPLES or place the file in ./data/; download from https://www.edrdg.org/pub/Nihongo/examples.utf.gz")
}

// Load parses an examples.utf(.gz) file and builds the index.
func Load(path string) (*Corpus, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var r io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		zr, err := gzip.NewReader(f)
		if err != nil {
			return nil, fmt.Errorf("examples: %w", err)
		}
		defer zr.Close()
		r = zr
	}
	return Parse(r)
}

// Parse reads the A:/B: line pairs from r. Exposed for tests.
func Parse(r io.Reader) (*Corpus, error) {
	c := &Corpus{index: make(map[string][]hit)}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var cur Pair
	haveA := false
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "A: "):
			body := line[3:]
			// Strip the trailing "#ID=..." bookkeeping.
			if i := strings.LastIndex(body, "#ID="); i >= 0 {
				body = body[:i]
			}
			jp, en, ok := strings.Cut(body, "\t")
			if !ok {
				haveA = false
				continue
			}
			cur = Pair{Text: jp, Translation: en}
			haveA = true
		case strings.HasPrefix(line, "B: ") && haveA:
			id := int32(len(c.pairs))
			c.pairs = append(c.pairs, cur)
			seen := map[string]bool{}
			for _, tok := range strings.Fields(line[3:]) {
				sense, reading := tagsFor(tok)
				for _, key := range keysFor(tok) {
					if key == "" || seen[key] {
						continue
					}
					seen[key] = true
					c.index[key] = append(c.index[key], hit{id, sense, reading})
				}
			}
			haveA = false
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return c, nil
}

// keysFor extracts the index keys from one B-line token:
// `headword(reading)[sense]{surface}~prio` → ["headword", "surface"].
func keysFor(tok string) []string {
	// Headword = prefix up to the first annotation marker.
	head := tok
	if i := strings.IndexAny(head, "([{~"); i >= 0 {
		head = head[:i]
	}
	keys := []string{head}
	// Braced inflected form, when present.
	if i := strings.IndexByte(tok, '{'); i >= 0 {
		if j := strings.IndexByte(tok[i:], '}'); j > 0 {
			keys = append(keys, tok[i+1:i+j])
		}
	}
	return keys
}

// tagsFor reads a B-line token's sense number and reading:
// `解ける(とける)[04]{解け}` → 4, "とける". Readings given as an entry id
// (`解ける(#1198910)`) don't name a reading and come back empty.
func tagsFor(tok string) (sense uint8, reading string) {
	if i := strings.IndexByte(tok, '('); i >= 0 {
		if j := strings.IndexByte(tok[i:], ')'); j > 0 {
			if r := tok[i+1 : i+j]; !strings.HasPrefix(r, "#") {
				reading = r
			}
		}
	}
	if i := strings.IndexByte(tok, '['); i >= 0 {
		if j := strings.IndexByte(tok[i:], ']'); j > 0 {
			n := 0
			for _, ch := range tok[i+1 : i+j] {
				if ch < '0' || ch > '9' {
					n = 0
					break
				}
				n = n*10 + int(ch-'0')
			}
			if n > 0 && n < 256 {
				sense = uint8(n)
			}
		}
	}
	return sense, reading
}

// Lookup returns up to limit example pairs for the first query that has any
// hits (callers pass [lemma, surface], mirroring /lookup). Shorter sentences
// first — better flashcard material and faster to read in the popover.
//
// A headword can carry several senses and readings (解ける: solve / come
// undone / melt; とける or ほどける), and a card is about one of them. Sentences
// tagged with the headword's dominant sense in the corpus rank first, then
// untagged ones, then other senses; when the caller passes a reading, lines
// the corpus reads differently go last.
//
// The Tanaka Corpus is full of near-duplicate drill clusters ("彼は高貴な
// 生まれだ" / "彼女は高貴な生まれだ" / …), and shortest-first ranking packs
// the top slots with exactly those clones. Selection is therefore greedy
// with a similarity gate: a candidate too close to an already-picked
// sentence is deferred, and deferred clones only fill in when the corpus
// has nothing more varied to offer.
func (c *Corpus) Lookup(queries []string, limit int) []Pair {
	return c.LookupReading(queries, "", limit)
}

// LookupReading is Lookup with the word's reading, so sentences the corpus
// reads differently (解ける as ほどける) fall behind the rest.
func (c *Corpus) LookupReading(queries []string, reading string, limit int) []Pair {
	if limit <= 0 {
		limit = 5
	}
	for _, q := range queries {
		hits := c.index[q]
		if len(hits) == 0 {
			continue
		}
		dominant := dominantSense(hits)
		tier := func(h hit) int {
			switch {
			case reading != "" && h.reading != "" && h.reading != reading:
				return 3
			case h.sense == 0:
				return 1
			case h.sense == dominant:
				return 0
			}
			return 2
		}
		sorted := make([]hit, len(hits))
		copy(sorted, hits)
		sort.Slice(sorted, func(i, j int) bool {
			ti, tj := tier(sorted[i]), tier(sorted[j])
			if ti != tj {
				return ti < tj
			}
			a, b := c.pairs[sorted[i].id], c.pairs[sorted[j].id]
			if len(a.Text) != len(b.Text) {
				return len(a.Text) < len(b.Text)
			}
			return sorted[i].id < sorted[j].id
		})
		out := make([]Pair, 0, limit)
		grams := make([]map[string]bool, 0, limit)
		var skipped []int32
		for _, h := range sorted {
			id := h.id
			if len(out) == limit {
				break
			}
			g := bigrams(c.pairs[id].Text)
			dup := false
			for _, picked := range grams {
				if diceSimilarity(g, picked) > maxExampleSimilarity {
					dup = true
					break
				}
			}
			if dup {
				skipped = append(skipped, id)
				continue
			}
			out = append(out, c.pairs[id])
			grams = append(grams, g)
		}
		// Not enough variety in the corpus — top back up with the clones.
		for _, id := range skipped {
			if len(out) == limit {
				break
			}
			out = append(out, c.pairs[id])
		}
		return out
	}
	return nil
}

// dominantSense is the most frequent non-zero sense tag among hits (lowest
// number on a tie), or 0 when nothing is tagged.
func dominantSense(hits []hit) uint8 {
	var count [256]int
	best := uint8(0)
	for _, h := range hits {
		if h.sense == 0 {
			continue
		}
		count[h.sense]++
		if count[h.sense] > count[best] || (count[h.sense] == count[best] && h.sense < best) {
			best = h.sense
		}
	}
	return best
}

// maxExampleSimilarity is the character-bigram Dice coefficient above which
// two sentences count as near-duplicates. 0.5 keeps "noble birth" vs "noble
// blood" apart while collapsing he-said/she-said clones of one sentence.
const maxExampleSimilarity = 0.5

// bigrams returns the set of rune bigrams of s (single-rune strings fall
// back to the rune itself so short sentences still compare).
func bigrams(s string) map[string]bool {
	runes := []rune(s)
	set := make(map[string]bool, len(runes))
	if len(runes) == 1 {
		set[string(runes)] = true
	}
	for i := 0; i+1 < len(runes); i++ {
		set[string(runes[i:i+2])] = true
	}
	return set
}

// diceSimilarity is the Dice coefficient of two bigram sets: 2·|A∩B|/(|A|+|B|).
func diceSimilarity(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	common := 0
	small, large := a, b
	if len(b) < len(a) {
		small, large = b, a
	}
	for g := range small {
		if large[g] {
			common++
		}
	}
	return 2 * float64(common) / float64(len(a)+len(b))
}
