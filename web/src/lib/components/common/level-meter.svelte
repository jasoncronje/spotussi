<!--
	Horizontal dot meter: lit dots show this period, the tick shows where the
	previous period landed (a peak-hold mark). Scaled so the larger of the two
	sits at ~85%, leaving headroom. Dots keep a fixed size at any width.
	Decorative: the figures beside it carry the values.
-->
<script lang="ts">
	let {
		value,
		previous,
		dots = 28
	}: { value: number; previous: number | null; dots?: number } = $props();

	const top = $derived(Math.max(value, previous ?? 0) / 0.85 || 1);
	const level = $derived((value / top) * dots);
	const mark = $derived(previous == null ? null : (previous / top) * 100);
</script>

<div class="relative h-3.5 w-full max-w-72" aria-hidden="true">
	<div class="grid h-full items-center" style="grid-template-columns: repeat({dots}, 1fr)">
		{#each { length: dots } as _, i (i)}
			{@const o = Math.max(0, Math.min(1, level - i))}
			<span class="relative mx-auto size-[5px] rounded-full bg-grille">
				{#if o > 0}
					<span
						class="absolute inset-0 rounded-full bg-signal transition-opacity duration-300"
						style="opacity: {o}"
					></span>
				{/if}
			</span>
		{/each}
	</div>
	{#if mark !== null}
		<span
			class="absolute top-0 h-full w-0.5 -translate-x-1/2 rounded-full bg-foreground"
			style="left: {mark}%"
		></span>
	{/if}
</div>
