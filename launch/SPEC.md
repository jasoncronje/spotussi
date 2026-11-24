# Launch video: design spec (draft 1)

Mode: autonomous (video-shotcraft pipeline; the owner said "take your picks and make the first draft").

## Brief

- **What:** spotussi for iPhone, a self-hosted Spotify stats app. Free and open source (AGPL).
- **For:** people who self-host and care about their listening data; posted on the site and social.
- **Must show:** Overview, Top, Artist, Sessions, Stats, Library; the self-hosted, offline and open-source promise.
- **Format:** 1920×1080, 30 fps, 43 s, English, no voice-over. Two renders: with music and without (`props-nobgm.json`).
- **Data:** only the demo-mode simulator captures in `brand/marketing/captures` (invented names, no real listening data). Meter strips use seeded heights.
- **Copy:** every line comes from the approved App Store, wide and social images in `brand/scripts/marketing.mjs`; nothing claims App Store availability.

## Look and motion

Brand kit as-is: Console ground, Archivo at 125% width weight 760, green as the only accent (one lit word per headline), lowercase lockup, the drawn phone from `marketing.mjs`.

Motion voice: "professional and trusted" preset. Entrances 18–24 f ease-out, no bounce; the only overshoot is the outro phones landing (bezier 0.34, 1.4, 0.44, 1). Lamps light with a short incandescent flash, like the hero reel's `Lamps.tsx`.

Deliberate departures from the cards: no gold dust, stage light or glow in the outro (the brand forbids glow and a second accent), and the beat-cut flash is used once, on the drop, not as a three-flash series.

## Music

`bgm-electro` take B (ElevenLabs). Measured 120.0 BPM, first beat 0.526 s (`BEAT0 = 15.78 f`, one beat = 15 f). Lighter 0–16.5 s (only about 4 dB below the drop), drop at beat 32 (16.5 s), a thinner section at beats 48–56 (not quieter), second high 56–72, ends about 42.5 s. A Whisper pass returned only its usual hallucination on instrumentals, so no vocals were found.

## Storyboard

| # | Beats (frames) | Shot | Key motion | Sound |
|---|---|---|---|---|
| 1 | 0–12 (0–196) | Open: the mark lights lamp by lamp, the word slides out, "Every play, kept forever." | Lamps on the off-beats; blur-slide | Relay click per lamp, soft whoosh |
| 2 | 12–20 (196–316) | Hero: Overview phone rises; "Spotify stats, in your pocket." | contact-shadow-lift (floor shadow stays behind) | Whoosh |
| 3 | 20–26 (316–406) | Top: "Know your real top 10." | Phone glides in, blur by speed | Whoosh |
| 4 | 26–32 (406–496) | Artist: "Go deep on any artist." | Same, slight tilt | Whoosh, riser into the drop |
| 5 | 32–37 (496–571) | Drop, Sessions: "Every session, remembered." | Hard cut on the beat, one white flash, shutter settle | Impact (slam 1 of 2) |
| 6 | 37–42 (571–646) | Stats: "Watch your taste move." | Hard cut, shutter settle | Knob |
| 7 | 42–48 (646–736) | Library: "Find the songs you never play." | Hard cut, shutter settle | Glass tap, tape stop |
| 8 | 48–56 (736–856) | Breakdown: "All yours. On your server." and five promises | One line and one lamp per beat | Relay click per line |
| 9 | 56–end (856–1290) | Outro: five phones fly in, lockup stamps at beat 60, "Spotify stats, self-hosted.", meter strip fills; fade from beat 76 | outro-group-photo-launch adapted; crane 4° → 0 | Long whoosh, impact (slam 2), lamp cascade, shimmer |

## Sound

Picks (from `public/palette.html`, chosen by measurement, not by ear): lamp-click C and A alternating, lamp-cascade B, knob B, whoosh A, C and B, glass-tap B, tape-stop B, riser B, impact B, shimmer B. Each cue pins the file's own energy peak to its target frame. The output track offset measured 0 f on this pipeline (two impact probes). SFX files are normalized to a −1 dBFS sample peak (gains recorded in the commit), and the music ducks about 5 dB under every cue. Mix: −3.3 dB peak, −19.1 dB mean.

## Independent review (draft 1)

A fresh-context reviewer found 11 issues; fixed: the knob cue never reached its peak (file trimmed, cue moved onto Stats); cues buried under the music (normalized files, ducking); four seams that popped to a blank frame (exits finish before each cut, entrances pre-rolled); Top missing from the outro (six phones now); meter strip early, off-centre and touching the phones; full-frame hits on every drop cut (now only the phone settles, the flash stays on the first hit); three unapproved copy lines; green full stops; phones exiting over captions (they now leave to the right). Kept: the lamp's brief brightness flash, which the hero reel also uses.

## Known gaps in draft 1

- No hand-listening has been done; the sound picks are by measurement only.
- Loudness is about −18 LUFS; social platforms sit near −14.
- The workbench manifest (`src/workbench.ts`) is not written yet.
