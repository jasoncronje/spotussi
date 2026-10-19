<!--
	The full ranking, one page at a time. Every row carries both measures; the
	active one (Plays or Time) is bright and drives the order and the meter,
	the other stays quiet beside it. The meter is the row's share of the #1
	entry over the whole ranking, so the drop-off reads the same on page 9 as
	on page 1.
-->
<script lang="ts">
	import Artwork from '#lib/components/common/artwork.svelte';
	import Movement from '#lib/components/common/movement.svelte';
	import GenreGlyph from '#lib/components/common/genre-glyph.svelte';
	import { int, duration } from '#lib/format.js';
	import type { TopItem } from '#lib/components/overview/top-list.svelte';
	import type { Metric } from '#lib/ranges.js';

	let {
		items,
		metric,
		hasPrevious,
		leader,
		kind,
		caption
	}: {
		items: TopItem[];
		metric: Metric;
		hasPrevious: boolean;
		/** Value of the #1 entry (by metric), for the share meter. */
		leader: number;
		kind: 'artists' | 'tracks' | 'albums' | 'genres';
		caption: string;
	} = $props();

	const v = (i: TopItem) => (metric === 'plays' ? i.plays : i.ms);
	const top = $derived(Math.max(1, leader));
</script>

<table class="w-full border-collapse text-left">
	<caption class="sr-only">{caption}</caption>
	<thead class="text-xs text-muted-foreground">
		<tr class="border-b">
			<th scope="col" class="w-14 py-2 pr-2 text-right font-medium">Rank</th>
			<th scope="col" class="py-2 pl-3 font-medium">
				<span class="capitalize">{kind.slice(0, -1)}</span>
			</th>
			<th scope="col" class="hidden w-[22%] py-2 pl-4 font-medium lg:table-cell">
				<span class="sr-only">Share of the top entry</span>
			</th>
			<th
				scope="col"
				class="w-24 py-2 pl-3 text-right font-medium {metric === 'plays'
					? 'text-foreground'
					: 'max-sm:hidden'}">Plays</th
			>
			<th
				scope="col"
				class="w-28 py-2 pl-3 text-right font-medium {metric === 'time'
					? 'text-foreground'
					: 'max-sm:hidden'}">Time</th
			>
		</tr>
	</thead>
	<tbody>
		{#each items as item (item.id)}
			{@const share = v(item) / top}
			<tr class="group border-b border-border/60 last:border-b-0">
				<td class="py-2 pr-2 align-middle">
					<div class="flex flex-col items-end gap-1">
						<span class="figure tnum text-base leading-none text-foreground/80">{item.rank}</span>
						<Movement rank={item.rank} prevRank={item.prevRank} {hasPrevious} />
					</div>
				</td>
				<td class="relative py-2 pl-3 align-middle">
					<div class="flex min-w-0 items-center gap-3">
						{#if kind === 'genres'}
							<GenreGlyph level={share} class="size-11" />
						{:else}
							<Artwork src={item.image} name={item.name} round={item.round} class="size-11" />
						{/if}
						<div class="min-w-0">
							<a
								href={item.href}
								class="hit-above line-clamp-1 font-medium break-all after:absolute after:inset-0 hover:underline hover:decoration-signal hover:underline-offset-4"
								title={item.name}>{item.name}</a
							>
							{#if item.subs?.length}
								<p class="narrow relative z-10 truncate text-xs text-muted-foreground">
									{#each item.subs as s, k (s.id + k)}{k > 0 ? ', ' : ''}<a
											href={s.href}
											class="hit-below hover:text-foreground hover:underline">{s.name}</a
										>{/each}
								</p>
							{/if}
							<!-- Phones: the meter sits under the name. -->
							<div class="mt-1.5 h-[3px] w-full max-w-48 rounded-full bg-grille/70 lg:hidden">
								<div
									class="h-full rounded-full bg-signal"
									style="width: {Math.max(2, share * 100)}%"
								></div>
							</div>
						</div>
					</div>
				</td>
				<td class="hidden py-2 pl-4 align-middle lg:table-cell" aria-hidden="true">
					<div class="h-[5px] w-full rounded-full bg-grille/70">
						<div
							class="h-full rounded-full bg-signal transition-[width] duration-300"
							style="width: {Math.max(1, share * 100)}%"
						></div>
					</div>
				</td>
				<td
					class="tnum py-2 pl-3 text-right align-middle whitespace-nowrap {metric === 'plays'
						? 'font-medium'
						: 'text-muted-foreground max-sm:hidden'}"
				>
					{int(item.plays)}
				</td>
				<td
					class="tnum py-2 pl-3 text-right align-middle whitespace-nowrap {metric === 'time'
						? 'font-medium'
						: 'text-muted-foreground max-sm:hidden'}"
				>
					{duration(item.ms)}
				</td>
			</tr>
		{/each}
	</tbody>
</table>
