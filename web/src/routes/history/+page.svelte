<!--
	History (D14): every row in the range, newest first, grouped by day.
	Filters (artist, album, track or genre; skipped; shuffle; counted;
	device or platform) live in the URL next to the range, so a filtered log
	can be linked from anywhere. Pages load by cursor and append below.
-->
<script lang="ts">
	import { page } from '$app/state';
	import X from '@lucide/svelte/icons/x';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import SectionError from '#lib/components/common/section-error.svelte';
	import EmptyState from '#lib/components/common/empty-state.svelte';
	import Segmented from '#lib/components/common/segmented.svelte';
	import EntitySearch from '#lib/components/common/entity-search.svelte';
	import PlayRow, { SOURCES } from '#lib/components/history/play-row.svelte';
	import { Resource } from '#lib/data/resource.svelte.js';
	import { fetchQuery, type Query } from '#lib/data/cache.js';
	import {
		albumQuery,
		artistQuery,
		devicesQuery,
		genreQuery,
		historyQuery,
		trackQuery,
		type HistoryFilters
	} from '#lib/data/queries.js';
	import { view, hrefWith, setParams } from '#lib/state/view.svelte.js';
	import { resolvedRange } from '#lib/state/resolved.svelte.js';
	import { dateSpan, dayKey, int, longDay } from '#lib/format.js';
	import { PRESET_LABEL } from '#lib/ranges.js';
	import type { Schemas } from '#lib/api/client.js';

	const PAGE = 100;
	const SCOPES = ['artist_id', 'album_id', 'track_id', 'genre_id'] as const;
	type ScopeKey = (typeof SCOPES)[number];
	const SCOPE_NOUN: Record<ScopeKey, string> = {
		artist_id: 'Artist',
		album_id: 'Album',
		track_id: 'Track',
		genre_id: 'Genre'
	};

	const sp = $derived(page.url.searchParams);
	const tri = (k: string) =>
		sp.get(k) === 'true' ? true : sp.get(k) === 'false' ? false : undefined;
	const scope = $derived.by(() => {
		for (const k of SCOPES) {
			const v = sp.get(k);
			if (v) return { key: k, id: v };
		}
		return null;
	});
	const filters = $derived.by<HistoryFilters>(() => {
		const f: HistoryFilters = {};
		if (scope) f[scope.key] = scope.id;
		const s = tri('skipped');
		if (s !== undefined) f.skipped = s;
		const sh = tri('shuffle');
		if (sh !== undefined) f.shuffle = sh;
		const c = tri('counted');
		if (c !== undefined) f.counted = c;
		if (sp.get('device')) f.device = sp.get('device')!;
		else if (sp.get('platform')) f.platform = sp.get('platform')!;
		return f;
	});
	const filtered = $derived(Object.keys(filters).length > 0);

	const first = new Resource(() => historyQuery(view.range, filters, undefined, PAGE));
	const devices = new Resource(() => devicesQuery(view.range));

	// Name of the scoping entity for its chip.
	type Named = {
		artist?: { name: string };
		album?: { name: string };
		track?: { name: string };
		genre?: { name: string };
	};
	const scopeEntity = new Resource<Named>(() => {
		if (!scope) return null;
		const q =
			scope.key === 'artist_id'
				? artistQuery(scope.id, view.range, 'plays')
				: scope.key === 'album_id'
					? albumQuery(scope.id, view.range, 'plays')
					: scope.key === 'track_id'
						? trackQuery(scope.id, view.range, 'plays')
						: genreQuery(scope.id, view.range, 'plays');
		return q as Query<Named>;
	});
	const scopeName = $derived.by(() => {
		const e = scopeEntity.current;
		return e?.artist?.name ?? e?.album?.name ?? e?.track?.name ?? e?.genre?.name ?? '…';
	});

	// Further pages, appended; reset whenever the first page's query changes.
	let more = $state<Schemas['Play'][]>([]);
	let cursor = $state<string | null>(null);
	let loadingMore = $state(false);
	let moreError = $state<Error | null>(null);
	let baseKey = '';
	$effect(() => {
		const h = first.current;
		const key = historyQuery(view.range, filters, undefined, PAGE).key;
		if (!h || first.stale) return;
		if (key !== baseKey) {
			baseKey = key;
			more = [];
			cursor = h.next_cursor;
			moreError = null;
		}
	});
	async function loadMore() {
		if (!cursor || loadingMore) return;
		loadingMore = true;
		moreError = null;
		try {
			const r = await fetchQuery(historyQuery(view.range, filters, cursor, PAGE));
			more = [...more, ...r.items];
			cursor = r.next_cursor;
		} catch (e) {
			moreError = e as Error;
		} finally {
			loadingMore = false;
		}
	}

	const h = $derived(first.current);
	$effect(() => {
		if (!h) return;
		resolvedRange.from = h.range.from;
		resolvedRange.to = h.range.to;
		resolvedRange.firstDate = h.range.first_play_date ?? '';
	});
	const rows = $derived([...(h?.items ?? []), ...more]);
	const days = $derived.by(() => {
		const out: { key: string; at: number; plays: Schemas['Play'][] }[] = [];
		for (const p of rows) {
			const k = dayKey(p.played_at);
			const last = out[out.length - 1];
			if (last && last.key === k) last.plays.push(p);
			else out.push({ key: k, at: p.played_at, plays: [p] });
		}
		return out;
	});

	const sel = $derived(view.range);
	const rangeTitle = $derived(
		sel.preset === 'custom' ? dateSpan(sel.from, sel.to) : PRESET_LABEL[sel.preset]
	);

	// ---------- filter controls ----------
	type Tri = 'any' | 'true' | 'false';
	const triValue = (k: string): Tri =>
		sp.get(k) === 'true' ? 'true' : sp.get(k) === 'false' ? 'false' : 'any';
	const setTri = (k: string) => (v: Tri) =>
		setParams({ [k]: v === 'any' ? null : v }, { replace: true });
	const deviceValue = $derived(
		sp.get('device') ? `d:${sp.get('device')}` : sp.get('platform') ? `p:${sp.get('platform')}` : ''
	);
	function setDevice(v: string) {
		setParams(
			{
				device: v.startsWith('d:') ? v.slice(2) : null,
				platform: v.startsWith('p:') ? v.slice(2) : null
			},
			{ replace: true }
		);
	}
	const clearHref = $derived(
		hrefWith({
			artist_id: null,
			album_id: null,
			track_id: null,
			genre_id: null,
			skipped: null,
			shuffle: null,
			counted: null,
			device: null,
			platform: null
		})
	);
</script>

<svelte:head><title>History, {rangeTitle} – spotussi</title></svelte:head>

<div class="mx-auto max-w-[90rem] px-4 pt-6 pb-20 sm:px-6 lg:px-10 lg:pt-10">
	<header class="flex flex-col gap-2">
		<h1 class="display text-2xl text-balance sm:text-3xl">History</h1>
		<p class="min-h-5 text-sm text-muted-foreground">
			{#if h}
				<span class="text-foreground">{rangeTitle}, {dateSpan(h.range.from, h.range.to)}.</span>
				{int(h.total)}
				{h.total === 1 ? 'row' : 'rows'}{filtered ? ' match the filters' : ''}, newest first.
			{:else if !first.error}
				<Skeleton class="inline-block h-4 w-72 align-middle" />
			{/if}
		</p>
	</header>

	<!-- Filters -->
	<section aria-label="Filters" class="mt-6 grid gap-3 rounded-xl border bg-card px-4 py-4 sm:px-5">
		<div class="flex flex-wrap items-center gap-3">
			{#if scope}
				<span
					class="inline-flex h-9 max-w-full items-center gap-2 rounded-lg bg-muted pr-1 pl-3 text-sm"
				>
					<span class="text-muted-foreground">{SCOPE_NOUN[scope.key]}</span>
					<span class="truncate font-medium">{scopeName}</span>
					<a
						href={hrefWith({ [scope.key]: null })}
						class="grid size-7 place-items-center rounded-md hover:bg-card"
						aria-label="Remove {SCOPE_NOUN[scope.key].toLowerCase()} filter"><X class="size-4" /></a
					>
				</span>
			{:else}
				<EntitySearch
					label="Filter by artist, album or track"
					placeholder="Artist, album or track"
					class="w-full sm:w-72"
					onselect={(p) => setParams({ [`${p.kind}_id`]: p.id }, { replace: false })}
				/>
			{/if}
			<label class="flex items-center gap-2 text-sm">
				<span class="text-muted-foreground">Device</span>
				<select
					value={deviceValue}
					onchange={(e) => setDevice(e.currentTarget.value)}
					class="h-9 max-w-56 rounded-lg border bg-card px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/40"
				>
					<option value="">Any device</option>
					{#if devices.current?.devices.length}
						<optgroup label="Devices (live capture)">
							{#each devices.current.devices as d (d.name)}
								<option value="d:{d.name}">{d.name}</option>
							{/each}
						</optgroup>
					{/if}
					{#if devices.current?.platforms.length}
						<optgroup label="Platforms (export)">
							{#each devices.current.platforms as p (p.name)}
								<option value="p:{p.name}">{p.name}</option>
							{/each}
						</optgroup>
					{/if}
				</select>
			</label>
			{#if filtered}
				<a
					href={clearHref}
					class="ml-auto text-sm text-muted-foreground hover:text-foreground hover:underline"
					>Clear filters</a
				>
			{/if}
		</div>
		<div class="flex flex-wrap items-center gap-x-5 gap-y-2">
			<Segmented
				label="Skipped"
				value={triValue('skipped')}
				onchange={setTri('skipped')}
				options={[
					{ value: 'any', label: 'Any' },
					{ value: 'true', label: 'Skipped' },
					{ value: 'false', label: 'Not skipped' }
				]}
			/>
			<Segmented
				label="Shuffle"
				value={triValue('shuffle')}
				onchange={setTri('shuffle')}
				options={[
					{ value: 'any', label: 'Any' },
					{ value: 'true', label: 'Shuffle on' },
					{ value: 'false', label: 'Shuffle off' }
				]}
			/>
			<Segmented
				label="Counted"
				value={triValue('counted')}
				onchange={setTri('counted')}
				options={[
					{ value: 'any', label: 'All rows' },
					{ value: 'true', label: 'Counted' },
					{ value: 'false', label: 'Not counted', hint: 'Too short to count under your play rule' }
				]}
			/>
		</div>
	</section>

	<!-- Legend -->
	<p class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
		<span>Sources, in order:</span>
		{#each SOURCES as s, i (s.key)}
			<span class="inline-flex items-center gap-1.5" title={s.hint}>
				<span class="inline-flex gap-[3px]" aria-hidden="true">
					{#each SOURCES as _, j (j)}<span
							class="size-[5px] rounded-full {i === j ? 'bg-signal' : 'bg-grille'}"
						></span>{/each}
				</span>
				{s.hint}
			</span>
		{/each}
		<span class="inline-flex items-center gap-1.5">
			<span
				aria-hidden="true"
				class="inline-block size-2.5 rounded-[2px]"
				style="background: repeating-linear-gradient(0deg, var(--signal-dim) 0 3px, transparent 3px 4.5px)"
			></span>
			≈ estimated from the track length
		</span>
	</p>

	{#if first.error && (!h || first.stale)}
		<SectionError class="mt-6" what="history" error={first.error} retry={first.retry} />
	{:else if h}
		<div class="mt-6 transition-opacity duration-200 {first.stale ? 'opacity-55' : ''}">
			{#if rows.length === 0}
				<EmptyState
					what={filtered ? 'No rows match these filters' : 'Nothing played'}
					hint={filtered ? 'Loosen a filter or try a longer range.' : undefined}
					{filtered}
				>
					{#if filtered}
						<a
							href={clearHref}
							class="inline-flex h-9 w-fit items-center rounded-lg border bg-card px-3.5 text-sm font-medium hover:bg-muted"
							>Clear filters</a
						>
					{/if}
				</EmptyState>
			{:else}
				{#each days as d (d.key)}
					<section aria-labelledby="day-{d.key}" class="mb-6">
						<header
							class="sticky top-[3.0625rem] z-10 -mx-1 flex items-baseline justify-between gap-3 border-b bg-background/95 px-1 pt-2 pb-2 backdrop-blur"
						>
							<h2 id="day-{d.key}" class="text-sm font-semibold">{longDay(d.at)}</h2>
							<span class="tnum text-xs text-muted-foreground"
								>{int(d.plays.length)} {d.plays.length === 1 ? 'row' : 'rows'}</span
							>
						</header>
						<ol class="divide-y divide-border/60">
							{#each d.plays as p (p.id)}<PlayRow play={p} />{/each}
						</ol>
					</section>
				{/each}
				<div class="flex flex-wrap items-center justify-between gap-3 border-t pt-4">
					<p class="tnum text-sm text-muted-foreground">
						{int(rows.length)} of {int(h.total)} rows
					</p>
					{#if moreError}
						<p role="alert" class="text-sm text-destructive">{moreError.message}</p>
					{/if}
					{#if cursor}
						<button
							type="button"
							onclick={loadMore}
							disabled={loadingMore}
							class="h-9 rounded-lg border bg-card px-3.5 text-sm font-medium hover:bg-muted disabled:opacity-60"
							>{loadingMore
								? 'Loading…'
								: `Load ${int(Math.min(PAGE, h.total - rows.length))} more`}</button
						>
					{/if}
				</div>
			{/if}
		</div>
	{:else}
		<div class="mt-6 grid gap-1" aria-busy="true">
			<Skeleton class="mb-2 h-6 w-48" />
			{#each { length: 10 } as _, i (i)}
				<div class="grid grid-cols-[4.5rem_auto_1fr_auto] items-center gap-x-3 py-2">
					<Skeleton class="h-4 w-12" /><Skeleton class="size-10 rounded-[4px]" />
					<div class="grid gap-1.5">
						<Skeleton class="h-4 w-1/2" /><Skeleton class="h-3 w-1/3" />
					</div>
					<Skeleton class="h-4 w-10" />
				</div>
			{/each}
		</div>
	{/if}
</div>
