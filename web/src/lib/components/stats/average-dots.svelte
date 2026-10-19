<!--
	A weighted average over time (release year, artists per track): one lamp
	per bucket at its value, joined by a hairline, with the whole range's
	average as a dashed reference. A lamp's brightness is the bucket's
	coverage (how much of its listening had the attribute known), so thin
	evidence looks thin. Empty buckets leave a gap. Plain SVG.
-->
<script lang="ts">
	import { axisTicks, bucketLabel, type Bucket } from '#lib/format.js';
	import type { Schemas } from '#lib/api/client.js';

	let {
		points,
		overall,
		bucket,
		format,
		step: yStep = 1
	}: {
		points: Schemas['AveragePoint'][];
		overall: number | null;
		bucket: Bucket;
		format: (v: number) => string;
		/** Tick spacing is a multiple of this (years: 1, ratios: 0.05). */
		step?: number;
	} = $props();

	const H = 200;
	const PAD = { top: 14, right: 10, bottom: 22, left: 44 };
	let width = $state(0);
	let hover = $state<number | null>(null);

	const iw = $derived(Math.max(0, width - PAD.left - PAD.right));
	const ih = H - PAD.top - PAD.bottom;
	const xstep = $derived(points.length ? iw / points.length : 0);
	const xTicks = $derived(
		axisTicks(
			points.map((p) => p.key),
			bucket,
			iw / 72
		)
	);

	const domain = $derived.by(() => {
		const vs = points.map((p) => p.value).filter((v): v is number => v !== null);
		if (overall !== null) vs.push(overall);
		if (!vs.length) return { lo: 0, hi: 1, ticks: [0, 1] };
		let lo = Math.min(...vs);
		let hi = Math.max(...vs);
		const span = Math.max(hi - lo, yStep * 4);
		const raw = span / 3;
		const mags = [1, 2, 2.5, 5, 10].map(
			(m) => m * yStep * Math.pow(10, Math.floor(Math.log10(raw / yStep)))
		);
		const t = mags.find((m) => m >= raw) ?? raw;
		lo = Math.floor((lo - span * 0.1) / t) * t;
		hi = Math.ceil((hi + span * 0.1) / t) * t;
		const ticks: number[] = [];
		for (let v = lo; v <= hi + t / 2; v += t) ticks.push(+v.toFixed(6));
		return { lo, hi, ticks };
	});
	const y = (v: number) => PAD.top + ih - ((v - domain.lo) / (domain.hi - domain.lo || 1)) * ih;
	const x = (i: number) => PAD.left + i * xstep + xstep / 2;

	/** Line segments between consecutive non-empty buckets. */
	const path = $derived.by(() => {
		let d = '';
		let pen = false;
		points.forEach((p, i) => {
			if (p.value === null) {
				pen = false;
				return;
			}
			d += `${pen ? 'L' : 'M'}${x(i).toFixed(1)},${y(p.value).toFixed(1)}`;
			pen = true;
		});
		return d;
	});

	function move(e: PointerEvent) {
		const rect = (e.currentTarget as SVGElement).getBoundingClientRect();
		const i = Math.floor((e.clientX - rect.left - PAD.left) / (xstep || 1));
		hover = i >= 0 && i < points.length && points[i]!.value !== null ? i : null;
	}
	const hp = $derived(hover === null ? null : points[hover]);
	const r = $derived(Math.max(2, Math.min(4, xstep / 3)));
</script>

<div class="relative px-2 pb-3 sm:px-3" bind:clientWidth={width}>
	{#if width > 0}
		<svg
			{width}
			height={H}
			aria-hidden="true"
			onpointermove={move}
			onpointerleave={() => (hover = null)}
			class="block touch-pan-y"
		>
			{#each domain.ticks as t (t)}
				<line
					x1={PAD.left}
					x2={width - PAD.right}
					y1={y(t)}
					y2={y(t)}
					class="stroke-border"
					shape-rendering="crispEdges"
				/>
				<text
					x={PAD.left - 10}
					y={y(t)}
					dy="0.32em"
					text-anchor="end"
					class="tnum fill-muted-foreground text-2xs">{format(t)}</text
				>
			{/each}
			{#if overall !== null}
				<line
					x1={PAD.left}
					x2={width - PAD.right}
					y1={y(overall)}
					y2={y(overall)}
					class="stroke-signal"
					stroke-dasharray="2 4"
					stroke-width="1.5"
				/>
			{/if}
			<path d={path} fill="none" class="stroke-foreground/30" stroke-width="1" />
			{#each points as p, i (i)}
				{#if p.value !== null}
					<circle
						cx={x(i)}
						cy={y(p.value)}
						r={hover === i ? r + 1.5 : r}
						class="fill-signal"
						fill-opacity={0.3 + 0.7 * Math.max(0, Math.min(1, p.coverage))}
					/>
				{/if}
				{#if xTicks.has(i)}
					<text
						x={x(i)}
						y={PAD.top + ih + 16}
						text-anchor="middle"
						class="fill-muted-foreground text-2xs">{xTicks.get(i)}</text
					>
				{/if}
			{/each}
		</svg>
	{/if}
	{#if hp && hover !== null && hp.value !== null}
		<div
			class="pointer-events-none absolute z-10 grid min-w-40 -translate-x-1/2 -translate-y-full gap-0.5 rounded-lg border bg-popover px-3 py-2 text-xs shadow-[0_8px_24px_-8px_rgb(0_0_0/0.25)]"
			style="left: {Math.min(Math.max(x(hover) + 12, 90), width - 90)}px; top: {y(hp.value) - 8}px"
		>
			<p class="text-muted-foreground">{bucketLabel(hp.key, bucket)}</p>
			<p class="tnum text-sm font-semibold">{format(hp.value)}</p>
			<p class="tnum text-muted-foreground">{Math.round(hp.coverage * 100)}% of listening known</p>
		</div>
	{/if}
</div>
