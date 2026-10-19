<!--
	Genres have no artwork, so they get a 4×4 dot level instead: lit dots show
	`level` (0..1), filled bottom-up column by column. Decorative.
-->
<script lang="ts">
	let { level, class: className = 'size-10' }: { level: number; class?: string } = $props();
	const lit = $derived(Math.max(1, Math.round(Math.min(1, Math.max(0, level)) * 16)));
</script>

<span
	class="grid shrink-0 place-items-center rounded-[4px] bg-muted {className}"
	aria-hidden="true"
>
	<svg viewBox="0 0 28 28" class="size-[60%]">
		{#each { length: 16 } as _, n (n)}
			<circle
				cx={Math.floor(n / 4) * 7 + 3.5}
				cy={(3 - (n % 4)) * 7 + 3.5}
				r="2.2"
				class={n < lit ? 'fill-signal' : 'fill-grille'}
			/>
		{/each}
	</svg>
</span>
