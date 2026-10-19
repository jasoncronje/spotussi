// Adapters from ranked API rows to the list item shape every ranking view
// draws (Overview top 5, Top pages, entity pages).
import type { Schemas } from '#lib/api/client.js';
import type { TopItem } from '#lib/components/overview/top-list.svelte';
import { albumHref, artistHref, genreHref, trackHref } from '#lib/state/view.svelte.js';

const artistSubs = (as: { id: string; name: string }[]) =>
	as.map((a) => ({ id: a.id, name: a.name, href: artistHref(a.id) }));

export function artistItem(r: Schemas['RankedArtist']): TopItem {
	return {
		id: r.artist.id,
		href: artistHref(r.artist.id),
		name: r.artist.name,
		image: r.artist.image_url,
		round: true,
		rank: r.rank,
		prevRank: r.prev_rank,
		plays: r.plays,
		ms: r.ms_played
	};
}

/** `withAlbum` adds the album after the artists (Top tracks page). */
export function trackItem(r: Schemas['RankedTrack'], withAlbum = false): TopItem {
	const subs = artistSubs(r.track.artists);
	if (withAlbum && r.track.album) {
		subs.push({
			id: 'album:' + r.track.album.id,
			name: r.track.album.name,
			href: albumHref(r.track.album.id)
		});
	}
	return {
		id: r.track.id,
		href: trackHref(r.track.id),
		name: r.track.name,
		image: r.track.image_url,
		subs,
		rank: r.rank,
		prevRank: r.prev_rank,
		plays: r.plays,
		ms: r.ms_played
	};
}

export function albumItem(r: Schemas['RankedAlbum']): TopItem {
	return {
		id: r.album.id,
		href: albumHref(r.album.id),
		name: r.album.name,
		image: r.album.image_url,
		subs: artistSubs(r.album.artists),
		rank: r.rank,
		prevRank: r.prev_rank,
		plays: r.plays,
		ms: r.ms_played
	};
}

export function genreItem(r: Schemas['RankedGenre']): TopItem {
	return {
		id: r.genre.id,
		href: genreHref(r.genre.id),
		name: r.genre.name,
		rank: r.rank,
		prevRank: r.prev_rank,
		plays: r.plays,
		ms: r.ms_played
	};
}
