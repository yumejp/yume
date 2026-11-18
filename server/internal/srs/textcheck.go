package srs

import (
	"strings"
	"unicode"
)

// checkText refuses Japanese that was lost on its way to the server. A
// shell on a Western-locale Windows turns kana and kanji in a command-line
// argument into '?', which arrives as valid JSON and would be saved with a
// 200; a Japanese-locale one (code page 932) sends Shift-JIS bytes, which
// the JSON decoder turns into U+FFFD. Either way the text is gone, so the
// write is refused instead of stored. field names the part in the error.
func checkText(field, text string) error {
	if strings.ContainsRune(text, unicode.ReplacementChar) {
		return clientErrf("%s contains characters that were not valid UTF-8: send the request body as UTF-8, for example from a file with curl --data-binary @body.json", field)
	}
	if onlyQuestionMarks(text) {
		return clientErrf("%s arrived as %q, so its Japanese was lost to the sender's text encoding: send the request body as UTF-8, for example from a file with curl --data-binary @body.json", field, strings.TrimSpace(text))
	}
	return nil
}

// checkTexts is checkText over a list field such as readings.
func checkTexts(field string, texts []string) error {
	for _, t := range texts {
		if err := checkText(field, t); err != nil {
			return err
		}
	}
	return nil
}

// checkCardText runs checkText over a new card's Japanese fields. prefix
// places the field in a batch ("row 3 ").
func checkCardText(prefix, lemma string, kanji, readings []string, sentence string) error {
	if err := checkText(prefix+"lemma", lemma); err != nil {
		return err
	}
	if err := checkTexts(prefix+"kanji", kanji); err != nil {
		return err
	}
	if err := checkTexts(prefix+"readings", readings); err != nil {
		return err
	}
	return checkText(prefix+"sentence", sentence)
}

// checkText runs checkText over the Japanese fields an update sends.
func (u CardUpdate) checkText() error {
	var lemma, sentence string
	var kanji, readings []string
	if u.Lemma != nil {
		lemma = *u.Lemma
	}
	if u.Sentence != nil {
		sentence = *u.Sentence
	}
	if u.Kanji != nil {
		kanji = *u.Kanji
	}
	if u.Readings != nil {
		readings = *u.Readings
	}
	return checkCardText("", lemma, kanji, readings, sentence)
}

// onlyQuestionMarks reports whether text is one or more '?' and nothing
// else but spaces: no real Japanese field looks like that.
func onlyQuestionMarks(text string) bool {
	seen := false
	for _, r := range text {
		switch {
		case r == '?':
			seen = true
		case unicode.IsSpace(r):
		default:
			return false
		}
	}
	return seen
}
