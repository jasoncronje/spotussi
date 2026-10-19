<!--
	Artists and tracks left out of every stat (plays stay recorded). Adding
	or removing one rebuilds derived data, so every view reloads afterwards.
-->
<script lang="ts">
	import X from '@lucide/svelte/icons/x';
	import Artwork from '#lib/components/common/artwork.svelte';
	import EntitySearch from '#lib/components/common/entity-search.svelte';
	import SettingsPanel from './settings-panel.svelte';
	import { api, type Schemas } from '#lib/api/client.js';
	import { unwrap } from '#lib/data/queries.js';
	import { refreshAll } from '#lib/data/resource.svelte.js';
	import { artistHref, trackHref } from '#lib/state/view.svelte.js';

	let { data }: { data: Schemas['Exclusions'] | undefined } = $props();

	let busy = $state('');
	let error = $state('');
	async function change(kind: 'artists' | 'tracks', id: string, add: boolean) {
		busy = id;
		error = '';
		const path = { kind, id };
		try {
			await unwrap(
				add
					? api.PUT('/exclusions/{kind}/{id}', { params: { path } })
					: api.DELETE('/exclusions/{kind}/{id}', { params: { path } })
			);
			refreshAll();
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy = '';
		}
	}
	const rows = $derived([
		...(data?.artists ?? []).map((a) => ({
			kind: 'artists' as const,
			id: a.id,
			name: a.name,
			sub: 'Artist',
			image: a.image_url,
			href: artistHref(a.id)
		})),
		...(data?.tracks ?? []).map((t) => ({
			kind: 'tracks' as const,
			id: t.id,
			name: t.name,
			sub: `Track, ${t.artists.map((a) => a.name).join(', ')}`,
			image: t.image_url,
			href: trackHref(t.id)
		}))
	]);
</script>

<SettingsPanel
	id="exclusions"
	title="Excluded from Stats"
	hint="Plays stay recorded; they just don't count."
>
	<div class="grid gap-4">
		<EntitySearch
			label="Exclude an artist or track"
			placeholder="Exclude an artist or track"
			kinds={['artist', 'track']}
			class="max-w-md"
			onselect={(p) => change(p.kind === 'artist' ? 'artists' : 'tracks', p.id, true)}
		/>
		{#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
		{#if data && rows.length === 0}
			<p class="text-sm text-muted-foreground">Nothing excluded.</p>
		{:else}
			<ul class="divide-y divide-border/60 rounded-lg border">
				{#each rows as r (r.kind + r.id)}
					<li class="flex items-center gap-3 px-3 py-2 {busy === r.id ? 'opacity-55' : ''}">
						<Artwork src={r.image} name={r.name} round={r.kind === 'artists'} class="size-9" />
						<div class="min-w-0 flex-1">
							<a href={r.href} class="block truncate text-sm font-medium hover:underline"
								>{r.name}</a
							>
							<p class="truncate text-xs text-muted-foreground">{r.sub}</p>
						</div>
						<button
							type="button"
							disabled={busy !== ''}
							onclick={() => change(r.kind, r.id, false)}
							class="inline-flex h-8 items-center gap-1 rounded-md px-2 text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
						>
							<X class="size-3.5" aria-hidden="true" />Include again<span class="sr-only"
								>: {r.name}</span
							>
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</SettingsPanel>
