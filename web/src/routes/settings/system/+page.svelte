<!-- What the server is doing, polled. Sign-out lives in the header menu. -->
<script lang="ts">
	import { onMount } from 'svelte';
	import SectionError from '#lib/components/common/section-error.svelte';
	import System from '#lib/components/settings/system.svelte';
	import { Resource } from '#lib/data/resource.svelte.js';
	import { statusQuery } from '#lib/data/queries.js';

	const status = new Resource(() => statusQuery());

	// Poll quickly while something is moving, slowly otherwise.
	onMount(() => {
		let t: ReturnType<typeof setTimeout>;
		const tick = () => {
			const s = status.current;
			const busy =
				s?.derive.state === 'rebuilding' ||
				s?.imports.some((j) => j.state === 'running' || j.state === 'queued') ||
				(s?.catalog.queue_length ?? 0) > 0;
			t = setTimeout(
				() => {
					if (document.visibilityState === 'visible') status.refresh();
					tick();
				},
				busy ? 4000 : 30000
			);
		};
		tick();
		return () => clearTimeout(t);
	});
</script>

{#if status.error && !status.current}
	<SectionError what="system status" error={status.error} retry={status.retry} />
{:else}
	<System data={status.current} />
{/if}
