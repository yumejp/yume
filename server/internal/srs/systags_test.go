package srs

import "testing"

func TestSystemTagsAndTagCard(t *testing.T) {
	s := newVocabStore(t)

	resp, err := s.SaveCard(SaveCardReq{
		Lemma:    "失う",
		Readings: []string{"うしなう"},
		Glosses:  [][]string{{"to lose"}},
		Sentence: "彼は信用を失った。",
	})
	if err != nil {
		t.Fatal(err)
	}

	// A registered system tag applies and is stored with kind=system.
	lemma, err := s.TagCard(resp.ID, TagNeedsAudio, false)
	if err != nil {
		t.Fatalf("TagCard: %v", err)
	}
	if lemma != "失う" {
		t.Fatalf("tagged lemma = %q, want 失う", lemma)
	}
	tags, err := s.ListAllTags()
	if err != nil {
		t.Fatal(err)
	}
	var found *TagCount
	for i := range tags {
		if tags[i].Tag == TagNeedsAudio {
			found = &tags[i]
		}
	}
	if found == nil {
		t.Fatal("needs-audio not listed")
	}
	if found.Kind != "system" {
		t.Fatalf("needs-audio kind = %q, want system", found.Kind)
	}

	// A non-system tag is rejected by TagCard.
	if _, err := s.TagCard(resp.ID, "my-custom-tag", false); err == nil {
		t.Fatal("TagCard accepted a non-system tag")
	}

	// delete_by_tag refuses system tags (it would nuke the card).
	if _, err := s.DeleteByTag(DeleteByTagReq{Tag: TagNeedsAudio}); err == nil {
		t.Fatal("DeleteByTag accepted a system tag")
	}

	// Removing the system tag clears it.
	if _, err := s.TagCard(resp.ID, TagNeedsAudio, true); err != nil {
		t.Fatalf("TagCard remove: %v", err)
	}
	tags, _ = s.ListAllTags()
	for _, tc := range tags {
		if tc.Tag == TagNeedsAudio {
			t.Fatal("needs-audio still present after removal")
		}
	}

	// Unknown card id is an error.
	if _, err := s.TagCard(999999, TagNeedsAudio, false); err == nil {
		t.Fatal("TagCard accepted a nonexistent card")
	}
}

// Free-form tags added via AddWordTag stay kind=user; system tags get
// kind=system even through that path.
func TestAddWordTagKind(t *testing.T) {
	s := newVocabStore(t)
	if err := s.AddWordTag("猫", "clipboard"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddWordTag("犬", TagNeedsFix); err != nil {
		t.Fatal(err)
	}
	tags, err := s.ListAllTags()
	if err != nil {
		t.Fatal(err)
	}
	kind := map[string]string{}
	for _, tc := range tags {
		kind[tc.Tag] = tc.Kind
	}
	if kind["clipboard"] != "user" {
		t.Fatalf("clipboard kind = %q, want user", kind["clipboard"])
	}
	if kind[TagNeedsFix] != "system" {
		t.Fatalf("needs-fix kind = %q, want system", kind[TagNeedsFix])
	}
}

// FlagWord (the MCP/tutor by-lemma path) sets/clears system tags and feeds
// SystemTaggedWords; non-system tags are rejected.
func TestFlagWordAndList(t *testing.T) {
	s := newVocabStore(t)

	if err := s.FlagWord("失う", TagNeedsAudio, false); err != nil {
		t.Fatalf("FlagWord: %v", err)
	}
	if err := s.FlagWord("食べる", TagNeedsSentence, false); err != nil {
		t.Fatalf("FlagWord: %v", err)
	}
	if err := s.FlagWord("猫", "banana", false); err == nil {
		t.Fatal("FlagWord accepted a non-system tag")
	}

	all, err := s.SystemTaggedWords("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("all system-tagged = %d, want 2 (%v)", len(all), all)
	}

	audio, err := s.SystemTaggedWords(TagNeedsAudio)
	if err != nil {
		t.Fatal(err)
	}
	if len(audio) != 1 || audio[0].Lemma != "失う" {
		t.Fatalf("needs-audio list = %v, want [失う]", audio)
	}

	if _, err := s.SystemTaggedWords("banana"); err == nil {
		t.Fatal("SystemTaggedWords accepted a non-system tag")
	}

	if err := s.FlagWord("失う", TagNeedsAudio, true); err != nil {
		t.Fatalf("FlagWord remove: %v", err)
	}
	all, _ = s.SystemTaggedWords("")
	if len(all) != 1 {
		t.Fatalf("after removal = %d, want 1", len(all))
	}
}

// System tags ride along on due cards (DueCard.SystemTags) so the review
// client can render persisted flag state (🚩 flagged / ❓ needs-explanation)
// straight from the due list. User tags stay off the wire, and an unflagged
// card keeps a nil slice (field omitted from JSON).
func TestDueCardsCarrySystemTags(t *testing.T) {
	s := newVocabStore(t)

	flagged, err := s.SaveCard(SaveCardReq{Lemma: "失う", Glosses: [][]string{{"to lose"}}})
	if err != nil {
		t.Fatalf("save flagged: %v", err)
	}
	plain, err := s.SaveCard(SaveCardReq{Lemma: "食べる", Glosses: [][]string{{"to eat"}}})
	if err != nil {
		t.Fatalf("save plain: %v", err)
	}
	// Flag through the existing system-tag API (what the mobile 🚩/❓ buttons hit).
	for _, tag := range []string{TagFlagged, TagNeedsExplanation} {
		if _, err := s.TagCard(flagged.ID, tag, false); err != nil {
			t.Fatalf("TagCard %s: %v", tag, err)
		}
	}
	// A free-form user tag on the same lemma must NOT leak into SystemTags.
	if err := s.AddWordTag("失う", "clipboard"); err != nil {
		t.Fatalf("AddWordTag: %v", err)
	}

	due, err := s.GetDueCards(10)
	if err != nil {
		t.Fatalf("GetDueCards: %v", err)
	}
	byID := map[int64]DueCard{}
	for _, c := range due {
		byID[c.ID] = c
	}
	got := byID[flagged.ID].SystemTags
	if len(got) != 2 || got[0] != TagFlagged || got[1] != TagNeedsExplanation {
		t.Errorf("flagged card system_tags = %v, want [%s %s]", got, TagFlagged, TagNeedsExplanation)
	}
	if tags := byID[plain.ID].SystemTags; tags != nil {
		t.Errorf("unflagged card system_tags = %v, want nil (omitted)", tags)
	}
}
