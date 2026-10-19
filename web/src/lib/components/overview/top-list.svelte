<!--
	A top-5 list. Each row drills into its entity; the thin meter under a name
	is its share of the #1 entry, so the gap between ranks is visible.
-->
<script lang="ts">
	import Artwork from '#lib/components/common/artwork.svelte';
	import Movement from '#lib/components/common/movement.svelte';
	import GenreGlyph from '#lib/components/common/genre-glyph.svelte';
	import { int, duration } from '#lib/format.js';
	import type { Metric } from '#lib/ranges.js';

	export type TopItem = {
		id: string;
		href: string;
		name: string;
		image?: string | null;
		round?: boolean;
		/** Secondary links (a track's or album's artists). */
		subs?: { id: string; name: string; href: string }[];
		rank: number;
		prevRank: number | null;
		plays: number;
		ms: number;
	};

	let {
		title,
		allHref,
		items,
		total,
		metric,
		hasPrevious,
		kind,
		empty = 'Nothing in this range.'
	}: {
		title: string;
		allHref: string;
		items: TopItem[];
		total: number;
		metric: Metric;
		hasPrevious: boolean;
		kind: 'artists' | 'tracks' | 'albums' | 'genres';
		/** Said when the list is empty. */
		empty?: string;
	} = $props();

	const uid = $props.id();
	const v = (i: TopItem) => (metric === 'plays' ? i.plays : i.ms);
	const top = $derived(Math.max(1, ...items.map(v)));
</script>

<section aria-labelledby="{uid}-h" class="min-w-0">
	<header class="mb-2 flex items-baseline justify-between gap-3 border-b pb-2.5">
		<h2 id="{uid}-h" class="text-base font-semibold">{title}</h2>
		<a
			href={allHref}
			class="hit tnum text-xs text-muted-foreground hover:text-foreground hover:underline"
			>All {int(total)}<span class="sr-only"> {kind}</span></a
		>
	</header>
	{#if items.length === 0}
		<p class="py-6 text-sm text-muted-foreground">{empty}</p>
	{:else}
		<ol>
			{#each items as item (item.id)}
				{@const share = v(item) / top}
				<li
					class="group relative grid grid-cols-[1.25rem_auto_1fr_auto] items-center gap-x-3 rounded-lg py-2 pr-1"
				>
					<span class="tnum text-right text-xs text-faint" aria-hidden="true">{item.rank}</span>
					{#if kind === 'genres'}
						<GenreGlyph level={share} />
					{:else}
						<Artwork src={item.image} name={item.name} round={item.round} class="size-10" />
					{/if}
					<div class="min-w-0">
						<a
							href={item.href}
							class="hit-above block truncate font-medium after:absolute after:inset-0 after:rounded-lg hover:underline hover:decoration-signal hover:underline-offset-4"
							title={item.name}>{item.name}</a
						>
						{#if item.subs?.length}
							<p class="narrow relative z-10 truncate text-xs text-muted-foreground">
								{#each item.subs as s, k (s.id)}{k > 0 ? ', ' : ''}<a
										href={s.href}
										class="hit-below hover:text-foreground hover:underline">{s.name}</a
									>{/each}
							</p>
						{/if}
						<div class="mt-1.5 h-[3px] w-full rounded-full bg-grille/70" aria-hidden="true">
							<div
								class="h-full rounded-full bg-signal transition-[width] duration-300"
								style="width: {Math.max(2, share * 100)}%"
							></div>
						</div>
					</div>
					<div class="flex flex-col items-end gap-1 pl-1">
						<span class="tnum text-sm font-medium whitespace-nowrap">
							{metric === 'plays' ? int(item.plays) : duration(item.ms)}
							<span class="sr-only">{metric === 'plays' ? 'plays' : 'listened'}</span>
						</span>
						<Movement rank={item.rank} prevRank={item.prevRank} {hasPrevious} />
					</div>
				</li>
			{/each}
		</ol>
	{/if}
</section>
