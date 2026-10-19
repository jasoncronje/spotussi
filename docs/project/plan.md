# Build plan

A fast, self-hosted Spotify listening stats app: one binary, one SQLite file, one Spotify account per instance. This plan turns the decisions in [decisions.md](decisions.md) (referenced as D1–D18, DF1–DF9) into an architecture and an ordered set of milestones.

## What makes it different

1. **Fast at any history size.** Every dashboard query reads precomputed rollups (D4). Page data comes back in milliseconds whether you have one month or fifteen years of history.
2. **Honest numbers.** Real `ms_played` and skips for live plays (D2), one play rule applied to every source (D7), and plays merged across sources without double counting (D8). Your Spotify records every live play as the full track length.
3. **Nothing thrown away.** Raw source rows are kept forever and `plays` is rebuilt from them (D3), so rule changes and bug fixes apply to all history.
4. **Trivial to run.** One container, one volume, zero required env vars, a setup wizard that handles Spotify's 2025 redirect rules (D12, D15).
5. **Built to be queried.** The UI runs on the public API (D6, D16); MCP and SQL access build on that.

## Architecture

Everything runs in one Go process (D5):

```
                         ┌────────────────────────── one Go binary ──────────────────────────┐
 Browser ── HTTPS ──▶    │ HTTP server                                                       │
 (SvelteKit SPA,         │  ├─ /            embedded SvelteKit build (D6, D18)               │
  embedded)              │  ├─ /api/v1      JSON API, OpenAPI at /api/docs (D16)             │
 API / MCP clients ──▶   │  ├─ /mcp         MCP server (D16, D28)                            │
                         │  └─ /oauth/...   Spotify connect wizard (D12)                     │
                         │                                                                   │
                         │ Auth: password | proxy | none + API tokens (D13)                  │
                         │                                                                   │
                         │ Background workers                                                │
 Spotify Web API ◀──────▶│  ├─ Player poller        /me/player, adaptive (D2)                │
   (one shared           │  ├─ Recent-plays backstop /me/player/recently-played, ~20 min (D2)│
    rate limiter)        │  ├─ Catalog hydrator     priority queue, tracks → artists (D9)    │
                         │  └─ Context resolver     playlist names, lazy (D17)               │
                         │                                                                   │
 Export upload ─────────▶│ Importer: streaming JSON parse → raw rows (D10)                   │
                         │                                                                   │
                         │ Derivation engine (D3, D7, D8)                                    │
                         │  raw rows ─▶ merge + dedupe + play rule ─▶ plays ─▶ rollups       │
                         │  incremental for live plays; full rebuild on rule/import changes  │
                         └───────────────────────────────┬───────────────────────────────────┘
                                                         ▼
                                       /data/spotussi.db (SQLite, WAL) + /data/key
```

### Go packages (proposed)

| Package | Responsibility |
|---|---|
| `cmd/spotussi` | Entry point; subcommands `serve` (default), `import`, `backup`, `restore`, `mcp` (stdio proxy) |
| `internal/config` | Env (`BASE_URL`, `AUTH_MODE`, trusted proxies) + settings stored in DB |
| `internal/db` | SQLite connection (modernc), migrations, backup and restore |
| `internal/spotify` | The only package that talks to Spotify: PKCE OAuth, token refresh/rotation, shared rate limiter with priority lanes, typed endpoints. Isolates us from API policy churn. |
| `internal/capture` | Player poller state machine, recently-played backstop |
| `internal/importer` | Extended export ingestion (zip or JSON files), streaming parse |
| `internal/hydrate` | Catalog hydration (M4, D24): seeding from live capture, the queue, the worker, context resolution; fallback stages (D27, D37) |
| `internal/metadata` | Fallback metadata clients (D27, D37): Spotify oEmbed, Deezer, ListenBrainz, MusicBrainz, each with its own limiter; name matching |
| `internal/derive` | Merge, play rule, plays table, rollups, incremental and full rebuild |
| `internal/query` | All read queries behind one layer (keeps DF1 DuckDB addable) |
| `internal/api` | HTTP handlers for `/api/v1`, OpenAPI spec, ETags |
| `internal/auth` | Password (argon2id), proxy header mode, sessions, API tokens |
| `internal/mcp` | MCP server over the query layer |
| `web/` | SvelteKit app, shadcn-svelte, LayerChart, generated TS API client |

## Data model (outline)

Timestamps are integer epoch milliseconds (UTC) everywhere, per the DF1 portability constraint.

**Raw layer** (append-only, never edited; D3)

| Table | Key columns | Notes |
|---|---|---|
| `raw_export_rows` | `id`, `import_id`, `row_hash` (unique), `ts_ms` (stream end), `ms_played`, `content_type`, `uri`, names, `reason_start`/`reason_end`, `shuffle`, `skipped`, `offline`, `incognito`, `platform`, `conn_country`, `video`, `json` | Verbatim row JSON plus extracted columns for merging. All content types kept (D11). Re-importing the same export is a no-op via `row_hash` (D23). |
| `raw_recent_plays` | `played_at_ms`, `track_id` (unique pair), `context_uri`, `json` | Verbatim item from `recently-played`. |
| `raw_poller_plays` | `id`, `track_id`, `started_ms`, `ended_ms`, `ms_played`, `skipped`, `shuffle`, `repeat`, `device_name`, `device_type`, `context_uri`, `json` | One row per completed play observed by the poller, with the last player snapshot. |
| `imports` | `id`, `filename`, `origin`, `kind`, `state`, file/byte/row progress (`rows_new`, `rows_duplicate`), timestamps, `error_code`, `error` | Import jobs and progress (D23). |

**Catalog** (D9, D17; schema in `internal/db/migrations/0005_catalog.sql`, the shared contract between M4 and M5)

`tracks` (id, name, duration_ms, explicit, isrc, album_id, disc/track number, hydrated_at), `albums` (id, name, type, release_date, precision, total_tracks, image_url), `artists` (id, name, image_url, hydrated_at), `track_artists` (track_id, artist_id, position), `artist_genres` (artist_id, genre), `contexts` (uri, type, name, name_source: `api`/`manual`/`unresolved`), `hydration_queue` (entity_type, id, priority, attempts, next_attempt_at).

Fallback metadata (D27; `0007_fallback.sql`): `fallback_artists` and `fallback_albums` (keyed by derivation's provisional `ar_`/`al_` IDs: images, Deezer/MusicBrainz IDs, album type, release date, track count), `fallback_state` (source, entity, status matched/no_match/failed, external ID, confidence, backoff), `catalog_provenance` (entity, field, source, confidence; no row means Spotify wrote it). Fallback track fields go on the track's own row (`source = 'fallback'`, `hydrated_at` null); fallback genres go into `artist_genres` under the provisional artist ID. `0008_borrowed.sql` adds the `borrowed` provenance source: an unhydrated real artist provably the same as a provisional one takes its image and genres. `0009_listenbrainz.sql` adds the `listenbrainz` source (D37) and `fallback_artists.spotify_id` (the one Spotify artist link MusicBrainz lists; stored, not used).

**Derived** (rebuildable from raw + catalog + settings)

- `plays`: `id`, `track_id`, `started_ms`, `ended_ms`, `local_day`, `local_hour`, `ms_played`, `ms_played_estimated`, `counts_as_play`, `skipped`, `shuffle`, `platform`, `device`, `context_uri`, `country`, `reason_start`, `reason_end`, `sources` (bitmask: export/poller/recent), `contradicted` flag (D8).
- Rollups keyed by `local_day` (D14 timezone): `rollup_day` (totals), `rollup_day_hour`, `rollup_day_track`, `rollup_day_artist`, `rollup_day_album`, `rollup_day_genre`. Each stores `plays`, `ms_played`, `skips` so every view supports the Plays/Time toggle (D7).
- `sessions`: contiguous listening runs for Longest sessions (D14).

**App state**: `settings` (play rule, timezone, date format, theme, exclusions), `secrets` (encrypted refresh token), `api_tokens` (hashed), `auth_sessions`.

Final rollup shapes are fixed during M5 against the actual D14 queries and the performance budgets below. Entity-level breakdowns that are narrow (one artist's listening clock) can query `plays` directly via indexes.

## Key flows

### Onboarding
1. Start the container → open the URL → set the owner password (or skip in proxy/none mode).
2. Wizard: create a Spotify app (dashboard link, exact redirect URI to copy), paste the Client ID, connect via direct callback (HTTPS) or loopback paste-back (HTTP) (D12).
3. Live capture starts immediately. The wizard explains requesting the Extended Streaming History export and that it can take up to 30 days (D10).
4. When the export arrives: upload the zip → import → rebuild → dashboards show lifetime history while hydration fills in artwork (D9).

### Live capture state machine (D2)
- `idle` (poll every 1–2 min) → `playing` (poll every 10–15 s + one poll at expected track end) → on track change, close the current play: `ms_played` from accumulated progress, `skipped` if it ended well before `duration_ms`, store in `raw_poller_plays`, then run incremental derivation.
- Handles: pause/resume (progress doesn't advance), seeks (progress jumps), repeat-one (same track restarts), device switches, ads/podcasts (gaps, D11), 429 (`Retry-After`, back off), token expiry (refresh; surface "reconnect Spotify" if the refresh fails).
- The recently-played backstop runs every ~20 min and inserts anything the poller missed; derivation merges it.

### Derivation (D3, D7, D8)
- **Incremental path** (live plays): merge the new raw row against plays in its time window, upsert into `plays`, adjust the affected rollup rows. Runs within milliseconds of a play ending.
- **Full rebuild** (import finished, play rule/timezone/exclusions changed, merge logic changed, hydration batch completed): recompute `plays`, rollups, and sessions in a single transaction, swapping in new tables so the UI never sees a half-built state.

## Performance budgets

Measured against a generated reference dataset: 15 years, 500k plays, 40k tracks, 10k artists. Targets, to be confirmed in M8 benchmarks:

| Measure | Target |
|---|---|
| Any `/api/v1` dashboard endpoint, server time p95 | < 25 ms |
| History page query (paginated, filtered) p95 | < 50 ms |
| Full rebuild (plays + rollups + sessions) | < 10 s |
| Import of a 300 MB extended export (excluding hydration) | < 60 s, memory < 200 MB |
| Idle memory | < 60 MB |
| Initial JS bundle (gzipped) | < 200 KB |
| Docker image | < 30 MB |

Benchmarks run in CI against the reference dataset so regressions fail the build. The generator also gives fixtures for UI development before a real export is available.

## Milestones

**0.1 beta** (D28), the first release, is M0–M9. Live capture comes early on purpose: plays that aren't captured are lost for good (except via export), so the poller should run on your own instance from the first weeks. The UI runs as a parallel track (below) rather than after the API.

| # | Milestone | Scope | Done when |
|---|---|---|---|
| **M0** | Foundations and API contract | Go module and package layout; SvelteKit (adapter-static, Svelte 5, Tailwind v4, shadcn-svelte, LayerChart) embedded via `go:embed`; SQLite + migrations; CI (lint, test); multi-arch Docker build; dev workflow (Go API + Vite dev server proxied, browser-facing dev server run through Portless). **Contract:** reference dataset generator (synthetic plays with realistic distributions), draft OpenAPI spec (`api/openapi.yaml`) for every D14 view, and `serve --mock`, which generates the dataset in memory and answers every endpoint by naive aggregation (later the correctness oracle for M5) | ✅ Done. Container serves the SPA and `/api/v1/health`; `serve --mock` answers every endpoint in the spec |
| **M1** | Auth and Spotify connect | D13 auth modes, sessions, API tokens; D12 wizard, PKCE, both callback modes, encrypted refresh token, rotation | ✅ Built and tested against a fake Spotify server; real-Spotify check pending the owner's Spotify app. Follow-ups (D21) land with M2 |
| **M2** | Live capture | Spotify client and shared rate limiter; poller state machine; recently-played backstop; raw tables; live "recent plays" view. **Spike:** confirm `played_at` semantics, quota behavior, audiobook appearance in `/me/player` | Poller runs on your instance for a week with no missed plays vs `recently-played` — ✅ Built; verified on real Spotify (plays, skips, play rule, backstop, cross-source merge) and deployed to the owner's instance on 2026-10-17; the week-long run and the `played_at` calibration verdict (system status → `capture.calibration`) are pending on the owner's instance. Audiobooks: `/me/player` is called without `additional_types`, so they should report as non-track items (a timeline gap); unverified |
| **M3** | Import | Upload of zip/JSON, streaming parse of `Streaming_History_Audio_*.json`, raw rows with dedupe by hash, import progress | Your real export imports within budget, and re-import is a no-op — ✅ Built (D23): `POST/GET /imports`, `spotussi import`, `raw_export_rows`, provisional `/history` with export rows. A real lifetime export (17 MB zip, 235 MB JSON, ~300k rows over 13 years) via the CLI on an M1 Max: 16.9 s wall, 78 MB max RSS, 17 MB peak Go heap (budget < 60 s, < 200 MB: met); ~296k new rows (tracks, plus some podcast episodes and audiobook chapters) and ~1,900 duplicates inside the export itself (most byte-identical); re-import 2.9 s, 0 new. Same via a throttled 43 s upload to a live server: 17.4 s processing. Provisional `/history` over those rows: 1–4 ms per page once counted; the first all-time request with a filter takes 54–58 ms (counting ~295k index entries; just over the 50 ms History budget, which M5's `plays` table takes over), unfiltered 43 ms, scoped or shorter ranges ≤ 5 ms. Import UI built: upload control (drag-and-drop or picker, upload progress with cancel, job progress, result and error copy) in Settings → Import with recent imports (CLI runs included) and on the setup guide's last step; axe WCAG 2.2 AA clean on both pages, desktop/phone, light/dark; +7.9 KB gzipped JS, loaded only by those two pages; screenshots in `docs/project/screenshots/m3-import` |
| **M4** | Catalog hydration | Priority queue (most-played first), tracks → artists, progress reporting, context resolver (D17). **Spike:** private and Spotify-owned playlist lookups | Top 1,000 tracks hydrated within minutes of import; whole catalog overnight — 🟡 Built (D24), but the target is bounded by a daily quota the spike found: `internal/hydrate` seeds the catalog from live capture (backfilling existing raw rows), queues export tracks by plays (≥ 30 s streams), artists at their track's priority, live contexts by plays; worker in the limiter's background lane with per-entry backoff, 404 parking, restart-safe queue; `catalog` in `/system/status` (state, per-type queue, failures, rate, quota, ETA); `OnCatalogChanged` hook for M5. A lifetime export of ~34k tracks and ~10k artists → ~44k lookups, ~1,800 for the top 1,000 (modelled with the fake on the export's shape). At 2 req/s that's ~15 min for the top 1,000 and ~6 h for everything, but catalog lookups are capped at **506 a day** (measured 2026-10-19; `429 QUOTA_EXCEEDED` until the daily reset at ~00:08 UTC), while live capture keeps working. That's 4 days for the top 1,000 and ~88 days for everything from Spotify alone, or 2 and ~68 days for tracks alone with artists last (decided 2026-10-19, D24; production lookups back on). QUOTA_EXCEEDED now pauses only its bucket. Spotify-made playlists answer 404 (not requested; manual naming). The user's own private and public playlists resolve (200), and `/playlists` kept working while the catalog quota was spent, so it's outside Spotify's catalog bucket: the limiter now gives `/playlists` its own quota bucket, and the worker keeps resolving playlist contexts while tracks and albums wait for the reset (and pauses only playlist contexts when their quota runs out). **Fallbacks (D27):** oEmbed, Deezer and MusicBrainz fill the gaps while the quota is spent (own limiters, `METADATA_*` flags, per-field provenance in migration 0007, per-source progress in `/system/status`); measured on a large real export: oEmbed 100%, Deezer 97.5% (top 1,000) / 89.8% (tail), MusicBrainz artists ~90% (genres for ~2/3), 30/30 spot-checked matches correct, ~6.5 h projected for the whole catalog. **ListenBrainz first pass (D37):** batch name lookups find the artist MBID and MusicBrainz genres for ~60% of export artist names (genres identical to MusicBrainz's), so MusicBrainz looks up only the rest: ~3 h projected for the whole catalog |
| **M5** | Derivation | Merge (D8), play rule + fixed-threshold alternative (D7), estimated-duration flags, rollups shaped by the queries of the approved UI pages, sessions, exclusions, timezone; incremental + full rebuild; benchmarks on the reference dataset | Rebuild and query budgets met on the reference dataset; merged counts reconcile with export totals — ✅ Built (D25). In-memory columnar snapshot over durable `plays`/`sessions` tables (migration 0006); D8 merge with nearest-first pairing; provisional name-keyed entities until hydration; full rebuild with a table swap on import, settings, exclusion and debounced catalog changes (`(*derive.Engine).CatalogChanged()`, reached via `live.Backend.Engine()`); incremental merge of live plays from the last listening gap; restart loads the table (watermarks). Numbers on an M1 Max under heavy load (load average 20–60). **Reference dataset full (500,019 plays):** full rebuild 5.5 s; dashboard worst p95 11.8 ms (all-time ranges); History worst p95 2.2 ms; live heap 44 MB. **A real lifetime export (~300k rows):** import 17.4 s, then full rebuild 4.4 s (restart load 2.6 s); dashboard worst p95 8.8 ms; History worst p95 1.6 ms; idle memory on Linux (Docker, arm64) after using every dashboard view 59.8 MB RSS, 54.9 MiB container (budget < 60 MB: met, narrowly). Reconciliation: every music row (non-video, with a URI) became exactly one play, with ms_played totals matching to the millisecond. Video, episode and audiobook rows excluded per D11. Every track provisional until M4 fills the catalog |
| **M6** | API v1 | Implement the approved contract for every D14 view; `/api/docs`; ETags; generated TS client; token access | All D14 data is retrievable via documented endpoints with a token; responses match the contract the UI was built against — ✅ Built (D26). Every Backend method is served by `internal/live` from the snapshot (no 501s), including `/history` from plays. A differential test against the mock oracle covers 1,127 requests with 0 differences, plus an incremental run (batches, then a restart) with 0 differences. Spec validation, ETags and token access are covered by tests. `/api/docs` serves Redoc over the embedded spec. Spec change: optional `derive.progress` in system status. Every UI page checked in the browser against a live server holding the real export (capture off) |
| **M7** | Dashboard integration | Swap mocks for the generated client; loading, empty, and catalog-hydrating states; provenance badges; range-switch performance pass; real-data polish | Feature parity checklist (D14) complete on real data — ✅ Done 2026-10-18, approved by the owner on production (D32). Provenance in the API (`metadata` on artist/album/track, `sources` and `catalog` mixes on the Overview summary; live, mock and diff test) and in the UI (Overview mix meters, History source lamps and the "Provisional details" badge behind an off-by-default "Data sources" setting; "Names only" badge and unlit artwork always). Catalog-loading strip, empty states by cause (fresh, import running, empty range) on every D14 page, checked on fresh, export-only and capture-only live instances with synthetic data. Range switches: no long tasks left on Overview, Stats, Top or entity pages; Overview cached switch 94–101 → 49–53 ms, Stats all time 159 → 70 ms (`docs/project/screenshots/m7/perf.md`). axe clean (68 runs), no console errors; screenshots in `docs/project/screenshots/m7`. D14 parity walk: complete; album pages also rank their artists when three or more appear on the tracks (owner review, 2026-10-18). |
| **M8** | Hardening and 0.1 beta release (D28) | Backups (`backup` subcommand + download in Settings), README with compose + Traefik labels, binaries on Releases, perf benchmarks in CI, error states, Spotify reconnect UX (UI parts built with M7, D32: server-unreachable banner with recovery, per-section failures, session-ended sign-in, reconnect banner and focused reconnect flow, Settings → System "Download backup" link) | Tagged `v0.1.0-beta.1` once M7 and M9 are in; fresh-install walkthrough under 10 minutes — 🟡 Backend and release half built (D30), M7 approved 2026-10-18; tag waits for the private GitHub repository and a green CI and release run (D33): `spotussi backup` and `GET /system/backup` (consistent `VACUUM INTO` snapshot, integrity-checked, session only), `spotussi restore` (verifies, keeps the old database, clears stale WAL files, refuses while the server runs, `--key`); README for beta users (compose, Traefik labels, forward-auth with a token router, Spotify setup, env vars, import, backup and restore, binaries, MCP link); `release.yml` on `v*` tags (multi-arch image to GHCR, static linux/darwin amd64/arm64 archives with `SHA256SUMS`, GitHub pre-release from `docs/release-notes/<tag>.md`; checked with actionlint, archives built locally); `perf` CI job failing past 2.5× the budgets. Reference dataset on an M1 Max under heavy load: rebuild 5.6–7.9 s, dashboard worst p95 9.0–16.3 ms, History worst p95 2.1–4.5 ms. Image 8.1 MB compressed, 29.2 MiB unpacked; idle container 29 MiB. Fresh install with the image: 20 s of commands and forms to the Spotify-app step, an estimated 8–9 minutes for a new user including the Spotify app; the walkthrough found and fixed two problems (an untrusted reverse proxy silently produced the plain-HTTP redirect URI, now logged with the fix; restore needed a subcommand). Error states and the Spotify reconnect UX: see M7 |
| **M9** | MCP server (was v1.1; in 0.1 beta per D28) | `/mcp` Streamable HTTP, `mcp` stdio proxy subcommand, wrapper tools (D16) | Works from Claude Code / Claude Desktop with an API token — ✅ Built (D29): `internal/mcp` on the official Go SDK, stateless Streamable HTTP with JSON responses at `/mcp`, API tokens only (browser sign-ins refused; open in `AUTH_MODE=none`), 8 read-only tools (`get_status`, `get_overview`, `get_top`, `get_entity`, `get_timeline`, `get_history`, `get_sessions`, `search_catalog`) over the same Backend as `/api/v1`, `spotussi mcp --url` stdio proxy (token from `SPOTUSSI_TOKEN`). Tests: every tool against the mock oracle (results compared with the Backend), auth (none, bad, revoked token, session cookie, `AUTH_MODE=none`), in-process and subprocess stdio proxy. Verified with Claude Code 2.1 over HTTP and over the stdio proxy against `serve --mock` through Portless: all 8 tools listed and answered, IDs chained, errors readable. Claude Desktop is covered by the stdio path and the subprocess test, not run by hand. Setup in [mcp.md](../mcp.md) |

### UI track (runs alongside M1–M6; D19)

The UI is designed and built against the M0 contract and fixtures, not after the API. Approved pages define which queries M5 rollups must serve.

| # | Scope | Done when |
|---|---|---|
| **U0** | Visual direction and app shell: layout and navigation, time-range picker, Plays/Time toggle, theme tokens (light/dark), typography, chart style (colors, axes, tooltips, listening clock). **Overview page** built to final quality as the reference page. | ✅ Approved 2026-10-17 ("hi-fi readout" direction; Overview initial JS 113 KB gzipped) |
| **U1** | Remaining D14 pages on fixtures: Top lists, Stats, artist/album/track pages, History, Longest sessions, Settings/system, onboarding wizard screens. Iterate with owner review. | All D14 pages approved on fixture data; contract changes fed back into the OpenAPI spec. 🟡 Built and merged 2026-10-17 (contract changes in D22; screenshots in `docs/project/screenshots/u1`); awaiting owner review. Open proposals: mock status/connection consistency, `prev_rank` on entity stats, Spotify-owned filter on `/contexts`. Design pass 1 after owner review (2026-10-17): narrative copy cut, Settings split into tabs at `/settings/{general,library,spotify,import,tokens,system}`, onboarding rebuilt around a dot-matrix meter; axe clean, notes and screenshots in `docs/project/screenshots/ui-pass-1`. |
| **U2** | Live data: switch the recent-plays view to real data once M2 runs on the owner's instance. | Owner sees their own live plays in the UI |

Integration with real endpoints happens in M7.

### Execution

| Work | Agent setup |
|---|---|
| UI track (U0–U2, M7 UI work) | Opus, high effort, using the `frontend-design` skill |
| Everything else (M0–M6, M8, M9) | Opus, medium effort |

Ordering: M0 runs first (everything depends on the scaffold and contract). Then the UI track and backend milestones M1 → M6 run in parallel, in separate worktrees, touching separate directories (`web/` vs `cmd/`, `internal/`). Contract changes from UI review go through the OpenAPI spec.

### After 0.1 beta (suggested order)

1. **DF9** Library and playlist sync (distinctive; scope partly granted already). 🟡 Built (D36), measured on synthetic data; the owner-account checks are pending. Scope `user-library-read` added. Migration 0010 has library syncs, membership spans, playlists and walks. `internal/library` runs an hourly quick check and a daily full walk in the limiter's background lane, and seeds the catalog without lookups. It has `/library/*` endpoints, `POST /library/sync`, `saved_at` on tracks, the `get_library` MCP tool, a Library page, a playlist page and a Settings → Spotify panel.
   - **Requests (fake Spotify, tests):** a quick check of an unchanged library costs 3 requests. Followed and Spotify-made playlists never request items. No catalog lookups are made.
   - **Correctness:** the differential test covers every library endpoint against the mock oracle, including after exclusion and settings changes. ISRC and relink matching are tested both ways.
   - **Speed:** library endpoints on the full reference dataset (500k plays, about 8k saved songs, 130 playlists) measure p50 1.6–22 ms and p95 2–28 ms. That was on an M1 Max at load average 140–310, so the 25 ms dashboard budget is met only within noise. The perf CI job now covers them.
   - **What fixed speed:** ISRCs, relinks, played name keys and the catalog rows of unplayed tracks are cached per snapshot, and spans are cached until the newest library sync row changes. Before this, p95 was 120–280 ms, 70% of it SQLite page reads.
   - **UI:** axe WCAG 2.2 AA found 0 violations in phone and desktop, light and dark, with no horizontal scroll.
   - **Pending on the owner's account:** whether `/me/tracks`, `/me/albums` and playlist items still answer while the catalog quota is spent, how many saved tracks match only by ISRC, and the real first-sync request count and duration. Each needs a reconnect for the new scope.
2. **DF1 + DF2** DuckDB, the data explorer, and the SQL `query` tool for API/MCP.
3. **DF8** Year in Review and period comparison (build on DF1), then pattern extras.
4. **DF6** Wrapped comparison from the basic package.
5. **DF7** Podcasts and audiobooks.
6. **DF5**, **DF3**, remaining DF8 items (sharing, write-scope actions), **DF4** as needed.

## Risks and items to verify

| Risk / unknown | Impact | Mitigation |
|---|---|---|
| **Spotify API policy keeps changing** (Nov 2024, Feb 2026, Jul 2026) | Endpoints or fields disappear; dev-mode limits tighten | All Spotify access goes through `internal/spotify`; raw layer keeps everything already captured; exports remain an API-independent source; track the Spotify changelog |
| **Spotify branding rules** | Spotify's developer terms restrict using "Spotify" in app names and logos | Keep "Spotify" out of the product name and icon; use only permitted attribution |
| Undisclosed rate limits / per-developer quota | Hydration slower than estimated; possible 429 storms. **Seen in M4 (D24):** catalog lookups have a daily quota of 506 (measured 2026-10-19) | Shared limiter with priority lanes, adaptive backoff, configurable rates, visible progress; quota pauses per bucket so live capture is never blocked; DF5 sources adopted as D27 fallbacks |
| `played_at` start vs end semantics | Merge window mismatch → duplicates or missed merges | M2 spike: calibrate against poller start/end before M5 |
| Private playlist lookup scope; Spotify-owned playlist failure mode | Context names unresolved | Spotify-owned: 404 (verified M4, not requested any more); private: own private playlists resolve with `playlist-read-private` (verified M4); manual naming fallback (D17) |
| Refresh token expiry or revocation (third-party reports of ~6-month lifetimes, unconfirmed) | Silent capture stop → lost plays | Health check plus a prominent "reconnect Spotify" banner; optional notification hook later |
| Export format drift (fields added or renamed over the years) | Import failures on older or newer exports | Store verbatim JSON; tolerant parser with per-field fallbacks; fixture exports from multiple years |
| Poller edge cases (seek, repeat-one, device switch, offline playback) | Wrong `ms_played` / skip data | State-machine unit tests with recorded player sequences; export later reconciles via D8 provenance flags |
| Travel across timezones | Day boundaries wrong for travel periods | v1 uses one display timezone (D14); `conn_country` is kept in raw data for a future per-play timezone option |

## Not in 0.1 beta

See [decisions.md](decisions.md) "Deferred" (DF1–DF9) and "Not planned" (multi-user/social features, mood stats).
