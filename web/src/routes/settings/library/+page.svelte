<!-- What counts and what things are called: exclusions, and names for Spotify's own playlists (D17). -->
<script lang="ts">
	import SectionError from '#lib/components/common/section-error.svelte';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import Exclusions from '#lib/components/settings/exclusions.svelte';
	import SettingsPanel from '#lib/components/settings/settings-panel.svelte';
	import ContextList from '#lib/components/entity/context-list.svelte';
	import { Resource } from '#lib/data/resource.svelte.js';
	import { invalidate } from '#lib/data/cache.js';
	import { contextsQuery, exclusionsQuery } from '#lib/data/queries.js';

	const exclusions = new Resource(() => exclusionsQuery());
	const contexts = new Resource(() => contextsQuery({ preset: 'all' }, 'plays', {}, 500));

	const spotifyPlaylists = $derived.by(() => {
		const c = contexts.current;
		if (!c) return null;
		const items = c.items.filter((r) => r.context.spotify_owned);
		return { ...c, items, total: items.length, unknown: { plays: 0, ms_played: 0 } };
	});
	const unnamed = $derived(
		spotifyPlaylists?.items.filter((r) => r.context.name_source === 'unresolved').length ?? 0
	);
</script>

<Exclusions data={exclusions.current} />

<SettingsPanel
	id="playlists"
	title="Spotify Playlist Names"
	hint="Spotify doesn't share the names of its own playlists with apps."
>
	{#snippet actions()}
		{#if spotifyPlaylists?.items.length}
			<p class="tnum text-sm {unnamed ? 'font-medium' : 'text-muted-foreground'}">
				{unnamed ? `${unnamed} unnamed` : 'All named'}
			</p>
		{/if}
	{/snippet}
	{#if spotifyPlaylists}
		{#if spotifyPlaylists.items.length}
			<ContextList
				data={spotifyPlaylists}
				metric="plays"
				header={false}
				ranked={false}
				onchange={() => {
					invalidate('/contexts');
					contexts.refresh();
				}}
			/>
		{:else}
			<p class="text-sm text-muted-foreground">None played yet.</p>
		{/if}
	{:else if contexts.error}
		<SectionError what="playlists" error={contexts.error} retry={contexts.retry} />
	{:else}
		<Skeleton class="h-40 w-full" />
	{/if}
</SettingsPanel>
