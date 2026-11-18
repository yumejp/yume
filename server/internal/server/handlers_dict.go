package server

import (
	"net/http"

	"github.com/yumejp/yume/server/internal/srs"
	"github.com/yumejp/yume/server/internal/tokenize"
)

func registerDictRoutes(mux *http.ServeMux, cfg Config) {
	// --- Imported Yomitan dictionaries -----------------------------------

	jsonPost(mux, "/create_dict", func(_ *http.Request, store *srs.Store, req srs.CreateDictReq) (any, error) {
		id, err := store.CreateDict(req)
		if err != nil {
			return nil, err
		}
		return map[string]any{"dict_id": id}, nil
	})

	jsonPost(mux, "/import_dict_chunk", func(_ *http.Request, store *srs.Store, req struct {
		DictID int64                `json:"dict_id"`
		Terms  []srs.NormalizedTerm `json:"terms"`
	}) (any, error) {
		n, err := store.ImportTermChunk(req.DictID, req.Terms)
		if err != nil {
			return nil, err
		}
		return map[string]any{"imported": n}, nil
	})

	jsonPost(mux, "/import_pitch_chunk", func(_ *http.Request, store *srs.Store, req struct {
		DictID  int64                 `json:"dict_id"`
		Pitches []srs.NormalizedPitch `json:"pitches"`
	}) (any, error) {
		n, err := store.ImportPitchChunk(req.DictID, req.Pitches)
		if err != nil {
			return nil, err
		}
		return map[string]any{"imported": n}, nil
	})

	jsonPost(mux, "/import_freq_chunk", func(_ *http.Request, store *srs.Store, req struct {
		DictID int64                `json:"dict_id"`
		Freqs  []srs.NormalizedFreq `json:"freqs"`
	}) (any, error) {
		n, err := store.ImportFreqChunk(req.DictID, req.Freqs)
		if err != nil {
			return nil, err
		}
		return map[string]any{"imported": n}, nil
	})

	jsonPost(mux, "/list_dicts", func(_ *http.Request, store *srs.Store, _ struct{}) (any, error) {
		dicts, err := store.ListDicts()
		if err != nil {
			return nil, err
		}
		return map[string]any{"dicts": dicts}, nil
	}, routeOpts{allowGET: true})

	jsonPost(mux, "/delete_dict", func(_ *http.Request, store *srs.Store, req struct {
		DictID int64 `json:"dict_id"`
	}) (any, error) {
		n, err := store.DeleteDict(req.DictID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"deleted": n}, nil
	})

	jsonPost(mux, "/set_dict_enabled", func(_ *http.Request, store *srs.Store, req struct {
		DictID  int64 `json:"dict_id"`
		Enabled bool  `json:"enabled"`
	}) (any, error) {
		if err := store.SetDictEnabled(req.DictID, req.Enabled); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	})

	// Pitch and frequency reach clients merged into /lookup results;
	// there are no standalone endpoints.

	// --- Tokenizer user dictionary ----------------------------------------
	// Entries preempt IPADIC and count as vocabulary downstream. The
	// dictionary is shared by every user, so only the owner may change it.

	jsonPost(mux, "/add_user_word", func(r *http.Request, _ *srs.Store, req struct {
		Text    string `json:"text"`
		Reading string `json:"reading,omitempty"`
	}) (any, error) {
		if err := requireOwner(r, cfg.AuthStore); err != nil {
			return nil, httpErr{http.StatusForbidden, err.Error()}
		}
		if err := tokenize.AddUserWord(req.Text, req.Reading); err != nil {
			return nil, badRequest(err.Error())
		}
		return map[string]any{"added": req.Text}, nil
	}, routeOpts{noStore: true})

	jsonPost(mux, "/remove_user_word", func(r *http.Request, _ *srs.Store, req struct {
		Text string `json:"text"`
	}) (any, error) {
		if err := requireOwner(r, cfg.AuthStore); err != nil {
			return nil, httpErr{http.StatusForbidden, err.Error()}
		}
		if err := tokenize.RemoveUserWord(req.Text); err != nil {
			return nil, badRequest(err.Error())
		}
		return map[string]any{"removed": req.Text}, nil
	}, routeOpts{noStore: true})

	jsonPost(mux, "/list_user_words", func(_ *http.Request, _ *srs.Store, _ struct{}) (any, error) {
		words, err := tokenize.ListUserWords()
		if err != nil {
			return nil, err
		}
		return map[string]any{"words": words}, nil
	}, routeOpts{allowGET: true, noStore: true})
}
