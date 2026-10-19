<script lang="ts">
	import SectionError from '#lib/components/common/section-error.svelte';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import Preferences from '#lib/components/settings/preferences.svelte';
	import { Resource } from '#lib/data/resource.svelte.js';
	import { settingsQuery } from '#lib/data/queries.js';

	const settings = new Resource(() => settingsQuery());
</script>

{#if settings.error && !settings.current}
	<SectionError what="settings" error={settings.error} retry={settings.retry} />
{:else if settings.current}
	<Preferences settings={settings.current} onchange={() => settings.refresh()} />
{:else}
	<Skeleton class="h-[34rem] w-full rounded-xl" />
{/if}
