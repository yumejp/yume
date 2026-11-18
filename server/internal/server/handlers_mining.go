package server

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/yumejp/yume/server/internal/language"
	"github.com/yumejp/yume/server/internal/srs"
)

// Transcript mining tokenizes submitted text without storing it, excludes
// known or ignored words, and returns recurring unknown content words.
// /add_watch_words turns accepted results into suggestions.

// miningStopJA excludes a small set of grammatical words that IPADIC tags as
// content words. Keep this list limited to closed-class words.
var miningStopJA = map[string]bool{
	"する": true, "ある": true, "いる": true, "なる": true, "やる": true,
	"できる": true, "くれる": true, "もらう": true, "いう": true, "言う": true,
	"こと": true, "もの": true, "ため": true, "よう": true, "ところ": true,
	"これ": true, "それ": true, "あれ": true, "どれ": true,
	"ここ": true, "そこ": true, "あそこ": true, "どこ": true,
	"わたし": true, "私": true, "あなた": true, "彼": true, "彼女": true,
}

// contentPOS is the ja content-word filter (see language.IsContent):
// exclude symbols/particles/auxiliaries/fillers by POS, and the 非自立/接尾
// subcategories. Language-specific by nature; guarded by lang == "ja" below
// until other profiles exist server-side.
func isContentTokenJA(pos, pos1, surface string) bool {
	switch pos {
	case "記号", "補助記号", "助詞", "助動詞", "フィラー":
		return false
	}
	if pos1 == "非自立" || pos1 == "接尾" {
		return false
	}
	// Pure punctuation / whitespace surfaces that slipped POS tagging.
	return strings.TrimFunc(surface, func(r rune) bool {
		return r < 0x3040 && !('0' <= r && r <= '9') && !('A' <= r && r <= 'z')
	}) != ""
}

type mineTranscriptReq struct {
	Text string `json:"text"`
	Lang string `json:"lang"`
	// MinCount drops words mentioned fewer times (default 2 — a word said
	// once is an encounter, a word said three times is a theme).
	MinCount int `json:"min_count"`
	// Limit caps the result rows (default 20, cap 100).
	Limit int `json:"limit"`
}

// MinedWord is one recurring unknown from a transcript.
type MinedWord struct {
	Lemma string `json:"lemma"`
	Count int    `json:"count"`
	// Status is the learner's current word status (0 unknown / 1 learning);
	// 2/3 rows are filtered out before this ever renders.
	Status int `json:"status"`
	// Sentence is the shortest transcript sentence containing the word —
	// context for the accept/reject decision, discarded with the response.
	Sentence string   `json:"sentence,omitempty"`
	Readings []string `json:"readings,omitempty"`
	Glosses  []string `json:"glosses,omitempty"`
}

func registerMiningRoutes(mux *http.ServeMux, cfg Config) {
	jsonPost(mux, "/mine_transcript", func(_ *http.Request, store *srs.Store, req mineTranscriptReq) (any, error) {
		if strings.TrimSpace(req.Text) == "" {
			return nil, badRequest("text is required")
		}
		lang, ok := language.For(req.Lang)
		if !ok || lang.Tokenize == nil {
			return nil, badRequest("no tokenizer for lang=" + req.Lang + "; supported: " + strings.Join(language.Supported(), ", "))
		}
		if !lang.Mining {
			return nil, badRequest("transcript mining is not available for lang=" + lang.Code)
		}
		if req.MinCount <= 0 {
			req.MinCount = 2
		}
		if req.Limit <= 0 {
			req.Limit = 20
		}
		if req.Limit > 100 {
			req.Limit = 100
		}

		// Sentence-split so each mined word can carry its shortest context
		// line. Japanese sentence enders plus newlines (ASR emits both).
		sentences := splitSentencesJA(req.Text)
		counts := map[string]int{}
		context := map[string]string{}
		for _, sent := range sentences {
			toks, err := lang.Tokenize(sent)
			if err != nil {
				return nil, fmt.Errorf("tokenize: %w", err)
			}
			seen := map[string]bool{}
			for _, t := range toks {
				if !isContentTokenJA(t.POS, t.POS1, t.Surface) {
					continue
				}
				lemma := t.Lemma
				if lemma == "" {
					lemma = t.Surface
				}
				if miningStopJA[lemma] {
					continue
				}
				// Count once per sentence — a word repeated within one
				// utterance is emphasis, not a second encounter.
				if seen[lemma] {
					continue
				}
				seen[lemma] = true
				counts[lemma]++
				if cur, ok := context[lemma]; !ok || len(sent) < len(cur) {
					context[lemma] = sent
				}
			}
		}

		lemmas := make([]string, 0, len(counts))
		for l, c := range counts {
			if c >= req.MinCount {
				lemmas = append(lemmas, l)
			}
		}
		// Drop known (2) and ignored (3) — the whole point is unknowns.
		status := map[string]int{}
		if len(lemmas) > 0 {
			rows, err := store.GetWordStatuses(lemmas)
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				status[row.Lemma] = row.Status
			}
		}
		out := make([]MinedWord, 0, len(lemmas))
		for _, l := range lemmas {
			if st := status[l]; st >= 2 {
				continue
			}
			out = append(out, MinedWord{Lemma: l, Count: counts[l], Status: status[l], Sentence: context[l]})
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Count != out[j].Count {
				return out[i].Count > out[j].Count
			}
			return out[i].Lemma < out[j].Lemma
		})
		if len(out) > req.Limit {
			out = out[:req.Limit]
		}
		// Gloss-enrich the survivors (post-cap: lookups aren't free).
		// Best-effort — a missing dict degrades to bare lemmas.
		if lang.Dict != nil {
			if d, err := lang.Dict(); err == nil {
				for i := range out {
					res := d.Lookup([]string{out[i].Lemma})
					if len(res) > 0 && len(res[0].Entries) > 0 {
						e := res[0].Entries[0]
						out[i].Readings = e.Readings
						if len(e.Senses) > 0 {
							out[i].Glosses = e.Senses[0].Glosses
						}
					}
				}
			}
		}
		return map[string]any{"words": out, "sentences": len(sentences)}, nil
	})

	// HTTP twin of MCP suggest_words: accepted mined words become
	// suggestions. Same store call the tutor uses.
	jsonPost(mux, "/add_watch_words", func(_ *http.Request, store *srs.Store, req struct {
		Words []srs.WatchWord `json:"words"`
	}) (any, error) {
		if len(req.Words) == 0 || len(req.Words) > 100 {
			return nil, badRequest("words must have 1-100 entries")
		}
		added, err := store.AddWatchWords(req.Words, "meeting")
		if err != nil {
			return nil, err
		}
		return map[string]any{"added": added}, nil
	})
}

// splitSentencesJA splits on Japanese sentence enders and newlines, keeping
// the ender attached. ASR transcripts mix punctuation and line breaks.
func splitSentencesJA(text string) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			out = append(out, s)
		}
		cur.Reset()
	}
	for _, r := range text {
		switch r {
		case '\n', '\r':
			flush()
		case '。', '！', '？', '!', '?':
			cur.WriteRune(r)
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}
