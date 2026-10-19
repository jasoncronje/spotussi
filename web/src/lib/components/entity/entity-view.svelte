<!--
	Artist, album, track and genre pages (D14). One layout for all four: who
	or what it is, where it ranks in the range, totals in range and all time,
	first and last play, listening over the range and around the clock, what
	inside it you played most, and where you played it from. Everything below
	the header is narrowed to this entity with the API's scope parameters.
-->
<script lang="ts">
	import { settingsHref } from '#lib/components/settings/tabs.js';
	import { onMount } from 'svelte';
	import EyeOff from '@lucide/svelte/icons/eye-off';
	import Eye from '@lucide/svelte/icons/eye';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import Artwork from '#lib/components/common/artwork.svelte';
	import GenreGlyph from '#lib/components/common/genre-glyph.svelte';
	import DetailsBadge from '#lib/components/common/details-badge.svelte';
	import Lamp from '#lib/components/shell/lamp.svelte';
	import { resolve } from '$app/paths';
	import ChartPanel from '#lib/components/common/chart-panel.svelte';
	import SectionError from '#lib/components/common/section-error.svelte';
	import EstimatedNote from '#lib/components/common/estimated-note.svelte';
	import ListeningClock from '#lib/components/overview/listening-clock.svelte';
	import TopList from '#lib/components/overview/top-list.svelte';
	import ContextList from './context-list.svelte';
	import PlayRow from '#lib/components/history/play-row.svelte';
	const timelineChart = import('#lib/components/overview/timeline-chart.svelte');
	import { api, type Schemas } from '#lib/api/client.js';
	import { Resource, refreshAll } from '#lib/data/resource.svelte.js';
	import { invalidate, prefetch, type Query } from '#lib/data/cache.js';
	import { registerPrefetcher } from '#lib/data/prefetch.js';
	import { albumItem, artistItem, trackItem } from '#lib/data/items.js';
	import {
		albumQuery,
		artistQuery,
		clockQuery,
		contextsQuery,
		genreQuery,
		historyQuery,
		timelineQuery,
		topAlbumsQuery,
		topArtistsQuery,
		topTracksQuery,
		trackQuery,
		unwrap,
		type Scope
	} from '#lib/data/queries.js';
	import {
		view,
		albumHref,
		artistHref,
		genreHref,
		historyHref,
		setRange,
		topHref
	} from '#lib/state/view.svelte.js';
	import { resolvedRange } from '#lib/state/resolved.svelte.js';
	import {
		BUCKET_NOUN,
		ago,
		bucketLabel,
		dateAt,
		dateSpan,
		duration,
		durationParts,
		hourLabel,
		hourSpan,
		int,
		trackLength,
		type Bucket
	} from '#lib/format.js';
	import { PRESET_LABEL, type Metric, type RangeSel } from '#lib/ranges.js';

	type Kind = 'artist' | 'album' | 'track' | 'genre';
	let { kind, id }: { kind: Kind; id: string } = $props();

	type Detail =
		| Schemas['ArtistDetail']
		| Schemas['AlbumDetail']
		| Schemas['TrackDetail']
		| Schemas['GenreDetail'];

	const scope = $derived<Scope>(
		kind === 'artist'
			? { artist_id: id }
			: kind === 'album'
				? { album_id: id }
				: kind === 'track'
					? { track_id: id }
					: { genre_id: id }
	);

	function detailQuery(sel: RangeSel, m: Metric) {
		return (
			kind === 'artist'
				? artistQuery(id, sel, m)
				: kind === 'album'
					? albumQuery(id, sel, m)
					: kind === 'track'
						? trackQuery(id, sel, m)
						: genreQuery(id, sel, m)
		) as Query<Detail>;
	}
	// What each page lists inside the entity.
	const LISTS = {
		artist: { tracks: 8, albums: 5, artists: 0 },
		album: { tracks: 12, albums: 0, artists: 5 },
		track: { tracks: 0, albums: 0, artists: 0 },
		genre: { tracks: 5, albums: 5, artists: 5 }
	} as const;
	const want = $derived(LISTS[kind]);

	function queries(sel: RangeSel, m: Metric): Query<unknown>[] {
		const s = scope;
		const out: Query<unknown>[] = [
			detailQuery(sel, m),
			timelineQuery(sel, s, true),
			clockQuery(sel, s),
			contextsQuery(sel, m, s)
		];
		if (want.tracks) out.push(topTracksQuery(sel, m, want.tracks, 0, s as never));
		if (want.albums) out.push(topAlbumsQuery(sel, m, want.albums, 0, s as never));
		if (want.artists) out.push(topArtistsQuery(sel, m, want.artists, 0, s as never));
		if (kind === 'track') out.push(historyQuery(sel, { track_id: id }, undefined, 10));
		return out;
	}

	const detail = new Resource<Detail>(() => detailQuery(view.range, view.metric));
	const timeline = new Resource(() => timelineQuery(view.range, scope, true));
	const clock = new Resource(() => clockQuery(view.range, scope));
	const contexts = new Resource(() => contextsQuery(view.range, view.metric, scope));
	const tracks = new Resource(() =>
		want.tracks ? topTracksQuery(view.range, view.metric, want.tracks, 0, scope as never) : null
	);
	const albums = new Resource(() =>
		want.albums ? topAlbumsQuery(view.range, view.metric, want.albums, 0, scope as never) : null
	);
	const artists = new Resource(() =>
		want.artists ? topArtistsQuery(view.range, view.metric, want.artists, 0, scope as never) : null
	);
	const plays = new Resource(() =>
		kind === 'track' ? historyQuery(view.range, { track_id: id }, undefined, 10) : null
	);

	onMount(() =>
		registerPrefetcher({
			range: (sel) => prefetch(queries(sel, view.metric)),
			metric: (m) => prefetch(queries(view.range, m))
		})
	);

	const d = $derived(detail.current);
	$effect(() => {
		if (!d) return;
		resolvedRange.from = d.range.from;
		resolvedRange.to = d.range.to;
		resolvedRange.firstDate = d.range.first_play_date ?? '';
	});

	const metric = $derived(view.metric);
	const sel = $derived(view.range);
	const rangeName = $derived(
		sel.preset === 'custom' ? dateSpan(sel.from, sel.to) : PRESET_LABEL[sel.preset]
	);
	const hasPrevious = $derived(Boolean(d?.previous));
	const notFound = $derived(
		detail.error && 'status' in detail.error && (detail.error as { status: number }).status === 404
	);

	// ---------- identity ----------
	type Head = {
		name: string;
		image: string | null;
		kicker: string;
		noun: string;
	};
	const head = $derived.by<Head | null>(() => {
		if (!d) return null;
		if ('artist' in d)
			return { name: d.artist.name, image: d.artist.image_url, kicker: 'Artist', noun: 'artists' };
		if ('album' in d)
			return {
				name: d.album.name,
				image: d.album.image_url,
				kicker:
					d.album.album_type === 'single'
						? 'Single'
						: d.album.album_type === 'compilation'
							? 'Compilation'
							: 'Album',
				noun: 'albums'
			};
		if ('track' in d)
			return { name: d.track.name, image: d.track.image_url, kicker: 'Track', noun: 'tracks' };
		return { name: d.genre.name, image: null, kicker: 'Genre', noun: 'genres' };
	});
	const meta = $derived(
		!d
			? null
			: 'artist' in d
				? d.artist.metadata
				: 'album' in d
					? d.album.metadata
					: 'track' in d
						? d.track.metadata
						: null
	);
	const excluded = $derived(d && 'excluded' in d ? d.excluded : false);
	const listened = $derived(d ? d.in_range.plays > 0 || d.in_range.ms_played > 0 : true);

	// ---------- exclusion ----------
	let excluding = $state(false);
	let excludeError = $state('');
	async function toggleExclude() {
		if (kind !== 'artist' && kind !== 'track') return;
		excluding = true;
		excludeError = '';
		const path = { kind: kind === 'artist' ? ('artists' as const) : ('tracks' as const), id };
		try {
			await unwrap(
				excluded
					? api.DELETE('/exclusions/{kind}/{id}', { params: { path } })
					: api.PUT('/exclusions/{kind}/{id}', { params: { path } })
			);
			refreshAll();
		} catch (e) {
			excludeError = (e as Error).message;
		} finally {
			excluding = false;
		}
	}

	// ---------- readouts ----------
	const other = (m: { plays: number; ms_played: number }) =>
		metric === 'plays'
			? duration(m.ms_played)
			: `${int(m.plays)} ${m.plays === 1 ? 'play' : 'plays'}`;
	const figure = (m: { plays: number; ms_played: number }) =>
		metric === 'plays' ? [{ value: int(m.plays), unit: '' }] : durationParts(m.ms_played);

	// ---------- charts ----------
	const tl = $derived(timeline.current);
	const bucket = $derived((tl?.bucket ?? 'day') as Bucket);
	const timelineTitle = $derived(
		`${metric === 'plays' ? 'Plays' : 'Time listened'} by ${BUCKET_NOUN[bucket]}`
	);
	const timelineSummary = $derived.by(() => {
		if (!tl || !tl.points.length) return undefined;
		const v = (p: Schemas['TimelinePoint']) => (metric === 'plays' ? p.plays : p.ms_played);
		const f = (n: number) => (metric === 'plays' ? `${int(n)} plays` : duration(n));
		const peak = tl.points.reduce((b, p) => (v(p) > v(b) ? p : b), tl.points[0]!);
		if (v(peak) === 0) return 'Nothing in this range.';
		const active = tl.points.filter((p) => v(p) > 0).length;
		const base = `Played in ${int(active)} of ${int(tl.points.length)} ${BUCKET_NOUN[bucket]}s. Peak ${bucketLabel(peak.key, bucket)}, ${f(v(peak))}.`;
		return tl.previous && tl.previous_points?.length
			? `${base} Unlit lamps show ${dateSpan(tl.previous.from, tl.previous.to)}.`
			: base;
	});
	const ck = $derived(clock.current);
	const clockSummary = $derived.by(() => {
		if (!ck) return undefined;
		const v = (h: Schemas['ClockHour']) => (metric === 'plays' ? h.plays : h.ms_played);
		const sorted = [...ck.hours].sort((a, b) => v(b) - v(a));
		if (v(sorted[0]!) === 0) return 'Nothing in this range.';
		return `Mostly around ${hourLabel(sorted[0]!.hour)}.`;
	});

	// ---------- lists ----------
	const trackItems = $derived(
		(tracks.current?.items ?? []).map((r) => trackItem(r, kind !== 'album'))
	);
	const albumItems = $derived((albums.current?.items ?? []).map(albumItem));
	const artistItems = $derived((artists.current?.items ?? []).map(artistItem));
	const scopeParam = $derived(Object.fromEntries(Object.entries(scope)) as Record<string, string>);
	// Most albums have one or two artists, named in the header; the ranked
	// list (artists on its tracks, features included) is for compilations,
	// soundtracks and feature-heavy albums, after the tracks (D14, D32).
	const lists = $derived.by(() => {
		const a = want.artists &&
			(kind !== 'album' || (artists.current?.total ?? 0) >= 3) && {
				key: 'artists' as const,
				title: kind === 'album' ? 'Artists on this album' : 'Top artists',
				res: artists,
				items: artistItems
			};
		const t = want.tracks && {
			key: 'tracks' as const,
			title: kind === 'album' ? 'Tracks on this album' : 'Top tracks',
			res: tracks,
			items: trackItems
		};
		const b = want.albums && {
			key: 'albums' as const,
			title: 'Top albums',
			res: albums,
			items: albumItems
		};
		return (kind === 'album' ? [t, a] : [a, t, b]).filter(Boolean) as {
			key: 'artists' | 'tracks' | 'albums';
			title: string;
			res: Resource<{ total: number }>;
			items: typeof trackItems;
		}[];
	});
	const historyLink = $derived(historyHref(scopeParam));
</script>

<svelte:head>
	<title>{head?.name ?? (notFound ? 'Not found' : 'Loading')} – spotussi</title>
</svelte:head>

{#snippet chartSkeleton()}
	<div class="flex min-h-60 flex-1 px-6 pt-6 pb-8 sm:min-h-64">
		<Skeleton class="flex-1 rounded-lg" />
	</div>
{/snippet}
{#snippet chartFailed()}
	<p
		class="grid min-h-60 flex-1 place-items-center px-6 py-10 text-center text-sm text-muted-foreground"
	>
		This chart couldn't be drawn. Switch to Table to see the numbers.
	</p>
{/snippet}

<div class="mx-auto max-w-[90rem] px-4 pt-6 pb-20 sm:px-6 lg:px-10 lg:pt-10">
	{#if notFound}
		<section class="grid gap-3 rounded-xl border border-dashed bg-card/50 px-6 py-10 sm:px-10">
			<h1 class="display text-2xl">Not in your library</h1>
			<p class="max-w-[60ch] text-muted-foreground">
				No {kind} with this ID in your history. The link may be from another library.
			</p>
		</section>
	{:else}
		<!-- Identity -->
		<header class="flex flex-col gap-5 sm:flex-row sm:items-end sm:justify-between">
			<div class="flex min-w-0 items-center gap-4 sm:gap-6">
				{#if head}
					{#if kind === 'genre'}
						<GenreGlyph
							level={d && d.in_range.rank
								? 1 - (d.in_range.rank - 1) / Math.max(1, d.in_range.ranked_total)
								: 0.1}
							class="size-20 sm:size-28"
						/>
					{:else}
						<Artwork
							src={head.image}
							name={head.name}
							round={kind === 'artist'}
							eager
							class="size-20 sm:size-28 lg:size-32"
						/>
					{/if}
				{:else}
					<Skeleton
						class="size-20 shrink-0 sm:size-28 lg:size-32 {kind === 'artist' ? 'rounded-full' : ''}"
					/>
				{/if}
				<div class="min-w-0">
					<p class="flex flex-wrap items-center gap-x-2.5 gap-y-1 text-sm text-muted-foreground">
						{head?.kicker ?? ' '}{#if d && 'track' in d && d.saved_at}<a
								href={resolve('/library')}
								class="hit inline-flex items-center gap-1.5 hover:text-foreground"
								title="In your saved songs"><Lamp on />Saved {dateAt(d.saved_at)}</a
							>{/if}{#if meta}<DetailsBadge {meta} />{/if}
					</p>
					<h1 class="display text-2xl text-balance break-words sm:text-3xl">
						{#if head}{head.name}{:else}<Skeleton class="h-9 w-64" />{/if}
					</h1>
					<div class="mt-1.5 min-h-5 text-sm text-muted-foreground">
						{#if d && 'artist' in d}
							{#if d.artist.genres.length}
								<ul class="flex flex-wrap gap-1.5" aria-label="Genres">
									{#each d.artist.genres.slice(0, 6) as g (g.id)}
										<li>
											<a
												href={genreHref(g.id)}
												class="inline-flex h-6 items-center rounded-md bg-muted px-2 text-xs hover:text-foreground"
												>{g.name}</a
											>
										</li>
									{/each}
								</ul>
							{:else}
								No genres known yet.
							{/if}
						{:else if d && 'album' in d}
							{#each d.album.artists as a, k (a.id)}{k > 0 ? ', ' : ''}<a
									href={artistHref(a.id)}
									class="inline-link hit">{a.name}</a
								>{/each}{#if d.album.release_year}, {d.album
									.release_year}{/if}{#if d.album.total_tracks},
								{int(d.album.total_tracks)}
								{d.album.total_tracks === 1 ? 'track' : 'tracks'}{/if}
						{:else if d && 'track' in d}
							{#each d.track.artists as a, k (a.id)}{k > 0 ? ', ' : ''}<a
									href={artistHref(a.id)}
									class="inline-link hit">{a.name}</a
								>{/each}{#if d.track.album}, from <a
									href={albumHref(d.track.album.id)}
									class="inline-link hit">{d.track.album.name}</a
								>{/if}{#if d.track.duration_ms}, {trackLength(d.track.duration_ms)}{/if}
						{:else if d && 'genre' in d}
							{int(d.artists)}
							{d.artists === 1 ? 'artist' : 'artists'} in your library carry this genre.
						{/if}
					</div>
				</div>
			</div>
			<div class="flex shrink-0 flex-wrap items-center gap-2">
				<a
					href={historyLink}
					class="inline-flex h-9 items-center rounded-lg border bg-card px-3.5 text-sm font-medium hover:bg-muted"
					>Plays in History</a
				>
				{#if (kind === 'artist' || kind === 'track') && d}
					<button
						type="button"
						onclick={toggleExclude}
						disabled={excluding}
						aria-pressed={excluded}
						class="inline-flex h-9 items-center gap-1.5 rounded-lg border bg-card px-3.5 text-sm font-medium hover:bg-muted disabled:opacity-60"
						title={excluded
							? 'Count this again in every stat'
							: 'Leave this out of every stat. Plays stay recorded.'}
					>
						{#if excluded}<Eye class="size-4" aria-hidden="true" />Include in stats{:else}<EyeOff
								class="size-4"
								aria-hidden="true"
							/>Exclude from stats{/if}
					</button>
				{/if}
			</div>
		</header>

		{#if excludeError}
			<p role="alert" class="mt-4 text-sm text-destructive">{excludeError}</p>
		{/if}
		{#if excluded}
			<p
				role="note"
				class="mt-5 rounded-lg border bg-muted/50 px-4 py-3 text-sm text-muted-foreground"
			>
				<span class="font-semibold text-foreground">Excluded from your stats.</span>
				Plays stay recorded.
				<a href={settingsHref('library')} class="inline-link hit">Manage exclusions</a>
			</p>
		{/if}

		{#if detail.error && !d}
			<SectionError class="mt-6" what="this {kind}" error={detail.error} retry={detail.retry} />
		{/if}

		<!-- Readouts -->
		<section
			aria-label="Totals"
			class="mt-6 overflow-hidden rounded-xl border bg-card transition-opacity duration-200 {detail.stale
				? 'opacity-55'
				: ''}"
		>
			<dl class="grid grid-cols-2 lg:grid-cols-4">
				<div class="flex min-w-0 flex-col gap-2 px-5 py-5 sm:px-6">
					<dt class="text-sm text-muted-foreground">Rank, {rangeName.toLowerCase()}</dt>
					<dd class="contents">
						{#if d}
							{#if d.in_range.rank}
								<p class="figure text-2xl leading-none sm:text-3xl">
									<span class="text-[0.6em] text-muted-foreground">#</span>{int(d.in_range.rank)}
								</p>
								<p class="tnum text-xs text-muted-foreground">
									of {int(d.in_range.ranked_total)}
									{head?.noun} by {metric === 'plays' ? 'plays' : 'time'}
								</p>
							{:else}
								<p class="figure text-2xl leading-none text-muted-foreground sm:text-3xl">–</p>
								<p class="text-xs text-muted-foreground">Not played in this range</p>
							{/if}
						{:else}
							<Skeleton class="h-9 w-20" /><Skeleton class="h-3.5 w-32" />
						{/if}
					</dd>
				</div>
				<div class="flex min-w-0 flex-col gap-2 border-l px-5 py-5 sm:px-6">
					<dt class="text-sm text-muted-foreground">In range</dt>
					<dd class="contents">
						{#if d}
							<p class="figure text-2xl leading-none whitespace-nowrap sm:text-3xl">
								{#each figure(d.in_range) as p, j (j)}<span class={j > 0 ? 'ml-[0.3em]' : ''}
										>{p.value}</span
									>{#if p.unit}<span
											class="ml-[0.12em] text-[0.5em] font-medium tracking-normal text-muted-foreground"
											>{p.unit}</span
										>{/if}{/each}
							</p>
							<p class="tnum text-xs text-muted-foreground">
								{other(d.in_range)}, {int(d.in_range.skips)} skipped
							</p>
							{#if metric === 'time' && d.in_range.ms_estimated > 0}
								<EstimatedNote ms={d.in_range.ms_estimated} total={d.in_range.ms_played} />
							{/if}
						{:else}
							<Skeleton class="h-9 w-24" /><Skeleton class="h-3.5 w-32" />
						{/if}
					</dd>
				</div>
				<div
					class="flex min-w-0 flex-col gap-2 border-t px-5 py-5 sm:px-6 lg:border-t-0 lg:border-l"
				>
					<dt class="text-sm text-muted-foreground">All time</dt>
					<dd class="contents">
						{#if d}
							<p class="figure text-2xl leading-none whitespace-nowrap sm:text-3xl">
								{#each figure(d.all_time) as p, j (j)}<span class={j > 0 ? 'ml-[0.3em]' : ''}
										>{p.value}</span
									>{#if p.unit}<span
											class="ml-[0.12em] text-[0.5em] font-medium tracking-normal text-muted-foreground"
											>{p.unit}</span
										>{/if}{/each}
							</p>
							<p class="tnum text-xs text-muted-foreground">
								{other(d.all_time)}, {int(d.all_time.skips)} skipped
							</p>
						{:else}
							<Skeleton class="h-9 w-24" /><Skeleton class="h-3.5 w-32" />
						{/if}
					</dd>
				</div>
				<div class="flex min-w-0 flex-col gap-2 border-t border-l px-5 py-5 sm:px-6 lg:border-t-0">
					<dt class="sr-only">First and last played</dt>
					<dd class="grid gap-3 text-sm">
						{#if d}
							<p>
								<span class="block text-muted-foreground">First played</span>
								<span class="tnum"
									>{d.all_time.first_played_at ? dateAt(d.all_time.first_played_at) : 'Never'}</span
								>
							</p>
							<p>
								<span class="block text-muted-foreground">Last played</span>
								<span class="tnum"
									>{d.all_time.last_played_at ? ago(d.all_time.last_played_at) : 'Never'}</span
								>
							</p>
						{:else}
							<Skeleton class="h-10 w-28" /><Skeleton class="h-10 w-28" />
						{/if}
					</dd>
				</div>
			</dl>
		</section>

		{#if d && !listened}
			<section
				class="mt-6 grid gap-4 rounded-xl border border-dashed bg-card/50 px-6 py-8 sm:px-10"
			>
				<div>
					<h2 class="text-lg font-semibold">Not played in {rangeName.toLowerCase()}</h2>
					<p class="mt-1 max-w-[60ch] text-muted-foreground">
						{#if d.all_time.last_played_at}Last played {ago(d.all_time.last_played_at)}. Pick a
							longer range to see it.{:else}No plays recorded yet.{/if}
					</p>
				</div>
				{#if d.all_time.plays > 0}
					<div class="flex flex-wrap gap-2">
						<button
							type="button"
							onclick={() => setRange({ preset: 'all' })}
							class="h-9 rounded-lg border bg-card px-3.5 text-sm font-medium hover:bg-muted"
							>Show all time</button
						>
					</div>
				{/if}
			</section>
		{:else}
			<!-- Charts -->
			<div class="mt-4 grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,24rem)]">
				{#if timeline.error && (!tl || timeline.stale)}
					<SectionError what="the timeline" error={timeline.error} retry={timeline.retry} />
				{:else}
					<ChartPanel title={timelineTitle} summary={timelineSummary} dim={timeline.stale}>
						{#if tl}
							{#await timelineChart}
								{@render chartSkeleton()}
							{:then { default: TimelineChart }}
								<svelte:boundary>
									<TimelineChart timeline={tl} {metric} />
									{#snippet failed()}{@render chartFailed()}{/snippet}
								</svelte:boundary>
							{/await}
						{:else}
							{@render chartSkeleton()}
						{/if}
						{#snippet table()}
							{#if tl}
								<table class="tnum w-full text-left text-xs">
									<caption class="sr-only">{timelineTitle}</caption>
									<thead class="text-muted-foreground">
										<tr class="border-b">
											<th scope="col" class="py-1.5 font-medium capitalize"
												>{BUCKET_NOUN[bucket]}</th
											>
											<th scope="col" class="py-1.5 text-right font-medium">Plays</th>
											<th scope="col" class="py-1.5 text-right font-medium">Time</th>
											<th scope="col" class="py-1.5 text-right font-medium">Skips</th>
										</tr>
									</thead>
									<tbody>
										{#each tl.points as p, n (n)}
											<tr class="border-b border-border/60">
												<th scope="row" class="py-1.5 font-normal">{bucketLabel(p.key, bucket)}</th>
												<td class="py-1.5 text-right">{int(p.plays)}</td>
												<td class="py-1.5 text-right">{duration(p.ms_played)}</td>
												<td class="py-1.5 text-right">{int(p.skips)}</td>
											</tr>
										{/each}
									</tbody>
								</table>
							{/if}
						{/snippet}
					</ChartPanel>
				{/if}

				{#if clock.error && (!ck || clock.stale)}
					<SectionError what="the listening clock" error={clock.error} retry={clock.retry} />
				{:else}
					<ChartPanel title="Listening clock" summary={clockSummary} dim={clock.stale}>
						<div class="px-6 pt-2 pb-6">
							{#if ck}
								<svelte:boundary>
									<ListeningClock clock={ck} {metric} />
									{#snippet failed()}{@render chartFailed()}{/snippet}
								</svelte:boundary>
							{:else}
								<Skeleton class="mx-auto aspect-square w-full max-w-[22rem] rounded-full" />
							{/if}
						</div>
						{#snippet table()}
							{#if ck}
								<table class="tnum w-full text-left text-xs">
									<caption class="sr-only">Listening by hour of day</caption>
									<thead class="text-muted-foreground">
										<tr class="border-b">
											<th scope="col" class="py-1.5 font-medium">Hour</th>
											<th scope="col" class="py-1.5 text-right font-medium">Plays</th>
											<th scope="col" class="py-1.5 text-right font-medium">Time</th>
										</tr>
									</thead>
									<tbody>
										{#each ck.hours as h (h.hour)}
											<tr class="border-b border-border/60">
												<th scope="row" class="py-1.5 font-normal">{hourSpan(h.hour)}</th>
												<td class="py-1.5 text-right">{int(h.plays)}</td>
												<td class="py-1.5 text-right">{duration(h.ms_played)}</td>
											</tr>
										{/each}
									</tbody>
								</table>
							{/if}
						{/snippet}
					</ChartPanel>
				{/if}
			</div>

			<!-- Inside it, and where from -->
			<div
				class="mt-12 grid gap-x-10 gap-y-12 md:grid-cols-2 {kind === 'genre'
					? '2xl:grid-cols-4'
					: 'xl:grid-cols-3'}"
			>
				{#each lists as l (l.key)}
					<div
						class="min-w-0 transition-opacity duration-200 {l.res.stale
							? 'opacity-55'
							: ''} {kind === 'album' && l.key === 'tracks' ? 'xl:col-span-2' : ''}"
					>
						{#if l.res.error && (!l.res.current || l.res.stale)}
							<SectionError what={l.title.toLowerCase()} error={l.res.error} retry={l.res.retry} />
						{:else if l.res.current}
							<TopList
								kind={l.key}
								title={l.title}
								allHref={topHref(l.key, scopeParam)}
								items={l.items}
								total={l.res.current.total}
								{metric}
								{hasPrevious}
							/>
						{:else}
							{@render listSkeleton()}
						{/if}
					</div>
				{/each}

				<div class="min-w-0 transition-opacity duration-200 {contexts.stale ? 'opacity-55' : ''}">
					{#if contexts.error && (!contexts.current || contexts.stale)}
						<SectionError what="sources" error={contexts.error} retry={contexts.retry} />
					{:else if contexts.current}
						<ContextList
							data={contexts.current}
							{metric}
							onchange={() => {
								invalidate('/contexts');
								contexts.refresh();
							}}
						/>
					{:else}
						{@render listSkeleton()}
					{/if}
				</div>

				{#if kind === 'track'}
					<div
						class="min-w-0 transition-opacity duration-200 md:col-span-1 xl:col-span-2 {plays.stale
							? 'opacity-55'
							: ''}"
					>
						<section aria-labelledby="track-plays-h">
							<header class="mb-2 flex items-baseline justify-between gap-3 border-b pb-2.5">
								<h2 id="track-plays-h" class="text-base font-semibold">Latest plays</h2>
								<a
									href={historyLink}
									class="hit tnum text-xs text-muted-foreground hover:text-foreground hover:underline"
									>All {int(plays.current?.total ?? 0)}<span class="sr-only">
										plays in History</span
									></a
								>
							</header>
							{#if plays.error && !plays.current}
								<SectionError what="plays" error={plays.error} retry={plays.retry} />
							{:else if plays.current}
								{#if plays.current.items.length}
									<ol class="divide-y divide-border/60">
										{#each plays.current.items as p (p.id)}<PlayRow play={p} withDate />{/each}
									</ol>
								{:else}
									<p class="py-6 text-sm text-muted-foreground">No plays in this range.</p>
								{/if}
							{:else}
								{@render listSkeleton()}
							{/if}
						</section>
					</div>
				{/if}
			</div>
		{/if}
	{/if}
</div>

{#snippet listSkeleton()}
	<div aria-busy="true">
		<div class="mb-2 flex justify-between border-b pb-2.5">
			<Skeleton class="h-6 w-28" /><Skeleton class="h-4 w-12" />
		</div>
		{#each { length: 5 } as _, i (i)}
			<div class="grid grid-cols-[1.25rem_auto_1fr_auto] items-center gap-x-3 py-2">
				<span></span>
				<Skeleton class="size-10 rounded-[4px]" />
				<div class="grid gap-1.5"><Skeleton class="h-4 w-3/4" /><Skeleton class="h-3 w-1/2" /></div>
				<Skeleton class="h-4 w-10" />
			</div>
		{/each}
	</div>
{/snippet}
