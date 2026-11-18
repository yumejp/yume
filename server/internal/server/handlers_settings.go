package server

import (
	"encoding/json"
	"net/http"

	"github.com/yumejp/yume/server/internal/srs"
)

// Per-user settings sync. The value is an opaque JSON blob owned by the client.
func registerSettingsRoutes(mux *http.ServeMux, cfg Config) {
	jsonPost(mux, "/get_settings", func(_ *http.Request, store *srs.Store, req struct {
		Key string `json:"key"`
	}) (any, error) {
		if req.Key == "" {
			req.Key = "mobile"
		}
		value, updatedAt, err := store.GetPref(req.Key)
		if err != nil {
			return nil, err
		}
		resp := map[string]any{"updated_at": updatedAt}
		if value != "" {
			resp["value"] = json.RawMessage(value)
		}
		return resp, nil
	})

	jsonPost(mux, "/save_settings", func(_ *http.Request, store *srs.Store, req struct {
		Key   string          `json:"key"`
		Value json.RawMessage `json:"value"`
	}) (any, error) {
		if req.Key == "" {
			req.Key = "mobile"
		}
		// Require a JSON object — not null/absent — so a client bug can't
		// wipe another device's copy with an empty save.
		trimmed := string(req.Value)
		if len(trimmed) == 0 || trimmed == "null" || trimmed[0] != '{' {
			return nil, badRequest("value must be a JSON object")
		}
		updatedAt, err := store.SetPref(req.Key, trimmed)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "updated_at": updatedAt}, nil
	})
}
