package kanjidic

import "strings"

// Part is one kanji of a word with the share of the word's reading it carries.
type Part struct {
	Kanji   string `json:"kanji"`
	Reading string `json:"reading"`
}

// SplitReading divides a word's kana reading among its kanji using their
// KANJIDIC readings, allowing voicing (か→が) and gemination (つ→っ) at kanji
// boundaries. It returns nil when any kanji can't be matched (ateji such as
// 今日, readings outside KANJIDIC) so a wrong split is never shown.
func (d *Dict) SplitReading(word, reading string) []Part {
	if d == nil || word == "" || reading == "" {
		return nil
	}
	reading = toHiragana(reading)
	var segs []string
	hasKanji := false
	for _, r := range word {
		s := string(r)
		if isKanji(r) || r == '々' {
			segs = append(segs, s)
			hasKanji = true
			continue
		}
		if n := len(segs); n > 0 && !isKanjiStr(segs[n-1]) {
			segs[n-1] += s
		} else {
			segs = append(segs, s)
		}
	}
	if !hasKanji {
		return nil
	}
	var out []Part
	var walk func(i int, rest string, prev string) bool
	walk = func(i int, rest string, prev string) bool {
		if i == len(segs) {
			return rest == ""
		}
		seg := segs[i]
		if !isKanjiStr(seg) {
			kana := toHiragana(seg)
			return strings.HasPrefix(rest, kana) && walk(i+1, rest[len(kana):], "")
		}
		lit := seg
		if seg == "々" {
			lit = prev
		}
		for _, c := range d.candidates(lit, i > 0, i < len(segs)-1) {
			if strings.HasPrefix(rest, c) {
				out = append(out, Part{Kanji: seg, Reading: c})
				if walk(i+1, rest[len(c):], lit) {
					return true
				}
				out = out[:len(out)-1]
			}
		}
		return false
	}
	if !walk(0, reading, "") {
		return nil
	}
	return out
}

// candidates lists a kanji's readings in hiragana, longest first, with the
// sound changes compounds make: voicing when not first, っ when not last.
func (d *Dict) candidates(kanji string, voiced, geminate bool) []string {
	e := d.Lookup(kanji)
	if e == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	var base []string
	for _, on := range e.On {
		base = append(base, toHiragana(strings.Trim(on, "-")))
	}
	for _, kun := range e.Kun {
		k := strings.Trim(kun, "-")
		if i := strings.Index(k, "."); i >= 0 {
			// Compounds often drop the okurigana but keep its sound: 売上 うりあげ.
			stem, okuri := k[:i], k[i+1:]
			base = append(base, toHiragana(stem+okuri), toHiragana(stem+nounForm(okuri)))
			k = stem
		}
		base = append(base, toHiragana(k))
	}
	for _, b := range base {
		add(b)
		if voiced {
			add(voice(b))
			add(semiVoice(b))
		}
		if geminate {
			if g := geminateEnd(b); g != "" {
				add(g)
				if voiced {
					add(voice(g))
				}
			}
		}
	}
	// Longest first so 学校 takes がっ over が.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && len(out[j]) > len(out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

var voicedOf = map[rune]rune{
	'か': 'が', 'き': 'ぎ', 'く': 'ぐ', 'け': 'げ', 'こ': 'ご',
	'さ': 'ざ', 'し': 'じ', 'す': 'ず', 'せ': 'ぜ', 'そ': 'ぞ',
	'た': 'だ', 'ち': 'ぢ', 'つ': 'づ', 'て': 'で', 'と': 'ど',
	'は': 'ば', 'ひ': 'び', 'ふ': 'ぶ', 'へ': 'べ', 'ほ': 'ぼ',
}

var semiVoicedOf = map[rune]rune{'は': 'ぱ', 'ひ': 'ぴ', 'ふ': 'ぷ', 'へ': 'ぺ', 'ほ': 'ぽ'}

func voice(s string) string {
	r := []rune(s)
	if v, ok := voicedOf[r[0]]; ok {
		r[0] = v
		return string(r)
	}
	return ""
}

func semiVoice(s string) string {
	r := []rune(s)
	if v, ok := semiVoicedOf[r[0]]; ok {
		r[0] = v
		return string(r)
	}
	return ""
}

func geminateEnd(s string) string {
	r := []rune(s)
	if len(r) < 2 {
		return ""
	}
	switch r[len(r)-1] {
	case 'つ', 'く', 'ち', 'き':
		r[len(r)-1] = 'っ'
		return string(r)
	}
	return ""
}

var iRow = map[rune]rune{'う': 'い', 'く': 'き', 'ぐ': 'ぎ', 'す': 'し', 'つ': 'ち', 'ぬ': 'に', 'ぶ': 'び', 'む': 'み', 'る': 'り'}

// nounForm turns okurigana into the verb's noun form: る→り (取る→取り),
// and drops the final る of ichidan verbs (上げる→上げ).
func nounForm(okuri string) string {
	r := []rune(okuri)
	if len(r) >= 2 && r[len(r)-1] == 'る' {
		return string(r[:len(r)-1])
	}
	if v, ok := iRow[r[len(r)-1]]; ok {
		r[len(r)-1] = v
	}
	return string(r)
}

func toHiragana(s string) string {
	r := []rune(s)
	for i, c := range r {
		if c >= 'ァ' && c <= 'ヶ' {
			r[i] = c - 0x60
		}
	}
	return string(r)
}

func isKanji(r rune) bool { return r >= 0x4E00 && r <= 0x9FFF || r >= 0x3400 && r <= 0x4DBF }

func isKanjiStr(s string) bool {
	r := []rune(s)
	return len(r) == 1 && (isKanji(r[0]) || r[0] == '々')
}
