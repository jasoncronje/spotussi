<!--
	Where an artist's, album's or track's details come from (D27), shown only
	when it isn't Spotify: provisional details from the fallback sources, or
	names only while lookups are pending. Explains why artwork, a length or
	genres may be missing, so the gap reads as "not yet", not as broken.
-->
<script lang="ts">
	import type { Schemas } from '#lib/api/client.js';
	import Hint from './hint.svelte';

	let { meta }: { meta: Schemas['EntityMetadata'] } = $props();

	const SOURCE: Record<Schemas['EntityMetadata']['sources'][number], string> = {
		oembed: 'Spotify’s public embed data',
		deezer: 'a public music catalog',
		musicbrainz: 'an open music database'
	};
	const from = $derived.by(() => {
		const names = meta.sources.map((s) => SOURCE[s]);
		if (names.length < 2) return names[0] ?? 'public music catalogs';
		return `${names.slice(0, -1).join(', ')} and ${names.at(-1)}`;
	});
</script>

{#if meta.state !== 'spotify'}
	<span class="inline-flex items-center gap-1.5 rounded-md bg-muted px-2 py-0.5 text-xs">
		<span
			class="size-[5px] rounded-full {meta.state === 'fallback' ? 'bg-signal-dim' : 'bg-grille'}"
			aria-hidden="true"
		></span>
		{meta.state === 'fallback' ? 'Provisional details' : 'Names only'}
		<Hint
			text={meta.state === 'fallback'
				? `Filled in from ${from} while Spotify lookups wait. Replaced once Spotify answers.`
				: 'Only the name from your history is known so far. Artwork, length and genres are looked up in the background.'}
		/>
	</span>
{/if}
