// Command seed populates a Yume DB with realistic backdated data for visual
// QA of the progress dashboard, review screen, and vocab explorer. NOT part
// of the shipped product — a dev tool.
//
// Cards come from the curated dataset in data.go: real words with natural
// example sentences (so the review screen's sentence line, cloze cards, and
// search QA exercise realistic content instead of 語1/語2 placeholders).
//
//	YUME_DB=/tmp/seed/cards.db go run ./cmd/seed
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yumejp/yume/server/internal/srs"
	_ "modernc.org/sqlite"
)

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		log.Fatal(err)
	}
	return string(b)
}

// validate enforces the dataset's core invariant: every sentence must
// actually contain the word (its surface form, kanji form, or lemma), so the
// sentence genuinely demonstrates usage and cloze blanking always works.
func validate() {
	seen := map[string]bool{}
	for _, c := range seedCards {
		if seen[c.lemma] {
			log.Fatalf("seed data: duplicate lemma %q", c.lemma)
		}
		seen[c.lemma] = true
		if c.sentence == "" {
			log.Fatalf("seed data: %q has no sentence", c.lemma)
		}
		if c.surface != "" && !strings.Contains(c.sentence, c.surface) {
			log.Fatalf("seed data: sentence for %q does not contain surface %q: %s", c.lemma, c.surface, c.sentence)
		}
		if c.surface == "" && !strings.Contains(c.sentence, c.lemma) &&
			(c.kanji == "" || !strings.Contains(c.sentence, c.kanji)) {
			log.Fatalf("seed data: sentence for %q does not contain the word: %s", c.lemma, c.sentence)
		}
		if c.cloze && c.surface == "" {
			log.Fatalf("seed data: cloze card %q needs a surface form", c.lemma)
		}
	}
}

func main() {
	wipe := flag.Bool("wipe", false, "remove previously seeded data (everything tagged 'seed') and exit")
	flag.Parse()

	validate()

	path := os.Getenv("YUME_DB")
	if path == "" {
		log.Fatal("set YUME_DB to the target db path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}

	// Open via the store so migrations run, then drop to raw SQL for backdated
	// inserts (the API only writes "now" timestamps).
	store, err := srs.Open(path)
	if err != nil {
		log.Fatal(err)
	}

	if *wipe {
		// Seeded cards carry review state, which delete_by_tag keeps by default.
		resp, err := store.DeleteByTag(srs.DeleteByTagReq{Tag: "seed", IncludeStudied: true})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("removed %d seeded lemmas (cards + statuses + tags) from %s\n", resp.DeletedLemmas, path)
		return
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	now := time.Now()
	rng := rand.New(rand.NewSource(42))

	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}

	// Cards mined over the last 120 days, weighted toward recent.
	cardCount := 0
	for i, c := range seedCards {
		daysAgo := int(120 * (1 - rng.Float64()*rng.Float64())) // recency bias
		created := now.AddDate(0, 0, -daysAgo).Unix()
		// Due: spread some over the next 14 days, some overdue.
		due := now.AddDate(0, 0, rng.Intn(20)-3).Unix()
		cardType := "vocab"
		if c.cloze {
			cardType = "cloze"
		}
		// FSRS state must be internally consistent: a non-new card needs
		// stability > 0 and difficulty in [1,10], or go-fsrs computes NaN on
		// the next review. New cards keep all-zero state.
		reps := rng.Intn(8)
		state := 0
		stability, difficulty := 0.0, 0.0
		if reps > 0 {
			state = []int{1, 2, 2, 2}[rng.Intn(4)] // mostly Review, some Learning
			stability = 1 + rng.Float64()*29       // 1-30 days
			difficulty = 3 + rng.Float64()*4       // 3-7, FSRS mid-range
		}
		var kanjiJSON any
		if c.kanji != "" {
			kanjiJSON = mustJSON([]string{c.kanji})
		}
		var surface any
		if c.surface != "" {
			surface = c.surface
		}
		_, err := tx.Exec(`
			INSERT INTO cards (jmdict_id, lemma, kanji, readings, glosses, sentence,
			   created_at, due, stability, difficulty, elapsed_days, scheduled_days,
			   reps, lapses, state, card_type, surface)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, ?, 0, ?, ?, ?)`,
			1000+i, c.lemma,
			kanjiJSON, mustJSON([]string{c.reading}), mustJSON([][]string{c.glosses}),
			c.sentence,
			created, due, stability, difficulty, reps, state, cardType, surface,
		)
		if err != nil {
			log.Fatal(err)
		}
		// Mined cards are words the user is studying — give them a Learning
		// status so the vocab explorer rows look coherent.
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO word_status (lemma, status, updated_at) VALUES (?, 1, ?)`,
			c.lemma, created,
		); err != nil {
			log.Fatal(err)
		}
		// Everything the seed writes is tagged so `go run ./cmd/seed -wipe`
		// (or delete_by_tag("seed")) removes it in one shot.
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO word_tags (lemma, tag) VALUES (?, 'seed')`, c.lemma,
		); err != nil {
			log.Fatal(err)
		}
		cardCount++
	}

	// word_status rows from real common words: mostly known, every 8th still
	// learning, plus the ignore-list of grammar glue.
	statusCount := 0
	for i, w := range statusWords {
		status := 2 // known
		if i%8 == 3 {
			status = 1 // learning
		}
		updated := now.AddDate(0, 0, -rng.Intn(120)).Unix()
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO word_status (lemma, status, updated_at) VALUES (?, ?, ?)`,
			w, status, updated,
		); err != nil {
			log.Fatal(err)
		}
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO word_tags (lemma, tag) VALUES (?, 'seed')`, w,
		); err != nil {
			log.Fatal(err)
		}
		statusCount++
	}
	for _, w := range ignoredWords {
		updated := now.AddDate(0, 0, -rng.Intn(120)).Unix()
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO word_status (lemma, status, updated_at) VALUES (?, 3, ?)`,
			w, updated,
		); err != nil {
			log.Fatal(err)
		}
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO word_tags (lemma, tag) VALUES (?, 'seed')`, w,
		); err != nil {
			log.Fatal(err)
		}
		statusCount++
	}

	// Reviews over 120 days, clustered into active days (so the heatmap +
	// streak look realistic — some days heavy, ~30% of days skipped).
	reviewCount := 0
	for daysAgo := 120; daysAgo >= 0; daysAgo-- {
		if rng.Float64() < 0.28 {
			continue // a rest day
		}
		// Heavier near the present.
		base := 10 + int(40*(1-float64(daysAgo)/120))
		n := base/2 + rng.Intn(base+1)
		day := now.AddDate(0, 0, -daysAgo)
		for j := 0; j < n; j++ {
			// Spread within the day.
			ts := day.Add(time.Duration(rng.Intn(16)) * time.Hour).Unix()
			_, err := tx.Exec(
				`INSERT INTO review_log (card_id, rating, reviewed_at) VALUES (?, ?, ?)`,
				1+rng.Intn(cardCount), 1+rng.Intn(4), ts,
			)
			if err != nil {
				log.Fatal(err)
			}
			reviewCount++
		}
	}

	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("seeded %d cards (real vocab + sentences), %d word_status, %d reviews into %s\n",
		cardCount, statusCount, reviewCount, path)
	fmt.Println("everything is tagged 'seed' — remove with: go run ./cmd/seed -wipe")
}
