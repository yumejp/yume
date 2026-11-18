package srs

import (
	"testing"
)

func TestSetMnemonicValidatesAndReplaces(t *testing.T) {
	s := openTestStore(t)
	resp, err := s.SaveCard(SaveCardReq{Lemma: "方面", Readings: []string{"ほうめん"}, Glosses: [][]string{{"direction"}}})
	if err != nil {
		t.Fatal(err)
	}
	id := resp.ID
	kresp, err := s.SaveCard(SaveCardReq{Lemma: "いっそ", Readings: []string{"いっそ"}, Glosses: [][]string{{"rather"}}})
	if err != nil {
		t.Fatal(err)
	}
	kana := kresp.ID
	if _, _, err := s.AddCardNote(id, NoteKindTutor, "🎓 keep me", ""); err != nil {
		t.Fatal(err)
	}
	// Mnemonics are no longer notes, whichever way a client tries.
	if _, _, err := s.AddCardNote(id, "", "🎓 Mnemonic\n方 direction\nstory", ""); !IsClientErr(err) {
		t.Errorf("mnemonic text as a note should be refused, got %v", err)
	}
	if _, _, err := s.AddCardNote(id, NoteKindMnemonic, "x", ""); !IsClientErr(err) {
		t.Errorf("mnemonic kind as a note should be refused, got %v", err)
	}

	if _, err := s.SetMnemonic(id, nil, "story only"); err != nil {
		t.Errorf("a story without parts is allowed (the audit reports it): %v", err)
	}
	if _, err := s.SetMnemonic(id, []MnemonicPart{{Kanji: "案", Meaning: "plan"}}, "story"); !IsClientErr(err) {
		t.Errorf("part outside a two-kanji lemma should be a client error, got %v", err)
	}
	single, _ := s.SaveCard(SaveCardReq{Lemma: "踊る", Readings: []string{"おどる"}, Glosses: [][]string{{"dance"}}})
	if _, err := s.SetMnemonic(single.ID, []MnemonicPart{{Kanji: "足", Meaning: "foot"}, {Kanji: "甬", Meaning: "passage"}}, "Feet through a passage."); err != nil {
		t.Errorf("radical opening of a single-kanji word should be allowed: %v", err)
	}
	if _, err := s.SetMnemonic(id, []MnemonicPart{{Kanji: "方", Meaning: ""}}, "story"); !IsClientErr(err) {
		t.Errorf("empty meaning should be a client error, got %v", err)
	}
	if _, err := s.SetMnemonic(id, []MnemonicPart{{Kanji: "方", Meaning: "direction"}}, ""); !IsClientErr(err) {
		t.Errorf("empty story should be a client error, got %v", err)
	}
	if _, err := s.SetMnemonic(9999, []MnemonicPart{{Kanji: "方", Meaning: "direction"}}, "x"); !IsClientErr(err) {
		t.Errorf("unknown card should be a client error, got %v", err)
	}

	replaced, err := s.SetMnemonic(id, []MnemonicPart{{Kanji: "方", Meaning: "direction"}, {Kanji: "面", Meaning: "side"}}, " The facing side: -bound. 新宿方面. ")
	if err != nil {
		t.Fatal(err)
	}
	if !replaced {
		t.Error("a write over the story-only mnemonic should report a replacement")
	}
	m, err := s.GetMnemonic(id)
	if err != nil || m == nil {
		t.Fatalf("GetMnemonic: %v %v", m, err)
	}
	if len(m.Parts) != 2 || m.Parts[1] != (MnemonicPart{Kanji: "面", Meaning: "side"}) || m.Story != "The facing side: -bound. 新宿方面." {
		t.Errorf("mnemonic = %+v", m)
	}
	if replaced, _ := s.SetMnemonic(id, []MnemonicPart{{Kanji: "方", Meaning: "direction"}, {Kanji: "面", Meaning: "side"}}, "again"); !replaced {
		t.Error("second write should report a replacement")
	}
	notes, _ := s.ListCardNotes(id)
	if len(notes) != 1 || notes[0].Note != "keep me" {
		t.Errorf("notes after SetMnemonic = %+v", notes)
	}
	// The due-card payload carries it.
	due, err := s.GetDueCardsGated(20, 20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range due {
		if c.ID == id && c.Mnemonic != nil && c.Mnemonic.Story == "again" {
			found = true
		}
	}
	if !found {
		t.Errorf("due card should carry the mnemonic: %+v", due)
	}
	// Clearing.
	if _, err := s.SetMnemonic(id, nil, ""); err != nil {
		t.Fatal(err)
	}
	if m, _ := s.GetMnemonic(id); m != nil {
		t.Errorf("cleared mnemonic = %+v", m)
	}

	// Kana-only words need no parts; a single origin part is allowed.
	if _, err := s.SetMnemonic(kana, nil, "Rather, then: the giving-up word."); err != nil {
		t.Errorf("kana word without parts: %v", err)
	}
	if _, err := s.SetMnemonic(kana, []MnemonicPart{{Kanji: "いっそ", Meaning: "from 一層, one layer more"}}, "story"); err != nil {
		t.Errorf("kana origin part: %v", err)
	}
	if _, err := s.SetMnemonic(kana, []MnemonicPart{{Kanji: "一", Meaning: "one"}, {Kanji: "層", Meaning: "layer"}}, "story"); err != nil {
		t.Errorf("kana word with kanji-origin parts: %v", err)
	}
}

func TestMnemonicAudit(t *testing.T) {
	s := openTestStore(t)
	mk := func(lemma string) int64 {
		resp, err := s.SaveCard(SaveCardReq{Lemma: lemma, Readings: []string{"x"}, Glosses: [][]string{{"g"}}})
		if err != nil {
			t.Fatal(err)
		}
		return resp.ID
	}
	ok := mk("到底")
	if _, err := s.SetMnemonic(ok, []MnemonicPart{{Kanji: "到", Meaning: "reach"}, {Kanji: "底", Meaning: "bottom"}}, "story"); err != nil {
		t.Fatal(err)
	}
	free := mk("圧")
	if _, err := s.q().Exec(`UPDATE cards SET mnemonic_story = 'just a story' WHERE id = ?`, free); err != nil {
		t.Fatal(err)
	}
	missing := mk("提案")
	kanaMissing := mk("ほっとく")

	kinds := map[int64]string{}
	audit, err := s.MnemonicAudit(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range audit {
		kinds[r.CardID] = r.Kind
	}
	for id, k := range map[int64]string{ok: "ok", free: "no_breakdown", missing: "missing"} {
		if kinds[id] != k {
			t.Errorf("card %d kind = %q, want %q", id, kinds[id], k)
		}
	}
	if _, listed := kinds[kanaMissing]; listed {
		t.Error("kana word without a mnemonic should not be listed unless kanaToo")
	}
	audit, _ = s.MnemonicAudit(true)
	found := false
	for _, r := range audit {
		if r.CardID == kanaMissing && r.Kind == "missing" {
			found = true
		}
	}
	if !found {
		t.Error("kanaToo should list the kana word as missing")
	}
}
