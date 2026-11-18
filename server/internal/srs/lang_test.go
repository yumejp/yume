package srs

import "testing"

// One deck file, two languages: nothing keyed by lemma may leak between them.
func TestLanguageViewsAreIsolated(t *testing.T) {
	ja := openTestStore(t)
	zh := ja.WithLang("zh")
	if ja.Lang() != "ja" || zh.Lang() != "zh" || ja.WithLang("") != ja || ja.WithLang(" JA ") != ja {
		t.Fatalf("lang views: ja=%q zh=%q", ja.Lang(), zh.Lang())
	}
	// 学生 exists in both languages with different statuses.
	if err := ja.SetWordStatus("学生", 2); err != nil {
		t.Fatal(err)
	}
	if err := zh.SetWordStatus("学生", 1); err != nil {
		t.Fatal(err)
	}
	jaRows, _ := ja.GetWordStatuses([]string{"学生"})
	zhRows, _ := zh.GetWordStatuses([]string{"学生"})
	if len(jaRows) != 1 || jaRows[0].Status != 2 || len(zhRows) != 1 || zhRows[0].Status != 1 {
		t.Errorf("statuses leak: ja=%+v zh=%+v", jaRows, zhRows)
	}
	jc, _ := ja.WordStatusCounts()
	zc, _ := zh.WordStatusCounts()
	if jc.Known != 1 || jc.Learning != 0 || zc.Learning != 1 || zc.Known != 0 {
		t.Errorf("counts leak: ja=%+v zh=%+v", jc, zc)
	}
	// Tags and suggestions are per language too.
	if err := ja.AddWordTag("学生", "school"); err != nil {
		t.Fatal(err)
	}
	if tags, _ := zh.ListAllTags(); len(tags) != 0 {
		t.Errorf("zh sees ja tags: %+v", tags)
	}
	if _, err := zh.AddWatchWords([]WatchWord{{Lemma: "学生"}}, "test"); err != nil {
		t.Fatal(err)
	}
	if ww, _ := ja.ListWatchWords(); len(ww) != 0 {
		t.Errorf("ja sees zh suggestions: %+v", ww)
	}
	// Cards and the vocab explorer are scoped; Known in ja does not retire a zh suggestion.
	if _, err := zh.SaveCard(SaveCardReq{Lemma: "学生", Readings: []string{"xuésheng"}, Glosses: [][]string{{"student"}}}); err != nil {
		t.Fatal(err)
	}
	has := true
	if jaVocab, _ := ja.ListVocab(VocabFilter{HasCard: &has}, 10, 0); jaVocab.Total != 0 {
		t.Errorf("ja vocab sees zh card: %+v", jaVocab.Rows)
	}
	if zhVocab, _ := zh.ListVocab(VocabFilter{}, 10, 0); zhVocab.Total != 1 || zhVocab.Rows[0].Status != 1 {
		t.Errorf("zh vocab = %+v", zhVocab.Rows)
	}
	if due, _ := ja.GetDueCards(10); len(due) != 0 {
		t.Errorf("ja due cards include zh: %+v", due)
	}
	// Kanji stories and imported dictionaries are per language.
	if err := zh.SetKanjiNote("学", "zh story"); err != nil {
		t.Fatal(err)
	}
	if info, _ := ja.KanjiInfo("学"); info.Note != "" {
		t.Errorf("ja sees zh kanji story: %q", info.Note)
	}
	id, err := zh.CreateDict(CreateDictReq{Name: "zh dict"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := zh.ImportTermChunk(id, []NormalizedTerm{{Term: "学生", DefinitionsJSON: `["student"]`}}); err != nil {
		t.Fatal(err)
	}
	if hits, _ := ja.LookupImported([]string{"学生"}); len(hits) != 0 {
		t.Errorf("ja lookup returns zh dictionary: %+v", hits)
	}
	if hits, _ := zh.LookupImported([]string{"学生"}); len(hits["学生"]) != 1 {
		t.Errorf("zh lookup = %+v", hits)
	}
	if dicts, _ := ja.ListDicts(); len(dicts) != 0 {
		t.Errorf("ja lists zh dict: %+v", dicts)
	}
}
