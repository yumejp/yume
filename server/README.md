# Yume server (Go)

The HTTP server every Yume client builds on. It loads JMdict once, tokenizes
Japanese with kagome, schedules reviews with FSRS, stores each account in
SQLite, and serves the bundled web app. It listens on `127.0.0.1:7654` by
default.

Phones, browsers, scripts and AI tools share one deck through the same
plain JSON-over-HTTP API (`/export_apkg` is the one binary response). The
endpoint tour is in [`API.md`](../API.md);
`internal/server/handlers*.go` is the authoritative source.

## Layout

```
server/
  main.go                 entry point — subcommands (setup, voicevox, dict, mnemonics, users), flags, serve
  internal/
    server/               HTTP routes + handlers + security middleware
      handlers*.go        /health, /tokenize, /lookup, /save_card, … (~60 endpoints)
      middleware.go       Host/Origin/Content-Type allowlists, body cap, CORS
      auth.go             open vs required auth modes (YUME_AUTH_MODE)
    language/             per-language capability registry (tokenizer + dict per lang)
    tokenize/             kagome wrapper + user dictionary
    dict/                 JMdict loader + in-memory lookup index
    examples/             Tanaka Corpus example sentences
    auth/                 user/session registry (users.db)
    mcpserver/            the /mcp Model Context Protocol endpoint
    setup/                `yume-server setup` (dictionary downloads) and `voicevox`
    webapp/               serves the bundled app from dist/ (see scripts/build-web.sh)
    kanjidic/             KANJIDIC2 loader for /kanji_info
    srs/                  SQLite card store
      store.go            DB open + path resolution (per-user stores)
      migrations.go       user_version-tracked schema, applied additively
      review.go           FSRS scheduling (go-fsrs)
      + vocab/notes/media/stats/import/dicts/backup
  cmd/
    seed/                 dev tool: backdates a test DB for dashboard QA
    parsebench/           tokenizer-quality benchmark vs a UD gold set
  scripts/build-web.sh    bundles ../mobile into internal/webapp/dist
  tools/voicevox/         the card-audio batch (installed by `yume-server voicevox`)
  deploy/pi/              run it on a Raspberry Pi over Tailscale (systemd unit)
```

## Run

```sh
cd server
sh scripts/build-web.sh          # optional: bundle the app (needs mobile/node_modules;
                                 # in PowerShell: cd ../mobile; npm run export:server)
go build -o yume-server .
./yume-server setup              # downloads JMdict, examples and KANJIDIC2
./yume-server dict ja-ja         # optional: Japanese definitions (Wiktionary)
./yume-server
# -> listening on http://127.0.0.1:7654
```

`yume-server help` lists the subcommands. Without the bundled app the root
URL shows a short notice and the API still works.

Smoke test:

```sh
curl http://127.0.0.1:7654/health
# {"dict":{"entries":<count>,"error":"","loaded":true},"ok":true,"schema_version":<n>,"version":"<version>"}

curl -X POST http://127.0.0.1:7654/tokenize \
  -H 'content-type: application/json' \
  -d '{"text":"仕事ができる人の特徴","lang":"ja"}'
```

### Accounts

`yume-server users` manages accounts, so nobody edits `users.db` by hand:

```sh
./yume-server users list                        # email, role, password set, sessions
./yume-server users owner you@example.com       # the owner's sign-in for YUME_AUTH_MODE=required
./yume-server users set-password a@example.com  # new password, other sessions signed out
./yume-server users export a@example.com        # the deck as an Anki .apkg
./yume-server users delete a@example.com -yes   # account, sessions and deck folder
```

New passwords are generated and printed once; `-password-stdin` reads one
instead. Stop the server before `delete` so the deck folder can be removed.

### Data folder

Everything the server writes or downloads lives in one folder: accounts
(`users.db`), the shared library (`content.db`), each deck
(`users/<id>/cards.db`) and its media, `userdict.txt`, the optional
`srs-tuning.json`, and the dictionary files.

The folder is `-data DIR` (accepted by any command), else `$YUME_DB`'s
folder, else the OS default (`~/.local/share/yume`,
`~/Library/Application Support/Yume`, `%LOCALAPPDATA%\Yume`). The server
logs it at startup. To back up, stop the server and copy the folder.

### Flags and environment

| Setting | Effect |
|---|---|
| `-bind ADDR` | Address to listen on. Loopback by default; bind a private (Tailscale) address for phones, never a public one |
| `-port N` | Port, default 7654 |
| `-data DIR` | [Data folder](#data-folder) |
| `YUME_ALLOWED_HOSTS` | Extra Host names allowed besides loopback and the bind address (comma-separated, e.g. a MagicDNS name) |
| `YUME_AUTH_MODE` | `open` (default: every request acts as the owner) or `required` (each person signs in and gets their own deck; see [Accounts](#accounts-separate-decks-on-one-server)). Any other value stops the server |
| `YUME_SIGNUP` | `open` or `closed`: whether `/auth/signup` creates accounts. Default: open in open mode, closed in required mode |
| `YUME_ALLOW_OPEN_NETWORK` | `1` lets open mode serve a non-loopback `-bind`. Set it only for a private address (Tailscale, WireGuard): anyone who can reach it, including devices shared into your tailnet, acts as the owner |
| `YUME_EXTENSION_ORIGINS` | Browser extensions that get CORS headers, exact origins, comma-separated. Not needed for an extension with host permissions for the server, such as Yume's |
| `YUME_DEV_CORS` | `1` lets a Metro-served browser build on localhost call the API. Development only |
| `YUME_DICTS` | Optional dictionaries to install on first start, comma-separated (`ja-ja` = Japanese definitions). Same as running `yume-server dict ja-ja` |
| `YUME_JMDICT`, `YUME_EXAMPLES`, `YUME_KANJIDIC` | Override the path of one dictionary file |
| `YUME_SRS_TUNING` | Path of the scheduler tuning file (JSON keys `request_retention`, `min_non_again_interval_hours`, `mature_lapse_floor`, `mature_lapse_threshold_days`; see `internal/srs/tuning.go`). Default `srs-tuning.json` in the data folder; a missing file means the built-in defaults, a malformed one stops the server. Startup logs the values and the path |

## Accounts: separate decks on one server

In the default `open` mode every request acts as the owner account: one
person, one deck, and anyone who can reach the port has full access. To give
each person their own deck, switch to `YUME_AUTH_MODE=required`:

1. **Give the owner a password first.** The owner account (your existing
   deck, the MCP tutor, and the shared dictionaries) starts without an email
   or password, and required mode refuses every request without a session.
   Run `yume-server users owner you@example.com`, or you are locked out of
   your own deck and your tutor.
2. **Restart with `YUME_AUTH_MODE=required`.** The app now opens on a sign-in
   screen; sign in with the owner's email and password. **Settings → Account**
   shows who is signed in and signs out. Sessions last 30 days.
3. **Let people join.** New accounts are turned off in required mode. Restart
   with `YUME_SIGNUP=open`, have each person choose **Create an account** on
   the sign-in screen, then remove the setting and restart. While signup is
   open, anyone who can reach the port can create an account.
4. **Reconnect the tutor.** `/mcp` now needs the owner's session: get a token
   with `POST /auth/login` and send it as `Authorization: Bearer <token>` from
   your MCP client. Other accounts get 403 there. Renew it when it expires.

Required mode has no invitations and no per-account quotas, so keep it on a
private network such as your tailnet and never publish its port on the
internet. Also:

- The bundled app's own files load without a session so the sign-in screen
  can appear; every API route needs one.
- Reset a forgotten password with `yume-server users set-password EMAIL`.
- Switching back to `open` needs no migration: every account's deck stays in
  `users/<id>/cards.db`, and open mode serves the owner's.

## Request log

The server writes one JSON line per API request to standard error, next to
its start-up messages. `docker compose logs yume` or `journalctl -u
yume-server` shows them. A request record looks like this:

```json
{"time":"2026-11-18T09:15:02.118Z","level":"INFO","msg":"HTTP request","request_id":"K3TQ…","route":"/save_card","event":"http.request","method":"POST","status":200,"duration_ms":4}
```

- `route` is the endpoint's registered path, not the URL that was requested.
- `level` follows the status: INFO, WARN for 4xx, ERROR for 5xx.
- `request_id` is also returned in the `X-Request-ID` response header, so a
  client report can be matched to its line.
- A handler that fails unexpectedly is answered with a 500 and recorded with
  `"panic":true`.

Saving a card adds a `card.save` record with the same `request_id` and an
`outcome`: `created` (with `card_id`), `existing` (the word already had a
card; with `card_id`) or `failed` (with the `status` returned). A deck that
cannot be opened adds `store.open_failed` with an `error_kind`.

The `/auth/signup`, `/auth/login`, `/auth/logout` and `/auth/me` routes emit
the same request record plus `remote_ip`, so failed logins (401) and the rate
limit (429) are visible. Requests the security or auth middleware refuses emit
one `http.rejected` record at WARN with `request_id` (also returned in
`X-Request-ID`), `route` (the path, cut to 64 characters), `method`, `status`,
`remote_ip`, and a fixed `reason`: `forbidden_host`, `forbidden_origin`,
`no_token`, `invalid_token`, or `not_owner`. Behind a reverse proxy
`remote_ip` is the proxy's address.

These records omit bodies, query strings, credentials, emails, user text, and
error or panic contents. They are route/action diagnostics, not a full access
log: preflights, unmatched routes, streaming/MCP, and other handlers outside
`jsonPost` and `/auth/*` are not recorded by this logger. Streaming response
writers are passed through unchanged.

## Build a release binary

You need [Node.js 20+](https://nodejs.org/) and `npm ci` in `../mobile` once.

```sh
sh scripts/build-web.sh
test -f internal/webapp/dist/index.html   # without it the binary serves no app
LDFLAGS="-s -w -X main.Version=0.1.0-alpha -X main.Commit=$(git rev-parse HEAD)"
go build -trimpath -ldflags "$LDFLAGS" -o yume-server .       # macOS / Linux
go build -trimpath -ldflags "$LDFLAGS" -o yume-server.exe .   # Windows
```

- The first line bundles the app; without it the binary serves a text notice
  instead. PowerShell has no `sh`: run `npm run export:server` in
  `../mobile` instead.
- `LDFLAGS` stamps the release and commit, which `/health` and the app's
  **Settings → About** report. An unstamped build reports version `dev`.

Cross-compile from anywhere (pure Go, no CGO):

```sh
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$LDFLAGS" -o yume-server.exe .
GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "$LDFLAGS" -o yume-server-mac-arm .
GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags "$LDFLAGS" -o yume-server-linux-arm64 .
```

Ship `tools/voicevox/` beside the binary, as `tools/voicevox`, so
`yume-server voicevox` can install the audio batch tool, and
`../LICENSES.md` with it.

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/ikawaha/kagome/v2` | JP tokenizer (MeCab-family, IPADIC) |
| `github.com/ikawaha/kagome-dict/ipa` | IPADIC dictionary, embedded |
| `modernc.org/sqlite` | pure-Go SQLite for the card store (no CGO) |
| `github.com/open-spaced-repetition/go-fsrs/v3` | FSRS spaced-repetition scheduler |

JMdict itself is **not** a Go dependency. `yume-server setup` downloads it
with the two optional files into the data folder, and the server loads them
at start:

| File | Used for | Without it |
|---|---|---|
| `JMdict_e.gz` | dictionary, lookups | required |
| `examples.utf.gz` | Tanaka Corpus example sentences | `/examples` returns 503 with a download hint |
| `kanjidic2.xml.gz` | KANJIDIC2 kanji facts for `/kanji_info` | kanji sheets show deck words and stories only |

`./data/<file>` next to the binary also works, and each path can be
overridden (see [Flags and environment](#flags-and-environment)). For
example:

```sh
curl -L -o data/kanjidic2.xml.gz https://www.edrdg.org/kanjidic/kanjidic2.xml.gz
```

EDRDG asks installs to refresh the dictionary files regularly. `setup` keeps
existing files, so delete them and run it again, as the root README's
"Refresh the dictionary files" section describes.

**Licensing:** JMdict is © EDRDG (CC BY-SA 4.0) and the embedded IPADIC data
carries the NAIST/ICOT notice. Both attribution obligations are collected in
[`LICENSES.md`](../LICENSES.md) at the repo root; keep that file with any
redistribution. The binary embeds `internal/notices/THIRD_PARTY_NOTICES.txt`
(LICENSES.md plus the license of every linked Go module and bundled npm
package) and serves it at `GET /licenses`, without sign-in in either auth
mode. After changing dependencies, regenerate it with
`node scripts/third-party-notices.mjs` from the repo root.
