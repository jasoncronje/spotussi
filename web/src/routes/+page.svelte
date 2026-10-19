<!--
	Overview (D14): how much you listened in the range, when, and to what.
	Every section reads its own granular endpoint in parallel; previous data
	stays on screen (dimmed) while a new range or metric loads.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import ChartPanel from '#lib/components/common/chart-panel.svelte';
	import SectionError from '#lib/components/common/section-error.svelte';
	import EmptyState from '#lib/components/common/empty-state.svelte';
	import SummaryPanel from '#lib/components/overview/summary-panel.svelte';
	// The chart library is the heaviest dependency; it loads in parallel with
	// the API requests instead of blocking first paint (initial JS budget).
	const timelineChart = import('#lib/components/overview/timeline-chart.svelte');
	import ListeningClock from '#lib/components/overview/listening-clock.svelte';
	import TopList, { type TopItem } from '#lib/components/overview/top-list.svelte';
	import NowPlaying from '#lib/components/overview/now-playing.svelte';
	import RecentPlays from '#lib/components/overview/recent-plays.svelte';
	import { Resource } from '#lib/data/resource.svelte.js';
	import { prefetch } from '#lib/data/cache.js';
	import { registerPrefetcher } from '#lib/data/prefetch.js';
	import {
		clockQuery,
		nowPlayingQuery,
		overviewQueries,
		recentQuery,
		summaryQuery,
		timelineQuery,
		topAlbumsQuery,
		topArtistsQuery,
		topGenresQuery,
		topTracksQuery
	} from '#lib/data/queries.js';
	import {
		view,
		artistHref,
		albumHref,
		trackHref,
		genreHref,
		topHref
	} from '#lib/state/view.svelte.js';
	import { resolvedRange } from '#lib/state/resolved.svelte.js';
	import {
		BUCKET_NOUN,
		bucketLabel,
		dateSpan,
		duration,
		hourLabel,
		hourSpan,
		int,
		shortDate,
		type Bucket
	} from '#lib/format.js';
	import { PRESET_LABEL, compareLabel, daySpan } from '#lib/ranges.js';
	import type { Schemas } from '#lib/api/client.js';

	const summary = new Resource(() => summaryQuery(view.range, view.metric));
	const timeline = new Resource(() => timelineQuery(view.range, {}, true));
	const clock = new Resource(() => clockQuery(view.range));
	const artists = new Resource(() => topArtistsQuery(view.range, view.metric));
	const tracks = new Resource(() => topTracksQuery(view.range, view.metric));
	const albums = new Resource(() => topAlbumsQuery(view.range, view.metric));
	const genres = new Resource(() => topGenresQuery(view.range, view.metric));

	// Live strip: poll now-playing while the tab is visible; refresh recent
	// plays when the playing track changes.
	const now = new Resource(() => nowPlayingQuery());
	const recent = new Resource(() => recentQuery(12));
	const nowTrack = $derived(now.current?.track?.id ?? null);
	let lastTrack: string | null | undefined;
	$effect(() => {
		if (lastTrack !== undefined && nowTrack !== lastTrack) recent.refresh();
		lastTrack = nowTrack;
	});

	onMount(() => {
		const id = setInterval(() => {
			if (document.visibilityState === 'visible') now.refresh();
		}, 15_000);
		const unregister = registerPrefetcher({
			range: (sel) => prefetch(overviewQueries(sel, view.metric)),
			metric: (m) => prefetch(overviewQueries(view.range, m))
		});
		return () => {
			clearInterval(id);
			unregister();
		};
	});

	const s = $derived(summary.current);
	$effect(() => {
		if (!s) return;
		resolvedRange.from = s.range.from;
		resolvedRange.to = s.range.to;
		resolvedRange.firstDate = s.range.first_play_date ?? '';
	});

	const sel = $derived(view.range);
	const metric = $derived(view.metric);
	const title = $derived(
		sel.preset === 'custom' ? dateSpan(sel.from, sel.to) : PRESET_LABEL[sel.preset]
	);
	const hasPrevious = $derived(Boolean(s?.previous));
	const empty = $derived(s && !summary.stale && s.current.plays === 0 && s.current.ms_played === 0);

	// ---------- list adapters ----------
	const artistItems = $derived<TopItem[]>(
		(artists.current?.items ?? []).map((r) => ({
			id: r.artist.id,
			href: artistHref(r.artist.id),
			name: r.artist.name,
			image: r.artist.image_url,
			round: true,
			rank: r.rank,
			prevRank: r.prev_rank,
			plays: r.plays,
			ms: r.ms_played
		}))
	);
	const trackItems = $derived<TopItem[]>(
		(tracks.current?.items ?? []).map((r) => ({
			id: r.track.id,
			href: trackHref(r.track.id),
			name: r.track.name,
			image: r.track.image_url,
			subs: r.track.artists.map((a) => ({ id: a.id, name: a.name, href: artistHref(a.id) })),
			rank: r.rank,
			prevRank: r.prev_rank,
			plays: r.plays,
			ms: r.ms_played
		}))
	);
	const albumItems = $derived<TopItem[]>(
		(albums.current?.items ?? []).map((r) => ({
			id: r.album.id,
			href: albumHref(r.album.id),
			name: r.album.name,
			image: r.album.image_url,
			subs: r.album.artists.map((a) => ({ id: a.id, name: a.name, href: artistHref(a.id) })),
			rank: r.rank,
			prevRank: r.prev_rank,
			plays: r.plays,
			ms: r.ms_played
		}))
	);
	const genreItems = $derived<TopItem[]>(
		(genres.current?.items ?? []).map((r) => ({
			id: r.genre.id,
			href: genreHref(r.genre.id),
			name: r.genre.name,
			rank: r.rank,
			prevRank: r.prev_rank,
			plays: r.plays,
			ms: r.ms_played
		}))
	);
	const lists = $derived([
		{ kind: 'artists', title: 'Top artists', res: artists, items: artistItems },
		{ kind: 'tracks', title: 'Top tracks', res: tracks, items: trackItems },
		{ kind: 'albums', title: 'Top albums', res: albums, items: albumItems },
		{ kind: 'genres', title: 'Top genres', res: genres, items: genreItems }
	] as const);

	// ---------- chart readings ----------
	const tl = $derived(timeline.current);
	const bucket = $derived((tl?.bucket ?? 'day') as Bucket);
	const timelineTitle = $derived(
		`${metric === 'plays' ? 'Plays' : 'Time listened'} by ${BUCKET_NOUN[bucket]}`
	);
	const timelineSummary = $derived.by(() => {
		if (!tl || tl.points.length === 0) return undefined;
		const v = (p: Schemas['TimelinePoint']) => (metric === 'plays' ? p.plays : p.ms_played);
		const total = tl.points.reduce((a, p) => a + v(p), 0);
		const avg = total / tl.points.length;
		const peak = tl.points.reduce((b, p) => (v(p) > v(b) ? p : b), tl.points[0]!);
		const fmt = (n: number) => (metric === 'plays' ? `${int(n)} plays` : duration(n));
		const base = `Average ${fmt(avg)} a ${BUCKET_NOUN[bucket]}. Peak ${bucket === 'week' ? 'week of ' + shortDate(peak.key) : bucketLabel(peak.key, bucket)}, ${fmt(v(peak))}.`;
		if (!tl.previous || !tl.previous_points?.length) return base;
		const prevTotal = tl.previous_points.reduce((a, p) => a + v(p), 0);
		return `${base} Unlit lamps show ${dateSpan(tl.previous.from, tl.previous.to)} (${fmt(prevTotal)}).`;
	});

	const ck = $derived(clock.current);
	const clockSummary = $derived.by(() => {
		if (!ck) return undefined;
		const v = (h: Schemas['ClockHour']) => (metric === 'plays' ? h.plays : h.ms_played);
		const sorted = [...ck.hours].sort((a, b) => v(b) - v(a));
		if (v(sorted[0]!) === 0) return 'No listening in this range.';
		return `Busiest around ${hourLabel(sorted[0]!.hour)}, quietest around ${hourLabel(sorted[23]!.hour)}.`;
	});

	const compare = $derived.by(() => {
		if (!s) return '';
		const days = daySpan(s.range.from, s.range.to);
		if (!s.range.first_play_date) return 'No plays yet.';
		if (!s.previous) {
			return `${int(days)} ${days === 1 ? 'day' : 'days'} of history.`;
		}
		return `Changes are ${compareLabel(sel, days)}, ${dateSpan(s.previous.from, s.previous.to)}.`;
	});
</script>

<svelte:head><title>{title} – spotussi</title></svelte:head>

{#snippet chartSkeleton()}
	<div class="flex min-h-60 flex-1 px-6 pt-6 pb-8 sm:min-h-64">
		<Skeleton class="flex-1 rounded-lg" />
	</div>
{/snippet}

<!-- A chart that throws while drawing stays contained; the table still works. -->
{#snippet chartFailed()}
	<p
		class="grid min-h-60 flex-1 place-items-center px-6 py-10 text-center text-sm text-muted-foreground"
	>
		This chart couldn't be drawn. Switch to Table to see the numbers.
	</p>
{/snippet}

<div class="mx-auto max-w-[90rem] px-4 pt-6 pb-20 sm:px-6 lg:px-10 lg:pt-10">
	<!-- Title + live strip -->
	<header class="flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
		<div class="min-w-0">
			<h1 class="display text-2xl text-balance sm:text-3xl">
				<span class="sr-only">Overview, </span>{title}
			</h1>
			<p class="mt-2 min-h-10 text-sm text-muted-foreground sm:min-h-5">
				{#if s}
					<span class="text-foreground">{dateSpan(s.range.from, s.range.to)}.</span>
					{compare}
				{:else if !summary.error}
					<Skeleton class="inline-block h-4 w-72 align-middle" />
				{/if}
			</p>
		</div>
		<NowPlaying now={now.current} last={recent.current?.items[0]} />
	</header>

	{#if summary.error && (!s || summary.stale)}
		<SectionError class="mt-6" what="the summary" error={summary.error} retry={summary.retry} />
	{/if}

	{#if empty}
		<EmptyState class="mt-6" what="Nothing played" />
	{:else}
		<!-- Summary -->
		<div class="mt-6 transition-opacity duration-200 {summary.stale ? 'opacity-55' : ''}">
			{#if s}
				<SummaryPanel data={s} {metric} />
			{:else if !summary.error}
				<div class="overflow-hidden rounded-xl border bg-card" aria-busy="true">
					<div class="grid grid-cols-2 lg:grid-cols-[1.35fr_1fr_1fr_1fr]">
						{#each { length: 4 } as _, i (i)}
							<div
								class="flex flex-col gap-3 px-5 py-5 sm:px-6 {i % 2 === 1 ? 'border-l' : ''} {i >= 2
									? 'border-t lg:border-t-0'
									: ''} {i === 2 ? 'lg:border-l' : ''}"
							>
								<!-- Line for line the heights of SummaryPanel's readouts. -->
								<Skeleton class="h-5 w-24" />
								<Skeleton class="w-36 {i === 0 ? 'h-9 sm:h-[3.25rem]' : 'h-[1.625rem] sm:h-9'}" />
								<Skeleton class="h-3.5 w-full max-w-72" />
								<Skeleton class="h-[1.1875rem] w-40" />
								{#if i === 0}<div class="h-9 sm:h-[1.125rem]"></div>{/if}
							</div>
						{/each}
					</div>
					<div class="grid border-t sm:grid-cols-2">
						{#each { length: 2 } as _, i (i)}
							<div
								class="flex items-center gap-4 px-5 py-4 sm:px-6 {i
									? 'border-t sm:border-t-0 sm:border-l'
									: ''}"
							>
								<Skeleton class="size-14 rounded-full" />
								<div class="grid gap-1.5">
									<Skeleton class="h-3.5 w-20" /><Skeleton class="h-6 w-44" /><Skeleton
										class="h-3.5 w-28"
									/>
								</div>
							</div>
						{/each}
					</div>
				</div>
			{/if}
		</div>

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
										<th scope="col" class="py-1.5 font-medium capitalize">{BUCKET_NOUN[bucket]}</th>
										<th scope="col" class="py-1.5 text-right font-medium">Plays</th>
										<th scope="col" class="py-1.5 text-right font-medium">Time</th>
										<th scope="col" class="py-1.5 text-right font-medium">Estimated</th>
										<th scope="col" class="py-1.5 text-right font-medium">Artists</th>
										<th scope="col" class="py-1.5 text-right font-medium">Skips</th>
										{#if tl.previous_points?.length}
											<th scope="col" class="py-1.5 text-right font-medium"
												>Previous {metric === 'plays' ? 'plays' : 'time'}</th
											>
										{/if}
									</tr>
								</thead>
								<tbody>
									{#each tl.points as p, i (i)}
										{@const q = tl.previous_points?.[i]}
										<tr class="border-b border-border/60">
											<th scope="row" class="py-1.5 font-normal">{bucketLabel(p.key, bucket)}</th>
											<td class="py-1.5 text-right">{int(p.plays)}</td>
											<td class="py-1.5 text-right">{duration(p.ms_played)}</td>
											<td class="py-1.5 text-right"
												>{p.ms_estimated ? duration(p.ms_estimated) : '–'}</td
											>
											<td class="py-1.5 text-right">{int(p.artists)}</td>
											<td class="py-1.5 text-right">{int(p.skips)}</td>
											{#if q}
												<td class="py-1.5 text-right"
													>{metric === 'plays' ? int(q.plays) : duration(q.ms_played)}</td
												>
											{/if}
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

		<!-- Top lists -->
		<div class="mt-12 grid gap-x-10 gap-y-12 md:grid-cols-2 2xl:grid-cols-4">
			{#each lists as l (l.kind)}
				<div class="transition-opacity duration-200 {l.res.stale ? 'opacity-55' : ''}">
					{#if l.res.error && (!l.res.current || l.res.stale)}
						<SectionError what={l.title.toLowerCase()} error={l.res.error} retry={l.res.retry} />
					{:else if l.res.current}
						<TopList
							kind={l.kind}
							title={l.title}
							allHref={topHref(l.kind)}
							items={l.items}
							total={l.res.current.total}
							{metric}
							{hasPrevious}
							empty={l.kind === 'genres' && s && s.current.plays > 0
								? 'No genres known yet. They arrive with artist details.'
								: undefined}
						/>
					{:else}
						<div aria-busy="true">
							<div class="mb-2 flex justify-between border-b pb-2.5">
								<Skeleton class="h-6 w-28" /><Skeleton class="h-4 w-12" />
							</div>
							<!-- Track and album rows carry an artist line: 63px against 56. -->
							{#each { length: 5 } as _, i (i)}
								<div
									class="grid grid-cols-[1.25rem_auto_1fr_auto] items-center gap-x-3 py-2 {l.kind ===
										'tracks' || l.kind === 'albums'
										? 'min-h-[3.9375rem]'
										: ''}"
								>
									<span></span>
									<Skeleton class="size-10 rounded-[4px]" />
									<div class="grid gap-1.5">
										<Skeleton class="h-4 w-3/4" /><Skeleton class="h-3 w-1/2" />
									</div>
									<Skeleton class="h-4 w-10" />
								</div>
							{/each}
						</div>
					{/if}
				</div>
			{/each}
		</div>

		<!-- Recent plays -->
		<div class="mt-14">
			{#if recent.current}
				<RecentPlays plays={recent.current.items} />
			{:else if recent.error}
				<SectionError what="recent plays" error={recent.error} retry={recent.retry} />
			{/if}
		</div>
	{/if}
</div>
