// Package dict loads JMdict_e.gz (English-only JMdict, edrdg.org; licence at
// https://www.edrdg.org/edrdg/licence.html) into an in-memory index keyed by
// every kanji and reading form (~400K keys, ~200K entries, ~80MB). The file
// is loaded at runtime, not embedded, so users can swap in a newer JMdict.
package dict

import (
	"bufio"
	"compress/gzip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// Entry is the wire-format shape returned to clients.
type Entry struct {
	ID int `json:"id"`
	// Dict names the imported dictionary an entry came from; empty for JMdict.
	Dict string `json:"dict,omitempty"`
	// Lang is the language of the glosses: "en" for JMdict, the dictionary's
	// gloss_lang for imported entries.
	Lang     string   `json:"lang,omitempty"`
	Kanji    []string `json:"kanji,omitempty"`
	Readings []string `json:"readings,omitempty"`
	Senses   []Sense  `json:"senses"`
	// Pitches is parallel to Readings (nil per reading without data); omitted
	// when no enabled pitch dict matches the entry.
	Pitches []*int `json:"pitches,omitempty"`
	// Frequency rank (lower = more common) from any enabled frequency dict.
	Frequency *int `json:"frequency,omitempty"`
	// Priority is prio on the wire: 1–48 = JMdict nf band (band × 500 ≈ rank),
	// 60 = ichi1/news1/spec1/gai1, 80 = the *2 classes; omitted when unranked.
	Priority int `json:"priority,omitempty"`
	// Common: a form carries ichi1 or news1. These classes outrank the nf
	// band as a commonness signal (食べる is ichi1 but only nf25), so clients
	// show "common" rather than a numeric band when it is set.
	Common bool `json:"common,omitempty"`

	// prio orders results (lower = more common; 999 = unranked).
	prio int
}

// Sense is one meaning of an entry.
type Sense struct {
	POS     []string `json:"pos,omitempty"`     // ["n"], ["v1", "vt"], etc.
	Glosses []string `json:"glosses,omitempty"` // ["work", "job", "employment"]
}

// LookupResult is one query's matches, in request order.
type LookupResult struct {
	Query   string  `json:"query"`
	Entries []Entry `json:"entries"`
}

// Dict is an in-memory JMdict, ready to be queried.
type Dict struct {
	entries []Entry          // master list, indexed by slice position
	index   map[string][]int // surface form (kanji or reading) -> entry indices
	enIndex map[string][]int // English gloss word -> entry indices (for reverse search)
}

var (
	defaultDict *Dict
	defaultOnce sync.Once
	defaultErr  error

	// setupCommand is what the not-installed error tells the owner to run.
	setupCommand = "yume-server setup"
)

// ErrNotInstalled is Default's error when no JMdict_e.gz can be found.
var ErrNotInstalled = errors.New("dictionary not installed")

// SetDataRoot names the data folder the server was started with, so the
// not-installed error gives the exact setup command. Call it before Default.
func SetDataRoot(dir string) {
	if strings.ContainsAny(dir, " \t") {
		dir = `"` + dir + `"`
	}
	setupCommand = "yume-server setup -data " + dir
}

// Default returns the process-wide Dict, loading it on first use from the
// path findJMdictPath resolves. The result is kept for the life of the
// process, so a dictionary installed later needs a restart.
func Default() (*Dict, error) {
	defaultOnce.Do(func() {
		path := findJMdictPath()
		if path == "" {
			defaultErr = notInstalled()
			return
		}
		defaultDict, defaultErr = LoadFromFile(path)
	})
	return defaultDict, defaultErr
}

func notInstalled() error {
	return fmt.Errorf("%w: run `%s` (or set YUME_JMDICT), then restart the server",
		ErrNotInstalled, setupCommand)
}

func findJMdictPath() string {
	if p := os.Getenv("YUME_JMDICT"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	candidates := []string{
		"data/JMdict_e.gz",
		"server/data/JMdict_e.gz", // when running from repo root
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := exe[:strings.LastIndexAny(exe, "/\\")]
		candidates = append(candidates,
			exeDir+"/JMdict_e.gz",
			exeDir+"/data/JMdict_e.gz",
		)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// LoadFromFile reads JMdict_e.gz from disk and builds the index (~1-2s).
func LoadFromFile(path string) (*Dict, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open jmdict: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("gunzip jmdict: %w", err)
	}
	defer gz.Close()
	return Load(gz)
}

// Load parses a JMdict XML stream and builds the index.
func Load(r io.Reader) (*Dict, error) {
	// encoding/xml can't resolve JMdict's ~130 DTD entities (&n; &v1; …);
	// stripDTDEntities turns each into its bare token, which is the wanted output.
	bufReader := bufio.NewReader(r)
	clean, err := stripDTDEntities(bufReader)
	if err != nil {
		return nil, fmt.Errorf("preprocess jmdict: %w", err)
	}

	dec := xml.NewDecoder(strings.NewReader(clean))

	d := &Dict{
		entries: make([]Entry, 0, 200_000),
		index:   make(map[string][]int, 400_000),
		enIndex: make(map[string][]int, 200_000),
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode jmdict: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "entry" {
			continue
		}
		var raw rawEntry
		if err := dec.DecodeElement(&raw, &se); err != nil {
			return nil, fmt.Errorf("decode entry: %w", err)
		}
		entry := raw.toEntry()
		idx := len(d.entries)
		d.entries = append(d.entries, entry)
		for _, k := range entry.Kanji {
			d.index[k] = append(d.index[k], idx)
		}
		for _, r := range entry.Readings {
			d.index[r] = append(d.index[r], idx)
		}
		// Reverse English index, deduped per entry.
		enSeen := make(map[string]bool)
		for _, s := range entry.Senses {
			for _, g := range s.Glosses {
				for _, word := range glossWords(g) {
					if !enSeen[word] {
						enSeen[word] = true
						d.enIndex[word] = append(d.enIndex[word], idx)
					}
				}
			}
		}
	}
	return d, nil
}

// glossWords splits a gloss into lowercase letter/digit runs:
// "(computer) science" -> ["computer", "science"].
func glossWords(gloss string) []string {
	lower := strings.ToLower(gloss)
	return strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// Lookup returns each query's entries in request order, deduped per query.
// A miss is retried katakana→hiragana: kagome emits katakana readings
// (キレイ) while JMdict keys hiragana (きれい); loanwords (コーヒー) hit
// directly first.
func (d *Dict) Lookup(queries []string) []LookupResult {
	out := make([]LookupResult, 0, len(queries))
	for _, q := range queries {
		idxs := d.index[q]
		if len(idxs) == 0 {
			if h := KataToHira(q); h != q {
				idxs = d.index[h]
			}
		}
		entries := make([]Entry, 0, len(idxs))
		seen := make(map[int]bool, len(idxs))
		for _, i := range idxs {
			if seen[i] {
				continue
			}
			seen[i] = true
			entries = append(entries, d.entries[i])
		}
		// Common words first (JMdict file order put 波斯 above 端 for はし);
		// stable so equal-prio entries keep file order.
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].prio < entries[j].prio })
		out = append(out, LookupResult{Query: q, Entries: entries})
	}
	return out
}

// SearchEnglish is the reverse lookup. Query words are AND-ed; ranking is
// exact gloss > contiguous phrase > scattered words, then betterHit's
// tie-breaks. limit <= 0 means 50.
func (d *Dict) SearchEnglish(query string, limit int) []Entry {
	words := glossWords(query)
	if len(words) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	qPhrase := normGloss(query)

	counts := make(map[int]int)
	for _, word := range words {
		seen := make(map[int]bool)
		for _, idx := range d.enIndex[word] {
			if !seen[idx] {
				seen[idx] = true
				counts[idx]++
			}
		}
	}

	cands := make([]hit, 0, len(counts))
	for idx, c := range counts {
		if c < len(words) {
			continue
		}
		best := hit{idx: idx, score: 3, sense: 1 << 30, gpos: 1 << 30, ln: 1 << 30, prio: d.entries[idx].prio}
		for si, s := range d.entries[idx].Senses {
			for gi, g := range s.Glosses {
				lg := normGloss(g)
				var sc int
				switch {
				case lg == qPhrase:
					sc = 0
				case strings.Contains(lg, qPhrase):
					sc = 1
				default:
					sc = 2
				}
				cand := hit{idx: idx, score: sc, sense: si, gpos: gi, ln: len(lg), prio: d.entries[idx].prio}
				if betterHit(cand, best) {
					best = cand
				}
			}
		}
		cands = append(cands, best)
	}

	sort.Slice(cands, func(i, j int) bool { return betterHit(cands[i], cands[j]) })
	if len(cands) > limit {
		cands = cands[:limit]
	}
	out := make([]Entry, len(cands))
	for i, c := range cands {
		out[i] = d.entries[c.idx]
	}
	return out
}

// hit is a scored English-search candidate.
type hit struct {
	idx   int // entry slice index
	score int // 0 exact gloss, 1 phrase substring, 2 scattered words
	sense int // index of the best-matching sense (0 = primary meaning)
	gpos  int // position of the matching gloss within its sense (0 = head)
	prio  int // JMdict commonness rank (lower = more common)
	ln    int // matched gloss length
}

// betterHit orders by match quality, then sense/gloss position, then
// commonness (犬 beats スベタ for "dog"), then gloss length, then id.
func betterHit(a, b hit) bool {
	if a.score != b.score {
		return a.score < b.score
	}
	if a.sense != b.sense {
		return a.sense < b.sense
	}
	if a.gpos != b.gpos {
		return a.gpos < b.gpos
	}
	if a.prio != b.prio {
		return a.prio < b.prio
	}
	if a.ln != b.ln {
		return a.ln < b.ln
	}
	return a.idx < b.idx
}

// normGloss lowercases, drops (nested) parentheticals, collapses whitespace.
func normGloss(g string) string {
	var b strings.Builder
	depth := 0
	for _, r := range strings.ToLower(g) {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// priorityRank maps ke_pri/re_pri tags to a commonness rank (lower = more common).
func priorityRank(tags []string) int {
	rank := 999
	for _, t := range tags {
		switch {
		case strings.HasPrefix(t, "nf"):
			if n, err := strconv.Atoi(strings.TrimPrefix(t, "nf")); err == nil {
				rank = minInt(rank, n) // nf01..nf48
			}
		case t == "ichi1" || t == "news1" || t == "spec1" || t == "gai1":
			rank = minInt(rank, 60)
		case t == "ichi2" || t == "news2" || t == "spec2" || t == "gai2":
			rank = minInt(rank, 80)
		}
	}
	return rank
}

// hasCommonTag reports whether tags include ichi1 or news1.
func hasCommonTag(tags []string) bool {
	for _, t := range tags {
		if t == "ichi1" || t == "news1" {
			return true
		}
	}
	return false
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Size returns the entry count (reported by /health).
func (d *Dict) Size() int {
	if d == nil {
		return 0
	}
	return len(d.entries)
}

// --- XML structures (one-to-one with the JMdict schema) ---

type rawEntry struct {
	XMLName xml.Name   `xml:"entry"`
	EntSeq  int        `xml:"ent_seq"`
	K       []rawKEle  `xml:"k_ele"`
	R       []rawREle  `xml:"r_ele"`
	Sense   []rawSense `xml:"sense"`
}

type rawKEle struct {
	Keb string   `xml:"keb"`
	Pri []string `xml:"ke_pri"`
}

type rawREle struct {
	Reb string   `xml:"reb"`
	Pri []string `xml:"re_pri"`
}

type rawSense struct {
	POS   []string   `xml:"pos"`
	Gloss []rawGloss `xml:"gloss"`
}

type rawGloss struct {
	Lang string `xml:"lang,attr"`
	Text string `xml:",chardata"`
}

func (r *rawEntry) toEntry() Entry {
	e := Entry{ID: r.EntSeq, Lang: "en"}
	prio := 999
	for _, k := range r.K {
		e.Kanji = append(e.Kanji, k.Keb)
		prio = minInt(prio, priorityRank(k.Pri))
		e.Common = e.Common || hasCommonTag(k.Pri)
	}
	for _, rd := range r.R {
		e.Readings = append(e.Readings, rd.Reb)
		prio = minInt(prio, priorityRank(rd.Pri))
		e.Common = e.Common || hasCommonTag(rd.Pri)
	}
	e.prio = prio
	if prio < 999 {
		e.Priority = prio
	}
	for _, s := range r.Sense {
		sense := Sense{POS: cleanPOSList(s.POS)}
		for _, g := range s.Gloss {
			if g.Lang == "" || g.Lang == "eng" {
				if t := strings.TrimSpace(g.Text); t != "" {
					sense.Glosses = append(sense.Glosses, t)
				}
			}
		}
		if len(sense.Glosses) > 0 {
			e.Senses = append(e.Senses, sense)
		}
	}
	return e
}

func cleanPOSList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// --- DTD entity stripping ---

// stripDTDEntities drops the DOCTYPE internal subset (encoding/xml can't
// parse it) and rewrites each `&xyz;` to `xyz`, leaving the five XML
// built-ins alone.
func stripDTDEntities(r io.Reader) (string, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	s := string(raw)

	if start := strings.Index(s, "<!DOCTYPE"); start >= 0 {
		end := strings.Index(s[start:], "]>")
		if end < 0 {
			end = strings.Index(s[start:], ">")
			if end < 0 {
				return "", fmt.Errorf("malformed DOCTYPE in JMdict")
			}
			s = s[:start] + s[start+end+1:]
		} else {
			s = s[:start] + s[start+end+2:]
		}
	}

	s = entityRefRE.ReplaceAllStringFunc(s, func(m string) string {
		name := m[1 : len(m)-1]
		switch name {
		case "amp", "lt", "gt", "quot", "apos":
			return m
		}
		return name
	})

	return s, nil
}

var entityRefRE = regexp.MustCompile(`&[A-Za-z][A-Za-z0-9-]*;`)
