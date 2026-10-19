<!--
	Where the listening came from: playlists, albums, artist pages, Liked
	Songs. Spotify's own playlists (Discover Weekly, Daily Mix, ...) cannot be
	read by apps (D17), so they arrive unnamed; each gets a one-time "Name
	it" action that stores a manual name, which can later be changed or
	cleared.
-->
<script lang="ts">
	import Pencil from '@lucide/svelte/icons/pencil';
	import ListMusic from '@lucide/svelte/icons/list-music';
	import Disc3 from '@lucide/svelte/icons/disc-3';
	import Heart from '@lucide/svelte/icons/heart';
	import MicVocal from '@lucide/svelte/icons/mic-vocal';
	import CircleDashed from '@lucide/svelte/icons/circle-dashed';
	import Artwork from '#lib/components/common/artwork.svelte';
	import { api, type Schemas } from '#lib/api/client.js';
	import { unwrap } from '#lib/data/queries.js';
	import { int, duration } from '#lib/format.js';
	import type { Metric } from '#lib/ranges.js';

	let {
		data,
		metric,
		title = 'Played from',
		header = true,
		ranked = true,
		onchange
	}: {
		data: Schemas['Contexts'];
		metric: Metric;
		title?: string;
		/** Show the title row (off inside a panel that has its own). */
		header?: boolean;
		/** Show each source's rank (off for a filtered list). */
		ranked?: boolean;
		/** Called after a playlist was named, so the owner can refresh. */
		onchange?: () => void;
	} = $props();

	const uid = $props.id();
	const v = (m: { plays: number; ms_played: number }) =>
		metric === 'plays' ? m.plays : m.ms_played;
	const fmt = (m: { plays: number; ms_played: number }) =>
		metric === 'plays' ? int(m.plays) : duration(m.ms_played);
	const top = $derived(Math.max(1, ...data.items.map(v)));

	const TYPE: Record<Schemas['ContextRef']['type'], string> = {
		playlist: 'Playlist',
		album: 'Album',
		artist: 'Artist page',
		collection: 'Liked Songs',
		other: 'Other'
	};
	const label = (c: Schemas['ContextRef']) =>
		c.name ??
		(c.type === 'collection'
			? 'Liked Songs'
			: c.spotify_owned
				? 'Unnamed Spotify playlist'
				: 'Unknown');

	// ---------- naming (D17) ----------
	let dialog = $state<HTMLDialogElement>();
	let editing = $state<Schemas['ContextRef'] | null>(null);
	let name = $state('');
	let saving = $state(false);
	let error = $state('');

	function open(c: Schemas['ContextRef']) {
		editing = c;
		name = c.name_source === 'manual' ? (c.name ?? '') : '';
		error = '';
		dialog?.showModal();
	}
	async function save(clear = false) {
		if (!editing) return;
		const value = clear ? null : name.trim();
		if (!clear && !value) {
			error = 'Enter a name, or close to leave it unnamed.';
			return;
		}
		saving = true;
		error = '';
		try {
			await unwrap(
				api.PUT('/playlists/{id}/name', {
					params: { path: { id: editing.id } },
					body: { name: value }
				})
			);
			dialog?.close();
			onchange?.();
		} catch (e) {
			error = (e as Error).message;
		} finally {
			saving = false;
		}
	}
</script>

<section
	aria-labelledby={header ? `${uid}-h` : undefined}
	aria-label={header ? undefined : title}
	class="min-w-0"
>
	{#if header}
		<header class="mb-2 flex items-baseline justify-between gap-3 border-b pb-2.5">
			<h2 id="{uid}-h" class="text-base font-semibold">{title}</h2>
			<span class="tnum text-xs text-muted-foreground">{int(data.total)} sources</span>
		</header>
	{/if}
	{#if data.items.length === 0 && v(data.unknown) === 0}
		<p class="py-6 text-sm text-muted-foreground">Nothing in this range.</p>
	{:else}
		<ol>
			{#each data.items as r (r.context.uri)}
				{@const c = r.context}
				{@const unnamed = c.spotify_owned && c.name_source === 'unresolved'}
				<li class="grid grid-cols-[1.25rem_auto_1fr_auto] items-center gap-x-3 py-2 pr-1">
					<span class="tnum text-right text-xs text-faint" aria-hidden="true"
						>{ranked ? r.rank : ''}</span
					>
					{#if c.image_url}
						<Artwork
							src={c.image_url}
							name={label(c)}
							round={c.type === 'artist'}
							class="size-10"
						/>
					{:else}
						<span
							class="grid size-10 place-items-center rounded-[4px] bg-muted text-muted-foreground"
							aria-hidden="true"
						>
							{#if c.type === 'collection'}<Heart class="size-4" />
							{:else if c.type === 'album'}<Disc3 class="size-4" />
							{:else if c.type === 'artist'}<MicVocal class="size-4" />
							{:else}<ListMusic class="size-4" />{/if}
						</span>
					{/if}
					<div class="min-w-0">
						<p class="flex min-w-0 items-center gap-2">
							<span class="truncate font-medium {unnamed ? 'text-muted-foreground italic' : ''}"
								>{label(c)}</span
							>
							{#if c.spotify_owned}
								<button
									type="button"
									onclick={() => open(c)}
									class="inline-flex h-6 shrink-0 items-center gap-1 rounded-md px-1.5 text-xs transition-colors hover:bg-muted {unnamed
										? 'font-medium text-signal-ink'
										: 'text-muted-foreground'}"
								>
									<Pencil class="size-3" aria-hidden="true" />
									{unnamed ? 'Name it' : 'Rename'}<span class="sr-only">: {label(c)}</span>
								</button>
							{/if}
						</p>
						{#if c.type !== 'collection'}
							<p class="narrow truncate text-xs text-muted-foreground">
								{TYPE[c.type]}{#if c.name_source === 'manual'}, named by you{/if}
							</p>
						{/if}
						<div class="mt-1.5 h-[3px] w-full rounded-full bg-grille/70" aria-hidden="true">
							<div
								class="h-full rounded-full bg-signal transition-[width] duration-300"
								style="width: {Math.max(2, (v(r) / top) * 100)}%"
							></div>
						</div>
					</div>
					<span class="tnum pl-1 text-sm font-medium whitespace-nowrap">{fmt(r)}</span>
				</li>
			{/each}
			{#if v(data.unknown) > 0}
				<li class="grid grid-cols-[1.25rem_auto_1fr_auto] items-center gap-x-3 py-2 pr-1">
					<span></span>
					<span
						class="grid size-10 place-items-center rounded-[4px] bg-muted text-muted-foreground"
						aria-hidden="true"><CircleDashed class="size-4" /></span
					>
					<div class="min-w-0">
						<p class="truncate font-medium text-muted-foreground">Source not recorded</p>
						<p class="mt-0.5 text-xs text-muted-foreground">Not in imported history</p>
					</div>
					<span class="tnum pl-1 text-sm whitespace-nowrap text-muted-foreground"
						>{fmt(data.unknown)}</span
					>
				</li>
			{/if}
		</ol>
	{/if}
</section>

<dialog
	bind:this={dialog}
	aria-labelledby="{uid}-dh"
	class="m-auto w-[min(28rem,calc(100vw-2rem))] rounded-xl border bg-popover p-0 text-popover-foreground shadow-[0_24px_64px_-16px_rgb(0_0_0/0.45)] backdrop:bg-black/50"
	onclose={() => (editing = null)}
>
	<form
		method="dialog"
		class="grid gap-4 p-6"
		onsubmit={(e) => {
			e.preventDefault();
			save();
		}}
	>
		<div>
			<h2 id="{uid}-dh" class="text-lg font-semibold">Name this playlist</h2>
			{#if editing}
				<p class="mt-3 truncate font-mono text-xs text-muted-foreground" title={editing.uri}>
					{editing.uri}
				</p>
			{/if}
		</div>
		<label class="grid gap-1.5 text-sm font-medium">
			Playlist name
			<input
				bind:value={name}
				maxlength="200"
				autocomplete="off"
				placeholder="Discover Weekly"
				class="h-10 rounded-lg border bg-background px-3 text-base font-normal outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/40 sm:text-sm"
			/>
		</label>
		{#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
		<div class="flex flex-wrap items-center justify-end gap-2">
			{#if editing?.name_source === 'manual'}
				<button
					type="button"
					disabled={saving}
					onclick={() => save(true)}
					class="mr-auto h-9 rounded-lg px-3 text-sm text-muted-foreground hover:bg-muted hover:text-foreground"
					>Clear name</button
				>
			{/if}
			<button
				type="button"
				onclick={() => dialog?.close()}
				class="h-9 rounded-lg border bg-card px-3.5 text-sm font-medium hover:bg-muted"
				>Cancel</button
			>
			<button
				type="submit"
				disabled={saving}
				class="h-9 rounded-lg bg-primary px-3.5 text-sm font-medium text-primary-foreground disabled:opacity-60"
				>{saving ? 'Saving…' : 'Save name'}</button
			>
		</div>
	</form>
</dialog>
