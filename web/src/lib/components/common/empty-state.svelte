<!--
	An empty view, said in terms of why it's empty (M7):
	- no history at all (fresh instance): how to get some, by connecting
	  Spotify (new plays) and importing the export (the past);
	- Spotify connected but nothing played yet: listening shows up here;
	- history exists, just not in this range: one click to a longer one.
	`children` adds view-specific actions (clear a filter).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { resolve } from '$app/paths';
	import { Resource } from '#lib/data/resource.svelte.js';
	import { statusQuery } from '#lib/data/queries.js';
	import { settingsHref } from '#lib/components/settings/tabs.js';
	import { setRange, view } from '#lib/state/view.svelte.js';
	import { PRESET_LABEL } from '#lib/ranges.js';
	import { dateSpan } from '#lib/format.js';

	let {
		what,
		hint,
		filtered = false,
		children,
		class: className = ''
	}: {
		/** "Nothing ranked", "No sessions": completed with the range. */
		what: string;
		hint?: string;
		/** A filter narrows the view: the range alone may not be why. */
		filtered?: boolean;
		children?: Snippet;
		class?: string;
	} = $props();

	const status = new Resource(() => statusQuery());
	const s = $derived(status.current);
	const fresh = $derived(s ? s.data.rows === 0 : false);
	const connected = $derived(s?.spotify.state === 'connected');
	const importing = $derived(
		s?.imports.some((j) => j.state === 'running' || j.state === 'queued') ?? false
	);

	const sel = $derived(view.range);
	/** "today", "this week", "in the last 4 weeks", "in Mar 1 – 9, 2026". */
	const when = $derived.by(() => {
		if (sel.preset === 'custom') return ` in ${dateSpan(sel.from, sel.to)}`;
		const l = PRESET_LABEL[sel.preset].toLowerCase();
		if (sel.preset === 'all') return '';
		if (sel.preset === 'today' || sel.preset === 'prev_day' || l.startsWith('this '))
			return ` ${l}`;
		return ` in the ${l}`;
	});
	const btn =
		'inline-flex h-9 items-center rounded-lg border bg-card px-3.5 text-sm font-medium hover:bg-muted';
</script>

<section
	class="grid gap-6 rounded-xl border border-dashed bg-card/50 px-6 py-10 sm:grid-cols-[auto_1fr] sm:items-center sm:px-10 {className}"
>
	<svg viewBox="0 0 96 48" class="h-12 w-auto" aria-hidden="true">
		{#each { length: 8 } as _, c (c)}
			{#each { length: 4 } as _, r (r)}
				<circle
					cx={c * 12 + 6}
					cy={r * 12 + 6}
					r="3.2"
					class={importing && r === 3 && c === 0 ? 'fill-signal' : 'fill-grille'}
				/>
			{/each}
		{/each}
	</svg>
	<div class="min-w-0">
		{#if fresh && importing}
			<h2 class="text-lg font-semibold">Your import is running</h2>
			<p class="mt-1 text-muted-foreground">Plays appear here as it finishes.</p>
			<div class="mt-4 flex flex-wrap gap-2">
				<a href={settingsHref('import')} class={btn}>See progress</a>
			</div>
		{:else if fresh}
			<h2 class="text-lg font-semibold">No plays yet</h2>
			<p class="mt-1 max-w-[60ch] text-muted-foreground">
				{connected
					? 'Play something on Spotify and it shows up here. Import your export for the years before.'
					: 'Connect Spotify to capture new plays, and import your export for the years before.'}
			</p>
			<div class="mt-4 flex flex-wrap gap-2">
				{#if !connected}
					<a
						href={resolve('/welcome')}
						class="inline-flex h-9 items-center rounded-lg bg-primary px-3.5 text-sm font-medium text-primary-foreground hover:opacity-90"
						>Connect Spotify</a
					>
				{/if}
				<a href={settingsHref('import')} class={btn}>Import history</a>
			</div>
		{:else}
			<h2 class="text-lg font-semibold">
				{what}{when}
			</h2>
			{#if hint}<p class="mt-1 max-w-[60ch] text-muted-foreground">{hint}</p>{/if}
			<div class="mt-4 flex flex-wrap gap-2">
				{@render children?.()}
				{#if !filtered && sel.preset !== 'all'}
					{#if sel.preset !== 'last_12m'}
						<button type="button" onclick={() => setRange({ preset: 'last_12m' })} class={btn}
							>Show last 12 months</button
						>
					{/if}
					<button type="button" onclick={() => setRange({ preset: 'all' })} class={btn}
						>Show all time</button
					>
				{/if}
			</div>
		{/if}
	</div>
</section>
