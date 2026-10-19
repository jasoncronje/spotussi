// Builds every spotussi brand asset from one geometry: the logo SVGs, the
// web icons and manifest, and the social and README images.
//
//   node brand/scripts/build.mjs          (repo root; needs Google Chrome and
//                                          web/node_modules for the Archivo font)
//
// Writes brand/logo, brand/web, brand/social and brand/previews/sheet.png,
// then copies brand/web into web/static. Chrome renders the PNGs (set CHROME
// to its binary if it isn't in /Applications). The wordmark outlines come
// from brand/src/wordmark.json (see outline.py), so the SVGs need no font.
import { copyFileSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { launch } from './cdp.mjs';

const BRAND = join(dirname(fileURLToPath(import.meta.url)), '..');
const ROOT = join(BRAND, '..');
const out = (...p) => {
	const f = join(BRAND, ...p);
	mkdirSync(dirname(f), { recursive: true });
	return f;
};

// Palette: the app's own tokens (web/src/routes/layout.css) in sRGB.
export const C = {
	console: '#121317', // dark ground (the app's dark theme-color)
	graphite: '#1A1C21', // the plate and dark panels
	grille: '#3A3D46', // unlit lamps on graphite
	lamp: '#F2AB3E', // the amber lamp, on dark grounds
	paper: '#ECEEF1', // type on dark grounds
	mist: '#A1A5AC', // secondary type on dark grounds
	aluminium: '#EDEEF0', // light ground
	ink: '#16171B', // type on light grounds
	slate: '#5B5F68', // secondary type on light grounds
	ember: '#DF7B0C', // the lamp on light grounds (day amber)
	grilleDay: '#CFD1D4' // unlit lamps on light grounds
};

// ── The mark ────────────────────────────────────────────────────────────
// A 4×4 dot-matrix level meter on a 32-unit grid: lamps 5.5 units across,
// 7 units apart, lit from the bottom to heights 2·4·3·1.
const HEIGHTS = [2, 4, 3, 1];
const PITCH = 7;
const R = 2.75;
const C0 = 5.5;
const lamps = () =>
	HEIGHTS.flatMap((h, c) =>
		[0, 1, 2, 3].map((r) => ({ cx: C0 + c * PITCH, cy: C0 + r * PITCH, lit: 3 - r < h }))
	);
const n = (v) => +v.toFixed(3);

/** The lamps alone; unlit lamps are rings when `ring` is set (mono). */
function dots({ lit, unlit, ring = false, dx = 0, dy = 0, s = 1 }) {
	return lamps()
		.map(({ cx, cy, lit: on }) => {
			const x = n(dx + cx * s);
			const y = n(dy + cy * s);
			if (on) return `<circle cx="${x}" cy="${y}" r="${n(R * s)}" fill="${lit}"/>`;
			if (ring)
				return `<circle cx="${x}" cy="${y}" r="${n((R - 0.45) * s)}" fill="none" stroke="${unlit}" stroke-width="${n(0.9 * s)}"/>`;
			return `<circle cx="${x}" cy="${y}" r="${n(R * s)}" fill="${unlit}"/>`;
		})
		.join('');
}

const svg = (viewBox, body, extra = '') =>
	`<svg xmlns="http://www.w3.org/2000/svg" viewBox="${viewBox}"${extra}>${body}</svg>\n`;
const TIGHT = `${R} ${R} ${32 - 2 * R} ${32 - 2 * R}`; // the lamps' own bounds

const grounds = {
	dark: { lit: C.lamp, unlit: C.grille, type: C.paper },
	light: { lit: C.ember, unlit: C.grilleDay, type: C.ink },
	'mono-black': { lit: '#000', unlit: '#000', type: '#000', ring: true },
	'mono-white': { lit: '#FFF', unlit: '#FFF', type: '#FFF', ring: true }
};

/** The plated mark: lamps on a graphite tile, legible on any ground. */
const plate = (radius = 7) =>
	`<rect width="32" height="32" rx="${radius}" fill="${C.graphite}"/>` +
	dots({ lit: C.lamp, unlit: C.grille });

// ── The wordmark ────────────────────────────────────────────────────────
const W = JSON.parse(readFileSync(join(BRAND, 'src/wordmark.json'), 'utf8'));

/** Bounds of an absolute SVG path (control points included). */
function pathBounds(d) {
	const tok = d.match(/[A-Za-z]|-?\d*\.?\d+(?:e-?\d+)?/g);
	let cmd = '';
	let x = 0;
	let y = 0;
	const b = { x0: Infinity, y0: Infinity, x1: -Infinity, y1: -Infinity };
	const add = () => {
		b.x0 = Math.min(b.x0, x);
		b.x1 = Math.max(b.x1, x);
		b.y0 = Math.min(b.y0, y);
		b.y1 = Math.max(b.y1, y);
	};
	for (let i = 0; i < tok.length; ) {
		if (/[A-Za-z]/.test(tok[i])) cmd = tok[i++];
		if (cmd === 'Z') continue;
		if (cmd === 'H') x = +tok[i++];
		else if (cmd === 'V') y = +tok[i++];
		else {
			x = +tok[i++];
			y = +tok[i++];
		}
		add();
	}
	return b;
}
const WB = pathBounds(W.d);

// Lockup, in font units: the bare mark is 2.2 x-heights tall, centred on
// the x-height, one mark-width × 0.42 from the word. The box is the mark's
// height, so the word's descender and the t's head sit inside it.
const XH = W.xHeight;
const MH = Math.round(XH * 2.2);
const GAP = Math.round(MH * 0.42);
const MY = Math.round(-XH / 2 - MH / 2);
const LOCK_W = MH + GAP + (WB.x1 - WB.x0);
const LOCK_VB = `0 ${MY} ${n(LOCK_W)} ${MH}`;
const markScale = MH / (32 - 2 * R);

function lockupBody(g) {
	const t = grounds[g];
	return (
		dots({ ...t, dx: -R * markScale, dy: MY - R * markScale, s: markScale }) +
		`<path transform="translate(${n(MH + GAP - WB.x0)} 0)" fill="${t.type}" d="${W.d}"/>`
	);
}
const wordmarkVB = `${n(WB.x0)} ${n(WB.y0)} ${n(WB.x1 - WB.x0)} ${n(WB.y1 - WB.y0)}`;

// ── A meter strip: the mark's grille carried on as a run of columns ─────
// Heights are synthetic (a seeded wave), never anyone's listening.
function stripHeights(count, seed) {
	let s = seed;
	const rnd = () => (s = (s * 16807) % 2147483647) / 2147483647;
	const h = [];
	for (let i = 0; i < count; i++) {
		// Two slow waves, a weekend lift and a little noise: reads as a run of
		// days without pretending to be anyone's plays.
		const v =
			1.5 +
			1.35 * Math.sin(i * 0.31 + seed) +
			0.75 * Math.sin(i * 0.97 + seed * 2) +
			(i % 7 >= 5 ? 0.6 : 0) +
			(rnd() - 0.5) * 0.9;
		h.push(Math.max(0, Math.min(4, Math.round(v))));
	}
	return h;
}

/** HTML for a 4-row strip of lamps at pitch `p` px, `cols` columns. */
function strip({ cols, p, lit, unlit, seed }) {
	const r = (p * R) / PITCH;
	const hs = stripHeights(cols, seed);
	let body = '';
	hs.forEach((h, c) => {
		for (let row = 0; row < 4; row++) {
			const on = 3 - row < h;
			body += `<circle cx="${n(r + c * p)}" cy="${n(r + row * p)}" r="${n(r)}" fill="${on ? lit : unlit}"/>`;
		}
	});
	const w = n((cols - 1) * p + 2 * r);
	const h = n(3 * p + 2 * r);
	return `<svg width="${w}" height="${h}" viewBox="0 0 ${w} ${h}" style="display:block">${body}</svg>`;
}

// ── Raster pages ────────────────────────────────────────────────────────
const fontFile = join(
	ROOT,
	'web/node_modules/@fontsource-variable/archivo/files/archivo-latin-wdth-normal.woff2'
);
const FONT = `@font-face{font-family:Archivo;src:url(data:font/woff2;base64,${readFileSync(fontFile).toString('base64')}) format('woff2');font-weight:100 900;font-stretch:62% 125%}`;

const page = (css, body) =>
	`<!doctype html><html><head><meta charset="utf-8"><style>${FONT}
*{margin:0;box-sizing:border-box}html,body{width:100%;height:100%}
body{font-family:Archivo,sans-serif;-webkit-font-smoothing:antialiased;font-kerning:normal}
.head{font-stretch:125%;font-weight:760;letter-spacing:-0.035em;line-height:.98}
${css}</style></head><body>${body}</body></html>`;

const inlineLockup = (g, height) =>
	svg(LOCK_VB, lockupBody(g), ` height="${height}" style="display:block"`);

/** Open Graph / GitHub social card: name, headline, one line, a meter strip. */
function socialCard(width, height) {
	const pad = Math.round(width * 0.06);
	const p = Math.round(width / 42);
	const cols = Math.ceil((width - pad) / p) + 1; // runs off the right edge
	return page(
		`body{background:${C.console};color:${C.paper};padding:${pad}px;display:flex;flex-direction:column;overflow:hidden}
h1{font-size:${Math.round(width * 0.07)}px;margin-top:auto}
p{font-size:${Math.round(width * 0.0205)}px;font-stretch:100%;color:${C.mist};margin-top:${Math.round(width * 0.018)}px;letter-spacing:-0.005em;line-height:1.35;max-width:${Math.round(width * 0.62)}px}
.strip{margin-top:auto;margin-right:-${pad}px}
body>svg{align-self:flex-start}`,
		`${inlineLockup('dark', Math.round(width * 0.036))}
<h1 class="head">Spotify Stats,<br>Self-Hosted.</h1>
<p>Live capture, lifetime history, stats on every artist, album and track, and an MCP server for your AI.</p>
<div class="strip">${strip({ cols, p, lit: C.lamp, unlit: C.grille, seed: 3 })}</div>`
	);
}

/** README header: the lockup, the line, and the mark's grille running on. */
function readmeHeader(g) {
	const dark = g === 'dark';
	const t = grounds[g];
	const mark = 50; // lockup height in px; the strip shares its lamp pitch and rows
	const p = (mark * PITCH) / (32 - 2 * R);
	return page(
		`body{background:transparent}
.panel{width:880px;height:220px;border-radius:16px;overflow:hidden;background:${dark ? C.graphite : C.aluminium};
display:flex;align-items:center;padding-left:44px}
.row{display:flex;align-items:flex-start;gap:${n(p * 3)}px}
p{font-size:19px;font-stretch:112%;font-weight:520;letter-spacing:-0.01em;color:${dark ? C.mist : C.slate};margin-top:16px}`,
		`<div class="panel"><div class="row"><div>${inlineLockup(g, mark)}<p>Spotify Stats, Self-Hosted.</p></div>
${strip({ cols: 40, p, lit: t.lit, unlit: t.unlit, seed: 5 })}</div></div>`
	);
}

/** A square icon: lamps centred on a graphite ground. */
function iconPage(size, { radius = 0, grid }) {
	// `grid` is the lamps' span as a fraction of the icon.
	const s = (size * grid) / (32 - 2 * R);
	const off = (size - (32 - 2 * R) * s) / 2 - R * s;
	return page(
		'body{background:transparent}',
		svg(
			`0 0 ${size} ${size}`,
			`<rect width="${size}" height="${size}" rx="${radius}" fill="${C.graphite}"/>` +
				dots({ lit: C.lamp, unlit: C.grille, dx: off, dy: off, s }),
			` width="${size}" height="${size}" style="display:block"`
		)
	);
}

// The 16 px favicon frame, drawn on whole pixels: 2 px lamps on a 3 px pitch
// (a 1 px gap), so they stay separate dots instead of a smear. The grid is
// 11 px wide, so it sits 2 px from the top-left and 3 px from the far edges.
const favicon16 = svg(
	'0 0 16 16',
	`<rect width="16" height="16" rx="3.5" fill="${C.graphite}"/>` +
		lamps()
			.map(
				({ cx, cy, lit }) =>
					`<rect x="${2 + ((cx - C0) / PITCH) * 3}" y="${2 + ((cy - C0) / PITCH) * 3}" width="2" height="2" rx=".4" fill="${lit ? C.lamp : C.grille}"/>`
			)
			.join('')
);

/** An .ico holding PNG frames (Vista+ format; every browser reads it). */
function ico(frames) {
	const head = Buffer.alloc(6 + 16 * frames.length);
	head.writeUInt16LE(0, 0);
	head.writeUInt16LE(1, 2);
	head.writeUInt16LE(frames.length, 4);
	let offset = head.length;
	frames.forEach(({ size, png }, i) => {
		const e = 6 + 16 * i;
		head.writeUInt8(size >= 256 ? 0 : size, e);
		head.writeUInt8(size >= 256 ? 0 : size, e + 1);
		head.writeUInt16LE(1, e + 4); // colour planes
		head.writeUInt16LE(32, e + 6); // bits per pixel
		head.writeUInt32LE(png.length, e + 8);
		head.writeUInt32LE(offset, e + 12);
		offset += png.length;
	});
	return Buffer.concat([head, ...frames.map((f) => f.png)]);
}

// ── Write everything ────────────────────────────────────────────────────
const write = (rel, data) => writeFileSync(out(rel), data);

write('logo/mark.svg', svg('0 0 32 32', plate()));
for (const g of Object.keys(grounds)) {
	write(`logo/mark-${g}.svg`, svg(TIGHT, dots(grounds[g])));
	write(`logo/lockup-${g}.svg`, svg(LOCK_VB, lockupBody(g)));
	write(
		`logo/wordmark-${g}.svg`,
		svg(wordmarkVB, `<path fill="${grounds[g].type}" d="${W.d}"/>`)
	);
}

write('web/favicon.svg', svg('0 0 32 32', plate()));
write(
	'web/manifest.webmanifest',
	JSON.stringify(
		{
			name: 'spotussi',
			short_name: 'spotussi',
			description: 'Self-hosted listening stats.',
			start_url: '.',
			scope: '.',
			display: 'standalone',
			background_color: C.console,
			theme_color: C.console,
			icons: [
				{ src: 'favicon.svg', sizes: 'any', type: 'image/svg+xml' },
				{ src: 'icon-192.png', sizes: '192x192', type: 'image/png' },
				{ src: 'icon-512.png', sizes: '512x512', type: 'image/png' },
				{ src: 'icon-maskable-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' }
			]
		},
		null,
		'\t'
	) + '\n'
);

const chrome = await launch();
try {
	const shot = (html, o) => chrome.render(html, o);
	const plated = (size) =>
		shot(iconPage(size, { radius: (size * 7) / 32, grid: (32 - 2 * R) / 32 }), {
			width: size,
			height: size,
			transparent: true
		});

	const f16 = await shot(page('body{background:transparent}', favicon16.replace('<svg', '<svg width="16" height="16" style="display:block"')), {
		width: 16,
		height: 16,
		transparent: true
	});
	const f32 = await plated(32);
	const f48 = await plated(48);
	write('web/favicon.ico', ico([
		{ size: 16, png: f16 },
		{ size: 32, png: f32 },
		{ size: 48, png: f48 }
	]));
	write('web/icon-192.png', await plated(192));
	write('web/icon-512.png', await plated(512));
	// Maskable: full bleed, lamps inside the 80% safe circle.
	write('web/icon-maskable-512.png', await shot(iconPage(512, { grid: 0.54 }), { width: 512, height: 512 }));
	// iOS rounds the corners itself; give it a full-bleed square.
	write('web/apple-touch-icon.png', await shot(iconPage(180, { grid: 0.6 }), { width: 180, height: 180 }));

	write('social/og-1200x630.png', await shot(socialCard(1200, 630), { width: 1200, height: 630 }));
	write('social/github-social-1280x640.png', await shot(socialCard(1280, 640), { width: 1280, height: 640 }));
	for (const g of ['dark', 'light'])
		write(
			`social/readme-header-${g}.png`,
			await shot(readmeHeader(g), { width: 880, height: 220, dpr: 2, transparent: true })
		);

	// A contact sheet of the kit, for review.
	const img = (rel) => `data:image/png;base64,${readFileSync(join(BRAND, rel)).toString('base64')}`;
	const svgUrl = (rel) => `data:image/svg+xml;base64,${readFileSync(join(BRAND, rel)).toString('base64')}`;
	write(
		'previews/sheet.png',
		await shot(
			page(
				`body{background:#0B0C0E;padding:40px;color:${C.mist};font-size:13px}
.g{display:grid;gap:20px;align-items:start}.t{border-radius:14px;padding:28px;display:grid;place-items:center}
.d{background:${C.console}}.l{background:${C.aluminium}}.w{background:#fff}
.k{background:${C.aluminium}}.k img{display:block}.px{image-rendering:pixelated}`,
				`<div class="g" style="grid-template-columns:repeat(4,1fr)">
<div class="t d"><img src="${svgUrl('logo/lockup-dark.svg')}" height="48"></div>
<div class="t l"><img src="${svgUrl('logo/lockup-light.svg')}" height="48"></div>
<div class="t w"><img src="${svgUrl('logo/lockup-mono-black.svg')}" height="48"></div>
<div class="t" style="background:#3a3d46"><img src="${svgUrl('logo/lockup-mono-white.svg')}" height="48"></div>
<div class="t d" style="grid-template-columns:repeat(4,auto);gap:22px;align-items:end"><img src="${svgUrl('logo/mark.svg')}" width="96"><img src="${img('web/icon-192.png')}" width="48"><img src="${img('web/apple-touch-icon.png')}" width="40" style="border-radius:9px"><img src="${img('web/icon-maskable-512.png')}" width="48" style="border-radius:50%"></div>
<div class="t l" style="grid-template-columns:repeat(4,auto);gap:22px;align-items:end"><img src="${svgUrl('logo/mark-light.svg')}" width="72"><img src="${svgUrl('web/favicon.svg')}" width="32"><img class="px" src="data:image/png;base64,${f16.toString('base64')}" width="64"><img src="data:image/png;base64,${f16.toString('base64')}" width="16"></div>
<div class="t d"><img src="${svgUrl('logo/mark-dark.svg')}" width="72"></div>
<div class="t w"><img src="${svgUrl('logo/mark-mono-black.svg')}" width="72"></div>
</div>
<div class="g" style="grid-template-columns:1fr 1fr;margin-top:20px">
<img src="${img('social/og-1200x630.png')}" style="width:100%;border-radius:10px">
<img src="${img('social/github-social-1280x640.png')}" style="width:100%;border-radius:10px">
<div class="t w" style="padding:20px"><img src="${img('social/readme-header-light.png')}" style="width:100%"></div>
<div class="t" style="background:#0d1117;padding:20px"><img src="${img('social/readme-header-dark.png')}" style="width:100%"></div>
</div>`
			),
			{ width: 1600, height: 1006 }
		)
	);
} finally {
	await chrome.close();
}

for (const f of [
	'favicon.svg',
	'favicon.ico',
	'apple-touch-icon.png',
	'icon-192.png',
	'icon-512.png',
	'icon-maskable-512.png',
	'manifest.webmanifest'
])
	copyFileSync(join(BRAND, 'web', f), join(ROOT, 'web/static', f));

console.log('brand assets written to brand/ and web/static/');
