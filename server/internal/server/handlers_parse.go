package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/yumejp/yume/server/internal/dict"
	"github.com/yumejp/yume/server/internal/examples"
	"github.com/yumejp/yume/server/internal/language"
	"github.com/yumejp/yume/server/internal/notices"
	"github.com/yumejp/yume/server/internal/srs"
	"github.com/yumejp/yume/server/internal/tokenize"
)

// APILevel is the HTTP contract this server speaks, reported by /health so a
// client can tell a server that is too old for it before calls start
// failing. Raise it by one in any release that adds or changes an endpoint
// or field a client relies on (docs/releasing.md).
const APILevel = 1

// MinClientAPI is the oldest client API level this server still serves
// correctly. Raise it only when a release removes or changes behaviour that
// older clients depend on.
const MinClientAPI = 1

func unsupportedLang(what, lang string) error {
	return badRequest("no " + what + " for lang=" + lang + "; supported: " + strings.Join(language.Supported(), ", "))
}

// dictUnavailable is the 503 for a dictionary that failed to load. A missing
// file's error already names the setup command, so it goes out as is.
func dictUnavailable(err error) error {
	if errors.Is(err, dict.ErrNotInstalled) {
		return httpErr{http.StatusServiceUnavailable, err.Error()}
	}
	return httpErr{http.StatusServiceUnavailable, "dictionary not loaded: " + err.Error()}
}

func registerParseRoutes(mux *http.ServeMux, cfg Config) {
	// Health — clients poll this to detect "is the server alive?"
	// Reports dict load state so a client can show whether lookups are
	// ready, separate from whether the server itself is up.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Cheap: this races on first call but sync.Once inside Default()
		// keeps it safe. We tolerate the latency hit here so the client
		// shows accurate state immediately.
		d, err := dict.Default()
		dictSize := 0
		dictErr := ""
		if err != nil {
			dictErr = err.Error()
		} else {
			dictSize = d.Size()
		}
		// ok means the server answers; ready means lookups work too.
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"ready":   dictErr == "",
			"version": cfg.Version,
			"commit":  cfg.Commit,
			"dict": map[string]any{
				"loaded":  dictErr == "",
				"entries": dictSize,
				"error":   dictErr,
			},
			"schema_version": srs.SchemaVersion(),
			"api_level":      APILevel,
			"min_client_api": MinClientAPI,
			// The app shows its sign-in screen in required mode, and offers
			// "Create an account" only while signup is open.
			"auth": map[string]any{
				"mode":   authMode(cfg),
				"signup": signupOpen(cfg),
			},
		})
	})

	// License notices for the data, the server's Go modules and the bundled
	// app; linked from the app's About sheet. Public like /health: it holds
	// no user data, and the licenses ask for it to reach every user.
	mux.HandleFunc("/licenses", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeErr(w, http.StatusMethodNotAllowed, "use GET")
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(notices.Text))
	})

	jsonPost(mux, "/tokenize", func(_ *http.Request, _ *srs.Store, req tokenizeReq) (any, error) {
		lang, ok := language.For(req.Lang)
		if !ok || lang.Tokenize == nil {
			return nil, unsupportedLang("tokenizer", req.Lang)
		}
		toks, err := lang.Tokenize(req.Text)
		if err != nil {
			return nil, fmt.Errorf("tokenize: %w", err)
		}
		return map[string]any{"tokens": toks}, nil
	}, routeOpts{noStore: true})

	// Tokenize many texts in a single round-trip: whole-video coverage and the
	// transcript hub tokenize every cue at once (~700 on a long video).
	jsonPost(mux, "/tokenize_batch", func(_ *http.Request, _ *srs.Store, req tokenizeBatchReq) (any, error) {
		lang, ok := language.For(req.Lang)
		if !ok || lang.Tokenize == nil {
			return nil, unsupportedLang("tokenizer", req.Lang)
		}
		batches := make([][]tokenize.Token, len(req.Texts))
		for i, text := range req.Texts {
			toks, err := lang.Tokenize(text)
			if err != nil {
				return nil, fmt.Errorf("tokenize: %w", err)
			}
			// Never emit a null batch — clients iterate each batch
			// directly, so a nil would throw. Empty text → empty slice.
			if toks == nil {
				toks = []tokenize.Token{}
			}
			batches[i] = toks
		}
		return map[string]any{"batches": batches}, nil
	}, routeOpts{noStore: true})

	// Lookup — runs each query against JMdict, returns matching entries.
	// Clients typically pass [lemma, surface] so a word view can
	// merge results across both forms. Dedupe happens server-side per query.
	jsonPost(mux, "/lookup", func(r *http.Request, _ *srs.Store, req lookupReq) (any, error) {
		lang, ok := language.For(req.Lang)
		if !ok || lang.Dict == nil {
			return nil, unsupportedLang("dictionary", req.Lang)
		}
		d, err := lang.Dict()
		if err != nil {
			log.Printf("/lookup: dict not loaded: %v", err)
			return nil, dictUnavailable(err)
		}
		results := d.Lookup(req.Queries)

		// Deconjugation fallback: raw strings (mouse selections, degraded
		// tokens, search boxes) often arrive conjugated — ぶつけた,
		// 食べさせられた — and miss the dictionary's base-form keys. For
		// ZERO-HIT queries only, run the query through the language's
		// tokenizer and look up the token lemmas instead (kagome's lattice is
		// a free deinflector), returning entries under the original query.
		// Exact hits are never shadowed — the fallback never runs for them.
		if lang.Tokenize != nil {
			deconjugateMisses(lang, d, results)
		}

		// Merge in imported-dict entries + annotate pitch/frequency. If the
		// SRS store isn't available we still return JMdict results — pitch
		// and imported dicts are enhancements, not requirements.
		if store, serr := storeFor(r); serr == nil {
			imported, ierr := store.LookupImported(req.Queries)
			if ierr != nil {
				log.Printf("/lookup: imported lookup failed: %v", ierr)
			}
			for i := range results {
				if extra := imported[results[i].Query]; len(extra) > 0 {
					results[i].Entries = append(results[i].Entries, extra...)
				}
				if aerr := store.AnnotatePitchFreq(results[i].Entries); aerr != nil {
					log.Printf("/lookup: annotate pitch/freq failed: %v", aerr)
				}
			}
		}
		return map[string]any{"results": results}, nil
	}, routeOpts{noStore: true})

	// Search English — reverse lookup. Given an English word/phrase, return
	// Japanese entries whose glosses match (ranked best-first). Same dict +
	// pitch/frequency annotation path as /lookup.
	jsonPost(mux, "/search_english", func(r *http.Request, _ *srs.Store, req struct {
		Query string `json:"query"`
		Lang  string `json:"lang"`
		Limit int    `json:"limit"`
	}) (any, error) {
		lang, ok := language.For(req.Lang)
		if !ok || lang.Dict == nil {
			return nil, unsupportedLang("dictionary", req.Lang)
		}
		d, err := lang.Dict()
		if err != nil {
			log.Printf("/search_english: dict not loaded: %v", err)
			return nil, dictUnavailable(err)
		}
		entries := d.SearchEnglish(req.Query, req.Limit)
		// Annotate pitch/frequency so EN-search results carry the same freq
		// rank as forward lookups (enhancement; skipped if no store).
		if store, serr := storeFor(r); serr == nil {
			if aerr := store.AnnotatePitchFreq(entries); aerr != nil {
				log.Printf("/search_english: annotate pitch/freq failed: %v", aerr)
			}
		}
		return map[string]any{"entries": entries}, nil
	}, routeOpts{noStore: true})

	// Example sentences — Tanaka Corpus (examples.utf.gz) for ja; routed
	// through the language registry like /lookup. Caller passes
	// [lemma, surface]; first query with hits wins.
	registerExamplesRoute(mux)
}

// deconjugateLemmaCap bounds how many distinct content lemmas a single
// zero-hit query may fan out to — a sentence-length selection tokenizes into
// many words, and a word view only has room for a handful of entries anyway.
const deconjugateLemmaCap = 8

// deconjugateMisses fills zero-hit /lookup results by tokenizing each missed
// query and looking up its CONTENT-token lemmas (per lang.IsContent, so
// 食べさせられた yields 食べる — not させる/られる/た). Entries merge under the
// original query, deduped by entry id. Queries that already have entries are
// left untouched.
func deconjugateMisses(lang *language.Language, d *dict.Dict, results []dict.LookupResult) {
	for i := range results {
		if len(results[i].Entries) > 0 {
			continue
		}
		q := results[i].Query
		toks, err := lang.Tokenize(q)
		if err != nil {
			continue
		}
		seen := map[string]bool{q: true}
		lemmas := make([]string, 0, deconjugateLemmaCap)
		for _, t := range toks {
			if t.Lemma == "" || seen[t.Lemma] {
				continue
			}
			if lang.IsContent != nil && !lang.IsContent(t) {
				continue
			}
			seen[t.Lemma] = true
			lemmas = append(lemmas, t.Lemma)
			if len(lemmas) == deconjugateLemmaCap {
				break
			}
		}
		if len(lemmas) == 0 {
			continue
		}
		haveID := make(map[int]bool)
		for _, sub := range d.Lookup(lemmas) {
			for _, e := range sub.Entries {
				if haveID[e.ID] {
					continue
				}
				haveID[e.ID] = true
				results[i].Entries = append(results[i].Entries, e)
			}
		}
	}
}

func registerExamplesRoute(mux *http.ServeMux) {
	jsonPost(mux, "/examples", func(_ *http.Request, _ *srs.Store, req examplesReq) (any, error) {
		lang, ok := language.For(req.Lang)
		if !ok || lang.Examples == nil {
			return nil, unsupportedLang("example corpus", req.Lang)
		}
		pairs, err := lang.Examples(req.Queries, req.Reading, req.Limit)
		if err != nil {
			log.Printf("/examples: corpus not loaded: %v", err)
			return nil, httpErr{http.StatusServiceUnavailable, err.Error()}
		}
		if pairs == nil {
			pairs = []examples.Pair{}
		}
		return map[string]any{"examples": pairs}, nil
	}, routeOpts{noStore: true})
}
