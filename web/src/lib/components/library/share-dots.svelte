<!--
	Play-through: how many of an album's or playlist's songs were ever
	played, as a row of dots at the meter's pitch (one per song while they
	fit, else a share of the row). Decorative: the figure beside it carries
	the value.
-->
<script lang="ts">
	let { played, total, max = 24 }: { played: number; total: number; max?: number } = $props();

	const dots = $derived(Math.max(1, Math.min(total, max)));
	const lit = $derived(total > 0 ? (played / total) * dots : 0);
</script>

<span class="flex items-center gap-[5px]" aria-hidden="true">
	{#each { length: dots } as _, i (i)}
		{@const o = Math.max(0, Math.min(1, lit - i))}
		<span class="relative size-[5px] shrink-0 rounded-full bg-grille">
			{#if o > 0}<span class="absolute inset-0 rounded-full bg-signal" style="opacity: {o}"
				></span>{/if}
		</span>
	{/each}
</span>
