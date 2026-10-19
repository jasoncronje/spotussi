<!--
	Library (D36): the songs, albums and playlists saved on Spotify, held
	against the listening. The headline is what was saved and never played;
	the readouts say how long saving takes to turn into plays, where heavy
	rotation is a song's 5th counted play. All-time: saves have their own
	dates, so the shell's range doesn't apply here.
-->
<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import SectionError from '#lib/components/common/section-error.svelte';
	import Segmented from '#lib/components/common/segmented.svelte';
	import Pager from '#lib/components/common/pager.svelte';
	import Artwork from '#lib/components/common/artwork.svelte';
	import Hint from '#lib/components/common/hint.svelte';
	import MixMeter, { type MixPart } from '#lib/components/common/mix-meter.svelte';
	import SavesChart from '#lib/components/library/saves-chart.svelte';
	import SongRow from '#lib/components/library/song-row.svelte';
	import ShareDots from '#lib/components/library/share-dots.svelte';
	import { Resource } from '#lib/data/resource.svelte.js';
	import { prefetch } from '#lib/data/cache.js';
	import {
		libraryAlbumsQuery,
		libraryPlaylistsQuery,
		libraryPlaylistQuery,
		librarySummaryQuery,
		libraryTimelineQuery,
		libraryTracksQuery,
		statusQuery,
		type LibraryAlbumSort,
		type LibraryPlayed,
		type LibraryPlaylistSort,
		type LibraryRelation,
		type LibraryTrackSort
	} from '#lib/data/queries.js';
	import {
		albumHref,
		artistHref,
		hrefWith,
		libraryPlaylistHref,
		setParams
	} from '#lib/state/view.svelte.js';
	import { settingsHref } from '#lib/components/settings/tabs.js';
	import { ago, dateAt, duration, int } from '#lib/format.js';

	type Tab = 'songs' | 'albums' | 'playlists';
	const PAGE_SIZE = 50;

	const params = $derived(page.url.searchParams);
	const tab = $derived(
		(['songs', 'albums', 'playlists'].includes(params.get('tab') ?? '')
			? params.get('tab')
			: 'songs') as Tab
	);
	const pageNo = $derived(Math.max(1, Number(params.get('page')) || 1));
	const offset = $derived((pageNo - 1) * PAGE_SIZE);

	const summary = new Resource(() => librarySummaryQuery());
	const timeline = new Resource(() => libraryTimelineQuery());
	const status = new Resource(() => statusQuery());
	const s = $derived(summary.current);
	const lib = $derived(status.current?.library);

	// Songs: never played first, it's the headline; all when there are none.
	const played = $derived.by((): LibraryPlayed => {
		const p = params.get('played');
		if (p === 'all' || p === 'played' || p === 'maybe_played' || p === 'never_played') return p;
		return s && s.tracks.never_played === 0 ? 'all' : 'never_played';
	});
	const trackSort = $derived((params.get('sort') ?? 'added') as LibraryTrackSort);
	const albumSort = $derived((params.get('sort') ?? 'added') as LibraryAlbumSort);
	const playlistSort = $derived((params.get('sort') ?? 'plays_here') as LibraryPlaylistSort);
	const relation = $derived((params.get('relation') ?? 'all') as LibraryRelation);

	const tracks = new Resource(() =>
		tab === 'songs' && s ? libraryTracksQuery(played, trackSort, PAGE_SIZE, offset) : null
	);
	const albums = new Resource(() =>
		tab === 'albums' ? libraryAlbumsQuery(albumSort, PAGE_SIZE, offset) : null
	);
	const playlists = new Resource(() =>
		tab === 'playlists' ? libraryPlaylistsQuery(relation, playlistSort, PAGE_SIZE, offset) : null
	);

	const pct = (n: number, of: number) => (of ? Math.round((n / of) * 100) : 0);
	const daysFigure = (d: number | null | undefined) =>
		d == null ? '–' : d < 10 ? d.toFixed(1).replace(/\.0$/, '') : int(Math.round(d));

	const mix = $derived.by((): MixPart[] => {
		if (!s) return [];
		const m = s.rotation_mix;
		return [
			{
				key: 'before',
				label: 'Before saving',
				all: 'All before saving',
				value: m.before_saving,
				tone: 'bg-chart-2'
			},
			{
				key: 'day',
				label: 'Within a day',
				all: 'All within a day',
				value: m.within_day,
				tone: 'bg-signal'
			},
			{
				key: 'week',
				label: 'Within a week',
				all: 'All within a week',
				value: m.within_week,
				tone: 'bg-signal/70'
			},
			{
				key: 'month',
				label: 'Within a month',
				all: 'All within a month',
				value: m.within_month,
				tone: 'bg-signal/45'
			},
			{ key: 'later', label: 'Later', all: 'All later', value: m.later, tone: 'bg-foreground/45' },
			{ key: 'not', label: 'Not yet', all: 'None yet', value: m.not_yet, tone: 'bg-grille' }
		];
	});

	/** Why the library is empty, if it is. */
	const emptyReason = $derived.by(() => {
		if (!s || s.saved_tracks + s.saved_albums + s.playlists > 0) return null;
		if (!lib || lib.state === 'disabled') return 'disabled';
		if (lib.state === 'not_connected') return 'not_connected';
		if (lib.lists.tracks.state === 'needs_scope') return 'needs_scope';
		if (!lib.last_sync_at) return 'first_sync';
		return 'nothing';
	});

	// Playlists sync without user-library-read, so the page can have rows and
	// still be missing saved songs and albums (D36).
	const needsScope = $derived(
		lib?.lists.tracks.state === 'needs_scope' || lib?.lists.albums.state === 'needs_scope'
	);
	const reconnectHref = $derived(
		`${resolve('/welcome')}?step=connect&reconnect=1&next=${encodeURIComponent(resolve('/library'))}`
	);

	function choose(changes: Record<string, string | null>) {
		setParams({ ...changes, page: null }, { replace: true });
	}
	const playlistOwner = (p: { relation: string; owner_name: string | null }) =>
		p.relation === 'own'
			? 'Yours'
			: p.relation === 'collaborative'
				? `Collaborative, by ${p.owner_name ?? 'someone else'}`
				: `Followed, by ${p.owner_name ?? 'someone else'}`;
</script>

<svelte:head><title>Library – spotussi</title></svelte:head>

<div class="mx-auto max-w-[90rem] px-4 pt-6 pb-20 sm:px-6 lg:px-10 lg:pt-10">
	<header>
		<h1 class="display text-2xl text-balance sm:text-3xl">Library</h1>
		<p class="mt-2 min-h-5 text-sm text-muted-foreground">
			{#if s}
				<span class="text-foreground"
					>{int(s.saved_tracks)} saved {s.saved_tracks === 1 ? 'song' : 'songs'}, {int(
						s.saved_albums
					)}
					{s.saved_albums === 1 ? 'album' : 'albums'} and {int(s.playlists)}
					{s.playlists === 1 ? 'playlist' : 'playlists'}.</span
				>
				{#if s.last_sync_at}
					<a
						href={settingsHref('spotify')}
						class="hit underline decoration-faint underline-offset-2 hover:text-foreground"
						>Synced {ago(s.last_sync_at)}</a
					>.
				{/if}
			{:else if !summary.error}
				<Skeleton class="inline-block h-4 w-72 align-middle" />
			{/if}
		</p>
		{#if needsScope && emptyReason !== 'needs_scope'}
			<p class="mt-2 text-sm text-muted-foreground">
				Saved songs and albums need one more Spotify permission.
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
				<a href={reconnectHref} class="inline-link hit font-medium"
					>Reconnect to grant library access</a
				>.
			</p>
		{/if}
	</header>

	{#if summary.error && !s}
		<SectionError class="mt-6" what="your library" error={summary.error} retry={summary.retry} />
	{:else if emptyReason}
		<section class="mt-6 rounded-xl border border-dashed bg-card/50 px-6 py-10 sm:px-10">
			{#if emptyReason === 'first_sync'}
				<h2 class="text-lg font-semibold">Reading your library from Spotify</h2>
				<p class="mt-1 max-w-[60ch] text-muted-foreground">
					The first sync lists your saved songs, albums and playlists. It shows up here when it
					finishes, usually within a few minutes.
				</p>
			{:else if emptyReason === 'needs_scope'}
				<h2 class="text-lg font-semibold">Spotify hasn't shared your saved songs</h2>
				<p class="mt-1 max-w-[60ch] text-muted-foreground">
					Reading them needs a permission this connection doesn't have. Reconnect Spotify once and
					approve the consent page to grant it.
				</p>
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
				<a
					href={reconnectHref}
					class="mt-4 inline-flex h-9 items-center rounded-lg bg-primary px-3.5 text-sm font-medium text-primary-foreground"
					>Reconnect to grant library access</a
				>
			{:else if emptyReason === 'not_connected'}
				<h2 class="text-lg font-semibold">Connect Spotify to read your library</h2>
				<p class="mt-1 max-w-[60ch] text-muted-foreground">
					Your saved songs, albums and playlists appear here after the first sync.
				</p>
				<a
					href={settingsHref('spotify')}
					class="mt-4 inline-flex h-9 items-center rounded-lg border bg-card px-3.5 text-sm font-medium hover:bg-muted"
					>Open Spotify settings</a
				>
			{:else if emptyReason === 'disabled'}
				<h2 class="text-lg font-semibold">Library sync is off</h2>
				<p class="mt-1 max-w-[60ch] text-muted-foreground">
					Set <code class="rounded bg-muted px-1 py-0.5 text-xs text-foreground"
						>LIBRARY_SYNC_ENABLED=true</code
					> and restart to read your saved songs, albums and playlists from Spotify.
				</p>
			{:else}
				<h2 class="text-lg font-semibold">Nothing saved yet</h2>
				<p class="mt-1 max-w-[60ch] text-muted-foreground">
					Songs you like on Spotify, albums you save and playlists you make show up here after the
					next sync.
				</p>
			{/if}
		</section>
	{:else if s}
		<!-- Readouts -->
		<section aria-label="Summary" class="mt-6 overflow-hidden rounded-xl border bg-card">
			<dl class="grid divide-y sm:grid-cols-3 sm:divide-x sm:divide-y-0">
				<div class="px-5 py-5 sm:px-6">
					<dt class="text-xs text-muted-foreground">Saved, never played</dt>
					<dd class="mt-1 flex items-baseline gap-2">
						<span class="figure tnum text-2xl sm:text-3xl">{int(s.tracks.never_played)}</span>
						<span class="tnum text-sm text-muted-foreground"
							>{pct(s.tracks.never_played, s.saved_tracks)}% of saved songs</span
						>
					</dd>
				</div>
				<div class="px-5 py-5 sm:px-6">
					<dt class="flex items-center gap-1.5 text-xs text-muted-foreground">
						Saving to first play<Hint
							text="Median days from saving a song to the first play after it, for songs played since."
						/>
					</dt>
					<dd class="mt-1 flex items-baseline gap-2">
						<span class="figure tnum text-2xl sm:text-3xl"
							>{daysFigure(s.days_to_first_play.median)}</span
						>
						<span class="text-sm text-muted-foreground"
							>{s.days_to_first_play.median === 1 ? 'day' : 'days'}, median of {int(
								s.days_to_first_play.count
							)}</span
						>
					</dd>
				</div>
				<div class="px-5 py-5 sm:px-6">
					<dt class="flex items-center gap-1.5 text-xs text-muted-foreground">
						Saving to heavy rotation<Hint
							text="Median days from saving a song to its {s.rotation_plays}th play, for songs that reached it after saving."
						/>
					</dt>
					<dd class="mt-1 flex items-baseline gap-2">
						<span class="figure tnum text-2xl sm:text-3xl"
							>{daysFigure(s.days_to_rotation.median)}</span
						>
						<span class="text-sm text-muted-foreground"
							>{s.days_to_rotation.median === 1 ? 'day' : 'days'}, median of {int(
								s.days_to_rotation.count
							)}</span
						>
					</dd>
				</div>
			</dl>
			<div
				class="grid gap-6 border-t px-5 py-5 sm:px-6 lg:grid-cols-[minmax(0,22rem)_minmax(0,1fr)] lg:gap-10"
			>
				<MixMeter
					label="When saved songs reached heavy rotation"
					hint="Heavy rotation is a song's {s.rotation_plays}th play. Not yet: fewer plays so far."
					parts={mix}
				/>
				<div class="min-w-0">
					<p class="text-xs text-muted-foreground">Songs saved per month</p>
					<div class="mt-2">
						{#if timeline.current}
							<SavesChart points={timeline.current.points} />
						{:else if timeline.error}
							<SectionError what="the saves chart" error={timeline.error} retry={timeline.retry} />
						{:else}
							<Skeleton class="h-32 w-full" />
						{/if}
					</div>
				</div>
			</div>
		</section>

		<!-- Lists -->
		<section aria-label="Saved items" class="mt-10">
			<div class="flex flex-wrap items-center justify-between gap-3">
				<Segmented
					label="Show"
					size="md"
					value={tab}
					onchange={(v) =>
						choose({ tab: v === 'songs' ? null : v, sort: null, played: null, relation: null })}
					options={[
						{ value: 'songs', label: `Songs ${int(s.saved_tracks)}` },
						{ value: 'albums', label: `Albums ${int(s.saved_albums)}` },
						{ value: 'playlists', label: `Playlists ${int(s.playlists)}` }
					]}
				/>
				{#if tab === 'songs'}
					<Segmented
						label="Sort songs"
						value={trackSort}
						onchange={(v) => choose({ sort: v === 'added' ? null : v })}
						options={[
							{ value: 'added', label: 'Recently saved' },
							{ value: 'plays', label: 'Most played' },
							{ value: 'last_played', label: 'Last played' }
						]}
					/>
				{:else if tab === 'albums'}
					<Segmented
						label="Sort albums"
						value={albumSort}
						onchange={(v) => choose({ sort: v === 'added' ? null : v })}
						options={[
							{ value: 'added', label: 'Recently saved' },
							{ value: 'plays', label: 'Most played' },
							{ value: 'played_share', label: 'Most heard' }
						]}
					/>
				{:else}
					<Segmented
						label="Sort playlists"
						value={playlistSort}
						onchange={(v) => choose({ sort: v === 'plays_here' ? null : v })}
						options={[
							{ value: 'plays_here', label: 'Most played' },
							{ value: 'played_share', label: 'Most heard' },
							{ value: 'name', label: 'Name' }
						]}
					/>
				{/if}
			</div>

			{#if tab === 'songs'}
				<div class="mt-4">
					<Segmented
						label="Songs to show"
						value={played}
						onchange={(v) => choose({ played: v })}
						options={[
							{ value: 'never_played', label: `Never played ${int(s.tracks.never_played)}` },
							{
								value: 'maybe_played',
								label: `Other release ${int(s.tracks.maybe_played)}`,
								hint: 'Never played as the saved release, but a song with the same title and artist was played'
							},
							{ value: 'played', label: `Played ${int(s.tracks.played)}` },
							{ value: 'all', label: 'All' }
						]}
					/>
				</div>
				{@const l = tracks.current}
				{#if tracks.error && !l}
					<SectionError class="mt-4" what="saved songs" error={tracks.error} retry={tracks.retry} />
				{:else if l}
					<div class="mt-4 transition-opacity duration-200 {tracks.stale ? 'opacity-55' : ''}">
						{#if l.items.length === 0}
							<p class="rounded-xl border border-dashed px-6 py-8 text-muted-foreground">
								{played === 'never_played'
									? 'Every saved song has been played.'
									: 'No saved songs here.'}
							</p>
						{:else}
							<ol class="overflow-hidden rounded-xl border bg-card">
								{#each l.items as t (t.track.id + (t.added_at ?? ''))}
									<SongRow
										track={t.track}
										status={t.status}
										quiet={played === 'never_played'}
										plays={t.plays}
										addedAt={t.added_at}
										lastPlayedAt={t.last_played_at}
										rotationAt={t.rotation_at}
									/>
								{/each}
							</ol>
							<div class="mt-4">
								<Pager
									page={pageNo}
									pageSize={PAGE_SIZE}
									total={l.total}
									noun="songs"
									href={(n) => hrefWith({ page: n > 1 ? String(n) : null })}
									warm={(n) =>
										prefetch([
											libraryTracksQuery(played, trackSort, PAGE_SIZE, (n - 1) * PAGE_SIZE)
										])}
								/>
							</div>
						{/if}
					</div>
				{:else}
					<div class="mt-4 grid gap-2" aria-busy="true">
						{#each { length: 8 } as _, i (i)}<Skeleton class="h-16 w-full rounded-lg" />{/each}
					</div>
				{/if}
			{:else if tab === 'albums'}
				{@const l = albums.current}
				{#if albums.error && !l}
					<SectionError
						class="mt-4"
						what="saved albums"
						error={albums.error}
						retry={albums.retry}
					/>
				{:else if l}
					<div class="mt-4 transition-opacity duration-200 {albums.stale ? 'opacity-55' : ''}">
						{#if l.items.length === 0}
							<p class="rounded-xl border border-dashed px-6 py-8 text-muted-foreground">
								No saved albums.
							</p>
						{:else}
							<ol class="overflow-hidden rounded-xl border bg-card">
								{#each l.items as a (a.album.id)}
									<li class="relative border-b last:border-b-0 hover:bg-muted/40">
										<div
											class="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-3 px-4 py-3 sm:grid-cols-[auto_minmax(0,1fr)_minmax(0,14rem)_7.5rem] sm:gap-x-5 sm:px-6"
										>
											<Artwork
												src={a.album.image_url}
												name={a.album.name}
												class="size-10 rounded-md"
											/>
											<div class="min-w-0">
												<a
													href={albumHref(a.album.id)}
													class="hit-above block truncate font-medium after:absolute after:inset-0 hover:underline hover:decoration-signal hover:underline-offset-4"
													>{a.album.name}</a
												>
												<p class="narrow truncate text-xs text-muted-foreground">
													{#each a.album.artists as ar, i (ar.id)}{i > 0 ? ', ' : ''}<a
															href={artistHref(ar.id)}
															class="hit-below relative z-10 hover:text-foreground hover:underline"
															>{ar.name}</a
														>{/each}{#if a.added_at}. Saved {dateAt(a.added_at)}{/if}
												</p>
											</div>
											<div
												class="col-span-2 col-start-2 row-start-2 mt-2 grid gap-1.5 sm:col-span-1 sm:col-start-auto sm:row-start-auto sm:mt-0"
											>
												{#if a.tracks_total}
													<ShareDots played={a.tracks_played} total={a.tracks_total} />
													<p class="tnum text-xs text-muted-foreground">
														{int(a.tracks_played)} of {int(a.tracks_total)} tracks played
													</p>
												{:else}
													<p class="tnum text-xs text-muted-foreground">
														{int(a.tracks_played)}
														{a.tracks_played === 1 ? 'track' : 'tracks'} played
													</p>
												{/if}
											</div>
											<p
												class="tnum col-start-3 row-start-1 text-right text-xs sm:col-start-auto sm:row-start-auto"
											>
												<span class="figure text-base">{int(a.plays)}</span>
												<span class="text-muted-foreground">{a.plays === 1 ? 'play' : 'plays'}</span
												>
												<br /><span class="text-muted-foreground">{duration(a.ms_played)}</span>
											</p>
										</div>
									</li>
								{/each}
							</ol>
							<div class="mt-4">
								<Pager
									page={pageNo}
									pageSize={PAGE_SIZE}
									total={l.total}
									noun="albums"
									href={(n) => hrefWith({ page: n > 1 ? String(n) : null })}
								/>
							</div>
						{/if}
					</div>
				{:else}
					<div class="mt-4 grid gap-2" aria-busy="true">
						{#each { length: 6 } as _, i (i)}<Skeleton class="h-16 w-full rounded-lg" />{/each}
					</div>
				{/if}
			{:else}
				<div class="mt-4">
					<Segmented
						label="Playlists to show"
						value={relation}
						onchange={(v) => choose({ relation: v === 'all' ? null : v })}
						options={[
							{ value: 'all', label: 'All' },
							{ value: 'own', label: 'Yours' },
							{ value: 'collaborative', label: 'Collaborative' },
							{ value: 'followed', label: 'Followed' }
						]}
					/>
				</div>
				{@const l = playlists.current}
				{#if playlists.error && !l}
					<SectionError
						class="mt-4"
						what="playlists"
						error={playlists.error}
						retry={playlists.retry}
					/>
				{:else if l}
					<div class="mt-4 transition-opacity duration-200 {playlists.stale ? 'opacity-55' : ''}">
						{#if l.items.length === 0}
							<p class="rounded-xl border border-dashed px-6 py-8 text-muted-foreground">
								No playlists here.
							</p>
						{:else}
							<ol class="overflow-hidden rounded-xl border bg-card">
								{#each l.items as p (p.playlist.id)}
									<li class="relative border-b last:border-b-0 hover:bg-muted/40">
										<div
											class="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-3 px-4 py-3 sm:grid-cols-[auto_minmax(0,1fr)_minmax(0,14rem)_8.5rem] sm:gap-x-5 sm:px-6"
										>
											<Artwork
												src={p.playlist.image_url}
												name={p.playlist.name}
												class="size-10 rounded-md"
											/>
											<div class="min-w-0">
												<a
													href={libraryPlaylistHref(p.playlist.id)}
													class="hit-above block truncate font-medium after:absolute after:inset-0 hover:underline hover:decoration-signal hover:underline-offset-4"
													onpointerenter={() => prefetch([libraryPlaylistQuery(p.playlist.id)])}
													onfocus={() => prefetch([libraryPlaylistQuery(p.playlist.id)])}
													>{p.playlist.name}</a
												>
												<p class="narrow truncate text-xs text-muted-foreground">
													{playlistOwner(p.playlist)}{#if p.items_total != null}, {int(
															p.items_total
														)}
														{p.items_total === 1 ? 'song' : 'songs'}{/if}
												</p>
											</div>
											<div
												class="col-span-2 col-start-2 row-start-2 mt-2 grid gap-1.5 sm:col-span-1 sm:col-start-auto sm:row-start-auto sm:mt-0"
											>
												{#if p.items_available && p.tracks}
													<ShareDots played={p.tracks_played ?? 0} total={p.tracks} />
													<p class="tnum text-xs text-muted-foreground">
														{pct(p.tracks_played ?? 0, p.tracks)}% of its songs played
													</p>
												{/if}
											</div>
											<p
												class="tnum col-start-3 row-start-1 text-right text-xs sm:col-start-auto sm:row-start-auto"
											>
												<span class="figure text-base">{int(p.plays_here)}</span>
												<span class="text-muted-foreground"
													>{p.plays_here === 1 ? 'play' : 'plays'} from it</span
												>
												{#if p.last_played_here_at}<br /><span class="text-muted-foreground"
														>Last {dateAt(p.last_played_here_at)}</span
													>{/if}
											</p>
										</div>
									</li>
								{/each}
							</ol>
							<div class="mt-4">
								<Pager
									page={pageNo}
									pageSize={PAGE_SIZE}
									total={l.total}
									noun="playlists"
									href={(n) => hrefWith({ page: n > 1 ? String(n) : null })}
								/>
							</div>
						{/if}
					</div>
				{:else}
					<div class="mt-4 grid gap-2" aria-busy="true">
						{#each { length: 6 } as _, i (i)}<Skeleton class="h-16 w-full rounded-lg" />{/each}
					</div>
				{/if}
			{/if}
		</section>
	{:else}
		<Skeleton class="mt-6 h-72 w-full rounded-xl" />
	{/if}
</div>
