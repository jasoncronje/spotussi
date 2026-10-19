// Captures the mock app's pages for the hero reel, each twice: "lit" (as the
// app draws it) and "dark" (every lamp, meter and bar switched off), plus the
// page geometry of each lamp so the video can light them one at a time.
//
//   portless run --name spotussi-mock ./bin/spotussi serve --mock   (repo root)
//   node scripts/capture.mjs [width] [https://spotussi-mock.localhost] [stop,stop]   (motion/)
//
// Run at 1440 (the desktop layout) and 390 (the phone layout). For each stop
// of the tour writes public/<width>/<stop>-lit.png, <stop>-dark.png (and, on
// the phone layout, <stop>-tabbar.png) and src/captures/<width>-<stop>.json.
// Where a stop's summary figures count up in the video, they are hidden in
// both captures and the video sets them in type.
import { mkdirSync, writeFileSync } from 'node:fs';
import { open, sleep } from './cdp.mjs';
import { intercept } from './realdata.mjs';

// The mock's certificate comes from the local portless CA.
process.env.NODE_TLS_REJECT_UNAUTHORIZED = '0';

const WIDTH = Number(process.argv[2] || 1440);
const base = process.argv[3] || 'https://spotussi-mock.localhost';
const PHONE = WIDTH < 600;
const DPR = PHONE ? 3 : 2.5;

// The tour, in order. `depth` is how far down the page the reel ever looks
// (CSS pixels); the capture stops there. `figures` sets the summary figures in
// type so they can count. The artist is whoever tops the Top artists list.
const stops = [
  { key: 'overview', path: '/', depth: PHONE ? 2600 : 2430, figures: true },
  { key: 'top', path: '/top/artists', depth: PHONE ? 2000 : 1500 },
  { key: 'artist', path: null, depth: PHONE ? 2400 : 1606, figures: true },
  { key: 'history', path: '/history', depth: PHONE ? 2000 : 1700 },
  { key: 'sessions', path: '/sessions', depth: PHONE ? 2000 : 1500 },
  { key: 'stats', path: '/stats', depth: PHONE ? 2000 : 1100 },
];

mkdirSync(`public/${WIDTH}`, { recursive: true });
mkdirSync('src/captures', { recursive: true });
const b = await open({ width: WIDTH, height: PHONE ? 844 : 900, dpr: DPR, mobile: PHONE });
let artist = null;
// Real artists, albums and tracks in place of the mock's invented ones.
const real = await intercept(b, base);
// The first load is cold; open the app once before the tour.
await b.send('Page.navigate', { url: base + '/' });
await sleep(5000);

// An optional comma list of stops captures just those (the artist needs top).
const only = process.argv[4]?.split(',');
for (const stop of stops) {
  if (only && !only.includes(stop.key) && !(stop.key === 'top' && only.includes('artist'))) continue;
  await b.send('Page.navigate', { url: base + (stop.path ?? artist) });
  await sleep(4500);

  // Mock mode runs without sign-in and says so in a banner the real app never shows.
  await b.evaluate(`(() => {
    const s = [...document.querySelectorAll('span')].find((e) => e.textContent.trim() === 'No sign-in required.');
    let el = s;
    while (el && el.parentElement && el.parentElement.getBoundingClientRect().height < 120) el = el.parentElement;
    el?.remove();
  })()`);
  await sleep(600);

  // Scroll through once so lazy artwork loads, then back to the top.
  await b.evaluate(`(async () => {
    for (let y = 0; y < ${stop.depth}; y += innerHeight / 2) { scrollTo(0, y); await new Promise((r) => setTimeout(r, 120)); }
    scrollTo(0, 0);
    const loads = Promise.all([...document.images].map((i) => i.complete || new Promise((r) => { i.onload = i.onerror = r; })));
    await Promise.race([loads, new Promise((r) => setTimeout(r, 4000))]);
  })()`);
  await sleep(800);

  // The top list's first row: where the reel zooms in before it opens the artist.
  const row = await b.evaluate(`(() => {
    const a = document.querySelector('main a[href^="/artists/"]');
    if (!a) return null;
    let el = a;
    while (el.parentElement && el.parentElement.getBoundingClientRect().height < 80) el = el.parentElement;
    const r = el.getBoundingClientRect();
    return { href: a.getAttribute('href'), x: r.x + scrollX, y: r.y + scrollY, w: r.width, h: r.height };
  })()`);
  if (stop.key === 'top') artist = row.href;

  // Where the reel's cursor clicks: the app's nav links (the header's on the
  // desktop, the tab bar's on the phone), in page pixels at the top of the page.
  const nav = await b.evaluate(`[...document.querySelectorAll('header a[href], nav a[href]')].map((a) => {
    const r = a.getBoundingClientRect();
    const fixed = !!a.closest('nav.fixed');
    return { label: a.textContent.trim(), href: a.getAttribute('href'), fixed, x: r.x, y: r.y + (fixed ? 0 : scrollY), w: r.width, h: r.height };
  }).filter((l) => l.w > 0 && l.h > 0)`);

  // The phone layout's tab bar is fixed to the bottom of the screen: capture it
  // on its own so the video can pin it there, and leave it out of the page.
  const tabbar = await b.evaluate(`(() => {
    const el = [...document.querySelectorAll('nav, div')].find((e) => {
      const r = e.getBoundingClientRect();
      return getComputedStyle(e).position === 'fixed' && r.bottom >= innerHeight - 1 && r.height > 30 && r.height < 120 && r.width >= innerWidth - 1;
    });
    if (!el) return null;
    el.setAttribute('data-reel-tabbar', '');
    const r = el.getBoundingClientRect();
    return { x: r.x, y: r.y, w: r.width, h: r.height };
  })()`);
  if (tabbar) {
    writeFileSync(`public/${WIDTH}/${stop.key}-tabbar.png`, await b.shot({ x: tabbar.x, y: tabbar.y, width: tabbar.w, height: tabbar.h }));
    // A style rule, not an inline style: some pages redraw the bar on resize.
    await b.evaluate(`(() => {
      const s = document.createElement('style');
      s.textContent = '[data-reel-tabbar], nav.fixed.bottom-0 { display: none !important; transition: none !important; }';
      document.head.append(s);
    })()`);
  }

  // The range bar sticks to the top of the screen once the header scrolls
  // away: capture it lit, on its own, so the phone cut can pin it.
  const rangebar = await b.evaluate(`(() => {
    const el = [...document.querySelectorAll('main *, body > div *')].find((e) => getComputedStyle(e).position === 'sticky' && parseFloat(getComputedStyle(e).top) === 0);
    if (!el) return null;
    const r = el.getBoundingClientRect();
    return { x: r.x, y: r.y + scrollY, w: r.width, h: r.height };
  })()`);

  const geometry = await b.evaluate(`(() => {
    const box = (e) => { const r = e.getBoundingClientRect(); return { x: r.x + scrollX, y: r.y + scrollY, w: r.width, h: r.height }; };
    const round = (o) => Object.fromEntries(Object.entries(o).map(([k, v]) => [k, typeof v === 'number' ? Math.round(v * 100) / 100 : v]));
    const depth = Math.min(${stop.depth}, Math.ceil(document.documentElement.scrollHeight));
    const page = { w: ${WIDTH}, h: depth };
    const within = (r) => r.y < depth;

    // Summary figures: the number, its unit, and the type they are set in.
    const figures = ${stop.figures ? 'true' : 'false'} ? [...document.querySelectorAll('.figure')].filter((e) => e.closest('section, [class*="rounded-xl"]') && /\\d/.test(e.textContent) && !/AM|PM/.test(e.textContent)).map((e) => {
      const cs = getComputedStyle(e);
      // Each run of type in order: spans in their own style, bare text in the figure's.
      const spans = [...e.childNodes].filter((n) => n.textContent.trim()).map((n) => {
        const ss = n.nodeType === 1 ? getComputedStyle(n) : cs;
        return { text: n.textContent.trim(), size: ss.fontSize, weight: ss.fontWeight, color: ss.color, spacing: ss.letterSpacing, marginLeft: n.nodeType === 1 ? ss.marginLeft : '0px' };
      });
      return {
        ...round(box(e)), spans,
        font: { size: cs.fontSize, weight: cs.fontWeight, stretch: cs.fontStretch, spacing: cs.letterSpacing, color: cs.color, features: cs.fontFeatureSettings },
      };
    }).filter(within) : [];

    // Every lit element: dots and lamps (circles, spans) and horizontal bars.
    const lamps = [];
    const bars = [];
    for (const e of document.querySelectorAll('.fill-signal, .bg-signal')) {
      if (e.closest('nav, header')) continue;
      if (e.tagName.toLowerCase() === 'path') {
        // A plays-by-day chart: one path, a rectangle per bucket, cut into
        // lamp segments by a mask counted up from the baseline.
        const m = e.getScreenCTM();
        const pattern = e.closest('svg').querySelector('pattern');
        if (!pattern) continue;
        const pitch = Number(pattern.getAttribute('height')) * m.d;
        const re = /M([\\d.]+) ([\\d.]+)V([\\d.]+)h([\\d.]+)V/g;
        let g;
        while ((g = re.exec(e.getAttribute('d')))) {
          const [x, y0, y1, w] = g.slice(1).map(Number);
          const bar = round({ x: m.a * x + m.e + scrollX, base: m.d * y0 + m.f + scrollY, top: m.d * y1 + m.f + scrollY, w: m.a * w, pitch });
          if (bar.base < depth) bars.push(bar);
        }
        continue;
      }
      const r = box(e);
      if (r.w < 0.5 || r.h < 0.5 || !within(r)) continue;
      const kind = r.w > r.h * 3 ? 'bar' : 'dot';
      const clock = e.closest('svg') && e.closest('svg').querySelectorAll('circle').length > 100;
      lamps.push(round({ ...r, kind, clock: !!clock }));
    }

    const clockSvg = [...document.querySelectorAll('svg')].find((s) => s.querySelectorAll('circle').length > 100);
    const headings = Object.fromEntries([...document.querySelectorAll('h1, h2, h3')].map((h) => {
      const panel = h.closest('[class*="rounded-xl"], section') || h;
      return [h.textContent.trim().replace(/^Overview,?/, ''), { heading: round(box(h)), panel: round(box(panel)) }];
    }));
    return { page, figures, lamps, bars, clock: clockSvg ? round(box(clockSvg)) : null, headings };
  })()`);

  // Figures are set in type by the video, so neither capture carries them.
  if (stop.figures) {
    await b.evaluate(`document.querySelectorAll('.figure').forEach((e) => { if (e.closest('section, [class*="rounded-xl"]') && /\\d/.test(e.textContent) && !/AM|PM/.test(e.textContent)) { e.style.color = 'transparent'; e.querySelectorAll('*').forEach((c) => (c.style.color = 'transparent')); } })`);
    await sleep(300);
  }
  // Hide the tab bar again right before the shots: a transition on the bar
  // outranks even an important style, so it goes from the layout too.
  const hideTabbar = () => b.evaluate(`document.querySelectorAll('[data-reel-tabbar], nav.fixed.bottom-0').forEach((e) => { e.style.setProperty('transition', 'none', 'important'); e.style.setProperty('display', 'none', 'important'); })`);
  await hideTabbar();
  const clip = { x: 0, y: 0, width: WIDTH, height: geometry.page.h };
  writeFileSync(`public/${WIDTH}/${stop.key}-lit.png`, await b.shot(clip));
  if (PHONE && rangebar) writeFileSync(`public/${WIDTH}/${stop.key}-rangebar.png`, await b.shot({ x: rangebar.x, y: rangebar.y, width: rangebar.w, height: rangebar.h }));

  await b.evaluate(`document.querySelectorAll('.fill-signal, .bg-signal').forEach((e) => { if (!e.closest('nav, header')) e.style.opacity = '0'; })`);
  await hideTabbar();
  await sleep(300);
  writeFileSync(`public/${WIDTH}/${stop.key}-dark.png`, await b.shot(clip));

  const { href, ...firstRow } = row ?? {};
  writeFileSync(`src/captures/${WIDTH}-${stop.key}.json`, JSON.stringify({ dpr: DPR, tabbar, firstRow: row ? firstRow : null, nav, rangebar: PHONE ? rangebar : null, ...geometry }, null, 1));
  console.log(`${stop.key}: ${geometry.page.w}x${geometry.page.h}, nav ${nav.map((l) => l.label).join('|')}, ${geometry.figures.length} figures, ${geometry.lamps.length} lamps, ${geometry.bars.length} chart columns`);
}
if (real.unknown.size) console.log('names left as they were under:', [...real.unknown].join(', '));
b.close();
