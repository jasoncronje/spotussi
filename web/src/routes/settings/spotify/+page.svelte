<script lang="ts">
	import SectionError from '#lib/components/common/section-error.svelte';
	import Connection from '#lib/components/settings/connection.svelte';
	import { Resource } from '#lib/data/resource.svelte.js';
	import { invalidate } from '#lib/data/cache.js';
	import LibrarySync from '#lib/components/settings/library-sync.svelte';
	import { connectionQuery, statusQuery } from '#lib/data/queries.js';

	const connection = new Resource(() => connectionQuery());
	const status = new Resource(() => statusQuery());
</script>

{#if connection.error && !connection.current}
	<SectionError what="the Spotify connection" error={connection.error} retry={connection.retry} />
{:else}
	<Connection
		data={connection.current}
		onchange={() => {
			invalidate('/system/status');
			connection.refresh();
		}}
	/>
{/if}

<LibrarySync
	data={status.current?.library}
	onchange={() => {
		invalidate('/system/status');
		status.refresh();
	}}
/>
