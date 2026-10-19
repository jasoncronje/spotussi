// Builds the hero reel's sample music (scripts/catalog.json) from Deezer's
// public API: each artist's photo, and their top tracks with real lengths,
// albums and album covers. The artwork is cached in .cache/img (not committed)
// for the capture to serve in place of the mock's placeholder artwork.
//
//   node scripts/fetch-catalog.mjs   (motion/)
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';

// In rank order: the mock's top artists take these names. Genres are the
// ones the app would show for them.
const artists = [
  ['Drake', ['hip hop', 'rap', 'canadian hip hop']],
  ['Kendrick Lamar', ['hip hop', 'west coast hip hop']],
  ['Travis Scott', ['rap', 'hip hop']],
  ['Fred again..', ['electronic', 'house']],
  ['Arctic Monkeys', ['indie rock', 'rock']],
  ['Metallica', ['metal', 'hard rock']],
  ['J. Cole', ['hip hop', 'rap']],
  ['The Weeknd', ['r&b', 'pop']],
  ['Tame Impala', ['psychedelic rock']],
  ['Future', ['rap', 'trap']],
  ['Post Malone', ['hip hop', 'pop']],
  ['Red Hot Chili Peppers', ['rock', 'funk rock']],
  ['Metro Boomin', ['hip hop', 'trap']],
  ['Daft Punk', ['electronic', 'french house']],
  ['Tyler, The Creator', ['hip hop', 'alternative hip hop']],
  ['Foo Fighters', ['rock', 'alternative rock']],
  ['21 Savage', ['rap', 'trap']],
  ['Zach Bryan', ['country', 'americana']],
  ['Mac Miller', ['hip hop']],
  ['Linkin Park', ['alternative rock', 'nu metal']],
  ['Eminem', ['hip hop', 'rap']],
  ['Pink Floyd', ['progressive rock']],
  ['Kid Cudi', ['hip hop']],
  ['AC/DC', ['hard rock']],
  ['Frank Ocean', ['r&b']],
  ['Gorillaz', ['alternative']],
  ['Led Zeppelin', ['hard rock', 'classic rock']],
  ['Calvin Harris', ['electronic', 'dance']],
  ['A$AP Rocky', ['hip hop']],
  ['Radiohead', ['alternative rock']],
  ['Morgan Wallen', ['country']],
  ['The Strokes', ['indie rock']],
  ['Queen', ['classic rock']],
  ['Playboi Carti', ['rap']],
  ['Nirvana', ['grunge']],
  ['Avicii', ['electronic', 'dance']],
  ['Childish Gambino', ['hip hop', 'funk']],
  ['Rage Against the Machine', ['rap metal']],
  ['Don Toliver', ['rap']],
  ['Oasis', ['britpop', 'rock']],
  ['Lil Baby', ['rap', 'trap']],
  ['Kings of Leon', ['rock']],
  ['Chris Stapleton', ['country']],
  ['Gunna', ['rap', 'trap']],
  ['OutKast', ['hip hop']],
  ['Bad Bunny', ['reggaeton', 'latin trap']],
  ['Fisher', ['tech house']],
];

const api = async (path) => {
  for (let i = 0; i < 4; i++) {
    const r = await fetch(`https://api.deezer.com${path}`);
    const j = await r.json();
    if (!j.error) return j;
    await new Promise((res) => setTimeout(res, 1500));
  }
  throw new Error(`deezer ${path}`);
};

const tidy = (s) =>
  s
    .replace(/\s*[([][^)\]]*\b(feat|ft|with|remaster\w*|deluxe|version|edition|box set|expanded|anniversary|radio edit|bonus|explicit|live)\b[^)\]]*[)\]]/gi, '')
    .replace(/\s+-\s+(\d{4} )?(remaster(ed)?|radio edit|single version).*$/i, '')
    .trim();
const rude = /\b(bitch|fuck|shit|nigg|hoe|stfu|pussy|dick)/i;

mkdirSync('.cache/img', { recursive: true });
const cache = async (url) => {
  const file = `.cache/img/${createHash('sha1').update(url).digest('hex').slice(0, 16)}.jpg`;
  if (!existsSync(file)) writeFileSync(file, Buffer.from(await (await fetch(url)).arrayBuffer()));
  return file;
};

const out = [];
for (const [name, genres] of artists) {
  // The best-known artist of that exact name.
  const found = (await api(`/search/artist?q=${encodeURIComponent(name)}&limit=10`)).data
    .filter((a) => a.name.toLowerCase() === name.toLowerCase())
    .sort((a, b) => b.nb_fan - a.nb_fan)[0];
  if (!found) throw new Error(`no artist ${name}`);
  const top = (await api(`/artist/${found.id}/top?limit=30`)).data;
  const albums = new Map();
  const seen = new Set();
  for (const t of top) {
    // Their own tracks only, named without edition tags, and nothing a
    // homepage would bleep.
    if (t.artist.id !== found.id) continue;
    const title = tidy(t.title_short);
    if (rude.test(title)) continue;
    if (seen.has(title.toLowerCase())) continue;
    seen.add(title.toLowerCase());
    const key = t.album.id;
    if (!albums.has(key)) albums.set(key, { name: tidy(t.album.title), cover: await cache(t.album.cover_big), tracks: [] });
    albums.get(key).tracks.push({ name: title, ms: t.duration * 1000 });
  }
  out.push({ name, genres, image: await cache(found.picture_big), albums: [...albums.values()] });
  console.log(`${name}: ${seen.size} tracks on ${albums.size} albums`);
}
writeFileSync('scripts/catalog.json', JSON.stringify(out, null, 1) + '\n');
