# Preparing an alpha

## Before tagging

- **CI.** Wait for the Checks workflow to pass on the main commit you will
  tag: server tests and vet, reachable Go vulnerability scan, mobile types
  and tests, the fixture browser journey (interaction behavior), the Docker
  build/startup/restore smoke test, and the browser journey against the
  image. The Docker test uses the real server and disposable volumes:
  dictionary download, card creation, rating, restart, a volume written by
  the older root-running image, and restored image/audio bytes. Run it
  locally with `docker build -t yume-ci .` followed by
  `node scripts/docker-smoke.mjs`.
- **Fresh install.** On an empty Docker volume, follow the README: start the
  server, search and save a word, review it, restart, and verify the card
  remains. Then exercise the backup/restore instructions with disposable
  data, including audio and images.
- **Phones.** Check phone access separately from the desktop browser, with
  the [device check](#device-check).
- **License notices.** `node scripts/third-party-notices.mjs --check` passes
  (CI runs it), and any published image, binary or archive carries
  `LICENSES.md` and `server/internal/notices/THIRD_PARTY_NOTICES.txt` (the
  Docker image puts both under `/usr/share/doc/yume/`).
- **Dependencies.** Review `npm audit` in `mobile` and dependency updates. Do
  not use `npm audit fix --force` to bypass Expo compatibility; review
  remaining findings and whether they affect build tooling or the shipped
  app. The weekly Dependency audit workflow fails on any high or critical
  advisory that [dependency-review.md](dependency-review.md) has not
  recorded.
- **Public history.** Before changing repository visibility, scan the history
  that will become public for credentials and personal data, including
  screenshots. Pattern scans alone are not proof that no sensitive material
  exists.

TODO(upgrade smoke): once a first tag exists, and before the first migration
past the baseline schema merges, add a step to `scripts/docker-smoke.mjs` that
starts the previous tagged image on a volume, writes cards, a review, media and
an account, then starts the new image on the same volume and checks the data
and the `premigrate` snapshots. Until then there is nothing to upgrade from.

## Versions

Releases are `vMAJOR.MINOR.PATCH`, with a prerelease suffix during the alpha
(`v0.1.0-alpha`, `v0.1.0-alpha.2`). One number covers the server and the app.
Set it in:

- `mobile/package.json` (the app's About screen shows it). Keep
  `mobile/app.json` at the plain `MAJOR.MINOR.PATCH` that native builds
  accept.
- The example tags in the README, `server/README.md` and
  `server/deploy/pi/README.md`.

The server's own version comes from the tag at build time and is `dev` for
untagged builds.

`/health` also reports `api_level` and `min_client_api`
(`server/internal/server/handlers_parse.go`):

- Raise `APILevel` by one in every release that adds or changes an endpoint
  or field a client relies on, and note it in `CHANGELOG.md`, so the
  extension and the app can tell a server that is too old for them.
- Raise `MinClientAPI` only when the release removes or breaks behavior that
  older clients use.

`schema_version` is the database's number and is independent of both.

## Device check

CI runs the app in desktop Chromium with typed ASCII; friends will use Safari
on an iPhone and a Japanese keyboard. Before inviting anyone, spend about ten
minutes on one iPhone and one Android phone, both opening the server's HTTPS
address from the README's Tailscale Serve recipe. Note each device, OS and
browser version, and the server commit.

1. **Install.** iPhone: Safari, Share, **Add to Home Screen**. Android:
   Chrome menu, **Install app** (it must be offered). Open Yume from the
   icon: it fills the screen without browser bars and shows the moon icon.
   In required mode, sign in from the icon, then close and reopen the app:
   you are still signed in and your settings kept.
2. **Japanese input.** With the kana keyboard (flick or 12-key) and the
   romaji keyboard, type into Dictionary search, **Scan Japanese text**, and
   a card's edit fields. Conversion candidates commit as chosen, no
   characters double or vanish, and confirming a conversion with Enter does
   not submit early.
3. **Audio.** On a card with clips, the first tap on **Listen** plays. With
   auto-play on, revealing the next answer plays without another tap. Try
   once with the silent switch on.
4. **Japanese glyphs.** Kanji such as 直, 骨 and 誤 use Japanese forms, and
   furigana sits over the right characters.
5. **Keyboard and sheets.** Open **Scan Japanese text** and focus the text
   box: the box and **Find repeated words** stay visible above the keyboard.
6. **Offline and reload.** Start a review, turn on airplane mode, rate three
   cards, and turn it off: the ratings sync once and the due count drops by
   three. Reload the app while online: settings are intact and the due
   count still matches the server. With airplane mode on, close the app and
   open it from the icon: over the HTTPS address it opens from its saved
   copy instead of a browser error page. Note what Review shows.

Record failures as issues before the invitation goes out, and mention the
tested devices in the release notes.

## Tag and publish

Describe the release in `CHANGELOG.md`, including upgrade steps, and merge it.
Then tag the tested main commit and push the tag:

```sh
git fetch origin && git tag -a v0.1.0-alpha origin/main -m "Yume v0.1.0-alpha" && git push origin v0.1.0-alpha
```

The tag runs the Checks workflow again. After the checks pass, its release
job:

1. refuses a tag that is not on main;
2. builds the image for linux/amd64 and linux/arm64 with the version and
   commit stamped in;
3. pushes it to `ghcr.io/yumejp/yume` as `:v0.1.0-alpha` plus the moving
   `:alpha` tag (a stable release gets `:vX.Y` and `:latest` instead);
4. checks that both architectures report the tagged version;
5. drafts a GitHub release.

Finish the draft's notes and publish it: describe the alpha's limitations
(self-hosted setup, external tutor connection, and which platforms were
tested) and any upgrade steps. Do not imply a tagged release or a successful
restore test exists until it does.

Main commits keep publishing single-architecture `:sha-<commit>` images for
testing. Dispatching the workflow on any other branch runs the checks but
publishes nothing.

While the repository is private, its GHCR package is private too: pulling
needs `docker login ghcr.io` with a token that can read packages, so friends
build from the tag. After the repository becomes public, make the package
public by hand in its package settings (this cannot be undone); the next
tagged release then also gets a build provenance attestation, verifiable with
`gh attestation verify oci://ghcr.io/yumejp/yume:<tag> -R yumejp/yume`.

On the day the repository becomes public, also switch on **Private
vulnerability reporting** under **Settings → Advanced Security**. GitHub
does not offer it for private repositories, and the security policy and the
code of conduct both send reports through it.

## Upgrade notes for v0.1.0-alpha

Put these in the first tagged release's notes:

- The Docker image now runs the server as uid 10001. Its first start after
  the update gives the existing files in `/data` to that user; nothing to do
  by hand.
- The Compose volume now has a fixed name, `yume_yume-data`, which is the
  name earlier versions used for a clone in a folder named `yume`. A clone in
  any other folder must set `YUME_VOLUME=<folder>_yume-data` in `.env` before
  updating, or it starts with an empty deck (the old volume stays intact).
  A deck started with the removed `server/docker-compose.yml` is in
  `server_yume-data`: set `YUME_VOLUME=server_yume-data`.
- Native builds report version `dev` unless built with the `-ldflags` shown
  in the README.
