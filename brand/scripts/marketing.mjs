// Builds the iPhone app's marketing images from real demo-mode captures.
//
//   node brand/scripts/marketing.mjs      (repo root; needs Google Chrome and
//                                          web/node_modules for the Archivo font)
//
// Reads brand/marketing/captures/*.png (simulator screenshots of the demo,
// dark appearance, 9:41 status bar) and writes:
//   brand/marketing/app-store/NN-name.png   1320×2868, the 6.9" App Store slot
//   brand/marketing/wide-1920x1080.png      three phones, for a site or a post
//   brand/marketing/social-1200x630.png     Open Graph / social card
// The phone frame is drawn here, not an Apple bezel, so the repo can ship it.
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { launch } from './cdp.mjs';

const BRAND = join(dirname(fileURLToPath(import.meta.url)), '..');
const ROOT = join(BRAND, '..');
const OUT = join(BRAND, 'marketing');

const C = {
	console: '#121317',
	graphite: '#1A1C21',
	grille: '#3A3D46',
	lamp: '#1DB954',
	paper: '#ECEEF1',
	mist: '#A1A5AC',
	aluminium: '#EDEEF0',
	ink: '#16171B',
	slate: '#5B5F68',
	leaf: '#169C45',
	grilleDay: '#CFD1D4'
};

const b64 = (f) => readFileSync(f).toString('base64');
const shot = (name) => `data:image/png;base64,${b64(join(OUT, 'captures', `${name}.png`))}`;
const lockup = (g) =>
	readFileSync(join(BRAND, 'logo', `lockup-${g}.svg`), 'utf8').replace('<svg', '<svg class="lockup"');

const fontFile = join(ROOT, 'web/node_modules/@fontsource-variable/archivo/files/archivo-latin-wdth-normal.woff2');
const FONT = `@font-face{font-family:Archivo;src:url(data:font/woff2;base64,${b64(fontFile)}) format('woff2');font-weight:100 900;font-stretch:62% 125%}`;

// ── Meter strip: the mark's grille run on. Heights are a seeded wave, never real data.
function strip({ cols, p, lit, unlit, seed, rows = 4 }) {
	let s = seed;
	const rnd = () => (s = (s * 16807) % 2147483647) / 2147483647;
	const r = p * 0.393;
	let body = '';
	for (let c = 0; c < cols; c++) {
		const v = 1.5 + 1.35 * Math.sin(c * 0.31 + seed) + 0.75 * Math.sin(c * 0.97 + seed * 2) + (c % 7 >= 5 ? 0.6 : 0) + (rnd() - 0.5) * 0.9;
		const h = Math.max(0, Math.min(rows, Math.round((v * rows) / 4)));
		for (let row = 0; row < rows; row++)
			body += `<circle cx="${(r + c * p).toFixed(1)}" cy="${(r + row * p).toFixed(1)}" r="${r.toFixed(1)}" fill="${rows - 1 - row < h ? lit : unlit}"/>`;
	}
	const w = (cols - 1) * p + 2 * r;
	const h = (rows - 1) * p + 2 * r;
	return `<svg width="${w}" height="${h}" viewBox="0 0 ${w} ${h}" style="display:block">${body}</svg>`;
}

// ── A drawn phone: graphite body, a thin lit edge, the island, the capture.
function phone(src, w, style = '') {
	const pad = w * 0.028;
	const sw = w - 2 * pad;
	const sh = (sw * 2622) / 1206;
	const r = w * 0.15;
	return `<div class="phone" style="width:${w}px;height:${sh + 2 * pad}px;border-radius:${r}px;padding:${pad}px;${style}">
<img src="${src}" style="width:${sw}px;height:${sh}px;border-radius:${r - pad}px">
<i style="top:${pad + sw * 0.028}px;width:${sw * 0.3}px;height:${sw * 0.088}px"></i></div>`;
}

const page = (css, body) => `<!doctype html><html><head><meta charset="utf-8"><style>${FONT}
*{margin:0;box-sizing:border-box}html,body{width:100%;height:100%;overflow:hidden}
body{font-family:Archivo,sans-serif;-webkit-font-smoothing:antialiased;position:relative}
.head{font-stretch:125%;font-weight:760;letter-spacing:-0.035em;line-height:1}
.head g{display:inline;color:var(--lit)}
.phone{position:absolute;background:#0B0C0E;box-shadow:0 0 0 3px #3A3D46,0 60px 140px rgba(0,0,0,.55)}
.phone img{display:block}
.phone i{position:absolute;left:50%;transform:translateX(-50%);background:#000;border-radius:999px}
${css}</style></head><body>${body}</body></html>`;

// ── App Store slides, 1320×2868 ──────────────────────────────────────────
const W = 1320;
const H = 2868;

function slide({ head, sub, screen, layout = 'bottom', light = false, second }) {
	const ground = light ? C.aluminium : C.console;
	const type = light ? C.ink : C.paper;
	const muted = light ? C.slate : C.mist;
	const lit = light ? C.leaf : C.lamp;
	const pw = 1040;
	let device = '';
	if (layout === 'bottom') device = phone(shot(screen), pw, `left:${(W - pw) / 2}px;top:980px`);
	if (layout === 'tilt') device = phone(shot(screen), pw, `left:${(W - pw) / 2 + 70}px;top:1010px;transform:rotate(-5deg)`);
	if (layout === 'pair')
		device =
			phone(shot(second), 900, `left:-120px;top:1160px;transform:rotate(-7deg);opacity:.9`) +
			phone(shot(screen), 960, `left:400px;top:1010px;transform:rotate(4deg)`);
	return page(
		`body{background:${ground};color:${type};--lit:${lit}}
.cap{position:absolute;left:110px;right:110px;top:190px}
.head{font-size:116px}
p{font-size:52px;line-height:1.3;color:${muted};margin-top:44px;letter-spacing:-0.005em}`,
		`<div class="cap"><h1 class="head">${head}</h1>${sub ? `<p>${sub}</p>` : ''}</div>${device}`
	);
}

function heroSlide() {
	return page(
		`body{background:${C.console};color:${C.paper};--lit:${C.lamp}}
.cap{position:absolute;left:110px;right:110px;top:170px}
.lockup{height:74px;display:block;margin-bottom:96px}
.head{font-size:124px}
p{font-size:54px;line-height:1.3;color:${C.mist};margin-top:48px}`,
		`<div class="cap">${lockup('dark')}<h1 class="head">Every play,<br>kept <g>forever</g>.</h1>
<p>Your Spotify stats, on your own server.</p></div>${phone(shot('overview'), 1000, `left:160px;top:1170px`)}`
	);
}

function wallSlide() {
	const items = ['Live capture', 'Lifetime history', 'Every artist, album, track', 'Listening sessions', 'Works offline', 'Read-only, private', 'Free and open source'];
	return page(
		`body{background:${C.console};color:${C.paper};--lit:${C.lamp}}
.cap{position:absolute;left:110px;right:110px;top:190px}
.head{font-size:116px}
ul{list-style:none;margin-top:130px}
li{font-stretch:112%;font-weight:640;font-size:78px;letter-spacing:-0.02em;padding:44px 0;border-top:3px solid ${C.grille};display:flex;align-items:center;gap:40px}
li::before{content:'';width:30px;height:30px;border-radius:50%;background:${C.lamp};flex:none}
.strip{position:absolute;left:110px;bottom:150px}
.lockup{position:absolute;right:110px;bottom:160px;height:64px}`,
		`<div class="cap"><h1 class="head">All yours.<br>On <g>your</g> server.</h1>
<ul>${items.map((t) => `<li>${t}</li>`).join('')}</ul></div>
<div class="strip">${strip({ cols: 14, p: 40, lit: C.lamp, unlit: C.grille, seed: 3 })}</div>${lockup('dark')}`
	);
}

const slides = [
	['01-hero', heroSlide()],
	['02-top', slide({ head: 'Know your<br><g>real</g> top 10.', sub: 'Tracks, artists, albums and genres, ranked by plays or time.', screen: 'top' })],
	['03-artist', slide({ head: 'Go deep on<br>any <g>artist</g>.', sub: 'Rank, all-time plays and your first listen.', screen: 'artist', layout: 'tilt' })],
	['04-sessions', slide({ head: 'Every session,<br><g>remembered</g>.', sub: 'Your history, grouped into the stretches you actually listened.', screen: 'sessions', layout: 'pair', second: 'history' })],
	['05-stats', slide({ head: 'Watch your<br>taste <g>move</g>.', sub: 'Plays by day and artist share, week on week.', screen: 'stats', light: true })],
	['06-library', slide({ head: 'Find the songs<br>you <g>never</g> play.', sub: 'Saved, unplayed and how long songs take to stick.', screen: 'library', layout: 'tilt' })],
	['07-features', wallSlide()]
];

// ── Wide 1920×1080 and social 1200×630 ───────────────────────────────────
function wide(width, height, { phones, headSize, subSize, sub }) {
	const pad = Math.round(width * 0.06);
	const pw = Math.round(height * 0.44);
	const set = phones
		.map((s, i) => {
			const x = width * 0.53 + i * pw * 0.74;
			const y = height * (i === 1 ? 0.08 : 0.17);
			const rot = [-6, 0, 6][i] ?? 0;
			return phone(shot(s), pw, `left:${x}px;top:${y}px;transform:rotate(${rot}deg);z-index:${i === 1 ? 2 : 1}`);
		})
		.join('');
	return page(
		`body{background:${C.console};color:${C.paper};--lit:${C.lamp}}
.cap{position:absolute;left:${pad}px;top:${pad}px;bottom:${pad}px;width:${width * 0.42}px;display:flex;flex-direction:column}
.lockup{height:${Math.round(width * 0.03)}px;align-self:flex-start}
.head{font-size:${headSize}px;margin-top:auto}
p{font-size:${subSize}px;color:${C.mist};line-height:1.35;margin-top:${Math.round(subSize)}px}
.strip{margin-top:auto}`,
		`<div class="cap">${lockup('dark')}<h1 class="head">Spotify stats,<br>in your <g>pocket</g>.</h1><p>${sub}</p>
<div class="strip">${strip({ cols: 16, p: Math.round(width / 60), lit: C.lamp, unlit: C.grille, seed: 5 })}</div></div>${set}`
	);
}

const chrome = await launch();
try {
	mkdirSync(join(OUT, 'app-store'), { recursive: true });
	for (const [name, html] of slides) {
		writeFileSync(join(OUT, 'app-store', `${name}.png`), await chrome.render(html, { width: W, height: H }));
		console.log(`app-store/${name}.png`);
	}
	writeFileSync(
		join(OUT, 'wide-1920x1080.png'),
		await chrome.render(wide(1920, 1080, { phones: ['top', 'overview', 'artist'], headSize: 84, subSize: 32, sub: 'Your self-hosted listening history and stats on every artist, album and track, now on iPhone.' }), { width: 1920, height: 1080 })
	);
	writeFileSync(
		join(OUT, 'social-1200x630.png'),
		await chrome.render(wide(1200, 630, { phones: ['top', 'overview', 'artist'], headSize: 52, subSize: 21, sub: 'Your self-hosted listening history, on iPhone.' }), { width: 1200, height: 630 })
	);
	console.log('wide-1920x1080.png, social-1200x630.png');
} finally {
	await chrome.close();
}
