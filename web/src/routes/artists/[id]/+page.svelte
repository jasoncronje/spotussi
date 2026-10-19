<script lang="ts">
	import { page } from '$app/state';
	import EntityView from '#lib/components/entity/entity-view.svelte';
</script>

{#key page.params.id}
	<EntityView kind="artist" id={page.params.id!} />
{/key}
