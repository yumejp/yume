package tokenize

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

// User-dictionary management. The file is kagome's 4-column format:
//
//	<text>,<space-separated tokens>,<space-separated readings>,<POS>
//
// Yume only writes single-token entries (the whole text as one word) — the
// point is to stop IPADIC from shattering modern slang and media/person
// names into garbage morphemes. Entries preempt IPADIC in the lattice and
// match literal text only (no conjugation), which fits names and the
// base-form slang that dominates casual speech.

// userDictPOS is a content POS so user words participate in coverage, i+1,
// and text mining.
const userDictPOS = "カスタム語"

var fileMu sync.Mutex // serializes file rewrites; reload handles the swap

// UserWord is one entry as exposed over the API.
type UserWord struct {
	Text    string `json:"text"`
	Reading string `json:"reading,omitempty"`
}

// AddUserWord appends an entry to the user dictionary and reloads the
// tokenizer. reading may be hiragana, katakana, or empty.
func AddUserWord(text, reading string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("text is required")
	}
	if strings.ContainsAny(text, ",\n") || strings.ContainsAny(reading, ",\n") {
		return errors.New("text/reading must not contain commas or newlines")
	}
	jaMu.RLock()
	path := userDictPath
	jaMu.RUnlock()
	if path == "" {
		return errors.New("user dictionary not configured")
	}

	words, err := ListUserWords()
	if err != nil {
		return err
	}
	for _, w := range words {
		if w.Text == text {
			return fmt.Errorf("%q is already in the dictionary", text)
		}
	}

	reading = strings.TrimSpace(reading)
	if reading == "" {
		reading = text // kagome requires the column; surface is the best default
	}
	reading = hiraToKata(reading)

	fileMu.Lock()
	defer fileMu.Unlock()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open user dict: %w", err)
	}
	_, werr := fmt.Fprintf(f, "%s,%s,%s,%s\n", text, text, reading, userDictPOS)
	cerr := f.Close()
	if werr != nil {
		return fmt.Errorf("append user dict: %w", werr)
	}
	if cerr != nil {
		return fmt.Errorf("close user dict: %w", cerr)
	}
	return ReloadUserDict()
}

// RemoveUserWord deletes an entry by its text and reloads the tokenizer.
func RemoveUserWord(text string) error {
	jaMu.RLock()
	path := userDictPath
	jaMu.RUnlock()
	if path == "" {
		return errors.New("user dictionary not configured")
	}

	fileMu.Lock()
	data, err := os.ReadFile(path)
	if err != nil {
		fileMu.Unlock()
		return fmt.Errorf("read user dict: %w", err)
	}
	var kept []string
	removed := false
	for _, line := range strings.Split(string(data), "\n") {
		if t, _, ok := parseUserDictLine(line); ok && t == text {
			removed = true
			continue
		}
		kept = append(kept, line)
	}
	if removed {
		err = os.WriteFile(path, []byte(strings.Join(kept, "\n")), 0o644)
	}
	fileMu.Unlock()
	if err != nil {
		return fmt.Errorf("rewrite user dict: %w", err)
	}
	if !removed {
		return fmt.Errorf("%q is not in the dictionary", text)
	}
	return ReloadUserDict()
}

// ListUserWords returns the current entries (comment/blank lines skipped).
// A missing file is an empty dictionary, not an error.
func ListUserWords() ([]UserWord, error) {
	jaMu.RLock()
	path := userDictPath
	jaMu.RUnlock()
	if path == "" {
		return []UserWord{}, nil
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []UserWord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open user dict: %w", err)
	}
	defer f.Close()

	out := []UserWord{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if text, reading, ok := parseUserDictLine(sc.Text()); ok {
			out = append(out, UserWord{Text: text, Reading: kataToHiraLocal(reading)})
		}
	}
	return out, sc.Err()
}

func parseUserDictLine(line string) (text, reading string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	cols := strings.Split(line, ",")
	if len(cols) != 4 {
		return "", "", false
	}
	return cols[0], cols[2], true
}

// SeedUserDictIfMissing writes a starter dictionary the first time the
// server runs — a small curated set of words IPADIC (2007 vintage) whiffs
// on: modern slang and a few mega-popular media titles. The file is meant
// to be user-edited; we never overwrite an existing one.
func SeedUserDictIfMissing(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	starter := `## Yume user dictionary — words the built-in tokenizer (IPADIC, ~2007)
## doesn't know. One entry per line:
##   <word>,<word>,<reading in katakana>,<POS label>
## Entries match literal text (no conjugation). Manage via /add_user_word,
## or edit this file and restart the server.
エモい,エモい,エモイ,カスタム語
ワンチャン,ワンチャン,ワンチャン,カスタム語
ガチ,ガチ,ガチ,カスタム語
メンヘラ,メンヘラ,メンヘラ,カスタム語
バズる,バズる,バズル,カスタム語
ググる,ググる,ググル,カスタム語
ドタキャン,ドタキャン,ドタキャン,カスタム語
コスパ,コスパ,コスパ,カスタム語
タイパ,タイパ,タイパ,カスタム語
インフルエンサー,インフルエンサー,インフルエンサー,カスタム語
サブスク,サブスク,サブスク,カスタム語
リモートワーク,リモートワーク,リモートワーク,カスタム語
推し活,推し活,オシカツ,カスタム語
`
	return os.WriteFile(path, []byte(starter), 0o644)
}

// hiraToKata converts hiragana runes to katakana (inverse of the dict
// package's KataToHira); kagome user dicts store readings as katakana.
func hiraToKata(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'ぁ' && r <= 'ゖ' {
			r += 0x60
		}
		b.WriteRune(r)
	}
	return b.String()
}

// kataToHiraLocal mirrors dict.KataToHira without importing the dict
// package (keeps tokenize dependency-free of the JMdict loader).
func kataToHiraLocal(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'ァ' && r <= 'ヶ' {
			r -= 0x60
		}
		b.WriteRune(r)
	}
	return b.String()
}
