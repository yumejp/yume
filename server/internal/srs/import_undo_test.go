package srs

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func boolPtr(v bool) *bool { return &v }

func importRows(lemmas ...string) []ImportRow {
	rows := make([]ImportRow, len(lemmas))
	for i, l := range lemmas {
		rows[i] = ImportRow{Lemma: l, Readings: []string{"よみ"}, Glosses: [][]string{{"gloss"}}}
	}
	return rows
}

func countRows(t *testing.T, s *Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := s.q().QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func wordTags(t *testing.T, s *Store, lemma string) []string {
	t.Helper()
	rows, err := s.q().Query(`SELECT tag FROM word_tags WHERE lang = ? AND lemma = ? ORDER BY tag`, s.lang, lemma)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			t.Fatal(err)
		}
		out = append(out, tag)
	}
	return out
}

// The audit's case: a word the learner already studies (review, note, audio)
// is in an Anki list. Importing and then undoing the import leaves it whole.
func TestImportUndoKeepsReviewedCardWithNotesAndAudio(t *testing.T) {
	s := openTestStore(t)
	id := 1467640
	card, err := s.SaveCard(SaveCardReq{JMdictID: &id, Lemma: "猫", Readings: []string{"ねこ"},
		Audio: &MediaBlob{Mime: "audio/mpeg", Data: "dGVzdA=="}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetWordStatus("猫", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviewCard(card.ID, 3); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AddCardNote(card.ID, NoteKindTutor, "🎓 keep me", ""); err != nil {
		t.Fatal(err)
	}
	var audio string
	if err := s.q().QueryRow(`SELECT audio_file FROM cards WHERE id = ?`, card.ID).Scan(&audio); err != nil {
		t.Fatal(err)
	}

	res, err := s.ImportCSV(ImportCSVReq{Tag: "anki-batch", Rows: importRows("猫", "犬")})
	if err != nil {
		t.Fatal(err)
	}
	if res.CreatedWords != 1 || res.CreatedCards != 1 || res.SkippedDuplicates != 1 {
		t.Fatalf("import = %+v", res)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != (ImportSkip{Lemma: "猫", Reason: "already in the deck"}) {
		t.Fatalf("skipped = %+v", res.Skipped)
	}
	if got := wordTags(t, s, "猫"); !slices.Equal(got, []string{"seen-in:anki-batch"}) {
		t.Fatalf("existing word tags = %v, want only the seen-in marker", got)
	}

	del, err := s.DeleteByTag(DeleteByTagReq{Tag: "anki-batch"})
	if err != nil {
		t.Fatal(err)
	}
	if del.DeletedLemmas != 1 || del.KeptStudied != 0 {
		t.Fatalf("undo = %+v", del)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM cards WHERE lemma = '犬'`); n != 0 {
		t.Fatalf("imported card survived the undo")
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM cards WHERE id = ? AND reps = 1`, card.ID); n != 1 {
		t.Fatal("reviewed card lost")
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM card_notes WHERE card_id = ?`, card.ID); n != 1 {
		t.Fatal("card note lost")
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM review_log WHERE card_id = ?`, card.ID); n != 1 {
		t.Fatal("review history lost")
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM word_status WHERE lemma = '猫' AND status = 1`); n != 1 {
		t.Fatal("word status lost")
	}
	if _, err := os.Stat(filepath.Join(s.MediaDir(), audio)); err != nil {
		t.Fatalf("audio lost: %v", err)
	}
	if got := wordTags(t, s, "猫"); len(got) != 0 {
		t.Fatalf("undo left tags %v", got)
	}
}

// A card saved from the dictionary has no word_status row; it still counts
// as in the deck.
func TestImportDedupsAgainstCardsWithoutStatus(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.SaveCard(SaveCardReq{Lemma: "意見"}); err != nil {
		t.Fatal(err)
	}
	res, err := s.ImportCSV(ImportCSVReq{Tag: "list", Rows: importRows("意見")})
	if err != nil {
		t.Fatal(err)
	}
	if res.CreatedWords != 0 || res.CreatedCards != 0 || res.SkippedDuplicates != 1 {
		t.Fatalf("import = %+v", res)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM word_status WHERE lemma = '意見'`); n != 0 {
		t.Fatal("import wrote a status for a word already in the deck")
	}
}

func TestImportWithoutDedupLeavesExistingWordsAlone(t *testing.T) {
	s := openTestStore(t)
	if err := s.SetWordStatus("犬", 2); err != nil {
		t.Fatal(err)
	}
	res, err := s.ImportCSV(ImportCSVReq{Tag: "extra", Dedup: boolPtr(false), Rows: importRows("犬")})
	if err != nil {
		t.Fatal(err)
	}
	if res.CreatedWords != 0 || res.CreatedCards != 1 {
		t.Fatalf("import = %+v", res)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM word_status WHERE lemma = '犬' AND status = 2`); n != 1 {
		t.Fatal("import overwrote the Known status")
	}
	if _, err := s.DeleteByTag(DeleteByTagReq{Tag: "extra"}); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM word_status WHERE lemma = '犬'`); n != 1 {
		t.Fatal("undo deleted a word that was in the deck before the import")
	}
}

// Words the import created but the learner has studied since stay unless the
// caller asks for them; a dry run reports the split and changes nothing.
func TestDeleteByTagKeepsStudiedWords(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.ImportCSV(ImportCSVReq{Tag: "batch", Rows: importRows("犬", "鳥")}); err != nil {
		t.Fatal(err)
	}
	var dog int64
	if err := s.q().QueryRow(`SELECT id FROM cards WHERE lemma = '犬'`).Scan(&dog); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviewCard(dog, 3); err != nil {
		t.Fatal(err)
	}

	dry, err := s.DeleteByTag(DeleteByTagReq{Tag: "batch", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !dry.DryRun || dry.DeletedLemmas != 1 || dry.KeptStudied != 1 || !slices.Equal(dry.KeptLemmas, []string{"犬"}) {
		t.Fatalf("dry run = %+v", dry)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM cards`); n != 2 {
		t.Fatal("dry run deleted cards")
	}

	del, err := s.DeleteByTag(DeleteByTagReq{Tag: "batch"})
	if err != nil {
		t.Fatal(err)
	}
	if del.DeletedLemmas != 1 || del.KeptStudied != 1 {
		t.Fatalf("undo = %+v", del)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM cards WHERE lemma = '犬'`); n != 1 {
		t.Fatal("studied card deleted")
	}

	all, err := s.DeleteByTag(DeleteByTagReq{Tag: "batch", IncludeStudied: true})
	if err != nil {
		t.Fatal(err)
	}
	if all.DeletedLemmas != 1 || all.KeptStudied != 0 {
		t.Fatalf("include_studied = %+v", all)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM cards`); n != 0 {
		t.Fatal("include_studied left cards")
	}
}

// Older imports put the batch tag on words that were already in the deck; the
// studied-word guard protects those too.
func TestDeleteByTagKeepsStudiedWordFromOlderImport(t *testing.T) {
	s := openTestStore(t)
	card, err := s.SaveCard(SaveCardReq{Lemma: "猫"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AddCardNote(card.ID, NoteKindExample, "猫が好き", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.AddWordTag("猫", "old-import"); err != nil {
		t.Fatal(err)
	}
	del, err := s.DeleteByTag(DeleteByTagReq{Tag: "old-import"})
	if err != nil {
		t.Fatal(err)
	}
	if del.DeletedLemmas != 0 || del.KeptStudied != 1 {
		t.Fatalf("undo = %+v", del)
	}
}

func TestSeenInTagsAreNotBatchTags(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.DeleteByTag(DeleteByTagReq{Tag: "seen-in:anki"}); !IsClientErr(err) {
		t.Fatalf("delete_by_tag on a seen-in tag: %v", err)
	}
	if _, err := s.ImportCSV(ImportCSVReq{Tag: "Seen-In:anki", Rows: importRows("犬")}); !IsClientErr(err) {
		t.Fatalf("import with a seen-in tag: %v", err)
	}
}

func TestKnownImportSetsStatusOnly(t *testing.T) {
	s := openTestStore(t)
	res, err := s.ImportCSV(ImportCSVReq{Tag: "known", DefaultStatus: intPtr(2), Rows: importRows("犬", "猫")})
	if err != nil {
		t.Fatal(err)
	}
	if res.CreatedWords != 2 || res.CreatedCards != 0 || res.SkippedCards != 2 {
		t.Fatalf("known import = %+v", res)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM word_status WHERE status = 2`); n != 2 {
		t.Fatalf("known statuses = %d", n)
	}

	res, err = s.ImportCSV(ImportCSVReq{Tag: "known-cards", DefaultStatus: intPtr(2), CreateCards: boolPtr(true), Rows: importRows("鳥")})
	if err != nil {
		t.Fatal(err)
	}
	if res.CreatedCards != 1 || res.SkippedCards != 0 {
		t.Fatalf("known import with create_cards = %+v", res)
	}
	res, err = s.ImportCSV(ImportCSVReq{Tag: "learning", Rows: importRows("魚")})
	if err != nil {
		t.Fatal(err)
	}
	if res.CreatedCards != 1 {
		t.Fatalf("learning import = %+v", res)
	}
}

func TestImportCleansLemmas(t *testing.T) {
	s := openTestStore(t)
	res, err := s.ImportCSV(ImportCSVReq{Tag: "raw", Rows: importRows(
		"<b>委員長</b>", "食[た]べる", "・・・", "安全&nbsp;", "日本[にほん] 語[ご]", "<br>", "食べる",
	)})
	if err != nil {
		t.Fatal(err)
	}
	want := []ImportSkip{
		{Lemma: "・・・", Reason: "punctuation, not a word"},
		{Lemma: "<br>", Reason: "no word left after removing formatting"},
		{Lemma: "食べる", Reason: "listed twice in this import"},
	}
	if !slices.Equal(res.Skipped, want) {
		t.Fatalf("skipped = %+v", res.Skipped)
	}
	if res.CreatedWords != 4 || res.CreatedCards != 4 || res.SkippedDuplicates != 1 {
		t.Fatalf("import = %+v", res)
	}
	for _, lemma := range []string{"委員長", "食べる", "安全", "日本語"} {
		if n := countRows(t, s, `SELECT COUNT(*) FROM cards WHERE lemma = ?`, lemma); n != 1 {
			t.Errorf("cards for %q = %d", lemma, n)
		}
	}
}

// save_card with a jmdict_id claims the word's imported card instead of
// adding a second one.
func TestSaveCardBackfillsImportedCard(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.ImportCSV(ImportCSVReq{Tag: "list", Rows: importRows("意見")}); err != nil {
		t.Fatal(err)
	}
	var imported int64
	if err := s.q().QueryRow(`SELECT id FROM cards WHERE lemma = '意見'`).Scan(&imported); err != nil {
		t.Fatal(err)
	}
	id := 1156530
	res, err := s.SaveCard(SaveCardReq{JMdictID: &id, Lemma: "意見", Sentence: "意見を言う。",
		Translation: "Say your opinion.", Readings: []string{"いけん"},
		Audio: &MediaBlob{Mime: "audio/mpeg", Data: "dGVzdA=="}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Created || res.ID != imported {
		t.Fatalf("save_card = %+v, want the imported card %d", res, imported)
	}
	// The imported card keeps its own readings and gains the mined sentence
	// and clip, which would otherwise be dropped.
	var readings, sentence, translation string
	if err := s.q().QueryRow(`SELECT readings, sentence, translation FROM cards WHERE id = ?`, imported).
		Scan(&readings, &sentence, &translation); err != nil {
		t.Fatal(err)
	}
	if readings != `["よみ"]` || sentence != "意見を言う。" || translation != "Say your opinion." {
		t.Fatalf("claimed card = %s / %s / %s", readings, sentence, translation)
	}
	if audio, _ := s.mediaFilesFor(imported); audio == "" {
		t.Fatal("the clip saved with the word was dropped")
	}
	st, err := s.CardStatus([]int{id})
	if err != nil {
		t.Fatal(err)
	}
	if st[0].CardID == nil || *st[0].CardID != imported {
		t.Fatalf("card_status = %+v", st)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM cards WHERE lemma = '意見'`); n != 1 {
		t.Fatalf("cards for the word = %d", n)
	}
	// Another language's card for the same lemma is never claimed.
	zh := s.WithLang("zh")
	if _, err := zh.SaveCard(SaveCardReq{Lemma: "学生"}); err != nil {
		t.Fatal(err)
	}
	other := 1206900
	res, err = s.SaveCard(SaveCardReq{JMdictID: &other, Lemma: "学生"})
	if err != nil || !res.Created {
		t.Fatalf("save_card across languages = %+v, %v", res, err)
	}
}

// Media removed by an undo waits in media/.trash; restoring a snapshot that
// still has the card moves it back, and old trash expires.
func TestDeleteByTagTrashesMediaAndRestoreBringsItBack(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.ImportCSV(ImportCSVReq{Tag: "batch", Rows: importRows("犬")}); err != nil {
		t.Fatal(err)
	}
	var dog int64
	if err := s.q().QueryRow(`SELECT id FROM cards WHERE lemma = '犬'`).Scan(&dog); err != nil {
		t.Fatal(err)
	}
	if err := s.AttachMedia(dog, &MediaBlob{Mime: "audio/mpeg", Data: "dGVzdA=="}, nil, "voicevox"); err != nil {
		t.Fatal(err)
	}
	audio, _ := s.mediaFilesFor(dog)
	snap, _, err := s.CreateBackup("with-audio")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.DeleteByTag(DeleteByTagReq{Tag: "batch"}); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(s.MediaDir(), audio)
	trashed := filepath.Join(s.mediaTrashDir(), audio)
	if _, err := os.Stat(live); !os.IsNotExist(err) {
		t.Fatalf("audio still in media/: %v", err)
	}
	if _, err := os.Stat(trashed); err != nil {
		t.Fatalf("audio not in the trash: %v", err)
	}

	if _, _, err := s.RestoreBackup(snap.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(live); err != nil {
		t.Fatalf("restore did not bring the audio back: %v", err)
	}

	// Expiry: a file trashed longer ago than MediaTrashDays is pruned.
	if err := os.MkdirAll(s.mediaTrashDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(s.mediaTrashDir(), "999-audio.mp3")
	if err := os.WriteFile(old, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().AddDate(0, 0, -MediaTrashDays-1)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	s.trashMedia(nil)
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("expired trash kept: %v", err)
	}
}
