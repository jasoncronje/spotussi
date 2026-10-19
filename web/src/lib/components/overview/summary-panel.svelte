<!--
	The summary readouts: time listened, plays, tracks, artists, then the top
	artist and top track. Every figure carries its change vs the previous
	period of equal length; the meter's tick marks where that period landed.
-->
<script lang="ts">
	import LevelMeter from '#lib/components/common/level-meter.svelte';
	import Artwork from '#lib/components/common/artwork.svelte';
	import EstimatedNote from '#lib/components/common/estimated-note.svelte';
	import MixMeter, { type MixPart } from '#lib/components/common/mix-meter.svelte';
	import {
		delta,
		durationParts,
		duration,
		int,
		metricValue,
		type DurationPart
	} from '#lib/format.js';
	import { artistHref, trackHref } from '#lib/state/view.svelte.js';
	import type { Schemas } from '#lib/api/client.js';
	import { sources as dataSources } from '#lib/state/sources.svelte.js';
	import type { Metric } from '#lib/ranges.js';

	let { data, metric }: { data: Schemas['OverviewSummary']; metric: Metric } = $props();

	const cur = $derived(data.current);
	const prev = $derived(data.previous_summary);

	type Readout = {
		label: string;
		parts: DurationPart[];
		value: number;
		previous: number | null;
		previousText: string | null;
		hero?: boolean;
	};

	const num = (n: number): DurationPart[] => [{ value: int(n), unit: '' }];

	const readouts = $derived<Readout[]>([
		{
			label: 'Time listened',
			parts: durationParts(cur.ms_played),
			value: cur.ms_played,
			previous: prev?.ms_played ?? null,
			previousText: prev ? duration(prev.ms_played) : null,
			hero: true
		},
		{
			label: 'Plays',
			parts: num(cur.plays),
			value: cur.plays,
			previous: prev?.plays ?? null,
			previousText: prev ? int(prev.plays) : null
		},
		{
			label: 'Tracks',
			parts: num(cur.tracks),
			value: cur.tracks,
			previous: prev?.tracks ?? null,
			previousText: prev ? int(prev.tracks) : null
		},
		{
			label: 'Artists',
			parts: num(cur.artists),
			value: cur.artists,
			previous: prev?.artists ?? null,
			previousText: prev ? int(prev.artists) : null
		}
	]);

	// Provenance (D8, D27): where the range's listening came from, and how
	// much of it has details from Spotify yet.
	const val = (m: Schemas['Measure']) => (metric === 'plays' ? m.plays : m.ms_played);
	const sourceParts = $derived<MixPart[]>([
		{
			key: 'export',
			label: 'Export',
			all: 'All from your export',
			value: val(data.sources.export),
			tone: 'bg-chart-2'
		},
		{
			key: 'both',
			label: 'Live + export',
			all: 'All live, confirmed by your export',
			value: val(data.sources.both),
			tone: 'bg-signal'
		},
		{
			key: 'live',
			label: 'Live only',
			all: 'All from live capture',
			value: val(data.sources.live),
			tone: 'bg-chart-4'
		}
	]);
	const catalogParts = $derived<MixPart[]>([
		{
			key: 'spotify',
			label: 'Spotify',
			all: 'All from Spotify',
			value: val(data.catalog.spotify),
			tone: 'bg-signal'
		},
		{
			key: 'fallback',
			label: 'Provisional',
			all: 'All provisional',
			value: val(data.catalog.fallback),
			tone: 'bg-signal-dim'
		},
		{
			key: 'pending',
			label: 'Names only',
			all: 'Names only so far',
			value: val(data.catalog.pending),
			tone: 'bg-grille'
		}
	]);
	const hasListening = $derived(cur.plays > 0 || cur.ms_played > 0);

	function movement(rank: number, prevRank: number | null) {
		if (!data.previous) return null;
		if (prevRank === null) return 'New this period';
		if (prevRank === rank) return 'Also #1 the period before';
		return `Up from #${int(prevRank)}`;
	}
</script>

<section aria-label="Summary" class="overflow-hidden rounded-xl border bg-card">
	<dl class="grid grid-cols-2 lg:grid-cols-[1.35fr_1fr_1fr_1fr]">
		{#each readouts as r, i (r.label)}
			{@const d = delta(r.value, r.previous)}
			<div
				class="flex min-w-0 flex-col gap-3 px-5 py-5 sm:px-6 {i % 2 === 1 ? 'border-l' : ''} {i >= 2
					? 'border-t lg:border-t-0'
					: ''} {i === 2 ? 'lg:border-l' : ''}"
			>
				<dt class="text-sm text-muted-foreground">{r.label}</dt>
				<dd class="contents">
					<p
						class="figure leading-none whitespace-nowrap {r.hero
							? 'text-2xl sm:text-3xl'
							: 'text-xl sm:text-2xl'}"
					>
						{#each r.parts as p, j (j)}
							<span class={j > 0 ? 'ml-[0.3em]' : ''}>{p.value}</span>{#if p.unit}<span
									class="ml-[0.12em] text-[0.5em] font-medium tracking-normal text-muted-foreground"
									>{p.unit}</span
								>{/if}
						{/each}
					</p>
					{#if r.previous !== null}
						<LevelMeter value={r.value} previous={r.previous} />
					{/if}
					<p class="tnum flex flex-wrap items-baseline gap-x-1.5 text-xs text-muted-foreground">
						{#if d.kind === 'change'}
							<span class="font-semibold text-foreground">
								<span aria-hidden="true" class="text-[0.75em]"
									>{d.dir === 'up' ? '▲' : d.dir === 'down' ? '▼' : ''}</span
								>
								{d.text.replace(/^[+−]/, '')}
								<span class="sr-only"
									>{d.dir === 'up' ? 'more' : d.dir === 'down' ? 'less' : ''}</span
								>
							</span>
							<span>from {r.previousText}</span>
						{:else if d.kind === 'new'}
							<span class="font-semibold text-foreground">New</span>
							<span>none the period before</span>
						{:else if r.previous === null}
							<span>No earlier period to compare</span>
						{:else}
							<span>Same as the period before</span>
						{/if}
					</p>
					{#if r.hero}
						<!-- Room for the note either way, so the panel keeps its height
						     from skeleton to data (it wraps in the narrow phone cell). -->
						<div class="min-h-9 sm:min-h-[1.125rem]">
							{#if cur.ms_estimated > 0}
								<EstimatedNote ms={cur.ms_estimated} total={cur.ms_played} />
							{/if}
						</div>
					{/if}
				</dd>
			</div>
		{/each}
	</dl>

	<div class="grid border-t sm:grid-cols-2">
		{#each [{ label: 'Top artist', item: cur.top_artist }, { label: 'Top track', item: cur.top_track }] as spot, i (spot.label)}
			<div
				class="flex min-w-0 items-center gap-4 px-5 py-4 sm:px-6 {i === 1
					? 'border-t sm:border-t-0 sm:border-l'
					: ''}"
			>
				{#if spot.item && 'artist' in spot.item}
					{@const a = spot.item}
					<Artwork src={a.artist.image_url} name={a.artist.name} round class="size-14" />
					<div class="min-w-0">
						<p class="text-xs text-muted-foreground">{spot.label}</p>
						<a
							href={artistHref(a.artist.id)}
							class="display block truncate text-lg leading-snug hover:underline hover:decoration-signal hover:underline-offset-4"
							>{a.artist.name}</a
						>
						<p class="tnum text-xs text-muted-foreground">
							{metricValue(metric, a)}{#if movement(1, a.prev_rank)}, {movement(
									1,
									a.prev_rank
								)?.toLowerCase()}{/if}
						</p>
					</div>
				{:else if spot.item && 'track' in spot.item}
					{@const t = spot.item}
					<Artwork src={t.track.image_url} name={t.track.name} class="size-14" />
					<div class="min-w-0">
						<p class="text-xs text-muted-foreground">{spot.label}</p>
						<a
							href={trackHref(t.track.id)}
							class="display block truncate text-lg leading-snug hover:underline hover:decoration-signal hover:underline-offset-4"
							>{t.track.name}</a
						>
						<p class="tnum truncate text-xs text-muted-foreground">
							{#each t.track.artists as ar, k (ar.id)}{k > 0 ? ', ' : ''}<a
									href={artistHref(ar.id)}
									class="hit-below hover:text-foreground hover:underline">{ar.name}</a
								>{/each}; {metricValue(metric, t)}{#if movement(1, t.prev_rank)}, {movement(
									1,
									t.prev_rank
								)?.toLowerCase()}{/if}
						</p>
					</div>
				{:else}
					<div class="grid size-14 place-items-center rounded-[4px] border border-dashed"></div>
					<div>
						<p class="text-xs text-muted-foreground">{spot.label}</p>
						<p class="text-muted-foreground">Nothing played</p>
					</div>
				{/if}
			</div>
		{/each}
	</div>

	{#if hasListening && dataSources.show}
		<div class="grid gap-5 border-t px-5 py-4 sm:grid-cols-2 sm:px-6">
			<MixMeter
				label="Plays from"
				parts={sourceParts}
				hint="Live: seen by live capture as you listened. Export: in the streaming history export you imported. Live + export: seen live, then confirmed by the export."
			/>
			<MixMeter
				label="Track details"
				parts={catalogParts}
				hint="Artwork, lengths and genres. Provisional details come from public music catalogs while Spotify lookups wait, and are replaced once Spotify answers. Names only: still to be looked up."
			/>
		</div>
	{/if}
</section>
