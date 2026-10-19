<!--
	What the instance is doing, one panel per background job, each with its
	state in the header: live capture, catalog details (D9: why names,
	artwork and some counts are still filling in, with the provisional
	sources of D27), the stats rebuild with how much history there is, and
	the database backup. Imports have their own tab.
-->
<script lang="ts">
	import SettingsPanel from './settings-panel.svelte';
	import Lamp from '#lib/components/shell/lamp.svelte';
	import Download from '@lucide/svelte/icons/download';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import type { Schemas } from '#lib/api/client.js';
	import { ago, dateAt, int } from '#lib/format.js';

	let { data }: { data: Schemas['SystemStatus'] | undefined } = $props();

	type State = { label: string; tone: 'on' | 'off' | 'bad' };

	const capture = $derived.by((): State | null => {
		if (!data) return null;
		switch (data.capture.state) {
			case 'playing':
				return { label: 'Capturing', tone: 'on' };
			case 'idle':
				return { label: 'Listening', tone: 'on' };
			case 'backing_off':
				return { label: 'Retrying after errors', tone: 'bad' };
			default:
				return data.spotify.state === 'reconnect_required'
					? { label: 'Stopped, Spotify needs reconnecting', tone: 'bad' }
					: { label: 'Stopped, Spotify not connected', tone: 'off' };
		}
	});

	const CATALOG_STATE = {
		idle: 'Up to date',
		hydrating: 'Looking up details',
		waiting: 'Waiting for the Spotify connection',
		backing_off: 'Retrying after errors',
		quota_exceeded: 'Daily Spotify lookup limit reached',
		disabled: 'Lookups turned off'
	} as const;
	const catalog = $derived.by((): State | null => {
		if (!data) return null;
		const c = data.catalog;
		const label =
			c.state === 'quota_exceeded' && c.resumes_at
				? `${CATALOG_STATE[c.state]}, resumes ${ago(c.resumes_at)}`
				: CATALOG_STATE[c.state];
		const tone = c.state === 'backing_off' ? 'bad' : c.state === 'hydrating' ? 'on' : 'off';
		return { label, tone };
	});

	// Fallback sources (D27), named by what they are rather than by brand.
	const SOURCE = {
		oembed: 'Spotify embeds',
		deezer: 'Music catalog',
		listenbrainz: 'Open music database, in batches',
		musicbrainz: 'Open music database, one by one'
	} as const;
	const FALLBACK_STATE = {
		idle: 'Idle',
		running: 'Running',
		paused: 'Paused while Spotify answers',
		backing_off: 'Retrying after errors',
		disabled: 'Off'
	} as const;

	const pct = (a: number, b: number) => (b ? Math.min(100, (a / b) * 100) : 0);

	/**
	 * Best match count across sources, capped at what Spotify hasn't covered.
	 * The two open-database stages split the artists between them (D37), so
	 * their artist counts add up.
	 */
	function provisional(kind: 'tracks' | 'artists') {
		if (!data) return 0;
		const c = data.catalog;
		const left =
			kind === 'tracks' ? c.tracks_total - c.tracks_hydrated : c.artists_total - c.artists_hydrated;
		const genreStage = (f: (typeof c.fallbacks)[number]) =>
			f.source === 'listenbrainz' || f.source === 'musicbrainz';
		const best =
			kind === 'tracks'
				? Math.max(0, ...c.fallbacks.filter((f) => !genreStage(f)).map((f) => f.matched))
				: Math.max(
						0,
						...c.fallbacks.filter((f) => !genreStage(f)).map((f) => f.artists_matched),
						c.fallbacks.filter(genreStage).reduce((n, f) => n + f.artists_matched, 0)
					);
		return Math.max(0, Math.min(left, best));
	}
	const meters = $derived(
		data
			? [
					{
						label: 'Tracks',
						done: data.catalog.tracks_hydrated,
						extra: provisional('tracks'),
						total: data.catalog.tracks_total
					},
					{
						label: 'Artists',
						done: data.catalog.artists_hydrated,
						extra: provisional('artists'),
						total: data.catalog.artists_total
					},
					{
						label: 'Playlists and Albums',
						done: data.catalog.contexts_resolved,
						extra: 0,
						total: data.catalog.contexts_total
					}
				]
			: []
	);
	const fallbacks = $derived(data?.catalog.fallbacks.filter((f) => f.state !== 'disabled') ?? []);
</script>

{#snippet status(s: State | null)}
	{#if s}
		<p
			class="flex items-start gap-2 text-sm font-medium {s.tone === 'bad'
				? 'text-destructive'
				: ''}"
		>
			{#if s.tone === 'bad'}
				<span
					aria-hidden="true"
					class="mt-[0.45rem] inline-block size-[5px] shrink-0 rounded-full bg-destructive"
				></span>
			{:else}
				<Lamp on={s.tone === 'on'} class="mt-[0.45rem]" />
			{/if}
			{s.label}
		</p>
	{/if}
{/snippet}

{#snippet facts(rows: [string, string | null][])}
	<dl class="grid gap-x-8 gap-y-2.5 text-sm sm:grid-cols-[9rem_1fr]">
		{#each rows as [k, v] (k)}
			<dt class="text-muted-foreground">{k}</dt>
			<dd class="tnum min-w-0 max-sm:-mt-2 {v === null ? 'text-muted-foreground' : ''}">
				{v ?? 'Never'}
			</dd>
		{/each}
	</dl>
{/snippet}

{#snippet meter(done: number, extra: number, total: number, label: string)}
	<div
		class="flex h-[5px] w-full overflow-hidden rounded-full bg-grille/70"
		role="progressbar"
		aria-label={label}
		aria-valuemin={0}
		aria-valuemax={total}
		aria-valuenow={done}
	>
		<div
			class="h-full bg-signal transition-[width] duration-500"
			style="width: {pct(done, total)}%"
		></div>
		{#if extra > 0}
			<div
				class="h-full bg-signal-dim transition-[width] duration-500"
				style="width: {pct(extra, total)}%"
			></div>
		{/if}
	</div>
{/snippet}

<SettingsPanel
	id="capture"
	title="Live Capture"
	hint="Records every play on your Spotify account as it happens."
>
	{#snippet actions()}{@render status(capture)}{/snippet}
	{#if data}
		<div class="grid gap-4">
			{@render facts([
				['Last Checked', data.capture.last_poll_at ? ago(data.capture.last_poll_at) : null],
				['Last Play', data.capture.last_play_at ? ago(data.capture.last_play_at) : null]
			])}
			{#if data.capture.last_error}
				<p class="rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm">
					{data.capture.last_error}
				</p>
			{/if}
		</div>
	{:else}
		<Skeleton class="h-14 w-full" />
	{/if}
</SettingsPanel>

<SettingsPanel
	id="catalog"
	title="Catalog Details"
	hint="Names, artwork, lengths and genres, fetched from Spotify in the background. While Spotify lookups wait, public music catalogs fill in provisional details. Until a track's length is known its plays use the 30-second rule, so counts can shift a little."
>
	{#snippet actions()}{@render status(catalog)}{/snippet}
	{#if data}
		<div class="grid gap-5">
			<dl class="grid gap-5 sm:grid-cols-3">
				{#each meters as m (m.label)}
					<div class="grid content-start gap-2">
						<dt class="text-xs text-muted-foreground">{m.label}</dt>
						<dd class="grid gap-1.5">
							<span class="figure tnum text-lg leading-none"
								>{Math.floor(pct(m.done, m.total))}%</span
							>
							{@render meter(m.done, m.extra, m.total, `${m.label} from Spotify`)}
							<span class="tnum text-xs text-muted-foreground"
								>{int(m.done)} of {int(m.total)}{#if m.extra > 0}, {int(m.extra)} provisional{/if}</span
							>
						</dd>
					</div>
				{/each}
			</dl>
			{#if data.catalog.queue_length > 0}
				<p class="tnum text-sm text-muted-foreground">
					<span class="text-foreground">{int(data.catalog.queue_length)}</span> to look up{#if data.catalog.eta_at},
						done {ago(data.catalog.eta_at)}{/if}
				</p>
			{/if}
			{#if data.catalog.last_error}
				<p class="rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm">
					{data.catalog.last_error}
				</p>
			{/if}
			{#if fallbacks.length}
				<div class="grid gap-2 border-t pt-4">
					<h3 class="text-sm font-medium" id="sources-h">Provisional Sources</h3>
					<ul class="grid gap-2.5 text-sm" aria-labelledby="sources-h">
						{#each fallbacks as f (f.source)}
							<li class="flex flex-wrap items-center gap-x-2.5 gap-y-0.5">
								<Lamp on={f.state === 'running'} />
								<span class="font-medium sm:w-44">{SOURCE[f.source]}</span>
								<span class="text-muted-foreground max-sm:order-last max-sm:w-full max-sm:pl-[15px]"
									>{FALLBACK_STATE[f.state]}</span
								>
								<span class="tnum ml-auto text-xs text-muted-foreground"
									>{int(f.matched)} matched{#if f.pending}, {int(f.pending)} to go{/if}</span
								>
							</li>
						{/each}
					</ul>
				</div>
			{/if}
		</div>
	{:else}
		<Skeleton class="h-24 w-full" />
	{/if}
</SettingsPanel>

<SettingsPanel
	id="stats"
	title="Stats"
	hint="Totals, top lists and sessions are recalculated after imports, exclusions and setting changes."
>
	{#snippet actions()}
		{@render status(
			data
				? data.derive.state === 'rebuilding'
					? { label: 'Recalculating', tone: 'on' }
					: { label: 'Up to date', tone: 'on' }
				: null
		)}
	{/snippet}
	{#if data}
		{@render facts([
			[
				'Last Rebuild',
				data.derive.last_rebuild_at
					? `${ago(data.derive.last_rebuild_at)}${data.derive.last_rebuild_ms !== null ? `, took ${(data.derive.last_rebuild_ms / 1000).toFixed(1)} s` : ''}`
					: null
			],
			['Plays', int(data.data.plays)],
			['Rows Stored', int(data.data.rows)],
			['First Play', data.data.first_play_at ? dateAt(data.data.first_play_at) : null]
		])}
	{:else}
		<Skeleton class="h-24 w-full" />
	{/if}
</SettingsPanel>

<SettingsPanel id="backup" title="Backup">
	<div class="flex flex-wrap items-center justify-between gap-x-6 gap-y-3">
		<p class="max-w-[52ch] text-sm text-muted-foreground">
			A snapshot of the database: history, settings, sign-in and the Spotify connection. Keep it
			private. Restoring elsewhere also needs the instance key file, or a Spotify reconnect.
		</p>
		<a
			href="/api/v1/system/backup"
			download
			class="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg border bg-card px-3.5 text-sm font-medium transition-colors hover:bg-muted max-sm:w-full max-sm:justify-center"
			><Download class="size-4" aria-hidden="true" />Download Backup</a
		>
	</div>
</SettingsPanel>

<!-- AGPL-3.0 §13: every running instance offers its users the source (D33). -->
<p class="px-1 text-xs text-muted-foreground">
	spotussi is free software under AGPL-3.0.
	<a
		href="https://github.com/jasoncronje/spotussi"
		target="_blank"
		rel="noopener"
		class="underline underline-offset-2 hover:text-foreground">Source code</a
	>. Modified versions run as a service must link their own.
</p>
