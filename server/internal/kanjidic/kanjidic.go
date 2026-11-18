// Package kanjidic loads EDRDG's KANJIDIC2 (kanjidic2.xml.gz) into memory:
// English meanings, on/kun readings, grade, JLPT level and stroke count per
// kanji. Loaded lazily on first use, like the examples corpus.
package kanjidic

import (
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Entry struct {
	Literal  string   `json:"kanji"`
	Meanings []string `json:"meanings,omitempty"`
	On       []string `json:"on,omitempty"`
	Kun      []string `json:"kun,omitempty"`
	Grade    int      `json:"grade,omitempty"`
	Strokes  int      `json:"strokes,omitempty"`
	JLPT     int      `json:"jlpt,omitempty"`
	Freq     int      `json:"freq,omitempty"`
}

type Dict struct {
	entries map[string]*Entry
}

func (d *Dict) Lookup(kanji string) *Entry {
	if d == nil {
		return nil
	}
	return d.entries[kanji]
}

func (d *Dict) Len() int {
	if d == nil {
		return 0
	}
	return len(d.entries)
}

var (
	defaultOnce sync.Once
	defaultDict *Dict
	defaultErr  error
)

// Default loads the dictionary on first use and caches it for the process
// lifetime. Missing file = (nil, err); callers treat that as "no kanji data".
func Default() (*Dict, error) {
	defaultOnce.Do(func() {
		path, err := findFile()
		if err != nil {
			defaultErr = err
			return
		}
		defaultDict, defaultErr = Load(path)
	})
	return defaultDict, defaultErr
}

func findFile() (string, error) {
	candidates := []string{}
	if env := os.Getenv("YUME_KANJIDIC"); env != "" {
		candidates = append(candidates, env)
	}
	candidates = append(candidates, filepath.Join("data", "kanjidic2.xml.gz"))
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "data", "kanjidic2.xml.gz"))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("kanjidic2.xml.gz not found. Run `yume-server setup`, set $YUME_KANJIDIC or place the file in ./data/; download from https://www.edrdg.org/kanjidic/kanjidic2.xml.gz")
}

func Load(path string) (*Dict, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var r io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		r = gz
	}
	return Parse(r)
}

// character mirrors the parts of a KANJIDIC2 <character> element we keep.
type character struct {
	Literal string `xml:"literal"`
	Misc    struct {
		Grade   int   `xml:"grade"`
		Strokes []int `xml:"stroke_count"` // first is the accepted count
		JLPT    int   `xml:"jlpt"`
		Freq    int   `xml:"freq"`
	} `xml:"misc"`
	RM struct {
		Groups []struct {
			Readings []struct {
				Type string `xml:"r_type,attr"`
				Text string `xml:",chardata"`
			} `xml:"reading"`
			Meanings []struct {
				Lang string `xml:"m_lang,attr"`
				Text string `xml:",chardata"`
			} `xml:"meaning"`
		} `xml:"rmgroup"`
	} `xml:"reading_meaning"`
}

// KANJIDIC2 keeps a few period glosses whose modern reading is a slur;
// say what they mean instead.
var plainMeanings = map[string]string{
	"faggot": "bundle of sticks",
}

func plainMeaning(m string) string {
	if p, ok := plainMeanings[m]; ok {
		return p
	}
	return m
}

// Parse streams <character> elements; the DTD's entities are irrelevant to the
// fields we read, so the decoder runs in non-strict mode.
func Parse(r io.Reader) (*Dict, error) {
	dec := xml.NewDecoder(r)
	dec.Strict = false
	d := &Dict{entries: make(map[string]*Entry, 13500)}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("kanjidic parse: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "character" {
			continue
		}
		var c character
		if err := dec.DecodeElement(&c, &se); err != nil {
			return nil, fmt.Errorf("kanjidic character: %w", err)
		}
		e := &Entry{Literal: c.Literal, Grade: c.Misc.Grade, JLPT: c.Misc.JLPT, Freq: c.Misc.Freq}
		if len(c.Misc.Strokes) > 0 {
			e.Strokes = c.Misc.Strokes[0]
		}
		for _, g := range c.RM.Groups {
			for _, rd := range g.Readings {
				switch rd.Type {
				case "ja_on":
					e.On = append(e.On, rd.Text)
				case "ja_kun":
					e.Kun = append(e.Kun, rd.Text)
				}
			}
			for _, m := range g.Meanings {
				if m.Lang == "" {
					e.Meanings = append(e.Meanings, plainMeaning(m.Text))
				}
			}
		}
		d.entries[c.Literal] = e
	}
	return d, nil
}
