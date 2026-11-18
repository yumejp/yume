package srs

import "testing"

func TestNormalizeNote(t *testing.T) {
	cases := []struct {
		kind, note, tr, wantNote, wantTr string
	}{
		{NoteKindExample, "📝 猫が寝ている。\nThe cat is sleeping.", "", "猫が寝ている。", "The cat is sleeping."},
		{NoteKindExample, "猫が寝ている。", "The cat is sleeping.", "猫が寝ている。", "The cat is sleeping."},
		{NoteKindExample, "猫が寝ている。", "", "猫が寝ている。", ""},
		{NoteKindTutor, "🎓 Compare 方向.\nSecond line stays.", "ignored", "Compare 方向.\nSecond line stays.", ""},
		{NoteKindTriage, "⚠ Triage — check the reading", "", "Triage — check the reading", ""},
	}
	for _, c := range cases {
		gotNote, gotTr := normalizeNote(c.kind, c.note, c.tr)
		if gotNote != c.wantNote || gotTr != c.wantTr {
			t.Errorf("normalizeNote(%q, %q, %q) = %q, %q; want %q, %q", c.kind, c.note, c.tr, gotNote, gotTr, c.wantNote, c.wantTr)
		}
	}
}

func TestExampleNoteRoundTrip(t *testing.T) {
	s := openTestStore(t)
	resp, err := s.SaveCard(SaveCardReq{Lemma: "猫", Readings: []string{"ねこ"}, Glosses: [][]string{{"cat"}}})
	if err != nil {
		t.Fatal(err)
	}
	id := resp.ID
	if _, _, err := s.AddCardNote(id, NoteKindExample, "猫が寝ている。", "The cat is sleeping."); err != nil {
		t.Fatal(err)
	}
	notes, err := s.ListCardNotes(id)
	if err != nil || len(notes) != 1 {
		t.Fatalf("notes = %+v, %v", notes, err)
	}
	if notes[0].Note != "猫が寝ている。" || notes[0].Translation != "The cat is sleeping." {
		t.Errorf("note = %+v", notes[0])
	}
}
