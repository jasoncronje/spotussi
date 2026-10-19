<!--
	A bar-graph meter of lit segments, like the LED ladder on a tape deck:
	the import's progress readout. The segment at the leading edge lights
	partially, so slow progress still moves. `value` null means not yet
	measurable (a queued job): the ladder stays unlit and the progressbar
	reports no value.
-->
<script lang="ts">
	let {
		value,
		label,
		valuetext,
		segments = 40
	}: {
		/** 0..1, or null while unknown. */
		value: number | null;
		label: string;
		valuetext?: string;
		segments?: number;
	} = $props();

	const level = $derived(value === null ? 0 : Math.max(0, Math.min(1, value)) * segments);
	const pct = $derived(value === null ? undefined : Math.round(Math.min(1, value) * 100));
</script>

<div
	role="progressbar"
	aria-label={label}
	aria-valuemin={0}
	aria-valuemax={100}
	aria-valuenow={pct}
	aria-valuetext={value === null ? undefined : valuetext}
	class="grid h-3 w-full gap-[3px]"
	style="grid-template-columns: repeat({segments}, minmax(0, 1fr))"
>
	{#each { length: segments } as _, i (i)}
		{@const o = Math.max(0, Math.min(1, level - i))}
		<span class="relative rounded-[2px] bg-grille">
			{#if o > 0}
				<span
					class="absolute inset-0 rounded-[2px] bg-signal transition-opacity duration-300"
					style="opacity: {o}"
				></span>
			{/if}
		</span>
	{/each}
</div>
