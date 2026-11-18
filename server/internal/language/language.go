// Package language is the per-language capability registry. Handlers never
// hardcode a language: they
// resolve through For(), and a nil capability is a 400 naming Supported().
// Adding a language = a tokenizer/dict package under internal/ + one entry
// in registry.
package language

import (
	"strings"

	"github.com/yumejp/yume/server/internal/dict"
	"github.com/yumejp/yume/server/internal/examples"
	"github.com/yumejp/yume/server/internal/tokenize"
)

// Language bundles one language's capabilities; nil means not available.
type Language struct {
	Code     string
	Tokenize func(text string) ([]tokenize.Token, error)
	Dict     func() (*dict.Dict, error)
	Examples func(queries []string, reading string, limit int) ([]examples.Pair, error)
	// IsContent gates /lookup's deconjugation fallback (食べさせられた → 食べる
	// without させる/られる/た). Clients that pre-filter content words should
	// use the same POS set.
	IsContent func(t tokenize.Token) bool
	// Mining: transcript mining (/mine_transcript) is available. Needs a
	// tokenizer whose POS set the miner's content filter understands.
	Mining bool
}

// jaNonContentPOS lists the IPADIC POS values that are never vocabulary.
var jaNonContentPOS = map[string]bool{
	"助詞": true, "助動詞": true, "記号": true, "補助記号": true,
	"フィラー": true, "空白": true, "その他": true,
}

var registry = []*Language{
	{
		Code:     "ja",
		Mining:   true,
		Tokenize: tokenize.Japanese, // kagome v2 + IPADIC + user dictionary
		Dict:     dict.Default,      // JMdict English
		Examples: func(queries []string, reading string, limit int) ([]examples.Pair, error) {
			c, err := examples.Default() // Tanaka Corpus (examples.utf.gz)
			if err != nil {
				return nil, err
			}
			return c.LookupReading(queries, reading, limit), nil
		},
		IsContent: func(t tokenize.Token) bool {
			return !jaNonContentPOS[t.POS] && t.POS1 != "非自立" && t.POS1 != "接尾"
		},
	},
}

// For resolves a code exactly, then by prefix ("ja-Hira" → "ja"). Empty
// defaults to "ja" when omitted.
func For(code string) (*Language, bool) {
	if code == "" {
		code = "ja"
	}
	for _, l := range registry {
		if code == l.Code || strings.HasPrefix(code, l.Code+"-") {
			return l, true
		}
	}
	return nil, false
}

// Supported lists registered language codes, for error messages.
func Supported() []string {
	out := make([]string, len(registry))
	for i, l := range registry {
		out[i] = l.Code
	}
	return out
}
