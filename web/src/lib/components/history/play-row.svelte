<!--
	One play in a log (History, a session, a track page). Says exactly what
	is known about it: when, how long you actually listened, whether it was
	skipped or too short to count, whether the time is estimated from the
	track length (D7), where it was played from, and which sources saw it
	(D8): Live capture, Recently played, and the Export. A live-only play the
	export later contradicts is flagged.
-->
<script lang="ts" module>
	import type { Schemas } from '#lib/api/client.js';
	export const SOURCES: { key: Schemas['PlaySource']; label: string; hint: string }[] = [
		{ key: 'poller', label: 'Live', hint: 'Seen while playing (live capture)' },
		{ key: 'recent', label: 'Recent', hint: "From Spotify's recently played list" },
		{ key: 'export', label: 'Export', hint: 'In your streaming history export' }
	];
</script>

<script lang="ts">
	import Shuffle from '@lucide/svelte/icons/shuffle';
	import Artwork from '#lib/components/common/artwork.svelte';
	import { albumHref, artistHref, trackHref } from '#lib/state/view.svelte.js';
	import { clockTime, dateAt, fullDateTime, trackLength } from '#lib/format.js';

	let {
		play: p,
		withDate = false,
		index
	}: {
		play: Schemas['Play'];
		/** Show the date as well as the time (lists not grouped by day). */
		withDate?: boolean;
		/** Position in a session, shown instead of artwork-only rows. */
		index?: number;
	} = $props();

	const sources = $derived(SOURCES.filter((s) => p.sources.includes(s.key)));
	const sourceText = $derived(sources.map((s) => s.label).join(' + '));
	const where = $derived(
		[
			p.device?.name ?? p.platform,
			p.context?.name ?? (p.context?.type === 'collection' ? 'Liked Songs' : null)
		]
			.filter(Boolean)
			.join(', ')
	);
	const len = $derived(p.track.duration_ms);
</script>

<li
	class="grid items-center gap-x-3 py-2 {withDate
		? 'grid-cols-[5.5rem_auto_minmax(0,1fr)_auto] sm:grid-cols-[6.5rem_auto_minmax(0,1fr)_auto_auto]'
		: 'grid-cols-[3.75rem_auto_minmax(0,1fr)_auto] sm:grid-cols-[4.5rem_auto_minmax(0,1fr)_auto_auto]'} {p.counts_as_play
		? ''
		: 'text-muted-foreground'}"
>
	<div class="tnum text-xs leading-tight text-muted-foreground">
		{#if index !== undefined}<span class="figure block text-sm text-foreground/80">{index}</span
			>{/if}
		<time datetime={new Date(p.played_at).toISOString()} title={fullDateTime(p.played_at)}>
			{#if withDate}<span class="block">{dateAt(p.played_at)}</span>{/if}
			{clockTime(p.played_at)}
		</time>
	</div>
	<Artwork
		src={p.track.image_url}
		name={p.track.name}
		class="size-10 {p.counts_as_play ? '' : 'opacity-60'}"
	/>
	<div class="min-w-0">
		<a
			href={trackHref(p.track.id)}
			class="hit-above line-clamp-1 font-medium break-all hover:underline hover:decoration-signal hover:underline-offset-4"
			>{p.track.name}</a
		>
		<p class="narrow truncate text-xs text-muted-foreground">
			{#each p.track.artists as a, k (a.id)}{k > 0 ? ', ' : ''}<a
					href={artistHref(a.id)}
					class="hit-below hover:text-foreground hover:underline">{a.name}</a
				>{/each}{#if p.track.album}<span class="max-md:hidden"
					>, <a
						href={albumHref(p.track.album.id)}
						class="hit-below hover:text-foreground hover:underline">{p.track.album.name}</a
					></span
				>{/if}
		</p>
		<p class="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-2xs text-muted-foreground">
			{#if p.skipped}
				<span class="rounded-sm bg-muted px-1.5 py-px font-medium text-foreground/80">Skipped</span>
			{/if}
			{#if !p.counts_as_play}
				<span
					class="rounded-sm bg-muted px-1.5 py-px"
					title="Too short to count as a play under your play rule">Not counted</span
				>
			{/if}
			{#if p.contradicted}
				<span
					class="rounded-sm bg-destructive/10 px-1.5 py-px font-medium text-destructive"
					title="Live capture saw this play but your export doesn't list it">Not in export</span
				>
			{/if}
			{#if p.shuffle}
				<span class="inline-flex items-center gap-1"
					><Shuffle class="size-3" aria-hidden="true" />Shuffle</span
				>
			{/if}
			{#if where}<span class="truncate max-sm:max-w-40">{where}</span>{/if}
		</p>
	</div>
	<!-- Sources (D8): three lamps in a fixed order, lit for each source. -->
	<span
		class="hidden items-center gap-1.5 text-2xs text-muted-foreground sm:inline-flex"
		title={sources.map((s) => s.hint).join('; ')}
	>
		<span class="inline-flex gap-[3px]" aria-hidden="true">
			{#each SOURCES as s (s.key)}
				<span
					class="size-[5px] rounded-full {p.sources.includes(s.key) ? 'bg-signal' : 'bg-grille'}"
				></span>
			{/each}
		</span>
		<span class="sr-only">Sources: </span>{sourceText}
	</span>
	<div class="tnum text-right text-xs whitespace-nowrap">
		{#if p.ms_estimated}
			<span
				title="Estimated from the track length: this play was only seen after it finished"
				class="inline-flex items-center gap-1"
			>
				<span
					aria-hidden="true"
					class="inline-block size-2.5 rounded-[2px]"
					style="background: repeating-linear-gradient(0deg, var(--signal-dim) 0 3px, transparent 3px 4.5px)"
				></span>
				≈ {trackLength(p.ms_played)}<span class="sr-only"> (estimated)</span>
			</span>
		{:else}
			{trackLength(p.ms_played)}
		{/if}
		{#if len && !p.ms_estimated}
			<span class="block text-2xs text-faint">of {trackLength(len)}</span>
		{/if}
		<span class="mt-0.5 flex justify-end gap-[3px] sm:hidden" aria-hidden="true">
			{#each SOURCES as s (s.key)}
				<span
					class="size-[4px] rounded-full {p.sources.includes(s.key) ? 'bg-signal' : 'bg-grille'}"
				></span>
			{/each}
		</span>
	</div>
</li>
