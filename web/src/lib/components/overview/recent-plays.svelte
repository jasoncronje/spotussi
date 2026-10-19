<!--
	The last plays, newest first, with the honest details: skipped plays show
	where you skipped, rows that don't pass the play rule say so, and estimated
	durations are marked.
-->
<script lang="ts">
	import Artwork from '#lib/components/common/artwork.svelte';
	import { ago, clockTime, fullDateTime, trackLength } from '#lib/format.js';
	import { artistHref, sectionHref, trackHref } from '#lib/state/view.svelte.js';
	import type { Schemas } from '#lib/api/client.js';

	let { plays }: { plays: Schemas['Play'][] } = $props();
	const now = Date.now();
</script>

<section aria-labelledby="recent-h">
	<header class="mb-2 flex items-baseline justify-between gap-3 border-b pb-2.5">
		<h2 id="recent-h" class="text-base font-semibold">Recently played</h2>
		<a
			href={sectionHref('/history', { preset: 'all' })}
			class="hit text-xs text-muted-foreground hover:text-foreground hover:underline"
			>Full history</a
		>
	</header>
	{#if plays.length === 0}
		<p class="py-6 text-sm text-muted-foreground">No plays yet.</p>
	{:else}
		<ol class="grid gap-x-10 md:grid-flow-col md:grid-cols-2 md:grid-rows-[repeat(5,auto)]">
			{#each plays.slice(0, 10) as p (p.id)}
				<li class="relative grid grid-cols-[auto_1fr_auto] items-center gap-x-3 py-2">
					<Artwork src={p.track.image_url} name={p.track.name} class="size-10" />
					<div class="min-w-0">
						<a
							href={trackHref(p.track.id)}
							class="hit-above block truncate font-medium after:absolute after:inset-0 hover:underline hover:decoration-signal hover:underline-offset-4"
							>{p.track.name}</a
						>
						<p class="narrow relative z-10 truncate text-xs text-muted-foreground">
							{#each p.track.artists as a, k (a.id)}{k > 0 ? ', ' : ''}<a
									href={artistHref(a.id)}
									class="hit-below hover:text-foreground hover:underline">{a.name}</a
								>{/each}
						</p>
					</div>
					<div
						class="tnum flex flex-col items-end gap-0.5 text-right text-xs text-muted-foreground"
					>
						<time datetime={new Date(p.played_at).toISOString()} title={fullDateTime(p.played_at)}>
							{now - p.played_at < 86_400_000 ? clockTime(p.played_at) : ago(p.played_at, now)}
						</time>
						{#if p.skipped}
							<span class="text-2xs text-faint">skipped at {trackLength(p.ms_played)}</span>
						{:else if !p.counts_as_play}
							<span class="text-2xs text-faint" title="Too short to count as a play"
								>not counted</span
							>
						{:else if p.ms_estimated}
							<span class="text-2xs text-faint" title="Duration estimated from the track length"
								>≈ {trackLength(p.ms_played)}</span
							>
						{/if}
					</div>
				</li>
			{/each}
		</ol>
	{/if}
</section>
