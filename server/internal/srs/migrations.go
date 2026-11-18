package srs

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/yumejp/yume/server/internal/dbsnap"
)

// Schema migrations, tracked by PRAGMA user_version.
//
// The first public schema is version 1. Databases written by pre-release
// builds are not an upgrade path and are refused: public decks carry
// applicationID, pre-release ones do not (see migrate).
//
// Rules for the next migration:
//   - Append, never edit. A migration that shipped is what every existing
//     database ran; changing it makes a fresh database differ from an old one.
//   - Freeze values inside the migration (no reads of registries or package
//     vars), so a later change elsewhere never alters what it wrote.
//   - Additive: ALTER TABLE ADD COLUMN, CREATE INDEX / TRIGGER IF NOT EXISTS.
//     Reshaping a table is CREATE NEW + INSERT SELECT + DROP OLD + RENAME in
//     one migration, followed by stampTriggers for the new table.
//   - SQLite does not enforce column types; epoch columns get
//     CHECK (typeof(col) = 'integer').
//   - Data moves that are not learner edits lift the table's stamp_update
//     trigger around the writes so updated_at stays honest.
//   - testdata/schema.json is the structural golden of a fresh database;
//     regenerate it (UPDATE_GOLDEN=1) in the same change.
//   - migrate snapshots a released deck before upgrading it
//     (backups/cards-premigrate-*-v<N>.db). The first migration past the
//     baseline also brings the upgrade smoke test (docs/releasing.md).

type migration struct {
	version int
	name    string
	apply   func(*sql.Tx) error
}

// baselineVersion is the oldest schema this build opens.
const baselineVersion = 1

var migrations = []migration{
	{
		version: baselineVersion,
		name:    "baseline: the schema at the first public release",
		apply: func(tx *sql.Tx) error {
			stmts := []string{
				// Cards: the deck, with FSRS state inline and the card's own
				// content (sentence, translation, explanation, mnemonic override).
				`CREATE TABLE IF NOT EXISTS cards (
					id             INTEGER PRIMARY KEY AUTOINCREMENT,
					jmdict_id      INTEGER UNIQUE,
					lemma          TEXT NOT NULL,
					kanji          TEXT,
					readings       TEXT,
					glosses        TEXT,
					sentence       TEXT,
					source_url     TEXT,
					created_at     INTEGER NOT NULL,
					due            INTEGER NOT NULL,
					stability      REAL    NOT NULL,
					difficulty     REAL    NOT NULL,
					elapsed_days   REAL    NOT NULL DEFAULT 0,
					scheduled_days REAL    NOT NULL DEFAULT 0,
					reps           INTEGER NOT NULL DEFAULT 0,
					lapses         INTEGER NOT NULL DEFAULT 0,
					state          INTEGER NOT NULL DEFAULT 0,
					last_review    INTEGER,
					card_type      TEXT NOT NULL DEFAULT 'vocab',
					surface        TEXT,
					audio_file     TEXT,
					image_file     TEXT,
					audio_source   TEXT,
					translation    TEXT,
					explanation    TEXT,
					updated_at     INTEGER,
					lang           TEXT NOT NULL DEFAULT 'ja',
					mnemonic_parts TEXT,
					mnemonic_story TEXT
				)`,
				`CREATE INDEX IF NOT EXISTS idx_cards_due ON cards(due)`,
				`CREATE INDEX IF NOT EXISTS idx_cards_lemma ON cards(lemma)`,
				`CREATE INDEX IF NOT EXISTS idx_cards_lang_lemma ON cards(lang, lemma)`,

				// Imported Yomitan-format dictionaries, pitch accent and frequency data.
				`CREATE TABLE IF NOT EXISTS dicts (
					id          INTEGER PRIMARY KEY AUTOINCREMENT,
					name        TEXT NOT NULL,
					revision    TEXT,
					author      TEXT,
					url         TEXT,
					description TEXT,
					enabled     INTEGER NOT NULL DEFAULT 1,
					imported_at INTEGER NOT NULL,
					created_at  INTEGER,
					updated_at  INTEGER,
					lang        TEXT NOT NULL DEFAULT 'ja',
					gloss_lang  TEXT NOT NULL DEFAULT 'en'
				)`,
				`CREATE TABLE IF NOT EXISTS dict_entries (
					id               INTEGER PRIMARY KEY AUTOINCREMENT,
					dict_id          INTEGER NOT NULL,
					term             TEXT NOT NULL,
					reading          TEXT,
					definitions_json TEXT NOT NULL,
					popularity       REAL NOT NULL DEFAULT 0,
					created_at       INTEGER,
					updated_at       INTEGER,
					FOREIGN KEY (dict_id) REFERENCES dicts(id) ON DELETE CASCADE
				)`,
				`CREATE INDEX IF NOT EXISTS idx_dict_entries_term ON dict_entries(term)`,
				`CREATE INDEX IF NOT EXISTS idx_dict_entries_dict ON dict_entries(dict_id)`,
				`CREATE TABLE IF NOT EXISTS pitches (
					id         INTEGER PRIMARY KEY AUTOINCREMENT,
					dict_id    INTEGER NOT NULL,
					expression TEXT NOT NULL,
					reading    TEXT,
					position   INTEGER NOT NULL,
					created_at INTEGER,
					updated_at INTEGER,
					FOREIGN KEY (dict_id) REFERENCES dicts(id) ON DELETE CASCADE
				)`,
				`CREATE INDEX IF NOT EXISTS idx_pitches_expr ON pitches(expression)`,
				`CREATE TABLE IF NOT EXISTS frequencies (
					id         INTEGER PRIMARY KEY AUTOINCREMENT,
					dict_id    INTEGER NOT NULL,
					expression TEXT NOT NULL,
					reading    TEXT,
					value      INTEGER NOT NULL,
					display    TEXT,
					created_at INTEGER,
					updated_at INTEGER,
					FOREIGN KEY (dict_id) REFERENCES dicts(id) ON DELETE CASCADE
				)`,
				`CREATE INDEX IF NOT EXISTS idx_frequencies_expr ON frequencies(expression)`,

				// Review history; rows outlive their card (history is real activity)
				// and carry the pre-review snapshot so a review can be undone.
				`CREATE TABLE IF NOT EXISTS review_log (
					id                  INTEGER PRIMARY KEY AUTOINCREMENT,
					card_id             INTEGER NOT NULL,
					rating              INTEGER NOT NULL,
					reviewed_at         INTEGER NOT NULL,
					review_id           TEXT,
					prev_due            INTEGER,
					prev_stability      REAL,
					prev_difficulty     REAL,
					prev_elapsed_days   REAL,
					prev_scheduled_days REAL,
					prev_reps           INTEGER,
					prev_lapses         INTEGER,
					prev_state          INTEGER,
					prev_last_review    INTEGER,
					word_status_prior   INTEGER,
					created_at          INTEGER,
					updated_at          INTEGER
				)`,
				`CREATE INDEX IF NOT EXISTS idx_review_log_time ON review_log(reviewed_at)`,
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_review_log_review_id ON review_log(review_id) WHERE review_id IS NOT NULL`,
				`CREATE INDEX IF NOT EXISTS idx_review_log_card ON review_log(card_id, id)`,

				// Media sidecars (files under <datadir>/media), one row per clip.
				`CREATE TABLE IF NOT EXISTS card_media (
					id         INTEGER PRIMARY KEY AUTOINCREMENT,
					card_id    INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
					kind       TEXT NOT NULL DEFAULT 'audio',
					label      TEXT,
					source     TEXT,
					file       TEXT NOT NULL,
					is_primary INTEGER NOT NULL DEFAULT 0,
					created_at INTEGER NOT NULL,
					voice      TEXT,
					updated_at INTEGER
				)`,
				`CREATE INDEX IF NOT EXISTS idx_card_media_card ON card_media(card_id)`,

				// Notes beside a card; the note's kind is a field, never read from text.
				`CREATE TABLE IF NOT EXISTS card_notes (
					id          INTEGER PRIMARY KEY AUTOINCREMENT,
					card_id     INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
					kind        TEXT NOT NULL CHECK (kind IN ('tutor', 'example', 'triage')),
					note        TEXT NOT NULL,
					created_at  INTEGER NOT NULL DEFAULT (CAST(strftime('%s','now') AS INTEGER)) CHECK (typeof(created_at) = 'integer'),
					updated_at  INTEGER NOT NULL DEFAULT (CAST(strftime('%s','now') AS INTEGER)) CHECK (typeof(updated_at) = 'integer'),
					translation TEXT NOT NULL DEFAULT ''
				)`,
				`CREATE INDEX IF NOT EXISTS idx_card_notes_card ON card_notes(card_id, kind)`,

				// Analyzed video library (what the learner watched, by readiness).
				`CREATE TABLE IF NOT EXISTS video_snapshots (
					id         INTEGER PRIMARY KEY AUTOINCREMENT,
					video_id   TEXT NOT NULL UNIQUE,
					url        TEXT NOT NULL,
					title      TEXT NOT NULL,
					platform   TEXT NOT NULL,
					lang       TEXT NOT NULL DEFAULT 'ja',
					cue_lemmas TEXT NOT NULL,
					cue_count  INTEGER NOT NULL,
					updated_at INTEGER NOT NULL,
					created_at INTEGER NOT NULL
				)`,

				// Settings blobs synced across clients.
				`CREATE TABLE IF NOT EXISTS user_prefs (
					key        TEXT PRIMARY KEY,
					value      TEXT NOT NULL,
					updated_at INTEGER NOT NULL,
					created_at INTEGER
				)`,

				// Per-lemma knowledge, scoped by language: status, tags,
				// suggestions and the deck's own kanji stories.
				`CREATE TABLE IF NOT EXISTS word_status (
					lang       TEXT NOT NULL DEFAULT 'ja',
					lemma      TEXT NOT NULL,
					status     INTEGER NOT NULL,
					updated_at INTEGER NOT NULL,
					created_at INTEGER,
					PRIMARY KEY (lang, lemma)
				)`,
				`CREATE INDEX IF NOT EXISTS idx_word_status_status ON word_status(lang, status)`,
				`CREATE TABLE IF NOT EXISTS word_tags (
					lang       TEXT NOT NULL DEFAULT 'ja',
					lemma      TEXT NOT NULL,
					tag        TEXT NOT NULL,
					kind       TEXT NOT NULL DEFAULT 'user',
					created_at INTEGER,
					updated_at INTEGER,
					PRIMARY KEY (lang, lemma, tag)
				)`,
				`CREATE INDEX IF NOT EXISTS idx_word_tags_tag ON word_tags(lang, tag)`,
				`CREATE TABLE IF NOT EXISTS watch_words (
					lang       TEXT NOT NULL DEFAULT 'ja',
					lemma      TEXT NOT NULL,
					note       TEXT NOT NULL DEFAULT '',
					source     TEXT NOT NULL DEFAULT 'mcp',
					created_at INTEGER NOT NULL,
					updated_at INTEGER,
					PRIMARY KEY (lang, lemma)
				)`,
				`CREATE TABLE IF NOT EXISTS kanji_notes (
					lang       TEXT NOT NULL DEFAULT 'ja',
					kanji      TEXT NOT NULL,
					note       TEXT NOT NULL,
					updated_at INTEGER NOT NULL,
					created_at INTEGER,
					PRIMARY KEY (lang, kanji)
				)`,
			}
			// Every table keeps created_at / updated_at through triggers, so
			// writers never set them by hand.
			for _, table := range []string{
				"cards", "dicts", "dict_entries", "pitches", "frequencies", "review_log",
				"card_media", "card_notes", "video_snapshots", "user_prefs",
				"word_status", "word_tags", "watch_words", "kanji_notes",
			} {
				stmts = append(stmts, stampTriggers(table)...)
			}
			for _, q := range stmts {
				if _, err := tx.Exec(q); err != nil {
					return fmt.Errorf("baseline %.60s...: %w", q, err)
				}
			}
			return nil
		},
	},
}

// stampTriggers returns the two triggers that keep a table's created_at /
// updated_at; a rebuilt table needs them again.
func stampTriggers(table string) []string {
	return []string{
		fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS trg_%[1]s_stamp_insert AFTER INSERT ON %[1]s
			FOR EACH ROW WHEN NEW.created_at IS NULL OR NEW.updated_at IS NULL
			BEGIN
				UPDATE %[1]s SET
					created_at = COALESCE(created_at, CAST(strftime('%%s','now') AS INTEGER)),
					updated_at = COALESCE(updated_at, CAST(strftime('%%s','now') AS INTEGER))
				WHERE rowid = NEW.rowid;
			END`, table),
		fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS trg_%[1]s_stamp_update AFTER UPDATE ON %[1]s
			FOR EACH ROW WHEN NEW.updated_at IS OLD.updated_at
			BEGIN
				UPDATE %[1]s SET updated_at = CAST(strftime('%%s','now') AS INTEGER) WHERE rowid = NEW.rowid;
			END`, table),
	}
}

// An out-of-order version would silently skip a migration; panic at startup instead.
func init() {
	for i := 1; i < len(migrations); i++ {
		if migrations[i].version <= migrations[i-1].version {
			panic(fmt.Sprintf(
				"srs migrations out of order: %d (%s) follows %d",
				migrations[i].version, migrations[i].name, migrations[i-1].version))
		}
	}
}

// applicationID marks a database written by a public Yume build (PRAGMA
// application_id; "YUME" in ASCII). Pre-release builds numbered their schema
// 1-22 in a different lineage, so user_version alone cannot tell a public
// schema-N deck from a pre-release one; the marker can. migrate sets it on
// every deck it opens; a public schema-1 deck from before the marker existed
// is recognized by its tables and marked then.
const applicationID = 0x59554D45

// PreReleaseDoc is where a refused pre-release deck's owner is sent.
const PreReleaseDoc = `docs/backup-and-restore.md, "Decks from pre-release builds"`

// Schema error kinds, stable identifiers for logs and API codes. The messages
// name files and may change; these never carry learner data.
const (
	ErrKindSchemaNewer      = "schema_newer"
	ErrKindSchemaPreRelease = "schema_prerelease"
	ErrKindNotYume          = "not_yume_database"
)

// schemaErr is a database this build refuses to open.
type schemaErr struct {
	kind string
	err  error
}

func (e schemaErr) Error() string { return e.err.Error() }
func (e schemaErr) Unwrap() error { return e.err }

// SchemaErrKind returns the kind of a refused-database error, or "".
func SchemaErrKind(err error) string {
	var se schemaErr
	if errors.As(err, &se) {
		return se.kind
	}
	return ""
}

// migrate brings the DB at path up to the latest version; idempotent. Before
// upgrading a deck that already holds a released schema it snapshots the
// file (backups/cards-premigrate-*), and it refuses to upgrade if that fails.
func migrate(db *sql.DB, path string) error {
	current, appID, err := checkSchema(db)
	if err != nil {
		return err
	}
	if current > 0 && current < SchemaVersion() {
		if _, err := dbsnap.BeforeMigrate(db, path, current); err != nil {
			return fmt.Errorf("snapshot before upgrading schema %d, so nothing was changed: %w", current, err)
		}
	}
	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for migration %d: %w", m.version, err)
		}
		if err := m.apply(tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}
		// PRAGMA values can't be bound parameters.
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("set user_version to %d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}
	}
	// A new deck, or a public schema-1 deck from before the marker.
	if appID != applicationID {
		if _, err := db.Exec(fmt.Sprintf("PRAGMA application_id = %d", applicationID)); err != nil {
			return fmt.Errorf("set application_id: %w", err)
		}
	}
	return nil
}

// checkSchema reads a deck's version and marker and refuses what this build
// cannot open (a newer, pre-release or foreign database). It never writes, so
// a restore can vet a snapshot before it replaces the live deck.
func checkSchema(db *sql.DB) (current, appID int, err error) {
	if err := db.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return 0, 0, fmt.Errorf("read user_version: %w", err)
	}
	if err := db.QueryRow("PRAGMA application_id").Scan(&appID); err != nil {
		return 0, 0, fmt.Errorf("read application_id: %w", err)
	}
	if current > 0 && appID != applicationID {
		if err := checkUnmarked(db, current, appID); err != nil {
			return 0, 0, err
		}
	}
	// Never guess whether an unknown schema is compatible with this binary.
	if latest := SchemaVersion(); current > latest {
		return 0, 0, schemaErr{ErrKindSchemaNewer, fmt.Errorf(
			"database schema %d is newer than supported schema %d: a newer Yume build wrote it; run that build, or restore the snapshot taken before the upgrade (docs/backup-and-restore.md)",
			current, latest)}
	}
	return current, appID, nil
}

// checkUnmarked accepts a versioned database without the public marker only
// when it is a public schema-1 deck written before the marker existed:
// user_version 1 and the baseline's tables. Anything else is a pre-release
// deck (or not a Yume database) and is refused.
func checkUnmarked(db *sql.DB, current, appID int) error {
	if appID != 0 {
		return schemaErr{ErrKindNotYume, fmt.Errorf(
			"this file is not a Yume database (application_id %d); point Yume at its own data folder", appID)}
	}
	if current == baselineVersion {
		ok, err := hasBaselineTables(db)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
	}
	return schemaErr{ErrKindSchemaPreRelease, fmt.Errorf(
		"database schema %d was written by a pre-release Yume build and cannot be opened; export it with that build and import it here (see %s)",
		current, PreReleaseDoc)}
}

// hasBaselineTables reports whether db has what separates the public baseline
// from the first pre-release schemas: language-scoped cards and word
// statuses and typed card notes. The check is deliberately loose; a deck
// that reached schema 1 by a hand conversion must keep opening.
func hasBaselineTables(db *sql.DB) (bool, error) {
	for table, cols := range map[string][]string{
		"cards":       {"lang"},
		"card_notes":  {"kind"},
		"word_status": {"lang"},
	} {
		rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
		if err != nil {
			return false, fmt.Errorf("inspect %s: %w", table, err)
		}
		have := map[string]bool{}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				rows.Close()
				return false, err
			}
			have[name] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return false, err
		}
		for _, c := range cols {
			if !have[c] {
				return false, nil
			}
		}
	}
	return true, nil
}

// SchemaVersion returns the latest migration version known to this binary.
func SchemaVersion() int {
	return migrations[len(migrations)-1].version
}
