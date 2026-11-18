// parsebench — boundary-accuracy benchmark for Yume's tokenizer.
//
// Scores word-boundary precision/recall/F1 against the gold-segmented
// UD Japanese-GSD test set (CC BY-SA 4.0):
//
//	curl -L -o data/ja_gsd-ud-test.conllu \
//	  https://raw.githubusercontent.com/UniversalDependencies/UD_Japanese-GSD/master/ja_gsd-ud-test.conllu
//	go run ./cmd/parsebench [-conllu path] [-diff N]
//
// Systems compared:
//   - kagome+IPADIC — what Yume ships (lattice Viterbi, real morphology)
//   - kagome+UniDic — benchmark-only modern dictionary
//   - TinySegmenter — lightweight pointwise tokenizer baseline
//
// Reference results (UD-GSD test, 543 sentences):
//   IPADIC 96.95 F1 · UniDic 98.59 F1 (R 99.79!) · TinySegmenter 94.48 F1
//
// CAVEAT on absolute numbers: UD-GSD uses UniDic Short-Unit-Word
// segmentation conventions; IPADIC's conventions differ systematically on
// some compounds, so part of each system's "error" rate is convention
// mismatch, not mistakes. The RELATIVE gap between systems on the same gold
// is the meaningful signal. The -diff dump identifies cases worth inspecting.
//
// (Yume's display-level word GROUPING intentionally diverges from SUW — it
// merges inflection chains — so it is NOT scored here.)
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ikawaha/kagome-dict/uni"
	kagome "github.com/ikawaha/kagome/v2/tokenizer"
	tiny "github.com/ikawaha/tinysegmenter.go"
	"github.com/yumejp/yume/server/internal/tokenize"
)

type sentence struct {
	text string
	gold []string
}

func main() {
	conllu := flag.String("conllu", "data/ja_gsd-ud-test.conllu", "path to a CoNLL-U file with gold segmentation")
	diffN := flag.Int("diff", 5, "show the N worst kagome mismatches (0 = none)")
	flag.Parse()

	sents, err := readConllu(*conllu)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n(download: see comment at top of cmd/parsebench/main.go)\n", *conllu, err)
		os.Exit(1)
	}
	fmt.Printf("gold: %d sentences from %s\n\n", len(sents), *conllu)

	kagomeSeg := func(text string) []string {
		toks, err := tokenize.Japanese(text)
		if err != nil {
			return nil
		}
		out := make([]string, 0, len(toks))
		for _, t := range toks {
			out = append(out, t.Surface)
		}
		return out
	}
	tinySeg := func(text string) []string { return tiny.Segment(text) }

	// UniDic shares the UD gold set's short-unit-word segmentation conventions,
	// so its score also benefits from convention alignment. It is included only
	// in this benchmark binary.
	uniTok, err := kagome.New(uni.Dict(), kagome.OmitBosEos())
	if err != nil {
		fmt.Fprintf(os.Stderr, "unidic init: %v\n", err)
		os.Exit(1)
	}
	uniSeg := func(text string) []string {
		toks := uniTok.Tokenize(text)
		out := make([]string, 0, len(toks))
		for _, t := range toks {
			out = append(out, t.Surface)
		}
		return out
	}

	type scored struct {
		name string
		f1   float64
	}
	var results []scored
	for _, sys := range []struct {
		name string
		seg  func(string) []string
	}{
		{"kagome+IPADIC (Yume ships this)", kagomeSeg},
		{"kagome+UniDic (benchmark)", uniSeg},
		{"TinySegmenter (browser-JS class)", tinySeg},
	} {
		p, r, f1, worst := evaluate(sents, sys.seg)
		fmt.Printf("%-34s boundary P %.2f%%  R %.2f%%  F1 %.2f%%\n", sys.name, p*100, r*100, f1*100)
		results = append(results, scored{sys.name, f1})
		if strings.HasPrefix(sys.name, "kagome+IPADIC") && *diffN > 0 {
			fmt.Printf("\n  worst kagome mismatches (gold ≠ system — many are SUW-convention diffs):\n")
			for i, w := range worst {
				if i >= *diffN {
					break
				}
				fmt.Printf("  · %s\n    gold:   %s\n    kagome: %s\n", w.text, strings.Join(w.gold, "|"), strings.Join(w.sys, "|"))
			}
			fmt.Println()
		}
	}
	if len(results) >= 2 {
		base := results[0].f1
		fmt.Println()
		for _, r := range results[1:] {
			fmt.Printf("F1 delta vs shipped IPADIC: %-36s %+.2f points\n", r.name, (r.f1-base)*100)
		}
	}
}

type mismatch struct {
	text      string
	gold, sys []string
	f1        float64
}

// evaluate scores boundary positions (character offsets where a token ends)
// — the standard segmentation metric, insensitive to differing token counts.
// Whitespace tokens are dropped from system output before scoring: UD gold
// omits the spaces between Latin tokens, so offsets are over non-space runes.
func evaluate(sents []sentence, seg func(string) []string) (p, r, f1 float64, worst []mismatch) {
	var tp, fp, fn int
	for _, s := range sents {
		sysToks := dropSpaces(seg(s.text))
		g := boundarySet(s.gold)
		y := boundarySet(sysToks)
		var stp, sfp, sfn int
		for b := range y {
			if g[b] {
				stp++
			} else {
				sfp++
			}
		}
		for b := range g {
			if !y[b] {
				sfn++
			}
		}
		tp += stp
		fp += sfp
		fn += sfn
		if sfp+sfn > 0 {
			sf1 := f1Of(stp, sfp, sfn)
			worst = append(worst, mismatch{s.text, s.gold, sysToks, sf1})
		}
	}
	// Sort worst ascending by per-sentence F1 (simple insertion — list is small).
	for i := 1; i < len(worst); i++ {
		for j := i; j > 0 && worst[j].f1 < worst[j-1].f1; j-- {
			worst[j], worst[j-1] = worst[j-1], worst[j]
		}
	}
	pre := float64(tp) / float64(tp+fp)
	rec := float64(tp) / float64(tp+fn)
	return pre, rec, 2 * pre * rec / (pre + rec), worst
}

func f1Of(tp, fp, fn int) float64 {
	if tp == 0 {
		return 0
	}
	p := float64(tp) / float64(tp+fp)
	r := float64(tp) / float64(tp+fn)
	return 2 * p * r / (p + r)
}

// dropSpaces removes whitespace-only tokens and strips spaces inside tokens,
// so boundary offsets are over non-space runes (matching the gold).
func dropSpaces(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		t = strings.ReplaceAll(t, " ", "")
		t = strings.ReplaceAll(t, "　", "")
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// boundarySet returns the set of rune offsets at which a token ENDS
// (excluding the final offset, which every segmentation shares).
func boundarySet(tokens []string) map[int]bool {
	out := map[int]bool{}
	pos := 0
	for i, t := range tokens {
		pos += len([]rune(t))
		if i < len(tokens)-1 {
			out[pos] = true
		}
	}
	return out
}

// readConllu extracts (# text, FORM sequence) pairs. Skips multiword-token
// ranges (IDs like 1-2) and empty nodes (1.1); only sentences whose forms
// concatenate exactly to the text (ignoring spaces) are kept, so boundary
// offsets line up.
func readConllu(path string) ([]sentence, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []sentence
	var cur sentence
	flush := func() {
		if cur.text != "" && len(cur.gold) > 0 &&
			strings.ReplaceAll(cur.text, " ", "") == strings.Join(cur.gold, "") {
			out = append(out, cur)
		}
		cur = sentence{}
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "# text = "):
			cur.text = strings.TrimPrefix(line, "# text = ")
		case line == "":
			flush()
		case strings.HasPrefix(line, "#"):
			// other comments
		default:
			cols := strings.Split(line, "\t")
			if len(cols) < 2 || strings.ContainsAny(cols[0], "-.") {
				continue
			}
			cur.gold = append(cur.gold, cols[1])
		}
	}
	flush()
	return out, sc.Err()
}
