// Package tokenize wraps kagome; nothing else in the server imports it.
package tokenize

import (
	"fmt"
	"os"
	"strings"
	"sync"

	kdict "github.com/ikawaha/kagome-dict/dict"
	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

// Token is the wire shape. Reading is katakana (as IPADIC stores it); Lemma
// is IPADIC's basic form, which is what JMdict keys on.
type Token struct {
	Surface string `json:"surface"`
	Reading string `json:"reading,omitempty"`
	Lemma   string `json:"lemma,omitempty"`
	POS     string `json:"pos,omitempty"`
	// POS1 is IPADIC's first subcategory; 非自立 (auxiliaries in て-chains,
	// formal nouns) must not count as vocabulary downstream.
	POS1 string `json:"pos1,omitempty"`
}

// The tokenizer is rebuilt and swapped whenever the user dictionary changes.
// User entries match literal text only (no conjugation).
var (
	jaMu         sync.RWMutex
	jaTok        *tokenizer.Tokenizer
	jaErr        error
	jaLoaded     bool
	userDictPath string // "" = none
)

func buildTokenizer() (*tokenizer.Tokenizer, error) {
	opts := []tokenizer.Option{tokenizer.OmitBosEos()}
	if userDictPath != "" {
		if _, err := os.Stat(userDictPath); err == nil {
			ud, err := kdict.NewUserDict(userDictPath)
			if err != nil {
				return nil, fmt.Errorf("user dict %s: %w", userDictPath, err)
			}
			opts = append(opts, tokenizer.UserDict(ud))
		}
	}
	return tokenizer.New(ipa.Dict(), opts...)
}

func jaTokenizer() (*tokenizer.Tokenizer, error) {
	jaMu.RLock()
	if jaLoaded {
		t, err := jaTok, jaErr
		jaMu.RUnlock()
		return t, err
	}
	jaMu.RUnlock()
	return reload()
}

func reload() (*tokenizer.Tokenizer, error) {
	jaMu.Lock()
	defer jaMu.Unlock()
	jaTok, jaErr = buildTokenizer()
	jaLoaded = true
	return jaTok, jaErr
}

// SetUserDictPath points the tokenizer at a kagome user dictionary
// (text,tokens,reading,POS); a missing file means plain IPADIC.
func SetUserDictPath(path string) error {
	jaMu.Lock()
	userDictPath = path
	alreadyLoaded := jaLoaded
	jaMu.Unlock()
	if alreadyLoaded {
		_, err := reload()
		return err
	}
	return nil
}

// ReloadUserDict rebuilds the tokenizer after the user dictionary changed.
func ReloadUserDict() error {
	_, err := reload()
	return err
}

// Japanese tokenizes text with IPADIC + the user dictionary. Empty input
// returns an empty slice, not an error.
func Japanese(text string) ([]Token, error) {
	if text == "" {
		return []Token{}, nil
	}
	tok, err := jaTokenizer()
	if err != nil {
		return nil, err
	}

	morphs := tok.Tokenize(text)
	out := make([]Token, 0, len(morphs))
	for _, m := range morphs {
		// IPADIC features: [POS, POS1, POS2, POS3, conj type, conj form, lemma,
		// reading, pronunciation]. User-dict tokens have the short vector
		// [POS, tokens, yomi]: no lemma (downstream keys on the surface, right
		// for names) but yomi must surface as the reading for furigana.
		feats := m.Features()
		var pos, pos1, lemma, reading string
		if len(feats) > 0 {
			pos = feats[0]
		}
		if len(feats) > 1 {
			pos1 = feats[1]
			if pos1 == "*" {
				pos1 = ""
			}
		}
		if len(feats) > 6 {
			lemma = feats[6]
			if lemma == "*" {
				lemma = ""
			}
		}
		if len(feats) > 7 {
			reading = feats[7]
			if reading == "*" {
				reading = ""
			}
		} else if len(feats) == 3 {
			// Multi-token entries space-separate the yomi.
			reading = strings.ReplaceAll(feats[2], " ", "")
		}
		out = append(out, Token{
			Surface: m.Surface,
			Reading: reading,
			Lemma:   lemma,
			POS:     pos,
			POS1:    pos1,
		})
	}
	return out, nil
}
