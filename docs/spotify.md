# Connecting Spotify

Every instance uses its own Spotify developer app: Spotify no longer lets one app be shared by a tool like this, and creating one needs a Premium account. The setup wizard shows each step with the exact values to copy.

1. Create an app (any name) in the [Spotify developer dashboard](https://developer.spotify.com/dashboard).
2. Add the redirect URI the wizard shows, exactly (scheme, host, port, path):
   - **HTTPS** (a reverse proxy listed in `TRUSTED_PROXIES`, or `BASE_URL=https://…`): `https://<your-host>/oauth/spotify/callback`. Spotify sends you straight back.
   - **Plain HTTP:** `http://127.0.0.1:<port>/oauth/spotify/callback`, with the port you open the UI on; Spotify only accepts plain-HTTP redirects to `127.0.0.1`. From another machine, the browser shows a "can't connect" page after you approve. That's expected: paste that page's full URL into the wizard. Set `SPOTIFY_LOOPBACK_PORT` if your port mapping differs.
3. Tick **Web API**, save, and paste the app's **Client ID** into the wizard. There's no client secret (spotussi uses PKCE).
4. Connect, approve the read-only permissions, and pick your time zone.

To reconnect later (a revoked token, a new app), use Settings → Spotify. A banner on every page tells you when it's needed.

## Live capture

Once connected, spotussi records listening by itself. It polls the player about every 12 s while music plays (90 s when idle) and checks recently played tracks every 20 minutes to catch anything missed. Spotify only keeps the last 50 recently played tracks, so keep the server running. Settings → System shows capture status and errors. Intervals are configurable ([configuration.md](configuration.md)).

## Artwork and metadata

Artwork, genres and durations are looked up in the background, most-played tracks first. Spotify gives apps like this a small daily lookup quota (about 150 lookups a day, observed in October 2026), so a lifetime export would take months through Spotify alone. While the quota is spent, free public sources fill the gaps:

| Source | Gives | Matched by |
|---|---|---|
| Spotify oEmbed | Artwork, titles | Exact Spotify URI |
| Deezer | Durations, release dates, pictures, ISRCs | Name, artist, album and duration |
| MusicBrainz | Artist genres | Name (1 request/s, as MusicBrainz asks) |

A large export's whole catalog fills in about 6 to 7 hours this way. Low-confidence matches are rejected, and Spotify's data replaces fallback data as its quota allows. Pages show what's known and mark provisional details. Each source can be turned off with its `METADATA_*` variable, and `HYDRATE_SPOTIFY_LOOKUPS=false` leaves Spotify's quota untouched.
