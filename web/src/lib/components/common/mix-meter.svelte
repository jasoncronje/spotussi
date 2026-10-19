<!--
	How a total splits by where it came from: one row of meter dots, each
	part a run of dots in its own tone (largest remainder, so the runs always
	fill the row), and a legend with each part's share. A part that is all of
	it collapses the legend to one line.
-->
<script lang="ts" module>
	export type MixPart = {
		key: string;
		label: string;
		/** Said instead of the legend when this part is everything. */
		all: string;
		value: number;
		/** Dot class: bg-signal, bg-signal-dim, bg-grille, or a chart colour. */
		tone: string;
	};
</script>

<script lang="ts">
	import Hint from './hint.svelte';

	let {
		label,
		parts,
		hint,
		dots = 30
	}: { label: string; parts: MixPart[]; hint?: string; dots?: number } = $props();

	const total = $derived(parts.reduce((a, p) => a + p.value, 0));
	/** Dots per part by largest remainder; any non-zero part gets at least one. */
	const runs = $derived.by(() => {
		if (!total) return parts.map(() => 0);
		const exact = parts.map((p) => (p.value / total) * dots);
		const n = exact.map((x, i) => (parts[i]!.value > 0 ? Math.max(1, Math.floor(x)) : 0));
		let left = dots - n.reduce((a, b) => a + b, 0);
		const order = exact
			.map((x, i) => ({ i, r: x - Math.floor(x) }))
			.sort((a, b) => b.r - a.r)
			.map((o) => o.i);
		for (let k = 0; left > 0 && k < order.length * 2; k++) {
			const i = order[k % order.length]!;
			if (parts[i]!.value > 0) {
				n[i]!++;
				left--;
			}
		}
		while (left < 0) {
			const i = n.indexOf(Math.max(...n));
			n[i]!--;
			left++;
		}
		return n;
	});
	const tones = $derived(runs.flatMap((r, i) => Array(r).fill(parts[i]!.tone) as string[]));
	const pct = (v: number) => {
		const p = (v / (total || 1)) * 100;
		return p > 0 && p < 1 ? '<1%' : `${Math.round(p)}%`;
	};
	const shown = $derived(parts.filter((p) => p.value > 0));
</script>

<div class="grid min-w-0 gap-2">
	<p class="flex items-center gap-1.5 text-xs text-muted-foreground">
		{label}{#if hint}<Hint text={hint} />{/if}
	</p>
	<div class="flex w-full max-w-80 justify-between" aria-hidden="true">
		{#each tones as t, i (i)}
			<span class="size-[5px] rounded-full {t}"></span>
		{/each}
	</div>
	<ul class="tnum flex flex-wrap gap-x-3.5 gap-y-1 text-xs">
		{#if shown.length === 1}
			<li class="flex items-center gap-1.5">
				<span class="size-[5px] rounded-full {shown[0]!.tone}" aria-hidden="true"></span>
				{shown[0]!.all}
			</li>
		{:else}
			{#each shown as p (p.key)}
				<li class="flex items-center gap-1.5">
					<span class="size-[5px] rounded-full {p.tone}" aria-hidden="true"></span>
					<span class="text-muted-foreground">{p.label}</span>
					<span>{pct(p.value)}</span>
				</li>
			{/each}
		{/if}
	</ul>
</div>
