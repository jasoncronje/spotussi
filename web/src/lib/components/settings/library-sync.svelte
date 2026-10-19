<!--
	Library sync (D36): what was last read from Spotify, when the next
	check runs, and a button to check now. Each list says if it couldn't be
	read (a missing permission, an error).
-->
<script lang="ts">
	import { resolve } from '$app/paths';
	import SettingsPanel from './settings-panel.svelte';
	import Lamp from '#lib/components/shell/lamp.svelte';
	import type { Schemas } from '#lib/api/client.js';
	import { syncLibrary } from '#lib/data/queries.js';
	import { invalidate } from '#lib/data/cache.js';
	import { ago, fullDateTime, int } from '#lib/format.js';

	let { data, onchange }: { data: Schemas['LibraryStatus'] | undefined; onchange: () => void } =
		$props();

	let pending = $state(false);
	let message = $state('');

	async function syncNow() {
		pending = true;
		message = '';
		try {
			await syncLibrary();
			message = 'Sync started.';
			invalidate('/library');
		} catch (e) {
			message = e instanceof Error ? e.message : 'Could not start a sync.';
		} finally {
			pending = false;
			onchange();
		}
	}

	const STATE: Record<Schemas['LibraryStatus']['state'], string> = {
		disabled: 'Off',
		not_connected: 'Waiting for Spotify',
		idle: 'Up to date',
		syncing: 'Syncing',
		backing_off: 'Retrying after an error',
		quota_exceeded: 'Paused, Spotify limit reached'
	};
	const LIST: Record<Schemas['LibraryListStatus']['state'], string> = {
		ok: 'Read',
		pending: 'Not read yet',
		needs_scope: 'Needs permission',
		error: 'Failed'
	};
	const lists = $derived(
		data
			? ([
					['Saved songs', data.lists.tracks, data.saved_tracks],
					['Saved albums', data.lists.albums, data.saved_albums],
					['Playlists', data.lists.playlists, data.playlists]
				] as const)
			: []
	);
	const btn = 'inline-flex h-9 items-center rounded-lg px-3.5 text-sm font-medium';
	const canSync = $derived(data && data.state !== 'disabled' && data.state !== 'not_connected');
</script>

<SettingsPanel
	id="library-sync"
	title="Library Sync"
	hint="Your saved songs, saved albums and playlists are read from Spotify about every hour, and in full once a day. It doesn't use the catalog lookup quota."
>
	{#snippet actions()}
		{#if data}
			<p
				class="flex items-center gap-2 text-sm {data.state === 'idle'
					? 'text-muted-foreground'
					: 'font-medium'}"
			>
				<Lamp on={data.state === 'idle' || data.state === 'syncing'} />{STATE[data.state]}
			</p>
		{/if}
	{/snippet}
	{#if data}
		{#if data.state === 'disabled'}
			<p class="text-sm text-muted-foreground">
				Set <code class="rounded bg-muted px-1 py-0.5 text-xs text-foreground"
					>LIBRARY_SYNC_ENABLED=true</code
				> and restart to read your library from Spotify.
			</p>
		{:else}
			<dl class="grid gap-x-8 gap-y-2 text-sm sm:grid-cols-[auto_1fr]">
				<dt class="text-muted-foreground">Last sync</dt>
				<dd class="tnum" title={data.last_sync_at ? fullDateTime(data.last_sync_at) : undefined}>
					{data.last_sync_at ? ago(data.last_sync_at) : 'Not yet'}
				</dd>
				{#if data.state === 'quota_exceeded' && data.resumes_at}
					<dt class="text-muted-foreground">Resumes</dt>
					<dd class="tnum">{fullDateTime(data.resumes_at)}</dd>
				{:else if data.next_sync_at && data.state !== 'syncing'}
					<dt class="text-muted-foreground">Next check</dt>
					<dd class="tnum">{ago(data.next_sync_at)}</dd>
				{/if}
				{#each lists as [label, l, n] (label)}
					<dt class="text-muted-foreground">{label}</dt>
					<dd class="tnum">
						{#if l.state === 'ok'}{int(n)}{:else}<span class="font-medium">{LIST[l.state]}</span
							>{#if l.last_error}<span class="text-muted-foreground">: {l.last_error}</span
								>{/if}{/if}
					</dd>
				{/each}
				{#if data.last_error && data.state === 'backing_off'}
					<dt class="text-muted-foreground">Last error</dt>
					<dd class="text-destructive">{data.last_error}</dd>
				{/if}
			</dl>
			{#if data.lists.tracks.state === 'needs_scope'}
				<p class="mt-4 max-w-[60ch] text-sm text-muted-foreground">
					Reading saved songs and albums needs the library permission. Reconnect Spotify above to
					grant it.
				</p>
			{/if}
			<div class="mt-5 flex flex-wrap items-center gap-3">
				<button
					type="button"
					onclick={syncNow}
					disabled={!canSync || pending || data.state === 'syncing'}
					class="{btn} border bg-card hover:bg-muted disabled:opacity-50 disabled:hover:bg-card"
					>{data.state === 'syncing' ? 'Syncing…' : 'Sync now'}</button
				>
				<a
					href={resolve('/library')}
					class="hit text-sm text-muted-foreground underline decoration-faint underline-offset-2 hover:text-foreground"
					>Open your library</a
				>
				<p class="text-sm text-muted-foreground" role="status">{message}</p>
			</div>
		{/if}
	{:else}
		<p class="text-sm text-muted-foreground">Loading…</p>
	{/if}
</SettingsPanel>
