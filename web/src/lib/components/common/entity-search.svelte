<!--
	Find an artist, album or track by name (GET /search) and pick it. Used by
	History's filter and Settings' exclusions. A combobox: arrow keys move
	through the results, Enter picks, Escape closes.
-->
<script lang="ts" module>
	export type Picked = {
		kind: 'artist' | 'album' | 'track';
		id: string;
		name: string;
		sub?: string;
	};
</script>

<script lang="ts">
	import Search from '@lucide/svelte/icons/search';
	import Artwork from './artwork.svelte';
	import { fetchQuery } from '#lib/data/cache.js';
	import { searchQuery } from '#lib/data/queries.js';

	let {
		label,
		placeholder = 'Search',
		kinds = ['artist', 'album', 'track'],
		onselect,
		class: className = ''
	}: {
		label: string;
		placeholder?: string;
		kinds?: Picked['kind'][];
		onselect: (p: Picked) => void;
		class?: string;
	} = $props();

	const uid = $props.id();
	let text = $state('');
	let open = $state(false);
	let active = $state(0);
	let results = $state<(Picked & { image: string | null })[]>([]);
	let busy = $state(false);
	let seq = 0;

	const KIND_LABEL = { artist: 'Artist', album: 'Album', track: 'Track' } as const;

	let timer: ReturnType<typeof setTimeout> | undefined;
	function input() {
		clearTimeout(timer);
		const t = text.trim();
		if (t.length < 2) {
			results = [];
			open = false;
			return;
		}
		timer = setTimeout(() => run(t), 160);
	}
	async function run(t: string) {
		const n = ++seq;
		busy = true;
		try {
			const r = await fetchQuery(searchQuery(t, 5));
			if (n !== seq) return;
			const out: (Picked & { image: string | null })[] = [];
			if (kinds.includes('artist'))
				for (const a of r.artists)
					out.push({ kind: 'artist', id: a.id, name: a.name, image: a.image_url });
			if (kinds.includes('album'))
				for (const a of r.albums)
					out.push({
						kind: 'album',
						id: a.id,
						name: a.name,
						sub: a.artists.map((x) => x.name).join(', '),
						image: a.image_url
					});
			if (kinds.includes('track'))
				for (const t of r.tracks)
					out.push({
						kind: 'track',
						id: t.id,
						name: t.name,
						sub: t.artists.map((x) => x.name).join(', '),
						image: t.image_url
					});
			results = out;
			active = 0;
			open = true;
		} catch {
			results = [];
		} finally {
			if (n === seq) busy = false;
		}
	}
	function pick(p: Picked) {
		onselect(p);
		text = '';
		results = [];
		open = false;
	}
	function key(e: KeyboardEvent) {
		if (!open || !results.length) return;
		if (e.key === 'ArrowDown') {
			active = (active + 1) % results.length;
			e.preventDefault();
		} else if (e.key === 'ArrowUp') {
			active = (active - 1 + results.length) % results.length;
			e.preventDefault();
		} else if (e.key === 'Enter') {
			pick(results[active]!);
			e.preventDefault();
		} else if (e.key === 'Escape') {
			open = false;
		}
	}
</script>

<div class="relative {className}">
	<label for="{uid}-in" class="sr-only">{label}</label>
	<Search
		class="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
		aria-hidden="true"
	/>
	<input
		id="{uid}-in"
		type="search"
		role="combobox"
		autocomplete="off"
		aria-expanded={open}
		aria-controls="{uid}-list"
		aria-autocomplete="list"
		aria-activedescendant={open && results.length ? `${uid}-o${active}` : undefined}
		bind:value={text}
		oninput={input}
		onkeydown={key}
		onfocus={() => results.length && (open = true)}
		onblur={() => setTimeout(() => (open = false), 150)}
		{placeholder}
		class="h-9 w-full rounded-lg border bg-card pr-3 pl-8 text-base outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/40 sm:text-sm"
	/>
	<ul
		id="{uid}-list"
		role="listbox"
		aria-label="{label} results"
		class="absolute top-full right-0 left-0 z-30 mt-1 max-h-80 overflow-auto rounded-lg border bg-popover p-1 shadow-[0_12px_32px_-12px_rgb(0_0_0/0.35)] {open
			? ''
			: 'hidden'}"
	>
		{#each results as r, i (r.kind + r.id)}
			<li
				id="{uid}-o{i}"
				role="option"
				aria-selected={i === active}
				class="flex cursor-pointer items-center gap-2.5 rounded-md px-2 py-1.5 text-sm aria-selected:bg-muted"
				onpointerdown={(e) => {
					e.preventDefault();
					pick(r);
				}}
				onpointerenter={() => (active = i)}
			>
				<Artwork src={r.image} name={r.name} round={r.kind === 'artist'} class="size-8" />
				<span class="min-w-0 flex-1">
					<span class="block truncate font-medium">{r.name}</span>
					<span class="block truncate text-xs text-muted-foreground"
						>{KIND_LABEL[r.kind]}{#if r.sub}, {r.sub}{/if}</span
					>
				</span>
			</li>
		{:else}
			<li class="px-2 py-3 text-sm text-muted-foreground" role="presentation">
				{busy ? 'Searching…' : 'Nothing found.'}
			</li>
		{/each}
	</ul>
</div>
