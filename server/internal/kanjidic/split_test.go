package kanjidic

import (
	"reflect"
	"testing"
)

func testDict() *Dict {
	e := func(lit string, on, kun []string) *Entry { return &Entry{Literal: lit, On: on, Kun: kun} }
	return &Dict{entries: map[string]*Entry{
		"供": e("供", []string{"キョウ", "ク"}, []string{"そな.える", "とも"}),
		"給": e("給", []string{"キュウ"}, []string{"たま.う"}),
		"学": e("学", []string{"ガク"}, []string{"まな.ぶ"}),
		"校": e("校", []string{"コウ"}, nil),
		"出": e("出", []string{"シュツ", "スイ"}, []string{"で.る", "だ.す"}),
		"発": e("発", []string{"ハツ", "ホツ"}, nil),
		"人": e("人", []string{"ジン", "ニン"}, []string{"ひと"}),
		"潰": e("潰", []string{"カイ"}, []string{"つぶ.れる"}),
		"今": e("今", []string{"コン", "キン"}, []string{"いま"}),
		"売": e("売", []string{"バイ"}, []string{"う.る", "う.れる"}),
		"上": e("上", []string{"ジョウ"}, []string{"うえ", "あ.げる", "あ.がる"}),
		"日": e("日", []string{"ニチ", "ジツ"}, []string{"ひ", "-び", "-か"}),
	}}
}

func TestSplitReading(t *testing.T) {
	d := testDict()
	cases := []struct {
		word, reading string
		want          []Part
	}{
		{"供給", "きょうきゅう", []Part{{"供", "きょう"}, {"給", "きゅう"}}},
		{"学校", "がっこう", []Part{{"学", "がっ"}, {"校", "こう"}}},
		{"出発", "しゅっぱつ", []Part{{"出", "しゅっ"}, {"発", "ぱつ"}}},
		{"人々", "ひとびと", []Part{{"人", "ひと"}, {"々", "びと"}}},
		{"潰れる", "つぶれる", []Part{{"潰", "つぶ"}}},
		{"売上", "うりあげ", []Part{{"売", "うり"}, {"上", "あげ"}}},
		{"今日", "きょう", nil}, // ateji: no honest split
		{"ひらがな", "ひらがな", nil},
	}
	for _, c := range cases {
		if got := d.SplitReading(c.word, c.reading); !reflect.DeepEqual(got, c.want) {
			t.Errorf("SplitReading(%s, %s) = %v, want %v", c.word, c.reading, got, c.want)
		}
	}
}
