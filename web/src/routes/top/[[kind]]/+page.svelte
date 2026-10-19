<!--
	Top lists (D14): tracks, artists, albums and genres, ranked by plays or
	time with no cap on length, each row moving against the previous period.
	Optionally narrowed to one artist, album or genre (from entity pages).
	Page, kind and scope live in the URL; the previous page stays on screen,
	dimmed, while the next one loads.
-->
<script lang="ts">
	import Hint from '#lib/components/common/hint.svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import X from '@lucide/svelte/icons/x';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import Lamp from '#lib/components/shell/lamp.svelte';
	import SectionError from '#lib/components/common/section-error.svelte';
	import EmptyState from '#lib/components/common/empty-state.svelte';
	import Pager from '#lib/components/common/pager.svelte';
	import RankedTable from '#lib/components/top/ranked-table.svelte';
	import { Resource } from '#lib/data/resource.svelte.js';
	import { prefetch, type Query } from '#lib/data/cache.js';
	import { registerPrefetcher } from '#lib/data/prefetch.js';
	import {
		albumQuery,
		artistQuery,
		genreQuery,
		topAlbumsQuery,
		topArtistsQuery,
		topGenresQuery,
		topTracksQuery,
		type TopKind
	} from '#lib/data/queries.js';
	import { albumItem, artistItem, genreItem, trackItem } from '#lib/data/items.js';
	import { view, hrefWith, rangeSearch } from '#lib/state/view.svelte.js';
	import { resolvedRange } from '#lib/state/resolved.svelte.js';
	import { dateSpan, int } from '#lib/format.js';
	import { compareLabel, daySpan, PRESET_LABEL, type Metric, type RangeSel } from '#lib/ranges.js';
	import type { TopItem } from '#lib/components/overview/top-list.svelte';

	const PAGE_SIZE = 50;
	const KINDS: { kind: TopKind; label: string }[] = [
		{ kind: 'artists', label: 'Artists' },
		{ kind: 'tracks', label: 'Tracks' },
		{ kind: 'albums', label: 'Albums' },
		{ kind: 'genres', label: 'Genres' }
	];

	const kind = $derived<TopKind>(
		(KINDS.find((k) => k.kind === page.params.kind)?.kind ?? 'artists') as TopKind
	);
	const pageNo = $derived(Math.max(1, Number(page.url.searchParams.get('page')) || 1));

	// Scope: which narrowing each kind allows (as in the API).
	type ScopeKey = 'artist_id' | 'album_id' | 'genre_id';
	const ALLOWED: Record<TopKind, ScopeKey[]> = {
		tracks: ['artist_id', 'album_id', 'genre_id'],
		artists: ['album_id', 'genre_id'],
		albums: ['artist_id', 'genre_id'],
		genres: []
	};
	const scope = $derived.by(() => {
		for (const k of ALLOWED[kind]) {
			const v = page.url.searchParams.get(k);
			if (v) return { key: k, id: v };
		}
		return null;
	});
	const scopeParam = $derived(scope ? { [scope.key]: scope.id } : {});

	function listQuery(sel: RangeSel, metric: Metric, n: number, offset: number): Query<unknown> {
		const sp = scopeParam as never;
		switch (kind) {
			case 'artists':
				return topArtistsQuery(sel, metric, n, offset, sp);
			case 'tracks':
				return topTracksQuery(sel, metric, n, offset, sp);
			case 'albums':
				return topAlbumsQuery(sel, metric, n, offset, sp);
			default:
				return topGenresQuery(sel, metric, n, offset);
		}
	}

	type List = {
		total: number;
		range: { from: string; to: string; first_play_date: string | null };
		previous: { from: string; to: string } | null;
		items: unknown[];
	};
	const list = new Resource<List>(
		() => listQuery(view.range, view.metric, PAGE_SIZE, (pageNo - 1) * PAGE_SIZE) as Query<List>
	);
	// The #1 entry, for the share meters on later pages (page 1 has it already).
	const leader = new Resource<List>(() =>
		pageNo > 1 ? (listQuery(view.range, view.metric, 1, 0) as Query<List>) : null
	);

	// Name of the scoping entity, for the "Tracks by …" chip.
	type Named = { artist?: { name: string }; album?: { name: string }; genre?: { name: string } };
	const scopeEntity = new Resource<Named>(() => {
		if (!scope) return null;
		const q =
			scope.key === 'artist_id'
				? artistQuery(scope.id, view.range, view.metric)
				: scope.key === 'album_id'
					? albumQuery(scope.id, view.range, view.metric)
					: genreQuery(scope.id, view.range, view.metric);
		return q as Query<Named>;
	});
	const scopeName = $derived.by(() => {
		const e = scopeEntity.current;
		return e?.artist?.name ?? e?.album?.name ?? e?.genre?.name ?? null;
	});

	const items = $derived.by<TopItem[]>(() => {
		const raw = (list.current?.items ?? []) as never[];
		switch (kind) {
			case 'artists':
				return raw.map(artistItem);
			case 'tracks':
				return raw.map((r) => trackItem(r, true));
			case 'albums':
				return raw.map(albumItem);
			default:
				return raw.map(genreItem);
		}
	});
	const leaderValue = $derived.by(() => {
		const first = (pageNo > 1 ? leader.current?.items[0] : list.current?.items[0]) as
			{ plays: number; ms_played: number } | undefined;
		if (!first) return items[0] ? (view.metric === 'plays' ? items[0].plays : items[0].ms) : 1;
		return view.metric === 'plays' ? first.plays : first.ms_played;
	});

	const l = $derived(list.current);
	$effect(() => {
		if (!l) return;
		resolvedRange.from = l.range.from;
		resolvedRange.to = l.range.to;
		resolvedRange.firstDate = l.range.first_play_date ?? '';
	});

	const sel = $derived(view.range);
	const rangeTitle = $derived(
		sel.preset === 'custom' ? dateSpan(sel.from, sel.to) : PRESET_LABEL[sel.preset]
	);
	const kindLabel = $derived(KINDS.find((k) => k.kind === kind)!.label);
	const title = $derived(`Top ${kindLabel.toLowerCase()}`);
	const scopeLabel = $derived.by(() => {
		if (!scope) return '';
		const name = scopeName ?? '…';
		if (scope.key === 'artist_id') return `by ${name}`;
		if (scope.key === 'album_id') return kind === 'tracks' ? `on ${name}` : `credited on ${name}`;
		return `in ${name}`;
	});
	const compare = $derived.by(() => {
		if (!l) return '';
		if (!l.previous) return 'No earlier period to compare, so no movement.';
		return `Movement is ${compareLabel(sel, daySpan(l.range.from, l.range.to))}.`;
	});

	function pageHref(n: number) {
		return hrefWith({ page: n > 1 ? String(n) : null });
	}
	function kindHref(k: TopKind) {
		// Keep a scope only where the target kind supports it.
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- a local builder, not state
		const p = new URLSearchParams(rangeSearch());
		if (page.url.searchParams.get('metric')) p.set('metric', page.url.searchParams.get('metric')!);
		if (scope && ALLOWED[k].includes(scope.key)) p.set(scope.key, scope.id);
		const s = p.toString();
		return resolve('/top/[[kind]]', { kind: k }) + (s ? `?${s}` : '');
	}
	const clearScopeHref = $derived(
		hrefWith({ artist_id: null, album_id: null, genre_id: null, page: null })
	);

	onMount(() =>
		registerPrefetcher({
			range: (s) => prefetch([listQuery(s, view.metric, PAGE_SIZE, 0)]),
			metric: (m) => prefetch([listQuery(view.range, m, PAGE_SIZE, 0)])
		})
	);
	function warmPage(n: number) {
		prefetch([listQuery(view.range, view.metric, PAGE_SIZE, (n - 1) * PAGE_SIZE)]);
	}
	function warmKind(k: TopKind) {
		if (k === kind) return;
		const sp = (scope && ALLOWED[k].includes(scope.key) ? scopeParam : {}) as never;
		const q =
			k === 'artists'
				? topArtistsQuery(view.range, view.metric, PAGE_SIZE, 0, sp)
				: k === 'tracks'
					? topTracksQuery(view.range, view.metric, PAGE_SIZE, 0, sp)
					: k === 'albums'
						? topAlbumsQuery(view.range, view.metric, PAGE_SIZE, 0, sp)
						: topGenresQuery(view.range, view.metric, PAGE_SIZE, 0);
		prefetch([q]);
	}
	// Warm the next page once this one is in.
	$effect(() => {
		if (l && !list.stale && pageNo * PAGE_SIZE < l.total) warmPage(pageNo + 1);
	});
</script>

<svelte:head
	><title>{title}{scope ? ` ${scopeLabel}` : ''}, {rangeTitle} – spotussi</title></svelte:head
>

<div class="mx-auto max-w-[90rem] px-4 pt-6 pb-20 sm:px-6 lg:px-10 lg:pt-10">
	<header class="flex flex-col gap-5 md:flex-row md:items-end md:justify-between">
		<div class="min-w-0">
			<h1 class="display text-2xl text-balance sm:text-3xl">
				{title}{#if scope}<span class="text-muted-foreground"> {scopeLabel}</span>{/if}
			</h1>
			<p class="mt-2 min-h-10 text-sm text-muted-foreground sm:min-h-5">
				{#if l}
					<span class="text-foreground">{rangeTitle}, {dateSpan(l.range.from, l.range.to)}.</span>
					{compare}
				{:else if !list.error}
					<Skeleton class="inline-block h-4 w-72 align-middle" />
				{/if}
			</p>
		</div>
		<nav aria-label="Top lists" class="-mx-1 overflow-x-auto px-1">
			<ul class="flex w-max items-center rounded-lg bg-muted p-[3px]">
				{#each KINDS as k (k.kind)}
					{@const active = k.kind === kind}
					<li>
						<a
							href={kindHref(k.kind)}
							aria-current={active ? 'page' : undefined}
							onpointerenter={() => warmKind(k.kind)}
							onfocus={() => warmKind(k.kind)}
							class="flex h-8 items-center gap-1.5 rounded-md px-3 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground aria-[current]:bg-card aria-[current]:text-foreground aria-[current]:shadow-[0_1px_0_var(--border),0_0_0_1px_var(--border)]"
						>
							<Lamp on={active} />
							{k.label}
						</a>
					</li>
				{/each}
			</ul>
		</nav>
	</header>

	{#if scope}
		<p class="mt-4 flex flex-wrap items-center gap-2 text-sm">
			<span class="text-muted-foreground">Showing only {kindLabel.toLowerCase()} {scopeLabel}.</span
			>
			<a
				href={clearScopeHref}
				class="inline-flex h-7 items-center gap-1 rounded-md border bg-card px-2 text-xs font-medium hover:bg-muted"
				><X class="size-3.5" aria-hidden="true" />Show all {kindLabel.toLowerCase()}</a
			>
		</p>
	{/if}

	<section class="mt-6 rounded-xl border bg-card px-4 pt-2 pb-4 sm:px-6" aria-label={title}>
		{#if list.error && (!l || list.stale)}
			<SectionError class="my-4" what={title.toLowerCase()} error={list.error} retry={list.retry} />
		{:else if l}
			<div class="transition-opacity duration-200 {list.stale ? 'opacity-55' : ''}">
				{#if l.total === 0}
					<EmptyState
						class="my-4 border-0 bg-transparent px-0 sm:px-2"
						what="Nothing ranked"
						hint={scope
							? 'The filter narrows this list.'
							: kind === 'genres'
								? 'Genres arrive with artist details, which may still be loading.'
								: undefined}
					/>
				{:else if items.length === 0}
					<div class="grid gap-3 py-12 text-center">
						<p class="text-lg font-semibold">This page is past the end of the list</p>
						<a href={pageHref(1)} class="text-sm underline underline-offset-4"
							>Go to the first page</a
						>
					</div>
				{:else}
					<RankedTable
						{items}
						metric={view.metric}
						hasPrevious={Boolean(l.previous)}
						leader={leaderValue}
						{kind}
						caption="{title}{scope ? ` ${scopeLabel}` : ''}, page {pageNo}"
					/>
				{/if}
			</div>
			{#if l.total > 0}
				<Pager
					page={pageNo}
					pageSize={PAGE_SIZE}
					total={l.total}
					noun={kindLabel.toLowerCase()}
					href={pageHref}
					warm={warmPage}
				/>
			{/if}
		{:else}
			<div aria-busy="true" class="pt-2">
				<span class="sr-only">Loading {title.toLowerCase()}</span>
				{#each { length: 10 } as _, i (i)}
					<div class="flex items-center gap-3 border-b border-border/60 py-2.5 last:border-0">
						<Skeleton class="ml-6 h-4 w-6" />
						<Skeleton class="size-11 rounded-[4px]" />
						<div class="grid flex-1 gap-1.5">
							<Skeleton class="h-4 w-1/3" /><Skeleton class="h-3 w-1/4" />
						</div>
						<Skeleton class="h-4 w-14" />
					</div>
				{/each}
			</div>
		{/if}
	</section>
	{#if l && l.total > 0}
		<p class="tnum mt-3 flex items-center gap-1.5 text-xs text-muted-foreground">
			{int(l.total)}
			{kindLabel.toLowerCase()}
			{#if kind === 'artists'}
				<Hint
					text="A play counts for every credited artist, so artist totals can add up to more than your plays."
				/>
			{:else if kind === 'genres'}
				<Hint
					text="A play counts for every genre of every credited artist, so genre totals overlap."
				/>
			{:else if kind === 'albums'}
				<Hint text="Albums rank by plays of any of their tracks." />
			{/if}
		</p>
	{/if}
</div>
