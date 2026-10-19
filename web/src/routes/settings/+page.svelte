<!--
	/settings opens General. Links from before the tabs (/settings#import)
	still land on the right tab: the hash only exists in the browser, so this
	redirects on mount rather than in a load function.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { LEGACY_ANCHORS, settingsHref } from '#lib/components/settings/tabs.js';

	onMount(() => {
		const tab = LEGACY_ANCHORS[location.hash.slice(1)] ?? 'general';
		// eslint-disable-next-line svelte/no-navigation-without-resolve
		goto(settingsHref(tab), { replaceState: true });
	});
</script>
