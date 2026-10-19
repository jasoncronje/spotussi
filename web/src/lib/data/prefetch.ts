// The active page registers how to warm the cache for another range, so the
// shell's range picker can prefetch on hover/focus without knowing what the
// page reads.
import type { RangeSel, Metric } from '#lib/ranges.js';

type Prefetcher = { range?: (sel: RangeSel) => void; metric?: (m: Metric) => void };

let current: Prefetcher = {};

export function registerPrefetcher(p: Prefetcher) {
	current = p;
	return () => {
		if (current === p) current = {};
	};
}

export function prefetchRange(sel: RangeSel) {
	current.range?.(sel);
}

export function prefetchMetric(m: Metric) {
	current.metric?.(m);
}
