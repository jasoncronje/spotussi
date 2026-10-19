# Decisions

Running log of design decisions for the self-hosted Spotify stats app. Each entry records what was chosen, why, and what was rejected. Deferred decisions record what would trigger revisiting them and any constraint they place on work done now.

## Context: Spotify API constraints (as of Oct 2026)

These shape almost every decision below.

- **Bring your own app.** Development Mode apps require the owner to have Premium and are capped at 5 users. Extended quota requires a registered organization with 250k+ MAU. Every instance registers its own Spotify app; we cannot ship a shared client ID.
- **No batch catalog endpoints.** `GET /tracks`, `/artists`, `/albums` (multi-ID) were removed for Development Mode in Feb 2026. Metadata hydration is one request per entity.
- **Removed data.** Audio features (Nov 2024), track/artist `popularity`, artist `followers`, `available_markets` (Feb 2026). Search `limit` max is 10.
- **No push.** There are no webhooks for playback changes; live capture must poll.
- **Quota.** Rolling 30s rate-limit window, undisclosed limits; Development Mode quota is counted per developer account (Jul 2026). 429s may carry `"reason": "QUOTA_EXCEEDED"`.

Sources: [Feb 2026 migration guide](https://developer.spotify.com/documentation/web-api/tutorials/february-2026-migration-guide), [Feb 2026 blog post](https://developer.spotify.com/blog/2026-02-06-update-on-developer-access-and-platform-security), [Jul 2026 changelog](https://developer.spotify.com/documentation/web-api/references/changes/july-2026).

## Decided

### D1. One Spotify account per instance

Single-user. No `user_id` in the schema, a single owner login (or none behind a reverse proxy), one instance-level API/MCP token.

- **Why:** Simplest model; the Development Mode cap rules out a public service anyway.
- **Consequence:** Data is small (~15–25k plays/year for a heavy listener, <500k rows lifetime). Performance comes from storage layout and precomputation, not database scale.
- **Rejected:** Small-group instances (up to 5 users), public multi-tenant service.

### D2. Full-fidelity live capture via adaptive polling

- `GET /me/player` polled adaptively: ~10–15s while playing, plus one poll timed to the expected track end (`duration_ms - progress_ms`); backs off to 1–2 min when paused/idle; honours `Retry-After` on 429. Intervals are configurable.
- `GET /me/player/recently-played` every ~20 min as an authoritative backstop for plays the poller missed (server down, offline devices syncing later).
- **Why:** The poller is the only live source of real `ms_played`, skips, shuffle, device, and context. Without it, live plays and exported plays mean different things for "minutes listened" and "skip rate".
- **Reference:** Your Spotify polls only `recently-played` every 2 min and records the track's full `duration_ms` as the play duration, so it has no skip data and overstates listening time.
- **Rejected:** `recently-played` polling only.

### D3. Two data layers: raw sources and derived plays

```
raw_export_rows  ─┐
raw_recent_plays ─┼─▶ merge + dedupe + counting rule ─▶ plays ─▶ rollups ─▶ UI / API / MCP
raw_poller_plays ─┘          (pure, re-runnable)                 (precomputed)
catalog: tracks, artists, albums (filled by background hydration job)
```

- Raw rows from every source are stored verbatim and append-only.
- `plays` is derived by a deterministic, re-runnable merge: dedupe across sources, prefer the richest source, apply one play-counting rule to all.
- **Why:** Nothing is lost. Merge and counting-rule fixes apply to all history by rebuilding, with no re-import. Rebuild takes seconds at this data size.
- **Rejected:** Normalizing to a single `plays` table on ingest.

### D4. SQLite (WAL) as the only database for v1; fixed dashboards first

- One SQLite file is the system of record. Dashboards read precomputed rollup tables (per-day × track/artist/album).
- v1 ships fixed dashboards, not an ad-hoc explorer.
- **Why:** Rollups make every dashboard query single-digit ms. One file means trivial backup and no extra services, and external tools (sqlite3, Datasette) can read it while the server runs.
- **Rejected:** Postgres (extra service, no benefit at this scale). DuckDB-only store (locks the file to one process; weaker at frequent small writes).

### D5. Go backend, single static binary, pure-Go SQLite

- Go, compiled to one static binary. SQLite via `modernc.org/sqlite` (no cgo), so cross-compiling to arm64 is one command.
- Poller, metadata hydration, importer, and HTTP server run as goroutines in the same process.
- **Why:** Best balance of performance, small footprint (~15–25 MB binary, ~20–40 MB RAM idle), simple packaging, and development speed. The workload is I/O-bound, so Rust's CPU advantage wouldn't be visible.
- **Note for DF1:** `duckdb-go` needs cgo but ships prebuilt static libraries for linux/darwin amd64/arm64. Adding DuckDB means switching the release build to cgo for those targets.
- **Rejected:** Rust (slower iteration, fewer contributors), TypeScript on Bun (heavier runtime and binary, weaker for long-running background work).

### D6. SvelteKit static SPA, embedded in the Go binary

- SvelteKit with `adapter-static`, built to static files and embedded via `go:embed`. Go serves the UI and API on one port; still one file to deploy.
- The UI consumes only the versioned JSON API (`/api/v1/…`). No UI-only endpoints.
- The API is described by an OpenAPI spec; the frontend uses a typed TypeScript client generated from it.
- API responses use ETags so unchanged data is served from cache.
- **Why:** A stats app is chart- and control-heavy, which is where an SPA is strongest. Svelte compiles to small bundles with minimal runtime. The dashboard dogfoods the API, so the public API and MCP server build on endpoints that are already complete and exercised daily.
- **Rejected:** Go templates + htmx (fastest first paint, but awkward for rich chart interactions, and the UI would not exercise the public API). React SPA (heavier bundles and runtime).

### D7. Play rule: fixed 30 s by default; Plays/Time toggle per view

- **Revised 2026-10-18 (owner):** the default is now the **fixed 30 s threshold**, and the former default is renamed **half-track** (`half_track`; the legacy setting name `lastfm` is still accepted and normalized). Why: with real track lengths from hydration (D27) the half-track rule counted about a quarter fewer plays than 30 s on a large real history, and 30 s is what Spotify, Wrapped, stats.fm and Your Spotify count (Your Spotify agreed with the 30 s count to within 0.01%, year by year). No third-party names or trademarks in product copy, rule names or the API.
- **Default rule:** a row counts as a play if `ms_played` ≥ 30 s (configurable threshold).
- **Alternative:** half-track: the track is longer than 30 s and `ms_played` ≥ min(50% of duration, 4 minutes), selectable in settings. Changing the rule triggers a `plays` rebuild (D3).
- **Time is independent of the rule:** "minutes listened" always sums all `ms_played`, including rows that don't count as plays.
- **Skips are a separate measure**, not discarded (skip counts, skip rate by context/shuffle).
- **Rollups store** play count, total `ms_played`, and skip count per entity per day, so every ranking has a per-view **Plays / Time** toggle (remembered per view).
- **Source handling:**
  - Poller plays: rule applies directly (duration from the track object).
  - `recently-played` plays the poller missed: Spotify already counted them (≥~30s), so they count as plays; minutes are estimated as full track length and **flagged as estimated**.
  - Export rows before their track is hydrated: fall back to the 30s rule, then re-evaluate automatically on the next rebuild once duration is known. Show catalog hydration progress in the UI so shifting counts are explained.
- **Why (original):** the half-track rule (the scrobbling convention) is fair to long tracks. Separating count from time avoids the Your Spotify problem, where "duration" is a single global setting and live plays use full track length as listening time.
- **Rejected:** Counting every row (shuffle-skipping inflates counts). (Originally also fixed 30 s as the default; reversed 2026-10-18, see above.)

### D8. Cross-source merge: field-level, with provenance

- **Matching:** same track, end times within a tolerance window of min(~30s, half the track duration), so back-to-back repeats of one track never merge.
- **Field precedence:**
  - From the export (Spotify's own record): `ms_played`, `reason_start`/`reason_end`, shuffle, platform, country, offline, incognito.
  - From the poller (not in exports): context (playlist/album/artist), device, measured start/end.
  - `recently-played`: confirms existence; context if the poller missed the play.
- **Provenance:** every play records which sources contributed (e.g. `poller+export`). Poller-only plays contradicted by a later export are flagged, which doubles as a poller correctness check.
- **To verify during build:** whether `recently-played.played_at` marks play start or end. Calibrate against poller-measured start/end rather than assume.
- **Why:** A merged play holds more than any single source, honouring "don't omit anything". The merge is a pure, re-runnable function (D3), so improvements apply to all history.
- **Rejected:** Highest-priority source wins (live-only fields like context and device would be lost once an export arrives).

### D9. Metadata hydration: Spotify first, prioritized, resumable

Imports produce track URIs without artwork, duration, artist IDs, or genres. With batch endpoints gone, a lifetime import (20–50k tracks, 5–15k artists) takes roughly 4–10 hours to hydrate at a conservative 1–2 req/s. The strategy minimizes time-to-feels-complete:

1. **Most-played first:** hydrate in descending play-count order so top lists and artwork are complete within minutes; the long tail fills in overnight.
2. **Tracks before artists:** a track response includes album name, artwork, release date, and artist IDs. Artist calls only add genres and images. No separate album calls.
3. **One shared rate budget** for all Spotify calls, with a reserved share for live capture so imports never cause missed plays. 429s slow the whole budget down.
4. **Resumable and visible:** the queue is stored in SQLite and survives restarts; the UI shows catalog progress (which also explains play counts shifting under D7).
5. **Artwork hotlinked** from Spotify's CDN (store URLs only). Local thumbnail cache can come later.
6. **Live plays arrive hydrated**, since poller and `recently-played` responses include full track objects. Only imports need the queue.

- **Design constraint:** the hydration job takes pluggable metadata sources so DF5 can be added without restructuring.
- **Built in M4:** see D24, including the daily lookup quota found by the spike, which bounds hydration far more than the request rate does.
- **Superseded in part by D27:** fallback sources (oEmbed, Deezer, MusicBrainz) now fill the gaps; Spotify stays primary.

### D10. Historical import: Extended Streaming History only

- v1 imports listening history only from the Extended Streaming History export (`Streaming_History_Audio_*.json`).
- The basic Account Data export's history files (`StreamingHistory_music_*.json`) are not parsed. Extended is a strict superset for history (lifetime vs 12 months, second-precision `ts` vs minute `endTime`, plus URIs and all other fields).
- **Onboarding:** connect Spotify (live capture starts immediately) → request the extended export → import it when it arrives. The UI explains the wait (Spotify says up to 30 days).
- **Why:** The basic export has no track URIs, so using it needs a name-matching subsystem (search calls limited to 10 results, wrong-match risk, review UI, supersession logic) that only serves the window before the extended export arrives. Live capture already covers that window going forward.
- **Rejected:** Basic export import with name matching.

### D11. Music only in v1

- `plays`, rollups, and dashboards cover music tracks only.
- The poller does not request episodes. Non-music playback (podcast episodes, audiobooks, ads) is treated as a gap in the timeline so it doesn't distort `ms_played` measurement of adjacent tracks.
- Export rows for podcasts, audiobooks, and video are still stored verbatim in the raw layer (D3) but are not derived into `plays`.
- **Why:** Keeps the v1 model to tracks/artists/albums. Other content types need their own model (show → episode, book → chapter) and would distort minutes-listened.
- **Rejected:** Capturing all content types live in v1; full podcast/audiobook dashboards in v1.

### D12. Spotify connection: setup wizard, PKCE, HTTPS callback with paste-back fallback

Since 27 Nov 2025 Spotify only accepts HTTPS redirect URIs or loopback IPs (`127.0.0.1` / `[::1]`, any port; `localhost` rejected), so plain-HTTP LAN setups cannot receive a normal OAuth callback.

- **Redirect URI** is derived from the host the setup page was opened on, with an optional `BASE_URL` override. One URL for UI and API (D6), so no separate API/client endpoint settings.
- **Over HTTPS:** direct callback to `https://<host>/oauth/spotify/callback`.
- **Over plain HTTP:** loopback paste-back. Register `http://127.0.0.1:<port>/oauth/spotify/callback` with an explicit port (see the correction below); after approving, the browser shows a "can't connect" page, and the user pastes the full URL into the setup page, which completes the exchange via the same handler. The wizard shows a screenshot explaining that the error page is expected.
- **Setup wizard:** step-by-step Spotify app creation with a dashboard link and copyable exact redirect URIs.
- **PKCE flow, Client ID only:** no client secret to copy or store. Persist the rotated refresh token on every refresh.
- **Minimal read-only scopes:** `user-read-playback-state`, `user-read-recently-played`, `playlist-read-private` (for context names, D17). `user-library-read` only if DF9 is added.
- **Refresh token encrypted at rest** in SQLite with a key generated on first run and stored next to the database (protects DB-file copies, backups, Datasette exposure).
- **Reference:** Your Spotify uses a client secret plus a fixed `${API_ENDPOINT}/oauth/spotify/callback` from env vars, and its README still shows an `http://localhost` redirect URI that Spotify now rejects.
- **Rejected:** Requiring HTTPS (locks out users without a reverse proxy).

### D13. Instance access control: password by default, proxy and none modes, separate API tokens

- **Default (`AUTH_MODE=password`):** owner sets a password on first run; stored as an argon2id hash; browser sessions via cookie.
- **`AUTH_MODE=proxy`:** trust a forward-auth header (default `Remote-User`) only from configured trusted proxy IPs/CIDRs. For Traefik + Authelia/Authentik/Pocket ID setups: single sign-on, no second password.
- **`AUTH_MODE=none`:** no login, for Tailscale/LAN-only setups. The UI shows a persistent warning.
- **API tokens** (for the API and MCP server) are separate from login in every mode: named per client, read-only, individually revocable, stored hashed.
- **Why:** Safe by default if a port is exposed by accident, without forcing a redundant login on users who already run an auth proxy. Separate tokens mean revoking a client never affects the owner's session, and a leaked token can only read.
- **Rejected:** No built-in auth (one misconfigured port publishes your history). Built-in OIDC (proxy mode covers SSO for a single user).

### D14. v1 dashboard scope: parity with Your Spotify and stats.fm (single-user features)

v1 mirrors the single-user features of Your Spotify (surveyed from its client source, Jul 2026) and stats.fm (surveyed from public feature write-ups). Multi-user, social, and audio-feature features are excluded (see "Not planned" and DF3). More advanced features are deferred (DF8).

**Global controls (every page)**
- Time-range picker: Today, This week/month/year, Last 7 days / 4 weeks / 6 months / 12 months, Previous day/week/month/year, All time, Custom range. (YS + stats.fm Plus)
- Plays / Time toggle per view (D7).
- Display timezone setting; day boundaries in rollups use it, and changing it triggers a rebuild. (YS)

**Overview / Home** (YS Home, stats.fm Home)
- Summary cards: time listened, plays, unique tracks, unique artists, top artist, top track. Each with change vs the previous period of equal length.
- Time listened over the period (bars by day/week/month depending on range).
- Listening clock: distribution of listening over the hours of the day.
- Top 5 artists, tracks, albums, genres.
- Recent plays strip / now playing.

**Top lists** (YS Tops, stats.fm Top)
- Tracks, artists, albums, genres. Paginated with no cap (stats.fm caps at 50 free / 100 Plus).
- Rank movement vs previous period (▲/▼/NEW).

**Stats page** (YS "All stats", stats.fm Stats)
- Time listened, plays, and different artists listened, over time.
- Artist listening distribution: share of listening held by top artists over time.
- Top artist/track for each hour of the day.
- Average album release date of what you listened to, over time.
- Average artists per track (features) over time.
- Genre breakdown over time.

**Artist, album, track pages** (YS + stats.fm detail pages)
- Your rank for the entity in the selected range.
- First and last time played.
- Total plays and time; plays over time chart.
- Listening clock for the entity (hour-of-day distribution).
- Artist: most listened tracks and albums by that artist. Album: most listened tracks on it, artists. Track: artist and album links.
- Context: which playlists/albums you played it from (poller-captured, D2/D8).

**History** (YS History, stats.fm recent plays)
- Chronological play log with filters (artist, date range, skipped, shuffle, device) and per-play provenance badge (D8).

**Longest sessions** (YS)
- Top listening sessions (continuous listening, split on gaps beyond a threshold), with the tracks in each.

**Settings and system**
- Spotify connection (D12), import (D10) with progress, catalog hydration progress (D9).
- Excluded artists/tracks: hidden from all stats (e.g. sleep sounds, kids' music); applied via rebuild. (YS "Blacklisted artists")
- Play rule (D7), timezone, date format, theme (light/dark/system).
- API tokens (D13).

### D15. Packaging: multi-arch Docker image first, standalone binaries too

- **Docker image:** multi-arch (`linux/amd64`, `linux/arm64`), built `FROM scratch` or distroless, target ~20–30 MB. One volume (`/data`: SQLite file and encryption key) and one port.
- **Zero required env vars.** Optional: `BASE_URL` (D12), `AUTH_MODE` (D13). Everything else is configured in the setup wizard.
- **README ships a `docker-compose.yml`** with Traefik labels, plus a plain-port variant.
- **Standalone binaries** on GitHub Releases (linux/darwin, amd64/arm64) for systemd/NAS users, built from the same pipeline.
- **Backups:** one-click "download database" in settings, and a `backup` CLI subcommand, both using SQLite's online backup API so they are consistent while the app is writing. (Built with `VACUUM INTO` instead, plus a `restore` subcommand: D30.)
- **Why:** Matches how people self-host and works with Traefik out of the box. Contrast with Your Spotify: three containers (server, client, MongoDB) and endpoint env vars that must agree.
- **Rejected for v1:** Binaries only (most self-hosters expect a container). Helm charts, Unraid templates, etc. (left to later or the community).

### D16. API in v1, MCP server in v1.1, SQL query tool with DuckDB

> Timing revised by D28 (2026-10-18): the MCP server ships in the first release, 0.1 beta, and the `/api/v1` stability promise starts at 1.0.

- **v1: public read-only REST API.** The same `/api/v1` the dashboard uses (D6), accessible with API tokens (D13). OpenAPI spec and docs served at `/api/docs`.
- **Stability promise:** `/api/v1` is stable once v1 ships; breaking changes go to `/api/v2`.
- **v1.1: built-in MCP server.**
  - Streamable HTTP transport at `/mcp` in the same binary, authenticated with an API token.
  - `mcp` CLI subcommand (`--url`, `--token`) runs the same binary as a local stdio proxy for clients that only support stdio servers.
  - Tools wrap existing API endpoints so results always match the UI: `get_overview(range)`, `get_top(type, range, metric)`, `get_entity(kind, id)`, `get_history(filters)`, `get_sessions(range)`, `search_catalog(query)`. Compact JSON with names and IDs so calls can be chained.
- **With DF1 (DuckDB): `query(sql)` tool** (and matching API endpoint) running read-only SQL against documented views (`plays`, `tracks`, `artists`, `albums`, …) with row and time limits.
- **Why:** The API is already built and exercised by the UI, so opening it is cheap. MCP wrappers are small but deserve their own testing against real clients. SQL access belongs with DuckDB rather than pulling DF1 into v1.
- **Rejected:** Everything in v1 (would reverse D4). API and MCP both after v1.

### D17. Play context names: lazy lookup with manual naming fallback

- Playlist contexts captured by the poller (D2) are resolved lazily: one `GET /playlists/{id}` per playlist seen, cached, using the `playlist-read-private` scope so the user's private playlists resolve.
- **Spotify-made playlists** (Discover Weekly, Daily Mixes, Release Radar, editorial) cannot be read by apps created after Nov 2024. They show as "Spotify playlist", and the owner can name each one once in the UI; the name persists since those IDs are stable per account.
- Album and artist contexts resolve through the catalog (D9).
- **Verified in M4 (D24, Oct 2026):** with `playlist-read-private` granted, `GET /playlists/{id}` returns 200 for the owner's own private and public playlists (id, name, owner, images). Spotify-made playlists answer 404. These lookups are outside the daily catalog quota, so they continue while track lookups wait for the reset (D24).
- **Why:** Much listening comes from private and Spotify-made playlists; without this, the "played from" data captured by the poller would mostly be unnamed. DF9 needs the scope anyway.
- **Rejected:** Keeping minimal scopes and showing unresolved contexts as a generic "Playlist" link.

### D18. UI components: shadcn-svelte; charts: LayerChart

- **shadcn-svelte** for UI components (Svelte 5, Tailwind CSS v4). Components are copied into the repo, so they're ours to restyle.
- **Charts via shadcn-svelte's Chart component, which is built on LayerChart.** Chart colors come from the theme's `--chart-1…5` CSS variables, so light/dark mode and theming apply to charts automatically.
- LayerChart covers every v1 chart type (bars, line/area, stacked area, radial listening clock) and renders SVG; at our data sizes (≤ a few thousand points per chart) this is instant.
- **Escape hatch:** uPlot (Canvas, ~20 KB) for any future chart needing very large point counts (e.g. DF2 explorer scatter plots), used per-chart rather than globally.
- **Rejected:** React-only libraries (Recharts, Nivo, Visx, Tremor: incompatible with Svelte). ECharts (~300 KB+, config-object theming). Chart.js (not Svelte-native).

### D19. UI as a parallel track, prototyped in code against an API contract

- The UI is built alongside the backend, not after it. M0 produces a reference dataset generator, a draft OpenAPI spec for every D14 view, and fixture responses; the SPA has a mock mode serving them.
- Design happens directly in SvelteKit + shadcn-svelte components with fixture data (no separate mockup phase). The first deliverable is the app shell plus a fully polished Overview page for owner review. A focused visual-style pass can follow if the look isn't distinctive enough.
- Approved pages define the queries M5 rollups must serve; contract changes from UI review go through the OpenAPI spec.
- **Why:** Finding data-shape gaps at M7 would force expensive rollup rework. "Simple yet powerful" is a UX judgment and gets the longest iteration time this way.
- **Rejected:** UI after the API (late feedback). Static mockups first (extra step whose output is rebuilt in code).

### D20. Data semantics settled after M0

Open questions raised by the M0 build, resolved with defaults (all changeable via rebuild, D3):

- **Unhydrated export rows:** exports carry track URIs but no artist/album IDs. Until a track is hydrated, its artist and album are provisional entities keyed by normalized name (as given in the export row); hydration replaces them with real Spotify entities on the next rebuild. Entity IDs stay opaque in the API so this is invisible to clients. Lets top artists/albums appear immediately after import.
- **Attribution:** a play counts for every artist credited on the track (so artist totals can exceed total plays), and for each genre of those artists once per play (no double counting when two artists share a genre).
- **Default timezone:** set from the browser's timezone (`Intl` API) during the setup wizard; falls back to the `TZ` env var, then UTC. Changeable in settings (D14).
- **Overview round trips:** keep granular endpoints (reused by entity pages, API, MCP). UI track U0 measures Overview load; add a composite endpoint only if the waterfall is noticeable.

### D21. Follow-ups settled after M1

- **First-run setup token:** in password mode, while no owner password exists, the server prints a one-time setup token to the logs at startup, and `POST /auth/setup` requires it. Prevents whoever reaches a freshly exposed instance first from claiming it. The wizard tells the owner where to find it (`docker logs`).
- **Live settings storage:** `GET/PATCH /settings` work on real instances (not just mock) so the setup wizard can save the timezone (D20) and other settings before the M5/M6 query backend exists.
- **Loopback redirect URIs always carry an explicit port (correction, verified 2026-10-17):** Spotify's dashboard rejects `http://127.0.0.1/oauth/spotify/callback` (no port) as "not secure", contradicting its docs, but accepts `http://127.0.0.1:8080/oauth/spotify/callback`. The plain-HTTP redirect URI is therefore `http://127.0.0.1:<port>/oauth/spotify/callback`, where `<port>` is the port the owner reaches the instance on (from the request Host, else `PORT`; `SPOTIFY_LOOPBACK_PORT` overrides). That one exact URI serves both a direct callback (instance opened on that loopback IP and port) and paste-back (any other plain-HTTP host). The wizard shows it and stresses an exact match, including the port.
- **`https://*.localhost` is accepted** as a redirect URI (verified with `https://spotussi.localhost/oauth/spotify/callback`), so Portless dev setups get a normal HTTPS callback.
- **`Forwarded` header (RFC 7239):** not supported; `X-Forwarded-*` only. Revisit on user demand.

### D22. Contract changes after U0

- **No composite Overview endpoint.** U0 measured 9 parallel requests finishing in the time of the slowest (~45 ms warm, ~450 ms cold page load to last data on 500k plays). A composite endpoint would remove progressive rendering, per-section error isolation, and metric-independent caching. Revisit only for HTTP/1.1 deployments over slow links.
- **Added in U1:** a genre detail endpoint plus a `genre_id` filter; an optional previous-period series on `/timeline`; the first-play date on range responses (custom-range picker minimum).
- **Top artist/track movement** uses `prev_rank`, with the semantics documented in the spec.
- **Theme is a per-browser preference** stored locally, not a server setting (removed from settings in D14's list).

### D23. Historical import: ingest paths, dedupe key, provisional history

Settled while building M3 (D10 says what is imported; this is how):

- **Two ingest paths.** `POST /api/v1/imports` takes the file itself as the request body (the export zip or one history JSON file; type from magic bytes, falling back to `Content-Type`; name from `?filename=` or `Content-Disposition`). The body streams to a temp file under `DATA_DIR/imports`, the server answers `202` with the queued job, and a background worker runs imports one at a time (later uploads queue in the `imports` table, so a restart resumes them). Uploads are capped by `IMPORT_MAX_BYTES` (default 2 GiB, `413 too_large`), extend the server's read/write timeouts per request, and need an owner session with CSRF like every unsafe route (API tokens stay read-only). The CLI `spotussi import <zip|json|dir>...` imports synchronously into the same database, safely while the server runs (WAL plus busy timeout).
- **Files.** Every `Streaming_History_Audio_*.json` and `Streaming_History_Video_*.json` at any depth; everything else is ignored. A package with only the basic export's `StreamingHistory_*.json` fails with `basic_export` (D10), one with neither with `no_history_files`. Job errors carry a code (`malformed_file`, `unsupported_file`, `interrupted`, `internal`) and never row contents.
- **Rows.** Every row of every content type is stored verbatim in `raw_export_rows` (D3, D11) with extracted columns: `ts_ms` (the stream's end), `ms_played`, `content_type` (track, episode, audiobook, unknown), `uri` (first non-null track, episode, audiobook chapter URI), names, reasons, flags, platform, country, and whether it came from a video file. `ip_addr` and `incognito_mode` stay in the raw layer (DF4).
- **Dedupe key:** `row_hash` = sha256 over `ts`, `uri` and `ms_played`, plus the track, artist and episode names when the URI is null; `ON CONFLICT DO NOTHING`, with new and duplicate rows counted per job. Parsed values are hashed rather than raw bytes, because Spotify re-exports overlap and can reformat or add fields; a later export of the same history must be a no-op. Rows that agree on all three but differ elsewhere (seen: about 0.04% of one real export) collapse into the first one imported.
- **Streaming everywhere.** Zip entries are read as streams and parsed one array element at a time (`encoding/json/jsontext`); inserts run in 5,000-row transactions with a prepared statement, each committing the job's progress with it. A failure keeps the rows committed before it (inserts are idempotent, so a fixed re-import completes the job).
- **Provisional history until M5.** `/history` lists export music rows next to the live sources: start = `ts` − `ms_played`; an export row whose end lies within a minute of a live row of the same track is folded into the live row (which gains `export` in its sources and the export's platform). Export artists and albums use the provisional name-keyed entities of D20 with opaque IDs (normalized name, encoded) that work as filters; the play rule uses its unknown-duration fallback (D7). Export rows are counted and paged in SQL on a partial covering index over their start time, with counts cached per data version.
- **Upload UI.** The browser uploads with XMLHttpRequest (fetch has no upload progress), sending the same cookie and CSRF header as the API client, then polls `GET /imports/{id}` about once a second until the job settles and refetches everything on screen. One upload runs at a time, held outside any page so navigating doesn't cancel it; several `Streaming_History_*.json` files are uploaded one after another. The control lives in Settings → Import (with every recent job, CLI runs included) and on the setup guide's last step. Copy says "streams" for export rows, the export's own unit; imported plays are promised in History only until M5 derives them.
- **Why:** Raw-body uploads are the simplest thing a browser (`fetch(file)`) and `curl --data-binary` both do, with no multipart parsing, and the temp file keeps memory flat at any size. Hashing semantic fields is what makes re-imports and overlapping exports no-ops. The provisional history lets the owner see imported years before the M5 derivation engine.
- **Rejected:** Multipart form uploads (more parsing, no benefit for one file). Hashing the raw row bytes (breaks on reformatted re-exports). Synchronous import in the request (a lifetime export takes ~15–20 s; the UI needs progress anyway). Importing the basic export's history (D10).

### D24. Catalog hydration: queue, seeding, quotas (settled building M4)

How D9 and D17 are built (`internal/hydrate`), and what the spike found:

- **Seeding is free.** Poller rows (last `/me/player` response) and recently-played rows (verbatim item) carry full track objects. They are upserted into `tracks`, `albums`, `album_artists`, `track_artists` and `artists` (known by name) as capture stores them (capture's `OnStored` hook), and every pre-existing raw row was backfilled once. Per-table watermarks in `settings` are written in the same transaction as the rows they cover. Local files are stored under their URI with `is_local = 1` and no credits. Relinked items (`linked_from`, the player sends a market) keep the original ID in `tracks.linked_from`. Lookups send no market, so Spotify answers with the requested track and the row's ID is always the one the raw data references.
- **Queue order.** Export track URIs that are not hydrated are queued with priority = streams of at least 30 s (the minimum of every D7 rule). Every stored track queues its unhydrated artists at the track's own priority, so a top track's artists come right after it. Albums come with every track object and get no lookups of their own (D9.2), except album contexts the catalog doesn't have. Live contexts are queued with priority = plays from them. Each planning pass covers the export rows added since the last one (the importer's `OnFinished` hook, plus a one-minute check for CLI imports) and reprioritises the tracks they mention.
- **Worker.** Batches of 8 due entries, 2 in flight, in the limiter's background lane. Results: a 429 (rate limit) is retried uncharged after the limiter's pause. Not connected or reconnect required pauses the worker without charging attempts. 404, 400 or an invalid ID parks the entry for 30 days (`last_error` "permanent: …", reported as failed). Any other failure retries after 1 min × 2^attempts, capped at 6 h. `OnCatalogChanged` (the M5 rebuild cue) fires at most every 2 min while busy, after 1,000 changes, and when the queue runs dry or pauses.
- **Contexts (D17, verified Oct 2026).** A Spotify-made editorial playlist (`37i9dQZF1DXcBWIGoYBM5M`) answers **404** to a development-mode app, so playlists with the `37i9dQZ` prefix aren't requested. They stay `unresolved` (resolved_at set) for manual naming. Any other 404 or 403 (deleted, or someone else's private playlist) also stays unresolved. Shows are skipped (D11). Liked Songs resolves without a request and keeps a null name (its type says what it is). Album and artist contexts take their names from the catalog. Manual names are never overwritten. The owner's own playlists resolve: one private and one public playlist each answered `200` with `id`, `name`, `owner` and `images` (spike re-run, 2026-10-18, with `playlist-read-private` granted).
- **Daily lookup quota (spike, Oct 2026).** Catalog lookups (`/tracks`, `/artists`) hit `429` with `"reason": "QUOTA_EXCEEDED"` and a `Retry-After` of about 18 h 39 min, counting down to about 00:08 UTC. **Measured on 2026-10-19: 506 lookups per day.** Sequential `/tracks/{id}` lookups at 10/s (spike, `SPIKE_QUOTA`) started at 00:10:43 UTC, 2.5 min after the reset, with nothing else using catalog lookups (production runs `HYDRATE_SPOTIFY_LOOKUPS=false`, D27). 506 returned 200 in 71 s, with no rate-limit 429s, and the 507th got `QUOTA_EXCEEDED`. Its `Retry-After` (23 h 57 min) ended at 00:08:41 UTC the next day, not 24 h after the window's first request, so the quota resets daily at about 00:08 UTC. The first spike's "at most ~150" was the remainder of a window that had already been partly spent. The size isn't documented and may change, so the worker keeps learning it per window (below). `/me` and `/me/player` kept working, and production capture went on uninterrupted. Spotify documents per-developer quota *buckets* but neither their sizes nor their resets. So quotas are now handled per bucket. A `QUOTA_EXCEEDED` 429 pauses only its bucket (`catalog`: tracks, albums, artists, search; `playlists`: `/playlists/…`; `user`: `/me/…`, including `/me/playlists`, and the rest) for `Retry-After`, at least `QuotaBackoff`. Requests to a paused bucket fail at once without being sent, and live capture never waits behind a spent catalog quota. **`/playlists` is not in Spotify's catalog bucket:** with `/tracks` and `/albums` returning `QUOTA_EXCEEDED`, `/me/playlists` and `GET /playlists/{id}` for the owner's own playlists still answered 200 (2026-10-18). So `/playlists` is its own `playlists` bucket in the limiter (2026-10-18): a spent catalog quota doesn't pause playlist lookups, and a `QUOTA_EXCEEDED` on `/playlists` pauses only playlists. Before this, the limiter paused every lane, which would have stopped live capture for the rest of the day. The worker keeps a pause per bucket and leaves that bucket's queue entries until its reset, uncharged. While the catalog bucket is paused it reports `quota_exceeded` with `resumes_at` and keeps resolving playlist contexts, so D17 names don't wait for the reset (album and artist contexts not yet in the catalog are catalog lookups and wait). The fallbacks (D27) keep running meanwhile, since the worker isn't `hydrating`. A spent playlist quota pauses only playlist contexts: tracks and artists carry on, and `resumes_at` stays the catalog's. Playlist lookups don't count toward the observed catalog quota. It records how many lookups a quota window allowed (`settings.hydrate_quota_lookups`; exact once a window starts at a reset), and the ETA counts whole days from that.
- **Consequence for D9's estimate.** A large lifetime export (~300k rows) needs about 44,000 lookups (about 34,000 tracks plus about 10,000 artists), and the top 1,000 tracks with their artists need about 1,800. Modelled with the fake on the export's shape (`TestModelLifetimeExport`): at the planning rate of 2/s, that's about 15 min for the top 1,000 and about 6 h for everything, *if* the rate were the only limit. With the measured 506 lookups a day, it's 4 days for the top 1,000 (~1,800 lookups: 3 full windows and part of a 4th) and about 88 days for everything (~44,000 / 506). Spotify alone can't fill the catalog in a reasonable time, so the fallbacks (D27, DF5) are needed, not optional. **Decided (owner, 2026-10-19): tracks before artists.** The worker takes every due track, album and context before any artist. The fallbacks already cover what an artist lookup adds: images from oEmbed by real ID, plus Deezer, and genres from MusicBrainz (Spotify's are coarse), lent to real artists by `Borrow`. A track lookup gives what no fallback can: the real artist IDs and credits, the album ID, and the canonical album. Queueing artists below every track hydrates the top 1,000 tracks in 2 days instead of 4, and all ~34,000 tracks in about 68 days instead of 88. Artist lookups go last, as polish for images and genres the fallbacks missed.
- **Rejected:** One global pause for every 429 (lets a spent catalog quota stop live capture). Requesting Spotify-made playlists to confirm each one (always 404, spends quota). Separate album lookups (every track object carries the album).

### D25. Derivation: in-memory snapshot over durable plays and sessions

Settled while building M5.

- **Query engine:** every API v1 read is answered from an immutable, columnar in-memory snapshot of the derived plays (start, end, ms, flags, track, context, device, platform, local day and hour, counted), with dense entity indexes and precomputed per-entity totals. A snapshot is a *base* (merge and entity resolution) plus a *view* (settings and exclusions: play rule, sessions, timezone fields), swapped atomically. Rankings tally per track and spread to albums, artists and genres; heavy all-time endpoints (hourly top) fan out over up to 8 goroutines. The SQLite tables `plays` and `sessions` (migration 0006) are the durable copy: written by every rebuild and incremental merge, loaded at startup, and readable by the planned SQL tool (D16, DF1).
- **Merge (D8) in detail:**
  - Recently-played items join a poller play of the same track, with pairs taken nearest-first across all items (|played_at − anchor|, anchor = end, or start per the calibration verdict, within a 60 s widened span). Poller plays under 25 s are not eligible, because recently-played lists only counted plays (≥ 30 s, minus poll slack). Without these two rules, a skip right after an unpolled play steals that play's item.
  - Export rows confirm the live play of the same track whose end is nearest the row's `ts`, nearest-first across all rows, within min(30 s, half the track). The export's fields win (ms played, reasons, platform, shuffle, skipped); the poller keeps its measured start and end.
  - Export rows naming a track Spotify has since relinked use the relinked ID (`tracks.linked_from`, from M4).
  - Unmatched rows are plays of their own. Recently-played-only plays have estimated length (the track's duration).
- **Entities:** catalog first, then the newest live track object, then provisional name-keyed artists and albums from export names (D20). A provisional artist whose normalized name matches exactly one real artist is that artist (albums likewise, by album and artist name). After a full resolution, all kept strings and index slices are compacted into single allocations.
- **Triggers:**
  - Startup loads the `plays` table when the fingerprint matches the last full rebuild, otherwise it rebuilds. The fingerprint covers export rows, catalog, settings, exclusions, the calibration verdict and the derivation version; it excludes the live raw tables.
  - A full rebuild runs 2 s after an import or a settings or exclusion change, and 5 s after the last `CatalogChanged()` call, at most once a minute. Settings and exclusions apply to the in-memory view at once.
  - Live rows (capture `OnNewRows`, or a 30 s poll for rows nobody announced) merge incrementally: the window starts at the last listening gap (≥ 5 min) before the new rows, and those plays are replaced in memory and in the table. With no gap within 24 h, a full rebuild is scheduled instead.
  - Watermarks of the raw tables are stored with each write, so a restart merges only the rows that arrived after it.
- **Full rebuild:** written into `plays_new` and `sessions_new`, then swapped into place in one transaction. System status reports `derive.state` and `progress`.
- **Memory:**
  - The rebuild's scratch memory is returned to the OS afterwards (`FreeOSMemory`), as is request garbage 10 s after the last request.
  - SQLite page caches are 2 MB per connection (was 16 MB), and idle connections close after 30 s.
- **Why:** At 300–500k plays, SQL aggregation per request missed the 25 ms dashboard budget on all-time ranges, and rollup tables shaped per page would multiply the write path. A columnar snapshot is ~35 MB on a lifetime export and answers every view in single-digit milliseconds. Keeping `plays` in SQLite keeps restarts fast (2.5 s load against a 4–6 s rebuild) and the data queryable.
- **Rejected:**
  - Rollup tables (D3's original plan): many shapes, a slow write path, and still too slow for arbitrary ranges.
  - Rebuilding on every start: the fingerprint makes a start a load.
  - Including the live tables in the fingerprint: every captured play would force a rebuild on the next start.

### D26. API v1 semantics settled in M6

- **Play IDs** are the derived play's numeric ID (a string in JSON). IDs survive incremental merges, and a full rebuild renumbers. A session's ID is the ID of its first play.
- **History cursor:** `<start base36>~<id base36>` of the last item. If that play is gone after a rebuild, the next page resumes after its start time.
- **Unknown scope or entity IDs** (`artist_id`, `album_id`, `track_id`, `genre_id`, entity pages) are `404 not_found`, not empty results.
- **Ties** in every ranking and list break by entity ID (contexts by URI, devices by name and type), in the live backend and the mock alike, so the two can be compared exactly.
- **Artwork** comes from catalog URLs, or `null` while an entity is unhydrated or provisional.
- **System status:** `derive.progress` (0–1) is reported while rebuilding.
- **Docs:** `/api/docs` serves Redoc (pinned with SRI) over the embedded spec, also at `/api/docs/openapi.yaml`.
- **Verification:** a differential test runs every UI query against both the live backend (raw rows written from refdata and derived from scratch) and the mock oracle: 1,127 requests with 0 differences. A second run feeds the same rows in batches with a restart in between: 0 differences, and the incremental tail equals a full rebuild.
### D27. Fallback metadata sources: oEmbed, Deezer, MusicBrainz (supersedes D9's "Spotify-only", resolves DF5)

The daily lookup quota (D24) makes Spotify-only hydration of a lifetime export take months. Decided by the owner on 2026-10-17:

- **Spotify stays primary and canonical.** Spotify IDs are the only IDs in `tracks`, `albums`, `artists`. A later Spotify write (lookup, poller, recently-played) of an entity replaces every field a fallback wrote for it.
- **Free public sources, no API keys, fill the gaps,** as lower-priority stages of the hydrate worker (`internal/metadata` clients, `internal/hydrate/fallback.go`):
  1. **Spotify oEmbed** (`open.spotify.com/oembed`, no auth): exact by URI. Gives the track title and the album cover (track URIs) or the artist image (artist URIs). It sends no credentials, so it cannot count against the developer-account quota. Thumbnails are 300 px (album) and 320 px (artist) `i.scdn.co` images whose size is encoded in the hash prefix; swapping it gives the 640 px and 64 px album (160 px artist) images Spotify's API returns.
  2. **Deezer public API** (`api.deezer.com`, ~50 requests per 5 s): name search for the track (free text artist plus `track:"…"`, then `track:"…" album:"…"`; Deezer's `artist:` filter returns nothing as of Oct 2026), scored on normalized title, artist, album and duration. Gives duration, ISRC, explicit, the artist picture, and, through `/album/{id}`, release date, record type and track count.
  3. **MusicBrainz** (1 request/s, User-Agent naming the project): artist genres, via the Deezer ISRC (`/isrc/{isrc}` → artist MBID) or else an exact-name artist search, then `/artist/{mbid}?inc=genres`.
- **Matching:** by URI where possible (oEmbed), else by name. The export's own `ms_played` gives the duration: streams ending in `trackdone` played the whole track, so their median is the expected duration (±2 s); otherwise the longest stream is a lower bound. Low-confidence candidates are rejected (no match is better than a wrong match).
- **Weak durations (decided by the owner on 2026-10-18):** the longest stream is no proof of a track's length (seeking back, a replay counted as one stream), and the lower bound was rejecting right matches in the long tail. For a track with no export stream ending in `trackdone`, the duration is a weak signal: it never rejects a candidate on its own, but a candidate the lower bound contradicts is accepted only on an exact normalized title and artist match (the duration then scores as unknown). Tracks with a completed stream keep the ±2.5/6/15 s rule. The matcher has a version (`metadata.MatcherVersion`, now 2, stored in `settings` as `fallback_deezer_matcher_version`); when it is newer than the stored one, the worker drops Deezer's `no_match` track states on start, so those tracks are re-tried once, and matches, failed entries and artist and album states are left alone (no re-fetching). Measured on 2026-10-18 (a large real export, Deezer only, top 500 plus 3,500 random tracks): the old matcher left 369 of 4,000 unmatched (137 without a completed stream); the new one matches 64 of them (17%, all without a completed stream), 1,107 requests in 3 min. Most (27 of the 30 spot-checked) are exact-name matches whose Deezer release differs from the export's album (a single found on its album or a compilation): album 0 and an unknown duration sum to exactly the 0.8 threshold, which floating point had put a hair under; the rest were rejected by a longest stream that outlasted the track. A manual check of 30 random new matches found 30 the same song by the same artist. Projected for the whole export: about 580 of the ~3,400 tracks Deezer left unmatched.
- **Borrowing for real artists (decided by the owner on 2026-10-18):** a real Spotify artist that Spotify has not hydrated (credited on a hydrated or live track) has no fallback data of its own except oEmbed's image, while its provisional twin (the export name) has Deezer's image and MusicBrainz genres. It borrows them only when provably the same artist: exactly one real artist (hydrated or not) has that normalized name, exactly one provisional artist has it (by construction), and they share a track (one credited to the real artist in `track_artists` whose export rows name the provisional artist). Two bands of one name are two real artists, so neither borrows. Borrowing fills gaps only (an artist's own image or genres stay); each borrowed field gets a `borrowed` provenance row with the lender's confidence (migration 0008 rebuilds `catalog_provenance` to allow it), so a higher-confidence source and any Spotify write replace it, and an artist that stops qualifying (a second real artist of the name appears) has its borrowed fields withdrawn. The pass (`Hydrator.Borrow`) runs at most every 2 minutes while fallback stages work, and when they go idle.
- **Identities (compatible with D20 and the M5 resolver):** fallback data for a track is written to its `tracks` row (Spotify ID from the export URI; `hydrated_at` stays null, `source = 'fallback'`, so the track stays queued for Spotify). Artists and albums known only by export names are keyed by derivation's provisional IDs (`ar_`/`al_` + base64url of the normalized name), in `fallback_artists` and `fallback_albums`, never in the Spotify tables. Fallback genres go into `artist_genres` under the provisional artist ID. Real Spotify artists still queued (from live or hydrated tracks) get their oEmbed image on their `artists` row. No Spotify IDs are invented. Derivation (D25) reads them in its last resolution step: a provisional artist takes its fallback image and genres, a provisional album its cover, type, release date and track count, and a provisional track its fallback duration (so the play rule, D7, stops using its unknown-duration fallback). A provisional entity that adopts a real one keeps the real one's data. This refines D26's "artwork is null while provisional": provisional entities show fallback artwork when a source found it. The derivation fingerprint covers the fallback tables.
- **Provenance:** `catalog_provenance` holds source and confidence per entity and field written by a fallback (no row: Spotify wrote it). `fallback_state` holds per-entity, per-source outcomes (matched, no match, failed with backoff), the external ID and the confidence (migration 0007).
- **Order and budgets:** each source has its own limiter, independent of the Spotify limiter and its live reserve. The fallback stages run while the Spotify worker is not making progress (quota spent, not connected, backing off), in queue priority order, so Spotify keeps the top priority whenever it has quota. Each can be turned off (`METADATA_OEMBED`, `METADATA_DEEZER`, `METADATA_MUSICBRAINZ`). `HYDRATE_SPOTIFY_LOOKUPS=false` (decided 2026-10-18, so production can run the fallbacks without touching the quota while it is measured) turns the Spotify side off instead: the worker gets no Spotify client, so it makes no catalog calls at all (tracks, artists, albums, playlists and other contexts); seeding from live capture and the fallbacks still run, Spotify queue entries stay queued without being charged, and `/system/status` reports catalog `state: disabled` with no ETA. Fallbacks need `HYDRATE_ENABLED=true`.
- **Genre mapping:** MusicBrainz's curated genres only (not free-form tags), up to 5 per artist by vote count, keeping those with at least a third of the top genre's votes, with names as MusicBrainz spells them (lower case, e.g. `dance-pop`, `hip hop`). Spotify's genres use the same lower-case style and mostly the same names for broad genres, so the genre pages' slugs (`GenreID`) merge them; Spotify's micro-genres (`uk hip hop`) have no MusicBrainz counterpart and are not mapped. Deezer's own album genres are too coarse ("Pop") and are not used.
- **Why:** Exact sources first (oEmbed), then the most complete free catalog for durations (Deezer: the play rule D7 needs durations), then the best genre source. Keeping fallback entities out of the Spotify tables keeps M4/M5 semantics intact.
- **Measured (2026-10-18, a large real export, top 1,000 tracks by plays plus 1,000 random from the rest; no Spotify calls):**
  - Match rates (top / tail): oEmbed 100% / 100%; Deezer tracks 97.5% / 89.8% (albums 91% matched; artists 99% matched); MusicBrainz artists 94.2% / 88.6% of tracks covered, but MusicBrainz has curated genres for only about two thirds of matched artists.
  - Confidence of Deezer track matches: 66% at 1.0, 7% at 0.95, 23% at 0.90, 4% at 0.80–0.89. A manual check of 30 random Deezer matches found 30 the same recording.
  - Throughput at the default rates: oEmbed 3.97 tracks/s (1 request each); Deezer 2.05 tracks/s (2.9 requests each at 6/s); MusicBrainz 0.97 requests/s, 2.2 per artist. The 2,000 tracks took 43 min, MusicBrainz-bound.
  - Projected for the whole catalog (~34,000 tracks, ~10,000 artists), the stages running side by side: oEmbed ~2.4 h, Deezer ~4.6 h, MusicBrainz ~6.5 h, so about 6.5 h overall, against months for Spotify alone at ~150 lookups a day.
- **Rejected:** Scraping the Spotify embed page (`open.spotify.com/embed/track/…`; the owner decided on 2026-10-18 never to fetch it). It carries artist IDs, duration and release date, but it is an undocumented page, not an API: scraping it is a terms-of-service risk, it can change without notice, and it could get the server's IP blocked by Spotify, which live capture depends on. Last.fm (needs an API key). Writing fallback artists into `artists` under made-up IDs.

### D28. First release is 0.1 beta, with the MCP server (revises D16's timing)

Decided by the owner on 2026-10-18:

- **Version:** the first tagged release is `v0.1.0-beta.1`, not 1.0. "v1" in D1–D27 means this first release.
- **Scope:** M7 (dashboard integration), M8 (hardening and release) and the MCP server, now **M9** (was v1.1). The tag is cut once all three are in.
- **API stability:** the path stays `/api/v1`. During 0.x a breaking change is allowed but must be listed in the release notes; D16's promise (breaking changes go to `/api/v2`) starts at 1.0.
- **MCP** as specified in D16: Streamable HTTP at `/mcp` in the same binary, API-token auth (D13), `spotussi mcp --url --token` as a stdio proxy, read-only tools that wrap the query layer so answers match the UI.
- **Backup interface** (pinned here so the backend and UI work agree): `spotussi backup <file>` writes a consistent SQLite snapshot while the server runs; `GET /api/v1/system/backup` streams the same snapshot as a download. The download needs a signed-in session, not an API token, because the file holds the password hash, token hashes and the encrypted Spotify refresh token. `/data/key` is not in the snapshot; restoring on another machine needs that file too, or a Spotify reconnect.
- **Why:** the owner wants Claude to query the data from the first release, and "beta" says the data model and API can still change.
- **Rejected:** shipping 1.0 first and MCP in 1.1 (the previous plan).

### D29. MCP server details (M9)

Settled while building M9 (2026-10-18), refining D16:

- **SDK:** the official Go SDK, `github.com/modelcontextprotocol/go-sdk` v1.8.0 (one dependency plus `google/jsonschema-go`, which infers the input schemas from the argument structs).
- **Transport:** Streamable HTTP at `/mcp`, **stateless** (no `Mcp-Session-Id`, every POST self-contained) with **JSON responses** (no SSE): the tools are read-only request/response calls, so there is nothing to stream or keep per session, and any replica or restart can answer. The SDK's DNS-rebinding guard (403 for a loopback connection with a non-localhost `Host`) is off: every deployment sits behind a reverse proxy with some host name, and a rebinding page cannot supply the bearer token. Request bodies are capped at 1 MiB.
- **Auth:** API tokens only (`Authorization: Bearer`), the same tokens `/api/v1` accepts. Session cookies and proxy headers get `401 token_required`; no credential gets `401 unauthorized`; a bad or revoked token `401 invalid_token`. `AUTH_MODE=none` is open, as the API is. The OAuth discovery paths MCP clients probe after a 401 (`/.well-known/oauth-protected-resource`, `oauth-authorization-server`, `openid-configuration`) answer a JSON 404 instead of the SPA's `index.html`.
- **Tools** call the `api.Backend` the HTTP handlers use (live or mock), so results match the UI. D16's six tools plus `get_status` (timezone, play rule, data extent, background work; needed to read dates and know whether numbers are final) and `get_timeline` (per-bucket totals; "how much did I listen each month" is a common question the six could not answer). `get_entity` adds the related top 5 lists the entity pages show; `get_sessions` takes `session_id` for one session's plays. Defaults follow the API (`last_4w`, history `all`).
- **Results:** compact JSON as text content (no structured content or output schemas, which would double every result in a model's context): names and IDs, no artwork URLs; minutes for aggregates, seconds for single plays; times as RFC 3339 in the display timezone. Caps: top 50, history 100, sessions 50, session plays 100, timeline 400 points (an error asks for a coarser bucket rather than truncating), and 96 KB per result.
- **Errors** are tool errors (`isError`) with an actionable message: not found (with where IDs come from), invalid arguments (from schema validation or the API's own validation), timeouts (30 s per call). Unexpected errors are logged and reported as "internal error" without details.
- **stdio proxy:** `spotussi mcp --url <base> [--token]`, token from `SPOTUSSI_TOKEN` by default. A plain JSON-RPC forwarder rather than an SDK client/server pair, so it is transparent to protocol versions and capabilities: each stdin line is POSTed (up to 8 at once; `initialize` alone, and its negotiated version is sent as `MCP-Protocol-Version` afterwards), JSON or SSE responses are written back one message per line, and HTTP or network failures become JSON-RPC errors for the request that caused them (e.g. "invalid or revoked API token").
- **Rejected:** stateful sessions (nothing to keep); SSE responses (no progress or server-initiated messages to send); an OAuth server for remote connectors (out of scope for 0.1; stdio covers Claude Desktop); accepting browser sessions at `/mcp` (browser credentials belong to the CSRF-protected UI; an MCP client should be its own revocable token, D13).

### D30. Backups, restore, release pipeline and CI performance gate (settled building M8)

- **Snapshots** use SQLite's `VACUUM INTO` on a connection of their own (D15 named the online backup API; `VACUUM INTO` gives the same consistency, since it reads inside one read transaction while WAL writers continue, and modernc runs it without the driver-specific backup handle). The copy is compacted, written to a 0600 temp file next to the destination, checked with `PRAGMA integrity_check`, fsynced and hard-linked into place, so a destination only ever appears complete and is never overwritten. `spotussi backup <file | dir/>` and `GET /api/v1/system/backup` share it; the download writes the snapshot next to the database, streams it (`application/vnd.sqlite3`, `spotussi-YYYYMMDD-HHMMSS.db`, Range support, 2 h write deadline), one at a time (`409 backup_running`), and API tokens get `403 forbidden` in every auth mode (D28).
- **Restore is a subcommand,** `spotussi restore [--key KEY] <backup.db>`, because copying a file over `spotussi.db` leaves the old `-wal`/`-shm`, which SQLite would replay onto the restored file, and the distroless image has no shell to delete them. It verifies the backup, keeps the current database (checkpointed) as `spotussi.db.before-restore-<ms>`, removes stale WAL files and renames the copy in; `--key` installs the backup's instance key, keeping the old one. Every `db.Open` takes a shared `flock` on `spotussi.db.lock` and restore an exclusive one, so a restore refuses while the server (or a CLI import) has the database open, also from another container on the same volume. Run as root (to read backup files a host user owns), it hands the files back to `DATA_DIR`'s owner.
- **Untrusted-proxy warning.** Found in the fresh-install walkthrough: behind an HTTPS proxy missing from `TRUSTED_PROXIES`, the wizard silently offered the plain-HTTP loopback redirect URI with the container's port. The server now logs once, naming the peer and suggesting `TRUSTED_PROXIES=<peer>`. Trusting private networks by default was rejected: in proxy auth mode a trusted peer can assert the user.
- **Release pipeline** (`.github/workflows/release.yml`, tag `v*`): check (tag format, `docs/project/release-notes/<tag>.md` exists, vet, test) → multi-arch image to `ghcr.io/<owner>/<repo>:<version>` (`:latest` only for tags without a pre-release suffix, a moving `:beta` for `-beta.N` tags (D33); the pushed image must report the tag) → static `CGO_ENABLED=0` archives for linux/darwin × amd64/arm64 with the UI embedded and `SHA256SUMS` (`scripts/release-binaries.sh`, runnable locally) → GitHub Release with the notes file, `--prerelease` when the tag has a `-` suffix. SLSA build provenance (`actions/attest@v4`, what `attest-build-provenance@v4` wraps) for the image digest (pushed to GHCR too) and for every archive via `SHA256SUMS`; skipped while the repository is private, since GitHub only attests private repositories on Enterprise Cloud. Only `docker login`, `gh` and `actions/attest` are added beyond the actions CI already pins. Rejected: GoReleaser (another tool and config for four `go build` calls).
- **CI performance gate:** a `perf` job runs `BenchmarkLive` on the full reference dataset and fails past 2.5× the plan budgets (rebuild 25 s, dashboard p95 62.5 ms, History p95 125 ms). An endpoint over its limit is measured once more before failing. 2.5× because shared runners have fewer, slower cores than the reference machine and noisy neighbours; tighter limits would flake, looser ones would miss a doubling of the slowest endpoint.
- **Measured (2026-10-18, M1 Max, heavily loaded machine):** three runs, load average 20–70: full rebuild 5.6–7.9 s; worst dashboard p95 9.0–16.3 ms; worst History p95 2.1–4.5 ms, all within the 1× budgets. Image 8.1 MB compressed, 29.2 MiB unpacked (budget < 30 MB); idle container 29 MiB. Fresh install from `docker compose up` to the wizard's Spotify-app step: 20 s of commands and forms; with reading the README and creating the Spotify app, an estimated 8–9 minutes (target under 10).

### D31. Marketing site at spotussi.app: a static page in `site/`

- **What:** one static page (`site/index.html`, `styles.css`, `main.js`, `assets/`), no build step, deployed to Cloudflare at `https://spotussi.app/` (D35). Preview locally with the `spotussi-site` launch configuration (Portless: `https://spotussi-site.localhost`).
- **Direction:** dressed as a hi-fi component catalogue, built on the app's own "level meter" brand (graphite, aluminium, one amber lamp colour, Archivo with its width axis). Sections: a hero that puts the app first: a 21 s Remotion loop of someone using the app, 3.5 s a page, in a flat app window that never moves (on phones, the phone layout at full width). A cursor (on phones, a fingertip on the tab bar) clicks the app's own nav from the Overview to Top artists, opens the artist who tops it by their row, then History, Sessions and Stats, and clicks Overview to loop. Each page swaps in on the click, its body zooms and rises into place under the app's header, and it arrives dark and powers on (figures count, meters, rank bars, dot trains and chart columns light); on phones a thumb scrolls each page once, down to what its caption is about, while the range bar sticks to the top as it does in the app. The pages show real artists, albums and tracks with their real lengths (see Before launch). One plain caption per page and the stop dots follow the video's clock, with a pause button (owner, 2026-10-18: show more pages than the Overview. A first cut with camera zooms, pans, slides and a match cut was rejected the same day as nauseating and without context, so was a proposal of in-window slides and slow scrolls, and so was a cut with no motion but the lamps, which also showed the mock's invented names; the owner chose this one, where every movement has a visible cause, then asked for quicker clicks (4.5 s a page read as slow; you could scroll past before the next one) and for the page contents to move after each click; it replaced a 14.5 s loop of the Overview alone). The hero fits one screen at 16:9 and 16:10: the headline, "Spotify Stats, Self-Hosted." (owner, 2026-10-18; it replaced "Your listening, self-hosted."; title case to match the feature tiles, owner 2026-10-18), in the app's own wide headline cut, sits on the left with the supporting line, and one-word actions (Install, Compare); the video sits on the right in the window's own 3:2 shape, sized by the screen's height so the caption bar stays on screen. Phones and portrait screens stack it, the app first. It replaced headline-under-video, centred-headline-over-video and offset layouts (none fitted a 16:9 screen), a slogan-above-video layout ("Every play, metered."), a Remotion cut whose close-ups zoomed into fragments, a CSS camera reel and two earlier heroes, all rejected by the owner), a features section ("Every Play Since Day One", owner 2026-10-18: the first screen after the hero says what the app does, as a bento of eight tiles, each a heading, one line and a small animated sketch drawn in the app's own style (lamp dots, graphite rows, amber accent) with the switch's real sample artists: an uncapped top list scrolling past rank 1,500 with rank moves, the listening clock lighting round to its busiest hour, an export zip dropping in and filling fifteen years of columns before the live one, two player lanes (one played to the end, one skipped at 0:29), a marquee of daily top-artist shares, sessions filling with their lengths counting up, an artist page counting its rank and hours, and an assistant asking the MCP server who you played most; each sketch replays while it is on screen and holds its final frame for reduced motion (owner 2026-10-18: crops of the real screens made no sense at tile size, and a text-only version read as a wall of copy); the tiles, in order (headings and tile titles across the page are title case, owner 2026-10-18; the switch title's swapping word is capitalised in CSS, so it reads "Rank by Plays." and "Rank by Time."): Top Lists (every track, artist, album and genre you have played, ranked for any range, with rank movement), Listening Clock (when in the day you listen and your busiest hour), Lifetime History (importing Spotify's lifetime streaming history), Live Capture (your Spotify streams read every 12 seconds, so it knows what you finished and what you skipped; two lines on desktop), Stats Over Time, Sessions (your longest listening runs, said lightly), "Artists, Albums, Tracks" (open any one for your rank, first play and when you play it) and API and MCP (owner 2026-10-18: titles and lines rewritten to match their sketches, which stayed; "without a cap", "Overview", "split at 15 minutes of silence" and "read-only" were cut; the Sessions rows are tightened so the sketch sits as far below its line as the other tiles' do); it replaced a "Built for your own server" section the owner rejected as vague), a cost comparison titled "Free Is Better.", a working Plays/Time switch that re-ranks a demo top 5 (it explains why the rankings differ, with a line defining each mode; the section pins for a screen and a half and scrolling through it flips plays to time, so it reads as a control without a click, and the full-width buttons scroll to the matching half), "From Stream to Dashboard" (step titles, owner 2026-10-19: Stream on Spotify, Each Stream Is Ingested, All Your Data, One Dashboard, Ask Your Assistant; no config, endpoint or URL labels under the steps; the phone in step one shows the real album covers from the reel's catalog, in `site/assets/covers/`; owner 2026-10-18: the History panel with margin notes, "Your history, as you listen", proved nothing and its graphic did not match its text; a first rework, "How a play gets counted", drew the internals (poll log, 30 s rule, export merge) and the owner found its steps illustrated nothing useful; a second, "From your player to your dashboard", followed the user's flow, and the owner asked for the shorter title, a player mock on the first step, ingestion on the second and a chat on the fourth; it follows one morning's three tracks from the player to the dashboard and an assistant; four steps scroll past a pinned stage, which sits beside them on wide screens and along the top on phones; the step at the reading line lights its lamp and plays its scene once, holding the last frame; the scenes: Stream (a phone's now-playing screen drawn in the streaming app's manner without its logo or name, playing from the "Late trains" playlist: Drake's Passionfruit plays out, Arctic Monkeys' Arabella is skipped at 0:29 with a press of next, Metallica's Nothing Else Matters starts; beside it, spotussi reads the player every 12 s; D2), Ingest (the player reads arriving every 12 s, the read that finds a new track, and the stored play it produces with the API's field names, `ms_played` 29000, `skipped` true, `counts_as_play` false, device, context and `sources` poller; under it the export zip lands in the same table, matched so nothing counts twice; D7, D8, D23, D25, `api/openapi.yaml`), Dashboard (plays, time and skips count up, the 11 AM column gains its lamps and Metallica climbs a place in today's top artists; the skip adds time but no play) and Ask (a chat: the user asks for their last skip and their most-played artist today, the assistant shows the MCP tool it used, `get_history` then `get_top`, with typing dots before each answer; `docs/mcp.md`); reduced motion and no script show every scene finished), three figures under "Faster than my cat at 3 AM" (owner 2026-10-19; was "Quick to answer, light to run"), with a small running cat beside it in the light-ground lamp colour (a filled silhouette, two leg frames and a bob; still for reduced motion) in their original staggered layout, each with one quiet sketch of its claim (owner 2026-10-18: the earlier graphics did not relate to the figures, and a busier second attempt with paragraph-led tiles was rejected; the short figure titles stay; "12 s" was briefly "Near real-time" and went back, its sentence now saying listening is captured every 12 s while music plays and every 90 s when idle): "< 25 ms" switches a range control between 7 days, 4 weeks and All time and its lamp chart redraws at once, with no loading; "12 s" lights a ring of 12 lamps for the 12 s read (four times faster), and when the player moves to a new track, the "In spotussi" line catches up on the next read; "29 MiB" draws a small arm64 board in outline with its status lamp ticking over at idle. Each moves only while on screen and holds its last frame for reduced motion and without script (the query columns, 128-lamp memory grid, lamp-bar race and meter footage were dropped), "The Nitty-Gritty" (a block diagram of the binary; see Audience and copy), an "Installation" section (owner 2026-10-18: a five-stage accordion, Run, Claim, Connect, Import, Listen, opening one stage at a time; each stage says in a line or two what happens and links its guide in `docs/`, so the site keeps no install guide of its own; each runs an illustrative sketch of the step drawn in HTML and CSS like the bento: a short compose file and `docker compose up -d` bringing the container up, the claim card with the setup token and password typed in, the wizard's tuner needle moving through the Spotify app, Client ID and Connect steps to a read-only connection, the export zip dropping in with the meter and the years filling, and a play by Metallica landing at the top of today, each track with its artist and album; a sketch plays once, the first time its stage is open on screen, and then holds its last frame, also after the stage closes (owner 2026-10-19: replaying loops and sketches that reset on closing read as glitches); side by side the row keeps the height of the tallest stage, so switching stages never shifts the page; reduced motion shows the last frames from the start; the open stage sits in the flow so its text can never be cut off; it replaced the full compose listing and screenshots of the setup wizard and the overview, which the owner found long to maintain and ugly at card size, with text clipped below the card), an "FAQ", and a one-word Install band that links to the GitHub repository.
- **Hero video:** made with Remotion in `motion/` (its own npm project, not part of the Go or web builds). `motion/scripts/capture.mjs` captures six pages of the mock (Overview, Top artists, the page of the artist who tops it, History, Sessions, Stats) in the app's desktop (1440px, 2.5x) and phone (390px, 3x) layouts, each twice, lit and with every lamp switched off. It records each lamp's page geometry (dots, rank bars, chart columns from the SVG path), the summary figures' type and, on the phone layout, the fixed tab bar as its own layer. The composition lays pieces of the lit capture over the dark one so every lamp that lights is the app's own pixels, and sets the Overview's and the artist's figures in Archivo to count them. While it captures, the browser's API and artwork responses are rewritten (`motion/scripts/realdata.mjs`) so the mock's invented artists, albums, tracks and genres become real ones, with the artists' real photos and album covers. `motion/scripts/fetch-catalog.mjs` builds that catalog (`motion/scripts/catalog.json`) from Deezer's public API: each artist's own top tracks with their real lengths, albums and covers, edition tags such as "(Remastered)" removed and explicit titles left out; the artwork is cached in `motion/.cache` and not committed. The mock's top artists over the default range take the catalog in rank order, led by Drake, Kendrick Lamar, Travis Scott, Fred again.., Arctic Monkeys and Metallica (owner, 2026-10-19: Drake on top, a catalogue leaning to hip hop, rock and electronic, and real artwork everywhere). Other artists borrow one of the top twenty, tracks take their real lengths and plays keep their share of the track. The capture also records the nav links, the top list's first row and, on phones, the sticky range bar. The tour (`motion/src/Tour.tsx`) gives each page its start, its lamp times, what is clicked to leave it and, on phones, how far it scrolls; `motion/src/Pointer.tsx` draws the cursor (an eased glide with a slight arc, a pause, a press and a ring) and the fingertip (a drag that moves with the page, then a tap). The camera never moves on either cut: the window stays one size and in place, because zooms and pans (and, earlier, a zoom from window to page) read as the frame moving and lost the context of where you are (owner, 2026-10-18); only the pointer and the page's own scroll move, as they do when you use the app. The wide cut is 1800x1200, 3:2 like the window, which fills the frame, and does not scroll. The phone cut (1080x1350) holds the phone layout at full width with the tab bar pinned to the bottom of the frame and its lit tab following the page; `<source media>` picks it for phones and portrait screens. Each is an H.264 MP4 (VP9 WebM decoded the dark ground darker than the page, so it was dropped) with a poster of the Overview lit, before the pointer leaves it, which reduced motion holds on; the hero shows it uncropped (`object-fit: contain` on a matching ground). Recapture and re-render when one of those pages changes.
- **Width:** content stops growing at 100rem (1600 px) and stays centred; section grounds still run full width. One token does it: `--margin: max(clamp(1rem, 2.2vw, 2rem), (100vw - 100rem) / 2)`, so nothing changes below ~1660 px (owner, 2026-10-18: on 2560 px screens the bento meters and figure cards stretched into empty bands).
- **Pictures:** the link preview is the brand kit's Open Graph card (`brand/social/og-1200x630.png`, copied to `site/og.png`; PNG, 1200×630, with width, height, type and alt declared and a `summary_large_image` Twitter/X card), replacing a WebP crop of the listening clock that link previews (iMessage among them) showed poorly (owner 2026-10-18); the favicon set and Apple touch icon also come from `brand/web/`. The feature bento and the Installation stages draw their sketches in HTML and CSS, so they have no pictures to retake. The site uses no third-party pictures or footage; any it takes on is credited in the footer and listed in `site/assets/credits.json`.
- **Facts on the page** come from the user docs (`README.md`, `docs/`) and D24–D30 (12 s capture, 25 ms dashboard budget measured 9–16 ms on 500k plays, 29 MiB idle container, 8.1 MB image, ~17 s import of a 300k-row export, ~150 lookups a day, 6–7 h fallback fill). Update the page when those change.
- **Navigation:** Features, How (the nitty-gritty), Setup, FAQ, a GitHub button (icon only on phones) and Install. The closing Install band is a plain link (its hover only tints the word), and the footer sits in normal flow below it; the earlier sticky footer revealed under the page read as a bug.
- **Before launch:** the site links `github.com/jasoncronje/spotussi` (compose image, nav button, guides, footer, Install band), which is private until launch (D33), so those links fail for visitors until then; the decisions link points at `docs/project/decisions.md`, so keep that path (or update the link) in the public cut. The footer is three short columns (release, guides, credits) and the legal line; the page-link list and the full-width wordmark were dropped to shorten it. The Plays/Time switch, the feature bento and "From Stream to Dashboard" use real artists and tracks as sample data, at the owner's request (2026-10-18), so the point lands on names people know. The sample music matches the hero reel's catalog (`motion/scripts/catalog.json`: real titles and lengths, explicit titles left out), led by Drake, Kendrick Lamar, Travis Scott, Fred again.., Arctic Monkeys and Metallica (owner, 2026-10-19: Drake on top, leaning to hip hop, rock and electronic; it replaced Olivia Rodrigo, Taylor Swift, Billie Eilish, The Beatles and Pink Floyd). The flow section follows Drake's Passionfruit (4:59, 11:26:40 to 11:31:39, played to the end), Arctic Monkeys' Arabella (3:27, skipped at 0:29) and Metallica's Nothing Else Matters (6:28, starting at 11:32:08), all from a "Late trains" playlist on an iPhone, with start times that do not overlap; the 12 s figure and the Listen install stage use the same three tracks; its day totals, plays by hour and top-artist counts are invented. The switch ranks Drake (212), Kendrick Lamar (196), Travis Scott (141), Fred again.. (120) and Arctic Monkeys (104) by plays over the last 4 weeks; by time Kendrick Lamar takes first and Metallica (sixth by plays with 92, tracks averaging 6.6 min against 3.4 to 4.2 for the others) climbs to third. Time is set at about 92% of plays times each artist's average catalog track length, for skips and partial plays. The bento's top list, artist page (Metallica), stats key and assistant answer use the same numbers. The figures are invented and labelled sample data; no artist endorsement is implied.
- **Audience and copy (2026-10-18):** written for technical self-hosters who care about privacy or cost and would otherwise pay for a hosted stats service. Competitors are named only as a category ("hosted stats services") in running copy. The one exception, at the owner's request (2026-10-18), is the cost comparison, titled "Free Is Better." (owner 2026-10-18; it was "What five years of stats cost", and the five-year totals were dropped as noise): its two-line lede says hosted apps charge a subscription and keep your plays on their servers, while spotussi runs on your own machine, keeps every play in a file you own, and costs nothing. The three cards compare row for row (owner 2026-10-18): where plays are stored, history import, and top-list depth; spotussi reads "$0, free forever" with plays on your own server, the full Spotify history imported free and every top list ranked in full for any range (the Spotify Premium requirement is answered in the FAQ, not on the card). It names Last.fm Pro ($49.99 a year, shown without the $4.99 monthly option; from last.fm/pro) and stats.fm Plus ($6 for lifetime Plus, per the owner 2026-10-18; stats.fm/plus publishes no price, so it replaced the earlier $20.99 top App Store price; its free tier shows a top 50 and no import, from stats.fm/plus). It dates the prices, links its sources, carries a trademark and non-affiliation line, and states spotussi's own cost honestly, including the Spotify Premium requirement. Re-check the prices before launch and whenever the page is touched. The stack section ("The Nitty-Gritty", for people who read the Dockerfile first) was cut back at the owner's request (2026-10-18): the "Over Rust / Postgres / React" lines and the proof figures (2.5x gate, 1,127 requests, 34 decisions) read as try-hard and proved nothing to a visitor, so they are gone. It is now drawn as a block diagram of the binary, like the signal path in a service manual (2026-10-18, to read less like a wall of text): a chassis plate "One Go Binary" (one process, one port, pure-Go SQLite so no cgo, amd64 and arm64, 8.1 MB image) holding five units joined by lamp wires that light left to right as it scrolls in: Workers (capture, import and catalog lookups as goroutines), One SQLite File (WAL, live backups, open it in sqlite3), In-Memory Index (columnar, ~35 MB for a lifetime, swapped in whole), Documented API (OpenAPI at /api/docs, revocable tokens, no private endpoints; "read-only" dropped at the owner's request, 2026-10-18, as on the bento tile, the Ask step and the FAQ) and UI and MCP (Svelte 5 compiled in, MCP on the same API). Each unit has a small CSS sketch, a short title and a few short fragments in the owner's style ("One process, one port."), no long sentences, from D4, D5, D6, D16, D25 and D29. The path runs down the page under 1100px and down a left-hand rail on phones. Live capture intervals and the play rule are product behaviour, covered by the features bento and the History section, so the owner kept them out of this section. Leave out anything an engineer would not care about; do not argue against alternatives on the page. "No subscription" is stated as fact (there is no paid tier); the page makes no open-source claim until the repository is public (licence: AGPL-3.0, D33).
- **No third-party claims:** the footer states that spotussi is independent and not affiliated with Spotify.

### D32. Dashboard integration: provenance fields, instance banners, mock scenarios (settled building M7)

- **Provenance in the API.** Artist, album and track details carry `metadata {state, sources}`: `spotify` (looked up, or a length from Spotify's live track object, including poller-seeded catalog rows), `fallback` (filled by D27's sources; `sources` lists which, from `catalog_provenance`) or `pending` (names only). `OverviewSummary` adds `sources` (export only / live only / both, from the play's source flags) and `catalog` (the range's listening by its tracks' metadata state), each a Measure. Additive, so no API version change (D28).
- **How the UI shows it.** Source detail is opt-in: "Data sources" in Settings → General (per browser, like the theme; off by default) shows Overview's two dot-row meters under the summary ("Plays from", "Track details"), History's per-play source lamps and their legend, and the "Provisional details" badge on entity pages. Most people don't need where each play came from; the owner reviewing M7 asked for it behind a switch. Always shown: the "Names only" badge (lookups pending), because the page visibly lacks artwork, a length and genres and the badge says that's "not yet", not broken. Spotify is the default and gets no badge. Missing artwork is an unlit dot grille with initials. Fallback sources are described by what they are, with the same words everywhere ("Spotify embeds", "a public music catalog", "an open music database"), never by brand, in line with the no-third-party-trademarks rule for product copy.
- **Album artists (D14).** An album page names its credited artists in the header. When three or more artists appear on its tracks (features included: compilations, soundtracks), it also ranks them in "Artists on this album" after the tracks; with fewer, the list would repeat the header.
- **Instance state lives in the shell.** One polled `/system/status` (60 s and on tab focus) drives banners above the header on every page: Spotify needs reconnecting (not dismissible; links to a focused reconnect at `/welcome?step=connect&reconnect=1&next=…` that returns to the page you were on), Spotify not connected while history exists, and a hideable "track details N% looked up" strip on data pages while Spotify coverage is under 97% and lookups are active.
- **Errors.** A request that can't reach the server (fetch failure, or a bare 502/503/504 from a gateway) marks the link down: one banner, `/health` probed with backoff (2–30 s, and on the browser's online event), everything refetched when it answers, and data already on screen stays (dimmed) instead of turning into per-section errors. Other failures stay per section and say whether the server failed (5xx) or refused. A 401 mid-session goes to sign-in with a "session ended" note and returns to the page.
- **Empty states** depend on why: no history at all (connect Spotify, import), an import running, or nothing in this range (buttons to a longer range). One component for every D14 page.
- **Mock scenarios** (`POST /_mock/scenario`, mock mode only, not in the OpenAPI spec): sparse catalog (a quarter Spotify, half fallback, a quarter names only), Spotify reconnect required, not configured or connected without `user-library-read` (`needs_scope`, D36), failing endpoints by path prefix and status, added latency. Defaults keep the mock identical to the live backend for the differential test (D26).
- **Range-switch performance rules** (M7 pass, `docs/project/screenshots/m7/perf.md`): API responses are immutable, so resources hold them in raw state; effects never write state they or their readers depend on (defer to a microtask); charts with one element per bucket draw one SVG path per series; `{#each}` over buckets is keyed by index so switching ranges updates nodes instead of recreating them.
- **Why:** production's catalog is mostly provisional for days after an import (D24 quota, D27), so "missing" has to look deliberate and explain itself; a revoked Spotify token silently stops capture, so it must be impossible to miss.
- **Rejected:** a provenance badge on every row and card (noise: the Overview mix and History's lamps cover it); source detail shown to everyone by default; naming the fallback services; per-page status polling; dismissible or calmer reconnect banner (capture is stopped while it shows); an artist list on every album page.

### D33. Licence, repository and release process (owner, 2026-10-18)

- **Licence:** AGPL-3.0-only for the app and everything else in the repository except third-party assets, which keep their own licences and are listed in `site/assets/credits.json` (today: album covers and artist photos from Deezer's public API, used as sample data in the site and the hero reel, © their owners). The network clause means anyone who runs a modified spotussi as a service must offer their changes to its users; self-hosters running it for themselves owe nothing. The owner holds the copyright, so a hosted offering of their own stays possible. No paid tier, no open-core split; contributions come in under the same licence (DCO sign-off, no CLA).
- **Source offer (§13):** Settings → System ends with one muted line linking the public repository, noting that modified versions run as a service must link their own source. It links the repository rather than the running tag, since the version lives in `/health`, not the system status that page loads.
- **Repository:** one repository (monorepo) on GitHub, private until launch. The app, `site/` and `motion/` stay together: the site's figures, screenshots and hero video are made from the app's mock and docs, so they change in the same commits. The site deploys from `site/` on its own (static host, path-filtered), not with the app's release. Large rendered media stays out of history where possible (video re-renders replace files rather than accumulating them).
- **History:** the private repository carries the full development history. At the public launch it is renamed `spotussi-archive` (kept private) and a new public `spotussi` starts from one squashed commit, after replacing private hostnames and the owner's own figures in the docs and moving the private deploy script out of the repository. gitleaks over the full history found nothing (2026-10-18); it runs again before the cut. **Done 2026-10-18:** the private repository is now `spotussi-archive`, and the public `spotussi` starts from one squashed commit (gitleaks clean on both).
- **Process:** trunk-based on `main`, one pull request per change, squash merges, Conventional Commit titles; CI (lint, tests, perf gate) required to merge. SemVer pre-releases (`v0.1.0-beta.N`); released migrations never change. Tags drive `release.yml`; GHCR tags are the exact version and a moving `beta` (`latest` only for stable releases). Dependabot, SECURITY.md and CONTRIBUTING.md before launch.
- **Rejected:** a separate site repository (two places to keep facts and screenshots in step, for a static page with no build); MIT or Apache-2.0 (a closed hosted fork could compete without giving changes back); publishing the full history (it records the owner's listening figures and private deployment details).

### D34. Brand kit in `brand/` (owner, 2026-10-18)

- **Mark:** "Ladder", the 4×4 dot-matrix level meter the app already wore (lit heights 2·4·3·1), redrawn on a 32-unit grid (lamps 5.5 units across, 7 apart). Its plated form (lamps on a graphite tile) is the app icon and favicon and works on any ground; bare forms exist for dark and light grounds, mono forms draw unlit lamps as rings. The wordmark is Archivo at 125% width, weight 760, −0.035 em, outlined so the SVGs need no font. Colours, clear space, sizes and do/don't are in `brand/README.md`.
- **Assets:** `brand/scripts/build.mjs` (Node, headless Chrome, no dependencies) builds everything from that geometry: logo SVGs, the web icons (favicon.svg; favicon.ico with a 16 px frame redrawn on whole pixels; apple-touch-icon; 192/512 and maskable PWA icons; manifest), Open Graph and GitHub social images, and light/dark README headers. The web icons are copied into `web/static` and linked from `web/src/app.html`. Wider images carry the grille on as a meter strip with synthetic heights, never real listening data.
- **Rejected:** "Peak hold" (LED bar segments with a held peak lamp; busier, mushy at 16 px) and "Matrix s" (a lowercase s lit on a 5×5 grille; texture at 16 px and a doubled s beside the wordmark). Both are in `brand/previews/mark-directions.png`.

### D35. The marketing site deploys to Cloudflare as a static-assets Worker (owner, 2026-10-18)

- **Host:** Cloudflare's free plan, as a Worker named `spotussi` that only serves static assets (no script runs), at `https://spotussi.app/` (custom domain; the `workers.dev` address is off). Picked as Cloudflare Pages; `wrangler pages project create` (4.147.0) now makes this kind of Worker instead, since Cloudflare is folding Pages into Workers, and the owner kept it rather than forcing a classic Pages project that is on its way out. Static asset requests are free; limits that matter (20,000 files, 25 MiB per file) are far above a ~4.8 MB page.
- **Config and deploy:** `wrangler.jsonc` at the repository root (assets directory `site/`, `workers_dev` off, `preview_urls` on); `site/.assetsignore` keeps the README out. `.github/workflows/site.yml`, path-filtered to `site/**` and the config (D33), runs a pinned Wrangler: pushes to `main` run `wrangler deploy`; pull requests run `wrangler versions upload --preview-alias <branch>`, a preview at `<branch>-spotussi.<subdomain>.workers.dev` linked from the job summary, without touching production. Secrets: `CLOUDFLARE_API_TOKEN` (Account → Workers Scripts → Edit, one account) and `CLOUDFLARE_ACCOUNT_ID`; runs without them (forks, Dependabot) skip the deploy.
- **Custom domain:** attached once with `wrangler deploy --domain spotussi.app` and deliberately not listed in `wrangler.jsonc`: deploys without routes leave it attached (checked: a later deploy kept it serving), so the CI token needs no zone permissions.
- **Previews are public URLs.** The page holds nothing private, but share them knowing that.
- **Rejected:** GitHub Pages (needs a paid GitHub plan while the repository is private; no pull-request previews); Vercel (the free Hobby plan is for non-commercial use); classic Cloudflare Pages via `--force` (the path Cloudflare is retiring).

### D36. Library and playlist sync (DF9) (owner, 2026-10-18)

Design for DF9, approved by the owner decision by decision on 2026-10-18 before any code. Spotify facts checked against the [Feb 2026 migration guide](https://developer.spotify.com/documentation/web-api/tutorials/february-2026-migration-guide) and the endpoint references the same day; items marked *measure* are verified on the owner's account in step 2.

**What Spotify still allows a Development Mode app (read-only)**

| Data | Endpoint | Scope | Page size | Quota bucket (D24) |
|---|---|---|---|---|
| Saved tracks, newest first, with `added_at` and a full track object | `GET /me/tracks` | `user-library-read` (new) | 50 | `user` |
| Saved albums with `added_at` and a full album object (its first 50 tracks) | `GET /me/albums` | `user-library-read` (new) | 50 | `user` |
| The user's playlists (own, collaborative, followed) with `snapshot_id` and owner | `GET /me/playlists` | `playlist-read-private` (have) | 50 | `user` |
| Items of a playlist with `added_at`, `added_by`, `is_local` and a full track object | `GET /playlists/{id}/items` | `playlist-read-private` (have) | 50 | `playlists` |

- **Followed playlists have no items.** Since Feb 2026, `/playlists/{id}/items` answers `403` unless the user owns or collaborates on the playlist, and a playlist object for anyone else's playlist carries metadata only. So followed playlists sync as metadata (name, owner, image, `snapshot_id`), not contents. Spotify-made playlists (`37i9dQZ` prefix) are skipped without a request, as in D24.
- **None of these are catalog lookups.** `/me/…` is in the `user` bucket and `/playlists/…` in its own (both verified outside the catalog quota for `/me/playlists` and `/playlists/{id}` in D24). *Measure:* `/me/tracks`, `/me/albums` and `/playlists/{id}/items` still answer `200` while the catalog quota is spent.
- **Every item carries a full track object,** so the library seeds the catalog the way live capture does (D24 seeding): no lookups, no quota.

**Scope and reconnect**

- `Scopes` gains `user-library-read` (D12 anticipated this). `spotify_connection.scopes` already stores what Spotify granted, so the server knows when it is missing.
- **Set up from scratch (owner, 2026-10-18):** the instance is treated as unreleased, so there is no reconnect strip or upgrade flow in the UI. The wizard's consent step lists the new scope. An instance connected without it (the homelab, until it reconnects once; noted in the release notes) syncs playlists only, and the Library page and Settings → Spotify say the scope is missing instead of failing (`library.lists.tracks.state: needs_scope`). A `403` on `/me/tracks` (scope revoked in Spotify) sets the same state and is not retried until the next connect.
- **Reconnect fix (2026-10-19, after beta.3):** in beta.3 Settings → Spotify → Reconnect opened the setup guide's connect step, which showed only the "Connected" panel for a connected account, so the homelab couldn't grant the new scope from the UI. Now `?reconnect=1` always offers consent again (naming any missing scope; capture keeps running), and the Connected panel with **Done** back to `next` shows only after Spotify returns (`?spotify=connected`) or the paste-back lands. The library sync panel and the Library page (empty or not) link straight to that reconnect when tracks or albums are `needs_scope`. Still no global banner: the missing scope only affects the Library page. Mock scenario `spotify=needs_scope` reproduces it.
- `/system/status` `spotify` gains `scopes` (granted).

**Data model (migration 0010): observations, kept forever (D3)**

The API shows only current state, so sync records what it saw and when:

- `library_syncs`: one row per run (`id`, kind `quick`/`full`, started/finished, `complete`, requests, error). Only a complete walk of a list can prove a removal.
- `library_spans`: one row per stretch of membership: `list` (`tracks`, `albums` or a playlist ID), `item_uri`, `added_at`, `added_by`, `first_seen_sync`, `last_seen_sync`, `gone_sync` (null while present), `json` (the verbatim item at first sight). An item removed and saved again is a new span (it has a new `added_at`). Reorders are not tracked.
- `library_playlists`: one row per playlist seen: ID, name, owner, `relation` (`own`, `collaborative`, `followed`), public, image, `snapshot_id`, item total, `items_available`, first/last seen, `gone_sync`, verbatim JSON. Names update in place and also fill `contexts` (D17), so every playlist in the library is named without a lookup.
- Adds and removals over time are derived from the spans (`added_at`, and the sync that first missed the item), not stored twice.

**Sync job (`internal/library`)**

- Background lane of the shared limiter, so live capture keeps its reserve; a `QUOTA_EXCEEDED` on a bucket pauses only that part of the sync until its reset (D24).
- **Quick check, hourly** (`LIBRARY_SYNC_INTERVAL`, default 1 h): page 1 of `/me/tracks` and `/me/albums` (50 each, newest first) and every page of `/me/playlists`. A list whose total and first page match the stored state is unchanged. Otherwise that list gets a full walk. Owned and collaborative playlists whose `snapshot_id` changed (or that are new) get their items walked; unchanged ones cost nothing. For a library of ~2,000 songs and ~50 playlists that's about 3 requests an hour.
- **Full walk, daily** (and on first sync): every page of every list, as a safety net for changes the quick check can't see (an add plus a removal deeper in the list keeps the total). First sync for 2,000 songs, 200 albums and 50 own playlists averaging 100 items: about 40 + 4 + 1 + 100 = 145 requests, about 75 s at the planning rate of 2/s.
- First run 2 minutes after start or after (re)connecting; `LIBRARY_SYNC_ENABLED=false` turns it off. `POST /library/sync` (signed-in session only, like backups) runs a quick check now, in the interactive lane.
- Restart-safe: an interrupted walk is not `complete`, so it never closes spans.
- **Status:** `/system/status` gains `library`: `state` (`disabled`, `not_connected`, `idle`, `syncing`, `backing_off`, `quota_exceeded`), `last_sync_at`, `last_full_at`, `next_sync_at`, `progress` (0–1 while walking), per-list `state` (`ok`, `needs_scope`, `error`), counts (saved tracks, albums, playlists, playlists with items), `last_error`.

**Catalog: seed, don't queue**

- Track and album objects from the library are upserted into the catalog as live capture's are (D24), so saved and playlist tracks have names, artwork and durations at once, at no quota cost. Local files are kept with `is_local = 1`.
- **Unplayed library tracks are not queued for lookups,** and their unhydrated artists are not queued either: the quota (506/day) goes to what you listen to. A library track that gets played is queued by the usual rules. The catalog-changed cue (D25) fires once per sync that seeded anything.

**Matching saves to plays**

- A saved track counts as played when a play has its ID, or its relinked ID (`tracks.linked_from`), or the same ISRC (so a song saved from one release and played from another is not "never played"). *Measure* how many saved tracks match only by ISRC on the owner's account.
- Play counts follow the play rule and exclusions (D7), as everywhere else.
- A saved track with no play by ID and no ISRC on either side yet (an export track not looked up) is reported as `maybe_played` rather than never played, so the never-played list doesn't fill with tracks that only lack metadata.

**What it unlocks: API (OpenAPI first, D6/D16)**

All read-only, answered from the library tables plus the derive snapshot's per-track totals; `range`/`from`/`to` filter by save date or play date as stated.

- `GET /library/summary`: counts (saved tracks, albums, playlists), never-played saved tracks (count and share), save → first play and save → heavy rotation (median days), the save-lag mix, last sync. **Heavy rotation is a track's 5th counted play** (owner, 2026-10-18): simple to explain and the same for export and live history. The mix buckets saved tracks by when that play came: before saving, within a day, a week, a month, later, or not yet (under 5 plays). Rejected: "N plays within 30 days" (harder to explain; a slow, steady favourite never qualifies) and a user-set threshold (a setting few would change; easy to add later).
- `GET /library/tracks?played=never|ever|all&sort=added|plays|last_played`: saved tracks with `added_at`, plays, first and last play, days to first play; paginated.
- `GET /library/albums?…`: saved albums with tracks played / total and plays.
- `GET /library/playlists`: each playlist with relation, items, tracks ever played (and the share: the play-through), plays started from it (contexts, D17); followed playlists list plays from them but no play-through.
- `GET /library/playlists/{id}`: the playlist's items in order, each with `added_at`, `added_by` (owner or a collaborator), plays, plays from this playlist.
- `GET /library/timeline?bucket=…`: saves and removals per bucket (tracks, albums, playlist items).
- Track pages (`/tracks/{id}`) gain `saved_at` (null when not saved). Additive, no version change (D28).
- `serve --mock` generates a library from the reference dataset (saves spread over the years, a share never played, a few playlists of each relation), and the differential test covers every new endpoint.

**UI (hi-fi readout, D32 patterns)**

- New **Library** page in the nav: a summary row (saved, never played, median save → heavy rotation), a saves timeline, and the save-lag mix as a dot-row meter; tabs for Songs (filter "never played" first, it's the headline), Albums and Playlists (play-through as a meter per playlist); a playlist detail page. Empty states by cause: scope missing (reconnect button), first sync running, nothing saved.
- Track pages show "Saved <date>". Settings → Spotify shows the granted scopes and the library sync status, with "Sync now".

**MCP:** one tool, `get_library(view: summary|tracks|albums|playlists|playlist, played, playlist_id, limit)`, over the same Backend methods ("which songs did I save and never play?" is the question it answers).

- **Why:** Neither Your Spotify nor stats.fm syncs the library (DF9). Every endpoint used is outside the catalog quota, and the library seeds the catalog for free, so the feature costs a few requests an hour. Spans keep the raw observations without storing a copy of the whole library every hour.
- **Rejected:** storing a full snapshot per sync (hourly copies of thousands of rows that are almost always identical); queueing unplayed library tracks for lookups (would spend days of quota on music not listened to); contents of followed playlists (Spotify refuses them since Feb 2026); an in-app reconnect strip for the missing scope (the owner treats the instance as set up from scratch).

**As built (2026-10-18).** Where the build differs from or settles the design above:

- **Play status has three values:** `played`, `maybe_played`, `never_played`; `/library/tracks?played=` takes those or `all` (not `never|ever|all`). `played` = a counted play of the ID, of a relinked ID in either direction (`tracks.linked_from`), or of the same ISRC. `maybe_played` = not played, but a played track has the same normalized title and main artist (the export-history case where the ISRC isn't known yet). The design's "no ISRC on either side" rule was replaced by this one: it is testable and doesn't depend on lookup progress.
- **The daily full walk covers saved tracks and albums.** Playlist items are walked only when the playlist's `snapshot_id` differs from the one their items were last walked at, on every run; a snapshot is trustworthy for that, so a daily re-walk would only spend requests.
- **Saved albums seed the album, not its tracks.** The album object's embedded track page is ignored: album pages list tracks by lookup as before, and the play-through counts tracks with plays against `total_tracks`.
- **`POST /library/sync`** answers `202` with the library status, `409 sync_running` while a sync runs, `503 sync_disabled` when `LIBRARY_SYNC_ENABLED=false`. The mock answers `202` and never changes.
- **Config:** `LIBRARY_SYNC_ENABLED` (default `true`), `LIBRARY_SYNC_INTERVAL` (default `1h`, 10 m–24 h).
- **UI:** the Library page opens on the never-played filter; the save-lag mix uses the shared mix meter; song rows show five rotation lamps (one per counted play up to heavy rotation) and a ring for `maybe_played`. With the scope missing, the empty state points to Settings → Spotify (setup from scratch, no reconnect button). Settings → Spotify has a Library Sync panel (state, last sync, next check, per-list counts, Sync now). Checked with axe (WCAG 2.2 AA) in phone and desktop, light and dark.
- **MCP `get_library`** also takes `sort`, `relation` and `offset`; at most 50 items per call.
- **Never-played pages open (2026-10-19).** An album, song or artist the catalog has but no play does (saved, never played) has a page with zero stats instead of "Not in your library": the detail falls back to the catalog row, and a scope on its ID matches no play, so the page's lists are empty rather than `404`. Before this, every saved album never played linked from the Library page to a `404`.

**Measured on the owner's account (2026-10-18).** A local spike server on a fresh database with the owner's export imported (capture and Spotify lookups off), connected once with the new scope; `internal/library/spike_test.go` (build tag `spike`) made the direct requests.

- **Outside the catalog quota: confirmed.** With a catalog lookup answering `429 QUOTA_EXCEEDED` (retry after ~18 h), `/me/tracks`, `/me/albums`, `/me/playlists` and `/playlists/{id}/items` all answered `200`.
- **First sync (full):** 1,287 saved songs, 45 saved albums, 56 playlists (28 own, 1 collaborative, 27 followed, items of the 29 readable ones: 1,241 items). 68 requests in 30.8 s, no errors; 2,150 tracks seeded into the catalog with ISRCs. No unplayed library track was queued for a lookup.
- **Quick check of the unchanged library:** 4 requests (the playlist list is 2 pages) in 1.2 s; no playlist items re-read.
- **Matching:** of 1,287 saved songs, 1,256 match a play by ID, 0 by relink and 13 only by ISRC (1%), 16 are `maybe_played` and 3 never played. ISRCs were known for 2,411 played tracks at the time (library seeding plus a few minutes of Deezer), so 13 is a lower bound: some `maybe_played` songs should turn `played` as hydration fills ISRCs.
- **Fixed from the run:** after connecting, the library status said `not_connected` until the first sync (the previous run had found no connection); it now reports `idle`.

### D37. ListenBrainz as a first pass before the MusicBrainz stage (owner, 2026-10-18)

Decided by the owner on 2026-10-18 (option A below; option B is open as O1). The MusicBrainz stage (D27) is the slow one, ~6.5 h for the whole catalog at 1 request/s and 2.2 requests per artist. ListenBrainz serves MusicBrainz data through keyless batch endpoints. The question was whether those can replace that stage or cut its work.

- **Endpoints (no auth, User-Agent as for MusicBrainz):** `labs.api.listenbrainz.org/acrr-lookup/json` (artist credit + recording + release name → recording, release and artist MBIDs; matching is exact after lower-casing and removing accents and punctuation), `acr-lookup` (the same without the release), `spotify-id-from-mbid` (recording MBID → the Spotify track IDs ListenBrainz maps it to), and `api.listenbrainz.org/1/metadata/recording/?recording_mbids=…&inc=artist tag release` (length, ISRCs, first release date, credited artists, release `caa_id`, and recording, release-group and artist tags; tags that are MusicBrainz genres carry a `genre_mbid`). Batch limits: metadata takes 100 MBIDs per request, while 150 gets a 502 from the gateway (a GET URL of ~5.5 KB). The two labs lookups answered 1,000-item POSTs in 0.7–2 s. The metadata rate limit (30 requests per 10 s window) was never reached. ListenBrainz's artist `rels` keep one URL per link type, so Spotify artist links are lost there (Deezer's wins "free streaming"). They come only from MusicBrainz `/ws/2/artist/{mbid}?inc=url-rels`.
- **Harness:** `internal/hydrate/lbmeasure_test.go` (build tag `measure`, no Spotify Web API calls), run on the D27 sample: the owner's export (34,115 queued tracks, 10,325 export artist names), top 1,000 tracks by plays plus 1,000 random from the rest (the same seed). Cascade over the tracks still unmatched: `acrr` with the export names, then with qualifiers stripped (`metadata.StripQualifiers`), then `acr`, then `acr` stripped. Each hit is scored with the D27 track matcher (`metadata.TrackScore`, ≥ 0.8) on the recording's title, best credited artist, release and length. A hit whose `spotify-id-from-mbid` list contains the export's own track ID counts as verified and is accepted as well. The artist MBID is the credited artist whose name matches the export artist (`NameSim` ≥ 0.9), by vote over that artist's accepted tracks. A MusicBrainz lookup with `inc=genres+url-rels` for every resolved artist gave the reference.
- **Tracks (top / tail):** lookup hits 70.7% / 63.6%; accepted 68.5% / 57.8% (`acrr` 1,126, stripped `acrr` 55, `acr` 79, stripped `acr` 3); verified by ListenBrainz's own Spotify mapping 37.7% / 30.7% (54% of accepted). Of accepted tracks, 67.7% / 57.2% of all tracks have a length, 52.2% / 43.1% an ISRC, 63.7% / 53.9% a Cover Art Archive front. Deezer already gives 97.5% / 89.8% with duration and ISRC, and oEmbed gives every cover. **At track level ListenBrainz adds nothing.**
- **Wrong matches:** a manual check of 30 random accepted, unverified matches found 29 the same recording. The last ("Crew Love", Drake, an `acr` hit without the release) is the same song by the same artist on a bootleg mixtape release with no length. The artist was right in 30 of 30. No export artist got conflicting MBID votes. A sample of 20 rejected hits was mostly remixes and live versions (the export names the remix, ListenBrainz found the original) or exact names whose length was 25–30 s off: correctly rejected.
- **Artists:** 705 of the sample's 1,146 export artist names got an MBID (61.5%), covering 81.3% / 66.3% of tracks. The D27 MusicBrainz stage matched 998 and covered 94.2% / 88.6%, so **ListenBrainz cannot replace the MusicBrainz stage, but it resolves about 70% of the artists that stage finds.**
- **Genres:** ListenBrainz artist tags with a `genre_mbid`, put through the D27 mapping (`metadata.TopGenres`, 5 at most), **equal MusicBrainz's curated genres.** Of the 705 artists, 493 have genres from both sources: 484 with the identical top list, 486 with the same set, mean Jaccard 0.99. No artist had genres from one source and none from the other. That is 70% of resolved artists with genres (D27: 666 of 998, 67%).
- **Spotify artist IDs from url-rels:** of the 705 resolved artists, 634 have exactly one `open.spotify.com/artist/` link, 45 have several and 26 have none. Through those, a real Spotify artist ID covers 74.1% / 60.3% of tracks. An oEmbed check of 100 random links (`spotify:artist:{id}`, keyless, D27) returned the export's artist name for all 100.
- **Time:** the ListenBrainz part took 42 s for 2,000 tracks (lookups 10 s, metadata 24 s for 17 requests, Spotify IDs 7 s; 51 requests, no rate-limit waits), so **~12 min for the whole catalog.** The MusicBrainz reference made 718 requests in 16 min 48 s.
- **Projected for the whole catalog** (10,200 artists; ListenBrainz resolves somewhat under the sample's 61.5% because the catalog is mostly tail; 55–60% assumed):
  - Today: ~22,400 MusicBrainz requests, ~6.5 h.
  - **A. ListenBrainz first, genres only:** artists it resolves need no MusicBrainz request. The ~4,100–4,600 left go through today's route (≤ 2.2 requests each), about 9,000–10,100 requests. **About 3 h in all, with the same genres.** ListenBrainz also needs no Deezer ISRC, so it can start with the first fallback pass.
  - **B. A, plus Spotify artist IDs:** one more MusicBrainz request (`inc=url-rels`) per artist ListenBrainz resolved (~1.7 h). For artists on today's route, `url-rels` joins the existing `inc=genres` lookup at no extra request. About 4.5 h in all, or ~3 h for genres with the links filling in afterwards on a lower-priority pass.
- **Decided (built: `internal/hydrate/listenbrainz.go`, `internal/metadata/listenbrainz.go`, migration `0009_listenbrainz.sql`):**
  1. A new fallback stage, `listenbrainz`, between Deezer and MusicBrainz, needing no Deezer data. It runs the `acrr` → stripped `acrr` → `acr` cascade in batches (lookups ≤ 1,000 items, metadata 100 per request) and accepts hits with the D27 matcher or a verified Spotify track ID. It writes only what the MusicBrainz stage writes: the artist MBID (`fallback_artists.mbid`) and genres under the provisional artist ID, with `catalog_provenance` source `listenbrainz`. Confidence 0.95 when a verified track gave the MBID, else 0.95 × the track score. It writes no track fields (Deezer's are more complete). The MusicBrainz stage waits for it (until it settles a track or has failed it 3 times) and records a track whose artist it matched as matched, without a request. A track's ListenBrainz state is matched once its artist is; an artist stays open to MusicBrainz until one of its tracks matches.
  2. Today's MusicBrainz lookup asks for `inc=genres+url-rels` and stores the artist's one Spotify artist link in `fallback_artists.spotify_id`, at no extra request. Nothing uses it (O1).
  3. `METADATA_LISTENBRAINZ` turns the stage off like the other sources. `labs.api.listenbrainz.org` is an experimental host with no stability promise, so an error there (or a changed response shape) only backs the stage off: each track it fails is retried (1, 2, 4 min …), and after 3 failures MusicBrainz takes it.
- **Not decided:** option B, real Spotify artist IDs from MusicBrainz for provisional artists (O1).
- **Why:** it roughly halves the slowest stage with the same genres and no new keys, and nothing else changes: no Spotify tables, no track data, low-confidence hits rejected.
- **Rejected:** ListenBrainz in place of MusicBrainz (it misses ~30% of the artists MusicBrainz finds). ListenBrainz for track fields (Deezer covers more). The authenticated `api.listenbrainz.org/1/metadata/lookup/` fuzzy lookup (needs a user token).

### D38. Navigation: a sliding key, centred, with Settings as the last section (owner, 2026-10-19)
- **Desktop header:** the wordmark on the left and the sections centred in the header from 1024 px up (a three-column grid; between 768 and 1023 px they sit after the wordmark without icons so nothing overflows). Each section is an icon and a label; the current one stands on a raised key (the card surface with a hairline) and its icon lights in the signal colour. The key is one element under the links that glides to the new section on each navigation (300 ms, ease-out-quart), so the move reads as one control changing position; it jumps without the glide for reduced motion.
- **Phone tab bar:** the same pattern, with a pill behind the current tab's icon gliding between tabs.
- **Settings is the seventh section** (gear icon, last), linking straight to Settings → General. The header's settings dropdown is gone: the theme was already in Settings → General (D22), and sign-out, with "Signed in as" in proxy mode, moved to an Account panel at the end of Settings → General (shown only in password and proxy modes).
- **Why:** the owner compared five patterns live in the mock (text with the indicator lamp beside the current one, as before; a static raised key; a row of lit meter dots on the header's hairline; icons in a recessed well with only the current one labelled; this one) and chose the raised key, then asked for it to slide, for the links to be centred and for Settings to join them.
- **Rejected:** the other four patterns above; a dropdown for settings, theme and sign-out in the header.

### D39. A first paint that doesn't move (settled fixing the load twitch, 2026-10-19)

Pages twitched when they first loaded: text reflowed when Archivo replaced the fallback font, and content dropped when data replaced skeletons that were shorter than what they stood in for. The fix, measured with a `layout-shift` PerformanceObserver on the production build (mock mode, API delayed 1.5 s so the skeletons show, 1440 and 390 px wide):

- **The font loads before the first render.** The root `load` (`web/src/routes/+layout.ts`) waits for Archivo's latin face (`web/src/lib/fonts.ts`) next to the session check, at most 1.5 s. The face is registered from the same hashed file `layout.css` points at, so the browser fetches it once. A size-adjusted fallback `@font-face` was rejected: Archivo's width axis spans 62–125% and no fallback metric override matches every cut. `font-display: block` was rejected: invisible text still lays out in the fallback, so the shift remains.
- **Skeletons take the height of what they replace,** line for line: summary readouts, chart panels (the summary line is reserved, two lines on phones), stats charts at their drawn height, the 24-row hours table, list rows with an artist line (63 px against 56), as many list rows as the list asks for. Data-dependent heights reserve their room either way: the estimated-time note under the hero readout, the now-playing card (its playing height), page subtitles (two lines on phones, where they wrap).
- **The range bar stays hidden on Library (decided):** no empty 48 px bar. On first load nothing moves, because the layout knows the route before it first paints. Moving between Library and another section shifts the page by the bar's height, which follows a click and doesn't count as a layout shift. An empty bar would be a band of chrome that suggests missing controls.
- **Accepted:** shifts from data that only the response knows, such as a long name wrapping to two lines or a list with fewer rows than it asked for.

## Deferred

### DF1. DuckDB analytics engine

Add DuckDB as a read-only analytics layer that attaches the SQLite file directly.

- **Revisit when:** Building the read-only SQL `query` tool for the API/MCP (D16), or the data explorer (DF2), whichever comes first.
- **Constraints on work now:**
  - Keep the schema portable: integer epoch-ms timestamps, plain integer/text/real columns, no SQLite-only types or JSON-function-dependent query paths for core data.
  - Keep queries behind a query layer so a second engine can be added without touching handlers.
  - When added: set a memory cap and thread limit (defaults use up to 80% of RAM), and check arm64 build support for the chosen language.

### DF2. "Explore your data" feature

Ad-hoc filters, breakdowns, and groupings beyond fixed dashboards, possibly a SQL box for power users.

- **Revisit when:** v1 dashboards ship. Likely lands together with DF1.

### DF3. Mood / audio-feature stats

Spotify no longer provides audio features to new apps. Would need an external source (e.g. the frozen AcousticBrainz data dump matched via ISRC/MusicBrainz, or local audio analysis).

- **Revisit when:** Core stats are done. Low priority.

### DF4. Visibility of sensitive raw-only fields

Extended export rows include `ip_addr`, `conn_country`, and `incognito_mode`. They are stored in the raw layer (per D3); whether and how they surface in the UI/API is undecided.

- **Revisit when:** Designing the API/MCP permission model.

### DF5. Metadata hydration from non-Spotify sources

Supplement Spotify with other free sources, for example MusicBrainz (richer genres and release info; Spotify's artist genres are coarse) or Deezer's public API (artwork, durations).

- **Resolved by D27 (2026-10-17):** oEmbed, Deezer and MusicBrainz fill the gaps; Spotify stays primary. Original trigger: Spotify-only hydration proves too slow in practice, or genre quality becomes a limiting factor for stats. **Met (D24):** catalog lookups have a daily quota of 506 (measured 2026-10-19), so a lifetime export would take about 88 days to hydrate from Spotify alone.
- **Cost to weigh:** cross-catalog matching (by ISRC or name) and its wrong-match bugs; another rate limit to manage.

### DF6. Import non-history files from the basic Account Data package

These files are only available in the basic package (exact set varies by account and over time; verify against a real export before building parsers). They would land in the raw layer (D3). In priority order:

1. **`Wrapped20XX.json`:** "our numbers vs Spotify's" comparison for each year (trust-builder and a correctness check on D7/D8), plus a Wrapped archive across years.
2. **`SearchQueries.json`:** search-to-play discovery ("searched X, first played 2 min later, became a top artist"), searched-but-never-played, search timeline.
3. **`Inferences.json`, `Marquee.json`:** a "how Spotify sees you" page; compare Spotify's per-artist listener labels with actual listening.
4. **`YourSoundCapsule.json`:** low value; mostly duplicates our own stats.

Library and playlist files are not included here; that data should come from live API sync (see open questions).

- **Revisit when:** v1 dashboards ship. Start with the Wrapped comparison.
- **Also:** if users ask for basic-export history import (D10), it becomes easier later since name matching can run against the already-populated catalog before falling back to search.

### DF7. Podcasts, audiobooks, and video

Live capture, a content model (show → episode, book → chapter), and dashboards for non-music content.

- **Revisit when:** Music v1 ships, or on user demand.
- **Trade-off accepted now:** live podcast/audiobook plays are not captured until this lands, and `recently-played` never reports them, so that live history is lost. Export history is unaffected: those rows are kept in the raw layer and can be derived later via rebuild.
- **When added:** poller requests `additional_types=episode`; `plays` gains a `content_type` column; music dashboards filter to music by default. Verify how audiobooks appear in `/me/player`. Video exports arrive as separate `Streaming_History_Video_*.json` files.

### DF8. Advanced dashboard features beyond parity

- **Year in Review:** Wrapped-style summary for any year, including years before Spotify Wrapped existed.
- **Compare two periods:** side-by-side ("2019 vs 2025"), overlapping and dropped artists.
- **Patterns extras:** weekday × hour heatmap, listening streaks, new artists discovered per month, discovery source (first-play context), skip-rate analysis by context/shuffle.
- **Shareable cards:** Wrapped-style image cards for social media (stats.fm). Ties into public sharing (open question).
- **Public share link:** read-only public view of selected stats (YS "Public token"). Ties into public sharing.
- **Actions requiring write scopes:** create a playlist from a top list (YS), start playback from the app (YS). Both need extra Spotify scopes beyond D12's read-only set.
- **Revisit when:** v1 ships. Year in Review and Compare fit naturally with the explorer (DF2) and DuckDB (DF1).

### DF9. Library and playlist sync

Periodic sync of saved tracks/albums and the user's playlists, with snapshot diffs to detect removals.

- **Unlocks:** saved-but-never-played, playlist play-through (share of each playlist ever played), library timeline (save date → time to heavy rotation).
- **Costs:** `user-library-read` and `playlist-read-private` scopes; a sync job sharing the D9 rate budget; snapshot/diff model since the API only exposes current state.
- **v1 instead:** playlist context names from the poller are resolved lazily, one `GET /playlists/{id}` per playlist seen, cached.
- **Revisit when:** v1 ships. High on the post-v1 list: neither Your Spotify nor stats.fm offers it.

## Not planned

Features of Your Spotify / stats.fm that conflict with decided scope:

- **Multi-user and social** (conflicts with D1): YS collaborative affinity and shared top lists; stats.fm friends, compare links, taste matching, friend suggestions, chat, "top listeners" of a track/artist, global stats.
- **Mood / track-type stats** (energy, acoustic, etc.): Spotify no longer provides audio features. Tracked separately as DF3.

## Open (not yet discussed)

### O1. Linking provisional artists to MusicBrainz's Spotify artist IDs (from D37)

- **The data:** MusicBrainz lists one `open.spotify.com/artist/` link for most artists. The MusicBrainz stage now stores it (`fallback_artists.spotify_id`) and nothing reads it. When measured, 634 of 705 artists had exactly one link, and oEmbed returned the export's artist name for 100 of 100 checked.
- **The question:** may a provisional artist link to that real Spotify artist? These are real Spotify IDs, not invented ones, so D27's rule allows them. But linking changes derivation: the provisional artist would adopt the real one (D25), which then queues for Spotify hydration and borrows nothing. Proposed guard: exactly one link, and oEmbed returns a matching name. Artists ListenBrainz found would need one more MusicBrainz request each to get their link (~1.7 h for the whole catalog).

See [plan.md](plan.md) for the build plan.
