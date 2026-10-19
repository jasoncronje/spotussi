# M7 range-switch performance pass

Measured with `scripts/perf/range-switch.sh` (agent-browser, headless Chrome,
1440×900) against `make mock` (full mock dataset, ~500k plays), production
build. Per switch: time to the URL change, time until nothing is loading or
dimmed, long tasks (> 50 ms) and the worst frame gap. Switches are clicks
without a hover first, so prefetch doesn't help (worst case).

## Before (main at 8a18501)

| Page | Switch | Settled | Long tasks |
|---|---|---|---|
| Overview | All time (cached) | 101 ms | 75 ms |
| Overview | Last 4 weeks (cached) | 94 ms | 72 ms |
| Overview | Last 4 weeks (from all time, cold) | 96 ms | 75 ms |
| Overview | Plays/Time at all time | 133 ms | 70 ms |
| Stats | All time (cached) | 159 ms | 131 ms |
| Stats | Last 4 weeks (from all time) | 129 ms | 84 ms |
| Stats | Plays/Time at all time | 91–181 ms | 68–80 ms |
| Top artists | any | 15–65 ms | none |

Profile: ~30 ms per switch in Svelte's `schedule_possible_effect_self_invalidation`
(a cache hit written to state inside the Resource effect walked every
reaction of `current`), deep proxies over every API response, ~40 ms
rebuilding a LayerChart `Bar` component per timeline bucket, and Stats share
columns as 900 keyed `<rect>`s that were destroyed and recreated per switch.

## After

| Page | Switch | Settled | Long tasks |
|---|---|---|---|
| Overview | All time (cached) | 49 ms | none |
| Overview | Last 4 weeks (cached) | 53 ms | none |
| Overview | Last 4 weeks (from all time, cold) | 49 ms | none |
| Overview | Plays/Time at all time | 15–66 ms | none |
| Stats | All time (cached) | 70 ms | none |
| Stats | Last 4 weeks (from all time) | 49 ms | none |
| Stats | Plays/Time at all time | 38–266 ms¹ | none |
| Top artists | any | 15–65 ms | none |
| Artist page | any | 15–98 ms | none |

¹ Waiting on the network for an uncached query; frames keep flowing (worst
gap 17 ms) and the previous data stays on screen, dimmed.

Worst frame gap after: 33 ms on any switch (one frame skipped), against
50–117 ms before. The remaining cost on Stats' all-time switch (~55 ms of
script in an unminified build, under the long-task threshold minified) is
the screen-reader tables: five tables growing from 28 to 180 rows.

## Re-check after the provenance, banner and error-state work

Same harness, after dd72fb9–2025939 (shell status polling, mix meters,
banners): no long tasks on Overview or Stats; Overview cached switches
48–49 ms, Stats all time cached 74 ms, worst frame gap 33 ms.
