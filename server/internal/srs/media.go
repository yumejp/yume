package srs

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Media capture: a client may attach a short audio clip and an image to a
// card, base64-encoded in the save_card payload. We write each to a sidecar file in
// <dataDir>/media/<card_id>.<ext> and store just the filename on the card row.
//
// Why files, not BLOBs in SQLite: media can be a few hundred KB to a few MB
// per card. Keeping them out of the DB keeps the DB small + fast to back up,
// and lets the user browse/delete the media folder directly if they want.

// MediaBlob is the wire shape for attached media: a MIME type + base64 bytes.
type MediaBlob struct {
	Mime string `json:"mime"`
	Data string `json:"data"` // base64, no data: prefix
}

// extForMime maps the MIME types clients send to file extensions.
func extForMime(mime string) string {
	switch mime {
	case "audio/webm":
		return ".webm"
	case "audio/ogg":
		return ".ogg"
	case "audio/mp4", "audio/aac":
		return ".m4a"
	case "audio/mpeg":
		return ".mp3"
	case "audio/wav", "audio/x-wav", "audio/wave":
		return ".wav"
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		// Unknown — fall back to a generic extension so we still persist it.
		return ".bin"
	}
}

// writeMedia decodes a base64 blob and writes it to <mediaDir>/<base><ext>.
// Returns the filename (not full path) to store on the card row.
func (s *Store) writeMedia(base string, blob *MediaBlob) (string, error) {
	if blob == nil || blob.Data == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(blob.Data)
	if err != nil {
		return "", fmt.Errorf("decode media base64: %w", err)
	}
	mediaDir := s.MediaDir()
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir media: %w", err)
	}
	filename := base + extForMime(blob.Mime)
	full := filepath.Join(mediaDir, filename)
	if err := os.WriteFile(full, raw, 0o644); err != nil {
		return "", fmt.Errorf("write media file: %w", err)
	}
	return filename, nil
}

// AttachMedia writes the audio/image sidecars for a card and updates the row
// with their filenames. Used both by save_card (on create) and the standalone
// /attach_media endpoint (attach to an existing card — e.g. batch VOICEVOX
// audio). audioSource tags the audio's origin (e.g. "voicevox:zundamon").
// Each clip is routed through AddCardMedia as the new primary of its kind, so
// it lands in card_media (visible to list_card_media / set_primary_media) AND
// the legacy cards.audio_file/image_file pointers. AddCardMedia also verifies
// the card exists, so attaching to a nonexistent card errors instead of
// silently writing an orphan file.
func (s *Store) AttachMedia(cardID int64, audio, image *MediaBlob, audioSource string) error {
	if audio != nil && audio.Data != "" {
		if _, err := s.AddCardMedia(cardID, "audio", "", audioSource, "", audio, true); err != nil {
			return err
		}
	}
	if image != nil && image.Data != "" {
		if _, err := s.AddCardMedia(cardID, "image", "", "", "", image, true); err != nil {
			return err
		}
	}
	return nil
}

// GetCardMedia reads back a card's media sidecars as base64 blobs. Returns
// nil blobs for media that doesn't exist. Mirrors /get_card_media.
type GetCardMediaResp struct {
	Audio       *MediaBlob `json:"audio,omitempty"`
	Image       *MediaBlob `json:"image,omitempty"`
	AudioSource string     `json:"audio_source,omitempty"`
}

func (s *Store) GetCardMedia(cardID int64) (GetCardMediaResp, error) {
	var resp GetCardMediaResp
	var audioFile, imageFile, audioSource sql.NullString
	err := s.q().QueryRow(
		`SELECT audio_file, image_file, audio_source FROM cards WHERE id = ?`, cardID,
	).Scan(&audioFile, &imageFile, &audioSource)
	if errors.Is(err, sql.ErrNoRows) {
		return resp, clientErrf("card %d not found", cardID)
	}
	if err != nil {
		return resp, fmt.Errorf("query card media: %w", err)
	}
	resp.AudioSource = audioSource.String
	if audioFile.Valid && audioFile.String != "" {
		blob, err := s.readMedia(audioFile.String)
		if err == nil {
			resp.Audio = blob
		}
	}
	if imageFile.Valid && imageFile.String != "" {
		blob, err := s.readMedia(imageFile.String)
		if err == nil {
			resp.Image = blob
		}
	}
	return resp, nil
}

// readMedia loads a sidecar file and returns it as a base64 MediaBlob with a
// MIME guessed from its extension. filepath.Base prevents a stored path from
// escaping the media directory.
func (s *Store) readMedia(filename string) (*MediaBlob, error) {
	full := filepath.Join(s.MediaDir(), filepath.Base(filename))
	raw, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	return &MediaBlob{
		Mime: mimeForExt(filepath.Ext(filename)),
		Data: base64.StdEncoding.EncodeToString(raw),
	}, nil
}

func mimeForExt(ext string) string {
	switch ext {
	case ".webm":
		return "audio/webm"
	case ".ogg":
		return "audio/ogg"
	case ".m4a":
		return "audio/mp4"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}

// SweepOrphanedMedia removes files in media/ that no card or card_media row
// references. Restoring a database snapshot can leave such files behind.
// It also prunes expired files from media/.trash.
//
// This remains an explicit maintenance call because the pre-restore safety
// snapshot may still reference those files.
func (s *Store) SweepOrphanedMedia() (removed int, kept int, err error) {
	referenced, err := s.referencedMedia()
	if err != nil {
		return 0, 0, err
	}
	s.pruneMediaTrash(time.Now())

	entries, err := os.ReadDir(s.MediaDir())
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("read media dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if referenced[e.Name()] {
			kept++
			continue
		}
		if err := os.Remove(filepath.Join(s.MediaDir(), e.Name())); err == nil {
			removed++
		}
	}
	return removed, kept, nil
}

// referencedMedia returns the base names of every file a card or card_media
// row points at.
func (s *Store) referencedMedia() (map[string]bool, error) {
	referenced := make(map[string]bool)
	rows, err := s.q().Query(
		`SELECT COALESCE(audio_file,''), COALESCE(image_file,'') FROM cards`)
	if err != nil {
		return nil, fmt.Errorf("scan cards media refs: %w", err)
	}
	for rows.Next() {
		var a, i string
		if err := rows.Scan(&a, &i); err != nil {
			rows.Close()
			return nil, err
		}
		if a != "" {
			referenced[filepath.Base(a)] = true
		}
		if i != "" {
			referenced[filepath.Base(i)] = true
		}
	}
	rows.Close()
	rows, err = s.q().Query(`SELECT file FROM card_media WHERE file != ''`)
	if err != nil {
		return nil, fmt.Errorf("scan card_media refs: %w", err)
	}
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			rows.Close()
			return nil, err
		}
		referenced[filepath.Base(f)] = true
	}
	rows.Close()
	return referenced, nil
}

// Media trash: files removed by delete_by_tag wait in media/.trash for
// MediaTrashDays instead of being unlinked, so restoring the snapshot taken
// before an import brings back its cards AND their audio. Expired files are
// pruned whenever something is trashed and by SweepOrphanedMedia.

// MediaTrashDays is how long a trashed media file is kept.
const MediaTrashDays = 30

func (s *Store) mediaTrashDir() string {
	return filepath.Join(s.MediaDir(), ".trash")
}

// trashMedia moves the named sidecars into the trash, stamped now so the
// expiry counts from the delete. Best effort: a file that cannot be moved
// stays in media/, where SweepOrphanedMedia finds it.
func (s *Store) trashMedia(files []string) {
	now := time.Now()
	if len(files) > 0 {
		dir := s.mediaTrashDir()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return
		}
		for _, f := range files {
			name := filepath.Base(f)
			dst := filepath.Join(dir, name)
			if err := os.Rename(filepath.Join(s.MediaDir(), name), dst); err == nil {
				_ = os.Chtimes(dst, now, now)
			}
		}
	}
	s.pruneMediaTrash(now)
}

// pruneMediaTrash removes trashed files older than MediaTrashDays.
func (s *Store) pruneMediaTrash(now time.Time) {
	entries, err := os.ReadDir(s.mediaTrashDir())
	if err != nil {
		return
	}
	cutoff := now.AddDate(0, 0, -MediaTrashDays)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if st, err := e.Info(); err == nil && st.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(s.mediaTrashDir(), e.Name()))
		}
	}
}

// untrashReferencedMedia moves trashed files that the live database points
// at back into media/ (after a snapshot restore). A file already in media/
// wins over its trashed copy.
func (s *Store) untrashReferencedMedia() error {
	entries, err := os.ReadDir(s.mediaTrashDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read media trash: %w", err)
	}
	referenced, err := s.referencedMedia()
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !referenced[e.Name()] {
			continue
		}
		dst := filepath.Join(s.MediaDir(), e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := os.Rename(filepath.Join(s.mediaTrashDir(), e.Name()), dst); err != nil {
			return fmt.Errorf("restore trashed media: %w", err)
		}
	}
	return nil
}

// mediaFilesFor returns the legacy audio/image filenames stored on a card.
func (s *Store) mediaFilesFor(cardID int64) (audio, image string) {
	var a, i sql.NullString
	_ = s.q().QueryRow(`SELECT audio_file, image_file FROM cards WHERE id = ?`, cardID).Scan(&a, &i)
	return a.String, i.String
}
