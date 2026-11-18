package mcpserver

// Gloss enrichment for video-vocab rows — shared by the /get_video_vocab HTTP
// endpoint (handlers_deck.go) and the get_video_vocab MCP tool. The handler
// layer resolves dictionary data because the srs package has no dictionary
// access.

import (
	"github.com/yumejp/yume/server/internal/dict"
	"github.com/yumejp/yume/server/internal/language"
	"github.com/yumejp/yume/server/internal/srs"
)

// VideoVocabEntry is one enriched video-vocab row — the exact wire shape of
// both the HTTP endpoint and the MCP tool. Readings/Glosses are omitted when
// the dictionary has no match for the lemma.
type VideoVocabEntry struct {
	Lemma         string     `json:"lemma"`
	Status        int        `json:"status"`         // 0 unknown, 1 learning, 2 known, 3 ignored
	InVideoCues   int        `json:"in_video_cues"`  // cues of THIS video containing the lemma
	LibraryVideos int        `json:"library_videos"` // library reach: videos in the learner's same-lang library containing it
	Readings      []string   `json:"readings,omitempty"`
	Glosses       [][]string `json:"glosses,omitempty"` // per-sense gloss lists of the first dict entry
}

// EnrichVideoVocab attaches first-entry readings and per-sense glosses from
// the language's dictionary to the store's rows. Missing dictionaries or
// unmatched lemmas leave those optional fields empty. Never returns nil.
func EnrichVideoVocab(langCode string, words []srs.VideoVocabWord) []VideoVocabEntry {
	out := bareEntries(words)
	lang, ok := language.For(langCode)
	if !ok || lang.Dict == nil {
		return out
	}
	d, err := lang.Dict()
	if err != nil {
		return out
	}
	return enrichWithDict(d, out)
}

// bareEntries copies the store rows into wire rows with no dict fields.
func bareEntries(words []srs.VideoVocabWord) []VideoVocabEntry {
	out := make([]VideoVocabEntry, len(words))
	for i, w := range words {
		out[i] = VideoVocabEntry{
			Lemma:         w.Lemma,
			Status:        w.Status,
			InVideoCues:   w.InVideoCues,
			LibraryVideos: w.LibraryVideos,
		}
	}
	return out
}

// enrichWithDict fills Readings/Glosses from the first matching entry per
// lemma. Split from EnrichVideoVocab so tests can inject a small dict.
func enrichWithDict(d *dict.Dict, entries []VideoVocabEntry) []VideoVocabEntry {
	queries := make([]string, len(entries))
	for i, e := range entries {
		queries[i] = e.Lemma
	}
	// One batched lookup; results come back in request order.
	for i, res := range d.Lookup(queries) {
		if len(res.Entries) == 0 {
			continue
		}
		first := res.Entries[0]
		entries[i].Readings = first.Readings
		glosses := make([][]string, 0, len(first.Senses))
		for _, s := range first.Senses {
			if len(s.Glosses) > 0 {
				glosses = append(glosses, s.Glosses)
			}
		}
		if len(glosses) > 0 {
			entries[i].Glosses = glosses
		}
	}
	return entries
}

// WatchWordEntry is one enriched /list_watch_words row: the stored suggestion
// plus best-effort first-entry readings so clients can render furigana over
// the lemma. Readings are omitted when the dictionary has no match.
type WatchWordEntry struct {
	srs.WatchWord
	Readings []string `json:"readings,omitempty"`
}

// EnrichWatchWords attaches first-entry readings from the language's
// dictionary to watch-word rows. Same best-effort contract as
// EnrichVideoVocab: any missing capability just leaves Readings empty.
func EnrichWatchWords(langCode string, words []srs.WatchWord) []WatchWordEntry {
	out := make([]WatchWordEntry, len(words))
	for i, w := range words {
		out[i] = WatchWordEntry{WatchWord: w}
	}
	lang, ok := language.For(langCode)
	if !ok || lang.Dict == nil {
		return out
	}
	d, err := lang.Dict()
	if err != nil {
		return out
	}
	return enrichWatchWordsWithDict(d, out)
}

// enrichWatchWordsWithDict fills Readings per lemma; split out for tests.
func enrichWatchWordsWithDict(d *dict.Dict, entries []WatchWordEntry) []WatchWordEntry {
	queries := make([]string, len(entries))
	for i, e := range entries {
		queries[i] = e.Lemma
	}
	for i, res := range d.Lookup(queries) {
		if len(res.Entries) > 0 {
			entries[i].Readings = res.Entries[0].Readings
		}
	}
	return entries
}
