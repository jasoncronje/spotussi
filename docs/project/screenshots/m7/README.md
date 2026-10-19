# M7 dashboard integration (and M8 UI states)

All data is mock or synthetic: `serve --mock --mock-size full` with mock
scenarios (`POST /_mock/scenario`), and live binaries on empty temp data
directories with a generated export (`Synthetic Artist N`) or generated
poller rows (`Capture Artist N`). CAPTURE_ENABLED=false throughout.

| File | State |
|---|---|
| overview-sparse-reconnect-{light,dark} | Mock, sparse catalog, Spotify reconnect required: reconnect banner, catalog strip (quota), Plays from / Track details mixes |
| overview-phone-light | Same, phone width, full page |
| track-provisional-dark | Track with fallback details: "Provisional details" badge |
| track-names-only-dark | Track known by name only: badge, unlit artwork |
| artist-phone-dark | Artist page, phone |
| settings-system-{light,dark} | Catalog state, Spotify + provisional coverage, fallback sources, Download backup |
| reconnect-dark | Focused reconnect at `/welcome?step=connect&reconnect=1` |
| card-failure-light | Top artists failing with a 500 (mock scenario), rest of Overview intact |
| server-unreachable-dark | Server stopped mid-session: banner, previous data kept dimmed |
| session-ended-dark | 401 mid-session lands on sign-in with the notice |
| fresh-overview-light, fresh-stats-light | Fresh live instance, no history |
| export-only-overview-light | Live instance with only a synthetic export, part fallback-matched |
| capture-only-overview-light | Live instance with only synthetic poller plays |

Checks: axe-core 4.11 (WCAG 2.0/2.1/2.2 A+AA) 0 violations on 68 runs:
11 mock pages (Overview, Top, Stats, History, Sessions, artist, album,
track, Settings System, Settings Spotify, reconnect) and 5 fresh-instance
empty pages, desktop/phone × light/dark, plus Overview with a failed card.
No console errors or warnings clicking through every page, both settings
tabs, every onboarding step and range switches on each.
Range-switch timings: `perf.md`.
