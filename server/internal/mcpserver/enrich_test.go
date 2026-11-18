package mcpserver

import (
	"strings"
	"testing"

	"github.com/yumejp/yume/server/internal/dict"
	"github.com/yumejp/yume/server/internal/srs"
)

// Miniature JMdict, same structural quirks as the real file (see the dict
// package's tests): DOCTYPE entities, kana-only entry.
const sampleXML = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE JMdict [
<!ENTITY n "noun (common) (futsuumeishi)">
]>
<JMdict>
<entry>
<ent_seq>1244210</ent_seq>
<k_ele><keb>今日</keb></k_ele>
<r_ele><reb>きょう</reb></r_ele>
<sense><pos>&n;</pos><gloss>today</gloss><gloss>this day</gloss></sense>
<sense><pos>&n;</pos><gloss>these days</gloss></sense>
</entry>
</JMdict>`

func TestEnrichWithDict(t *testing.T) {
	d, err := dict.Load(strings.NewReader(sampleXML))
	if err != nil {
		t.Fatalf("dict.Load: %v", err)
	}
	entries := enrichWithDict(d, bareEntries([]srs.VideoVocabWord{
		{Lemma: "今日", Status: 0, InVideoCues: 3, LibraryVideos: 2},
		{Lemma: "存在しない語", Status: 1, InVideoCues: 1, LibraryVideos: 1},
	}))
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	hit := entries[0]
	if hit.InVideoCues != 3 || hit.LibraryVideos != 2 {
		t.Errorf("counts not preserved: %+v", hit)
	}
	if len(hit.Readings) != 1 || hit.Readings[0] != "きょう" {
		t.Errorf("Readings = %v, want [きょう]", hit.Readings)
	}
	if len(hit.Glosses) != 2 || hit.Glosses[0][0] != "today" || hit.Glosses[1][0] != "these days" {
		t.Errorf("Glosses = %v, want per-sense lists", hit.Glosses)
	}
	// Unmatched lemma: fields stay nil so omitempty drops them on the wire.
	miss := entries[1]
	if miss.Readings != nil || miss.Glosses != nil {
		t.Errorf("unmatched lemma should have no dict fields: %+v", miss)
	}
	if miss.Status != 1 || miss.InVideoCues != 1 || miss.LibraryVideos != 1 {
		t.Errorf("unmatched lemma lost its counts: %+v", miss)
	}
}

func TestEnrichVideoVocabUnknownLangDegrades(t *testing.T) {
	// An unregistered language must pass the rows through unenriched — the
	// counts are the signal; glosses are best-effort.
	words := []srs.VideoVocabWord{{Lemma: "貓", Status: 0, InVideoCues: 2, LibraryVideos: 1}}
	entries := EnrichVideoVocab("zz", words)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Lemma != "貓" || e.InVideoCues != 2 || e.LibraryVideos != 1 || e.Readings != nil || e.Glosses != nil {
		t.Errorf("unexpected passthrough row: %+v", e)
	}
	// Empty input still yields a non-nil slice ("words": [] on the wire).
	if got := EnrichVideoVocab("zz", nil); got == nil {
		t.Error("EnrichVideoVocab(nil) returned nil, want empty slice")
	}
}

func TestEnrichWatchWordsWithDict(t *testing.T) {
	d, err := dict.Load(strings.NewReader(sampleXML))
	if err != nil {
		t.Fatalf("dict.Load: %v", err)
	}
	rows := []WatchWordEntry{
		{WatchWord: srs.WatchWord{Lemma: "今日", Note: "n", Status: 0}},
		{WatchWord: srs.WatchWord{Lemma: "存在しない語", Status: 1}},
	}
	entries := enrichWatchWordsWithDict(d, rows)
	if len(entries[0].Readings) != 1 || entries[0].Readings[0] != "きょう" {
		t.Errorf("Readings = %v, want [きょう]", entries[0].Readings)
	}
	if entries[0].Note != "n" {
		t.Errorf("note lost: %+v", entries[0])
	}
	// Unmatched lemma: Readings stays nil so omitempty drops it on the wire.
	if entries[1].Readings != nil || entries[1].Status != 1 {
		t.Errorf("unmatched lemma wrong: %+v", entries[1])
	}
}
