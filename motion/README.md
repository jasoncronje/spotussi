# motion

The spotussi.app hero video, made with [Remotion](https://www.remotion.dev). It is a 21 s loop of someone using the app. A cursor clicks the app's own nav from the Overview to Top artists, opens the artist who tops it by their row, then History, Sessions and Stats, and clicks Overview to loop (3.5 s a page). Each page swaps in on the click: its contents rise and settle into place, push in slowly while it holds, and power on (figures count up, meters, rank bars, dot trains and chart columns light). On the phone cut a fingertip taps the tab bar instead, and a thumb scrolls each page once, down to what its caption is about.

The window never moves (the page's contents do, after a click): camera zooms, pans and slides were tried and rejected, and so was a cut where nothing moved but the lamps (see D31 in `docs/project/decisions.md`). Every movement needs a visible cause: the pointer, or the scroll it makes.

There are two cuts. The wide cut (1800x1200) is the desktop layout in a flat app window (`src/Window.tsx`); it is 3:2, the window's own shape, so the window fills the frame. The phone cut (1080x1350) is the app's own phone layout at full width, with its tab bar pinned to the bottom of the frame. The posters are the Overview lit, before the pointer leaves it (3 s).

## How it is made

`scripts/capture.mjs <width>` opens each page of the mock server in headless Chrome. Run it at 1440 (desktop, 2.5x) and at 390 (phone, 3x). For each stop it writes:

- `public/<width>/<stop>-lit.png`: the page as the app draws it.
- `public/<width>/<stop>-dark.png`: the same page with every lamp, meter and bar switched off.
- `src/captures/<width>-<stop>.json`: where each lamp sits, the charts' columns (read from the SVG path), the nav links, the first row of a list and the summary figures' type.
- `public/<width>/<stop>-tabbar.png` and `<stop>-rangebar.png`: the phone layout's fixed tab bar and sticky range bar, captured on their own.

The capture swaps the mock's invented names and placeholder artwork for real music as the browser receives them (`scripts/realdata.mjs`). The catalog, `scripts/catalog.json`, comes from Deezer's public API: run `node scripts/fetch-catalog.mjs` to rebuild it and to cache the artist photos and album covers in `.cache/img` (not committed; the capture needs them). The mock's top artists take the catalog in rank order, so the reel opens Drake's page.

The composition lays pieces of the lit capture over the dark one, so every lamp that lights is the app's own pixels. On the Overview and the artist page the figures are hidden in both captures and set in Archivo (`public/archivo-wdth.woff2`, from `web/node_modules`) so they can count.

## Commands

From the repo root, start the mock (the capture expects `https://spotussi-mock.localhost`; pass another address as the second argument, such as a worktree-prefixed one):

```bash
portless run --name spotussi-mock ./bin/spotussi serve --mock
```

Then, in `motion/`:

```bash
npm i
node scripts/fetch-catalog.mjs
node scripts/capture.mjs 1440
node scripts/capture.mjs 390
portless run --name spotussi-motion --app-port 3210 npx remotion studio --no-open --port 3210
```

A third argument recaptures only some stops, for example `node scripts/capture.mjs 390 https://spotussi-mock.localhost overview,artist`.

Render into the site with `npm run render`. It writes `site/assets/hero-reel{,-phone}.mp4` and the posters.

The tour lives in `src/Tour.tsx`: each page's start, lamp times, what is clicked to leave it and how far the phone scrolls; `src/Pointer.tsx` draws the cursor and fingertip. Its `beats` and `HOME` are the site's caption times in `site/main.js`, so change them together.

## Third-party artwork

The album covers and artist photos in the captures (`public/**`) and the rendered reel come from Deezer's public API. They are © their labels, artists and photographers, are shown only as sample data, and are not covered by spotussi's AGPL-3.0 licence. They are listed in `site/assets/credits.json`.
