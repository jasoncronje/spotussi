# spotussi brand kit

The mark is the app's own level meter: a 4×4 grille of lamps, lit from the bottom to heights 2·4·3·1, in Spotify Green on graphite. It is drawn from straight columns of dots, with no circles or waves, so it can't be mistaken for a streaming service's logo.

![The kit: lockups, marks, icons, social images](previews/sheet.png)

## Files

| Use | File |
| --- | --- |
| App icon, avatars, anywhere the ground is unknown | `logo/mark.svg` (lamps on a graphite plate) |
| Mark on a dark ground | `logo/mark-dark.svg` |
| Mark on a light ground | `logo/mark-light.svg` |
| Name with the mark | `logo/lockup-dark.svg`, `logo/lockup-light.svg` |
| One colour (print, engraving, embossing) | `logo/*-mono-black.svg`, `logo/*-mono-white.svg` |
| Name alone, when the mark is already nearby | `logo/wordmark-*.svg` |
| Browser and home-screen icons | `web/` (copied into `web/static`) |
| Link previews | `social/og-1200x630.png` (Open Graph), `social/github-social-1280x640.png` (GitHub social preview) |
| README header | `social/readme-header-light.png`, `social/readme-header-dark.png` |

Every file comes from `scripts/build.mjs`. The wordmark is outlined from Archivo, so no SVG needs the font.

## Clear space and size

- **Clear space:** keep one lamp pitch, the distance between two lamp centres, clear on every side of the mark and the lockup. That is about a quarter of the mark's height.
- **Smallest sizes:** the lockup at 20 px tall, the bare mark at 12 px, the plated mark at 16 px (the `favicon.ico` 16 px frame is redrawn on whole pixels for that size).
- **Lockup geometry:** the mark is 2.2 x-heights of the wordmark tall, centred on the x-height, and 0.42 of its own height from the word.

## Colours

| Name | Hex | Role | App token |
| --- | --- | --- | --- |
| Console | `#121317` | Dark ground, `theme-color` | `--background` (dark) |
| Graphite | `#1A1C21` | The plate, dark panels | `--card` (dark) |
| Grille | `#3A3D46` | Unlit lamps on dark grounds | `--grille` (dark) |
| Lamp | `#1DB954` | Lit lamps on dark grounds | `--signal` (dark) |
| Aluminium | `#EDEEF0` | Light ground | `--background` (light) |
| Grille, day | `#CFD1D4` | Unlit lamps on light grounds | `--grille` (light) |
| Leaf | `#169C45` | Lit lamps on light grounds | `--signal` (light) |
| Ink | `#16171B` | Type on light grounds | `--foreground` (light) |
| Paper | `#ECEEF1` | Type on dark grounds | `--foreground` (dark) |

Green is the only colour. It marks something lit: a lamp, a value. Everything else is graphite, aluminium and their greys.

## Type

Archivo, one family, using its width axis. The wordmark is Archivo at 125% width, weight 760, tracking −0.035 em, always lowercase. Headlines use the same cut; running text uses the normal width.

## Do

- Use `mark.svg` when you don't control the ground. The plate keeps it legible on white, black and photos.
- Use the `-light` files on light grounds and the `-dark` files on dark grounds. The green shifts from lamp to leaf so the lit lamps keep their contrast.
- Carry the grille on as a meter strip (see the social images) when a layout needs a graphic. Keep the mark's lamp pitch and rows, and use made-up heights, never real listening data.

## Don't

- Change the lit pattern, the number of lamps, or the lamps' shape.
- Recolour the lamps, add a second accent, or put the mark on a green ground.
- Set "spotussi" in another typeface, in capitals, or with the first letter capitalised.
- Stretch, rotate, outline, shadow or add a glow to the mark.
- Pair the mark with a third party's logo, or imply an affiliation with Spotify.

## Rebuilding

```sh
node brand/scripts/build.mjs
```

It needs Google Chrome (set `CHROME` if it isn't in `/Applications`) and `web/node_modules` for the Archivo font, and copies the web icons into `web/static`. *spotussi* is a working name; to change the wordmark, re-outline it and rebuild:

```sh
uv run --with fonttools --with brotli --with uharfbuzz python brand/scripts/outline.py \
  web/node_modules/@fontsource-variable/archivo/files/archivo-latin-wdth-normal.woff2 \
  760 125 -35 newname > brand/src/wordmark.json
```

`previews/mark-directions.png` records the three directions the mark was chosen from (Ladder, chosen; Peak hold; Matrix s).

## iPhone marketing images

`brand/marketing/` holds the iPhone app's marketing images, built by `node brand/scripts/marketing.mjs` from real demo-mode captures (dark appearance, Try the demo, status bar at 9:41): `brand/marketing/captures/` from the iPhone 18 Pro simulator, `captures/duo-inner/` and `captures/duo-outer/` from the iPhone Duo simulator (iOS 27.1) unfolded and folded, both in portrait. `node brand/scripts/marketing.mjs duo` rebuilds only the Duo sets; `iphone`, `duo-inner`, `duo-outer` and `wide` pick single sets.

| File | Use |
| --- | --- |
| `marketing/app-store/01…07-*.png` | App Store screenshots, 1320×2868 (6.9" slot) |
| `marketing/app-store/duo-inner/01…07-*.png` | App Store screenshots, iPhone Duo slot, inner display, 2007×2853 portrait |
| `marketing/app-store/duo-outer/01…07-*.png` | App Store screenshots, iPhone Duo slot, outer display, 1398×2034 portrait |
| `marketing/wide-1920x1080.png` | Site hero, posts, slides |
| `marketing/social-1200x630.png` | Open Graph / social card for the app |

The phone frames are drawn in the script, not taken from Apple's bezels, so it can live in this repo. To refresh, re-capture the screens with the same names and run the script again.

The Duo sets keep the same seven screens, headlines and order. The app is iPhone-only, but on the Duo it fills both displays at their own size (669×951 points inner, 466×678 outer) rather than running letterboxed, so the captures show it as it runs. The Duo frame uses the simulator's own corner radii (55 points all round inside; 8 on the hinge side and 59 on the other outside), draws a faint crease across the inner panel and a hinge spine down the outer panel's left edge. Screenshots leave the camera out, so the script draws the outer panel's punch-hole where the simulator shows it; the inner panel has none. To capture: open the Duo, `fold open` (or `fold closed`) and `orientation portrait` with agent-device, `simctl status_bar <udid> override --time 9:41`, then take each screen at `--pixel-density 3`.
