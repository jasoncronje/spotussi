# Configuration

Every environment variable is optional.

| Variable | Default | |
|---|---|---|
| `PORT` | `8080` | Listen port |
| `HOST` | all interfaces | Listen address |
| `DATA_DIR` | `./data` (`/data` in the image) | SQLite database and the instance key |
| `TZ` | `UTC` | Default display time zone until you pick one in the wizard or Settings |
| `BASE_URL` | derived per request | External URL, e.g. `https://stats.example.com`. Decides the Spotify redirect URI |
| `TRUSTED_PROXIES` | none | Comma-separated IPs/CIDRs of reverse proxies whose `X-Forwarded-*` headers (and auth header) are believed. Needed behind a proxy for HTTPS detection, Secure cookies and client IPs; required for `proxy` mode |
| `AUTH_MODE` | `password` | `password` (owner password, set on first visit with the setup token from the log), `proxy` (forward-auth header from a trusted proxy) or `none` (no login; private networks only) |
| `AUTH_PROXY_HEADER` | `Remote-User` | User header set by the forward-auth proxy |
| `SPOTIFY_LOOPBACK_PORT` | request port, else `PORT` | Port in the plain-HTTP redirect URI `http://127.0.0.1:<port>/oauth/spotify/callback` |
| `SPOTIFY_RATE` / `SPOTIFY_BURST` | `2` / `10` | Spotify request budget shared by every caller (requests per second, bucket size) |
| `SPOTIFY_LIVE_RESERVE` | `0.3` | Share of the budget only live capture may use, so background lookups never delay it |
| `CAPTURE_ENABLED` | `true` | Record listening (player poller and recently-played backstop) |
| `CAPTURE_PLAYING_INTERVAL` / `CAPTURE_IDLE_INTERVAL` | `12s` / `90s` | Player poll interval while playing and while idle |
| `CAPTURE_RECENT_INTERVAL` | `20m` | Recently-played check interval |
| `HYDRATE_ENABLED` | `true` | Look up track, artist, album and playlist details in the background (Spotify first, then the fallback sources) |
| `HYDRATE_SPOTIFY_LOOKUPS` | `true` | `false` makes no Spotify catalog lookups at all and leaves the daily quota untouched; capture and the fallback sources still fill in details |
| `METADATA_OEMBED` | `true` | Fallback: Spotify oEmbed (artwork and titles by exact URI; no credentials) |
| `METADATA_DEEZER` | `true` | Fallback: Deezer's public API (durations, ISRCs, pictures, release dates; matched by name) |
| `METADATA_MUSICBRAINZ` | `true` | Fallback: MusicBrainz (artist genres; 1 request/s as MusicBrainz asks) |
| `METADATA_CONTACT` | unset | Appended to the User-Agent the fallback sources see (a URL or address for the operator, as MusicBrainz recommends) |
| `IMPORT_MAX_BYTES` | `2147483648` (2 GiB) | Largest accepted export upload |

## Command line

```
spotussi [serve]                    run the server (default)
spotussi import PATH...             import exports (zip, JSON or directory); safe while the server runs
spotussi backup FILE|DIR/           consistent database snapshot; safe while the server runs
spotussi restore [--key KEY] FILE   replace the database with a backup (server stopped)
spotussi reset-password             forget the owner password
spotussi mcp --url URL --token T    stdio proxy to an instance's MCP server
spotussi health                     exit 0 if the local server is healthy (container health check)
spotussi version
```

`spotussi serve --mock` serves a synthetic history instead of your data, with no login and no state: `docker run --rm -p 8080:8080 ghcr.io/jasoncronje/spotussi:0.1.0-beta.1 serve --mock` shows the UI without connecting anything.

**Forgot the owner password?** Run `docker exec spotussi spotussi reset-password`, then set a new one in the UI with the setup token from the log (restart the container if it has scrolled away).
