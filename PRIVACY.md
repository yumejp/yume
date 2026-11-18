# Privacy

This note covers the self-hosted Yume server and the app it serves. Whoever
runs the server holds the data; the Yume authors never receive it. A hosted
Yume service, if you use one, has its own privacy policy.

## What the server stores

Everything stays in the server's data folder (see the
[server guide](server/README.md#data-folder)):

- your deck: saved words, card content, notes, mnemonics, tags and flags
- your review history, with the time and rating of every review
- audio and images added to cards
- app and study settings kept on the server
- accounts, when there is more than the built-in owner: email, display name,
  a bcrypt hash of the password, and hashes of session tokens
- deck snapshots you or an import create, and dictionaries you install

Text you paste into **Scan Japanese text** is processed and not stored. The
server log records which route was called, its status and how long it took,
never what you sent.

## What the app stores on your device

The server address, your session token and account email and display name
if you signed in (or the test account you picked), your display settings,
which tutor notes you have already seen, reviews waiting to sync while
offline, and a cached copy of the current review session. Clearing the site's
data or removing the home-screen app deletes them.

## What leaves your server

Yume has no analytics, telemetry, crash reporting or update checks, and sends
nothing to the Yume authors. The server connects to other sites only when you
ask for something:

- **Dictionary downloads.** `yume-server setup`, and the first start of the
  Docker image, download JMdict, KANJIDIC2 and the example sentences from
  the Electronic Dictionary Research and Development Group (edrdg.org).
  Refreshing them later does the same.
- **Japanese definitions**, if you install them (`yume-server dict ja-ja` or
  `YUME_DICTS=ja-ja`): a download from GitHub.
- **VOICEVOX**, if you install it (`yume-server voicevox`): VOICEVOX's
  release files from GitHub, the engine, voice models and dictionary its own
  downloader fetches, and Python packages from PyPI.

These requests carry your server's IP address, as any download does, and no
deck data.

## Services you connect yourself

- **Your AI tutor.** An assistant connected over MCP or the HTTP API reads
  and edits your deck. What it reads goes to that assistant's provider under
  that provider's terms. Yume does not choose or contact the provider.
- **Device voice.** With **Playback source** set to **Device voice**, your
  browser or operating system speaks the text. Some speech engines use online
  voices and send the text to their provider.
- **Links.** "Open in Jisho" and the links in Settings › About open those
  sites in your browser.

Building the Docker image or the server from source downloads packages from
npm, the Go module proxy and Alpine Linux. That happens on the build machine,
not while Yume runs.
