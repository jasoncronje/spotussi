<!--
	For someone who has no export yet: what to tick on Spotify's privacy
	page and how long it takes, with one link straight there. Stays in
	Settings; the setup guide has its own numbered steps.
-->
<script lang="ts">
	import ExternalLink from '@lucide/svelte/icons/external-link';
	import MailCheck from '@lucide/svelte/icons/mail-check';
</script>

<div
	class="flex flex-wrap items-center gap-x-4 gap-y-3 rounded-xl border border-dashed px-4 py-3.5 sm:flex-nowrap"
>
	<span
		class="grid size-9 shrink-0 place-items-center rounded-lg bg-muted text-muted-foreground"
		aria-hidden="true"><MailCheck class="size-4" /></span
	>
	<div class="min-w-0 flex-1 text-sm">
		<p class="font-medium">No Export Yet?</p>
		<p class="text-muted-foreground">
			Tick <span class="text-foreground">Extended streaming history</span>. Spotify emails it within
			30 days.
		</p>
	</div>
	<a
		href="https://www.spotify.com/account/privacy/"
		target="_blank"
		rel="noopener noreferrer"
		class="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg border bg-card px-3.5 text-sm font-medium transition-colors hover:bg-muted max-sm:w-full max-sm:justify-center"
		>Request on Spotify<ExternalLink class="size-3.5" aria-hidden="true" /><span class="sr-only">
			(opens in a new tab)</span
		></a
	>
</div>
