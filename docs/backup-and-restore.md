# Backup and restore

For a complete backup, stop Yume and copy `/data` as shown in the
[README](../README.md#back-up-and-update). Use a new backup directory each
time; do not merge successive copies. Keep the server version and commit with
the backup: `/health` and **Settings → About** report both. A database-only
vocabulary snapshot does not include media, accounts, or the shared library.

Never restore a newer database into an older server.

## Back up and restore without Docker

A from-source or Pi install keeps everything in its data folder: `-data DIR`
if you pass it, otherwise the folder of `$YUME_DB` (the Pi service unit sets
it to `~/yume/data/cards.db`), otherwise `~/.local/share/yume`,
`~/Library/Application Support/Yume`, or `%LOCALAPPDATA%\Yume`.

To back up, stop the server (for example `sudo systemctl stop yume`), copy
the whole folder to a new destination, then start it again:

```sh
cp -a ~/yume/data ~/yume-data-backup-2026-09-08
```

To restore, stop the server, copy the backup into a new, empty folder, and
start the same server version with `-data` pointing at it:

```sh
cp -a ~/yume-data-backup-2026-09-08 ~/yume/data-restored
./yume-server -data ~/yume/data-restored
```

Keep the original folder until your library, review counts, audio and account
sign-in check out. Then point the service unit at the restored folder
(`-data`, or `YUME_DB`).

## Restore a Docker backup

First preserve the current data in a separate backup. Stop the service and
restore into a **new, empty volume**, leaving the existing volume untouched:

```sh
docker compose stop yume
docker volume create yume-restored
docker create --name yume-restore-copy -v yume-restored:/data alpine:3.24
docker cp ./yume-data-backup-2026-09-08/. yume-restore-copy:/data/
docker rm yume-restore-copy
```

Create `restore.compose.yml` beside `docker-compose.yml`:

```yaml
volumes:
  yume-data:
    external: true
    name: yume-restored
```

Start the same server version that created the backup: check out its tag
(`git checkout v0.1.0-alpha`, for example) and set `YUME_VERSION` in `.env` to
match, then:

```sh
docker compose -f docker-compose.yml -f restore.compose.yml up --build -d
```

The copied files arrive owned by root; the server's first start gives them to
its own user.

Check your library, review counts, a card's audio and image, and account login
if enabled. Keep the original volume until these checks succeed. Keep using
both Compose files for the restored installation. To return to the original
volume, stop the service and start with only `docker-compose.yml`.

## Snapshots on the server

The server also keeps snapshots of each deck database in the deck's
`backups/` folder. They cover the deck only: no media, accounts, or shared
library. The newest three snapshots of each kind are kept:

- `manual`: made on request (`/create_backup`, or the tutor's `backup_vocab`).
- `import`: taken before every import; the import response names it, and
  restoring it undoes that batch.
- `prerestore`: taken before a restore, so a restore can be undone.
- `premigrate`: taken when a new server version is about to change the
  database's schema. `users.db` and `content.db` get the same copy in the data
  folder's `backups/` (`users-premigrate-*`, `content-premigrate-*`). If the
  copy fails, that database is not upgraded and the server says why.

Automatic snapshots only replace older snapshots of their own kind, so a run
of imports never removes a manual backup.

## Roll back an upgrade

An older server refuses a database that a newer one has upgraded, and says so
when it starts. To go back to the previous version:

1. Stop the server.
2. Put the `premigrate` copies back: each deck's newest
   `backups/cards-premigrate-*-v<N>.db` becomes its `cards.db`, and the same for
   `users.db` and `content.db` if they were upgraded. `<N>` is the schema the
   copy holds. Delete any `cards.db-wal` and `cards.db-shm` (and the same for
   the other two) left next to a file you replace. Work that happened after
   the upgrade is not in these copies.
3. Start the previous server version.

If you took a full copy of the data folder before upgrading, restoring that
copy is simpler and also covers media.

## Decks from pre-release builds

Builds from before the first public release numbered their schema differently.
The current server refuses those databases with "written by a pre-release Yume
build" rather than guessing. To carry such a deck over:

1. Start the pre-release build on a copy of the old data folder.
2. Save what you want to keep. `/export_apkg` writes the whole deck, with
   review history and media, as an Anki package; `/list_vocab` returns your
   words with their status, sentence and translation.
3. Start the current server on a new data folder and bring the words in
   through `/import_csv` (see [API.md](../API.md)), one batch per status with
   `default_status`, or ask your tutor to do it.

Review history and scheduling do not carry over this way; the Anki package
keeps them. Keep the old data folder until you have checked the new deck.
