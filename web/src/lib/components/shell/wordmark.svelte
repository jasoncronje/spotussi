<!--
	The mark: a 4×4 dot-matrix level meter (lit heights 2·4·3·1) beside the
	wordmark set in Archivo at its widest. Deliberately nothing circular or
	wave-like, so it can't be mistaken for any streaming service's logo.
	Geometry matches brand/logo (32-unit grid: lamps 5.5 across, 7 apart).
-->
<script lang="ts">
	let { compact = false }: { compact?: boolean } = $props();
	const heights = [2, 4, 3, 1];
</script>

<span class="inline-flex items-center gap-2.5">
	<svg viewBox="2.75 2.75 26.5 26.5" class="size-5 shrink-0" aria-hidden="true">
		{#each heights as h, c (c)}
			{#each [0, 1, 2, 3] as r (r)}
				<circle
					cx={5.5 + c * 7}
					cy={5.5 + r * 7}
					r="2.75"
					class={3 - r < h ? 'fill-signal' : 'fill-grille'}
				/>
			{/each}
		{/each}
	</svg>
	{#if !compact}
		<span class="text-[1.0625rem] leading-none font-[760] tracking-[-0.035em] [font-stretch:125%]">
			spotussi
		</span>
	{/if}
</span>
