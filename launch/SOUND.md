# Launch video: sound palette

Direction: an analog hi-fi console, matching the level-meter brand. Relay clicks light the lamps, detent knobs switch ranges, a flip counter rolls the figures, tape spins up into the logo and stops on hard cuts. Warm and mechanical; no synthetic UI bleeps.

All audio is generated with ElevenLabs (SFX: `eleven_text_to_sound_v3`; music: `eleven_music_v2_5`) on the flow "Spotussi launch video: sound palette". Four takes per sound.

Audition and pick: open `public/palette.html` in a browser. Takes live in `public/audio/palette/` (not committed until picked; `palette.json` lists durations and peaks).

| Role | Key | Use |
| --- | --- | --- |
| Music | `bgm-electro` | Minimal electronic, 120 BPM, builds to a drop and a final hit |
| Music | `bgm-house` | Hi-fi deep house, 118 BPM, Rhodes and tape warmth |
| Brand | `lamp-click` | One meter lamp lights |
| Brand | `lamp-cascade` | A meter strip or chart fills |
| Brand | `knob` | Range switch or dial |
| Brand | `counter` | Figures count up |
| Motion | `whoosh` | Phone or card crosses the frame |
| Motion | `glass-swipe` | Thumb scroll (takes A and B are near-silent) |
| Motion | `glass-tap` | Tap on a tab or row (take D is near-silent) |
| Motion | `tape-stop` | Hard cut or scene end |
| Accent | `riser` | Build into the logo or the drop |
| Accent | `impact` | Wordmark lands (3 uses at most) |
| Accent | `shimmer` | Afterglow after the logo lands |

The music prompts asked for no vocals, but the generations ran with the instrumental flag off, so check each music take for stray vocals before picking.
