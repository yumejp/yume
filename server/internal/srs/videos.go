package srs

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Video library — persisted per-video coverage snapshots, the "watch next"
// ranking feature. A client tokenizes a video's cues once and sends a
// compact snapshot (per-cue unique CONTENT lemmas); the server stores it and,
// on every list, recomputes coverage / i+1 / maturing-soon against the
// learner's CURRENT word statuses. Statuses change as the learner studies, so
// the same snapshot ranks better every day — nothing derived is ever stored.

// Snapshot caps. The client sends tokenizer output, but the wire is open —
// reject anything beyond these with a clear error instead of storing junk.
const (
	maxVideoTitleChars = 300    // title length (runes)
	maxVideoCues       = 20000  // cues per snapshot
	maxVideoLemmas     = 200000 // total lemmas across all cues
	maxVideoLemmaChars = 100    // single lemma length (runes)
)

// VideoSnapshotReq mirrors the /save_video_snapshot wire shape.
type VideoSnapshotReq struct {
	VideoID  string `json:"video_id"` // platform-scoped, e.g. "site:abc123"
	URL      string `json:"url"`
	Title    string `json:"title"`
	Platform string `json:"platform"` // the client's source name, e.g. "site" | "file"
	Lang     string `json:"lang,omitempty"`
	// Per-cue unique content lemmas (the client pre-filters particles,
	// punctuation, auxiliaries — the same rule as language.IsContent).
	CueLemmas [][]string `json:"cue_lemmas"`
}

// VideoStats is one /list_video_snapshots row: the stored snapshot fields plus
// the stats recomputed fresh against current word_status / cards.
type VideoStats struct {
	VideoID  string `json:"video_id"`
	URL      string `json:"url"`
	Title    string `json:"title"`
	Platform string `json:"platform"`
	Lang     string `json:"lang"`
	CueCount int    `json:"cue_count"`
	// Unique content lemmas in the video, ignored lemmas excluded. Always
	// equals known + learning + unknown.
	UniqueLemmas int `json:"unique_lemmas"`
	Known        int `json:"known"`
	Learning     int `json:"learning"`
	Unknown      int `json:"unknown"`
	// known / (known+learning+unknown) * 100, one decimal; 0 when the
	// denominator is 0.
	CoveragePct float64 `json:"coverage_pct"`
	// Cues whose unique lemmas contain EXACTLY ONE unknown — the i+1 signal.
	I1Cues int `json:"i1_cues"`
	// = Unknown; the client-friendly alias for "words you'd meet for the
	// first time".
	NewWords int `json:"new_words"`
	// Of this video's Learning lemmas, how many have a card due within the
	// next 14 days — the "this video gets easier soon" forecast seed.
	MaturingSoon int   `json:"maturing_soon"`
	UpdatedAt    int64 `json:"updated_at"` // epoch seconds
	CreatedAt    int64 `json:"created_at"` // epoch seconds
}

// maturingSoonWindow is how far ahead a Learning lemma's card due date may be
// and still count toward MaturingSoon. Overdue cards count too — they're
// about to be reviewed.
const maturingSoonWindow = 14 * 24 * time.Hour

// SaveVideoSnapshot upserts a snapshot on video_id: an update refreshes
// url/title/cue_lemmas/cue_count/updated_at and preserves created_at.
// Returns the canonical (trimmed) video id and whether an existing row was
// updated (false on first insert).
func (s *Store) SaveVideoSnapshot(req VideoSnapshotReq) (videoID string, updated bool, err error) {
	videoID = strings.TrimSpace(req.VideoID)
	url := strings.TrimSpace(req.URL)
	title := strings.TrimSpace(req.Title)
	platform := strings.TrimSpace(req.Platform)
	lang := strings.TrimSpace(req.Lang)
	if lang == "" {
		lang = "ja"
	}
	if videoID == "" {
		return "", false, clientErrf("video_id is required")
	}
	if url == "" {
		return "", false, clientErrf("url is required")
	}
	if title == "" {
		return "", false, clientErrf("title is required")
	}
	if n := utf8.RuneCountInString(title); n > maxVideoTitleChars {
		return "", false, clientErrf("title too long: %d chars (max %d)", n, maxVideoTitleChars)
	}
	if len(req.CueLemmas) > maxVideoCues {
		return "", false, clientErrf("too many cues: %d (max %d)", len(req.CueLemmas), maxVideoCues)
	}
	total := 0
	for _, cue := range req.CueLemmas {
		total += len(cue)
		if total > maxVideoLemmas {
			return "", false, clientErrf("too many lemmas: over %d total (max %d)", total, maxVideoLemmas)
		}
		for _, l := range cue {
			if n := utf8.RuneCountInString(l); n > maxVideoLemmaChars {
				return "", false, clientErrf("lemma too long: %d chars (max %d)", n, maxVideoLemmaChars)
			}
		}
	}
	// Compact JSON (json.Marshal emits no whitespace); never nil so the
	// NOT NULL column always gets at least "[]".
	cues := req.CueLemmas
	if cues == nil {
		cues = [][]string{}
	}
	blob, err := json.Marshal(cues)
	if err != nil {
		return "", false, fmt.Errorf("encode cue_lemmas: %w", err)
	}

	now := time.Now().Unix()
	tx, err := s.q().Begin()
	if err != nil {
		return "", false, err
	}
	defer func() { _ = tx.Rollback() }()
	var existing int64
	err = tx.QueryRow(`SELECT id FROM video_snapshots WHERE video_id = ?`, videoID).Scan(&existing)
	switch {
	case err == nil:
		if _, err := tx.Exec(
			`UPDATE video_snapshots
			    SET url = ?, title = ?, cue_lemmas = ?, cue_count = ?, updated_at = ?
			  WHERE id = ?`,
			url, title, string(blob), len(cues), now, existing,
		); err != nil {
			return "", false, fmt.Errorf("update video snapshot: %w", err)
		}
		updated = true
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.Exec(
			`INSERT INTO video_snapshots
			   (video_id, url, title, platform, lang, cue_lemmas, cue_count, updated_at, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			videoID, url, title, platform, lang, string(blob), len(cues), now, now,
		); err != nil {
			return "", false, fmt.Errorf("insert video snapshot: %w", err)
		}
	default:
		return "", false, fmt.Errorf("look up video snapshot: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", false, err
	}
	return videoID, updated, nil
}

// ListVideoSnapshots returns every snapshot for the language ("" → "ja") with
// stats computed FRESH against the current word_status + cards tables —
// this is the whole point: rankings improve as the learner studies. Sorted by
// updated_at DESC (the client does presentation sorting).
func (s *Store) ListVideoSnapshots(lang string) ([]VideoStats, error) {
	lang = strings.TrimSpace(lang)
	if lang == "" {
		lang = "ja"
	}
	rows, err := s.q().Query(
		`SELECT video_id, url, title, platform, lang, cue_lemmas, cue_count, updated_at, created_at
		   FROM video_snapshots
		  WHERE lang = ?
		  ORDER BY updated_at DESC, id DESC`, lang)
	if err != nil {
		return nil, fmt.Errorf("query video snapshots: %w", err)
	}
	defer rows.Close()

	type snapshot struct {
		stats VideoStats
		cues  [][]string
	}
	var snaps []snapshot
	for rows.Next() {
		var sn snapshot
		var cueJSON string
		if err := rows.Scan(
			&sn.stats.VideoID, &sn.stats.URL, &sn.stats.Title, &sn.stats.Platform,
			&sn.stats.Lang, &cueJSON, &sn.stats.CueCount,
			&sn.stats.UpdatedAt, &sn.stats.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan video snapshot: %w", err)
		}
		if err := json.Unmarshal([]byte(cueJSON), &sn.cues); err != nil {
			return nil, fmt.Errorf("decode cue_lemmas for %s: %w", sn.stats.VideoID, err)
		}
		snaps = append(snaps, sn)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// One batched status read for the union of unique lemmas across ALL
	// videos — absent lemma = implicit unknown (0).
	union := make(map[string]struct{})
	for _, sn := range snaps {
		for _, cue := range sn.cues {
			for _, l := range cue {
				union[l] = struct{}{}
			}
		}
	}
	unionList := make([]string, 0, len(union))
	for l := range union {
		unionList = append(unionList, l)
	}
	statusOf, err := s.wordStatusMapChunked(unionList)
	if err != nil {
		return nil, err
	}

	// One batched due read for the Learning subset (MaturingSoon only looks
	// at Learning lemmas, so don't drag the whole union through cards).
	var learningLemmas []string
	for _, l := range unionList {
		if statusOf[l] == 1 {
			learningLemmas = append(learningLemmas, l)
		}
	}
	dueOf, err := s.minCardDueChunked(learningLemmas)
	if err != nil {
		return nil, err
	}
	horizon := time.Now().Add(maturingSoonWindow).Unix()

	out := make([]VideoStats, 0, len(snaps))
	for _, sn := range snaps {
		st := sn.stats

		// Unique lemmas across the video, counted by status. Ignored (3)
		// lemmas are excluded from every count and from the denominator.
		uniq := make(map[string]struct{})
		for _, cue := range sn.cues {
			for _, l := range cue {
				uniq[l] = struct{}{}
			}
		}
		for l := range uniq {
			switch statusOf[l] {
			case 2:
				st.Known++
			case 1:
				st.Learning++
				if due, ok := dueOf[l]; ok && due <= horizon {
					st.MaturingSoon++
				}
			case 3:
				// ignored — excluded entirely
			default:
				st.Unknown++
			}
		}
		st.UniqueLemmas = st.Known + st.Learning + st.Unknown
		st.NewWords = st.Unknown
		if st.UniqueLemmas > 0 {
			st.CoveragePct = math.Round(float64(st.Known)/float64(st.UniqueLemmas)*1000) / 10
		}

		// i+1 cues: exactly one unknown among the cue's unique lemmas.
		// Clients send per-cue-unique lists, but the wire is open — dedupe
		// defensively so a duplicated unknown can't hide an i+1 cue.
		for _, cue := range sn.cues {
			unknowns := 0
			seen := make(map[string]struct{}, len(cue))
			for _, l := range cue {
				if _, dup := seen[l]; dup {
					continue
				}
				seen[l] = struct{}{}
				switch statusOf[l] {
				case 1, 2, 3: // learning / known / ignored — not unknown
				default:
					unknowns++
				}
			}
			if unknowns == 1 {
				st.I1Cues++
			}
		}
		out = append(out, st)
	}
	return out, nil
}

// VideoVocabWord is one row of GetVideoVocab: a lemma occurring in a video,
// with the two signals that make it personally worth (or not worth) learning.
type VideoVocabWord struct {
	Lemma string `json:"lemma"`
	// Status is the learner's word status: 0 unknown, 1 learning, 2 known,
	// 3 ignored (absent word_status row = 0).
	Status int `json:"status"`
	// InVideoCues counts how many of THIS video's cues contain the lemma —
	// in-video frequency.
	InVideoCues int `json:"in_video_cues"`
	// LibraryVideos counts how many videos in the learner's library (same
	// lang, INCLUDING this one) contain the lemma — "library reach", the
	// personal-usefulness signal: a word in many of their own videos pays off
	// everywhere they watch.
	LibraryVideos int `json:"library_videos"`
}

// maxVideoVocabWords caps GetVideoVocab's result — enough for any curation
// pass; the sort puts the highest-reach words first so the tail that gets cut
// is the least interesting.
const maxVideoVocabWords = 500

// GetVideoVocab returns the words of one video filtered to the given statuses
// (nil/empty → {0 unknown, 1 learning}: the study-relevant default; ignored
// (3) only appears when explicitly requested), each with its in-video cue
// count and library reach. Sorted LibraryVideos DESC, then InVideoCues DESC,
// then lemma; capped at maxVideoVocabWords. Errors with "video not found"
// when no snapshot exists for videoID.
func (s *Store) GetVideoVocab(videoID string, statuses []int) ([]VideoVocabWord, error) {
	videoID = strings.TrimSpace(videoID)
	if videoID == "" {
		return nil, clientErrf("video_id is required")
	}
	var lang, cueJSON string
	err := s.q().QueryRow(
		`SELECT lang, cue_lemmas FROM video_snapshots WHERE video_id = ?`, videoID,
	).Scan(&lang, &cueJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, clientErrf("video not found: %s", videoID)
	}
	if err != nil {
		return nil, fmt.Errorf("look up video snapshot: %w", err)
	}
	var cues [][]string
	if err := json.Unmarshal([]byte(cueJSON), &cues); err != nil {
		return nil, fmt.Errorf("decode cue_lemmas for %s: %w", videoID, err)
	}

	// In-video frequency: per unique lemma, the number of cues containing it.
	// Clients send per-cue-unique lists, but the wire is open — dedupe within
	// each cue so a duplicated lemma can't double-count.
	inCues := make(map[string]int)
	for _, cue := range cues {
		seen := make(map[string]struct{}, len(cue))
		for _, l := range cue {
			if _, dup := seen[l]; dup {
				continue
			}
			seen[l] = struct{}{}
			inCues[l]++
		}
	}

	// Batched status read; absent row = implicit unknown (0).
	lemmas := make([]string, 0, len(inCues))
	for l := range inCues {
		lemmas = append(lemmas, l)
	}
	statusOf, err := s.wordStatusMapChunked(lemmas)
	if err != nil {
		return nil, err
	}

	want := make(map[int]struct{}, len(statuses))
	if len(statuses) == 0 {
		want[0] = struct{}{}
		want[1] = struct{}{}
	} else {
		for _, st := range statuses {
			want[st] = struct{}{}
		}
	}
	keep := make(map[string]struct{}, len(lemmas))
	for _, l := range lemmas {
		if _, ok := want[statusOf[l]]; ok {
			keep[l] = struct{}{}
		}
	}

	// Library reach: across ALL of the learner's same-lang snapshots, count
	// the distinct videos whose lemma set contains each kept lemma (the
	// target video counts toward its own reach). This re-parses every
	// snapshot's cue_lemmas JSON per call — fine at library scale (dozens of
	// videos); revisit with a lemma→video index table if libraries ever grow
	// to thousands.
	reach := make(map[string]int, len(keep))
	rows, err := s.q().Query(`SELECT cue_lemmas FROM video_snapshots WHERE lang = ?`, lang)
	if err != nil {
		return nil, fmt.Errorf("query library snapshots: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var blob string
		if err := rows.Scan(&blob); err != nil {
			return nil, err
		}
		var libCues [][]string
		if err := json.Unmarshal([]byte(blob), &libCues); err != nil {
			return nil, fmt.Errorf("decode library cue_lemmas: %w", err)
		}
		counted := make(map[string]struct{})
		for _, cue := range libCues {
			for _, l := range cue {
				if _, ok := keep[l]; !ok {
					continue
				}
				if _, done := counted[l]; done {
					continue
				}
				counted[l] = struct{}{}
				reach[l]++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]VideoVocabWord, 0, len(keep))
	for l := range keep {
		out = append(out, VideoVocabWord{
			Lemma:         l,
			Status:        statusOf[l],
			InVideoCues:   inCues[l],
			LibraryVideos: reach[l],
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LibraryVideos != out[j].LibraryVideos {
			return out[i].LibraryVideos > out[j].LibraryVideos
		}
		if out[i].InVideoCues != out[j].InVideoCues {
			return out[i].InVideoCues > out[j].InVideoCues
		}
		return out[i].Lemma < out[j].Lemma
	})
	if len(out) > maxVideoVocabWords {
		out = out[:maxVideoVocabWords]
	}
	return out, nil
}

// VideoSnapshotLang returns the stored lang of one snapshot — handlers use it
// to resolve the right dictionary for gloss enrichment. Errors with "video
// not found" when no snapshot exists.
func (s *Store) VideoSnapshotLang(videoID string) (string, error) {
	videoID = strings.TrimSpace(videoID)
	if videoID == "" {
		return "", clientErrf("video_id is required")
	}
	var lang string
	err := s.q().QueryRow(
		`SELECT lang FROM video_snapshots WHERE video_id = ?`, videoID,
	).Scan(&lang)
	if errors.Is(err, sql.ErrNoRows) {
		return "", clientErrf("video not found: %s", videoID)
	}
	if err != nil {
		return "", fmt.Errorf("look up video snapshot: %w", err)
	}
	return lang, nil
}

// DeleteVideoSnapshot removes one snapshot; reports whether it existed.
func (s *Store) DeleteVideoSnapshot(videoID string) (bool, error) {
	videoID = strings.TrimSpace(videoID)
	if videoID == "" {
		return false, clientErrf("video_id is required")
	}
	res, err := s.q().Exec(`DELETE FROM video_snapshots WHERE video_id = ?`, videoID)
	if err != nil {
		return false, fmt.Errorf("delete video snapshot: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// wordStatusMapChunked reads statuses for an arbitrary number of lemmas as a
// map, chunking the IN clauses at 500 (same reasoning as DeleteByTag — a big
// video library's lemma union can dwarf SQLite's bind-variable comfort zone).
// Absent lemmas simply aren't in the map — the zero value is implicit unknown.
func (s *Store) wordStatusMapChunked(lemmas []string) (map[string]int, error) {
	out := make(map[string]int, len(lemmas))
	const batch = 500
	for start := 0; start < len(lemmas); start += batch {
		end := start + batch
		if end > len(lemmas) {
			end = len(lemmas)
		}
		chunk := lemmas[start:end]
		placeholders := strings.Repeat("?,", len(chunk))
		placeholders = placeholders[:len(placeholders)-1]
		args := make([]any, len(chunk))
		for i, l := range chunk {
			args[i] = l
		}
		rows, err := s.q().Query(
			`SELECT lemma, status FROM word_status WHERE lang = ? AND lemma IN (`+placeholders+`)`, append([]any{s.lang}, args...)...)
		if err != nil {
			return nil, fmt.Errorf("batch word_status: %w", err)
		}
		for rows.Next() {
			var lemma string
			var status int
			if err := rows.Scan(&lemma, &status); err != nil {
				rows.Close()
				return nil, err
			}
			out[lemma] = status
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}

// minCardDueChunked returns each lemma's earliest card due date (epoch
// seconds), chunked at 500 like wordStatusMapChunked. Lemmas without a card
// aren't in the map.
func (s *Store) minCardDueChunked(lemmas []string) (map[string]int64, error) {
	out := make(map[string]int64, len(lemmas))
	const batch = 500
	for start := 0; start < len(lemmas); start += batch {
		end := start + batch
		if end > len(lemmas) {
			end = len(lemmas)
		}
		chunk := lemmas[start:end]
		placeholders := strings.Repeat("?,", len(chunk))
		placeholders = placeholders[:len(placeholders)-1]
		args := make([]any, len(chunk))
		for i, l := range chunk {
			args[i] = l
		}
		rows, err := s.q().Query(
			`SELECT lemma, MIN(due) FROM cards WHERE lang = ? AND lemma IN (`+placeholders+`) GROUP BY lemma`, append([]any{s.lang}, args...)...)
		if err != nil {
			return nil, fmt.Errorf("batch card due: %w", err)
		}
		for rows.Next() {
			var lemma string
			var due int64
			if err := rows.Scan(&lemma, &due); err != nil {
				rows.Close()
				return nil, err
			}
			out[lemma] = due
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}
