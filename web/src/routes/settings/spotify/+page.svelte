<script lang="ts">
	import SectionError from '#lib/components/common/section-error.svelte';
	import Connection from '#lib/components/settings/connection.svelte';
	import { Resource } from '#lib/data/resource.svelte.js';
	import { invalidate } from '#lib/data/cache.js';
	import { connectionQuery } from '#lib/data/queries.js';

	const connection = new Resource(() => connectionQuery());
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
