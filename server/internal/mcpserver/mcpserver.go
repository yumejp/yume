// Package mcpserver exposes Yume to AI tutors over the Model Context
// Protocol (Streamable HTTP at /mcp on the same localhost server).
//
// A connected tutor can read the learner's progress and suggest words. The
// learner decides which suggestions enter the deck; Known and Ignored words
// leave the suggestion list automatically.
//
// Security: the endpoint rides the same middleware as the rest of the
// server — loopback Host allowlist, browser Origins rejected. MCP clients
// are non-browser local processes (no Origin header), which is exactly the
// caller class the middleware admits. In "required" auth mode the endpoint
// additionally demands the OWNER's session bearer token (enforced by
// userResolveMiddleware in package server), since the tools below operate on
// the owner's deck.
package mcpserver

import (
	"context"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yumejp/yume/server/internal/srs"
)

type suggestWordsIn struct {
	Words []struct {
		Word string `json:"word" jsonschema:"the word (dictionary/lemma form) the learner should look out for"`
		Note string `json:"note,omitempty" jsonschema:"one-line reason for the suggestion, shown to the learner next to the suggestion"`
	} `json:"words" jsonschema:"words to suggest; use the dictionary (lemma) form so tokens match"`
}

type suggestWordsOut struct {
	Added int `json:"added"`
}

type listOut struct {
	Words []srs.WatchWord `json:"words"`
}

type listTaggedIn struct {
	Tags  []string `json:"tags" jsonschema:"return words carrying ALL of these tags (AND). The learner applies tags in the vocab explorer."`
	Limit int      `json:"limit,omitempty" jsonschema:"max words to return (default 200, cap 500)"`
}

type taggedWord struct {
	Lemma    string     `json:"lemma"`
	Status   int        `json:"status"` // 0=unknown 1=learning 2=known 3=ignored
	Readings []string   `json:"readings,omitempty"`
	Glosses  [][]string `json:"glosses,omitempty"`
	Tags     []string   `json:"tags"`
}

type listTaggedOut struct {
	Total int          `json:"total"`
	Words []taggedWord `json:"words"`
}

type removeIn struct {
	Word string `json:"word" jsonschema:"the suggested word to remove"`
}

type removeOut struct {
	Removed bool `json:"removed"`
}

type backupIn struct {
	Label string `json:"label,omitempty" jsonschema:"optional short label baked into the backup filename (e.g. before-n2-import)"`
}

type backupOut struct {
	Backup srs.BackupInfo `json:"backup"`
	Pruned []string       `json:"pruned_old_backups"`
	Kept   int            `json:"max_backups_kept"`
}

type listBackupsOut struct {
	Backups []srs.BackupInfo `json:"backups"`
}

type restoreIn struct {
	Name string `json:"name" jsonschema:"backup filename from list_vocab_backups"`
}

type restoreOut struct {
	RestoredFrom  string         `json:"restored_from"`
	SafetyBackup  srs.BackupInfo `json:"safety_backup_of_previous_state"`
	Pruned        []string       `json:"pruned_old_backups"`
	TotalCardsNow int            `json:"total_cards_now"`
}

type flagWordIn struct {
	Word   string `json:"word" jsonschema:"the word (dictionary/lemma form) whose card to flag"`
	Tag    string `json:"tag" jsonschema:"which system flag to set: 'needs-audio' (generated audio is stale or wrong; regenerate), 'needs-fix' (card data wrong or mis-mined: bad lemma, reading or gloss), or 'needs-sentence' (no example sentence). With remove, any system flag, including the learner's 'flagged' and 'needs-explanation'"`
	Remove bool   `json:"remove,omitempty" jsonschema:"set true to clear the flag instead of setting it"`
}

type flagWordOut struct {
	Word    string `json:"word"`
	Tag     string `json:"tag"`
	Removed bool   `json:"removed"`
}

type listFlaggedIn struct {
	Tag string `json:"tag,omitempty" jsonschema:"optional: restrict to one flag (needs-audio | needs-fix | needs-sentence); omit to list words carrying any system flag"`
}

type listFlaggedOut struct {
	Flagged []srs.TaggedWord `json:"flagged"`
}

type listVideosIn struct {
	Lang string `json:"lang,omitempty" jsonschema:"language code of the library to list (default ja)"`
}

// videoRow is the tutor-facing subset of srs.VideoStats — the readiness
// numbers that matter for "what should they watch / pre-learn".
type videoRow struct {
	VideoID      string  `json:"video_id"`
	Title        string  `json:"title"`
	Platform     string  `json:"platform"`
	CoveragePct  float64 `json:"coverage_pct"`  // % of the video's unique content words the learner knows
	I1Cues       int     `json:"i1_cues"`       // lines with exactly one unknown word — comprehensible-input sweet spots
	NewWords     int     `json:"new_words"`     // unknown unique words
	Learning     int     `json:"learning"`      // words currently being studied
	MaturingSoon int     `json:"maturing_soon"` // Learning words with a review due within 14 days
	UpdatedAt    int64   `json:"updated_at"`    // epoch seconds
}

type listVideosOut struct {
	Videos []videoRow `json:"videos"`
	// Hint explains an empty library, so a tutor does not read it as "watched nothing".
	Hint string `json:"hint,omitempty"`
}

// noVideosHint answers list_videos when no client has saved a snapshot.
const noVideosHint = "No analyzed videos. The video library is filled by the Yume browser " +
	"extension while the learner watches with it; the server and the app alone never add " +
	"videos, so ground suggestions in get_learning_snapshot instead."

type videoVocabIn struct {
	VideoID  string `json:"video_id" jsonschema:"the video to inspect, from list_videos"`
	Statuses []int  `json:"statuses,omitempty" jsonschema:"word statuses to include (0=unknown 1=learning 2=known 3=ignored); default [0,1] — the study-relevant set"`
}

type videoVocabOut struct {
	Words []VideoVocabEntry `json:"words"`
}

type snapshotOut struct {
	TotalCards int             `json:"total_cards"`
	DueCards   int             `json:"due_cards"` // same as due_reviews; kept for older tutors
	Learning   int             `json:"learning_words"`
	Known      int             `json:"known_words"`
	Ignored    int             `json:"ignored_words"`
	Suggested  []srs.WatchWord `json:"current_suggestions"`
	Recent     []string        `json:"recently_learning"`

	// DueReviews counts studied cards due now. NewWaiting counts never-seen
	// cards, which the learner starts on request (Learn new words); due_cards
	// once included them and read as a 2,001-review backlog after an import.
	DueReviews         int `json:"due_reviews"`
	NewWaiting         int `json:"new_waiting"`
	NewIntroducedToday int `json:"new_introduced_today"`
}

// Handler builds the MCP Streamable HTTP handler. resolve supplies the SRS
// store used by its tools.
func Handler(version string, resolve func() (*srs.Store, error)) http.Handler {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "yume", Version: version},
		&mcp.ServerOptions{
			Instructions: "Yume is a self-hosted Japanese study server: dictionary, cards and FSRS reviews. " +
				"You may improve card content; never rate cards or change the review schedule. " +
				"Use get_learning_snapshot to see the learner's progress, then suggest_words to flag " +
				"words worth acquiring; they stay in the learner's Suggested list until the learner " +
				"marks them Known or Ignored. Suggest dictionary (lemma) forms. Keep suggestion " +
				"lists small and purposeful (≤20 active at a time) so suggestions stay meaningful. " +
				"The learner can also tag words in the vocab explorer; use list_tagged_words to fetch a tagged set (e.g. to generate examples for them). " +
				"list_flagged_words returns cards carrying a system flag. The learner sets and sees " +
				"these in a card's menu in the app: 'flagged' (something is wrong: triage it), " +
				"'needs-explanation' (write the card's explanation field, or a tutor note if it needs " +
				"more room), 'needs-sentence', 'needs-audio' and 'needs-fix'. flag_word sets " +
				"needs-audio, needs-fix or needs-sentence when you notice a problem, and with " +
				"remove=true clears a flag once the work is done. Flags are never shown as card content. " +
				"Card content is written through Yume's HTTP API, at this endpoint's origin without " +
				"/mcp, with the header X-Yume-User: owner (open mode) or the owner's bearer token: " +
				"POST /update_card for sentence, translation and explanation (card fields, never notes), " +
				"/add_card_note with kind tutor, example (translation required) or triage, " +
				"/set_shared_mnemonic, /set_shared_kanji_note and /tag_card. Those writes need a client " +
				"that can make HTTP requests; the yume-sensei skill in the Yume repo describes the " +
				"workflow. Send Japanese as a UTF-8 JSON body from a file or a script, never inline in " +
				"shell arguments: Windows shells turn it into '?', which the server refuses. " +
				"Call backup_vocab before changing many cards. If the learner uses the Yume browser " +
				"extension, list_videos ranks the videos it analyzed by readiness and get_video_vocab " +
				"lists a video's unknown and learning words with library reach; without the extension " +
				"the video library is empty.",
		},
	)

	mcp.AddTool(server, &mcp.Tool{
		Name: "suggest_words",
		Description: "Flag words the learner should look out for. Each suggested word is " +
			"shown in the learner's Suggested list until learned. Re-suggesting a word updates its note.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in suggestWordsIn) (*mcp.CallToolResult, suggestWordsOut, error) {
		store, err := resolve()
		if err != nil {
			return nil, suggestWordsOut{}, fmt.Errorf("card store unavailable: %w", err)
		}
		words := make([]srs.WatchWord, 0, len(in.Words))
		for _, w := range in.Words {
			words = append(words, srs.WatchWord{Lemma: w.Word, Note: w.Note})
		}
		n, err := store.AddWatchWords(words, "mcp")
		if err != nil {
			return nil, suggestWordsOut{}, err
		}
		return nil, suggestWordsOut{Added: n}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_suggested_words",
		Description: "List the current suggestions with the learner's status for each " +
			"(0=unknown 1=learning 2=known 3=ignored). Known/Ignored words retire automatically.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, listOut, error) {
		store, err := resolve()
		if err != nil {
			return nil, listOut{}, fmt.Errorf("card store unavailable: %w", err)
		}
		words, err := store.ListWatchWords()
		if err != nil {
			return nil, listOut{}, err
		}
		return nil, listOut{Words: words}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "remove_suggested_word",
		Description: "Withdraw one suggestion from the learner's list.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in removeIn) (*mcp.CallToolResult, removeOut, error) {
		store, err := resolve()
		if err != nil {
			return nil, removeOut{}, fmt.Errorf("card store unavailable: %w", err)
		}
		ok, err := store.RemoveWatchWord(in.Word)
		if err != nil {
			return nil, removeOut{}, err
		}
		return nil, removeOut{Removed: ok}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "get_learning_snapshot",
		Description: "The learner's current state: deck size, due reviews, new words waiting, " +
			"word-status counts, active suggestions, and a sample of words currently being learned. " +
			"due_reviews (also due_cards) counts studied cards due now; new_waiting counts saved " +
			"words the learner has not started, which they begin on request, so they are not overdue. " +
			"Use this to ground suggestions in what the learner is actually studying.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, snapshotOut, error) {
		store, err := resolve()
		if err != nil {
			return nil, snapshotOut{}, fmt.Errorf("card store unavailable: %w", err)
		}
		return nil, learningSnapshot(store), nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_tagged_words",
		Description: "Fetch the learner's words that carry ALL of the given tags. The learner " +
			"applies free-form tags to words in the vocab explorer, so this is how they hand you a " +
			"curated set — e.g. tag a batch of words and ask you to write example sentences for them. " +
			"Returns lemma, status (0=unknown 1=learning 2=known 3=ignored), readings and glosses.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in listTaggedIn) (*mcp.CallToolResult, listTaggedOut, error) {
		store, err := resolve()
		if err != nil {
			return nil, listTaggedOut{}, fmt.Errorf("card store unavailable: %w", err)
		}
		if len(in.Tags) == 0 {
			return nil, listTaggedOut{}, fmt.Errorf("at least one tag is required")
		}
		limit := in.Limit
		if limit <= 0 || limit > 500 {
			limit = 200
		}
		resp, err := store.ListVocab(srs.VocabFilter{Tags: in.Tags}, limit, 0)
		if err != nil {
			return nil, listTaggedOut{}, err
		}
		words := make([]taggedWord, 0, len(resp.Rows))
		for _, r := range resp.Rows {
			words = append(words, taggedWord{
				Lemma:    r.Lemma,
				Status:   r.Status,
				Readings: r.Readings,
				Glosses:  r.Glosses,
				Tags:     r.Tags,
			})
		}
		return nil, listTaggedOut{Total: resp.Total, Words: words}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_videos",
		Description: "The learner's analyzed video library, filled by the Yume browser extension " +
			"(empty, with a hint, when the learner does not use it): every video they've watched with it, " +
			"with readiness stats recomputed against their CURRENT knowledge on each call — " +
			"coverage_pct (share of the video's words they know), i1_cues (lines with exactly one " +
			"unknown word — ideal comprehensible input), new_words, learning, and maturing_soon " +
			"(studied words about to be reviewed, so the video gets easier soon). Use it to ground " +
			"recommendations about what to watch next and which videos are worth pre-learning words " +
			"for (see get_video_vocab).",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in listVideosIn) (*mcp.CallToolResult, listVideosOut, error) {
		store, err := resolve()
		if err != nil {
			return nil, listVideosOut{}, fmt.Errorf("card store unavailable: %w", err)
		}
		stats, err := store.ListVideoSnapshots(in.Lang)
		if err != nil {
			return nil, listVideosOut{}, err
		}
		videos := make([]videoRow, 0, len(stats))
		for _, v := range stats {
			videos = append(videos, videoRow{
				VideoID:      v.VideoID,
				Title:        v.Title,
				Platform:     v.Platform,
				CoveragePct:  v.CoveragePct,
				I1Cues:       v.I1Cues,
				NewWords:     v.NewWords,
				Learning:     v.Learning,
				MaturingSoon: v.MaturingSoon,
				UpdatedAt:    v.UpdatedAt,
			})
		}
		out := listVideosOut{Videos: videos}
		if len(videos) == 0 {
			out.Hint = noVideosHint
		}
		return nil, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "get_video_vocab",
		Description: "The study-relevant words of one video (see list_videos for ids): each word's " +
			"status (0=unknown, 1=learning), in_video_cues (how many of the video's lines contain " +
			"it), and library_videos — its library reach: how many of the learner's OWN videos " +
			"(including this one) contain the word. High reach = high personal value: learning it " +
			"pays off across everything they watch. Readings and glosses are attached when the " +
			"dictionary knows the word. Use this with suggest_words to push a curated 'learn these " +
			"first for this video' plan — not every word is worth learning: names, one-off slang " +
			"and archaisms usually aren't, and a missing gloss is often that hint. By default " +
			"returns unknown + learning words; pass statuses to override (2=known, 3=ignored).",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in videoVocabIn) (*mcp.CallToolResult, videoVocabOut, error) {
		store, err := resolve()
		if err != nil {
			return nil, videoVocabOut{}, fmt.Errorf("card store unavailable: %w", err)
		}
		words, err := store.GetVideoVocab(in.VideoID, in.Statuses)
		if err != nil {
			return nil, videoVocabOut{}, err
		}
		lang, err := store.VideoSnapshotLang(in.VideoID)
		if err != nil {
			return nil, videoVocabOut{}, err
		}
		return nil, videoVocabOut{Words: EnrichVideoVocab(lang, words)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "backup_vocab",
		Description: "Snapshot the learner's whole vocab store (cards, statuses, tags, notes, " +
			"review history, suggestions) into a managed backup. Retention is automatic: the newest " +
			fmt.Sprint(srs.MaxBackups) + " manual backups are kept, and pruned_old_backups names any " +
			"older manual backup this one replaced. Snapshots taken automatically before imports, " +
			"restores and upgrades are kept separately and never prune a manual one. Use before " +
			"risky bulk operations (imports, mass status changes, restores), and tell the learner " +
			"the backup's name: restoring it returns the deck to this point.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in backupIn) (*mcp.CallToolResult, backupOut, error) {
		store, err := resolve()
		if err != nil {
			return nil, backupOut{}, fmt.Errorf("card store unavailable: %w", err)
		}
		info, pruned, err := store.CreateBackup(in.Label)
		if err != nil {
			return nil, backupOut{}, err
		}
		if pruned == nil {
			pruned = []string{}
		}
		return nil, backupOut{Backup: info, Pruned: pruned, Kept: srs.MaxBackups}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_vocab_backups",
		Description: "List the managed vocab backups, newest first. kind is manual, import (taken " +
			"before an import), prerestore (taken before a restore) or premigrate (taken before a " +
			"server upgrade changed the database).",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, listBackupsOut, error) {
		store, err := resolve()
		if err != nil {
			return nil, listBackupsOut{}, fmt.Errorf("card store unavailable: %w", err)
		}
		backups, err := store.ListBackups()
		if err != nil {
			return nil, listBackupsOut{}, err
		}
		return nil, listBackupsOut{Backups: backups}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "restore_vocab_backup",
		Description: "Replace the learner's vocab store with a named backup (see " +
			"list_vocab_backups). A 'prerestore' safety snapshot of the CURRENT state is taken " +
			"first, so a restore is itself undoable by restoring that snapshot. DESTRUCTIVE: " +
			"confirm with the learner before calling. They should reload open Yume pages after.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in restoreIn) (*mcp.CallToolResult, restoreOut, error) {
		store, err := resolve()
		if err != nil {
			return nil, restoreOut{}, fmt.Errorf("card store unavailable: %w", err)
		}
		safety, pruned, err := store.RestoreBackup(in.Name)
		if err != nil {
			return nil, restoreOut{}, err
		}
		if pruned == nil {
			pruned = []string{}
		}
		out := restoreOut{RestoredFrom: in.Name, SafetyBackup: safety, Pruned: pruned}
		if ds, err := store.DeckStats(); err == nil {
			out.TotalCardsNow = ds.Total
		}
		return nil, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "flag_word",
		Description: "Flag a word's card for follow-up with a system tag. Use 'needs-audio' when " +
			"a card's generated pronunciation is stale or wrong (e.g. its reading was corrected), " +
			"'needs-fix' when the card's data is wrong or mis-mined (bad lemma/reading/gloss), or " +
			"'needs-sentence' when it has no example sentence. The learner sees a card's flags as " +
			"requests in its menu in the app, never as card content. Set remove=true to clear a " +
			"flag, including the learner's own 'flagged' and 'needs-explanation' once handled.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in flagWordIn) (*mcp.CallToolResult, flagWordOut, error) {
		store, err := resolve()
		if err != nil {
			return nil, flagWordOut{}, fmt.Errorf("card store unavailable: %w", err)
		}
		if err := store.FlagWord(in.Word, in.Tag, in.Remove); err != nil {
			return nil, flagWordOut{}, err
		}
		return nil, flagWordOut{Word: in.Word, Tag: in.Tag, Removed: in.Remove}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_flagged_words",
		Description: "List words whose cards are flagged for follow-up (see flag_word). Optionally " +
			"restrict to one flag. Use this to triage what still needs audio regeneration, a data " +
			"fix, or an example sentence.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in listFlaggedIn) (*mcp.CallToolResult, listFlaggedOut, error) {
		store, err := resolve()
		if err != nil {
			return nil, listFlaggedOut{}, fmt.Errorf("card store unavailable: %w", err)
		}
		flagged, err := store.SystemTaggedWords(in.Tag)
		if err != nil {
			return nil, listFlaggedOut{}, err
		}
		return nil, listFlaggedOut{Flagged: flagged}, nil
	})

	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
}

// learningSnapshot assembles get_learning_snapshot. Each part is best-effort:
// a failed count leaves its fields zero rather than failing the tool.
func learningSnapshot(store *srs.Store) snapshotOut {
	out := snapshotOut{Recent: []string{}}
	if ds, err := store.DeckStats(); err == nil {
		out.TotalCards = ds.Total
		out.DueCards = ds.DueReviews
		out.DueReviews = ds.DueReviews
		out.NewWaiting = ds.NewTotal
		out.NewIntroducedToday = ds.NewIntroducedToday
	}
	if c, err := store.WordStatusCounts(); err == nil {
		out.Learning = c.Learning
		out.Known = c.Known
		out.Ignored = c.Ignored
	}
	if ws, err := store.ListWatchWords(); err == nil {
		out.Suggested = ws
	}
	learning := 1
	if resp, err := store.ListVocab(srs.VocabFilter{Status: &learning}, 25, 0); err == nil {
		for _, r := range resp.Rows {
			out.Recent = append(out.Recent, r.Lemma)
		}
	}
	return out
}
