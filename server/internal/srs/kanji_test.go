package srs

import "testing"

func TestKanjiInfoListsDeckWordsAndNote(t *testing.T) {
	s := openTestStore(t)
	for _, c := range []SaveCardReq{
		{Lemma: "提案", Readings: []string{"ていあん"}, Glosses: [][]string{{"proposal"}}},
		{Lemma: "案内", Readings: []string{"あんない"}, Glosses: [][]string{{"guidance"}}},
		{Lemma: "提出", Readings: []string{"ていしゅつ"}, Glosses: [][]string{{"submission"}}},
	} {
		if _, err := s.SaveCard(c); err != nil {
			t.Fatalf("SaveCard %s: %v", c.Lemma, err)
		}
	}
	if err := s.SetWordStatus("案内", 2); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKanjiNote("案", "A tree under a roof you sit at to plan: a plan, a proposal."); err != nil {
		t.Fatal(err)
	}
	info, err := s.KanjiInfo("案")
	if err != nil {
		t.Fatal(err)
	}
	if info.Note == "" {
		t.Error("note missing")
	}
	if len(info.Words) != 2 || info.Words[0].Lemma != "案内" || info.Words[0].Status != 2 || info.Words[1].Lemma != "提案" {
		t.Errorf("words = %+v", info.Words)
	}
	if info.Words[1].Readings[0] != "ていあん" {
		t.Errorf("readings = %+v", info.Words[1].Readings)
	}
	if err := s.SetKanjiNote("案", ""); err != nil {
		t.Fatal(err)
	}
	if info, _ = s.KanjiInfo("案"); info.Note != "" {
		t.Error("empty note should delete")
	}
	if _, err := s.KanjiInfo("提案"); !IsClientErr(err) {
		t.Errorf("multi-char kanji should be a client error, got %v", err)
	}
}

func TestKanjiInfoExplainsComponentsAndMarksKnownParts(t *testing.T) {
	lib := openTestLibrary(t)
	s := openTestStore(t)
	for _, c := range []SaveCardReq{
		{Lemma: "踊る", Readings: []string{"おどる"}, Glosses: [][]string{{"to dance"}}},
		{Lemma: "足す", Readings: []string{"たす"}, Glosses: [][]string{{"to add"}}},
		{Lemma: "通じる", Readings: []string{"つうじる"}, Glosses: [][]string{{"to get through"}}},
	} {
		if _, err := s.SaveCard(c); err != nil {
			t.Fatalf("SaveCard %s: %v", c.Lemma, err)
		}
	}
	if err := s.SetWordStatus("足す", 2); err != nil {
		t.Fatal(err)
	}
	// The library explains 踊 through its radicals; the deck knows 足 from 足す but has never met 甬.
	if _, err := lib.SetMnemonic("ja", "踊る", []MnemonicPart{{Kanji: "踊", Meaning: "= 足 foot"}, {Kanji: "甬", Meaning: "passage"}}, "Feet through a passage: to dance."); err != nil {
		t.Fatal(err)
	}
	if err := lib.SetKanjiNote("ja", "甬", "A passage, a hollow tube. 通じる to get through, 踊る to dance."); err != nil {
		t.Fatal(err)
	}
	info, err := s.KanjiInfo("踊")
	if err != nil {
		t.Fatal(err)
	}
	if info.Known {
		t.Error("踊 is only in a learning word")
	}
	if len(info.Components) != 2 || info.Components[0].Kanji != "足" || info.Components[0].Meaning != "foot" || !info.Components[0].Known ||
		info.Components[1].Kanji != "甬" || info.Components[1].Note == "" || !info.Components[1].NoteShared || info.Components[1].Known {
		t.Errorf("components = %+v", info.Components)
	}
	if info, _ = s.KanjiInfo("足"); !info.Known || len(info.Components) != 0 {
		t.Errorf("足: known through 足す, no single-kanji mnemonic: %+v", info)
	}

	// Served parts say which ones the learner has met; a card's own parts win over the library's.
	due, err := s.GetDueCards(10)
	if err != nil {
		t.Fatal(err)
	}
	var dance *DueCard
	for i := range due {
		if due[i].Lemma == "踊る" {
			dance = &due[i]
		}
	}
	if dance == nil || dance.Mnemonic == nil || len(dance.Mnemonic.Parts) != 2 {
		t.Fatalf("踊る due card = %+v", dance)
	}
	if k := dance.Mnemonic.Parts[1].Known; k == nil || *k {
		t.Errorf("甬 should be marked not known, got %v", k)
	}
	if k := dance.Mnemonic.Parts[0].Known; k == nil || *k {
		t.Errorf("踊 itself is not in a known word, got %v", k)
	}
	if _, err := s.SetMnemonic(dance.ID, []MnemonicPart{{Kanji: "足", Meaning: "foot"}, {Kanji: "甬", Meaning: "passage"}}, "Own story."); err != nil {
		t.Fatal(err)
	}
	if info, _ = s.KanjiInfo("踊"); len(info.Components) != 2 || info.Components[0].Kanji != "足" || !info.Components[0].Known {
		t.Errorf("own parts: %+v", info.Components)
	}
	stored, _ := s.GetMnemonic(dance.ID)
	if stored.Parts[0].Known != nil {
		t.Error("known must not be stored")
	}
}
