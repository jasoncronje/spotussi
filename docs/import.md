# Importing your history

Live capture only sees what you play from now on. For everything before, request Spotify's **Extended Streaming History** on Spotify's privacy page (Account → Privacy → Download your data). It can take up to 30 days to arrive, so request it as soon as you install. The basic Account Data package isn't supported: it only covers the last year and lacks the details spotussi needs.

When the email arrives, upload `my_spotify_data.zip` in Settings → Import (or on the wizard's last step). Imports run in the background with progress; a 300 MB export takes well under a minute. Or use the command line, also while the server runs:

```sh
docker cp my_spotify_data.zip spotussi:/data/
docker exec spotussi spotussi import /data/my_spotify_data.zip
```

`spotussi import` also takes a single history JSON file or a directory.

- **Re-importing is safe.** The same or an overlapping export only adds rows that are new.
- **Nothing is thrown away.** Every row is kept as it came, so changing the play rule later applies to your whole history.
- **No double counting.** Plays already recorded by live capture are merged with the export's rows.
- **Music only in 0.1.** Podcasts, audiobooks and videos are kept in the raw data but not counted.

Imports larger than `IMPORT_MAX_BYTES` (2 GiB by default) are refused.
