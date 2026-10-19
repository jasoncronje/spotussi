<!--
	Share of each bucket held by a few series (top artists or genres), as
	columns of lamp segments: one colour per series stacked from the bottom,
	unlit grille above. The scale tops out just above the busiest column
	(not at 100%), since a few artists rarely hold most of the listening and
	thin slivers would be unreadable; the axis says where it stops. Drawn in plain SVG (no chart
	library) so Stats stays light. Hover or focus a column for its numbers;
	the panel's table carries every value for screen readers.
-->
<script lang="ts">
	import { axisTicks, bucketLabel, duration, int, type Bucket } from '#lib/format.js';
	import type { Schemas } from '#lib/api/client.js';
	import type { Metric } from '#lib/ranges.js';

	let {
		series,
		points,
		bucket,
		metric
	}: {
		series: { id: string; name: string }[];
		points: Schemas['BreakdownPoint'][];
		bucket: Bucket;
		metric: Metric;
	} = $props();

	const uid = $props.id();
	const H = 216;
	const PAD = { top: 8, right: 4, bottom: 22, left: 36 };
	const SEG = 4;
	const GAP = 2;

	let width = $state(0);
	let hover = $state<number | null>(null);

	const iw = $derived(Math.max(0, width - PAD.left - PAD.right));
	const ih = H - PAD.top - PAD.bottom;
	const step = $derived(points.length ? iw / points.length : 0);
	const bw = $derived(
		Math.max(1, step * (points.length > 60 ? 0.8 : points.length > 24 ? 0.7 : 0.6))
	);
	const xTicks = $derived(
		axisTicks(
			points.map((p) => p.key),
			bucket,
			iw / 72
		)
	);

	/** Stacked shares (0..1) per point, series first then other. */
	const stacks = $derived(
		points.map((p) => {
			const t = p.total || 1;
			let acc = 0;
			const parts = p.values.map((v, i) => {
				const s = { i, y0: acc, y1: acc + v / t };
				acc = s.y1;
				return s;
			});
			return { parts, other: p.total ? Math.max(0, 1 - acc) : 0, empty: p.total === 0, sum: acc };
		})
	);
	/** Scale top: the busiest column's share, rounded up to 5%, at most 100%. */
	const top = $derived(
		Math.min(
			1,
			Math.max(0.05, Math.ceil(Math.max(0, ...stacks.map((s) => s.sum)) * 20 + 0.0001) / 20)
		)
	);
	const sy = (v: number) => PAD.top + ih - (v / top) * ih;

	/**
	 * One path per series (and one for the grille) instead of an element per
	 * column and part: 180 monthly columns of 5 parts cost ~40 ms to rebuild
	 * on a range switch (M7 pass). `only` draws a single column (hover).
	 */
	function paths(only: number | null = null) {
		const n = series.length;
		const d = Array.from({ length: n }, () => '');
		let grille = '';
		for (let k = 0; k < stacks.length; k++) {
			if (only !== null && k !== only) continue;
			const s = stacks[k]!;
			const x = PAD.left + k * step + (step - bw) / 2;
			grille += `M${x} ${PAD.top}h${bw}v${ih}h${-bw}Z`;
			if (s.empty) continue;
			for (const part of s.parts) {
				const h = Math.max(0, ((part.y1 - part.y0) / top) * ih);
				if (h <= 0 || part.i >= n) continue;
				d[part.i] += `M${x} ${sy(part.y1)}h${bw}v${h}h${-bw}Z`;
			}
		}
		return { grille, series: d };
	}
	const all = $derived(paths());
	const focused = $derived(hover === null ? null : paths(hover));

	const fmt = (n: number) =>
		metric === 'plays' ? `${int(n)} ${n === 1 ? 'play' : 'plays'}` : duration(n);
	const pct = (n: number, t: number) => (t ? `${Math.round((n / t) * 100)}%` : '–');

	function move(e: PointerEvent) {
		const rect = (e.currentTarget as SVGElement).getBoundingClientRect();
		const x = e.clientX - rect.left - PAD.left;
		const i = Math.floor(x / (step || 1));
		hover = i >= 0 && i < points.length ? i : null;
	}
	const hp = $derived(hover === null ? null : points[hover]);
	const tipLeft = $derived(
		hover === null ? 0 : Math.min(Math.max(PAD.left + hover * step + step / 2, 110), width - 110)
	);
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
			<defs>
				<pattern
					id="{uid}-seg"
					width="8"
					height={SEG + GAP}
					y={PAD.top + ih}
					patternUnits="userSpaceOnUse"
				>
					<rect y={GAP} width="8" height={SEG} fill="white" />
				</pattern>
				<mask id="{uid}-mask" maskUnits="userSpaceOnUse">
					<rect x="0" y="0" {width} height={H} fill="url(#{uid}-seg)" />
				</mask>
			</defs>
			{#each [0, top / 2, top] as t (t)}
				<text
					x={PAD.left - 8}
					y={sy(t)}
					dy="0.32em"
					text-anchor="end"
					class="tnum fill-muted-foreground text-2xs">{Math.round(t * 1000) / 10}%</text
				>
			{/each}
			<g mask="url(#{uid}-mask)">
				<g opacity={focused ? 0.45 : 1}>
					<path d={all.grille} class="fill-grille" />
					{#each all.series as d, i (i)}
						<path {d} style="fill: var(--chart-{(i % 5) + 1})" />
					{/each}
				</g>
				{#if focused}
					<path d={focused.grille} class="fill-grille" />
					{#each focused.series as d, i (i)}
						<path {d} style="fill: var(--chart-{(i % 5) + 1})" />
					{/each}
				{/if}
			</g>
			<line
				x1={PAD.left}
				x2={width - PAD.right}
				y1={PAD.top + ih + 0.5}
				y2={PAD.top + ih + 0.5}
				class="stroke-foreground/25"
			/>
			{#each points as _, k (k)}
				{#if xTicks.has(k)}
					<text
						x={PAD.left + k * step + step / 2}
						y={PAD.top + ih + 16}
						text-anchor="middle"
						class="fill-muted-foreground text-2xs">{xTicks.get(k)}</text
					>
				{/if}
			{/each}
		</svg>
	{/if}
	{#if hp}
		<div
			class="pointer-events-none absolute top-2 z-10 grid w-52 -translate-x-1/2 gap-1 rounded-lg border bg-popover px-3 py-2.5 text-xs shadow-[0_8px_24px_-8px_rgb(0_0_0/0.25)]"
			style="left: {tipLeft}px"
		>
			<p class="text-muted-foreground">{bucketLabel(hp.key, bucket)}</p>
			<p class="tnum text-sm font-semibold">{fmt(hp.total)}</p>
			{#each series as s, i (s.id)}
				<p class="tnum flex items-center gap-1.5">
					<span
						class="size-2.5 shrink-0 rounded-[2px]"
						style="background: var(--chart-{(i % 5) + 1})"
					></span>
					<span class="min-w-0 flex-1 truncate">{s.name}</span>
					<span class="text-muted-foreground">{pct(hp.values[i] ?? 0, hp.total)}</span>
				</p>
			{/each}
			<p class="tnum flex items-center gap-1.5 text-muted-foreground">
				<span class="size-2.5 shrink-0 rounded-[2px] bg-grille"></span>
				<span class="flex-1">Everything else</span>
				<span>{pct(hp.other, hp.total)}</span>
			</p>
		</div>
	{/if}
</div>
