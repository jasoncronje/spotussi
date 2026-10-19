<!--
	"Catalog still loading" (D9, D27): a fresh import knows tracks by name
	only, and Spotify lookups can take days on a large history (daily quota).
	Until most listening has details, data pages say so once, in one line:
	how far along it is, why it is slow when it is, and that provisional
	details fill in meanwhile. Hideable; Settings, System keeps the detail.
-->
<script lang="ts">
	import type { Schemas } from '#lib/api/client.js';
	import { settingsHref } from '#lib/components/settings/tabs.js';
	import { ago, clockTime, int } from '#lib/format.js';
	import X from '@lucide/svelte/icons/x';

	let { catalog }: { catalog: Schemas['SystemStatus']['catalog'] } = $props();

	const KEY = 'spotussi:catalog-strip-hidden';
	let hidden = $state(read());
	function read() {
		try {
			return localStorage.getItem(KEY) === '1';
		} catch {
			return false;
		}
	}
	function hide() {
		hidden = true;
		try {
			localStorage.setItem(KEY, '1');
		} catch {
			// private window: hidden for this visit only
		}
	}

	const total = $derived(catalog.tracks_total);
	const spotify = $derived(catalog.tracks_hydrated);
	/** Tracks the fallback sources matched (a track can match several; take the best). */
	const provisional = $derived(
		Math.min(
			Math.max(0, total - spotify),
			Math.max(
				0,
				...catalog.fallbacks.filter((f) => f.source !== 'musicbrainz').map((f) => f.matched)
			)
		)
	);
	const share = $derived(total ? spotify / total : 1);
	const active = $derived(
		['hydrating', 'waiting', 'backing_off', 'quota_exceeded'].includes(catalog.state)
	);
	const show = $derived(!hidden && active && total > 0 && share < 0.97);

	const DOTS = 24;
	const lit = $derived(Math.round(share * DOTS));
	const dim = $derived(Math.min(DOTS - lit, Math.round((provisional / (total || 1)) * DOTS)));

	const why = $derived.by(() => {
		switch (catalog.state) {
			case 'quota_exceeded':
				return catalog.resumes_at
					? `Spotify’s daily lookup limit is reached; lookups resume ${ago(catalog.resumes_at).replace(/\.$/, '')}.`
					: 'Spotify’s daily lookup limit is reached.';
			case 'backing_off':
				return 'Lookups are retrying after errors.';
			case 'waiting':
				return 'Lookups wait for the Spotify connection.';
			default:
				return catalog.eta_at
					? `Artwork, lengths and genres fill in as it runs, done around ${clockTime(catalog.eta_at)}.`
					: 'Artwork, lengths and genres fill in as it runs.';
		}
	});
</script>

{#if show}
	<aside aria-label="Catalog details" class="border-b bg-card/60 text-sm">
		<div
			class="mx-auto flex max-w-[90rem] items-start gap-3 px-4 py-2 sm:items-center sm:px-6 lg:px-10"
		>
			<span
				class="mt-1.5 hidden shrink-0 gap-[3px] sm:mt-0 sm:inline-flex"
				aria-hidden="true"
				title="{int(spotify)} from Spotify, {int(provisional)} provisional, of {int(total)}"
			>
				{#each { length: DOTS } as _, i (i)}
					<span
						class="size-[5px] rounded-full {i < lit
							? 'bg-signal'
							: i < lit + dim
								? 'bg-signal-dim'
								: 'bg-grille'}"
					></span>
				{/each}
			</span>
			<p class="min-w-0 flex-1 text-muted-foreground">
				<span class="tnum font-medium text-foreground"
					>Track details {Math.floor(share * 100)}% looked up.</span
				>
				{why}{provisional > 0 ? ` Provisional details cover ${int(provisional)} more.` : ''}
				<a href={settingsHref('system')} class="inline-link hit whitespace-nowrap">Details</a>
			</p>
			<button
				type="button"
				onclick={hide}
				aria-label="Hide catalog progress"
				title="Hide"
				class="-m-1 grid size-7 shrink-0 place-items-center rounded-md text-faint hover:bg-muted hover:text-foreground"
			>
				<X class="size-4" aria-hidden="true" />
			</button>
		</div>
	</aside>
{/if}
