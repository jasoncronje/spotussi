<!--
	One saved song or playlist item: artwork, title and artists, when it was
	saved, its plays as rotation lamps, and the play that took it into
	heavy rotation (or why it has no plays).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { Schemas } from '#lib/api/client.js';
	import Artwork from '#lib/components/common/artwork.svelte';
	import RotationLamps from './rotation-lamps.svelte';
	import { trackHref, artistHref } from '#lib/state/view.svelte.js';
	import { dateAt, int } from '#lib/format.js';

	let {
		track,
		status,
		plays,
		addedAt,
		addedLabel = 'Saved',
		lastPlayedAt = null,
		quiet = false,
		rotationAt = null,
		lead,
		trail
	}: {
		track: Schemas['TrackRef'];
		status: 'played' | 'maybe_played' | 'never_played';
		plays: number;
		addedAt: number | null;
		addedLabel?: string;
		lastPlayedAt?: number | null;
		/** Leave out the "never played" note (a list of only those). */
		quiet?: boolean;
		rotationAt?: number | null;
		/** Before the artwork (a playlist position). */
		lead?: Snippet;
		/** Under the plays (plays from this playlist). */
		trail?: Snippet;
	} = $props();

	const DAY = 86_400_000;
	function lag(from: number, to: number) {
		const d = Math.round((to - from) / DAY);
		if (d < 0) return 'before saving';
		if (d === 0) return 'the day it was saved';
		return d === 1 ? '1 day after saving' : `${int(d)} days after saving`;
	}
	const note = $derived.by(() => {
		if (status === 'never_played') return quiet ? '' : 'Never played';
		if (status === 'maybe_played') return 'Played as another release';
		if (rotationAt)
			return addedAt ? `In rotation ${lag(addedAt, rotationAt)}` : 'In heavy rotation';
		if (lastPlayedAt) return `Last played ${dateAt(lastPlayedAt)}`;
		return '';
	});
</script>

<li class="relative border-b last:border-b-0 hover:bg-muted/40">
	<div
		class="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-3 px-4 py-3 sm:grid-cols-[auto_minmax(0,1fr)_minmax(0,12rem)_7.5rem] sm:gap-x-5 sm:px-6"
	>
		<div class="flex items-center gap-3">
			{@render lead?.()}
			<Artwork
				src={track.image_url}
				name={track.album?.name ?? track.name}
				class="size-10 rounded-md"
			/>
		</div>
		<div class="min-w-0">
			<a
				href={trackHref(track.id)}
				class="hit-above block truncate font-medium after:absolute after:inset-0 hover:underline hover:decoration-signal hover:underline-offset-4"
				>{track.name}</a
			>
			<p class="narrow truncate text-xs text-muted-foreground">
				{#each track.artists as a, i (a.id)}{i > 0 ? ', ' : ''}<a
						href={artistHref(a.id)}
						class="hit-below relative z-10 hover:text-foreground hover:underline">{a.name}</a
					>{/each}
			</p>
		</div>
		<p class="narrow tnum hidden text-xs text-muted-foreground sm:block">
			{#if addedAt}{addedLabel} {dateAt(addedAt)}{:else}{addedLabel} date unknown{/if}
			{#if note}<br /><span class={status === 'never_played' ? 'text-foreground' : ''}>{note}</span
				>{/if}
		</p>
		<div class="grid justify-items-end gap-1.5">
			<RotationLamps {plays} maybe={status === 'maybe_played'} />
			<p class="tnum text-xs">
				<span class="figure">{int(plays)}</span>
				<span class="text-muted-foreground">{plays === 1 ? 'play' : 'plays'}</span>
			</p>
			{@render trail?.()}
		</div>
		<p class="narrow tnum col-span-2 col-start-2 mt-1 text-xs text-muted-foreground sm:hidden">
			{#if addedAt}{addedLabel} {dateAt(addedAt)}{/if}{#if note}{addedAt ? '. ' : ''}<span
					class={status === 'never_played' ? 'text-foreground' : ''}>{note}</span
				>{/if}
		</p>
	</div>
</li>
