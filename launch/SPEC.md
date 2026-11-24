# Launch video: design spec (draft 2, "Power on")

Draft 1 (`src/LaunchV1.tsx`, composition `LaunchV1`) was rejected as boring and phone-first. Draft 2 makes the desktop app the star and the phone a short coda.

## Concept

The desktop app boots like a hi-fi amp. Built from the hero reel's desktop captures (`motion/public/1440`, lit and unlit pairs with every lamp's position), so every lamp that lights is the app's own pixels.

## Music

`bgm-power` take D (ElevenLabs, `public/audio/bgm-power.mp3`). 128 BPM, first beat 0.434 s (`BEAT0 = 13.02 f`, one beat = 14.0625 f). Quiet build to the drop at beat 30 (14.49 s); full energy to beat 70 (33.25 s); a lower tail to the end.

## Storyboard (beats)

| # | Beats | Shot | Motion | Sound |
|---|---|---|---|---|
| 1 | 0–8 | Self-test: macro on each Overview meter | Each meter sweeps on and its figure counts; the camera hops meter to meter (gauge-readout-moves, adapted to lamp meters) | Tube hum swell, cascade and click per meter |
| 2 | 8–14 | "Every / play, / kept / forever." | One word per beat, then the full line pushes in | Relay click per word |
| 3 | 14–30 | Crane: plays-by-day peak to the full dashboard | crane-rise-reveal; every lamp lights as it enters view; pushes in through the riser | Cascades, riser |
| 4 | 30–38 | Drop: five lit pages as a train past a 60° wall | steep-tilt-glide with speed ghost; flash | Impact (slam 1 of 3), long whoosh |
| 5 | 38–46 | Top: rank bars sweep, then a 6-frame crash zoom onto the #1 row; a giant "#1" slams in | crash-zoom-punch (bounce) | Selector click, whoosh, impact (slam 2) |
| 6 | 46–54 | Artist: chart climbs, listening clock lights around the dial; console label | Slow push | Selector, cascade, a click a beat |
| 7 | 54–62 | Sessions: tabletop, scrolling, meters light in a wave; caption types out | Tilted plane | Selector, cascade, a click a beat |
| 8 | 62–70 | Stats: lateral dolly, columns climb; "taste move." slams in the centre | Dolly | Selector, cascade, small hit |
| 9 | 70–76 | Pocket: desktop steps back into a window, the phone slides out | | Tape stop, whoosh, tap |
| 10 | 76–end | Lockup: the mark ignites, word slides out, tagline, meter strip, footer | | Impact (slam 3), clicks, cascade, shimmer |

Between pages, the amp's source selector (OVERVIEW · TOP · ARTIST · SESSIONS · STATS) clicks over and the cut lands on the beat.

## Rules kept

Brand palette and type; green only on lamps and one word per caption. Full-frame hits limited to three (drop, crash zoom, lockup). Cues pin the sound's peak to the target frame (output offset measured 0 f). Two renders: with and without music.

## Review

A fresh-context reviewer checked an earlier render of this draft. Fixed from that review: figures disappearing when pages lit fully, one caption template repeated five times (now five styles), the dashboard settling too small, silent Artist and Sessions shots, a weak crash zoom, an overlong lockup, a near-silent opening, the music dipping before the phone cut. Not done: the meters pulsing with the music, and a fly-through drop.

## Known gaps

- Sound is picked and mixed by measurement; nobody has listened yet.
- Loudness is about −17 dB mean; social platforms sit near −14 LUFS.
