<!--
	A saved song's way into heavy rotation (D36): one lamp per counted play
	up to the 5th, which is heavy rotation. A song never played as this
	release but played under another one ("maybe played") shows its first
	lamp as a ring. Decorative: the plays figure beside it carries the value.
-->
<script lang="ts">
	let {
		plays,
		maybe = false,
		of = 5,
		class: className = ''
	}: { plays: number; maybe?: boolean; of?: number; class?: string } = $props();

	const lit = $derived(Math.min(plays, of));
</script>

<span class="inline-flex items-center gap-[5px] {className}" aria-hidden="true">
	{#each { length: of } as _, i (i)}
		{#if i < lit}
			<span
				class="size-[7px] rounded-full bg-signal {lit === of
					? 'shadow-[0_0_0_2.5px_var(--signal-soft)]'
					: ''}"
			></span>
		{:else if maybe && i === 0}
			<span class="size-[7px] rounded-full border-[1.5px] border-signal-dim"></span>
		{:else}
			<span class="size-[7px] rounded-full bg-grille"></span>
		{/if}
	{/each}
</span>
