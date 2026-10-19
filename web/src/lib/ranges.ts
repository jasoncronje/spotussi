// Range presets from the API contract (RangePreset) with their labels, plus the
// URL vocabulary every view shares: `range`, `from`, `to` (custom) and `metric`.
import type { Schemas } from '#lib/api/client.js';

export type Preset = Schemas['RangePreset'];
export type Metric = Schemas['Metric'];

/** The contract default when no range is given. */
export const DEFAULT_PRESET: Preset = 'last_4w';

export const PRESET_LABEL: Record<Preset, string> = {
	today: 'Today',
	this_week: 'This week',
	this_month: 'This month',
	this_year: 'This year',
	last_7d: 'Last 7 days',
	last_4w: 'Last 4 weeks',
	last_6m: 'Last 6 months',
	last_12m: 'Last 12 months',
	prev_day: 'Yesterday',
	prev_week: 'Previous week',
	prev_month: 'Previous month',
	prev_year: 'Previous year',
	all: 'All time',
	custom: 'Custom range'
};

/** Short labels for the segmented quick picks. */
export const QUICK_PRESETS: { preset: Preset; short: string }[] = [
	{ preset: 'today', short: 'Today' },
	{ preset: 'last_7d', short: '7 days' },
	{ preset: 'last_4w', short: '4 weeks' },
	{ preset: 'last_6m', short: '6 months' },
	{ preset: 'last_12m', short: '12 months' },
	{ preset: 'all', short: 'All time' }
];

/** Every preset, grouped the way people think about them. */
export const PRESET_GROUPS: { label: string; presets: Preset[] }[] = [
	{ label: 'Rolling', presets: ['last_7d', 'last_4w', 'last_6m', 'last_12m'] },
	{ label: 'So far', presets: ['today', 'this_week', 'this_month', 'this_year'] },
	{ label: 'Completed', presets: ['prev_day', 'prev_week', 'prev_month', 'prev_year'] },
	{ label: 'Everything', presets: ['all'] }
];

const COMPARE_LABEL: Partial<Record<Preset, string>> = {
	today: 'vs the same time yesterday',
	this_week: 'vs the same point last week',
	this_month: 'vs the same point last month',
	this_year: 'vs the same point last year',
	last_7d: 'vs the 7 days before',
	last_4w: 'vs the 4 weeks before',
	last_6m: 'vs the 6 months before',
	last_12m: 'vs the 12 months before',
	prev_day: 'vs the day before',
	prev_week: 'vs the week before',
	prev_month: 'vs the month before',
	prev_year: 'vs the year before'
};

const PRESETS = new Set<string>(Object.keys(PRESET_LABEL));
const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;

export type RangeSel =
	| { preset: Exclude<Preset, 'custom'>; from?: undefined; to?: undefined }
	| { preset: 'custom'; from: string; to: string };

/** Reads the range selection from URL search params (invalid values fall back). */
export function rangeFromParams(params: { get(name: string): string | null }): RangeSel {
	const from = params.get('from');
	const to = params.get('to');
	if (from && to && DATE_RE.test(from) && DATE_RE.test(to)) {
		return from <= to ? { preset: 'custom', from, to } : { preset: 'custom', from: to, to: from };
	}
	const r = params.get('range');
	if (r && PRESETS.has(r) && r !== 'custom') return { preset: r as Exclude<Preset, 'custom'> };
	return { preset: DEFAULT_PRESET as Exclude<Preset, 'custom'> };
}

export function metricFromParams(params: { get(name: string): string | null }): Metric | null {
	const m = params.get('metric');
	return m === 'plays' || m === 'time' ? m : null;
}

/** API query parameters for a range selection. */
export function rangeQuery(sel: RangeSel): { range?: Preset; from?: string; to?: string } {
	return sel.preset === 'custom' ? { from: sel.from, to: sel.to } : { range: sel.preset };
}

/** Writes a range selection into search params (the default preset is omitted). */
export function writeRange(params: URLSearchParams, sel: RangeSel) {
	params.delete('range');
	params.delete('from');
	params.delete('to');
	if (sel.preset === 'custom') {
		params.set('from', sel.from);
		params.set('to', sel.to);
	} else if (sel.preset !== DEFAULT_PRESET) {
		params.set('range', sel.preset);
	}
}

export function sameRange(a: RangeSel, b: RangeSel) {
	return a.preset === b.preset && a.from === b.from && a.to === b.to;
}

export function compareLabel(sel: RangeSel, days?: number): string | null {
	if (sel.preset === 'all') return null;
	if (sel.preset === 'custom') {
		return days ? `vs the ${days === 1 ? 'day' : `${days} days`} before` : 'vs the period before';
	}
	return COMPARE_LABEL[sel.preset] ?? null;
}

/** Inclusive day count between two local dates. */
export function daySpan(from: string, to: string) {
	const a = Date.UTC(+from.slice(0, 4), +from.slice(5, 7) - 1, +from.slice(8, 10));
	const b = Date.UTC(+to.slice(0, 4), +to.slice(5, 7) - 1, +to.slice(8, 10));
	return Math.round((b - a) / 86_400_000) + 1;
}
