// Typed query builders over the generated client. Each returns a cacheable
// Query (see cache.ts); views compose them, prefetchers reuse them.
import { api, type Schemas } from '#lib/api/client.js';
import { ApiError, queryKey, type Query } from './cache.js';
import { rangeQuery, type Metric, type RangeSel } from '#lib/ranges.js';
import { noteFailure, noteSuccess, unreachable } from '#lib/state/link.svelte.js';

type Result<T> = { data?: T; error?: unknown; response: Response };

export async function unwrap<T>(p: Promise<Result<T>>): Promise<T> {
	let res: Result<T>;
	try {
		res = await p;
	} catch (e) {
		// A non-JSON answer (a proxy's error or block page) fails to parse.
		const err =
			e instanceof SyntaxError
				? new ApiError('The server sent an unexpected response.', 0, 'bad_response')
				: new ApiError('The server could not be reached.', 0, 'network');
		noteFailure(err);
		throw err;
	}
	if (res.data !== undefined) {
		noteSuccess();
		return res.data;
	}
	if (res.response.ok) {
		noteSuccess();
		return undefined as T; // 204
	}
	const err = (res.error as Schemas['Error'] | undefined)?.error;
	const e = new ApiError(
		err?.message ?? `Request failed (${res.response.status}).`,
		res.response.status,
		err?.code
	);
	if (unreachable(e)) noteFailure(e);
	else noteSuccess();
	throw e;
}

function q<T>(path: string, query: Record<string, unknown>, run: () => Promise<Result<T>>) {
	return { key: queryKey(path, query), run: () => unwrap(run()) } satisfies Query<T>;
}

/** One entity a list, chart or log can be narrowed to (at most one per request). */
export type Scope =
	| { artist_id: string }
	| { album_id: string }
	| { track_id: string }
	| { genre_id: string }
	| Record<string, never>;

// ---------- overview ----------

export function summaryQuery(sel: RangeSel, metric: Metric) {
	const query = { ...rangeQuery(sel), metric };
	return q('/overview/summary', query, () => api.GET('/overview/summary', { params: { query } }));
}

export function timelineQuery(sel: RangeSel, scope: Scope = {}, compare = false) {
	const query = { ...rangeQuery(sel), ...scope, ...(compare ? { compare: true } : {}) };
	return q('/timeline', query, () => api.GET('/timeline', { params: { query } }));
}

export function clockQuery(sel: RangeSel, scope: Scope = {}) {
	const query = { ...rangeQuery(sel), ...scope };
	return q('/clock', query, () => api.GET('/clock', { params: { query } }));
}

// ---------- top lists ----------

export type TopKind = 'artists' | 'tracks' | 'albums' | 'genres';

export function topArtistsQuery(
	sel: RangeSel,
	metric: Metric,
	limit = 5,
	offset = 0,
	scope: { album_id: string } | { genre_id: string } | Record<string, never> = {}
) {
	const query = { ...rangeQuery(sel), metric, limit, offset: offset || undefined, ...scope };
	return q('/top/artists', query, () => api.GET('/top/artists', { params: { query } }));
}

export function topTracksQuery(
	sel: RangeSel,
	metric: Metric,
	limit = 5,
	offset = 0,
	scope:
		{ artist_id: string } | { album_id: string } | { genre_id: string } | Record<string, never> = {}
) {
	const query = { ...rangeQuery(sel), metric, limit, offset: offset || undefined, ...scope };
	return q('/top/tracks', query, () => api.GET('/top/tracks', { params: { query } }));
}

export function topAlbumsQuery(
	sel: RangeSel,
	metric: Metric,
	limit = 5,
	offset = 0,
	scope: { artist_id: string } | { genre_id: string } | Record<string, never> = {}
) {
	const query = { ...rangeQuery(sel), metric, limit, offset: offset || undefined, ...scope };
	return q('/top/albums', query, () => api.GET('/top/albums', { params: { query } }));
}

export function topGenresQuery(sel: RangeSel, metric: Metric, limit = 5, offset = 0) {
	const query = { ...rangeQuery(sel), metric, limit, offset: offset || undefined };
	return q('/top/genres', query, () => api.GET('/top/genres', { params: { query } }));
}

export function contextsQuery(sel: RangeSel, metric: Metric, scope: Scope = {}, limit = 8) {
	const query = { ...rangeQuery(sel), metric, limit, ...scope };
	return q('/contexts', query, () => api.GET('/contexts', { params: { query } }));
}

// ---------- stats ----------

export function hourlyTopQuery(sel: RangeSel, metric: Metric) {
	const query = { ...rangeQuery(sel), metric };
	return q('/stats/hourly-top', query, () => api.GET('/stats/hourly-top', { params: { query } }));
}

export function artistsOverTimeQuery(sel: RangeSel, metric: Metric, series = 5) {
	const query = { ...rangeQuery(sel), metric, series };
	return q('/stats/artists-over-time', query, () =>
		api.GET('/stats/artists-over-time', { params: { query } })
	);
}

export function genresOverTimeQuery(sel: RangeSel, metric: Metric, series = 5) {
	const query = { ...rangeQuery(sel), metric, series };
	return q('/stats/genres-over-time', query, () =>
		api.GET('/stats/genres-over-time', { params: { query } })
	);
}

export function releaseYearsQuery(sel: RangeSel, metric: Metric) {
	const query = { ...rangeQuery(sel), metric };
	return q('/stats/release-years', query, () =>
		api.GET('/stats/release-years', { params: { query } })
	);
}

export function artistsPerTrackQuery(sel: RangeSel, metric: Metric) {
	const query = { ...rangeQuery(sel), metric };
	return q('/stats/artists-per-track', query, () =>
		api.GET('/stats/artists-per-track', { params: { query } })
	);
}

// ---------- live ----------

export function recentQuery(limit = 12) {
	const query = { limit };
	return q('/history', query, () => api.GET('/history', { params: { query } }));
}

export function nowPlayingQuery() {
	return q('/now-playing', {}, () => api.GET('/now-playing'));
}

export function statusQuery() {
	return q('/system/status', {}, () => api.GET('/system/status'));
}

export function importsQuery() {
	return q('/imports', {}, () => api.GET('/imports'));
}

// ---------- library (D36) ----------

export type LibraryPlayed = 'all' | 'played' | 'maybe_played' | 'never_played';
export type LibraryTrackSort = 'added' | 'plays' | 'last_played';
export type LibraryAlbumSort = 'added' | 'plays' | 'played_share';
export type LibraryPlaylistSort = 'plays_here' | 'played_share' | 'name';
export type LibraryRelation = 'all' | 'own' | 'collaborative' | 'followed';

export function librarySummaryQuery() {
	return q('/library/summary', {}, () => api.GET('/library/summary'));
}

export function libraryTimelineQuery() {
	const query = { range: 'all' as const, bucket: 'month' as const };
	return q('/library/timeline', query, () => api.GET('/library/timeline', { params: { query } }));
}

export function libraryTracksQuery(
	played: LibraryPlayed,
	sort: LibraryTrackSort,
	limit = 50,
	offset = 0
) {
	const query = { played, sort, limit, offset: offset || undefined };
	return q('/library/tracks', query, () => api.GET('/library/tracks', { params: { query } }));
}

export function libraryAlbumsQuery(sort: LibraryAlbumSort, limit = 50, offset = 0) {
	const query = { sort, limit, offset: offset || undefined };
	return q('/library/albums', query, () => api.GET('/library/albums', { params: { query } }));
}

export function libraryPlaylistsQuery(
	relation: LibraryRelation,
	sort: LibraryPlaylistSort,
	limit = 50,
	offset = 0
) {
	const query = { relation, sort, limit, offset: offset || undefined };
	return q('/library/playlists', query, () => api.GET('/library/playlists', { params: { query } }));
}

export function libraryPlaylistQuery(id: string, limit = 100, offset = 0) {
	const query = { limit, offset: offset || undefined };
	return q(`/library/playlists/${id}`, query, () =>
		api.GET('/library/playlists/{id}', { params: { path: { id }, query } })
	);
}

export async function syncLibrary() {
	return unwrap(api.POST('/library/sync'));
}

// ---------- entities ----------

export function artistQuery(id: string, sel: RangeSel, metric: Metric) {
	const query = { ...rangeQuery(sel), metric };
	return q(`/artists/${id}`, query, () =>
		api.GET('/artists/{id}', { params: { path: { id }, query } })
	);
}

export function albumQuery(id: string, sel: RangeSel, metric: Metric) {
	const query = { ...rangeQuery(sel), metric };
	return q(`/albums/${id}`, query, () =>
		api.GET('/albums/{id}', { params: { path: { id }, query } })
	);
}

export function trackQuery(id: string, sel: RangeSel, metric: Metric) {
	const query = { ...rangeQuery(sel), metric };
	return q(`/tracks/${id}`, query, () =>
		api.GET('/tracks/{id}', { params: { path: { id }, query } })
	);
}

export function genreQuery(id: string, sel: RangeSel, metric: Metric) {
	const query = { ...rangeQuery(sel), metric };
	return q(`/genres/${id}`, query, () =>
		api.GET('/genres/{id}', { params: { path: { id }, query } })
	);
}

export function searchQuery(text: string, limit = 5) {
	const query = { q: text, limit };
	return q('/search', query, () => api.GET('/search', { params: { query } }));
}

// ---------- history & sessions ----------

export type HistoryFilters = {
	artist_id?: string;
	album_id?: string;
	track_id?: string;
	genre_id?: string;
	skipped?: boolean;
	shuffle?: boolean;
	counted?: boolean;
	device?: string;
	platform?: string;
};

export function historyQuery(sel: RangeSel, filters: HistoryFilters, cursor?: string, limit = 50) {
	const query = { ...rangeQuery(sel), ...filters, cursor, limit };
	return q('/history', query, () => api.GET('/history', { params: { query } }));
}

export function devicesQuery(sel: RangeSel) {
	const query = { ...rangeQuery(sel) };
	return q('/devices', query, () => api.GET('/devices', { params: { query } }));
}

export function sessionsQuery(sel: RangeSel, limit = 20, offset = 0) {
	const query = { ...rangeQuery(sel), limit, offset: offset || undefined };
	return q('/sessions', query, () => api.GET('/sessions', { params: { query } }));
}

export function sessionQuery(id: string) {
	return q(`/sessions/${id}`, {}, () => api.GET('/sessions/{id}', { params: { path: { id } } }));
}

// ---------- settings & system ----------

export function settingsQuery() {
	return q('/settings', {}, () => api.GET('/settings'));
}

export function exclusionsQuery() {
	return q('/exclusions', {}, () => api.GET('/exclusions'));
}

export function tokensQuery() {
	return q('/auth/tokens', {}, () => api.GET('/auth/tokens'));
}

export function connectionQuery() {
	return q('/spotify/connection', {}, () => api.GET('/spotify/connection'));
}

// ---------- page bundles (prefetch) ----------

/** Everything the Overview page reads for a range + metric. */
export function overviewQueries(sel: RangeSel, metric: Metric) {
	return [
		summaryQuery(sel, metric),
		timelineQuery(sel, {}, true),
		clockQuery(sel),
		topArtistsQuery(sel, metric),
		topTracksQuery(sel, metric),
		topAlbumsQuery(sel, metric),
		topGenresQuery(sel, metric)
	];
}

export function statsQueries(sel: RangeSel, metric: Metric) {
	return [
		timelineQuery(sel),
		artistsOverTimeQuery(sel, metric),
		hourlyTopQuery(sel, metric),
		releaseYearsQuery(sel, metric),
		artistsPerTrackQuery(sel, metric),
		genresOverTimeQuery(sel, metric)
	];
}
