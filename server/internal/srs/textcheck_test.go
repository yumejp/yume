package srs

import (
	"strings"
	"testing"
)

func TestCheckText(t *testing.T) {
	for _, c := range []struct {
		text string
		ok   bool
	}{
		{"意見が分かれる。", true},
		{"", true},
		{"本当?", true},
		{"What? Why?", true},
		{"？", true}, // full-width: typed, not lost
		{"??", false},
		{"?? ???", false},
		{" ? ", false},
		{"意見�", false},
		{"��", false},
	} {
		err := checkText("sentence", c.text)
		if (err == nil) != c.ok {
			t.Errorf("checkText(%q) = %v, want ok=%v", c.text, err, c.ok)
		}
		if err != nil && (!IsClientErr(err) || !strings.Contains(err.Error(), "sentence")) {
			t.Errorf("checkText(%q) error %v: want a client error naming the field", c.text, err)
		}
	}
}

// A Windows shell turns inline Japanese into '?' and a Japanese-locale one
// into bytes the JSON decoder makes U+FFFD; every Japanese write refuses
// both instead of saving them with a 200.
func TestJapaneseWritesRefuseLostText(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.SaveCard(SaveCardReq{Lemma: "??"}); !IsClientErr(err) || !strings.Contains(err.Error(), "encoding") {
		t.Errorf("SaveCard lemma ??: %v", err)
	}
	if _, err := s.SaveCard(SaveCardReq{Lemma: "意見", Sentence: "?????"}); !IsClientErr(err) {
		t.Errorf("SaveCard sentence ?????: %v", err)
	}
	if _, err := s.SaveCard(SaveCardReq{Lemma: "意見", Readings: []string{"��"}}); !IsClientErr(err) {
		t.Errorf("SaveCard reading U+FFFD: %v", err)
	}
	resp, err := s.SaveCard(SaveCardReq{Lemma: "意見", Readings: []string{"いけん"}, Sentence: "意見が分かれる。"})
	if err != nil {
		t.Fatal(err)
	}
	id := resp.ID

	bad := "???"
	if err := s.UpdateCard(id, CardUpdate{Sentence: &bad}); !IsClientErr(err) {
		t.Errorf("UpdateCard sentence ???: %v", err)
	}
	if err := s.UpdateCard(id, CardUpdate{Readings: &[]string{"??"}}); !IsClientErr(err) {
		t.Errorf("UpdateCard readings ??: %v", err)
	}
	good, en := "意見を言う。", "Say what you think."
	if err := s.UpdateCard(id, CardUpdate{Sentence: &good, Translation: &en}); err != nil {
		t.Errorf("UpdateCard good sentence: %v", err)
	}

	if _, _, err := s.AddCardNote(id, NoteKindTutor, "?? ??", ""); !IsClientErr(err) {
		t.Errorf("AddCardNote ?? ??: %v", err)
	}
	if _, _, err := s.AddCardNote(id, NoteKindExample, "意見�", "An opinion."); !IsClientErr(err) {
		t.Errorf("AddCardNote U+FFFD: %v", err)
	}
	if err := s.SetKanjiNote("意", "??"); !IsClientErr(err) {
		t.Errorf("SetKanjiNote ??: %v", err)
	}
	if _, err := s.SetMnemonic(id, []MnemonicPart{{Kanji: "意", Meaning: "mind"}, {Kanji: "見", Meaning: "see"}}, "?????"); !IsClientErr(err) {
		t.Errorf("SetMnemonic story ?????: %v", err)
	}
	if _, err := openTestLibrary(t).SetMnemonic(DefaultLang, "??", nil, "A story."); !IsClientErr(err) {
		t.Errorf("library SetMnemonic lemma ??: %v", err)
	}

	card, err := s.ListVocab(VocabFilter{Search: "意見"}, 10, 0)
	if err != nil || len(card.Rows) != 1 || card.Rows[0].Sentence == nil || *card.Rows[0].Sentence != good {
		t.Fatalf("card after refused writes = %+v, %v", card.Rows, err)
	}
	if notes, _ := s.ListCardNotes(id); len(notes) != 0 {
		t.Errorf("refused notes were stored: %+v", notes)
	}
}

// One lost row refuses the whole import before its backup or any write.
func TestImportCSVRefusesLostText(t *testing.T) {
	s := openTestStore(t)
	_, err := s.ImportCSV(ImportCSVReq{Tag: "batch", Rows: []ImportRow{
		{Lemma: "意見", Sentence: "意見が分かれる。"},
		{Lemma: "??", Sentence: "???"},
	}})
	if !IsClientErr(err) || !strings.Contains(err.Error(), "row 2 lemma") {
		t.Fatalf("ImportCSV: %v", err)
	}
	if backups, _ := s.ListBackups(); len(backups) != 0 {
		t.Errorf("refused import left backups: %+v", backups)
	}
	if c, _ := s.WordStatusCounts(); c.Learning != 0 {
		t.Errorf("refused import wrote words: %+v", c)
	}
}

// A tutor always sends an example's English, but a learner's own example
// and a sentence saved from yume-clipboard come without it and must still
// be stored.
func TestExampleNoteTranslationIsOptional(t *testing.T) {
	s := openTestStore(t)
	resp, err := s.SaveCard(SaveCardReq{Lemma: "意見"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AddCardNote(resp.ID, NoteKindExample, "意見が分かれる。", ""); err != nil {
		t.Errorf("example without translation: %v", err)
	}
	// The legacy two-line form still carries its English.
	if _, _, err := s.AddCardNote(resp.ID, "", "📝 意見が分かれる。\nOpinions differ.", ""); err != nil {
		t.Errorf("legacy example: %v", err)
	}
	if _, _, err := s.AddCardNote(resp.ID, NoteKindTutor, "Compare 意思.", ""); err != nil {
		t.Errorf("tutor note: %v", err)
	}
}
