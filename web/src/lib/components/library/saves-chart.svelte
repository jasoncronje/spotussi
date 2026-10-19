<!--
	Songs saved over the years, drawn as the app's meter dots: one column per
	month, quarter or year (whichever keeps columns at least 7px wide), lit
	dots for songs saved, dim dots under the line for songs removed (known
	only since sync started). A table for screen readers carries the yearly
	totals.
-->
<script lang="ts">
	import type { Schemas } from '#lib/api/client.js';
	import { int } from '#lib/format.js';

	let { points }: { points: Schemas['LibraryTimelinePoint'][] } = $props();

	let width = $state(0);
	const ROWS = 9;
	const DOWN = 3;
	const PITCH = 9;

	type Col = { key: string; label: string; year: number; saved: number; removed: number };

	/** Monthly points grouped into n-month columns. */
	function group(months: number): Col[] {
		const out: Col[] = [];
		for (const p of points) {
			const y = +p.key.slice(0, 4);
			const m = +p.key.slice(5, 7) - 1;
			const k = `${y}-${Math.floor(m / months)}`;
			let c = out.at(-1);
			if (!c || c.key !== k) {
				const label =
					months === 12
						? String(y)
						: months === 3
							? `Q${Math.floor(m / 3) + 1} ${y}`
							: new Date(y, m, 1).toLocaleDateString(undefined, {
									month: 'short',
									year: 'numeric'
								});
				c = { key: k, label, year: y, saved: 0, removed: 0 };
				out.push(c);
			}
			c.saved += p.tracks_saved;
			c.removed += p.tracks_removed;
		}
		return out;
	}

	const cols = $derived.by(() => {
		for (const months of [1, 3, 12]) {
			const c = group(months);
			if (!width || width / c.length >= 7 || months === 12) return { months, cols: c };
		}
		return { months: 12, cols: group(12) };
	});
	const peak = $derived(Math.max(1, ...cols.cols.map((c) => c.saved)));
	const peakRemoved = $derived(Math.max(1, ...cols.cols.map((c) => c.removed)));
	const colW = $derived(width / Math.max(1, cols.cols.length));
	const r = $derived(Math.max(1.4, Math.min(2.6, colW * 0.3)));
	const base = ROWS * PITCH + 4;
	const height = base + DOWN * PITCH + 18;

	/** Year labels under the first column of a year, thinned to fit. */
	const ticks = $derived.by(() => {
		const years = [...new Set(cols.cols.map((c) => c.year))];
		const every = Math.max(1, Math.ceil((years.length * 34) / Math.max(1, width)));
		const out: { x: number; label: string }[] = [];
		let seen = -1;
		cols.cols.forEach((c, i) => {
			if (c.year === seen) return;
			seen = c.year;
			if ((c.year - years[0]!) % every === 0) out.push({ x: i * colW, label: String(c.year) });
		});
		return out;
	});
	const yearly = $derived(
		Object.entries(
			points.reduce<Record<string, { saved: number; removed: number }>>((a, p) => {
				const y = p.key.slice(0, 4);
				a[y] ??= { saved: 0, removed: 0 };
				a[y].saved += p.tracks_saved;
				a[y].removed += p.tracks_removed;
				return a;
			}, {})
		)
	);
</script>

<div bind:clientWidth={width} class="w-full">
	{#if width > 0}
		<svg {width} {height} class="block overflow-visible" aria-hidden="true">
			{#each cols.cols as c, i (c.key)}
				{@const x = i * colW + colW / 2}
				{@const lit = (c.saved / peak) * ROWS}
				{@const gone = (c.removed / peakRemoved) * DOWN}
				<g>
					<title
						>{c.label}: {int(c.saved)} saved{c.removed ? `, ${int(c.removed)} removed` : ''}</title
					>
					<rect x={i * colW} y="0" width={colW} height={base + DOWN * PITCH} fill="transparent" />
					{#each { length: ROWS } as _, k (k)}
						{@const o = Math.max(0, Math.min(1, lit - k))}
						<circle cx={x} cy={base - 4 - k * PITCH - PITCH / 2} {r} class="fill-grille" />
						{#if o > 0 && c.saved > 0}
							<circle
								cx={x}
								cy={base - 4 - k * PITCH - PITCH / 2}
								{r}
								class="fill-signal"
								opacity={Math.max(o, 0.35)}
							/>
						{/if}
					{/each}
					{#if c.removed > 0}
						{#each { length: Math.max(1, Math.round(gone)) } as _, k (k)}
							<circle
								cx={x}
								cy={base + 4 + k * PITCH + PITCH / 2}
								{r}
								class="fill-muted-foreground"
								opacity="0.55"
							/>
						{/each}
					{/if}
				</g>
			{/each}
			<line x1="0" x2={width} y1={base} y2={base} class="stroke-border" />
			{#each ticks as t (t.label)}
				<text x={t.x} y={height - 2} class="tnum fill-muted-foreground text-[11px]">{t.label}</text>
			{/each}
		</svg>
	{:else}
		<div style="height: {height}px"></div>
	{/if}
	<table class="sr-only">
		<caption>Songs saved and removed per year</caption>
		<thead
			><tr><th scope="col">Year</th><th scope="col">Saved</th><th scope="col">Removed</th></tr
			></thead
		>
		<tbody>
			{#each yearly as [y, v] (y)}
				<tr><th scope="row">{y}</th><td>{v.saved}</td><td>{v.removed}</td></tr>
			{/each}
		</tbody>
	</table>
</div>
