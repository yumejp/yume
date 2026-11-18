package srs

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v3"
)

// clientErr marks a request-caused error so handlers answer 400, not 500.
// Wire contract: the offline replayer drops ratings on 4xx and retries on 5xx.
// Code, when set, is a stable identifier clients branch on; the message is
// for people and may change.
type clientErr struct {
	err  error
	code string
}

func (e clientErr) Error() string { return e.err.Error() }
func (e clientErr) Unwrap() error { return e.err }

// ErrCode returns the stable code of a request-caused error, or "".
func ErrCode(err error) string {
	var ce clientErr
	if errors.As(err, &ce) {
		return ce.code
	}
	return ""
}

// clientCodef is clientErrf with a stable code.
func clientCodef(code, format string, args ...any) error {
	return clientErr{fmt.Errorf(format, args...), code}
}

// ErrCodeReviewedAtRejected: the client's reviewed_at is before the card's
// last review; resend without a timestamp to keep the rating.
const ErrCodeReviewedAtRejected = "reviewed_at_rejected"

func clientErrf(format string, args ...any) error {
	return clientErr{err: fmt.Errorf(format, args...)}
}

// IsClientErr reports whether err is the caller's fault (→ HTTP 4xx).
func IsClientErr(err error) bool {
	var ce clientErr
	return errors.As(err, &ce)
}

// ReadingPart is one kanji of a card's word with its share of the reading.
type ReadingPart struct {
	Kanji   string `json:"kanji"`
	Reading string `json:"reading"`
}

// DueCard is the /get_due_cards row: everything needed to render a card
// without follow-up queries.
type DueCard struct {
	ID       int64      `json:"id"`
	Lemma    string     `json:"lemma"`
	Kanji    []string   `json:"kanji"`
	Readings []string   `json:"readings"`
	Glosses  [][]string `json:"glosses"` // per-sense, matches frontend shape
	Sentence *string    `json:"sentence,omitempty"`
	// The card's own English: the sentence's translation and, for tricky
	// words only, an explanation (register, nuance, traps).
	Translation *string `json:"translation,omitempty"`
	Explanation *string `json:"explanation,omitempty"`
	// Mnemonic: the card's memory aid (parts + story); omitted when none.
	Mnemonic *Mnemonic `json:"mnemonic,omitempty"`
	// MnemonicShared: the mnemonic is the server's shared one, not the card's own.
	MnemonicShared bool    `json:"mnemonic_shared,omitempty"`
	SourceURL      *string `json:"source_url,omitempty"`
	CardType       string  `json:"card_type"`
	Surface        *string `json:"surface,omitempty"`
	// IsNew (FSRS New state): the client shows the card already revealed.
	IsNew bool `json:"is_new,omitempty"`
	// Mature: reviewing with an interval of three weeks or more; clients
	// collapse recall aids on mature cards.
	Mature bool `json:"mature,omitempty"`
	// ClozeTarget: for cloze cards, the text the front hides inside the
	// sentence (the saved surface form, else the headword, else a kanji
	// spelling); omitted when nothing in the sentence matches.
	ClozeTarget *string `json:"cloze_target,omitempty"`
	// Populated by handlers from sidecar files, not stored on the row.
	AudioPath   *string `json:"audio_path,omitempty"`
	ImagePath   *string `json:"image_path,omitempty"`
	AudioSource *string `json:"audio_source,omitempty"`
	// Preview: would-be next-due per rating, for button labels. Advisory —
	// /review_card recomputes at commit time.
	Preview *IntervalPreview `json:"preview,omitempty"`
	// SystemTags: the lemma's kind='system' word_tags; omitted when none.
	SystemTags []string `json:"system_tags,omitempty"`
	// TryKanji: a furigana card that has held for a month; the client offers
	// to try it without the reading.
	TryKanji bool `json:"try_kanji,omitempty"`
	steady   bool
	// LastReviewID is the newest review_log row's review_id — the version an
	// offline client echoes back as prev_review_id. Omitted when untokened.
	LastReviewID string `json:"last_review_id,omitempty"`

	// ReadingParts: the reading split per kanji (供 きょう, 給 きゅう), filled by
	// the server from KANJIDIC; absent when no honest split exists.
	ReadingParts []ReadingPart `json:"reading_parts,omitempty"`
}

// IntervalPreview holds the would-be next-due time (unix seconds) for each of
// the four ratings, computed at the moment the due list is built.
type IntervalPreview struct {
	// At is when the preview was computed (unix seconds); labels measure from it.
	At    int64 `json:"at"`
	Again int64 `json:"again"`
	Hard  int64 `json:"hard"`
	Good  int64 `json:"good"`
	Easy  int64 `json:"easy"`
}

// clampDue floors non-Again due dates at now+minIvl (Tuning
// min_non_again_interval_hours) so a card cleared in a sitting comes back
// tomorrow at the earliest; same-day reinforcement is the client's Again lap.
// Only Due/ScheduledDays move — stability/difficulty stay as FSRS computed.
func clampDue(c fsrs.Card, rating fsrs.Rating, now time.Time, minIvl time.Duration) fsrs.Card {
	if rating == fsrs.Again || minIvl <= 0 {
		return c
	}
	if floor := now.Add(minIvl); c.Due.Before(floor) {
		c.Due = floor
		if c.ScheduledDays < 1 {
			c.ScheduledDays = 1
		}
	}
	return c
}

// floorMatureLapse keeps a lapse on a card scheduled ≥ matureDays at
// floorFrac × its pre-lapse stability, so a first miss isn't a near-reset;
// a second miss compounds the floor away. Only stability moves — the Again
// due is untouched and the credit lands on the recovery rating.
func floorMatureLapse(next fsrs.Card, prev fsrs.Card, rating fsrs.Rating, floorFrac, matureDays float64) fsrs.Card {
	if rating != fsrs.Again || floorFrac <= 0 {
		return next
	}
	if prev.State != fsrs.Review || float64(prev.ScheduledDays) < matureDays {
		return next
	}
	if floored := floorFrac * prev.Stability; next.Stability < floored {
		next.Stability = floored
	}
	return next
}

func (s *Store) minNonAgainInterval() time.Duration {
	return time.Duration(s.tune.MinNonAgainIntervalHours * float64(time.Hour))
}

// previewFor computes the would-be next-due per rating. The card must be
// sanitized (sanitizeFSRS) — go-fsrs produces NaN from out-of-domain values.
func (s *Store) previewFor(card fsrs.Card, now time.Time) *IntervalPreview {
	sched := s.fsr.Repeat(card, now)
	// Same clamp as the persist path: buttons must promise what a rating does.
	minIvl := s.minNonAgainInterval()
	return &IntervalPreview{
		At:    now.Unix(),
		Again: sched[fsrs.Again].Card.Due.Unix(),
		Hard:  clampDue(sched[fsrs.Hard].Card, fsrs.Hard, now, minIvl).Due.Unix(),
		Good:  clampDue(sched[fsrs.Good].Card, fsrs.Good, now, minIvl).Due.Unix(),
		Easy:  clampDue(sched[fsrs.Easy].Card, fsrs.Easy, now, minIvl).Due.Unix(),
	}
}

const maxDueCards = 500

const dueCardSelect = `
	SELECT id, lemma, kanji, readings, glosses, sentence, source_url,
	       translation, explanation, mnemonic_parts, mnemonic_story,
	       card_type, surface,
	       due, stability, difficulty, elapsed_days, scheduled_days,
	       reps, lapses, state, last_review, audio_source,
	       audio_file, image_file
	FROM cards `

// GetDueCards returns up to limit due cards, oldest first, with no new-card gating.
func (s *Store) GetDueCards(limit int) ([]DueCard, error) {
	return s.getDue(limit, -1, nil)
}

// GetDueCardsGated caps never-reviewed (FSRS New) cards at newLimit per day;
// due review cards are never gated (a demoted or lapsed card is a review, not
// a new word). newLimit<0 disables the cap.
func (s *Store) GetDueCardsGated(limit, newLimit int) ([]DueCard, error) {
	return s.getDue(limit, newLimit, nil)
}

// GetDueCardsGatedTZ is GetDueCardsGated with the day boundary in the
// client's timezone (see dayStart).
func (s *Store) GetDueCardsGatedTZ(limit, newLimit int, tzOffsetMin *int) ([]DueCard, error) {
	return s.getDue(limit, newLimit, tzOffsetMin)
}

func (s *Store) getDue(limit, newLimit int, tzOffsetMin *int) ([]DueCard, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > maxDueCards {
		limit = maxDueCards
	}
	cards, err := s.getDuePage(limit, newLimit, tzOffsetMin)
	if err != nil {
		return nil, err
	}
	if err := s.attachSystemTags(cards); err != nil {
		return nil, err
	}
	if err := s.attachLastReviewIDs(cards); err != nil {
		return nil, err
	}
	if err := s.attachSharedMnemonics(cards); err != nil {
		return nil, err
	}
	return cards, nil
}

// attachSharedMnemonics gives cards without a mnemonic of their own the
// library's, flagged as shared.
func (s *Store) attachSharedMnemonics(cards []DueCard) error {
	err := s.fillSharedMnemonics(len(cards),
		func(i int) (string, *Mnemonic) { return cards[i].Lemma, cards[i].Mnemonic },
		func(i int, m *Mnemonic) { cards[i].Mnemonic, cards[i].MnemonicShared = m, true })
	if err != nil {
		return err
	}
	known, err := s.knownKanji()
	if err != nil {
		return err
	}
	for i := range cards {
		markKnown(cards[i].Mnemonic, known)
	}
	return nil
}

// attachLastReviewIDs fills LastReviewID from each card's newest review_log
// row in one batched query; untokened/legacy rows leave it "".
func (s *Store) attachLastReviewIDs(cards []DueCard) error {
	if len(cards) == 0 {
		return nil
	}
	byID := make(map[int64]int, len(cards))
	args := make([]any, 0, len(cards))
	for i := range cards {
		byID[cards[i].ID] = i
		args = append(args, cards[i].ID)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")
	rows, err := s.q().Query(`
		SELECT card_id, review_id FROM review_log
		WHERE id IN (
			SELECT MAX(id) FROM review_log
			WHERE card_id IN (`+placeholders+`) GROUP BY card_id
		)`, args...)
	if err != nil {
		return fmt.Errorf("load last review ids: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cardID int64
		var reviewID sql.NullString
		if err := rows.Scan(&cardID, &reviewID); err != nil {
			return fmt.Errorf("scan last review id: %w", err)
		}
		if i, ok := byID[cardID]; ok && reviewID.Valid {
			cards[i].LastReviewID = reviewID.String
		}
	}
	return rows.Err()
}

func (s *Store) getDuePage(limit, newLimit int, tzOffsetMin *int) ([]DueCard, error) {
	cut := s.dueCutAt(clientLocation(tzOffsetMin))

	if newLimit < 0 {
		return s.queryDueCards(dueCardSelect+`WHERE lang = ? AND `+dueSQL+` ORDER BY due ASC LIMIT ?`, s.lang, cut.dayEnd, cut.now, limit)
	}

	// New cards get their room reserved first and reviews fill what's left,
	// so a page-filling review pile can't starve the promised new words.
	introduced, err := s.newCardsIntroducedToday(time.Now(), tzOffsetMin)
	if err != nil {
		return nil, fmt.Errorf("count introduced: %w", err)
	}
	allowance := newLimit - introduced
	if allowance > limit {
		allowance = limit
	}
	var newer []DueCard
	if allowance > 0 {
		newer, err = s.queryDueCards(dueCardSelect+`WHERE lang = ? AND due <= ? AND state = 0 ORDER BY due ASC LIMIT ?`, s.lang, cut.now, allowance)
		if err != nil {
			return nil, err
		}
	}
	room := limit - len(newer)
	var reviews []DueCard
	if room > 0 {
		reviews, err = s.queryDueCards(dueCardSelect+`WHERE lang = ? AND state != 0 AND `+dueSQL+` ORDER BY due ASC LIMIT ?`, s.lang, cut.dayEnd, cut.now, room)
		if err != nil {
			return nil, err
		}
	}
	// Reviews still lead the session; the new words close it out.
	return append(reviews, newer...), nil
}

// maxTZOffsetMin bounds a client offset (real zones span UTC-12..+14);
// out-of-range values are clamped, not rejected.
const maxTZOffsetMin = 16 * 60

// dayStart returns midnight (unix seconds) of the day containing now — the
// boundary for the daily new-card quota and the undo walk. tzOffsetMin is the
// client's offset in minutes EAST of UTC (-getTimezoneOffset()); nil keeps
// server-local, and an explicit 0 is a real UTC zone, never "absent".
// Mirrors clientLocation in stats.go.
func dayStart(now time.Time, tzOffsetMin *int) int64 {
	loc := now.Location()
	if tzOffsetMin != nil {
		off := *tzOffsetMin
		if off > maxTZOffsetMin {
			off = maxTZOffsetMin
		} else if off < -maxTZOffsetMin {
			off = -maxTZOffsetMin
		}
		loc = time.FixedZone("client", off*60)
	}
	y, m, d := now.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc).Unix()
}

// newCardsIntroducedToday counts cards reviewed today while still FSRS-New
// (prev_state = 0). Counting the first log row would misclassify imported
// cards that carry scheduling history without local log rows.
func (s *Store) newCardsIntroducedToday(now time.Time, tzOffsetMin *int) (int, error) {
	var n int
	err := s.q().QueryRow(`
		SELECT COUNT(DISTINCT card_id) FROM review_log
		WHERE prev_state = 0 AND reviewed_at >= ?
	`, dayStart(now, tzOffsetMin)).Scan(&n)
	return n, err
}

// attachSystemTags fills SystemTags from kind='system' word_tags in one
// batched query; lemmas with none keep nil (field omitted).
func (s *Store) attachSystemTags(cards []DueCard) error {
	if len(cards) == 0 {
		return nil
	}
	byLemma := make(map[string][]int, len(cards))
	args := make([]any, 0, len(cards))
	for i := range cards {
		l := cards[i].Lemma
		if _, seen := byLemma[l]; !seen {
			args = append(args, l)
		}
		byLemma[l] = append(byLemma[l], i)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")
	rows, err := s.q().Query(
		`SELECT lemma, tag FROM word_tags WHERE kind = 'system' AND lang = ? AND lemma IN (`+placeholders+`) ORDER BY tag`,
		append([]any{s.lang}, args...)...,
	)
	if err != nil {
		return fmt.Errorf("load system tags: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var lemma, tag string
		if err := rows.Scan(&lemma, &tag); err != nil {
			return fmt.Errorf("scan system tag: %w", err)
		}
		for _, i := range byLemma[lemma] {
			cards[i].SystemTags = append(cards[i].SystemTags, tag)
			if tag == TagFurigana {
				cards[i].TryKanji = cards[i].steady
			}
		}
	}
	return rows.Err()
}

func (s *Store) queryDueCards(query string, args ...any) ([]DueCard, error) {
	nowT := time.Now()
	rows, err := s.q().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query due cards: %w", err)
	}
	defer rows.Close()

	out := []DueCard{}
	for rows.Next() {
		var c DueCard
		var kanjiJSON, readingsJSON, glossesJSON sql.NullString
		var sentence, sourceURL, surface, audioSource sql.NullString
		var translation, explanation, mnemonicParts, mnemonicStory sql.NullString
		var audioFile, imageFile sql.NullString
		var (
			dueEpoch                   int64
			stability, difficulty      float64
			elapsedDays, scheduledDays float64
			reps, lapses, state        int
			lastReview                 sql.NullInt64
		)
		if err := rows.Scan(
			&c.ID, &c.Lemma,
			&kanjiJSON, &readingsJSON, &glossesJSON,
			&sentence, &sourceURL,
			&translation, &explanation, &mnemonicParts, &mnemonicStory,
			&c.CardType, &surface,
			&dueEpoch, &stability, &difficulty, &elapsedDays, &scheduledDays,
			&reps, &lapses, &state, &lastReview, &audioSource,
			&audioFile, &imageFile,
		); err != nil {
			return nil, fmt.Errorf("scan due card: %w", err)
		}
		stability, difficulty = sanitizeFSRS(state, stability, difficulty)
		card := fsrs.Card{
			Due:           time.Unix(dueEpoch, 0),
			Stability:     stability,
			Difficulty:    difficulty,
			ElapsedDays:   uint64(elapsedDays),
			ScheduledDays: uint64(scheduledDays),
			Reps:          uint64(reps),
			Lapses:        uint64(lapses),
			State:         fsrs.State(state),
		}
		if lastReview.Valid {
			card.LastReview = time.Unix(lastReview.Int64, 0)
		}
		c.IsNew = fsrs.State(state) == fsrs.New
		c.Mature = fsrs.State(state) == fsrs.Review && scheduledDays >= 21
		c.steady = fsrs.State(state) == fsrs.Review && scheduledDays >= furiganaSteadyDays
		c.Preview = s.previewFor(card, nowT)
		c.Kanji = decodeStrArray(kanjiJSON)
		c.Readings = decodeStrArray(readingsJSON)
		c.Glosses = decodeGlosses(glossesJSON)
		if sentence.Valid {
			s := sentence.String
			c.Sentence = &s
		}
		if sourceURL.Valid {
			s := sourceURL.String
			c.SourceURL = &s
		}
		if translation.Valid && translation.String != "" {
			s := translation.String
			c.Translation = &s
		}
		if explanation.Valid && explanation.String != "" {
			s := explanation.String
			c.Explanation = &s
		}
		c.Mnemonic = decodeMnemonic(mnemonicParts, mnemonicStory)
		if surface.Valid {
			s := surface.String
			c.Surface = &s
		}
		if c.CardType == "cloze" && c.Sentence != nil {
			if t := clozeTargetFor(*c.Sentence, c.Surface, c.Lemma, c.Kanji); t != "" {
				c.ClozeTarget = &t
			}
		}
		if audioSource.Valid && audioSource.String != "" {
			as := audioSource.String
			c.AudioSource = &as
		}
		// Media paths are a "has media" signal; bytes come via get_card_media.
		if audioFile.String != "" {
			c.AudioPath = &audioFile.String
		}
		if imageFile.String != "" {
			c.ImagePath = &imageFile.String
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ReviewRating is the wire rating: 1 = Again, 2 = Hard, 3 = Good, 4 = Easy.
type ReviewRating int

const (
	RatingAgain ReviewRating = 1
	RatingHard  ReviewRating = 2
	RatingGood  ReviewRating = 3
	RatingEasy  ReviewRating = 4
)

// Auto-graduation: Learning→Known once state == Review, stability ≥
// knownStabilityDays (Anki's "mature" line) and reps ≥ knownMinReps (so one
// Easy can't crown a word). Known→Learning on the Tuning demote_after_lapses-th
// lapse from Review within demote_lapse_window_days: by default one miss
// reschedules but keeps the label (WaniKani and Bunpro's "minimal" mode
// charge sessions, not answers), and same-session laps never count. These
// are the only word_status writes in the review loop.
const (
	knownStabilityDays = 21.0
	knownMinReps       = 4
)

// ReviewResult is the /review_card response; NewDue is RFC3339.
type ReviewResult struct {
	CardID   int64  `json:"card_id"`
	NewDue   string `json:"new_due"`
	NewState int    `json:"new_state"`
	// Promoted: this review moved the lemma Learning→Known.
	Promoted bool `json:"promoted"`
	// Demoted: this review moved the lemma Known→Learning.
	Demoted bool `json:"demoted"`
	// Furigana: this review switched the card to furigana on the front.
	Furigana bool `json:"furigana,omitempty"`
	// Preview is the POST-review interval preview; a client re-showing the
	// card in the same sitting must relabel its buttons from here.
	Preview *IntervalPreview `json:"preview,omitempty"`
	// Conflict: prev_review_id no longer matched the newest review, so
	// nothing was applied. The client drops the rating, not retries.
	Conflict bool `json:"conflict,omitempty"`
}

// ReviewCard applies a rating through FSRS and persists it in one transaction.
func (s *Store) ReviewCard(cardID int64, rating ReviewRating) (ReviewResult, error) {
	return s.reviewCardIdemAt(cardID, rating, "", nil, time.Now())
}

// ReviewCardIdem is ReviewCard with an idempotency token: a reviewID already
// in review_log is answered from current state instead of applying FSRS twice.
func (s *Store) ReviewCardIdem(cardID int64, rating ReviewRating, reviewID string) (ReviewResult, error) {
	return s.reviewCardIdemAt(cardID, rating, reviewID, nil, time.Now())
}

// ReviewCardGuarded is ReviewCardIdem with an optimistic-concurrency check:
// prevReviewID is the newest review_id as the client last saw it ("" = none).
// On mismatch nothing is applied and Conflict is set. The idempotency check
// runs first so a committed-but-unacknowledged retry can't conflict with itself.
func (s *Store) ReviewCardGuarded(cardID int64, rating ReviewRating, reviewID, prevReviewID string) (ReviewResult, error) {
	return s.reviewCardIdemAt(cardID, rating, reviewID, &prevReviewID, time.Now())
}

// ReviewCardGuardedAt is ReviewCardGuarded with the offline rating's own time.
func (s *Store) ReviewCardGuardedAt(cardID int64, rating ReviewRating, reviewID, prevReviewID string, reviewedAt time.Time) (ReviewResult, error) {
	return s.reviewCardIdemAtChecked(cardID, rating, reviewID, &prevReviewID, reviewedAt, true)
}

// reviewCardAt is ReviewCard with an injectable clock (simulation tests).
func (s *Store) reviewCardAt(cardID int64, rating ReviewRating, now time.Time) (ReviewResult, error) {
	return s.reviewCardIdemAt(cardID, rating, "", nil, now)
}

func (s *Store) reviewCardIdemAt(cardID int64, rating ReviewRating, reviewID string, prevReviewID *string, now time.Time) (ReviewResult, error) {
	return s.reviewCardIdemAtChecked(cardID, rating, reviewID, prevReviewID, now, false)
}

func (s *Store) reviewCardIdemAtChecked(cardID int64, rating ReviewRating, reviewID string, prevReviewID *string, now time.Time, enforceChronology bool) (ReviewResult, error) {
	if rating < 1 || rating > 4 {
		return ReviewResult{}, clientErrf("invalid rating %d (expected 1-4)", rating)
	}
	if err := validateReviewID(reviewID); err != nil {
		return ReviewResult{}, err
	}
	// Every logged review must carry a token: "" is also the guard's "no
	// reviews yet" sentinel, so an untokened tip would let a stale offline
	// rating replay undetected. Minted tokens skip the idempotency lookup.
	minted := false
	if reviewID == "" {
		buf := make([]byte, 16)
		if _, err := rand.Read(buf); err != nil {
			return ReviewResult{}, fmt.Errorf("mint review_id: %w", err)
		}
		reviewID = "srv-" + hex.EncodeToString(buf)
		minted = true
	}

	tx, err := s.q().Begin()
	if err != nil {
		return ReviewResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		lemma         string
		dueEpoch      int64
		stability     float64
		difficulty    float64
		elapsedDays   float64
		scheduledDays float64
		reps          int
		lapses        int
		state         int
		lastReview    sql.NullInt64
	)
	err = tx.QueryRow(`
		SELECT lemma, due, stability, difficulty, elapsed_days, scheduled_days,
		       reps, lapses, state, last_review
		FROM cards WHERE id = ?
	`, cardID).Scan(
		&lemma, &dueEpoch, &stability, &difficulty, &elapsedDays, &scheduledDays,
		&reps, &lapses, &state, &lastReview,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ReviewResult{}, clientErrf("card %d not found", cardID)
	}
	if err != nil {
		return ReviewResult{}, fmt.Errorf("load card: %w", err)
	}

	stability, difficulty = sanitizeFSRS(state, stability, difficulty)
	card := fsrs.Card{
		Due:           time.Unix(dueEpoch, 0),
		Stability:     stability,
		Difficulty:    difficulty,
		ElapsedDays:   uint64(elapsedDays),
		ScheduledDays: uint64(scheduledDays),
		Reps:          uint64(reps),
		Lapses:        uint64(lapses),
		State:         fsrs.State(state),
	}
	if lastReview.Valid {
		card.LastReview = time.Unix(lastReview.Int64, 0)
	}

	// Idempotent replay: the current row is already the post-review state.
	// Promoted/Demoted stay false — the side effect already happened.
	if !minted {
		var loggedCard int64
		err := tx.QueryRow(`SELECT card_id FROM review_log WHERE review_id = ?`, reviewID).Scan(&loggedCard)
		switch {
		case err == nil && loggedCard == cardID:
			return ReviewResult{
				CardID:   cardID,
				NewDue:   time.Unix(dueEpoch, 0).Format(time.RFC3339),
				NewState: state,
				Preview:  s.previewFor(card, now),
			}, nil
		case err == nil:
			return ReviewResult{}, clientErrf("review_id already used by card %d", loggedCard)
		case !errors.Is(err, sql.ErrNoRows):
			return ReviewResult{}, fmt.Errorf("check review_id: %w", err)
		}
	}

	// Optimistic-concurrency guard: if the review chain's tip moved since the
	// client chose this rating, report Conflict and apply nothing. Must run
	// after the idempotency replay.
	if prevReviewID != nil {
		var lastID sql.NullString
		err := tx.QueryRow(
			`SELECT review_id FROM review_log WHERE card_id = ? ORDER BY id DESC LIMIT 1`,
			cardID,
		).Scan(&lastID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return ReviewResult{}, fmt.Errorf("check review chain: %w", err)
		}
		current := ""
		if lastID.Valid {
			current = lastID.String
		}
		if current != *prevReviewID {
			return ReviewResult{
				CardID:   cardID,
				NewDue:   time.Unix(dueEpoch, 0).Format(time.RFC3339),
				NewState: state,
				Conflict: true,
				Preview:  s.previewFor(card, now),
			}, nil
		}
	}
	if enforceChronology && lastReview.Valid && now.Before(card.LastReview) {
		return ReviewResult{}, clientCodef(ErrCodeReviewedAtRejected, "reviewed_at precedes the card's last review")
	}

	// card still holds the pre-Repeat state, which floorMatureLapse needs.
	scheduling := s.fsr.Repeat(card, now)
	next := clampDue(scheduling[mapRating(rating)].Card, mapRating(rating), now, s.minNonAgainInterval())
	next = floorMatureLapse(next, card, mapRating(rating), s.tune.MatureLapseFloor, s.tune.MatureLapseThresholdDays)

	_, err = tx.Exec(`
		UPDATE cards
		SET due           = ?,
		    stability     = ?,
		    difficulty    = ?,
		    elapsed_days  = ?,
		    scheduled_days = ?,
		    reps          = ?,
		    lapses        = ?,
		    state         = ?,
		    last_review   = ?
		WHERE id = ?
	`,
		next.Due.Unix(),
		next.Stability,
		next.Difficulty,
		float64(next.ElapsedDays),
		float64(next.ScheduledDays),
		int(next.Reps),
		int(next.Lapses),
		int(next.State),
		now.Unix(),
		cardID,
	)
	if err != nil {
		return ReviewResult{}, fmt.Errorf("persist review: %w", err)
	}

	// Each UPDATE's WHERE status clause scopes it to the expected starting
	// state, so a manual Known/Ignored or a never-studied lemma is never
	// touched.
	promoted, demoted := false, false
	// Logged with the review so undo can revert the side effect.
	var wordStatusPrior sql.NullInt64
	switch fsrs.State(next.State) {
	case fsrs.Review:
		if next.Stability >= knownStabilityDays && next.Reps >= knownMinReps {
			res, err := tx.Exec(`
				UPDATE word_status SET status = 2, updated_at = ?
				WHERE lang = (SELECT lang FROM cards WHERE id = ?) AND lemma = (SELECT lemma FROM cards WHERE id = ?) AND status = 1
				AND NOT EXISTS (SELECT 1 FROM word_tags t WHERE t.lang = word_status.lang AND t.lemma = word_status.lemma AND t.tag = ?)
			`, now.Unix(), cardID, cardID, TagFurigana)
			if err != nil {
				return ReviewResult{}, fmt.Errorf("auto-promote: %w", err)
			}
			if n, err := res.RowsAffected(); err == nil && n > 0 {
				promoted = true
				wordStatusPrior = sql.NullInt64{Int64: 1, Valid: true}
			}
		}
	case fsrs.Relearning:
		if card.State != fsrs.Review {
			break
		}
		// This review is not logged yet, so the count is earlier lapses only.
		var earlier int
		window := time.Duration(s.tune.DemoteLapseWindowDays * float64(24*time.Hour))
		if err := tx.QueryRow(`
			SELECT COUNT(*) FROM review_log
			WHERE card_id = ? AND rating = 1 AND prev_state = ? AND reviewed_at > ?
		`, cardID, int(fsrs.Review), now.Add(-window).Unix()).Scan(&earlier); err != nil {
			return ReviewResult{}, fmt.Errorf("auto-demote: %w", err)
		}
		if earlier+1 < s.tune.DemoteAfterLapses {
			break
		}
		res, err := tx.Exec(`
			UPDATE word_status SET status = 1, updated_at = ?
			WHERE lang = (SELECT lang FROM cards WHERE id = ?) AND lemma = (SELECT lemma FROM cards WHERE id = ?) AND status = 2
		`, now.Unix(), cardID, cardID)
		if err != nil {
			return ReviewResult{}, fmt.Errorf("auto-demote: %w", err)
		}
		if n, err := res.RowsAffected(); err == nil && n > 0 {
			demoted = true
			wordStatusPrior = sql.NullInt64{Int64: 2, Valid: true}
		}
	}

	furigana := false
	if rating == RatingAgain && int(next.Lapses) >= furiganaAfterLapses {
		if furigana, err = switchToFurigana(tx, s.lang, lemma, now); err != nil {
			return ReviewResult{}, err
		}
		if furigana && !demoted {
			if n, err := demoteKnown(tx, s.lang, lemma, now); err != nil {
				return ReviewResult{}, err
			} else if n {
				demoted = true
				wordStatusPrior = sql.NullInt64{Int64: 2, Valid: true}
			}
		}
	}

	// The prev_* snapshot is what UndoReview restores; it holds the sanitized
	// values the scheduler saw, so undo keeps a repaired row repaired.
	_, err = tx.Exec(`
		INSERT INTO review_log (card_id, rating, reviewed_at, review_id,
			prev_due, prev_stability, prev_difficulty, prev_elapsed_days,
			prev_scheduled_days, prev_reps, prev_lapses, prev_state,
			prev_last_review, word_status_prior)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cardID, int(rating), now.Unix(), nullIfEmpty(reviewID),
		dueEpoch, stability, difficulty, elapsedDays, scheduledDays,
		reps, lapses, state, nullableEpoch(lastReview), wordStatusPrior,
	)
	if err != nil {
		return ReviewResult{}, fmt.Errorf("log review: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return ReviewResult{}, fmt.Errorf("commit review: %w", err)
	}

	// Same as a manual mark-Known; undo leaves the card removed.
	if promoted {
		s.retireWatchWord(lemma)
	}

	return ReviewResult{
		CardID:   cardID,
		NewDue:   next.Due.Format(time.RFC3339),
		NewState: int(next.State),
		Promoted: promoted,
		Demoted:  demoted,
		Furigana: furigana,
		Preview:  s.previewFor(next, now),
	}, nil
}

// UndoReview reverts the newest review: restores the prev_* FSRS snapshot,
// reverts any auto-promote/demote, and deletes the log row. Repeatable, but
// bounded to today's reviews (dayStart) — yesterday's schedule is settled.
// Reviews of deleted cards are skipped; rows without a snapshot end the walk.
func (s *Store) UndoReview() (int64, error) {
	cardID, _, err := s.undoReviewAt(time.Now(), nil)
	return cardID, err
}

// UndoReviewTZ is UndoReview with the day boundary in the client's zone; it
// also returns the undone review_id so the client can verify which one went.
func (s *Store) UndoReviewTZ(tzOffsetMin *int) (int64, string, error) {
	return s.undoReviewAt(time.Now(), tzOffsetMin)
}

func (s *Store) undoReviewAt(now time.Time, tzOffsetMin *int) (int64, string, error) {
	tx, err := s.q().Begin()
	if err != nil {
		return 0, "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		logID, cardID   int64
		lemma           string
		reviewID        sql.NullString
		prevDue         sql.NullInt64
		prevStability   sql.NullFloat64
		prevDifficulty  sql.NullFloat64
		prevElapsed     sql.NullFloat64
		prevScheduled   sql.NullFloat64
		prevReps        sql.NullInt64
		prevLapses      sql.NullInt64
		prevState       sql.NullInt64
		prevLastReview  sql.NullInt64
		wordStatusPrior sql.NullInt64
	)
	// The JOIN skips reviews of deleted cards.
	err = tx.QueryRow(`
		SELECT rl.id, rl.card_id, c.lemma, rl.review_id,
		       rl.prev_due, rl.prev_stability, rl.prev_difficulty,
		       rl.prev_elapsed_days, rl.prev_scheduled_days,
		       rl.prev_reps, rl.prev_lapses, rl.prev_state,
		       rl.prev_last_review, rl.word_status_prior
		FROM review_log rl JOIN cards c ON c.id = rl.card_id
		WHERE rl.reviewed_at >= ?
		ORDER BY rl.id DESC LIMIT 1
	`, dayStart(now, tzOffsetMin)).Scan(
		&logID, &cardID, &lemma, &reviewID,
		&prevDue, &prevStability, &prevDifficulty,
		&prevElapsed, &prevScheduled,
		&prevReps, &prevLapses, &prevState,
		&prevLastReview, &wordStatusPrior,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", clientErrf("nothing to undo")
	}
	if err != nil {
		return 0, "", fmt.Errorf("load last review: %w", err)
	}
	if !prevDue.Valid {
		// Legacy row (pre-snapshot) — nothing safe to restore from.
		return 0, "", clientErrf("nothing to undo")
	}

	if _, err := tx.Exec(`
		UPDATE cards
		SET due = ?, stability = ?, difficulty = ?, elapsed_days = ?,
		    scheduled_days = ?, reps = ?, lapses = ?, state = ?, last_review = ?
		WHERE id = ?
	`,
		prevDue.Int64, prevStability.Float64, prevDifficulty.Float64,
		prevElapsed.Float64, prevScheduled.Float64, prevReps.Int64,
		prevLapses.Int64, prevState.Int64, nullableEpoch(prevLastReview), cardID,
	); err != nil {
		return 0, "", fmt.Errorf("restore card: %w", err)
	}
	// Scoped by the status the review left behind: a manual change since
	// then wins and is left alone.
	if wordStatusPrior.Valid {
		reviewSet := int64(2) // promotion wrote Known
		if wordStatusPrior.Int64 == 2 {
			reviewSet = 1 // demotion wrote Learning
		}
		if _, err := tx.Exec(`
			UPDATE word_status SET status = ?, updated_at = ?
			WHERE lang = ? AND lemma = ? AND status = ?
		`, wordStatusPrior.Int64, now.Unix(), s.lang, lemma, reviewSet); err != nil {
			return 0, "", fmt.Errorf("restore word status: %w", err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM review_log WHERE id = ?`, logID); err != nil {
		return 0, "", fmt.Errorf("remove log row: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, "", fmt.Errorf("commit undo: %w", err)
	}
	return cardID, reviewID.String, nil
}

// validateReviewID bounds the client-supplied idempotency token: opaque, but
// short and printable so it can't smuggle junk into the log table.
func validateReviewID(id string) error {
	if id == "" {
		return nil
	}
	if len(id) > 64 {
		return clientErrf("review_id too long (%d > 64)", len(id))
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.', r == ':':
		default:
			return clientErrf("review_id contains invalid character %q", r)
		}
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// sanitizeFSRS repairs out-of-domain FSRS values (stability <= 0, difficulty
// outside [1,10]) on non-new cards — go-fsrs produces NaN from them, and
// imports/seeds can create them.
func sanitizeFSRS(state int, stability, difficulty float64) (float64, float64) {
	if fsrs.State(state) == fsrs.New {
		return stability, difficulty
	}
	// NaN/Inf fail ordered comparisons and need explicit checks; modernc/sqlite
	// persists a NaN REAL as NULL, which breaks every float64 scan of the row.
	if math.IsNaN(stability) || math.IsInf(stability, 0) || stability <= 0 {
		stability = 0.1
	}
	if math.IsNaN(difficulty) || math.IsInf(difficulty, -1) || difficulty < 1 {
		difficulty = 5
	} else if math.IsInf(difficulty, 1) || difficulty > 10 {
		difficulty = 10
	}
	return stability, difficulty
}

func nullableEpoch(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

func mapRating(r ReviewRating) fsrs.Rating {
	switch r {
	case RatingAgain:
		return fsrs.Again
	case RatingHard:
		return fsrs.Hard
	case RatingGood:
		return fsrs.Good
	case RatingEasy:
		return fsrs.Easy
	default:
		return fsrs.Good
	}
}

// --- JSON column helpers --------------------------------------------------

// decodeStrArray reads a JSON string-array TEXT column. Always non-nil so the
// field serializes as [] — the frontend indexes [0]. NULL/empty/malformed → [].
func decodeStrArray(s sql.NullString) []string {
	if !s.Valid || strings.TrimSpace(s.String) == "" {
		return []string{}
	}
	var arr []string
	if err := json.Unmarshal([]byte(s.String), &arr); err != nil || arr == nil {
		return []string{}
	}
	return arr
}

// decodeGlosses accepts the per-sense shape [["work","job"],…] and the legacy
// flat ["work","job"] (wrapped as one sense).
func decodeGlosses(s sql.NullString) [][]string {
	empty := [][]string{} // [] not null — frontend indexes this
	if !s.Valid || strings.TrimSpace(s.String) == "" {
		return empty
	}
	var nested [][]string
	if err := json.Unmarshal([]byte(s.String), &nested); err == nil {
		if nested == nil {
			return empty
		}
		return nested
	}
	var flat []string
	if err := json.Unmarshal([]byte(s.String), &flat); err == nil {
		if len(flat) == 0 {
			return empty
		}
		return [][]string{flat}
	}
	return empty
}

// clozeTargetFor picks what a cloze card hides: the surface form saved when
// the card was made (it covers conjugation), else the headword, else a kanji
// spelling; "" when none appears in the sentence.
func clozeTargetFor(sentence string, surface *string, lemma string, kanji []string) string {
	var candidates []string
	if surface != nil {
		candidates = append(candidates, *surface)
	}
	candidates = append(candidates, lemma)
	candidates = append(candidates, kanji...)
	for _, c := range candidates {
		if c != "" && strings.Contains(sentence, c) {
			return c
		}
	}
	return ""
}
