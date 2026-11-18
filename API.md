# Yume server API: build your own study flow

The API is available at `http://127.0.0.1:7654` by default. Most endpoints use
`POST /<method>` with a JSON request body and JSON response. `/health` also
supports GET, and `/export_apkg` returns a binary file.

Authoritative source: `server/internal/server/handlers*.go`.

## Conventions

- **Optional fields.** The server omits empty arrays/fields (`omitempty`), so
  treat most response fields as optional. `mobile/src/api/types.ts` mirrors
  the shapes in TypeScript.
- **Writes** require `Content-Type: application/json` (CSRF guard).
- **Host.** The `Host` header must be loopback, the `-bind` address or a name
  in `YUME_ALLOWED_HOSTS`.
- **Trusted clients** can call every endpoint; in open auth mode each acts as
  the owner. They are:
  - clients that send no `Origin` (curl, MCP, native apps);
  - the bundled web app on the server's own origin;
  - installed browser extensions. One with host permissions for the server
    needs no CORS headers; one without them gets CORS only when its exact
    origin is in `YUME_EXTENSION_ORIGINS` (comma-separated, e.g.
    `chrome-extension://<id>`);
  - web pages on the `-bind` address or a `YUME_ALLOWED_HOSTS` name, on any
    port, such as a Metro web preview on the server's machine.

  Other web pages are rejected; localhost pages pass only when the server
  runs with `YUME_DEV_CORS=1` (dev only).
- **Timestamps.** Every table carries `created_at` and `updated_at` (unix
  seconds) kept by the database; writes never need to send them.
- **Language.** Any request may carry `lang` (or `?lang=`), default `ja`.
  Statuses, tags, suggestions, cards, kanji stories and imported dictionaries
  are scoped to it.
- **Days.** Day-based methods accept `tz_offset_minutes` (minutes east of
  UTC, i.e. JS `-new Date().getTimezoneOffset()`) so "today" buckets at your
  midnight.
- **Encoding.** Send bodies as UTF-8. Writes refuse a Japanese field (lemma,
  kanji, readings, sentence, note, mnemonic or kanji story) that is only `?`
  or contains U+FFFD: that is what a Windows shell makes of Japanese passed
  inline, so send the JSON from a file (`curl --data-binary @body.json`) or
  a script.
- **Errors** are `{ ok: false, error }` with a 4xx status for a bad request
  and 500 otherwise. A few carry a stable `code` a client can branch on
  instead of the message; today: `reviewed_at_rejected` (a replayed review is
  older than the card's last review, so drop it).

## Auth

By default the server runs in **open** mode: unauthenticated requests act as
the owner account, so a personal localhost setup needs no auth and can skip
this section.

- `X-Yume-User: test` (or `owner`) picks the sandbox or owner deck in open
  mode; only these role names are accepted.
- Open mode on a non-loopback `-bind` starts only with
  `YUME_ALLOW_OPEN_NETWORK=1`.

Multi-user is opt-in: run with `YUME_AUTH_MODE=required` (setup steps in
[server/README.md](server/README.md#accounts-separate-decks-on-one-server)).
Any value other than `open` or `required` stops the server.

- Each account gets its own deck. Use `/auth/login` for a bearer token,
  `/auth/logout` and `/auth/me`; send `Authorization: Bearer <token>` on
  every call.
- A missing or expired session is 401 on every route except `/health`,
  `/auth/*` and the bundled app's own files.
- `/auth/signup` creates accounts only while signup is open
  (`YUME_SIGNUP=open`; closed by default in required mode), and answers 403
  with code `signup_closed` otherwise.
- `/health` reports both as `auth: { mode, signup }`.
- The owner has no password until you run `yume-server users owner EMAIL`.
  `yume-server users` also lists accounts, resets a password, exports a deck
  as `.apkg` and deletes an account.

## Versions

`/health` tells a client which server it is talking to:

- `version` is the release (`0.1.0-alpha`), `dev` for an untagged build;
  `commit` is the git commit it was built from, empty when unknown.
- `api_level` is the HTTP contract the server speaks, an integer raised in
  every release that adds or changes something clients rely on. A client
  that needs level N and finds a lower one (or none: servers before 0.1.0
  omit it) should say the server is too old and needs updating, rather than
  fail on individual calls.
- `min_client_api` is the oldest client level the server still supports. A
  client below it should ask the learner to update the client.
- `schema_version` is the database schema, independent of the other two.

Clients may name themselves in an `X-Yume-Client` header, such as
`extension/0.1.0`; browser extensions may send it cross-origin.

## Language

| Method | Body | What it does |
|---|---|---|
| `/health` | — | `{ ok, ready, version, commit, dict, schema_version, api_level, min_client_api, auth }`. `ok` is liveness, `ready` means lookups work too; `dict` is `{ loaded, entries, error }`, where `error` says how to install a missing dictionary; `auth` is `{ mode: "open" \| "required", signup: bool }`. For the version fields see [Versions](#versions) |
| `/tokenize` | `{ text, lang }` | Morpheme tokens (kagome + IPADIC for `ja`) |
| `/tokenize_batch` | `{ texts, lang }` | Same, many lines in one round-trip |
| `/lookup` | `{ queries }` | Dictionary entries (JMdict + imported dicts + pitch/frequency), ranked by commonness. Pass `[lemma, surface]` and dedupe by entry id. JMdict entries carry `priority` (nf band 1–48, 60 or 80 for the ichi/news/spec classes) and `common` when a form is tagged ichi1 or news1. Each entry carries `lang`, the language of its glosses (`en` for JMdict, the dictionary's `gloss_lang` for imported ones). Imported-dictionary entries have negative ids and a `dict` name (e.g. the Japanese Wiktionary from `yume-server dict ja-ja`) |
| `/search_english` | `{ query }` | Reverse lookup: English word/phrase → Japanese entries whose glosses match, ranked best-first |
| `/examples` | `{ queries, lang, limit? }` | Tanaka Corpus example sentences with translations |
| `/kanji_info` | `{ kanji }` | KANJIDIC2 facts, the effective story, deck words containing one kanji, `known` (the kanji is in a Known word) and `components`: the parts it is explained through, from the mnemonic of its single-kanji word, each with its own story and `known` |
| `/set_kanji_note` / `/set_shared_kanji_note` | `{ kanji, note }` | The deck's own story for one kanji (an override), or the server-wide shared one (owner only); empty clears. `/kanji_info` returns the effective story with `note_shared: true` when it is the shared one |
| `/add_user_word` / `/remove_user_word` / `/list_user_words` | | Custom tokenizer words (names, slang) that override the built-in dictionary |

## Cards & reviews (FSRS)

| Method | Body | What it does |
|---|---|---|
| `/save_card` | `{ lemma, jmdict_id?, kanji?, readings?, glosses?, sentence?, source_url?, audio?, image? }` | Create a card (dedup on `jmdict_id`; media as base64 sidecars). An imported card for the same word, which has no `jmdict_id`, takes the id instead of getting a second card (`created: false`) and gains the sentence and media it lacked |
| `/card_status` | `{ jmdict_ids, lemmas? }` | Which of these words are already saved as cards |
| `/update_card` | `{ card_id, sentence?, translation?, explanation?, lemma?, ... }` | Edit a card's fields in place. `translation` is the sentence's English, `explanation` the note for tricky words (empty string clears) |
| `/get_due_cards` | `{ limit?, new_limit?, tz_offset_minutes? }` | Due cards, ordered by due date, each with `translation` and `explanation` when set, `mature` once its interval reaches three weeks, `preview` (next review per rating, plus `at`, when it was computed) and, for cloze cards, `cloze_target`, the text the front hides. Send `new_limit: 0` for a pure review session; a positive value releases that many never-seen cards |
| `/review_card` | `{ card_id, rating, review_id? }` | Rate 1–4 (Again/Hard/Good/Easy); FSRS reschedules. `review_id` makes retries idempotent |
| `/undo_review` | `{ tz_offset_minutes? }` | Undo the most recent review of the day (repeatable, walks backwards) |
| `/delete_card` | `{ card_id }` | Remove a card (word status survives) |
| `/deck_stats` | `{ tz_offset_minutes? }` | Totals, due counts, new-cards-introduced-today |
| `/stats` | `{ tz_offset_minutes? }` | Full dashboard payload: streak, heatmap, forecast, reviews today. `totals.due_now` and `due_forecast` count reviews only; never-seen cards are `totals.new_waiting` |
| `/add_card_note` / `/list_card_notes` / `/delete_card_note` | `{ card_id, kind, note, translation? }` | Notes beside a card: `kind` is `tutor`, `example` (with the sentence's `translation`) or `triage`; rows come back with their kind. A mnemonic is a card field, not a note |
| `/set_mnemonic` | `{ card_id, parts[{kanji, meaning}], story }` | Write the card's own mnemonic, overriding the shared one for its lemma (parts checked against the lemma; empty parts and story clear the override). Every card payload carries `mnemonic: {parts, story}`, with `mnemonic_shared: true` when it is the shared one |
| `/shared_mnemonic` / `/set_shared_mnemonic` | `{ lemma }` / `{ lemma, parts, story }` | The server-wide mnemonic library, one entry per lemma, shown to every deck whose card has none of its own. Writes are the owner's. `yume-server mnemonics share` moves a deck's mnemonics and kanji stories into it; `export` / `import` move it as JSON |
| `/mnemonic_audit` | `{ kind?, kana_too? }` | Every card's effective mnemonic state: `ok`, `no_breakdown`, `missing`; `shared: true` on rows read from the library |

## Card media

| Method | What it does |
|---|---|
| `/get_card_media` | A card's audio/image sidecars, base64 |
| `/add_card_media` / `/list_card_media` / `/delete_card_media` | Multiple labeled clips/images per card; `source` = `<engine>:<id>` and `voice` = credited name (see docs/card-content.md, Audio) |
| `/list_media_voices` | Every credited voice in the deck with clip counts (the About credit) |
| `/set_primary_media` | Choose which clip plays first on reveal |
| `/attach_media` | Attach audio/image to an existing card (e.g. batch TTS generation over the whole deck) |
| `/get_card_media_file` | One media file by id |
| `/sweep_media` | Maintenance: delete orphaned sidecar files nothing references |

## Word knowledge

| Method | Body | What it does |
|---|---|---|
| `/set_word_status` | `{ lemma, status }` | 0 unknown (clears) · 1 learning · 2 known · 3 ignored |
| `/get_word_statuses` | `{ lemmas }` | Batched read (only explicit rows come back) |
| `/word_status_counts` | — | `{ learning, known, ignored }` |
| `/list_vocab` | `{ filter{ status?, has_card?, search?, tags?, new?, band?, sort? }, limit, offset }` | Paginated vocab explorer query; `sort: "frequency"` ranks common words first, `sort: "lapses"` the most-forgotten cards first (rows carry `reps` and `lapses`) |
| `/vocab_bands` | `{ filter }` | Per-frequency-band status counts and coverage totals |
| `/set_word_tags` / `/add_word_tag` / `/remove_word_tag` / `/list_all_tags` | | Free-form per-word tags |
| `/list_system_tags` / `/tag_card` | | Reserved app-defined flags (needs-audio, flagged, …) for triage workflows |
| `/delete_by_tag` | `{ tag, include_studied?, dry_run? }` | Undo a tagged batch: deletes the tagged words' cards, statuses and tags. Words studied since (any review or note on their cards) are kept unless `include_studied: true` and come back as `kept_studied` / `kept_lemmas`. `dry_run: true` reports the split and deletes nothing. Media files move to `media/.trash` for 30 days, and restoring a snapshot that still has the cards brings them back. Refuses system tags and `seen-in:` tags |

## Video library

| Method | What it does |
|---|---|
| `/save_video_snapshot` | Per-video coverage snapshot (which words appear in which lines) |
| `/list_video_snapshots` | Your analyzed videos ranked by readiness (coverage %, i+1 lines) |
| `/get_video_vocab` | One video's study-relevant words, sorted by how many library videos share them |
| `/delete_video_snapshot` | Remove one |

## Import / export / backup / misc

| Method | Body | What it does |
|---|---|---|
| `/import_csv` | `{ rows, tag, default_status?, dedup?, create_cards? }` | Anki-style import in one transaction, after an automatic snapshot: `backup` names it (`backup_path` is its server path) and `pruned_backups` lists older pre-import snapshots it replaced. Only words the import creates get `tag`, so `/delete_by_tag` removes those and nothing else; words already in the deck (a status or a card) are skipped and marked `seen-in:<tag>`. Learning lists (`default_status` 1, the default) become cards; Known (2) and Ignored (3) lists set the status only unless `create_cards: true` (`skipped_cards` counts them). Lemmas lose HTML and `漢字[かな]` furigana; `skipped` lists each row not added, with the reason. With `dedup: false`, a word already in the deck can get an extra card, which the undo leaves: pass `backup` to `/restore_backup` to remove it |
| `/export_apkg` | | Full deck as an Anki `.apkg` **(binary response)**. Scheduling, review history, media, and tags survive the trip |
| `/create_backup` / `/list_backups` / `/restore_backup` | | Managed snapshots of the whole store. Each snapshot has a `kind`: `manual`, `import`, `prerestore` or `premigrate` (taken before a server upgrade changed the database). The newest 3 of each kind are kept, so automatic snapshots never prune a manual one; `pruned` lists what a create or restore removed. Restore takes a `prerestore` snapshot first. The app lists and creates them under Settings → Your data when the server allows these routes |
| `/create_dict` + `/import_dict_chunk` / `/import_pitch_chunk` / `/import_freq_chunk`, `/list_dicts`, `/delete_dict`, `/set_dict_enabled` | | Import Yomitan-format dictionaries, pitch-accent and frequency data. `create_dict` takes `gloss_lang` (default `en`), the language its definitions are written in |
| `/get_settings` / `/save_settings` | | Per-account client-settings blobs by key, opaque to the server. The mobile app syncs its settings this way; your client can use its own key |
| `/mine_transcript` | `{ text, lang, min_count?, limit? }` | Scan Japanese text for recurring words you do not know; returns counts, readings, glosses, and a context sentence per word. The submitted text is not stored |
| `/add_watch_words` | | Turn accepted mined words into suggestions (the HTTP twin of MCP `suggest_words`) |

## AI tutor (MCP)

`POST /mcp` speaks Model Context Protocol over Streamable HTTP. Connect a
compatible client to `http://127.0.0.1:7654/mcp`; setup examples for Codex,
ChatGPT desktop, and Claude Code are in [docs/ai-cards.md](docs/ai-cards.md).
The tools include `get_learning_snapshot` (deck overview), `suggest_words`
(push "look out for these" words), `list_tagged_words`, `list_videos` /
`get_video_vocab`, backup tools, and card-triage flags. The related HTTP reads
are `/list_watch_words` / `/remove_watch_word`.

## A minimal client

```sh
# what's due?
curl -s -X POST localhost:7654/get_due_cards \
  -H 'Content-Type: application/json' -d '{"limit": 5, "new_limit": 0}'

# rate card 42 "Good"
curl -s -X POST localhost:7654/review_card \
  -H 'Content-Type: application/json' -d '{"card_id": 42, "rating": 3}'
```

Those two calls are enough for a minimal review client; the server remains the
source of truth for scheduling.
