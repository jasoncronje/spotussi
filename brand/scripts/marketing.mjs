// Builds the iPhone app's marketing images from real demo-mode captures.
//
//   node brand/scripts/marketing.mjs          (repo root; needs Google Chrome
//   node brand/scripts/marketing.mjs duo       and web/node_modules for the
//                                              Archivo font)
//
// With no argument it builds every set; name sets (iphone, duo-inner,
// duo-outer, duo for both Duo panels, wide) to build only those.
//
// Reads simulator screenshots of the demo (dark appearance, 9:41 status bar)
// and writes:
//   brand/marketing/app-store/NN-name.png            1320×2868, the 6.9" slot,
//                                                     from captures/*.png
//   brand/marketing/app-store/duo-inner/NN-name.png  2007×2853, iPhone Duo
//                                                     inner display, from
//                                                     captures/duo-inner/*.png
//   brand/marketing/app-store/duo-outer/NN-name.png  1398×2034, iPhone Duo
//                                                     outer display, from
//                                                     captures/duo-outer/*.png
//   brand/marketing/wide-1920x1080.png      three phones, for a site or a post
//   brand/marketing/social-1200x630.png     Open Graph / social card
// The device frames are drawn here, not Apple bezels, so the repo can ship them.
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
const capture = (dir) => (name) => `data:image/png;base64,${b64(join(OUT, dir, `${name}.png`))}`;
const shot = capture('captures');
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

// ── A drawn iPhone Duo panel. The corner radii are the simulator's own
// (capabilities.plist, in points, upper-left clockwise): the inner display is
// 669 points wide with 55-point corners all round; the outer is 466 wide with
// tight 8-point corners on the hinge side and 59-point ones on the other. The
// outer capture already carries its camera, so nothing is drawn over it. The
// inner panel shows the fold as a faint crease across its middle (portrait
// turns the book-style fold on its side), and a thicker hinge spine runs down
// the outer panel's left edge.
const DUO = {
	inner: { cw: 2007, ch: 2853, pts: 669, corners: [55, 55, 55, 55], bezel: 0.02, crease: true },
	outer: { cw: 1398, ch: 2034, pts: 466, corners: [8, 59, 59, 8], bezel: 0.024, spine: true }
};

function duo(src, w, style, dev) {
	const pad = w * dev.bezel;
	const spine = dev.spine ? pad * 0.8 : 0;
	const sw = w - 2 * pad - spine;
	const sh = (sw * dev.ch) / dev.cw;
	const screen = dev.corners.map((c) => (c / dev.pts) * sw);
	const radii = (add) => screen.map((r) => `${(r + add).toFixed(1)}px`).join(' ');
	const crease = dev.crease
		? `<b style="left:${pad}px;right:${pad}px;top:${pad + sh / 2 - sw * 0.012}px;height:${sw * 0.024}px"></b>`
		: '';
	return `<div class="phone duo" style="width:${w}px;height:${sh + 2 * pad}px;border-radius:${radii(pad)};padding:${pad}px ${pad}px ${pad}px ${pad + spine}px;${style}">
<img src="${src}" style="width:${sw}px;height:${sh}px;border-radius:${radii(0)}">${crease}</div>`;
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

// ── App Store slides ─────────────────────────────────────────────────────
// One layout, sized per set: `g` holds every measure, `g.device` draws the
// phone and `g.shot` loads a capture by name.

const IPHONE = {
	W: 1320,
	H: 2868,
	shot,
	device: phone,
	css: '',
	cap: { side: 110, top: 190, head: 116, sub: 52, gap: 44 },
	bottom: { w: 1040, top: 980 },
	tilt: { w: 1040, dx: 70, top: 1010, rot: -5 },
	pair: { back: { w: 900, left: -120, top: 1160, rot: -7 }, front: { w: 960, left: 400, top: 1010, rot: 4 } },
	hero: { top: 170, lockup: 74, lockupGap: 96, head: 124, sub: 54, gap: 48, phone: { w: 1000, left: 160, top: 1170 } },
	wall: { top: 130, item: 78, pad: 44, rule: 3, dot: 30, dotGap: 40, stripBottom: 150, cols: 14, p: 40, lockup: 64, lockupBottom: 160 }
};

// The Duo panels are near 7:10, so the phone is wider and shorter than on
// the 6.9" slide and the type is set a little smaller against the width. The
// measures are for the inner display's 2007 px and scale with the width, so
// the outer display's 1398 px gets the same page.
function duoSet(W, H, dev, dir) {
	const u = W / 2007;
	const s = (n) => Math.round(n * u * 10) / 10;
	const at = (o) => Object.fromEntries(Object.entries(o).map(([k, v]) => [k, typeof v === 'number' && k !== 'rot' && k !== 'cols' ? s(v) : v]));
	return {
		W,
		H,
		shot: capture(dir),
		device: (src, w, style) => duo(src, w, style, dev),
		css: `.phone{box-shadow:0 0 0 ${s(4)}px #3A3D46,0 ${s(90)}px ${s(210)}px rgba(0,0,0,.55)}
.phone b{position:absolute;background:linear-gradient(rgba(255,255,255,0),rgba(255,255,255,.035) 45%,rgba(0,0,0,.12) 55%,rgba(0,0,0,0))}`,
		cap: at({ side: 150, top: 230, head: 156, sub: 68, gap: 52 }),
		bottom: at({ w: 1500, top: 960 }),
		tilt: at({ w: 1500, dx: 90, top: 1000, rot: -5 }),
		pair: { back: at({ w: 1240, left: -200, top: 1160, rot: -7 }), front: at({ w: 1340, left: 680, top: 990, rot: 4 }) },
		hero: { ...at({ top: 210, lockup: 100, lockupGap: 120, head: 170, sub: 72, gap: 56 }), phone: at({ w: 1440, left: 283.5, top: 1100 }) },
		wall: at({ top: 150, item: 100, pad: 50, rule: 4, dot: 38, dotGap: 52, stripBottom: 210, cols: 14, p: 56, lockup: 86, lockupBottom: 224 })
	};
}

function slide(g, { head, sub, screen, layout = 'bottom', light = false, second }) {
	const { W, cap } = g;
	const ground = light ? C.aluminium : C.console;
	const type = light ? C.ink : C.paper;
	const muted = light ? C.slate : C.mist;
	const lit = light ? C.leaf : C.lamp;
	let device = '';
	if (layout === 'bottom') device = g.device(g.shot(screen), g.bottom.w, `left:${(W - g.bottom.w) / 2}px;top:${g.bottom.top}px`);
	if (layout === 'tilt') {
		const t = g.tilt;
		device = g.device(g.shot(screen), t.w, `left:${(W - t.w) / 2 + t.dx}px;top:${t.top}px;transform:rotate(${t.rot}deg)`);
	}
	if (layout === 'pair') {
		const { back, front } = g.pair;
		device =
			g.device(g.shot(second), back.w, `left:${back.left}px;top:${back.top}px;transform:rotate(${back.rot}deg);opacity:.9`) +
			g.device(g.shot(screen), front.w, `left:${front.left}px;top:${front.top}px;transform:rotate(${front.rot}deg)`);
	}
	return page(
		`body{background:${ground};color:${type};--lit:${lit}}
.cap{position:absolute;left:${cap.side}px;right:${cap.side}px;top:${cap.top}px}
.head{font-size:${cap.head}px}
p{font-size:${cap.sub}px;line-height:1.3;color:${muted};margin-top:${cap.gap}px;letter-spacing:-0.005em}${g.css && `\n${g.css}`}`,
		`<div class="cap"><h1 class="head">${head}</h1>${sub ? `<p>${sub}</p>` : ''}</div>${device}`
	);
}

function heroSlide(g) {
	const { cap, hero } = g;
	return page(
		`body{background:${C.console};color:${C.paper};--lit:${C.lamp}}
.cap{position:absolute;left:${cap.side}px;right:${cap.side}px;top:${hero.top}px}
.lockup{height:${hero.lockup}px;display:block;margin-bottom:${hero.lockupGap}px}
.head{font-size:${hero.head}px}
p{font-size:${hero.sub}px;line-height:1.3;color:${C.mist};margin-top:${hero.gap}px}${g.css && `\n${g.css}`}`,
		`<div class="cap">${lockup('dark')}<h1 class="head">Every play,<br>kept <g>forever</g>.</h1>
<p>Your Spotify stats, on your own server.</p></div>${g.device(g.shot('overview'), hero.phone.w, `left:${hero.phone.left}px;top:${hero.phone.top}px`)}`
	);
}

function wallSlide(g) {
	const { cap, wall } = g;
	const items = ['Live capture', 'Lifetime history', 'Every artist, album, track', 'Listening sessions', 'Works offline', 'Read-only, private', 'Free and open source'];
	return page(
		`body{background:${C.console};color:${C.paper};--lit:${C.lamp}}
.cap{position:absolute;left:${cap.side}px;right:${cap.side}px;top:${cap.top}px}
.head{font-size:${cap.head}px}
ul{list-style:none;margin-top:${wall.top}px}
li{font-stretch:112%;font-weight:640;font-size:${wall.item}px;letter-spacing:-0.02em;padding:${wall.pad}px 0;border-top:${wall.rule}px solid ${C.grille};display:flex;align-items:center;gap:${wall.dotGap}px}
li::before{content:'';width:${wall.dot}px;height:${wall.dot}px;border-radius:50%;background:${C.lamp};flex:none}
.strip{position:absolute;left:${cap.side}px;bottom:${wall.stripBottom}px}
.lockup{position:absolute;right:${cap.side}px;bottom:${wall.lockupBottom}px;height:${wall.lockup}px}`,
		`<div class="cap"><h1 class="head">All yours.<br>On <g>your</g> server.</h1>
<ul>${items.map((t) => `<li>${t}</li>`).join('')}</ul></div>
<div class="strip">${strip({ cols: wall.cols, p: wall.p, lit: C.lamp, unlit: C.grille, seed: 3 })}</div>${lockup('dark')}`
	);
}

const slides = (g) => [
	['01-hero', heroSlide(g)],
	['02-top', slide(g, { head: 'Know your<br><g>real</g> top 10.', sub: 'Tracks, artists, albums and genres, ranked by plays or time.', screen: 'top' })],
	['03-artist', slide(g, { head: 'Go deep on<br>any <g>artist</g>.', sub: 'Rank, all-time plays and your first listen.', screen: 'artist', layout: 'tilt' })],
	['04-sessions', slide(g, { head: 'Every session,<br><g>remembered</g>.', sub: 'Your history, grouped into the stretches you actually listened.', screen: 'sessions', layout: 'pair', second: 'history' })],
	['05-stats', slide(g, { head: 'Watch your<br>taste <g>move</g>.', sub: 'Plays by day and artist share, week on week.', screen: 'stats', light: true })],
	['06-library', slide(g, { head: 'Find the songs<br>you <g>never</g> play.', sub: 'Saved, unplayed and how long songs take to stick.', screen: 'library', layout: 'tilt' })],
	['07-features', wallSlide(g)]
];

const SETS = {
	iphone: { dir: 'app-store', g: IPHONE },
	'duo-inner': { dir: 'app-store/duo-inner', g: duoSet(2007, 2853, DUO.inner, 'captures/duo-inner') },
	'duo-outer': { dir: 'app-store/duo-outer', g: duoSet(1398, 2034, DUO.outer, 'captures/duo-outer') }
};

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

const args = process.argv.slice(2).flatMap((a) => (a === 'duo' ? ['duo-inner', 'duo-outer'] : [a]));
const known = [...Object.keys(SETS), 'wide'];
const unknown = args.filter((a) => !known.includes(a));
if (unknown.length) throw new Error(`unknown set ${unknown.join(', ')}; pick from ${known.join(', ')}, duo`);
const want = (name) => args.length === 0 || args.includes(name);

const chrome = await launch();
try {
	for (const [set, { dir, g }] of Object.entries(SETS)) {
		if (!want(set)) continue;
		mkdirSync(join(OUT, dir), { recursive: true });
		for (const [name, html] of slides(g)) {
			writeFileSync(join(OUT, dir, `${name}.png`), await chrome.render(html, { width: g.W, height: g.H }));
			console.log(`${dir}/${name}.png`);
		}
	}
	if (want('wide')) {
		writeFileSync(
			join(OUT, 'wide-1920x1080.png'),
			await chrome.render(wide(1920, 1080, { phones: ['top', 'overview', 'artist'], headSize: 84, subSize: 32, sub: 'Your self-hosted listening history and stats on every artist, album and track, now on iPhone.' }), { width: 1920, height: 1080 })
		);
		writeFileSync(
			join(OUT, 'social-1200x630.png'),
			await chrome.render(wide(1200, 630, { phones: ['top', 'overview', 'artist'], headSize: 52, subSize: 21, sub: 'Your self-hosted listening history, on iPhone.' }), { width: 1200, height: 630 })
		);
		console.log('wide-1920x1080.png, social-1200x630.png');
	}
} finally {
	await chrome.close();
}
