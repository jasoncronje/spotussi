// Client-side response cache for /api/v1 GETs.
//
// - Stale-while-revalidate: a cached response renders immediately; anything
//   older than FRESH_MS is revalidated in the background. The API sends weak
//   ETags with `Cache-Control: no-cache`, so the browser turns those
//   revalidations into conditional requests (If-None-Match -> 304) by itself.
// - In-flight requests are shared, so a hover prefetch and the navigation that
//   follows it cost one request.
// - Bounded LRU so a long session of range hopping can't grow without limit.

export type Query<T> = {
	/** Stable identity: path plus sorted query string. */
	key: string;
	run: () => Promise<T>;
};

type Entry = { data: unknown; at: number };

const FRESH_MS = 30_000;
const MAX_ENTRIES = 300;

const store = new Map<string, Entry>();
const inflight = new Map<string, Promise<unknown>>();

export class ApiError extends Error {
	constructor(
		message: string,
		readonly status: number,
		readonly code?: string
	) {
		super(message);
	}
}

export function queryKey(path: string, query: Record<string, unknown> = {}) {
	const qs = Object.entries(query)
		.filter(([, v]) => v !== undefined && v !== null && v !== '')
		.sort(([a], [b]) => (a < b ? -1 : 1))
		.map(([k, v]) => `${k}=${encodeURIComponent(String(v))}`)
		.join('&');
	return qs ? `${path}?${qs}` : path;
}

export function peek<T>(key: string): { data: T; fresh: boolean } | undefined {
	const e = store.get(key);
	if (!e) return undefined;
	// LRU touch
	store.delete(key);
	store.set(key, e);
	return { data: e.data as T, fresh: Date.now() - e.at < FRESH_MS };
}

/** Fetches through the cache (dedupes concurrent requests). */
export function fetchQuery<T>(q: Query<T>): Promise<T> {
	const pending = inflight.get(q.key);
	if (pending) return pending as Promise<T>;
	const p = q
		.run()
		.then((data) => {
			store.set(q.key, { data, at: Date.now() });
			if (store.size > MAX_ENTRIES) store.delete(store.keys().next().value!);
			return data;
		})
		.finally(() => inflight.delete(q.key));
	inflight.set(q.key, p);
	return p;
}

/** Warms the cache for queries that aren't already fresh. Errors are ignored. */
export function prefetch(queries: Query<unknown>[]) {
	for (const q of queries) {
		const hit = store.get(q.key);
		if (hit && Date.now() - hit.at < FRESH_MS) continue;
		fetchQuery(q).catch(() => {});
	}
}

/**
 * Drops cached responses (all, or those whose key starts with a prefix).
 * Mounted resources refetch through `bumpEpoch` (resource.svelte.ts).
 */
export function invalidate(prefix = '') {
	for (const k of [...store.keys()]) if (k.startsWith(prefix)) store.delete(k);
}
