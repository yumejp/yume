package dict

import "strings"

// KataToHira converts katakana to hiragana, leaving everything else (kanji,
// ASCII, the long-vowel mark ー) untouched.
//
// Why this exists: kagome/IPADIC emits readings and kana lemmas as katakana
// (キョウ, キレイ) while JMdict stores reading forms as hiragana (きょう,
// きれい). Without normalization, kana-only words miss the index entirely.
// Loanwords aren't affected — JMdict stores those readings as katakana
// (コーヒー), so they hit the index directly before any fallback runs.
//
// The mapped block is U+30A1 (ァ) through U+30F6 (ヶ), which sits exactly
// 0x60 above its hiragana counterpart ぁ..ゖ. Halfwidth katakana (U+FF66+)
// never appears in IPADIC output, so it's deliberately not handled.
func KataToHira(s string) string {
	var b strings.Builder
	changed := false
	for _, r := range s {
		if r >= 'ァ' && r <= 'ヶ' {
			r -= 0x60
			changed = true
		}
		b.WriteRune(r)
	}
	if !changed {
		return s
	}
	return b.String()
}
