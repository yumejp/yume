package srs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteByTagPreservesOtherLanguageMedia(t *testing.T) {
	ja := openTestStore(t)
	zh := ja.WithLang("zh")
	files := map[string][]string{}
	for _, s := range []*Store{ja, zh} {
		card, err := s.SaveCard(SaveCardReq{Lemma: "学生", Audio: &MediaBlob{Mime: "audio/mpeg", Data: "dGVzdA=="}, Image: &MediaBlob{Mime: "image/png", Data: "dGVzdA=="}})
		if err != nil {
			t.Fatal(err)
		}
		var audio, image string
		if err := s.q().QueryRow(`SELECT audio_file, image_file FROM cards WHERE id = ?`, card.ID).Scan(&audio, &image); err != nil {
			t.Fatal(err)
		}
		extra, err := s.AddCardMedia(card.ID, "audio", "alternate", "", "", &MediaBlob{Mime: "audio/mpeg", Data: "b3RoZXI="}, false)
		if err != nil {
			t.Fatal(err)
		}
		files[s.Lang()] = []string{audio, image, extra.File}
		if err := s.AddWordTag("学生", "import-test"); err != nil {
			t.Fatal(err)
		}
	}
	for _, names := range files {
		for _, name := range names {
			if _, err := os.Stat(filepath.Join(ja.MediaDir(), name)); err != nil {
				t.Fatal(err)
			}
		}
	}
	result, err := ja.DeleteByTag(DeleteByTagReq{Tag: "import-test"})
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedLemmas != 1 {
		t.Fatalf("deleted = %d", result.DeletedLemmas)
	}
	for lang, names := range files {
		for _, name := range names {
			_, err := os.Stat(filepath.Join(ja.MediaDir(), name))
			if lang == "ja" && !os.IsNotExist(err) {
				t.Errorf("deleted card file still exists: %s (%v)", name, err)
			}
			if lang == "zh" && err != nil {
				t.Errorf("other language file lost: %s (%v)", name, err)
			}
		}
	}
	var count int
	if err := ja.q().QueryRow(`SELECT COUNT(*) FROM cards WHERE lang = 'zh'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("surviving cards = %d (%v)", count, err)
	}
}
