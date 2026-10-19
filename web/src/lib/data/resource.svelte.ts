// A reactive view of one cached query. When the query changes (new range or
// metric) the previous data stays on screen, marked `stale`, until the new
// response arrives, so range switches never flash skeletons or jump layout.
//
// Performance (M7 range-switch pass): responses are immutable, so `current`
// is raw state (a deep proxy over a 180-point timeline cost more than drawing
// it), and the effect that follows the query never writes state itself: a
// cache hit applied inside the effect made Svelte walk every reaction of
// `current` for self-invalidation, a 70-90 ms long task on Overview.
import { untrack } from 'svelte';
import { fetchQuery, invalidate, peek, type Query } from './cache.js';
import { unreachable } from '#lib/state/link.svelte.js';

/** How many resources are replacing data that is on screen (range/metric switch). */
export const activity = $state({ refreshing: 0 });

/** Bumped when stored data changed (settings, exclusions): every resource refetches. */
const epoch = $state({ n: 0 });

/** Clears the response cache and refetches everything on screen (old data stays visible). */
export function refreshAll() {
	invalidate();
	epoch.n++;
}

export class Resource<T> {
	/** Latest data we have (possibly for the previous query while loading). */
	current = $state.raw<T | undefined>(undefined);
	error = $state.raw<Error | null>(null);
	loading = $state(false);
	/** True while `current` belongs to a previous query. */
	stale = $state(false);

	#key = '';
	#query: Query<T> | null = null;
	#counted = false;
	#epoch = epoch.n;
	#disposed = false;

	#setLoading(on: boolean) {
		this.loading = on;
		const count = on && this.stale && !this.#disposed;
		if (count !== this.#counted) {
			activity.refreshing += count ? 1 : -1;
			this.#counted = count;
		}
	}

	constructor(getQuery: () => Query<T> | null) {
		// Seed from the cache while the component initialises, so a page
		// opened on cached data renders it on its first pass.
		const first = untrack(getQuery);
		const hit = first && peek<T>(first.key);
		if (first && hit) {
			this.current = hit.data;
			if (hit.fresh) {
				this.#key = first.key;
				this.#query = first;
			}
		}
		$effect(() => {
			const q = getQuery();
			const n = epoch.n;
			if (!q) return;
			const force = n !== this.#epoch;
			this.#epoch = n;
			// Apply outside the effect (still before the next paint).
			queueMicrotask(() => {
				if (!this.#disposed) this.#load(q, force);
			});
		});
		$effect(() => () => {
			this.#setLoading(false);
			this.#disposed = true;
		});
	}

	/** Initial load with nothing to show yet. */
	get pending() {
		return this.current === undefined && this.error === null;
	}

	/** Refetch the current query (keeps showing the current data meanwhile). */
	refresh = () => {
		if (this.#query) this.#load(this.#query, true);
	};
	retry = this.refresh;

	#load(q: Query<T>, force = false) {
		if (q.key === this.#key && !force && !this.error) return;
		this.#key = q.key;
		this.#query = q;
		const hit = peek<T>(q.key);
		if (hit) {
			if (this.current !== hit.data) this.current = hit.data;
			if (this.error) this.error = null;
			this.stale = false;
			if (hit.fresh && !force) return;
		} else {
			this.stale = this.current !== undefined;
		}
		this.#setLoading(true);
		if (force) this.error = null;
		fetchQuery(q).then(
			(data) => {
				if (this.#key !== q.key || this.#disposed) return;
				this.current = data;
				this.error = null;
				this.stale = false;
				this.#setLoading(false);
			},
			(err: Error) => {
				if (this.#key !== q.key || this.#disposed) return;
				// Server unreachable with something on screen: keep it (dimmed
				// if it belongs to another query). The shell's banner says why
				// and refetches everything once the server answers.
				if (!(unreachable(err) && this.current !== undefined)) this.error = err;
				this.#setLoading(false);
			}
		);
	}
}
