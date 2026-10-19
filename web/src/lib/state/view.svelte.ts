// View state lives in the URL: `range` (or `from`/`to`) is global and carried
// across navigation; `metric` belongs to one view and is remembered per view
// (D7), so the toggle you left on Top stays on Top when you come back.
//
// The URLSearchParams built here are short-lived scratch objects used to
// serialise a URL, never reactive state, hence the rule exemption.
/* eslint-disable svelte/prefer-svelte-reactivity */
import { page } from '$app/state';
import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import {
	metricFromParams,
	rangeFromParams,
	writeRange,
	type Metric,
	type RangeSel
} from '#lib/ranges.js';

const METRIC_KEY = 'spotussi:metric:';

function readStored(view: string): Metric | null {
	try {
		const v = localStorage.getItem(METRIC_KEY + view);
		return v === 'plays' || v === 'time' ? v : null;
	} catch {
		return null;
	}
}

// Metrics chosen this session, so components react when a view's metric
// changes. Written only by setMetric: `view.metric` is read while rendering,
// where writing state throws (state_unsafe_mutation) and aborts the page.
const remembered = $state<Record<string, Metric>>({});
// localStorage reads, cached outside reactivity for the same reason.
const stored = new Map<string, Metric | null>();

function viewId() {
	return page.route.id ?? page.url.pathname;
}

function storedMetric(view: string): Metric | null {
	if (view in remembered) return remembered[view];
	if (!stored.has(view)) stored.set(view, readStored(view));
	return stored.get(view) ?? null;
}

export const view = {
	get range(): RangeSel {
		return rangeFromParams(page.url.searchParams);
	},
	get metric(): Metric {
		return metricFromParams(page.url.searchParams) ?? storedMetric(viewId()) ?? 'plays';
	}
};

/** Range params only (what navigation carries between views). */
export function rangeSearch(sel: RangeSel = view.range) {
	const p = new URLSearchParams();
	writeRange(p, sel);
	const s = p.toString();
	return s ? `?${s}` : '';
}

/** The current URL with a different range (metric and other params kept). */
export function hrefWithRange(sel: RangeSel) {
	const p = new URLSearchParams(page.url.searchParams.toString());
	writeRange(p, sel);
	const s = p.toString();
	return page.url.pathname + (s ? `?${s}` : '');
}

export function setRange(sel: RangeSel) {
	// Same route, new query: page.url.pathname already includes the base path.
	// eslint-disable-next-line svelte/no-navigation-without-resolve
	return goto(hrefWithRange(sel), { reset: false });
}

export function setMetric(metric: Metric) {
	const id = viewId();
	remembered[id] = metric;
	try {
		localStorage.setItem(METRIC_KEY + id, metric);
	} catch {
		// Private mode: the URL still carries it.
	}
	const p = new URLSearchParams(page.url.searchParams.toString());
	p.set('metric', metric);
	// eslint-disable-next-line svelte/no-navigation-without-resolve
	return goto(`${page.url.pathname}?${p}`, { reset: false });
}

type Section = '/' | '/top' | '/stats' | '/history' | '/sessions' | '/settings';

/** Link to a top-level view, keeping the selected range. */
export function sectionHref(section: Section, sel?: RangeSel) {
	const base = section === '/top' ? resolve('/top/[[kind]]', {}) : resolve(section);
	return base + rangeSearch(sel);
}

/** A Top list, optionally narrowed to one artist, album or genre. */
export function topHref(
	kind: 'artists' | 'tracks' | 'albums' | 'genres',
	scope?: Record<string, string>
) {
	const p = new URLSearchParams(rangeSearch());
	for (const [k, v] of Object.entries(scope ?? {})) p.set(k, v);
	const s = p.toString();
	return resolve('/top/[[kind]]', { kind }) + (s ? `?${s}` : '');
}

/** History, optionally filtered (keeps the selected range). */
export function historyHref(filters?: Record<string, string>, sel?: RangeSel) {
	const p = new URLSearchParams(rangeSearch(sel));
	for (const [k, v] of Object.entries(filters ?? {})) p.set(k, v);
	const s = p.toString();
	return resolve('/history') + (s ? `?${s}` : '');
}

export function sessionHref(id: string) {
	return resolve('/sessions/[id]', { id }) + rangeSearch();
}

/** The current URL with some params set (string) or removed (null). */
export function hrefWith(changes: Record<string, string | null>) {
	const p = new URLSearchParams(page.url.searchParams.toString());
	for (const [k, v] of Object.entries(changes)) {
		if (v === null || v === '') p.delete(k);
		else p.set(k, v);
	}
	const s = p.toString();
	return page.url.pathname + (s ? `?${s}` : '');
}

export function setParams(
	changes: Record<string, string | null>,
	opts: { replace?: boolean } = {}
) {
	// eslint-disable-next-line svelte/no-navigation-without-resolve
	return goto(hrefWith(changes), { reset: false, replace: opts.replace });
}

export function artistHref(id: string) {
	return resolve('/artists/[id]', { id }) + rangeSearch();
}
export function albumHref(id: string) {
	return resolve('/albums/[id]', { id }) + rangeSearch();
}
export function trackHref(id: string) {
	return resolve('/tracks/[id]', { id }) + rangeSearch();
}
export function genreHref(id: string) {
	return resolve('/genres/[id]', { id }) + rangeSearch();
}
