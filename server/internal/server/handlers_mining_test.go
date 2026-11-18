package server

import "testing"

func TestSplitSentencesJA(t *testing.T) {
	got := splitSentencesJA("納期は来週です。大丈夫？\nコストが問題だ！")
	want := []string{"納期は来週です。", "大丈夫？", "コストが問題だ！"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sentence %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestIsContentTokenJA(t *testing.T) {
	cases := []struct {
		pos, pos1, surface string
		want               bool
	}{
		{"名詞", "一般", "納期", true},
		{"動詞", "自立", "進める", true},
		{"助詞", "", "は", false},
		{"助動詞", "", "です", false},
		{"記号", "", "、", false},
		{"名詞", "接尾", "さん", false},
		{"動詞", "非自立", "くる", false},
		{"名詞", "一般", "。。", false}, // punctuation-only surface
	}
	for _, c := range cases {
		if got := isContentTokenJA(c.pos, c.pos1, c.surface); got != c.want {
			t.Errorf("isContentTokenJA(%q,%q,%q)=%v want %v", c.pos, c.pos1, c.surface, got, c.want)
		}
	}
}
