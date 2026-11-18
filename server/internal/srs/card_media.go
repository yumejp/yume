package srs

// Multiple media per card. A card can carry several audio clips (e.g. the same
// sentence in different TTS voices) and/or images. One row per kind is flagged
// is_primary — that's the one /get_due_cards + /get_card_media surface, kept in
// sync with the legacy cards.audio_file / image_file columns for back-compat.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CardMedia is one audio/image clip attached to a card.
type CardMedia struct {
	ID        int64  `json:"id"`
	CardID    int64  `json:"card_id"`
	Kind      string `json:"kind"`             // "audio" | "image"
	Label     string `json:"label,omitempty"`  // display label, e.g. the voice name
	Source    string `json:"source,omitempty"` // e.g. "voicevox:kyushu-sora"
	Voice     string `json:"voice,omitempty"`  // credited voice, e.g. "ずんだもん" (VOICEVOX terms)
	File      string `json:"file"`             // sidecar filename
	IsPrimary bool   `json:"is_primary"`
	CreatedAt int64  `json:"created_at"`
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// setPrimary makes mediaID the sole primary of its kind for the card, and keeps
// the legacy cards.audio_file/image_file (+ audio_source) pointer in sync so the
// existing single-media read paths return the chosen clip. One transaction —
// a crash between the statements must not leave a card with zero or two
// primaries of a kind, or a legacy pointer disagreeing with card_media.
func (s *Store) setPrimary(cardID, mediaID int64, kind, file, source string) error {
	tx, err := s.q().Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`UPDATE card_media SET is_primary=0 WHERE card_id=? AND kind=?`, cardID, kind); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE card_media SET is_primary=1 WHERE id=?`, mediaID); err != nil {
		return err
	}
	if kind == "image" {
		if _, err := tx.Exec(`UPDATE cards SET image_file=? WHERE id=?`, file, cardID); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(`UPDATE cards SET audio_file=?, audio_source=? WHERE id=?`, file, nullStr(source), cardID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AddCardMedia writes a new media sidecar and links it to the card. It becomes
// primary if makePrimary is set or if the card has no primary of that kind yet.
func (s *Store) AddCardMedia(cardID int64, kind, label, source, voice string, blob *MediaBlob, makePrimary bool) (CardMedia, error) {
	if kind == "" {
		kind = "audio"
	}
	if kind != "audio" && kind != "image" {
		return CardMedia{}, clientErrf("kind must be 'audio' or 'image'")
	}
	if blob == nil || blob.Data == "" {
		return CardMedia{}, clientErrf("media data required")
	}
	var x int
	if err := s.q().QueryRow(`SELECT 1 FROM cards WHERE id=?`, cardID).Scan(&x); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CardMedia{}, clientErrf("card %d not found", cardID)
		}
		return CardMedia{}, err
	}
	now := time.Now().Unix()
	res, err := s.q().Exec(
		`INSERT INTO card_media (card_id, kind, label, source, voice, file, is_primary, created_at) VALUES (?,?,?,?,?,?,0,?)`,
		cardID, kind, nullStr(label), nullStr(source), nullStr(strings.TrimSpace(voice)), "", now)
	if err != nil {
		return CardMedia{}, err
	}
	id, _ := res.LastInsertId()
	// Sidecar name embeds the media id so a card's many clips never collide.
	file, err := s.writeMedia(fmt.Sprintf("%d-m%d", cardID, id), blob)
	if err != nil {
		_, _ = s.q().Exec(`DELETE FROM card_media WHERE id=?`, id)
		return CardMedia{}, err
	}
	if _, err := s.q().Exec(`UPDATE card_media SET file=? WHERE id=?`, file, id); err != nil {
		return CardMedia{}, err
	}
	cm := CardMedia{ID: id, CardID: cardID, Kind: kind, Label: label, Source: source, Voice: strings.TrimSpace(voice), File: file, CreatedAt: now}
	if !makePrimary {
		var cnt int
		_ = s.q().QueryRow(`SELECT COUNT(*) FROM card_media WHERE card_id=? AND kind=? AND is_primary=1`, cardID, kind).Scan(&cnt)
		makePrimary = cnt == 0
	}
	if makePrimary {
		if err := s.setPrimary(cardID, id, kind, file, source); err != nil {
			return cm, err
		}
		cm.IsPrimary = true
	}
	return cm, nil
}

// ListCardMedia returns all media for a card (audio first by id, then image).
func (s *Store) ListCardMedia(cardID int64) ([]CardMedia, error) {
	rows, err := s.q().Query(
		`SELECT id, card_id, kind, COALESCE(label,''), COALESCE(source,''), COALESCE(voice,''), file, is_primary, created_at
		 FROM card_media WHERE card_id=? ORDER BY kind, id`, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CardMedia{}
	for rows.Next() {
		var cm CardMedia
		var prim int
		if err := rows.Scan(&cm.ID, &cm.CardID, &cm.Kind, &cm.Label, &cm.Source, &cm.Voice, &cm.File, &prim, &cm.CreatedAt); err != nil {
			return nil, err
		}
		cm.IsPrimary = prim == 1
		out = append(out, cm)
	}
	return out, rows.Err()
}

// MediaVoice is one credited voice across the deck: the About screen lists
// these so the VOICEVOX credit is complete even when cards hide it.
type MediaVoice struct {
	Voice  string `json:"voice"`
	Source string `json:"source,omitempty"`
	Clips  int    `json:"clips"`
}

// ListMediaVoices groups the deck's audio clips by credited voice, most used first.
func (s *Store) ListMediaVoices() ([]MediaVoice, error) {
	rows, err := s.q().Query(`
		SELECT voice, COALESCE(source,''), COUNT(*) FROM card_media
		WHERE kind='audio' AND voice IS NOT NULL AND voice != ''
		GROUP BY voice, COALESCE(source,'') ORDER BY COUNT(*) DESC, voice`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MediaVoice{}
	for rows.Next() {
		var v MediaVoice
		if err := rows.Scan(&v.Voice, &v.Source, &v.Clips); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// SetPrimaryMedia marks which clip /get_due_cards + /get_card_media surface.
func (s *Store) SetPrimaryMedia(cardID, mediaID int64) error {
	var kind, file string
	var source sql.NullString
	err := s.q().QueryRow(`SELECT kind, file, source FROM card_media WHERE id=? AND card_id=?`, mediaID, cardID).
		Scan(&kind, &file, &source)
	if errors.Is(err, sql.ErrNoRows) {
		return clientErrf("media %d not found on card %d", mediaID, cardID)
	}
	if err != nil {
		return err
	}
	return s.setPrimary(cardID, mediaID, kind, file, source.String)
}

// DeleteCardMedia removes a clip + its sidecar. If it was primary, the most
// recent remaining clip of that kind is promoted; if none remain, the legacy
// pointer is cleared.
func (s *Store) DeleteCardMedia(mediaID int64) error {
	var cardID int64
	var kind, file string
	var prim int
	err := s.q().QueryRow(`SELECT card_id, kind, file, is_primary FROM card_media WHERE id=?`, mediaID).
		Scan(&cardID, &kind, &file, &prim)
	if errors.Is(err, sql.ErrNoRows) {
		return clientErrf("media %d not found", mediaID)
	}
	if err != nil {
		return err
	}
	if _, err := s.q().Exec(`DELETE FROM card_media WHERE id=?`, mediaID); err != nil {
		return err
	}
	if file != "" {
		_ = os.Remove(filepath.Join(s.MediaDir(), filepath.Base(file)))
	}
	if prim != 1 {
		return nil
	}
	var nid int64
	var nfile string
	var nsrc sql.NullString
	e := s.q().QueryRow(`SELECT id, file, source FROM card_media WHERE card_id=? AND kind=? ORDER BY id DESC LIMIT 1`, cardID, kind).
		Scan(&nid, &nfile, &nsrc)
	if e == nil {
		return s.setPrimary(cardID, nid, kind, nfile, nsrc.String)
	}
	if kind == "image" {
		_, err = s.q().Exec(`UPDATE cards SET image_file=NULL WHERE id=?`, cardID)
	} else {
		_, err = s.q().Exec(`UPDATE cards SET audio_file=NULL, audio_source=NULL WHERE id=?`, cardID)
	}
	return err
}

// GetCardMediaByID reads one clip's bytes as a base64 MediaBlob.
func (s *Store) GetCardMediaByID(mediaID int64) (*MediaBlob, error) {
	var file string
	err := s.q().QueryRow(`SELECT file FROM card_media WHERE id=?`, mediaID).Scan(&file)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, clientErrf("media %d not found", mediaID)
	}
	if err != nil {
		return nil, err
	}
	return s.readMedia(file)
}

// CardUpdate carries the editable card fields; nil pointers are left unchanged.
type CardUpdate struct {
	Sentence    *string     `json:"sentence"`
	Translation *string     `json:"translation"` // "" clears
	Explanation *string     `json:"explanation"` // "" clears
	SourceURL   *string     `json:"source_url"`
	Lemma     *string     `json:"lemma"`
	Glosses   *[][]string `json:"glosses"` // per-sense; used to fill an empty English gloss
	Readings  *[]string   `json:"readings"`
	Kanji     *[]string   `json:"kanji"`
}

// UpdateCard edits the provided fields of a card in place (no scheduling change).
func (s *Store) UpdateCard(cardID int64, u CardUpdate) error {
	var sets []string
	var args []any
	if err := u.checkText(); err != nil {
		return err
	}
	if u.Sentence != nil {
		sets = append(sets, "sentence=?")
		args = append(args, *u.Sentence)
	}
	if u.Translation != nil {
		sets = append(sets, "translation=?")
		args = append(args, nullStr(strings.TrimSpace(*u.Translation)))
	}
	if u.Explanation != nil {
		sets = append(sets, "explanation=?")
		args = append(args, nullStr(strings.TrimSpace(*u.Explanation)))
	}
	if u.SourceURL != nil {
		sets = append(sets, "source_url=?")
		args = append(args, *u.SourceURL)
	}
	if u.Lemma != nil {
		if strings.TrimSpace(*u.Lemma) == "" {
			return clientErrf("lemma cannot be empty")
		}
		sets = append(sets, "lemma=?")
		args = append(args, *u.Lemma)
	}
	if u.Glosses != nil {
		b, err := json.Marshal(*u.Glosses)
		if err != nil {
			return fmt.Errorf("encode glosses: %w", err)
		}
		sets = append(sets, "glosses=?")
		args = append(args, string(b))
	}
	if u.Readings != nil {
		sets = append(sets, "readings=?")
		args = append(args, jsonOrNil(*u.Readings))
	}
	if u.Kanji != nil {
		sets = append(sets, "kanji=?")
		args = append(args, jsonOrNil(*u.Kanji))
	}
	if len(sets) == 0 {
		return clientErrf("no fields to update")
	}
	args = append(args, cardID)
	res, err := s.q().Exec(`UPDATE cards SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return clientErrf("card %d not found", cardID)
	}
	return nil
}
