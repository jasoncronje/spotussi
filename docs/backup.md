# Backup and restore

The database holds everything: your history, settings, the owner password hash, API token hashes and the encrypted Spotify connection.

## Backup

Back up while the server runs, either way:

- **Download:** the backup button in Settings → System (`GET /api/v1/system/backup`; needs a signed-in browser, API tokens are refused).
- **Command line:** `spotussi backup <file>`, or a directory ending in `/` for a `spotussi-YYYYMMDD-HHMMSS.db` inside it:

  ```sh
  docker exec spotussi spotussi backup /data/backups/
  docker cp spotussi:/data/backups/. ./backups/
  ```

Both write a consistent snapshot (SQLite `VACUUM INTO`, so the server keeps writing meanwhile) and check it with `PRAGMA integrity_check` before reporting success.

**The instance key is not in the backup.** `/data/key` encrypts the Spotify connection. Copy it once and keep it with your backups:

```sh
docker cp spotussi:/data/key ./backups/key
```

Without it, a restored database still works, but you reconnect Spotify in Settings.

## Restore

Restore with the server stopped (the command refuses otherwise). It checks the backup, keeps the current database as `spotussi.db.before-restore-<time>` and swaps the backup in:

```sh
docker compose stop spotussi
docker compose run --rm --user root -v ./backups:/backups spotussi restore --key /backups/key /backups/spotussi-20261018-120000.db
docker compose start spotussi
```

- `--user root` lets it read backup files owned by your user; it hands the restored files back to the container's user.
- Leave out `--key` when restoring on the same instance (its key hasn't changed).
- Delete the `*.before-restore-*` files from `/data` once everything looks right.
- A backup from an older 0.x release restores into a newer one (migrations run on start), not the other way round.
