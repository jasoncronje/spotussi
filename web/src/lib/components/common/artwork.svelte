<!--
	Cover art or artist image. While the catalog is still hydrating (D9) the URL
	is null: show initials on a quiet tile instead of a broken image.
	Fixed box from the class, so images never shift layout as they load.
-->
<script lang="ts">
	let {
		src,
		name,
		round = false,
		class: className = 'size-10',
		eager = false
	}: {
		src: string | null | undefined;
		name: string;
		round?: boolean;
		class?: string;
		eager?: boolean;
	} = $props();

	let failed = $state(false);
	const initials = $derived(
		name
			.replace(/[^\p{L}\p{N} ]/gu, '')
			.split(/\s+/)
			.filter(Boolean)
			.slice(0, 2)
			.map((w) => w[0]!.toUpperCase())
			.join('')
	);
</script>

<span
	class="relative block shrink-0 overflow-hidden bg-muted {round
		? 'rounded-full'
		: 'rounded-[4px]'} {className}"
>
	{#if src && !failed}
		<img
			{src}
			alt=""
			loading={eager ? 'eager' : 'lazy'}
			decoding="async"
			width="300"
			height="300"
			onerror={() => (failed = true)}
			class="absolute inset-0 size-full object-cover"
		/>
	{:else}
		<!-- No artwork yet (names only, or provisional without a picture):
		     an unlit grille with the initials, sized to the tile. -->
		<span class="unlit @container absolute inset-0 grid place-items-center" aria-hidden="true">
			<span
				class="font-semibold tracking-wide text-muted-foreground"
				style="font-size: clamp(0.625rem, 28cqi, 2.75rem)">{initials}</span
			>
		</span>
	{/if}
	<!-- hairline inner edge so pale covers don't melt into the panel -->
	<span
		class="pointer-events-none absolute inset-0 shadow-[inset_0_0_0_1px_rgb(0_0_0/0.08)] dark:shadow-[inset_0_0_0_1px_rgb(255_255_255/0.06)] {round
			? 'rounded-full'
			: 'rounded-[4px]'}"
	></span>
</span>

<style>
	.unlit {
		background-image: radial-gradient(
			circle,
			color-mix(in oklab, var(--grille) 70%, transparent) 1px,
			transparent 1.4px
		);
		background-size: 6px 6px;
		background-position: center;
	}
</style>
