package srs

import "testing"

func TestVoiceCreditOnClipsAndBackfill(t *testing.T) {
	s := openTestStore(t)
	resp, err := s.SaveCard(SaveCardReq{Lemma: "方面", Readings: []string{"ほうめん"}, Glosses: [][]string{{"direction"}}})
	if err != nil {
		t.Fatal(err)
	}
	blob := &MediaBlob{Mime: "audio/wav", Data: "UklGRg=="}
	// A new clip carries its credit directly.
	if _, err := s.AddCardMedia(resp.ID, "audio", "word", "voicevox:3", " ずんだもん ", blob, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddCardMedia(resp.ID, "audio", "sentence", "voicevox:2", "四国めたん", blob, true); err != nil {
		t.Fatal(err)
	}
	media, err := s.ListCardMedia(resp.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, m := range media {
		got[m.Label] = m.Voice
	}
	if got["word"] != "ずんだもん" || got["sentence"] != "四国めたん" {
		t.Errorf("voices = %v (whitespace trimmed, stored per clip)", got)
	}
	voices, err := s.ListMediaVoices()
	if err != nil {
		t.Fatal(err)
	}
	if len(voices) != 2 || voices[0].Clips != 1 {
		t.Errorf("voices = %+v", voices)
	}
}

func TestListVocabSortLapses(t *testing.T) {
	s := openTestStore(t)
	for _, l := range []string{"提案", "案内"} {
		if _, err := s.SaveCard(SaveCardReq{Lemma: l, Readings: []string{"x"}, Glosses: [][]string{{"g"}}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.q().Exec(`UPDATE cards SET lapses = 3, reps = 5 WHERE lemma = '案内'`); err != nil {
		t.Fatal(err)
	}
	has := true
	resp, err := s.ListVocab(VocabFilter{HasCard: &has, Sort: "lapses"}, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Rows) != 2 || resp.Rows[0].Lemma != "案内" || resp.Rows[0].Lapses != 3 || resp.Rows[0].Reps != 5 {
		t.Errorf("rows = %+v", resp.Rows)
	}
}
