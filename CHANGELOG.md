# Changelog

Notable changes per release, newest first. Releases are git tags; see
[docs/releasing.md](docs/releasing.md) for how they are made.

## Unreleased

### Added

- Tagged releases attach standalone `yume-server` binaries with the app
  embedded, for Windows (amd64), macOS (arm64, amd64) and Linux (amd64,
  arm64), plus a `SHA256SUMS` file, so Yume runs without Docker or a
  toolchain. See "Run without Docker" in the README.

## v0.1.0-alpha (2026-11-19)

The first tagged alpha.

### Upgrading

- The Docker image runs the server as an unprivileged user (uid 10001) on
  Alpine 3.24, with a read-only root filesystem and no capabilities in
  `docker-compose.yml`. Its first start gives the existing files in the data
  volume to that user; nothing to do by hand.
- The Compose volume has a fixed name, `yume_yume-data`. A clone in a folder
  other than `yume` sets `YUME_VOLUME=<folder>_yume-data` in `.env` before
  updating, or it starts with an empty deck; the old volume stays intact.
- Install and update by release tag: `git fetch --tags && git checkout <tag>`,
  with `YUME_VERSION=<tag>` in `.env`.
- `server/Dockerfile` and `server/docker-compose.yml` are gone; build from the
  repository root. A deck started from `server/` lives in `server_yume-data`:
  set `YUME_VOLUME=server_yume-data` in `.env` to keep using it.

### Added

- `/health` reports the build's `commit`, `api_level` and `min_client_api`;
  **Settings → About** shows the server's version and commit.
- Tagged releases publish a linux/amd64 + linux/arm64 image to
  `ghcr.io/yumejp/yume`.
- The Docker image has a health check.
