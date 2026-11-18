package server

import (
	"net/http"
	"time"

	"github.com/yumejp/yume/server/internal/kanjidic"
	"github.com/yumejp/yume/server/internal/mcpserver"
	"github.com/yumejp/yume/server/internal/srs"
)

const (
	maxOfflineReviewAge       = 180 * 24 * time.Hour
	maxOfflineReviewClockSkew = 24 * time.Hour
)

func registerDeckRoutes(mux *http.ServeMux, cfg Config) {
	// --- SRS card store endpoints ----------------------------------------

	// The log notes only how a save ended (requestlog.go): never the word.
	jsonPost(mux, "/save_card", func(r *http.Request, store *srs.Store, req srs.SaveCardReq) (any, error) {
		saved, err := store.SaveCard(req)
		noteCardSave(r, saved, err)
		if err != nil {
			return nil, err
		}
		return saved, nil
	})

	// Attach audio/image to an EXISTING card (save_card only attaches on
	// create), e.g. VOICEVOX clips applied to the whole deck after the fact.
	jsonPost(mux, "/attach_media", func(_ *http.Request, store *srs.Store, req struct {
		CardID      int64          `json:"card_id"`
		Audio       *srs.MediaBlob `json:"audio,omitempty"`
		Image       *srs.MediaBlob `json:"image,omitempty"`
		AudioSource string         `json:"audio_source,omitempty"`
	}) (any, error) {
		if req.CardID == 0 {
			return nil, badRequest("card_id is required")
		}
		if req.Audio == nil && req.Image == nil {
			return nil, badRequest("audio or image is required")
		}
		if err := store.AttachMedia(req.CardID, req.Audio, req.Image, req.AudioSource); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "card_id": req.CardID}, nil
	})

	jsonPost(mux, "/card_status", func(_ *http.Request, store *srs.Store, req struct {
		JMdictIDs []int `json:"jmdict_ids"`
		// Lemmas also catches cards saved without a jmdict_id (imports, mined words).
		Lemmas []string `json:"lemmas"`
	}) (any, error) {
		rows, err := store.CardStatus(req.JMdictIDs)
		if err != nil {
			return nil, err
		}
		resp := map[string]any{"statuses": rows}
		if len(req.Lemmas) > 0 {
			lemmaRows, err := store.CardStatusByLemma(req.Lemmas)
			if err != nil {
				return nil, err
			}
			resp["lemma_statuses"] = lemmaRows
		}
		return resp, nil
	})

	// tz_offset_minutes (minutes east of UTC) resets new_introduced_today at
	// the learner's midnight, not the server's; absent keeps server-local.
	jsonPost(mux, "/deck_stats", func(_ *http.Request, store *srs.Store, req struct {
		TZOffsetMinutes *int `json:"tz_offset_minutes"`
	}) (any, error) {
		ds, err := store.DeckStatsTZ(req.TZOffsetMinutes)
		if err != nil {
			return nil, err
		}
		since, err := store.VacationSince()
		if err != nil {
			return nil, err
		}
		var plan *srs.CatchUp
		if since == 0 {
			if plan, err = store.CatchUpPlan(req.TZOffsetMinutes); err != nil {
				return nil, err
			}
		}
		limit, err := store.ReviewLimit()
		if err != nil {
			return nil, err
		}
		return struct {
			srs.DeckStats
			CatchUp       *srs.CatchUp `json:"catch_up,omitempty"`
			VacationSince int64        `json:"vacation_since,omitempty"`
			ReviewLimit   int          `json:"review_limit"`
		}{ds, plan, since, limit}, nil
	}, routeOpts{allowGET: true})

	// park_cards takes cards out of review, keeping their history.
	jsonPost(mux, "/park_cards", func(_ *http.Request, store *srs.Store, req struct {
		CardIDs []int64 `json:"card_ids"`
	}) (any, error) {
		n, err := store.ParkCards(req.CardIDs)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "parked": n}, nil
	})

	// unpark_cards brings parked cards back, per_day of them a day.
	jsonPost(mux, "/unpark_cards", func(_ *http.Request, store *srs.Store, req struct {
		Limit  int `json:"limit"`
		PerDay int `json:"per_day"`
	}) (any, error) {
		n, err := store.UnparkCards(req.Limit, req.PerDay)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "unparked": n}, nil
	})

	// set_review_limit stores the daily review limit (0 = no limit).
	jsonPost(mux, "/set_review_limit", func(_ *http.Request, store *srs.Store, req struct {
		Limit int `json:"limit"`
	}) (any, error) {
		if err := store.SetReviewLimit(req.Limit); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "review_limit": req.Limit}, nil
	})

	// vacation turns vacation mode on or off (see srs.SetVacation).
	jsonPost(mux, "/vacation", func(_ *http.Request, store *srs.Store, req struct {
		On bool `json:"on"`
	}) (any, error) {
		since, shifted, err := store.SetVacation(req.On)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "vacation_since": since, "shifted": shifted}, nil
	})

	jsonPost(mux, "/set_word_status", func(_ *http.Request, store *srs.Store, req struct {
		Lemma  string `json:"lemma"`
		Status int    `json:"status"`
	}) (any, error) {
		if err := store.SetWordStatus(req.Lemma, req.Status); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	})

	jsonPost(mux, "/get_word_statuses", func(_ *http.Request, store *srs.Store, req struct {
		Lemmas []string `json:"lemmas"`
	}) (any, error) {
		rows, err := store.GetWordStatuses(req.Lemmas)
		if err != nil {
			return nil, err
		}
		return map[string]any{"statuses": rows}, nil
	})

	// --- Review (FSRS) ---------------------------------------------------

	// Defaults (GET or empty body): 20 cards, new cards ungated. new_limit
	// caps never-reviewed cards per day (mobile); tz_offset_minutes is
	// minutes east of UTC and moves that day boundary — see srs.dayStart.
	jsonPost(mux, "/get_due_cards", func(_ *http.Request, store *srs.Store, req struct {
		Limit           *int `json:"limit"`
		NewLimit        *int `json:"new_limit"`
		TZOffsetMinutes *int `json:"tz_offset_minutes"`
	}) (any, error) {
		limit, newLimit := 20, -1
		if req.Limit != nil && *req.Limit > 0 {
			limit = *req.Limit
		}
		if req.NewLimit != nil && *req.NewLimit >= 0 {
			newLimit = *req.NewLimit
		}
		// A plain review fetch during catch-up is served most-at-risk first.
		if newLimit == 0 {
			plan, err := store.CatchUpPlan(req.TZOffsetMinutes)
			if err != nil {
				return nil, err
			}
			if plan != nil {
				cards, err := store.GetCatchUpCards(limit, req.TZOffsetMinutes)
				if err != nil {
					return nil, err
				}
				attachReadingParts(cards)
				return map[string]any{"cards": cards}, nil
			}
		}
		cards, err := store.GetDueCardsGatedTZ(limit, newLimit, req.TZOffsetMinutes)
		if err != nil {
			return nil, err
		}
		attachReadingParts(cards)
		return map[string]any{"cards": cards}, nil
	}, routeOpts{allowGET: true})

	jsonPost(mux, "/review_card", func(_ *http.Request, store *srs.Store, req struct {
		CardID int64 `json:"card_id"`
		Rating int   `json:"rating"`
		// Idempotency token: a retry with the same token replays the stored
		// result instead of double-applying FSRS.
		ReviewID string `json:"review_id"`
		// Offline replay: the card's last_review_id as the client last fetched
		// it ("" = no tokened reviews yet). Absent means no check; on mismatch
		// nothing is applied and the result has conflict=true.
		PrevReviewID *string `json:"prev_review_id"`
		// Unix seconds of an offline rating; accepted only with PrevReviewID,
		// so a stale client cannot rewrite history.
		ReviewedAt *int64 `json:"reviewed_at"`
	}) (any, error) {
		rating := srs.ReviewRating(req.Rating)
		switch {
		case req.ReviewedAt != nil && req.PrevReviewID == nil:
			return nil, badRequest("reviewed_at requires prev_review_id")
		case req.ReviewedAt != nil:
			now := time.Now()
			reviewedAt := time.Unix(*req.ReviewedAt, 0)
			if reviewedAt.Before(now.Add(-maxOfflineReviewAge)) || reviewedAt.After(now.Add(maxOfflineReviewClockSkew)) {
				return nil, badRequest("reviewed_at is outside the accepted range")
			}
			// A modestly fast device clock must not discard a real rating.
			if reviewedAt.After(now) {
				reviewedAt = now
			}
			return store.ReviewCardGuardedAt(req.CardID, rating, req.ReviewID, *req.PrevReviewID, reviewedAt)
		case req.PrevReviewID != nil:
			return store.ReviewCardGuarded(req.CardID, rating, req.ReviewID, *req.PrevReviewID)
		}
		return store.ReviewCardIdem(req.CardID, rating, req.ReviewID)
	})

	jsonPost(mux, "/word_status_counts", func(_ *http.Request, store *srs.Store, _ struct{}) (any, error) {
		return store.WordStatusCounts()
	}, routeOpts{allowGET: true})

	// --- Vocab explorer --------------------------------------------------

	jsonPost(mux, "/list_vocab", func(_ *http.Request, store *srs.Store, req struct {
		Filter srs.VocabFilter `json:"filter"`
		Limit  int             `json:"limit"`
		Offset int             `json:"offset"`
	}) (any, error) {
		if req.Filter.Sort != "frequency" && req.Filter.Band == "" {
			return store.ListVocab(req.Filter, req.Limit, req.Offset)
		}
		// Ranks live in the in-memory dictionary, so a frequency sort or a
		// band filter orders the whole matching set here and pages it.
		rows, err := store.ListVocabAll(req.Filter)
		if err != nil {
			return nil, err
		}
		annotateRanks(rows)
		rows = filterBand(rows, req.Filter.Band)
		limit, offset := req.Limit, req.Offset
		if limit <= 0 {
			limit = 50
		}
		if limit > 500 {
			limit = 500
		}
		if offset < 0 || offset > len(rows) {
			offset = len(rows)
		}
		end := offset + limit
		if end > len(rows) {
			end = len(rows)
		}
		return srs.ListVocabResp{Total: len(rows), Rows: rows[offset:end]}, nil
	})

	// Per-band totals by status for the same filter — the tile view's
	// section headers.
	jsonPost(mux, "/vocab_bands", func(_ *http.Request, store *srs.Store, req struct {
		Filter srs.VocabFilter `json:"filter"`
	}) (any, error) {
		req.Filter.Band = ""
		rows, err := store.ListVocabAll(req.Filter)
		if err != nil {
			return nil, err
		}
		annotateRanks(rows)
		return map[string]any{"bands": countBands(rows)}, nil
	})

	// --- Tags ------------------------------------------------------------

	jsonPost(mux, "/set_word_tags", func(_ *http.Request, store *srs.Store, req struct {
		Lemma string   `json:"lemma"`
		Tags  []string `json:"tags"`
	}) (any, error) {
		tags, err := store.SetWordTags(req.Lemma, req.Tags)
		if err != nil {
			return nil, err
		}
		return map[string]any{"tags": tags}, nil
	})

	jsonPost(mux, "/add_word_tag", func(_ *http.Request, store *srs.Store, req struct {
		Lemma string `json:"lemma"`
		Tag   string `json:"tag"`
	}) (any, error) {
		if err := store.AddWordTag(req.Lemma, req.Tag); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	})

	jsonPost(mux, "/remove_word_tag", func(_ *http.Request, store *srs.Store, req struct {
		Lemma string `json:"lemma"`
		Tag   string `json:"tag"`
	}) (any, error) {
		if err := store.RemoveWordTag(req.Lemma, req.Tag); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	})

	jsonPost(mux, "/list_all_tags", func(_ *http.Request, store *srs.Store, _ struct{}) (any, error) {
		tags, err := store.ListAllTags()
		if err != nil {
			return nil, err
		}
		return map[string]any{"tags": tags}, nil
	}, routeOpts{allowGET: true})

	// The catalog of reserved system tags (tag + description).
	mux.HandleFunc("/list_system_tags", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"tags": srs.SystemTags()})
	})

	// Applies (or removes) a system tag to a card's lemma; free-form tags go
	// through add_word_tag/set_word_tags.
	jsonPost(mux, "/tag_card", func(r *http.Request, store *srs.Store, req struct {
		CardID int64  `json:"card_id"`
		Tag    string `json:"tag"`
		Remove bool   `json:"remove"`
	}) (any, error) {
		if req.CardID == 0 {
			return nil, badRequest("card_id required")
		}
		lemma, err := store.TagCard(req.CardID, req.Tag, req.Remove)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"ok": true, "card_id": req.CardID, "lemma": lemma,
			"tag": req.Tag, "removed": req.Remove,
		}, nil
	})

	// --- Video library (watch-next ranking) --------------------------------
	//
	// A client may persist a compact per-video coverage snapshot; list
	// recomputes coverage / i+1 / maturing-soon against the learner's CURRENT
	// word statuses on every call, so the ranking improves as they study.

	jsonPost(mux, "/save_video_snapshot", func(_ *http.Request, store *srs.Store, req srs.VideoSnapshotReq) (any, error) {
		videoID, updated, err := store.SaveVideoSnapshot(req)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "video_id": videoID, "updated": updated}, nil
	})

	jsonPost(mux, "/list_video_snapshots", func(_ *http.Request, store *srs.Store, req struct {
		Lang string `json:"lang"`
	}) (any, error) {
		videos, err := store.ListVideoSnapshots(req.Lang)
		if err != nil {
			return nil, err
		}
		if videos == nil {
			videos = []srs.VideoStats{}
		}
		return map[string]any{"videos": videos}, nil
	}, routeOpts{allowGET: true})

	// One video's study-relevant words (default: unknown + learning) with
	// in-video cue counts and library reach. Gloss enrichment happens here,
	// not in srs (the store has no dict access).
	jsonPost(mux, "/get_video_vocab", func(_ *http.Request, store *srs.Store, req struct {
		VideoID  string `json:"video_id"`
		Statuses []int  `json:"statuses"`
	}) (any, error) {
		words, err := store.GetVideoVocab(req.VideoID, req.Statuses)
		if err != nil {
			return nil, err
		}
		lang, err := store.VideoSnapshotLang(req.VideoID)
		if err != nil {
			return nil, err
		}
		// EnrichVideoVocab never returns nil, so "words" is [] never null.
		return map[string]any{"words": mcpserver.EnrichVideoVocab(lang, words)}, nil
	})

	jsonPost(mux, "/delete_video_snapshot", func(_ *http.Request, store *srs.Store, req struct {
		VideoID string `json:"video_id"`
	}) (any, error) {
		deleted, err := store.DeleteVideoSnapshot(req.VideoID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"deleted": deleted}, nil
	})

	jsonPost(mux, "/delete_by_tag", func(_ *http.Request, store *srs.Store, req srs.DeleteByTagReq) (any, error) {
		return store.DeleteByTag(req)
	})
}

// attachReadingParts splits each card's reading per kanji. Best effort: with
// KANJIDIC missing the cards go out without it.
func attachReadingParts(cards []srs.DueCard) {
	d, err := kanjidic.Default()
	if err != nil {
		return
	}
	for i := range cards {
		if len(cards[i].Readings) == 0 {
			continue
		}
		for _, p := range d.SplitReading(cards[i].Lemma, cards[i].Readings[0]) {
			cards[i].ReadingParts = append(cards[i].ReadingParts, srs.ReadingPart{Kanji: p.Kanji, Reading: p.Reading})
		}
	}
}
