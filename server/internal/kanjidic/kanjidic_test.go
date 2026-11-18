package kanjidic

import (
	"os"
	"strings"
	"testing"
)

const sample = `<?xml version="1.0" encoding="UTF-8"?>
<kanjidic2><header><file_version>4</file_version></header>
<character><literal>案</literal><misc><grade>4</grade><stroke_count>10</stroke_count><freq>206</freq><jlpt>2</jlpt></misc>
<reading_meaning><rmgroup><reading r_type="pinyin">an4</reading><reading r_type="ja_on">アン</reading><reading r_type="ja_kun">つくえ</reading>
<meaning>plan</meaning><meaning>suggestion</meaning><meaning>faggot</meaning><meaning m_lang="fr">plan</meaning></rmgroup></reading_meaning></character>
</kanjidic2>`

func TestParseKeepsEnglishMeaningsAndJapaneseReadings(t *testing.T) {
	d, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	e := d.Lookup("案")
	if e == nil {
		t.Fatal("案 missing")
	}
	if len(e.Meanings) != 3 || e.Meanings[0] != "plan" || e.Meanings[2] != "bundle of sticks" || e.On[0] != "アン" || e.Kun[0] != "つくえ" {
		t.Errorf("entry = %+v", e)
	}
	if e.Grade != 4 || e.Strokes != 10 || e.JLPT != 2 || e.Freq != 206 {
		t.Errorf("misc = %+v", e)
	}
	if d.Lookup("提") != nil {
		t.Error("unknown kanji should be nil")
	}
}

func TestLoadRealFile(t *testing.T) {
	path := os.Getenv("YUME_KANJIDIC")
	if path == "" {
		t.Skip("YUME_KANJIDIC not set")
	}
	d, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Len() < 13000 || d.Lookup("夢") == nil || len(d.Lookup("夢").Meanings) == 0 {
		t.Errorf("len=%d 夢=%+v", d.Len(), d.Lookup("夢"))
	}
}
