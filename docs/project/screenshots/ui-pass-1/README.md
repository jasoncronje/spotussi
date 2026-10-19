# UI pass 1 (after owner review of U1)

Mock backend (`--mock --mock-size small`, `AUTH_MODE=password`, capture off),
Chrome headless with reduced motion, 1440×960 desktop and 390×844 phone.
`before-*` is `dc4870a` (U1 + M3), `after-*` is this pass. Mobile shots of
app-shell pages are one viewport tall (the fixed phone nav would otherwise sit
mid-page in a full-page capture); everything else is full page. Connection
states other than "not configured" (paste-back with a Client ID, direct
HTTPS, connected, waiting for a first play) are route fakes over the mock's
real responses; the import is a real upload to the mock.

## What changed

**Copy.** Labels, values and the shortest hint that prevents a mistake.
Kept: the exact redirect URI and its exact-match rule (with the port), the
setup-token location, error messages that say what to do, the "token shown
only once" warning, destructive confirmations, the open-access warning.
Explanations moved into info tooltips (`common/hint.svelte`): the play rule,
catalog hydration, Top attribution rules.

**Settings copy, second cut.** Section descriptions became info hints on
the panel title (`settings-panel.svelte` `hint`), and sentences that
restated a label, a button or a readout were removed. Kept: the
token-shown-once warning, destructive confirmations, the open-access
warning, "Keep this tab open" during upload, the file the upload expects,
and validation errors.

**Header menu and Settings polish.** The theme cycle button and the
settings link are replaced by one menu (cog) holding Settings, the theme
(System, Light, Dark, with a check) and Sign out; the old button showed
the stored choice, so "system" looked like a random screen icon. Pages
outside the shell keep a two-state light/dark switch. Settings tabs have
icons, titles and labels are in Title Case. Import offers "Request on
Spotify" in place instead of sending you into the setup guide. API tokens:
a styled reveal card with a Done button, and a table of tokens. Endpoint
commands truncate instead of wrapping. System is three panels (Live
Capture, Catalog Details, Stats), each with its state in the header; the
Access panel is gone (sign-out lives in the menu).

**Settings tabs**, one route each, plain links with `aria-current="page"`:

| URL | Contents |
|---|---|
| `/settings/general` | Counting and time, Display |
| `/settings/library` | Excluded from stats, Spotify playlist names |
| `/settings/spotify` | Spotify account |
| `/settings/import` | Upload and recent imports |
| `/settings/tokens` | API tokens |
| `/settings/system` | Capture/catalog/rebuild status, Access |

`/settings` opens General; old anchors (`/settings#import`, `#exclusions`,
`#access`, …) redirect to their tab. Rail with lamps on wide screens, a
scrolling strip with an amber underline on phones.

**Onboarding.** The progress indicator is a tuner's station scale behind
graphite glass: one station per step, printed like frequencies, with minor
ticks between, an amber needle on the current step and the scale lit behind
it. The stations are the step links. Changing step glides the needle over
with a dial-cord overshoot, and nothing else moves on its own. The "tuned"
lamp above the glass lights once Spotify is connected. Reduced motion jumps
the needle. Sign-in has no decoration.

## Checks

- axe-core 4.11, WCAG 2.0/2.1/2.2 A+AA tags: 0 violations on 22 pages ×
  desktop/phone × light/dark (88 runs): setup, login, every guide step and
  state, all six settings tabs, overview, top, stats, history, sessions.
- First-load JS (encoded bytes over the wire, fresh context):

| Route | Before | After |
|---|---|---|
| `/` | 211.3 KB | 213.8 KB |
| `/stats` | 203.6 KB | 205.7 KB |
| `/history` | 104.6 KB | 106.9 KB |
| `/sessions` | 99.4 KB | 101.7 KB |
| `/settings` (one page) → tabs | 119.2 KB | 101.3–108.4 KB |
| `/welcome` | 107.8 KB | 112.3 KB |
| `/login` | 96.3 KB | 98.6 KB |
| `/setup` | 96.6 KB | 100.3 KB |

The root layout chunk grew 0.04 KB; the ~2 KB on untouched routes is Rollup
re-splitting shared chunks across the six new routes (6–7 more, smaller
files per page) plus the larger route manifest. Settings tabs each load only
what they show. Onboarding pays ~2–4 KB for the scale and the first-play
watch. No new dependencies.
