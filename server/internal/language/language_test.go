package language

import "testing"

func TestFor(t *testing.T) {
	cases := []struct {
		code   string
		wantOK bool
		want   string
	}{
		{"ja", true, "ja"},
		{"ja-Hira", true, "ja"}, // prefix fallback
		{"", true, "ja"},        // empty defaults to ja (wire compat)
		{"zh", false, ""},
		{"jab", false, ""}, // prefix must be at a '-' boundary
	}
	for _, c := range cases {
		l, ok := For(c.code)
		if ok != c.wantOK {
			t.Fatalf("For(%q) ok = %v, want %v", c.code, ok, c.wantOK)
		}
		if ok && l.Code != c.want {
			t.Fatalf("For(%q) = %q, want %q", c.code, l.Code, c.want)
		}
	}
}

func TestJaCapabilities(t *testing.T) {
	l, ok := For("ja")
	if !ok {
		t.Fatal("ja not registered")
	}
	if l.Tokenize == nil || l.Dict == nil {
		t.Fatal("ja must have both a tokenizer and a dictionary")
	}
	toks, err := l.Tokenize("新しい生活")
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	if len(toks) == 0 {
		t.Fatal("tokenize returned no tokens")
	}
}
