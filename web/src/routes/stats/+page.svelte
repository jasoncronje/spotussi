<!--
	Stats (D14): how listening changes over time. Volume (time, plays,
	different artists), how concentrated it is in a few artists, who owns each
	hour of the day, the age of what you play, how many artists share a track,
	and genres over time. Each section loads on its own and keeps its previous
	reading on screen while a new range or metric arrives.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import ChartPanel from '#lib/components/common/chart-panel.svelte';
	import SectionError from '#lib/components/common/section-error.svelte';
	import EmptyState from '#lib/components/common/empty-state.svelte';
	import Segmented from '#lib/components/common/segmented.svelte';
	import Artwork from '#lib/components/common/artwork.svelte';
	import ShareColumns from '#lib/components/stats/share-columns.svelte';
	import AverageDots from '#lib/components/stats/average-dots.svelte';
	const timelineChart = import('#lib/components/overview/timeline-chart.svelte');
	import { Resource } from '#lib/data/resource.svelte.js';
	import { prefetch } from '#lib/data/cache.js';
	import { registerPrefetcher } from '#lib/data/prefetch.js';
	import {
		artistsOverTimeQuery,
		artistsPerTrackQuery,
		genresOverTimeQuery,
		hourlyTopQuery,
		releaseYearsQuery,
		statsQueries,
		timelineQuery
	} from '#lib/data/queries.js';
	import { view, artistHref, genreHref, trackHref, setParams } from '#lib/state/view.svelte.js';
	import { resolvedRange } from '#lib/state/resolved.svelte.js';
	import {
		BUCKET_NOUN,
		bucketLabel,
		dateSpan,
		duration,
		hourLabel,
		hourSpan,
		int,
		type Bucket
	} from '#lib/format.js';
	import { PRESET_LABEL } from '#lib/ranges.js';
	import type { Schemas } from '#lib/api/client.js';

	type Measure = 'time' | 'plays' | 'artists';
	const MEASURES: { value: Measure; label: string }[] = [
		{ value: 'time', label: 'Time' },
		{ value: 'plays', label: 'Plays' },
		{ value: 'artists', label: 'Artists' }
	];
	// The volume chart's measure lives in the URL; it follows the global
	// Plays/Time toggle until you pick one here.
	const measure = $derived.by<Measure>(() => {
		const m = page.url.searchParams.get('measure');
		return m === 'time' || m === 'plays' || m === 'artists' ? m : view.metric;
	});

	const timeline = new Resource(() => timelineQuery(view.range));
	const artists = new Resource(() => artistsOverTimeQuery(view.range, view.metric));
	const hourly = new Resource(() => hourlyTopQuery(view.range, view.metric));
	const years = new Resource(() => releaseYearsQuery(view.range, view.metric));
	const perTrack = new Resource(() => artistsPerTrackQuery(view.range, view.metric));
	const genres = new Resource(() => genresOverTimeQuery(view.range, view.metric));

	onMount(() =>
		registerPrefetcher({
			range: (sel) => prefetch(statsQueries(sel, view.metric)),
			metric: (m) => prefetch(statsQueries(view.range, m))
		})
	);

	const tl = $derived(timeline.current);
	const empty = $derived(
		tl && !timeline.stale && tl.points.every((p) => p.plays === 0 && p.ms_played === 0)
	);
	/** Share of the range's listening an average could use (details known so far). */
	function known(series: Schemas['AverageSeries'] | undefined) {
		if (!series || !tl || tl.points.length !== series.points.length) return 1;
		const w = (i: number) => (metric === 'plays' ? tl.points[i]!.plays : tl.points[i]!.ms_played);
		let sum = 0;
		let cov = 0;
		series.points.forEach((p, i) => {
			sum += w(i);
			cov += w(i) * p.coverage;
		});
		return sum ? cov / sum : 1;
	}
	$effect(() => {
		if (!tl) return;
		resolvedRange.from = tl.range.from;
		resolvedRange.to = tl.range.to;
		resolvedRange.firstDate = tl.range.first_play_date ?? '';
	});

	const sel = $derived(view.range);
	const metric = $derived(view.metric);
	const rangeTitle = $derived(
		sel.preset === 'custom' ? dateSpan(sel.from, sel.to) : PRESET_LABEL[sel.preset]
	);
	const fmt = (n: number) =>
		metric === 'plays' ? `${int(n)} ${n === 1 ? 'play' : 'plays'}` : duration(n);
	const byMetric = $derived(metric === 'plays' ? 'plays' : 'time');

	// ---------- volume ----------
	const bucket = $derived((tl?.bucket ?? 'day') as Bucket);
	const volumeTitle = $derived(
		`${measure === 'plays' ? 'Plays' : measure === 'artists' ? 'Different artists' : 'Time listened'} by ${BUCKET_NOUN[bucket]}`
	);
	const volumeSummary = $derived.by(() => {
		if (!tl || !tl.points.length) return undefined;
		const v = (p: Schemas['TimelinePoint']) =>
			measure === 'plays' ? p.plays : measure === 'artists' ? p.artists : p.ms_played;
		const f = (n: number) =>
			measure === 'plays'
				? `${int(n)} plays`
				: measure === 'artists'
					? `${int(Math.round(n))} artists`
					: duration(n);
		const avg = tl.points.reduce((a, p) => a + v(p), 0) / tl.points.length;
		const peak = tl.points.reduce((b, p) => (v(p) > v(b) ? p : b), tl.points[0]!);
		if (v(peak) === 0) return 'No listening in this range.';
		return `Average ${f(avg)} a ${BUCKET_NOUN[bucket]}. Peak ${bucketLabel(peak.key, bucket)}, ${f(v(peak))}.`;
	});

	// ---------- concentration ----------
	const ar = $derived(artists.current);
	const arBucket = $derived((ar?.bucket ?? 'day') as Bucket);
	const shareOf = (d: { points: Schemas['BreakdownPoint'][] } | undefined, i: number) => {
		if (!d) return 0;
		const total = d.points.reduce((a, p) => a + p.total, 0);
		return total ? d.points.reduce((a, p) => a + (p.values[i] ?? 0), 0) / total : 0;
	};
	const pct = (x: number) => `${Math.round(x * 100)}%`;
	const artistSummary = $derived.by(() => {
		if (!ar || !ar.series.length) return undefined;
		const top = ar.series.reduce((a, _, i) => a + shareOf(ar, i), 0);
		return `Your top ${ar.series.length} artists hold ${pct(top)} of your ${byMetric} in this range.`;
	});

	// ---------- hours ----------
	const hr = $derived(hourly.current);
	const hourMax = $derived(
		Math.max(1, ...(hr?.hours ?? []).map((h) => (metric === 'plays' ? h.plays : h.ms_played)))
	);
	const hourSummary = $derived.by(() => {
		if (!hr) return undefined;
		const counts: Record<string, { name: string; n: number }> = {};
		for (const h of hr.hours) {
			if (!h.top_artist) continue;
			const a = h.top_artist.artist;
			(counts[a.id] ??= { name: a.name, n: 0 }).n++;
		}
		const best = Object.values(counts).sort((a, b) => b.n - a.n)[0];
		if (!best) return 'No listening in this range.';
		return best.n > 1
			? `${best.name} leads ${best.n} of the 24 hours.`
			: 'A different artist leads every hour.';
	});

	// ---------- averages ----------
	const yr = $derived(years.current);
	const pt = $derived(perTrack.current);
	const yearFmt = (v: number) => String(Math.round(v));
	const ratioFmt = (v: number) => v.toFixed(2);
	const yearsSummary = $derived.by(() => {
		if (!yr) return undefined;
		if (yr.overall === null) return 'No release dates known for this range yet.';
		const known = yr.points.filter((p) => p.value !== null);
		if (known.length < 2) return `Weighted by ${byMetric}.`;
		const first = known[0]!.value!;
		const last = known[known.length - 1]!.value!;
		const d = Math.round(last - first);
		return d === 0
			? `Steady across the range, weighted by ${byMetric}.`
			: `${Math.abs(d)} ${Math.abs(d) === 1 ? 'year' : 'years'} ${d > 0 ? 'newer' : 'older'} at the end than at the start, weighted by ${byMetric}.`;
	});
	const perTrackSummary = $derived.by(() => {
		if (!pt) return undefined;
		if (pt.overall === null) return 'No listening in this range.';
		return `Credited artists per track, weighted by ${byMetric}. 1.00 would be all solo tracks.`;
	});

	// ---------- genres ----------
	const gr = $derived(genres.current);
	const grBucket = $derived((gr?.bucket ?? 'day') as Bucket);
	const genreSummary = $derived.by(() => {
		if (!gr || !gr.series.length) return undefined;
		return `${gr.series[0]!.name} leads with ${pct(shareOf(gr, 0))} of your ${byMetric}. A play counts toward every genre of its artists, so shares overlap.`;
	});
</script>

<svelte:head><title>Stats, {rangeTitle} – spotussi</title></svelte:head>

{#snippet chartSkeleton(h = 'min-h-56')}
	<div class="flex {h} flex-1 px-6 pt-4 pb-6"><Skeleton class="flex-1 rounded-lg" /></div>
{/snippet}
{#snippet chartFailed()}
	<p
		class="grid min-h-56 flex-1 place-items-center px-6 py-10 text-center text-sm text-muted-foreground"
	>
		This chart couldn't be drawn. Switch to Table to see the numbers.
	</p>
{/snippet}
{#snippet noData(what: string)}
	<p
		class="grid min-h-40 flex-1 place-items-center px-6 py-10 text-center text-sm text-muted-foreground"
	>
		{what}
	</p>
{/snippet}

{#snippet legend(
	series: { id: string; name: string; href: string; share: number }[],
	other: string
)}
	<ul class="flex flex-wrap gap-x-5 gap-y-1.5 px-5 pb-5 text-sm sm:px-6">
		{#each series as s, i (s.id)}
			<li class="flex min-w-0 items-center gap-2">
				<span class="size-2.5 shrink-0 rounded-[2px]" style="background: var(--chart-{(i % 5) + 1})"
				></span>
				<a
					href={s.href}
					class="truncate hover:underline hover:decoration-signal hover:underline-offset-4"
					>{s.name}</a
				>
				<span class="tnum text-xs text-muted-foreground">{pct(s.share)}</span>
			</li>
		{/each}
		<li class="flex items-center gap-2 text-muted-foreground">
			<span class="size-2.5 shrink-0 rounded-[2px] bg-grille"></span>{other}
		</li>
	</ul>
{/snippet}

{#snippet breakdownTable(
	caption: string,
	d: { series: { name: string }[]; points: Schemas['BreakdownPoint'][] },
	b: Bucket
)}
	<table class="tnum w-full text-left text-xs">
		<caption class="sr-only">{caption}</caption>
		<thead class="text-muted-foreground">
			<tr class="border-b">
				<th scope="col" class="py-1.5 font-medium capitalize">{BUCKET_NOUN[b]}</th>
				<th scope="col" class="py-1.5 text-right font-medium">Total</th>
				{#each d.series as s, i (i)}
					<th scope="col" class="py-1.5 pl-3 text-right font-medium">{s.name}</th>
				{/each}
				<th scope="col" class="py-1.5 pl-3 text-right font-medium">Everything else</th>
			</tr>
		</thead>
		<tbody>
			{#each d.points as p, n (n)}
				<tr class="border-b border-border/60">
					<th scope="row" class="py-1.5 font-normal whitespace-nowrap">{bucketLabel(p.key, b)}</th>
					<td class="py-1.5 text-right whitespace-nowrap">{fmt(p.total)}</td>
					{#each p.values as v, i (i)}
						<td class="py-1.5 pl-3 text-right">{p.total ? pct(v / p.total) : '–'}</td>
					{/each}
					<td class="py-1.5 pl-3 text-right">{p.total ? pct(p.other / p.total) : '–'}</td>
				</tr>
			{/each}
		</tbody>
	</table>
{/snippet}

{#snippet averageTable(
	caption: string,
	d: Schemas['AverageSeries'],
	b: Bucket,
	f: (v: number) => string
)}
	<table class="tnum w-full text-left text-xs">
		<caption class="sr-only">{caption}</caption>
		<thead class="text-muted-foreground">
			<tr class="border-b">
				<th scope="col" class="py-1.5 font-medium capitalize">{BUCKET_NOUN[b]}</th>
				<th scope="col" class="py-1.5 text-right font-medium">Average</th>
				<th scope="col" class="py-1.5 text-right font-medium">Known</th>
			</tr>
		</thead>
		<tbody>
			{#each d.points as p, n (n)}
				<tr class="border-b border-border/60">
					<th scope="row" class="py-1.5 font-normal">{bucketLabel(p.key, b)}</th>
					<td class="py-1.5 text-right">{p.value === null ? '–' : f(p.value)}</td>
					<td class="py-1.5 text-right">{pct(p.coverage)}</td>
				</tr>
			{/each}
		</tbody>
	</table>
{/snippet}

<div class="mx-auto max-w-[90rem] px-4 pt-6 pb-20 sm:px-6 lg:px-10 lg:pt-10">
	<header>
		<h1 class="display text-2xl text-balance sm:text-3xl">Stats</h1>
		<p class="mt-2 min-h-5 text-sm text-muted-foreground">
			{#if tl}
				<span class="text-foreground">{rangeTitle}, {dateSpan(tl.range.from, tl.range.to)}</span>
			{:else if !timeline.error}
				<Skeleton class="inline-block h-4 w-72 align-middle" />
			{/if}
		</p>
	</header>

	{#if empty}
		<EmptyState class="mt-6" what="Nothing played" />
	{:else}
		<!-- Volume -->
		<div class="mt-6">
			{#if timeline.error && (!tl || timeline.stale)}
				<SectionError what="listening over time" error={timeline.error} retry={timeline.retry} />
			{:else}
				<ChartPanel title={volumeTitle} summary={volumeSummary} dim={timeline.stale}>
					{#snippet meta()}
						<Segmented
							label="Measure"
							options={MEASURES}
							value={measure}
							onchange={(m) => setParams({ measure: m === metric ? null : m }, { replace: true })}
						/>
					{/snippet}
					{#if tl}
						{#await timelineChart}
							{@render chartSkeleton('min-h-64')}
						{:then { default: TimelineChart }}
							<svelte:boundary>
								<!-- A definite height: nothing beside this panel stretches it. -->
								<div class="flex h-64 flex-col sm:h-72">
									<TimelineChart timeline={tl} metric={measure} />
								</div>
								{#snippet failed()}{@render chartFailed()}{/snippet}
							</svelte:boundary>
						{/await}
					{:else}
						{@render chartSkeleton('min-h-64')}
					{/if}
					{#snippet table()}
						{#if tl}
							<table class="tnum w-full text-left text-xs">
								<caption class="sr-only">Listening by {BUCKET_NOUN[bucket]}</caption>
								<thead class="text-muted-foreground">
									<tr class="border-b">
										<th scope="col" class="py-1.5 font-medium capitalize">{BUCKET_NOUN[bucket]}</th>
										<th scope="col" class="py-1.5 text-right font-medium">Time</th>
										<th scope="col" class="py-1.5 text-right font-medium">Plays</th>
										<th scope="col" class="py-1.5 text-right font-medium">Artists</th>
									</tr>
								</thead>
								<tbody>
									{#each tl.points as p, n (n)}
										<tr class="border-b border-border/60">
											<th scope="row" class="py-1.5 font-normal">{bucketLabel(p.key, bucket)}</th>
											<td class="py-1.5 text-right">{duration(p.ms_played)}</td>
											<td class="py-1.5 text-right">{int(p.plays)}</td>
											<td class="py-1.5 text-right">{int(p.artists)}</td>
										</tr>
									{/each}
								</tbody>
							</table>
						{/if}
					{/snippet}
				</ChartPanel>
			{/if}
		</div>

		<!-- Concentration -->
		<div class="mt-4">
			{#if artists.error && (!ar || artists.stale)}
				<SectionError what="top artist shares" error={artists.error} retry={artists.retry} />
			{:else}
				<ChartPanel
					title="Share held by your top artists"
					summary={artistSummary}
					dim={artists.stale}
				>
					{#if ar}
						{#if ar.series.length}
							<ShareColumns series={ar.series} points={ar.points} bucket={arBucket} {metric} />
							{@render legend(
								ar.series.map((a, i) => ({
									id: a.id,
									name: a.name,
									href: artistHref(a.id),
									share: shareOf(ar, i)
								})),
								'Everyone else'
							)}
						{:else}
							{@render noData('No artists in this range.')}
						{/if}
					{:else}
						{@render chartSkeleton()}
					{/if}
					{#snippet table()}
						{#if ar}{@render breakdownTable('Share of listening by top artist', ar, arBucket)}{/if}
					{/snippet}
				</ChartPanel>
			{/if}
		</div>

		<!-- Hours -->
		<section class="mt-12" aria-labelledby="hours-title">
			<div class="flex items-baseline justify-between gap-4 border-b pb-2.5">
				<h2 id="hours-title" class="text-base font-semibold">Who you play at each hour</h2>
				{#if hourSummary}<p class="text-xs text-muted-foreground max-sm:hidden">
						{hourSummary}
					</p>{/if}
			</div>
			{#if hourly.error && (!hr || hourly.stale)}
				<SectionError class="mt-4" what="top by hour" error={hourly.error} retry={hourly.retry} />
			{:else if hr}
				<div class="transition-opacity duration-200 {hourly.stale ? 'opacity-55' : ''}">
					<table class="w-full border-collapse text-left text-sm">
						<caption class="sr-only"
							>Top artist and track for each hour of the day, by {byMetric}</caption
						>
						<thead class="text-xs text-muted-foreground">
							<tr class="border-b">
								<th scope="col" class="w-20 py-2 font-medium">Hour</th>
								<th scope="col" class="hidden w-[22%] py-2 pr-4 font-medium sm:table-cell">
									{metric === 'plays' ? 'Plays' : 'Time'}
								</th>
								<th scope="col" class="py-2 pr-4 font-medium">Top artist</th>
								<th scope="col" class="hidden py-2 font-medium md:table-cell">Top track</th>
							</tr>
						</thead>
						<tbody>
							{#each hr.hours as h (h.hour)}
								{@const v = metric === 'plays' ? h.plays : h.ms_played}
								<tr class="border-b border-border/60 last:border-b-0">
									<th
										scope="row"
										class="tnum py-1.5 pr-2 align-middle font-normal whitespace-nowrap"
									>
										<span class="sr-only">{hourSpan(h.hour)}</span>
										<span aria-hidden="true">{hourLabel(h.hour)}</span>
									</th>
									<td class="hidden py-1.5 pr-4 align-middle sm:table-cell">
										<div class="flex items-center gap-3">
											<div class="h-[5px] flex-1 rounded-full bg-grille/70" aria-hidden="true">
												<div
													class="h-full rounded-full bg-signal"
													style="width: {Math.max(v ? 2 : 0, (v / hourMax) * 100)}%"
												></div>
											</div>
											<span
												class="tnum w-20 text-right text-xs whitespace-nowrap text-muted-foreground"
												>{v ? fmt(v) : '–'}</span
											>
										</div>
									</td>
									<td class="py-1.5 pr-4 align-middle">
										{#if h.top_artist}
											<div class="flex min-w-0 items-center gap-2.5">
												<Artwork
													src={h.top_artist.artist.image_url}
													name={h.top_artist.artist.name}
													round
													class="size-8"
												/>
												<div class="min-w-0">
													<a
														href={artistHref(h.top_artist.artist.id)}
														class="hit-above line-clamp-1 font-medium break-all hover:underline hover:decoration-signal hover:underline-offset-4"
														>{h.top_artist.artist.name}</a
													>
													<p class="tnum text-xs text-muted-foreground">
														{fmt(
															metric === 'plays' ? h.top_artist.plays : h.top_artist.ms_played
														)}{#if h.top_track}<span class="md:hidden"
																>, top track <a
																	href={trackHref(h.top_track.track.id)}
																	class="hit-below hover:text-foreground hover:underline"
																	>{h.top_track.track.name}</a
																></span
															>{/if}
													</p>
												</div>
											</div>
										{:else}
											<span class="text-faint">Nothing</span>
										{/if}
									</td>
									<td class="hidden py-1.5 align-middle md:table-cell">
										{#if h.top_track}
											<div class="flex min-w-0 items-center gap-2.5">
												<Artwork
													src={h.top_track.track.image_url}
													name={h.top_track.track.name}
													class="size-8"
												/>
												<div class="min-w-0">
													<a
														href={trackHref(h.top_track.track.id)}
														class="hit-above line-clamp-1 font-medium break-all hover:underline hover:decoration-signal hover:underline-offset-4"
														>{h.top_track.track.name}</a
													>
													<p class="truncate text-xs text-muted-foreground">
														{h.top_track.track.artists.map((a) => a.name).join(', ')}, {fmt(
															metric === 'plays' ? h.top_track.plays : h.top_track.ms_played
														)}
													</p>
												</div>
											</div>
										{/if}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{:else}
				<div aria-busy="true" class="grid gap-2 pt-3">
					{#each { length: 8 } as _, i (i)}<Skeleton class="h-9 w-full" />{/each}
				</div>
			{/if}
		</section>

		<!-- Averages -->
		<div class="mt-12 grid gap-4 lg:grid-cols-2">
			{#if years.error && (!yr || years.stale)}
				<SectionError what="release years" error={years.error} retry={years.retry} />
			{:else}
				<ChartPanel title="Average release year" summary={yearsSummary} dim={years.stale}>
					{#snippet meta()}
						{#if yr?.overall != null}
							<span class="figure tnum text-xl leading-none">{yearFmt(yr.overall)}</span>
						{/if}
					{/snippet}
					{#if yr}
						<AverageDots
							points={yr.points}
							overall={yr.overall}
							bucket={yr.bucket as Bucket}
							format={yearFmt}
						/>
					{:else}
						{@render chartSkeleton()}
					{/if}
					{#snippet table()}
						{#if yr}{@render averageTable(
								'Average release year',
								yr,
								yr.bucket as Bucket,
								yearFmt
							)}{/if}
					{/snippet}
				</ChartPanel>
			{/if}
			{#if perTrack.error && (!pt || perTrack.stale)}
				<SectionError what="artists per track" error={perTrack.error} retry={perTrack.retry} />
			{:else}
				<ChartPanel
					title="Average artists per track"
					summary={perTrackSummary}
					dim={perTrack.stale}
				>
					{#snippet meta()}
						{#if pt?.overall != null}
							<span class="figure tnum text-xl leading-none">{ratioFmt(pt.overall)}</span>
						{/if}
					{/snippet}
					{#if pt}
						<AverageDots
							points={pt.points}
							overall={pt.overall}
							bucket={pt.bucket as Bucket}
							format={ratioFmt}
							step={0.05}
						/>
					{:else}
						{@render chartSkeleton()}
					{/if}
					{#snippet table()}
						{#if pt}{@render averageTable(
								'Average artists per track',
								pt,
								pt.bucket as Bucket,
								ratioFmt
							)}{/if}
					{/snippet}
				</ChartPanel>
			{/if}
		</div>
		<p class="mt-3 text-xs text-muted-foreground">
			Dashed line: the range's average. Fainter lamps: less of that listening has the detail.{#if yr && known(yr) < 0.9}
				Release years are known for {Math.round(known(yr) * 100)}% of this listening so far; the
				rest fills in as track details are looked up.{/if}
		</p>

		<!-- Genres -->
		<div class="mt-12">
			{#if genres.error && (!gr || genres.stale)}
				<SectionError what="genres over time" error={genres.error} retry={genres.retry} />
			{:else}
				<ChartPanel title="Genres over time" summary={genreSummary} dim={genres.stale}>
					{#if gr}
						{#if gr.series.length}
							<ShareColumns series={gr.series} points={gr.points} bucket={grBucket} {metric} />
							{@render legend(
								gr.series.map((g, i) => ({
									id: g.id,
									name: g.name,
									href: genreHref(g.id),
									share: shareOf(gr, i)
								})),
								'Other genres'
							)}
						{:else}
							{@render noData(
								'No genres known for this range yet. Genres arrive as artist details are fetched.'
							)}
						{/if}
					{:else}
						{@render chartSkeleton()}
					{/if}
					{#snippet table()}
						{#if gr}{@render breakdownTable('Share of listening by genre', gr, grBucket)}{/if}
					{/snippet}
				</ChartPanel>
			{/if}
		</div>
	{/if}
</div>
