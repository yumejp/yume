package server

import (
	"encoding/json"
	"fmt"
	"github.com/yumejp/yume/server/internal/kanjidic"
	"net/http"
	"time"

	"github.com/yumejp/yume/server/internal/mcpserver"
	"github.com/yumejp/yume/server/internal/srs"
)

func registerMaintenanceRoutes(mux *http.ServeMux, cfg Config) {
	// --- Anki CSV import -------------------------------------------------

	jsonPost(mux, "/import_csv", func(_ *http.Request, store *srs.Store, req srs.ImportCSVReq) (any, error) {
		return store.ImportCSV(req)
	})

	// Delete one card (notes cascade; media sidecars removed). The lemma's
	// word_status survives — deleting a flashcard doesn't un-know the word.
	jsonPost(mux, "/delete_card", func(_ *http.Request, store *srs.Store, req struct {
		CardID int64 `json:"card_id"`
	}) (any, error) {
		deleted, err := store.DeleteCard(req.CardID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"deleted": deleted}, nil
	})

	// Undo the most recent review (single step). Optional tz_offset_minutes
	// (minutes east of UTC) bounds "today's reviews" at the learner's
	// midnight — see srs.dayStart. undone_review_id lets the client verify
	// WHICH review was reverted ("" = an untokened legacy row).
	jsonPost(mux, "/undo_review", func(_ *http.Request, store *srs.Store, req struct {
		TZOffsetMinutes *int `json:"tz_offset_minutes"`
	}) (any, error) {
		cardID, reviewID, err := store.UndoReviewTZ(req.TZOffsetMinutes)
		if err != nil {
			return nil, err
		}
		return map[string]any{"card_id": cardID, "undone_review_id": reviewID}, nil
	}, routeOpts{optionalBody: true})

	// --- Anki .apkg export ------------------------------------------------

	// Returns the package as binary (the only non-JSON response body in the
	// API) — the vocab page downloads it straight to a file. Scheduling,
	// review history, tags, and media all ride along; see srs.ExportApkg.
	mux.HandleFunc("/export_apkg", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "use POST")
			return
		}
		var req struct {
			DeckName string `json:"deck_name,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		store, err := storeFor(r)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		data, err := store.ExportApkg(req.DeckName)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("attachment; filename=%q", "yume-deck-"+time.Now().Format("2006-01-02")+".apkg"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})

	// --- Card notes ------------------------------------------------------

	// add_card_note: kind is tutor | example | triage; an example carries
	// its English in translation. Sent without a kind, the kind is read from
	// the markers older clients wrote. A card's translation and its mnemonic
	// are not notes (see /update_card, /set_mnemonic).
	jsonPost(mux, "/add_card_note", func(_ *http.Request, store *srs.Store, req struct {
		CardID      int64  `json:"card_id"`
		Kind        string `json:"kind"`
		Note        string `json:"note"`
		Translation string `json:"translation"`
	}) (any, error) {
		id, createdAt, err := store.AddCardNote(req.CardID, req.Kind, req.Note, req.Translation)
		if err != nil {
			return nil, err
		}
		return map[string]any{"id": id, "created_at": createdAt}, nil
	})

	jsonPost(mux, "/list_card_notes", func(_ *http.Request, store *srs.Store, req struct {
		CardID int64 `json:"card_id"`
	}) (any, error) {
		notes, err := store.ListCardNotes(req.CardID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"notes": notes}, nil
	})

	jsonPost(mux, "/delete_card_note", func(_ *http.Request, store *srs.Store, req struct {
		NoteID int64 `json:"note_id"`
	}) (any, error) {
		n, err := store.DeleteCardNote(req.NoteID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"deleted": n}, nil
	})

	// --- Kanji ------------------------------------------------------------

	// Deck words + tutor story from the user's DB, dictionary facts from
	// KANJIDIC2 (absent when the file isn't installed).
	jsonPost(mux, "/kanji_info", func(_ *http.Request, store *srs.Store, req struct {
		Kanji string `json:"kanji"`
	}) (any, error) {
		info, err := store.KanjiInfo(req.Kanji)
		if err != nil {
			return nil, err
		}
		var entry *kanjidic.Entry
		if d, err := kanjidic.Default(); err == nil {
			entry = d.Lookup(req.Kanji)
		}
		return struct {
			srs.KanjiInfo
			Dict *kanjidic.Entry `json:"dict,omitempty"`
		}{info, entry}, nil
	})

	// The deck's own story, an override of the shared one; empty clears it.
	jsonPost(mux, "/set_kanji_note", func(_ *http.Request, store *srs.Store, req struct {
		Kanji string `json:"kanji"`
		Note  string `json:"note"`
	}) (any, error) {
		if err := store.SetKanjiNote(req.Kanji, req.Note); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	})

	// The shared story every deck sees when it has none of its own (owner only).
	jsonPost(mux, "/set_shared_kanji_note", func(r *http.Request, store *srs.Store, req struct {
		Kanji string `json:"kanji"`
		Note  string `json:"note"`
	}) (any, error) {
		if err := requireOwner(r, cfg.AuthStore); err != nil {
			return nil, httpErr{http.StatusForbidden, "only the owner may edit the shared kanji stories"}
		}
		lib := srs.SharedLibrary()
		if lib == nil {
			return nil, httpErr{http.StatusServiceUnavailable, "no shared library on this server"}
		}
		if err := lib.SetKanjiNote(store.Lang(), req.Kanji, req.Note); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	})

	// --- Mnemonics ---------------------------------------------------------

	// /set_mnemonic writes a card's own mnemonic, replacing any it had; it
	// overrides the shared one for that lemma. Empty parts and story clear
	// the override, so the card shows the shared mnemonic again. parts is
	// one {kanji, meaning} per kanji of the lemma (a radical opening is
	// fine; a kana-only word may send none or one origin part); story is
	// one to three sentences plus a usage example.
	jsonPost(mux, "/set_mnemonic", func(_ *http.Request, store *srs.Store, req struct {
		CardID int64              `json:"card_id"`
		Parts  []srs.MnemonicPart `json:"parts"`
		Story  string             `json:"story"`
	}) (any, error) {
		replaced, err := store.SetMnemonic(req.CardID, req.Parts, req.Story)
		if err != nil {
			return nil, err
		}
		return map[string]any{"replaced": replaced}, nil
	})

	// The shared library: one mnemonic per (lang, lemma) for every deck on
	// this server. Reads are open; writes are the owner's, like the
	// tokenizer dictionary.
	jsonPost(mux, "/shared_mnemonic", func(_ *http.Request, store *srs.Store, req struct {
		Lemma string `json:"lemma"`
	}) (any, error) {
		lib := srs.SharedLibrary()
		if lib == nil {
			return map[string]any{"mnemonic": nil}, nil
		}
		m, err := lib.Mnemonic(store.Lang(), req.Lemma)
		if err != nil {
			return nil, err
		}
		return map[string]any{"mnemonic": m}, nil
	})

	jsonPost(mux, "/set_shared_mnemonic", func(r *http.Request, store *srs.Store, req struct {
		Lemma string             `json:"lemma"`
		Parts []srs.MnemonicPart `json:"parts"`
		Story string             `json:"story"`
	}) (any, error) {
		if err := requireOwner(r, cfg.AuthStore); err != nil {
			return nil, httpErr{http.StatusForbidden, "only the owner may edit the shared mnemonics"}
		}
		lib := srs.SharedLibrary()
		if lib == nil {
			return nil, httpErr{http.StatusServiceUnavailable, "no shared library on this server"}
		}
		replaced, err := lib.SetMnemonic(store.Lang(), req.Lemma, req.Parts, req.Story)
		if err != nil {
			return nil, err
		}
		return map[string]any{"replaced": replaced}, nil
	})

	// /mnemonic_audit classifies every card's effective mnemonic (own, else
	// shared): ok, no_breakdown, missing. kind filters; kana_too lists
	// kana-only lemmas without a mnemonic as missing.
	jsonPost(mux, "/mnemonic_audit", func(_ *http.Request, store *srs.Store, req struct {
		Kind    string `json:"kind"`
		KanaToo bool   `json:"kana_too"`
	}) (any, error) {
		rows, err := store.MnemonicAudit(req.KanaToo)
		if err != nil {
			return nil, err
		}
		counts := map[string]int{}
		out := []srs.MnemonicAuditRow{}
		for _, r := range rows {
			counts[r.Kind]++
			if req.Kind == "" || req.Kind == r.Kind {
				out = append(out, r)
			}
		}
		return map[string]any{"counts": counts, "rows": out}, nil
	})

	// --- Progress dashboard ----------------------------------------------

	// tz_offset_minutes (minutes east of UTC) buckets heatmap / streak /
	// forecast days at the learner's midnight; a UTC server and a learner in
	// Japan otherwise disagree on which day a pre-9am-JST review lands.
	jsonPost(mux, "/stats", func(_ *http.Request, store *srs.Store, req struct {
		TZOffsetMinutes *int `json:"tz_offset_minutes"`
	}) (any, error) {
		return store.ComputeStatsTZ(req.TZOffsetMinutes)
	}, routeOpts{allowGET: true})

	// --- Card media (audio/image sidecars) -------------------------------

	jsonPost(mux, "/get_card_media", func(_ *http.Request, store *srs.Store, req struct {
		CardID int64 `json:"card_id"`
	}) (any, error) {
		return store.GetCardMedia(req.CardID)
	})

	// update_card edits a card's fields in place.
	jsonPost(mux, "/update_card", func(_ *http.Request, store *srs.Store, req struct {
		CardID      int64       `json:"card_id"`
		Sentence    *string     `json:"sentence"`
		Translation *string     `json:"translation"`
		Explanation *string     `json:"explanation"`
		SourceURL   *string     `json:"source_url"`
		Lemma       *string     `json:"lemma"`
		Glosses     *[][]string `json:"glosses"`
		Readings    *[]string   `json:"readings"`
		Kanji       *[]string   `json:"kanji"`
	}) (any, error) {
		if req.CardID == 0 {
			return nil, badRequest("card_id required")
		}
		upd := srs.CardUpdate{Sentence: req.Sentence, Translation: req.Translation, Explanation: req.Explanation, SourceURL: req.SourceURL, Lemma: req.Lemma, Glosses: req.Glosses, Readings: req.Readings, Kanji: req.Kanji}
		if err := store.UpdateCard(req.CardID, upd); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "card_id": req.CardID}, nil
	})

	// add_card_media attaches a new audio/image clip to a card (one of several).
	// Becomes primary if make_primary is set or the card has no primary of that
	// kind yet.
	jsonPost(mux, "/add_card_media", func(_ *http.Request, store *srs.Store, req struct {
		CardID      int64          `json:"card_id"`
		Kind        string         `json:"kind"`
		Label       string         `json:"label"`
		Source      string         `json:"source"`
		Voice       string         `json:"voice"` // credited voice name; rendered under the audio and in About
		Media       *srs.MediaBlob `json:"media"`
		MakePrimary bool           `json:"make_primary"`
	}) (any, error) {
		if req.CardID == 0 || req.Media == nil {
			return nil, badRequest("card_id and media required")
		}
		return store.AddCardMedia(req.CardID, req.Kind, req.Label, req.Source, req.Voice, req.Media, req.MakePrimary)
	})

	// list_media_voices: every credited voice in the deck with clip counts,
	// so a client can print the VOICEVOX credit in one place.
	jsonPost(mux, "/list_media_voices", func(_ *http.Request, store *srs.Store, _ struct{}) (any, error) {
		voices, err := store.ListMediaVoices()
		if err != nil {
			return nil, err
		}
		return map[string]any{"voices": voices}, nil
	})

	// list_card_media returns every clip on a card (metadata only, no bytes).
	jsonPost(mux, "/list_card_media", func(_ *http.Request, store *srs.Store, req struct {
		CardID int64 `json:"card_id"`
	}) (any, error) {
		list, err := store.ListCardMedia(req.CardID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"media": list}, nil
	})

	// set_primary_media chooses which clip the review screen autoplays.
	jsonPost(mux, "/set_primary_media", func(_ *http.Request, store *srs.Store, req struct {
		CardID  int64 `json:"card_id"`
		MediaID int64 `json:"media_id"`
	}) (any, error) {
		if err := store.SetPrimaryMedia(req.CardID, req.MediaID); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	})

	// delete_card_media removes one clip (+ its sidecar); promotes a replacement
	// primary if needed.
	jsonPost(mux, "/delete_card_media", func(_ *http.Request, store *srs.Store, req struct {
		MediaID int64 `json:"media_id"`
	}) (any, error) {
		if err := store.DeleteCardMedia(req.MediaID); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	})

	// get_card_media_file returns one clip's bytes by media id (base64 blob).
	jsonPost(mux, "/get_card_media_file", func(_ *http.Request, store *srs.Store, req struct {
		MediaID int64 `json:"media_id"`
	}) (any, error) {
		return store.GetCardMediaByID(req.MediaID)
	})

	// MCP clients are non-browser local processes (no Origin header).
	mux.Handle("/mcp", mcpserver.Handler(cfg.Version, srs.Owner))

	// Watch words — client-facing CRUD for the MCP-pushed suggestions.
	jsonPost(mux, "/list_watch_words", func(_ *http.Request, store *srs.Store, _ struct{}) (any, error) {
		words, err := store.ListWatchWords()
		if err != nil {
			return nil, err
		}
		// Dictionary readings (default lang) let clients render furigana.
		return map[string]any{"words": mcpserver.EnrichWatchWords("", words)}, nil
	})
	jsonPost(mux, "/remove_watch_word", func(_ *http.Request, store *srs.Store, req struct {
		Lemma string `json:"lemma"`
	}) (any, error) {
		removed, err := store.RemoveWatchWord(req.Lemma)
		if err != nil {
			return nil, err
		}
		return map[string]any{"removed": removed}, nil
	})

	// Managed vocab backups — same operations the MCP tools expose, reachable
	// by any client / curl. srs.MaxBackups of each kind are kept; restore
	// takes a prerestore safety snapshot first.
	jsonPost(mux, "/create_backup", func(_ *http.Request, store *srs.Store, req struct {
		Label string `json:"label"`
	}) (any, error) {
		info, pruned, err := store.CreateBackup(req.Label)
		if err != nil {
			return nil, err
		}
		if pruned == nil {
			pruned = []string{}
		}
		return map[string]any{"backup": info, "pruned": pruned, "max_kept": srs.MaxBackups}, nil
	})
	// Explicit maintenance sweep of media/ — removes sidecar files no card or
	// card_media row references. NOT automatic: see SweepOrphanedMedia.
	jsonPost(mux, "/sweep_media", func(_ *http.Request, store *srs.Store, _ struct{}) (any, error) {
		removed, kept, err := store.SweepOrphanedMedia()
		if err != nil {
			return nil, err
		}
		return map[string]any{"removed": removed, "kept": kept}, nil
	})
	jsonPost(mux, "/list_backups", func(_ *http.Request, store *srs.Store, _ struct{}) (any, error) {
		backups, err := store.ListBackups()
		if err != nil {
			return nil, err
		}
		return map[string]any{"backups": backups}, nil
	})
	jsonPost(mux, "/restore_backup", func(_ *http.Request, store *srs.Store, req struct {
		Name string `json:"name"`
	}) (any, error) {
		safety, pruned, err := store.RestoreBackup(req.Name)
		if err != nil {
			return nil, err
		}
		if pruned == nil {
			pruned = []string{}
		}
		return map[string]any{"restored_from": req.Name, "safety_backup": safety, "pruned": pruned}, nil
	})
}
