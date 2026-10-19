<script lang="ts">
	import SectionError from '#lib/components/common/section-error.svelte';
	import Endpoints from '#lib/components/settings/endpoints.svelte';
	import Tokens from '#lib/components/settings/tokens.svelte';
	import { Resource } from '#lib/data/resource.svelte.js';
	import { tokensQuery } from '#lib/data/queries.js';

	const tokens = new Resource(() => tokensQuery());
</script>

{#if tokens.error && !tokens.current}
	<SectionError what="API tokens" error={tokens.error} retry={tokens.retry} />
{:else}
	<Tokens tokens={tokens.current?.items} onchange={() => tokens.refresh()} />
{/if}

<Endpoints />
