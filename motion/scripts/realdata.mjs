// Swaps the mock server's invented artists, albums and tracks for real ones
// (scripts/catalog.json, from scripts/fetch-catalog.mjs) as the capture's
// browser receives them, so the reel shows real music with its real artwork.
// The mock's top artists over the app's default range take the catalog's
// artists in rank order; every other artist borrows one of the top twenty, so
// a play history reads like someone's own. Track lengths become the real ones,
// and plays keep their share of the track.
import { readFileSync } from 'node:fs';

const RANKED = 20;
const hash = (s) => {
  let h = 2166136261;
  for (const c of s) h = Math.imul(h ^ c.charCodeAt(0), 16777619) >>> 0;
  return h;
};
const artists = JSON.parse(readFileSync(new URL('./catalog.json', import.meta.url), 'utf8'));
/** Genres for the top-genres lists, in rank order, to match the artists. */
const genres = [...new Set(artists.flatMap((a) => a.genres))];

export async function rewriter(base) {
  // The ranking the app opens on (web/src/lib/ranges.ts: DEFAULT_PRESET).
  const top = await (await fetch(`${base}/api/v1/top/artists?range=last_4w&limit=${artists.length}`)).json();
  const artistOf = new Map(top.items.map((it, i) => [it.artist.id, artists[i]]));
  const albumOf = new Map();
  const trackOf = new Map();
  const used = new Map();
  const genreOf = new Map();
  const unknown = new Set();

  const artist = (id) => {
    if (!artistOf.has(id)) artistOf.set(id, artists[hash(id) % RANKED]);
    return artistOf.get(id);
  };
  const album = (id, by) => {
    if (!albumOf.has(id)) {
      const a = by ?? artists[hash(id) % RANKED];
      albumOf.set(id, { artist: a, album: a.albums[hash(id + 'a') % a.albums.length] });
    }
    return albumOf.get(id);
  };
  // A track takes its album's next unused track, so one album's tracks differ.
  const track = (id, albumId, by) => {
    if (!trackOf.has(id)) {
      const { artist: a, album: al } = album(albumId ?? id, by);
      const taken = used.get(al) ?? new Set();
      used.set(al, taken);
      let i = hash(id) % al.tracks.length;
      for (let n = 0; n < al.tracks.length && taken.has(i); n++) i = (i + 1) % al.tracks.length;
      taken.add(i);
      trackOf.set(id, { artist: a, album: al, track: al.tracks[i] });
    }
    return trackOf.get(id);
  };
  const genre = (g) => {
    if (!genreOf.has(g)) genreOf.set(g, genres[genreOf.size % genres.length]);
    return genreOf.get(g);
  };

  const isArtist = (o, key) => key === 'artist' || key === 'artists' || key === 'series' || o.image_url?.includes('/artist/');
  const isTrack = (o, key) => key === 'track' || key === 'tracks' || 'duration_ms' in o;
  const isAlbum = (o, key) => key === 'album' || key === 'albums' || (o.image_url?.includes('/album/') && !isTrack(o, key));

  const visit = (node, key) => {
    if (Array.isArray(node)) {
      if (key === 'genres') return node.map((g) => (typeof g === 'string' ? genre(g) : visit(g, key)));
      const out = node.map((n) => visit(n, key));
      // Two mock artists can become one real one: name them once.
      if (/artists$/.test(key ?? '')) return out.filter((a, i) => !a?.name || out.findIndex((b) => b?.name === a.name) === i);
      return out;
    }
    if (!node || typeof node !== 'object') return node;
    const o = { ...node };
    if (typeof o.genre === 'string') o.genre = genre(o.genre);
    if (typeof o.id === 'string' && typeof o.name === 'string') {
      if (isTrack(o, key)) {
        const by = o.artists?.[0] ? artist(o.artists[0].id) : undefined;
        const t = track(o.id, o.album?.id, by);
        o.name = t.track.name;
        if (o.album) o.album = { ...o.album, name: t.album.name };
        // The real track's own artist; the mock's invented features go.
        if (o.artists?.length) o.artists = [{ ...o.artists[0], name: t.artist.name }];
        if (typeof o.duration_ms === 'number') o.duration_ms = t.track.ms;
        for (const [k, v] of Object.entries(o)) if (!['album', 'artists'].includes(k)) o[k] = visit(v, k);
        return o;
      }
      if (key === 'genre' || key === 'genres') o.name = genre(o.name);
      else if (isArtist(o, key)) o.name = artist(o.id).name;
      else if (isAlbum(o, key)) {
        const by = o.artists?.[0] ? artist(o.artists[0].id) : undefined;
        o.name = album(o.id, by).album.name;
      } else if (key !== 'context' && o.type !== 'playlist') unknown.add(key);
    }
    for (const [k, v] of Object.entries(o)) o[k] = visit(v, k);
    // An artist's page names their genres; the mock's artists have none.
    if (isArtist(o, key) && typeof o.id === 'string' && Array.isArray(o.genres) && !o.genres.length)
      o.genres = artist(o.id).genres.map((g) => ({ id: g.replace(/\W+/g, '-'), name: g }));
    return o;
  };

  // A play keeps its share of the track: full plays stay full.
  const plays = (node) => {
    if (Array.isArray(node)) return node.forEach(plays);
    if (!node || typeof node !== 'object') return;
    const t = node.track;
    if (t && typeof node.ms_played === 'number' && typeof t.duration_ms === 'number' && trackOf.has(t.id)) {
      const share = Math.min(1, node.ms_played / (node.__was ?? t.duration_ms));
      node.ms_played = Math.round((share > 0.97 ? 1 : share) * t.duration_ms);
      if (typeof node.played_at === 'number' && typeof node.ended_at === 'number') node.ended_at = node.played_at + node.ms_played;
    }
    delete node.__was;
    Object.values(node).forEach(plays);
  };
  // Remember each play's original track length before the swap.
  const was = (node) => {
    if (Array.isArray(node)) return node.forEach(was);
    if (!node || typeof node !== 'object') return;
    if (node.track && typeof node.ms_played === 'number' && typeof node.track.duration_ms === 'number') node.__was = node.track.duration_ms;
    Object.values(node).forEach(was);
  };

  return {
    unknown,
    json(body) {
      const data = JSON.parse(body);
      was(data);
      const out = visit(data, null);
      plays(out);
      return JSON.stringify(out);
    },
    /** The real artwork for /_mock/img/<kind>/<id>.svg: an artist's photo or an album's cover. */
    image(url) {
      const m = url.match(/\/_mock\/img\/(\w+)\/([^/.]+)\.svg/);
      if (!m) return null;
      const [, kind, id] = m;
      const file = kind === 'artist' ? artist(id).image : kind === 'album' ? album(id).album.cover : null;
      return file ? readFileSync(new URL(`../${file}`, import.meta.url)) : null;
    },
  };
}

/** Routes the browser's API and artwork responses through the rewriter. */
export async function intercept(b, base) {
  const r = await rewriter(base);
  b.on('Fetch.requestPaused', async (e) => {
    const { requestId, request, responseStatusCode: status, responseHeaders: headers = [] } = e;
    const type = headers.find((h) => /^content-type$/i.test(h.name))?.value ?? '';
    if (!/json|svg/.test(type) || status !== 200) return b.send('Fetch.continueRequest', { requestId });
    try {
      // Artwork: the real photo or cover (playlists keep their placeholder).
      if (/svg/.test(type)) {
        const img = r.image(request.url);
        if (!img) return b.send('Fetch.continueRequest', { requestId });
        return await b.send('Fetch.fulfillRequest', {
          requestId,
          responseCode: 200,
          responseHeaders: [{ name: 'Content-Type', value: 'image/jpeg' }, { name: 'Cache-Control', value: 'no-store' }],
          body: img.toString('base64'),
        });
      }
      const res = await b.send('Fetch.getResponseBody', { requestId });
      let body = res.result.base64Encoded ? Buffer.from(res.result.body, 'base64').toString('utf8') : res.result.body;
      body = r.json(body);
      await b.send('Fetch.fulfillRequest', {
        requestId,
        responseCode: status,
        responseHeaders: headers.filter((h) => !/^content-(length|encoding)$/i.test(h.name)),
        body: Buffer.from(body).toString('base64'),
      });
    } catch (err) {
      console.error('rewrite failed', request.url, err.message);
      await b.send('Fetch.continueRequest', { requestId });
    }
  });
  await b.send('Fetch.enable', {
    patterns: [
      { urlPattern: '*/api/v1/*', requestStage: 'Response' },
      { urlPattern: '*/_mock/img/*', requestStage: 'Response' },
    ],
  });
  return r;
}
