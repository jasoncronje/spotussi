// Number, duration, delta and date formatting. One place, so every view
// writes plays, listening time and changes the same way.
import { prefs, type DateFormat } from '#lib/state/prefs.svelte.js';

const intFmt = new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 });
const compactFmt = new Intl.NumberFormat(undefined, {
	notation: 'compact',
	maximumFractionDigits: 1
});
const MINUS = '−';

export const MIN = 60_000;
export const HOUR = 3_600_000;

/** 1,284 */
export function int(n: number) {
	return intFmt.format(Math.round(n));
}

/** 1.2K: axis ticks and tight spots only. */
export function compact(n: number) {
	return n < 1000 ? int(n) : compactFmt.format(n);
}

export function plural(n: number, one: string, many = one + 's') {
	return `${int(n)} ${n === 1 ? one : many}`;
}

const oneDecimal = new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 });

/** File sizes in decimal units, as Finder and Spotify's email show them: 812 KB, 42.1 MB, 1.2 GB. */
export function bytes(n: number) {
	const units = ['bytes', 'KB', 'MB', 'GB', 'TB'];
	let v = n;
	let i = 0;
	while (v >= 1000 && i < units.length - 1) {
		v /= 1000;
		i++;
	}
	if (i === 0) return `${int(v)} ${n === 1 ? 'byte' : 'bytes'}`;
	return `${v >= 100 ? int(v) : oneDecimal.format(v)} ${units[i]}`;
}

export type DurationPart = { value: string; unit: string };

/**
 * Listening time split into display parts:
 * 42 s · 42 min · 41 h 12 min · 1,412 h (minutes dropped past 100 h, where
 * they stop being meaningful).
 */
export function durationParts(ms: number): DurationPart[] {
	if (ms < MIN) return [{ value: int(ms / 1000), unit: 's' }];
	const totalMin = Math.floor(ms / MIN);
	if (totalMin < 60) return [{ value: int(totalMin), unit: 'min' }];
	const h = Math.floor(totalMin / 60);
	const m = totalMin % 60;
	if (h >= 100 || m === 0) return [{ value: int(h), unit: 'h' }];
	return [
		{ value: int(h), unit: 'h' },
		{ value: String(m), unit: 'min' }
	];
}

/** "41 h 12 min" */
export function duration(ms: number) {
	return durationParts(ms)
		.map((p) => `${p.value} ${p.unit}`)
		.join(' ');
}

/** Track length style: 3:41, 1:02:10 */
export function trackLength(ms: number) {
	const s = Math.round(ms / 1000);
	const h = Math.floor(s / 3600);
	const m = Math.floor((s % 3600) / 60);
	const sec = String(s % 60).padStart(2, '0');
	return h ? `${h}:${String(m).padStart(2, '0')}:${sec}` : `${m}:${sec}`;
}

/** Value of a measure in the active metric, formatted. */
export function metricValue(metric: 'plays' | 'time', m: { plays: number; ms_played: number }) {
	return metric === 'plays' ? plural(m.plays, 'play') : duration(m.ms_played);
}

/** Axis ticks for durations: hours when the tick step is whole hours, else minutes. */
export function durationTickFormatter(stepMs: number) {
	if (stepMs >= HOUR) {
		return (ms: number) => `${compact(Math.round((ms / HOUR) * 10) / 10)} h`;
	}
	return (ms: number) => `${int(ms / MIN)} min`;
}

/**
 * Round axis ticks from 0 up to at least `max`, at most ~`count` intervals.
 * Durations step in clean minutes/hours (15 min, 1 h, 6 h…), counts in 1-2-5.
 */
export function niceTicks(max: number, kind: 'count' | 'duration', count = 4): number[] {
	if (max <= 0) return [0, kind === 'duration' ? HOUR : 1];
	let steps: number[];
	if (kind === 'duration') {
		const m = [1, 2, 5, 10, 15, 20, 30].map((x) => x * MIN);
		const h = [1, 2, 3, 4, 6, 8, 12, 24, 48, 72, 120, 168, 240, 500, 1000, 2000, 5000].map(
			(x) => x * HOUR
		);
		steps = [...m, ...h];
	} else {
		steps = [];
		for (let p = 1; p < 1e12; p *= 10) steps.push(p, 2 * p, 5 * p);
	}
	const step = steps.find((s) => max / s <= count) ?? steps[steps.length - 1]!;
	const ticks: number[] = [];
	for (let v = 0; v < max + step; v += step) {
		ticks.push(v);
		if (v >= max) break;
	}
	return ticks;
}

export type Delta =
	| { kind: 'change'; dir: 'up' | 'down' | 'flat'; text: string; ratio: number }
	| { kind: 'new'; text: string }
	| { kind: 'none' };

/** Relative change vs the previous period: +34%, −2.4%, 0%. */
export function delta(current: number, previous: number | null | undefined): Delta {
	if (previous == null) return { kind: 'none' };
	if (previous === 0) return current === 0 ? { kind: 'none' } : { kind: 'new', text: 'new' };
	const ratio = (current - previous) / previous;
	const pct = Math.abs(ratio * 100);
	const digits = pct < 10 && pct !== 0 ? 1 : 0;
	const rounded = Number(pct.toFixed(digits));
	if (rounded === 0) return { kind: 'change', dir: 'flat', text: '0%', ratio };
	const sign = ratio > 0 ? '+' : MINUS;
	return {
		kind: 'change',
		dir: ratio > 0 ? 'up' : 'down',
		text: `${sign}${rounded.toFixed(digits)}%`,
		ratio
	};
}

// ---------- dates ----------
//
// Dates follow the instance's date_format setting (prefs): `auto` uses the
// browser locale, `dmy`/`mdy` pick a day-first or month-first English
// layout, `ymd` writes ISO dates. Times always follow the browser locale.

function parseLocalDate(d: string) {
	return new Date(+d.slice(0, 4), +d.slice(5, 7) - 1, +d.slice(8, 10) || 1);
}

type Fmt = { format(d: Date): string };
type DateFmts = {
	dayMonth: Fmt;
	dayMonthYear: Fmt;
	weekdayDayMonth: Fmt;
	monthYear: Fmt;
	longDay: Fmt;
};

const pad = (n: number) => String(n).padStart(2, '0');
const iso = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;

function intlFmts(locale: string | undefined): DateFmts {
	return {
		dayMonth: new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'short' }),
		dayMonthYear: new Intl.DateTimeFormat(locale, {
			day: 'numeric',
			month: 'short',
			year: 'numeric'
		}),
		weekdayDayMonth: new Intl.DateTimeFormat(locale, {
			weekday: 'short',
			day: 'numeric',
			month: 'short'
		}),
		monthYear: new Intl.DateTimeFormat(locale, { month: 'short', year: 'numeric' }),
		longDay: new Intl.DateTimeFormat(locale, {
			weekday: 'long',
			day: 'numeric',
			month: 'long',
			year: 'numeric'
		})
	};
}

const weekdayShort = new Intl.DateTimeFormat(undefined, { weekday: 'short' });
const weekdayLong = new Intl.DateTimeFormat(undefined, { weekday: 'long' });

const ymdFmts: DateFmts = {
	dayMonth: { format: (d) => `${pad(d.getMonth() + 1)}-${pad(d.getDate())}` },
	dayMonthYear: { format: iso },
	weekdayDayMonth: { format: (d) => `${weekdayShort.format(d)} ${iso(d)}` },
	monthYear: { format: (d) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}` },
	longDay: { format: (d) => `${weekdayLong.format(d)} ${iso(d)}` }
};

const FMTS: Record<DateFormat, DateFmts> = {
	auto: intlFmts(undefined),
	dmy: intlFmts('en-GB'),
	mdy: intlFmts('en-US'),
	ymd: ymdFmts
};

/** Formatters for the current date_format (reactive: reads prefs). */
function f(): DateFmts {
	return FMTS[prefs.dateFormat] ?? FMTS.auto;
}

const monthOnly = new Intl.DateTimeFormat(undefined, { month: 'short' });
const timeFmt = new Intl.DateTimeFormat(undefined, { hour: 'numeric', minute: '2-digit' });
const hourFmt = new Intl.DateTimeFormat(undefined, { hour: 'numeric' });

/** "20 Sep – 17 Oct 2026" for an inclusive local-date span. */
export function dateSpan(from: string, to: string) {
	const a = parseLocalDate(from);
	const b = parseLocalDate(to);
	const x = f();
	if (from === to) return x.dayMonthYear.format(a);
	if (a.getFullYear() === b.getFullYear() && prefs.dateFormat !== 'ymd') {
		return `${x.dayMonth.format(a)} – ${x.dayMonthYear.format(b)}`;
	}
	return `${x.dayMonthYear.format(a)} – ${x.dayMonthYear.format(b)}`;
}

export function shortDate(d: string) {
	return f().dayMonthYear.format(parseLocalDate(d));
}

/** Hour of day label: "9 PM" / "21" depending on locale. */
export function hourLabel(hour: number) {
	return hourFmt.format(new Date(2000, 0, 1, hour));
}

/** "21:00–22:00" style span, locale aware. */
export function hourSpan(hour: number) {
	const a = timeFmt.format(new Date(2000, 0, 1, hour));
	const b = timeFmt.format(new Date(2000, 0, 1, (hour + 1) % 24));
	return `${a}–${b}`;
}

export type Bucket = 'hour' | 'day' | 'week' | 'month' | 'year';

/** Long label for a timeline bucket key (tooltips, tables). */
export function bucketLabel(key: string, bucket: Bucket) {
	const x = f();
	switch (bucket) {
		case 'hour':
			return hourSpan(+key.slice(11, 13)) + ', ' + x.dayMonth.format(parseLocalDate(key));
		case 'day':
			return x.weekdayDayMonth.format(parseLocalDate(key));
		case 'week':
			return `Week of ${x.dayMonthYear.format(parseLocalDate(key))}`;
		case 'month':
			return x.monthYear.format(parseLocalDate(key + '-01'));
		case 'year':
			return key;
	}
}

/** Compact axis label for a bucket key. */
export function bucketTick(key: string, bucket: Bucket) {
	switch (bucket) {
		case 'hour':
			return hourLabel(+key.slice(11, 13));
		case 'day':
		case 'week':
			return f().dayMonth.format(parseLocalDate(key));
		case 'month': {
			const d = parseLocalDate(key + '-01');
			return d.getMonth() === 0 ? String(d.getFullYear()) : monthOnly.format(d);
		}
		case 'year':
			return key;
	}
}

const MONTH_STEPS = [1, 2, 3, 6, 12, 24, 36, 60, 120, 240];
const YEAR_STEPS = [1, 2, 5, 10, 20, 50];

/**
 * X-axis labels for a bucketed series, as index → label: at most about
 * `max` of them, on calendar-aligned buckets (every quarter, every
 * January, every second January…) so a label always sits under the
 * bucket it names, and showing years once the step reaches a year.
 * Day and week series spanning three months or more are labelled by
 * month the same way, on the first bucket of each labelled month;
 * shorter ones (and hours) are thinned evenly from the newest bucket.
 */
export function axisTicks(keys: string[], bucket: Bucket, max: number): Map<number, string> {
	const out = new Map<number, string>();
	const n = keys.length;
	max = Math.max(2, Math.floor(max));
	if (n === 0) return out;
	if (n <= max && bucket !== 'month' && bucket !== 'year') {
		keys.forEach((k, i) => out.set(i, bucketTick(k, bucket)));
		return out;
	}
	if (bucket === 'year') {
		const step = YEAR_STEPS.find((s) => Math.ceil(n / s) <= max) ?? YEAR_STEPS.at(-1)!;
		keys.forEach((k, i) => {
			if (+k % step === 0) out.set(i, k);
		});
		return out;
	}
	const months = keys.map((k) => +k.slice(0, 4) * 12 + (+k.slice(5, 7) - 1));
	const span = months[n - 1]! - months[0]! + 1;
	if (bucket === 'hour' || (bucket !== 'month' && span < 3)) {
		const step = Math.ceil(n / max);
		keys.forEach((k, i) => {
			if ((n - 1 - i) % step === 0) out.set(i, bucketTick(k, bucket));
		});
		return out;
	}
	// month, and long day or week series: label month starts.
	const step = MONTH_STEPS.find((s) => Math.ceil(span / s) <= max) ?? MONTH_STEPS.at(-1)!;
	months.forEach((m, i) => {
		if (m % step !== 0 || (i > 0 && months[i - 1] === m)) return;
		const year = Math.floor(m / 12);
		out.set(
			i,
			step >= 12 || m % 12 === 0 ? String(year) : monthOnly.format(new Date(year, m % 12, 1))
		);
	});
	return out;
}

export const BUCKET_NOUN: Record<Bucket, string> = {
	hour: 'hour',
	day: 'day',
	week: 'week',
	month: 'month',
	year: 'year'
};

const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto', style: 'short' });

/** "3 min ago", "yesterday", or a date for anything older than a week. */
export function ago(at: number, now = Date.now()) {
	const s = Math.round((at - now) / 1000);
	const abs = Math.abs(s);
	if (abs < 45) return 'just now';
	if (abs < 3600) return rtf.format(Math.round(s / 60), 'minute');
	if (abs < 86_400) return rtf.format(Math.round(s / 3600), 'hour');
	if (abs < 7 * 86_400) return rtf.format(Math.round(s / 86_400), 'day');
	return f().dayMonthYear.format(new Date(at));
}

export function clockTime(at: number) {
	return timeFmt.format(new Date(at));
}

export function dateAt(at: number) {
	return f().dayMonthYear.format(new Date(at));
}

export function fullDateTime(at: number) {
	return `${f().dayMonthYear.format(new Date(at))}, ${timeFmt.format(new Date(at))}`;
}

/** "Saturday, 17 October 2026" for a day heading. */
export function longDay(at: number) {
	return f().longDay.format(new Date(at));
}

/** Local calendar key of an instant ("2026-10-17"), for grouping by day. */
export function dayKey(at: number) {
	return iso(new Date(at));
}

/** "Oct 2026" for a local date string. */
export function monthYearOf(d: string) {
	return f().monthYear.format(parseLocalDate(d));
}

/** "9:41 PM – 1:07 AM" */
export function timeSpan(a: number, b: number) {
	return `${clockTime(a)} – ${clockTime(b)}`;
}
