// Stills of the mock app's Top lists for social posts, with the reel's real
// music swapped in (scripts/realdata.mjs): Top artists for this year and all
// time with Drake on top, and Top tracks this year by plays and by time.
//
//   portless run --name spotussi-mock ./bin/spotussi serve --mock   (repo root)
//   SPOTUSSI_CATALOG=.cache/catalog.json node scripts/social.mjs <out dir> [base] [shot,shot]   (motion/)
//
// Desktop layout, 1440 wide (the README screenshot's viewport), dark, 2x.
// For the Top tracks pair, the mock's top tracks get real tracks of about
// their own length, so a long track climbs under Time for the same reason it
// does in the mock: it is long, not because a name was moved.
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { open, sleep } from './cdp.mjs';
import { intercept } from './realdata.mjs';

process.env.NODE_TLS_REJECT_UNAUTHORIZED = '0';
const out = process.argv[2];
const base = process.argv[3] || 'https://spotussi-mock.localhost';
const only = process.argv[4]?.split(',');
if (!out) throw new Error('usage: node scripts/social.mjs <out dir> [base] [shot,shot]');
mkdirSync(out, { recursive: true });

const catalog = JSON.parse(readFileSync(process.env.SPOTUSSI_CATALOG ?? new URL('./catalog.json', import.meta.url), 'utf8'));

// Real tracks for the mock's top tracks this year (both orderings), nearest
// in length, longest first; an artist from the reel's top twenty when one is
// within 20 s, and no artist more than twice.
async function trackPins() {
  const get = async (metric) => (await (await fetch(`${base}/api/v1/top/tracks?range=this_year&limit=25&metric=${metric}`)).json()).items;
  const seen = new Map();
  for (const it of [...(await get('plays')), ...(await get('time'))]) seen.set(it.track.id, it.track);
  const pool = catalog.flatMap((a, ai) => a.albums.flatMap((al) => al.tracks.map((t) => ({ artist: a.name, ai, track: t.name, ms: t.ms }))));
  const taken = new Set();
  const per = new Map();
  const pins = {};
  for (const t of [...seen.values()].sort((a, b) => b.duration_ms - a.duration_ms)) {
    const free = pool.filter((p) => !taken.has(p.artist + p.track) && (per.get(p.artist) ?? 0) < 2);
    const dist = (p) => Math.abs(p.ms - t.duration_ms);
    free.sort((a, b) => dist(a) - dist(b));
    const pick = free.find((p) => p.ai < 20 && dist(p) <= 20000) ?? free[0];
    taken.add(pick.artist + pick.track);
    per.set(pick.artist, (per.get(pick.artist) ?? 0) + 1);
    pins[t.id] = { artist: pick.artist, track: pick.track };
  }
  return pins;
}

const shots = [
  { key: 'top-artists-2026-drake', path: '/top/artists?range=this_year', range: 'this_year' },
  { key: 'top-artists-alltime-drake', path: '/top/artists?range=all', range: 'all' },
  { key: 'top-tracks-plays', path: '/top/tracks?range=this_year', range: 'this_year', pins: true },
  { key: 'top-tracks-time', path: '/top/tracks?range=this_year&metric=time', range: 'this_year', pins: true },
];

const HEIGHT = Number(process.env.SOCIAL_HEIGHT || 1100);
for (const s of shots) {
  if (only && !only.includes(s.key)) continue;
  const b = await open({ width: 1440, height: HEIGHT, dpr: 2 });
  await intercept(b, base, { range: s.range, tracks: s.pins ? await trackPins() : {} });
  await b.send('Page.navigate', { url: base + s.path });
  await sleep(5000);
  // Mock mode runs without sign-in and says so in a banner the real app never shows.
  await b.evaluate(`(() => {
    const s = [...document.querySelectorAll('span')].find((e) => e.textContent.trim() === 'No sign-in required.');
    let el = s;
    while (el && el.parentElement && el.parentElement.getBoundingClientRect().height < 120) el = el.parentElement;
    el?.remove();
  })()`);
  await b.evaluate(`(async () => {
    const loads = Promise.all([...document.images].map((i) => i.complete || new Promise((r) => { i.onload = i.onerror = r; })));
    await Promise.race([loads, new Promise((r) => setTimeout(r, 4000))]);
  })()`);
  await sleep(1000);
  writeFileSync(`${out}/${s.key}.png`, await b.shot({ x: 0, y: 0, width: 1440, height: HEIGHT }));
  console.log(s.key);
  b.close();
  await sleep(500);
}
