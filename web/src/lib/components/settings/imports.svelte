<!--
	Importing past listening (D10, D23): the upload control, then every
	recent import, uploads and `spotussi import` runs alike. The list follows
	the shared upload and polls while a job someone else started (the CLI)
	is still running.
-->
<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import SettingsPanel from './settings-panel.svelte';
	import ImportUpload from '#lib/components/import/import-upload.svelte';
	import ExportRequest from '#lib/components/import/export-request.svelte';
	import ImportJobs from '#lib/components/import/import-jobs.svelte';
	import SectionError from '#lib/components/common/section-error.svelte';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import { Resource } from '#lib/data/resource.svelte.js';
	import { importsQuery } from '#lib/data/queries.js';
	import { settled } from '#lib/data/imports.js';
	import { importUpload } from '#lib/state/import-upload.svelte.js';

	const list = new Resource(() => importsQuery());

	// The upload created or finished a job: show it.
	$effect(() => {
		if (importUpload.changes) untrack(() => list.refresh());
	});

	// Follow jobs this browser isn't polling itself (CLI runs, an upload from
	// before a reload).
	onMount(() => {
		let t: ReturnType<typeof setTimeout>;
		const loop = () => {
			const active = list.current?.items.some((j) => !settled(j));
			t = setTimeout(() => {
				if (active && document.visibilityState === 'visible') list.refresh();
				loop();
			}, 2000);
		};
		loop();
		return () => clearTimeout(t);
	});
</script>

<SettingsPanel
	id="import"
	title="Import Streaming History"
	hint="Re-importing the same export adds only what's new."
>
	<div class="grid gap-7">
		<div class="grid gap-3">
			<ImportUpload />
			<ExportRequest />
		</div>

		<div class="grid gap-2">
			<h3 class="text-sm font-medium">Recent Imports</h3>
			{#if list.current}
				{#if list.current.items.length}
					<ImportJobs jobs={list.current.items} />
				{:else}
					<p class="text-sm text-muted-foreground">Nothing imported yet.</p>
				{/if}
			{:else if list.error}
				<SectionError what="imports" error={list.error} retry={list.retry} />
			{:else}
				<Skeleton class="h-24 w-full rounded-lg" />
			{/if}
		</div>
	</div>
</SettingsPanel>
