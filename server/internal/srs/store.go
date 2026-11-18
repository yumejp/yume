// Package srs is the persistence layer — cards, FSRS state, word status,
// tags — over SQLite via modernc.org/sqlite (pure Go, no CGO).
//
//   - One *sql.DB per Store; database/sql pools connections, so pragmas are
//     applied through a driver connection hook (see registerPragmaHook).
//   - Multi-row writes run inside transactions.
//   - FSRS state is stored inline on each cards row.
package srs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v3"
	"modernc.org/sqlite"
)

// SQLite pragmas are per-connection and database/sql pools connections, so
// db.Exec("PRAGMA …") would configure only one of them; the driver hook runs
// on every connection the pool opens.
var pragmaHookOnce sync.Once

func registerPragmaHook() {
	pragmaHookOnce.Do(func() {
		sqlite.RegisterConnectionHook(func(conn sqlite.ExecQuerierContext, _ string) error {
			_, err := conn.ExecContext(
				context.Background(),
				"PRAGMA journal_mode = WAL; PRAGMA synchronous = NORMAL; PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;",
				nil,
			)
			return err
		})
	})
}

// Store is the entry point — everything else hangs off this.
type Store struct {
	*storeCore
	// lang scopes every lemma-keyed read and write (word statuses, tags,
	// suggestions, kanji stories, cards, dictionaries). One deck file holds
	// all languages; WithLang returns a view of the same file for another.
	lang string
}

// storeCore is the per-file state shared by every language view of a deck.
type storeCore struct {
	// Atomic because backup restore swaps the handle while requests read it.
	// Access via q(); swap via setDB.
	dbPtr atomic.Pointer[sql.DB]
	fsr   *fsrs.FSRS
	tune  Tuning
	path  string // absolute path to the SQLite file; backups + media dir derive from it
	// backupMu serializes this store's backup/restore (they swap the DB
	// handle). Per store, so one user's restore never blocks another's backup.
	backupMu sync.Mutex
}

// DefaultLang is the language a request means when it names none.
const DefaultLang = "ja"

// normLang lowercases and defaults a language code ("" → ja).
func normLang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		return DefaultLang
	}
	return lang
}

// Lang is the language this view of the deck is scoped to.
func (s *Store) Lang() string { return s.lang }

// WithLang returns the same deck scoped to another language. Cheap: the file
// handle, scheduler and locks are shared.
func (s *Store) WithLang(lang string) *Store {
	lang = normLang(lang)
	if lang == s.lang {
		return s
	}
	return &Store{storeCore: s.storeCore, lang: lang}
}

// Scheduler tuning is server-wide (one deployment, many per-user stores),
// loaded once from $YUME_SRS_TUNING, else tuningDefault. A malformed file
// fails Open.
var (
	tuningOnce    sync.Once
	tuningVal     Tuning
	tuningErr     error
	tuningDefault = filepath.Join("data", "srs-tuning.json")
)

// SetTuningDefault sets where srs-tuning.json is read when $YUME_SRS_TUNING
// is unset. The server points it into the data folder (TuningPathIn) before
// the first Open; without a call it stays relative to the working directory.
func SetTuningDefault(path string) { tuningDefault = path }

// TuningPathIn is where the data folder root keeps srs-tuning.json. Builds
// before this one read ./data/srs-tuning.json from the working directory, so
// when only that older file exists it is still used and legacy reports it.
func TuningPathIn(root string) (path string, legacy bool) {
	path = filepath.Join(root, "srs-tuning.json")
	if _, err := os.Stat(path); err == nil {
		return path, false
	}
	old := filepath.Join("data", "srs-tuning.json")
	if _, err := os.Stat(old); err != nil {
		return path, false
	}
	return old, true
}

func loadTuningOnce() (Tuning, error) {
	tuningOnce.Do(func() {
		path := os.Getenv("YUME_SRS_TUNING")
		if path == "" {
			path = tuningDefault
		} else if _, err := os.Stat(path); os.IsNotExist(err) {
			log.Printf("srs tuning: YUME_SRS_TUNING names %s, which does not exist; using the defaults", path)
		}
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		tuningVal, tuningErr = LoadTuning(path)
		if tuningErr == nil {
			weights := "default"
			if len(tuningVal.Weights) > 0 {
				weights = "fitted"
			}
			log.Printf("srs tuning: weights=%s retention=%.2f nonAgainFloor=%.0fh demote=%d/%.0fd matureLapseFloor=%.2f@%.0fd (%s)",
				weights, tuningVal.RequestRetention, tuningVal.MinNonAgainIntervalHours,
				tuningVal.DemoteAfterLapses, tuningVal.DemoteLapseWindowDays,
				tuningVal.MatureLapseFloor, tuningVal.MatureLapseThresholdDays, path)
		}
	})
	return tuningVal, tuningErr
}

// q returns the live DB handle; grab it once per operation.
func (s *Store) q() *sql.DB { return s.dbPtr.Load() }

// setDB swaps the live handle (backup restore only).
func (s *Store) setDB(db *sql.DB) { s.dbPtr.Store(db) }

// DataDir holds the DB file, media sidecars and backups.
func (s *Store) DataDir() string {
	return filepath.Dir(s.path)
}

// MediaDir holds audio/image sidecars; created lazily by writers.
func (s *Store) MediaDir() string {
	return filepath.Join(s.DataDir(), "media")
}

var (
	defaultStore *Store
	defaultOnce  sync.Once
	defaultErr   error
)

// Default returns the process-wide Store, opening the DB on first call; the
// path comes from resolveDBPath.
func Default() (*Store, error) {
	defaultOnce.Do(func() {
		path, err := resolveDBPath()
		if err != nil {
			defaultErr = err
			return
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			defaultErr = fmt.Errorf("mkdir for db: %w", err)
			return
		}
		defaultStore, defaultErr = Open(path)
	})
	return defaultStore, defaultErr
}

func resolveDBPath() (string, error) {
	if p := os.Getenv("YUME_DB"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	switch goos := osHint(); goos {
	case "windows":
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			return filepath.Join(local, "Yume", "cards.db"), nil
		}
		return filepath.Join(home, "AppData", "Local", "Yume", "cards.db"), nil
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Yume", "cards.db"), nil
	default:
		return filepath.Join(home, ".local", "share", "yume", "cards.db"), nil
	}
}

// osHint prefers $GOOS so tests can override the platform.
func osHint() string {
	if v := os.Getenv("GOOS"); v != "" {
		return v
	}
	if _, err := os.Stat("C:\\"); err == nil {
		return "windows"
	}
	if _, err := os.Stat("/Applications"); err == nil {
		return "darwin"
	}
	return "linux"
}

// openDB opens + migrates the SQLite file. Shared by Open and backup restore.
func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Pragmas are applied per connection by the hook rather than by pinning
	// SetMaxOpenConns(1); no store method queries while iterating rows, so
	// a single connection would not deadlock either.
	registerPragmaHook()
	// A per-user store lives for the process; its connections must not. Idle
	// ones close after a few minutes, so a hosted server with many users
	// who signed in once does not hold a file handle per user forever.
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(5 * time.Minute)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pragma probe: %w", err)
	}
	if err := migrate(db, path); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// Open initializes a Store at path, creating the file + schema if needed.
func Open(path string) (*Store, error) {
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}
	tune, err := loadTuningOnce()
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	params := fsrs.DefaultParam()
	params.RequestRetention = tune.RequestRetention
	if len(tune.Weights) == len(params.W) {
		copy(params.W[:], tune.Weights)
	}
	st := &Store{storeCore: &storeCore{
		fsr:  fsrs.NewFSRS(params),
		tune: tune,
		path: path,
	}, lang: DefaultLang}
	st.setDB(db)
	return st, nil
}

// Close shuts down the DB. Idempotent.
func (s *Store) Close() error {
	if s == nil || s.q() == nil {
		return nil
	}
	return s.q().Close()
}

// --- Cards ----------------------------------------------------------------

// SaveCardReq mirrors the wire format. Glosses is per-sense ([][]string).
type SaveCardReq struct {
	JMdictID    *int       `json:"jmdict_id,omitempty"`
	Lemma       string     `json:"lemma"`
	Kanji       []string   `json:"kanji,omitempty"`
	Readings    []string   `json:"readings,omitempty"`
	Glosses     [][]string `json:"glosses,omitempty"`
	Sentence    string     `json:"sentence,omitempty"`
	Translation string     `json:"translation,omitempty"` // English of the sentence
	Explanation string     `json:"explanation,omitempty"` // tricky words only
	SourceURL   string     `json:"source_url,omitempty"`
	CardType    string     `json:"card_type,omitempty"` // "vocab" (default) or "cloze"
	Surface     string     `json:"surface,omitempty"`
	// Written to <dataDir>/media/<card_id>-{audio,image}.<ext> after insert.
	Audio *MediaBlob `json:"audio,omitempty"`
	Image *MediaBlob `json:"image,omitempty"`
	// AudioSource tags what produced the audio (e.g. "voicevox:zundamon").
	AudioSource string `json:"audio_source,omitempty"`
}

// SaveCardResp: Created is false when the card already existed (jmdict_id
// dedup, or an imported card for the same lemma that now carries the id).
type SaveCardResp struct {
	ID      int64 `json:"id"`
	Created bool  `json:"created"`
}

// SaveCard inserts a card (FSRS New, due now) or returns the existing id
// for its jmdict_id.
func (s *Store) SaveCard(req SaveCardReq) (SaveCardResp, error) {
	if req.Lemma == "" {
		return SaveCardResp{}, clientErrf("lemma is required")
	}
	if err := checkCardText("", req.Lemma, req.Kanji, req.Readings, req.Sentence); err != nil {
		return SaveCardResp{}, err
	}
	if err := checkText("surface", req.Surface); err != nil {
		return SaveCardResp{}, err
	}
	// Degraded per-character tokenization can try to save "？" or "…".
	if isJunkLemma(req.Lemma) {
		return SaveCardResp{}, clientErrf("lemma %q is punctuation, not a word", req.Lemma)
	}
	cardType := req.CardType
	if cardType == "" {
		cardType = "vocab"
	}
	if req.JMdictID != nil {
		var existing int64
		err := s.q().QueryRow(
			`SELECT id FROM cards WHERE jmdict_id = ? LIMIT 1`,
			*req.JMdictID,
		).Scan(&existing)
		if err == nil {
			return SaveCardResp{ID: existing, Created: false}, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return SaveCardResp{}, fmt.Errorf("dedup check: %w", err)
		}
		// Imported cards carry no jmdict_id. Give the word's imported card
		// this id instead of adding a second card for the same word, and
		// fill in what it lacks from this save (a mined sentence or clip
		// would otherwise be dropped). The sentence's translation only
		// comes along with the sentence.
		err = s.q().QueryRow(
			`SELECT id FROM cards WHERE lang = ? AND lemma = ? AND jmdict_id IS NULL AND card_type = ? ORDER BY id LIMIT 1`,
			s.lang, req.Lemma, cardType,
		).Scan(&existing)
		if err == nil {
			if _, err := s.q().Exec(`
				UPDATE cards SET
				  jmdict_id   = ?,
				  kanji       = COALESCE(kanji, ?),
				  readings    = COALESCE(readings, ?),
				  glosses     = COALESCE(glosses, ?),
				  translation = CASE WHEN sentence IS NULL THEN COALESCE(?, translation) ELSE translation END,
				  sentence    = COALESCE(sentence, ?),
				  source_url  = COALESCE(source_url, ?),
				  explanation = COALESCE(explanation, ?),
				  surface     = COALESCE(surface, ?)
				WHERE id = ?`,
				*req.JMdictID,
				jsonOrNil(req.Kanji), jsonOrNil(req.Readings), jsonOrNil(req.Glosses),
				nullableStr(strings.TrimSpace(req.Translation)), nullableStr(req.Sentence), nullableStr(req.SourceURL),
				nullableStr(strings.TrimSpace(req.Explanation)), nullableStr(req.Surface),
				existing,
			); err != nil {
				return SaveCardResp{}, fmt.Errorf("backfill jmdict_id: %w", err)
			}
			// Added beside any clip the card already has; it becomes the
			// primary only when the card has none.
			if req.Audio != nil && req.Audio.Data != "" {
				if _, err := s.AddCardMedia(existing, "audio", "", req.AudioSource, "", req.Audio, false); err != nil {
					return SaveCardResp{ID: existing, Created: false}, fmt.Errorf("card saved but audio failed: %w", err)
				}
			}
			if req.Image != nil && req.Image.Data != "" {
				if _, err := s.AddCardMedia(existing, "image", "", "", "", req.Image, false); err != nil {
					return SaveCardResp{ID: existing, Created: false}, fmt.Errorf("card saved but image failed: %w", err)
				}
			}
			return SaveCardResp{ID: existing, Created: false}, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return SaveCardResp{}, fmt.Errorf("dedup check: %w", err)
		}
	}

	now := time.Now().Unix()

	res, err := s.q().Exec(`
		INSERT INTO cards
		  (jmdict_id, lang, lemma, kanji, readings, glosses, sentence, source_url,
		   translation, explanation,
		   created_at, due, stability, difficulty, elapsed_days, scheduled_days,
		   reps, lapses, state, last_review, card_type, surface)
		VALUES
		  (?, ?, ?, ?, ?, ?, ?, ?,
		   ?, ?,
		   ?, ?, 0, 0, 0, 0,
		   0, 0, 0, NULL, ?, ?)
	`,
		nullableInt(req.JMdictID), s.lang, req.Lemma,
		jsonOrNil(req.Kanji), jsonOrNil(req.Readings), jsonOrNil(req.Glosses),
		nullableStr(req.Sentence), nullableStr(req.SourceURL),
		nullableStr(strings.TrimSpace(req.Translation)), nullableStr(strings.TrimSpace(req.Explanation)),
		now, now,
		cardType, nullableStr(req.Surface),
	)
	if err != nil {
		return SaveCardResp{}, fmt.Errorf("insert card: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return SaveCardResp{}, fmt.Errorf("last id: %w", err)
	}

	// A media failure returns the saved id alongside the error.
	if req.Audio != nil {
		if _, err := s.AddCardMedia(id, "audio", "", req.AudioSource, "", req.Audio, true); err != nil {
			return SaveCardResp{ID: id, Created: true}, fmt.Errorf("card saved but audio failed: %w", err)
		}
	}
	if req.Image != nil {
		if _, err := s.AddCardMedia(id, "image", "", "", "", req.Image, true); err != nil {
			return SaveCardResp{ID: id, Created: true}, fmt.Errorf("card saved but image failed: %w", err)
		}
	}
	return SaveCardResp{ID: id, Created: true}, nil
}

// CardStatusRow: card_id is null for unsaved jmdict_ids.
type CardStatusRow struct {
	JMdictID int    `json:"jmdict_id"`
	CardID   *int64 `json:"card_id"`
}

func (s *Store) CardStatus(jmdictIDs []int) ([]CardStatusRow, error) {
	out := make([]CardStatusRow, 0, len(jmdictIDs))
	if len(jmdictIDs) == 0 {
		return out, nil
	}
	// SQLite's default IN-param limit is 999; clients send a handful.
	placeholders := strings.Repeat("?,", len(jmdictIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(jmdictIDs))
	for i, id := range jmdictIDs {
		args[i] = id
	}
	rows, err := s.q().Query(
		fmt.Sprintf(`SELECT jmdict_id, id FROM cards WHERE jmdict_id IN (%s)`, placeholders),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("query card_status: %w", err)
	}
	defer rows.Close()
	found := make(map[int]int64)
	for rows.Next() {
		var jid int
		var cid int64
		if err := rows.Scan(&jid, &cid); err != nil {
			return nil, fmt.Errorf("scan card_status: %w", err)
		}
		found[jid] = cid
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range jmdictIDs {
		row := CardStatusRow{JMdictID: id}
		if cid, ok := found[id]; ok {
			row.CardID = &cid
		}
		out = append(out, row)
	}
	return out, nil
}

// LemmaStatusRow reports whether a lemma has ANY card regardless of
// jmdict_id — imported cards carry none, so an id-only check misses them.
type LemmaStatusRow struct {
	Lemma  string `json:"lemma"`
	CardID *int64 `json:"card_id"`
}

// CardStatusByLemma returns, in input order, the lowest card id per lemma.
func (s *Store) CardStatusByLemma(lemmas []string) ([]LemmaStatusRow, error) {
	out := make([]LemmaStatusRow, 0, len(lemmas))
	if len(lemmas) == 0 {
		return out, nil
	}
	placeholders := strings.Repeat("?,", len(lemmas))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(lemmas))
	for i, l := range lemmas {
		args[i] = l
	}
	rows, err := s.q().Query(
		fmt.Sprintf(`SELECT lemma, MIN(id) FROM cards WHERE lang = ? AND lemma IN (%s) GROUP BY lemma`, placeholders),
		append([]any{s.lang}, args...)...,
	)
	if err != nil {
		return nil, fmt.Errorf("query lemma status: %w", err)
	}
	defer rows.Close()
	found := make(map[string]int64)
	for rows.Next() {
		var lemma string
		var cid int64
		if err := rows.Scan(&lemma, &cid); err != nil {
			return nil, fmt.Errorf("scan lemma status: %w", err)
		}
		found[lemma] = cid
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, l := range lemmas {
		row := LemmaStatusRow{Lemma: l}
		if cid, ok := found[l]; ok {
			row.CardID = &cid
		}
		out = append(out, row)
	}
	return out, nil
}

// DeckStats mirrors /deck_stats — total cards + due now.
type DeckStats struct {
	Total  int    `json:"total"`
	Due    int    `json:"due"`
	DBPath string `json:"db_path"`
	// Due == DueReviews (state != New, never gated) + DueNew (released only
	// within the daily new-card cap). Clients should display DueReviews +
	// min(DueNew, remaining allowance); raw Due overcounts once the cap is spent.
	DueReviews int `json:"due_reviews"`
	DueNew     int `json:"due_new"`
	// DueIntraday: of DueReviews, cards last reviewed today and already re-due
	// (today's Again churn). A reviews-before-new gate must subtract them or
	// a few Again ratings lock the day's new words away.
	DueIntraday int `json:"due_intraday"`
	// Client computes "available today" = max(0, newPerDay - NewIntroducedToday).
	NewTotal           int `json:"new_total"`
	NewIntroducedToday int `json:"new_introduced_today"`
}

func (s *Store) DeckStats() (DeckStats, error) {
	return s.DeckStatsTZ(nil)
}

// DeckStatsTZ is DeckStats with today's boundary in the client's zone (see
// dayStart), so the quota display matches what /get_due_cards will gate.
func (s *Store) DeckStatsTZ(tzOffsetMin *int) (DeckStats, error) {
	var ds DeckStats
	if err := s.q().QueryRow(`SELECT COUNT(*) FROM cards WHERE lang = ?`, s.lang).Scan(&ds.Total); err != nil {
		return ds, fmt.Errorf("count cards: %w", err)
	}
	cut := s.dueCutAt(clientLocation(tzOffsetMin))
	if err := s.q().QueryRow(`
		SELECT COUNT(*),
		       COALESCE(SUM(state != 0), 0),
		       COALESCE(SUM(state = 0), 0),
		       COALESCE(SUM(state != 0 AND last_review >= ?), 0)
		FROM cards WHERE lang = ? AND `+dueSQL,
		dayStart(time.Now(), tzOffsetMin), s.lang, cut.dayEnd, cut.now,
	).Scan(&ds.Due, &ds.DueReviews, &ds.DueNew, &ds.DueIntraday); err != nil {
		return ds, fmt.Errorf("count due: %w", err)
	}
	if err := s.q().QueryRow(`SELECT COUNT(*) FROM cards WHERE lang = ? AND state = 0`, s.lang).Scan(&ds.NewTotal); err != nil {
		return ds, fmt.Errorf("count new: %w", err)
	}
	introduced, err := s.newCardsIntroducedToday(time.Now(), tzOffsetMin)
	if err != nil {
		return ds, fmt.Errorf("count introduced today: %w", err)
	}
	ds.NewIntroducedToday = introduced
	// Per-user stores live at users/<id>/cards.db; report this store's path,
	// not the global one.
	ds.DBPath = s.path
	return ds, nil
}

func isJunkLemma(lemma string) bool {
	for _, r := range lemma {
		if !unicode.IsPunct(r) && !unicode.IsSymbol(r) && !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// DeleteCard removes a card, its notes (FK cascade) and media sidecars. The
// lemma's word_status survives — deleting a flashcard doesn't un-know a word.
func (s *Store) DeleteCard(cardID int64) (bool, error) {
	audio, image := s.mediaFilesFor(cardID)
	files := []string{audio, image}
	// card_media rows cascade; their files don't — collect them before DELETE.
	if rows, err := s.q().Query(`SELECT file FROM card_media WHERE card_id = ?`, cardID); err == nil {
		for rows.Next() {
			var f string
			if rows.Scan(&f) == nil {
				files = append(files, f)
			}
		}
		rows.Close()
	}
	res, err := s.q().Exec(`DELETE FROM cards WHERE id = ?`, cardID)
	if err != nil {
		return false, fmt.Errorf("delete card: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return false, nil
	}
	for _, f := range files {
		if f != "" {
			_ = os.Remove(filepath.Join(s.MediaDir(), filepath.Base(f)))
		}
	}
	return true, nil
}

// --- Word status ----------------------------------------------------------

// WordStatusRow mirrors /get_word_statuses.
type WordStatusRow struct {
	Lemma     string `json:"lemma"`
	Status    int    `json:"status"`
	UpdatedAt int64  `json:"updated_at"`
}

// SetWordStatus writes or clears a lemma's status; status=0 deletes the row
// (unknown is implicit by absence).
func (s *Store) SetWordStatus(lemma string, status int) error {
	if lemma == "" {
		return clientErrf("lemma is required")
	}
	if status == 0 {
		_, err := s.q().Exec(`DELETE FROM word_status WHERE lang = ? AND lemma = ?`, s.lang, lemma)
		return err
	}
	if status < 1 || status > 3 {
		return clientErrf("invalid status %d (expected 0-3)", status)
	}
	_, err := s.q().Exec(`
		INSERT INTO word_status (lang, lemma, status, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(lang, lemma) DO UPDATE SET status=excluded.status, updated_at=excluded.updated_at
	`, s.lang, lemma, status, time.Now().Unix())
	if err == nil && (status == 2 || status == 3) {
		// Known/Ignored retires any AI-tutor suggestion for the lemma.
		s.retireWatchWord(lemma)
	}
	return err
}

func (s *Store) GetWordStatuses(lemmas []string) ([]WordStatusRow, error) {
	out := make([]WordStatusRow, 0, len(lemmas))
	if len(lemmas) == 0 {
		return out, nil
	}
	placeholders := strings.Repeat("?,", len(lemmas))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, 0, len(lemmas)+1)
	args = append(args, s.lang)
	for _, l := range lemmas {
		args = append(args, l)
	}
	rows, err := s.q().Query(
		fmt.Sprintf(`SELECT lemma, status, updated_at FROM word_status WHERE lang = ? AND lemma IN (%s)`, placeholders),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("query word_status: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var r WordStatusRow
		if err := rows.Scan(&r.Lemma, &r.Status, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan word_status: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// WordStatusCounts mirrors /word_status_counts.
type WordStatusCounts struct {
	Learning int `json:"learning"`
	Known    int `json:"known"`
	Ignored  int `json:"ignored"`
}

func (s *Store) WordStatusCounts() (WordStatusCounts, error) {
	var c WordStatusCounts
	rows, err := s.q().Query(`SELECT status, COUNT(*) FROM word_status WHERE lang = ? GROUP BY status`, s.lang)
	if err != nil {
		return c, fmt.Errorf("count word_status: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status, count int
		if err := rows.Scan(&status, &count); err != nil {
			return c, err
		}
		switch status {
		case 1:
			c.Learning = count
		case 2:
			c.Known = count
		case 3:
			c.Ignored = count
		}
	}
	return c, rows.Err()
}

// --- Helpers --------------------------------------------------------------

// nullableInt / nullableStr keep INSERTs honest about NULL vs zero/empty.
func nullableInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// jsonOrNil marshals v, returning nil (SQL NULL) for nil/empty inputs.
func jsonOrNil(v any) any {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	s := string(b)
	if s == "null" || s == "[]" || s == "{}" {
		return nil
	}
	return s
}
