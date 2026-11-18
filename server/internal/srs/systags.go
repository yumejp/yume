package srs

import "strings"

// System tags are reserved app-defined word tags (kind='system'). The
// systemTags registry is the source of truth: the migration backfill,
// /tag_card validation and the delete_by_tag guard all key off it. New
// workflow = one const + one entry; descriptions live in the registry.
const (
	TagNeedsAudio       = "needs-audio"
	TagNeedsFix         = "needs-fix"
	TagNeedsSentence    = "needs-sentence"
	TagFlagged          = "flagged"
	TagNeedsExplanation = "needs-explanation"
	TagParked           = "parked"
	TagFurigana         = "furigana"
)

// SystemTag is one catalog entry; Description is shown by clients.
type SystemTag struct {
	Tag         string `json:"tag"`
	Description string `json:"description"`
}

var systemTags = []SystemTag{
	{TagNeedsAudio, "Generated audio is stale or wrong and should be regenerated (e.g. after a reading fix)."},
	{TagNeedsFix, "Card data is wrong or mis-mined (bad lemma/reading/gloss) and needs correcting."},
	{TagNeedsSentence, "Card has no example sentence."},
	{TagFlagged, "Flagged by the learner from the app as having a problem (reason unspecified); triage it."},
	{TagNeedsExplanation, "Learner asked for more explanation on this card; write its explanation (a tutor note when it needs more room), then clear the tag."},
	{TagParked, "Taken out of review (/park_cards) until brought back with /unpark_cards; history kept."},
	{TagFurigana, "Card front shows the reading: the learner knows the word by sound but keeps missing its kanji reading. Never auto-promoted to Known."},
}

var systemTagSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(systemTags))
	for _, t := range systemTags {
		m[t.Tag] = struct{}{}
	}
	return m
}()

// normTag must match the word_tags writers' normalization.
func normTag(tag string) string { return strings.TrimSpace(strings.ToLower(tag)) }

// IsSystemTag reports whether tag is a reserved system tag.
func IsSystemTag(tag string) bool {
	_, ok := systemTagSet[normTag(tag)]
	return ok
}

// SystemTags returns the catalog.
func SystemTags() []SystemTag { return systemTags }

func tagKind(tag string) string {
	if IsSystemTag(tag) {
		return "system"
	}
	return "user"
}
