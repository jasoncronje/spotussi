<!--
	One playlist (D36): its songs in playlist order, who added each, its
	plays anywhere (rotation lamps) and the plays started from this
	playlist. Spotify shares the songs of the owner's own and collaborative
	playlists only; a followed playlist shows its plays alone.
-->
<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import ChevronLeft from '@lucide/svelte/icons/chevron-left';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import SectionError from '#lib/components/common/section-error.svelte';
	import Pager from '#lib/components/common/pager.svelte';
	import Artwork from '#lib/components/common/artwork.svelte';
	import SongRow from '#lib/components/library/song-row.svelte';
	import ShareDots from '#lib/components/library/share-dots.svelte';
	import { Resource } from '#lib/data/resource.svelte.js';
	import { libraryPlaylistQuery } from '#lib/data/queries.js';
	import { hrefWith } from '#lib/state/view.svelte.js';
	import { dateAt, duration, int } from '#lib/format.js';
	import { ApiError } from '#lib/data/cache.js';

	const PAGE_SIZE = 100;
	const id = $derived(page.params.id ?? '');
	const pageNo = $derived(Math.max(1, Number(page.url.searchParams.get('page')) || 1));
	const detail = new Resource(() => libraryPlaylistQuery(id, PAGE_SIZE, (pageNo - 1) * PAGE_SIZE));
	const d = $derived(detail.current);
	const p = $derived(d?.playlist);
	const missing = $derived(detail.error instanceof ApiError && detail.error.status === 404);
	const collaborative = $derived(p?.playlist.relation === 'collaborative');
	const owner = $derived.by(() => {
		if (!p) return '';
		const r = p.playlist;
		if (r.relation === 'own') return 'Your playlist';
		if (r.relation === 'collaborative')
			return `Collaborative playlist by ${r.owner_name ?? 'someone else'}`;
		return `Followed playlist by ${r.owner_name ?? 'someone else'}`;
	});
</script>

<svelte:head><title>{p?.playlist.name ?? 'Playlist'} – spotussi</title></svelte:head>

<div class="mx-auto max-w-[90rem] px-4 pt-6 pb-20 sm:px-6 lg:px-10 lg:pt-10">
	<a
		href={resolve('/library') + '?tab=playlists'}
		class="hit -ml-1 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
		><ChevronLeft class="size-4" aria-hidden="true" />Playlists</a
	>

	{#if missing}
		<section class="mt-6 rounded-xl border border-dashed bg-card/50 px-6 py-10 sm:px-10">
			<h2 class="text-lg font-semibold">This playlist isn't in your library</h2>
			<p class="mt-1 text-muted-foreground">
				It may have been deleted or unfollowed since the last sync.
			</p>
		</section>
	{:else if detail.error && !d}
		<SectionError class="mt-6" what="this playlist" error={detail.error} retry={detail.retry} />
	{:else if d && p}
		<header class="mt-4 grid grid-cols-[auto_minmax(0,1fr)] items-center gap-5 sm:gap-6">
			<Artwork
				src={p.playlist.image_url}
				name={p.playlist.name}
				class="size-20 rounded-lg sm:size-28"
				eager
			/>
			<div class="min-w-0">
				<p class="text-sm text-muted-foreground">{owner}</p>
				<h1 class="display mt-1 text-2xl text-balance break-words sm:text-3xl">
					{p.playlist.name}
				</h1>
			</div>
		</header>

		<section aria-label="Summary" class="mt-6 overflow-hidden rounded-xl border bg-card">
			<dl
				class="grid grid-cols-2 divide-border sm:grid-cols-4 sm:divide-x max-sm:[&>div:nth-child(n+3)]:border-t"
			>
				<div class="px-5 py-4 sm:px-6">
					<dt class="text-xs text-muted-foreground">Songs</dt>
					<dd class="figure tnum mt-1 text-xl">
						{p.items_total != null ? int(p.items_total) : '–'}
					</dd>
				</div>
				<div class="border-l px-5 py-4 sm:border-l-0 sm:px-6">
					<dt class="text-xs text-muted-foreground">Ever played</dt>
					<dd class="mt-1">
						{#if p.items_available && p.tracks}
							<span class="figure tnum text-xl">{int(p.tracks_played ?? 0)}</span>
							<span class="tnum text-sm text-muted-foreground">of {int(p.tracks)}</span>
							<div class="mt-2">
								<ShareDots played={p.tracks_played ?? 0} total={p.tracks} max={14} />
							</div>
						{:else}
							<span class="figure text-xl">–</span>
						{/if}
					</dd>
				</div>
				<div class="px-5 py-4 sm:px-6">
					<dt class="text-xs text-muted-foreground">Plays from it</dt>
					<dd class="mt-1">
						<span class="figure tnum text-xl">{int(p.plays_here)}</span>
						<span class="tnum text-sm text-muted-foreground">{duration(p.ms_here)}</span>
					</dd>
				</div>
				<div class="border-l px-5 py-4 sm:border-l-0 sm:px-6">
					<dt class="text-xs text-muted-foreground">Last played from it</dt>
					<dd class="tnum mt-1 text-base font-medium">
						{p.last_played_here_at ? dateAt(p.last_played_here_at) : 'Never'}
					</dd>
				</div>
			</dl>
		</section>

		{#if !p.items_available}
			<p class="mt-6 max-w-[60ch] text-muted-foreground">
				Spotify only shares the songs of playlists you made or collaborate on, so this one shows its
				plays alone.
			</p>
		{:else if d.items.length === 0}
			<p class="mt-6 rounded-xl border border-dashed px-6 py-8 text-muted-foreground">
				This playlist is empty.
			</p>
		{:else}
			<ol
				class="mt-6 overflow-hidden rounded-xl border bg-card"
				aria-label="Songs in playlist order"
			>
				{#each d.items as it (it.position)}
					<SongRow
						track={it.track}
						status={it.status}
						plays={it.plays}
						addedAt={it.added_at}
						addedLabel={collaborative && it.added_by
							? it.added_by_owner
								? 'You added'
								: `${it.added_by} added`
							: 'Added'}
					>
						{#snippet lead()}
							<span class="figure tnum w-7 text-right text-sm text-foreground/60"
								>{it.position + 1}</span
							>
						{/snippet}
						{#snippet trail()}
							<p class="tnum text-xs text-muted-foreground">{int(it.plays_here)} from here</p>
						{/snippet}
					</SongRow>
				{/each}
			</ol>
			<div class="mt-4">
				<Pager
					page={pageNo}
					pageSize={PAGE_SIZE}
					total={d.total}
					noun="songs"
					href={(n) => hrefWith({ page: n > 1 ? String(n) : null })}
				/>
			</div>
		{/if}
	{:else}
		<Skeleton class="mt-6 h-28 w-full rounded-xl" />
		<div class="mt-6 grid gap-2" aria-busy="true">
			{#each { length: 8 } as _, i (i)}<Skeleton class="h-16 w-full rounded-lg" />{/each}
		</div>
	{/if}
</div>
