# Connecting Spotify

Every instance uses its own Spotify developer app: Spotify no longer lets one app be shared by a tool like this, and creating one needs a Premium account. The setup wizard shows each step with the exact values to copy.

1. Create an app (any name) in the [Spotify developer dashboard](https://developer.spotify.com/dashboard).
2. Add the redirect URI the wizard shows, exactly (scheme, host, port, path):
   - **HTTPS** (a reverse proxy listed in `TRUSTED_PROXIES`, or `BASE_URL=https://…`): `https://<your-host>/oauth/spotify/callback`. Spotify sends you straight back.
   - **Plain HTTP:** `http://127.0.0.1:<port>/oauth/spotify/callback`, with the port you open the UI on; Spotify only accepts plain-HTTP redirects to `127.0.0.1`. From another machine, the browser shows a "can't connect" page after you approve. That's expected: paste that page's full URL into the wizard. Set `SPOTIFY_LOOPBACK_PORT` if your port mapping differs.
3. Tick **Web API**, save, and paste the app's **Client ID** into the wizard. There's no client secret (spotussi uses PKCE).
4. Connect, approve the read-only permissions, and pick your time zone.

To reconnect later (a revoked token, a new app), use Settings → Spotify. A banner on every page tells you when it's needed.

### Permissions

spotussi asks only for read access: what's playing, recently played tracks, your private and collaborative playlists, and your saved songs and albums (`user-library-read`, for the Library page). It never changes anything in your Spotify account.

An instance connected before the Library page existed lacks `user-library-read`. It keeps working, and the Library page and Settings → Spotify say the permission is missing. To add it, reconnect once: Settings → Spotify → reconnect, and approve the consent page.

## Live capture

Once connected, spotussi records listening by itself. It polls the player about every 12 s while music plays (90 s when idle) and checks recently played tracks every 20 minutes to catch anything missed. Spotify only keeps the last 50 recently played tracks, so keep the server running. Settings → System shows capture status and errors. Intervals are configurable ([configuration.md](configuration.md)).

## Artwork and metadata

Artwork, genres and durations are looked up in the background, most-played tracks first. Spotify gives apps like this a small daily lookup quota (about 150 lookups a day, observed in October 2026), so a lifetime export would take months through Spotify alone. While the quota is spent, free public sources fill the gaps:

| Source | Gives | Matched by |
|---|---|---|
| Spotify oEmbed | Artwork, titles | Exact Spotify URI |
| Deezer | Durations, release dates, pictures, ISRCs | Name, artist, album and duration |
| ListenBrainz | Artist genres (MusicBrainz's), in batches of 100 tracks | Name, artist, album and duration |
| MusicBrainz | Genres of the artists ListenBrainz didn't find | Name (1 request/s, as MusicBrainz asks) |

A large export's whole catalog fills in about 3 to 5 hours this way (MusicBrainz is the slow step; ListenBrainz takes most of its artists off it). Low-confidence matches are rejected, and Spotify's data replaces fallback data as its quota allows. Pages show what's known and mark provisional details. Each source can be turned off with its `METADATA_*` variable, and `HYDRATE_SPOTIFY_LOOKUPS=false` leaves Spotify's quota untouched.

## Library

spotussi also reads your saved songs, saved albums and playlists, so the Library page can show which saved songs you never play, how long saved songs took to reach heavy rotation (their 5th counted play), and how much of each playlist you actually play.

- **How often:** a quick check every hour (one page each of saved songs, saved albums and your playlist list; about 3 requests when nothing changed) and a full pass once a day. A playlist's songs are read again only when Spotify reports it changed. The first sync starts 2 minutes after connecting. Settings → Spotify shows the last sync and has **Sync now**.
- **No lookup quota:** these requests don't count against the daily lookup quota, and they fill in names and artwork for the songs they return. Saved songs you have never played are not looked up.
- **Followed playlists:** since February 2026 Spotify only shares the songs of playlists you made or collaborate on. Playlists by others (including Spotify's own mixes) show their name and the plays you started from them, but not their songs.
- **History:** spotussi keeps what each sync saw, so removals and re-saves show on the saves chart from the first sync on. Spotify doesn't report when something was removed before that.
- **Played or not:** a saved song counts as played when you played that track, a version Spotify relinked it to, or the same recording on another release (same ISRC). "Other release" means a played song has the same title and main artist but the recording isn't confirmed yet.

Turn it off with `LIBRARY_SYNC_ENABLED=false`, or change the interval with `LIBRARY_SYNC_INTERVAL` ([configuration.md](configuration.md)).
