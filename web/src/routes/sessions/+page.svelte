<!--
	Sessions (D14): the longest continuous listening runs in the range,
	longest first. A run ends where the gap to the next row exceeds the
	session gap setting. Each row opens the session's track list.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import SectionError from '#lib/components/common/section-error.svelte';
	import EmptyState from '#lib/components/common/empty-state.svelte';
	import Pager from '#lib/components/common/pager.svelte';
	import Artwork from '#lib/components/common/artwork.svelte';
	import { Resource } from '#lib/data/resource.svelte.js';
	import { prefetch } from '#lib/data/cache.js';
	import { registerPrefetcher } from '#lib/data/prefetch.js';
	import { sessionQuery, sessionsQuery } from '#lib/data/queries.js';
	import { view, hrefWith, sessionHref } from '#lib/state/view.svelte.js';
	import { resolvedRange } from '#lib/state/resolved.svelte.js';
	import { dateSpan, durationParts, int, longDay, timeSpan } from '#lib/format.js';
	import { PRESET_LABEL } from '#lib/ranges.js';
	import { settingsHref } from '#lib/components/settings/tabs.js';

	const PAGE_SIZE = 20;
	const pageNo = $derived(Math.max(1, Number(page.url.searchParams.get('page')) || 1));
	const list = new Resource(() => sessionsQuery(view.range, PAGE_SIZE, (pageNo - 1) * PAGE_SIZE));
	// The longest session overall scales every bar, on every page.
	const longest = new Resource(() => (pageNo > 1 ? sessionsQuery(view.range, 1, 0) : null));

	onMount(() =>
		registerPrefetcher({
			range: (sel) => prefetch([sessionsQuery(sel, PAGE_SIZE, 0)])
		})
	);

	const l = $derived(list.current);
	$effect(() => {
		if (!l) return;
		resolvedRange.from = l.range.from;
		resolvedRange.to = l.range.to;
		resolvedRange.firstDate = l.range.first_play_date ?? '';
	});
	const top = $derived(
		Math.max(
			1,
			(pageNo > 1 ? longest.current?.items[0]?.duration_ms : l?.items[0]?.duration_ms) ?? 1
		)
	);
	const sel = $derived(view.range);
	const rangeTitle = $derived(
		sel.preset === 'custom' ? dateSpan(sel.from, sel.to) : PRESET_LABEL[sel.preset]
	);
	/** Lamps per row: one per 10 minutes of the longest session, at most 48. */
	const LAMPS = 48;
</script>

<svelte:head><title>Sessions, {rangeTitle} – spotussi</title></svelte:head>

<div class="mx-auto max-w-[90rem] px-4 pt-6 pb-20 sm:px-6 lg:px-10 lg:pt-10">
	<header>
		<h1 class="display text-2xl text-balance sm:text-3xl">Sessions</h1>
		<p class="mt-2 min-h-5 text-sm text-muted-foreground">
			{#if l}
				<span class="text-foreground">{rangeTitle}, {dateSpan(l.range.from, l.range.to)}.</span>
				{int(l.total)}
				{l.total === 1 ? 'session' : 'sessions'}, longest first, split at
				<a
					href={settingsHref('general')}
					title="Change the session gap"
					class="hit underline decoration-faint underline-offset-2 hover:text-foreground"
					>{int(l.gap_minutes)} min</a
				> of silence.
			{:else if !list.error}
				<Skeleton class="inline-block h-4 w-72 align-middle" />
			{/if}
		</p>
	</header>

	{#if list.error && (!l || list.stale)}
		<SectionError class="mt-6" what="sessions" error={list.error} retry={list.retry} />
	{:else if l}
		<div class="mt-6 transition-opacity duration-200 {list.stale ? 'opacity-55' : ''}">
			{#if l.items.length === 0}
				<EmptyState what="No sessions" />
			{:else}
				<ol class="overflow-hidden rounded-xl border bg-card">
					{#each l.items as s, i (s.id)}
						{@const rank = (pageNo - 1) * PAGE_SIZE + i + 1}
						{@const lit = (s.duration_ms / top) * LAMPS}
						{@const parts = durationParts(s.duration_ms)}
						<li class="relative border-b last:border-b-0 hover:bg-muted/40">
							<div
								class="grid grid-cols-[2rem_1fr_auto] items-center gap-x-4 gap-y-2 px-4 py-4 sm:px-6 lg:grid-cols-[2.5rem_minmax(0,16rem)_minmax(0,1fr)_minmax(0,14rem)_7rem]"
							>
								<span class="figure tnum text-right text-lg text-foreground/70">{rank}</span>
								<div class="min-w-0">
									<a
										href={sessionHref(s.id)}
										class="hit-above block truncate font-medium after:absolute after:inset-0 hover:underline hover:decoration-signal hover:underline-offset-4"
										onpointerenter={() => prefetch([sessionQuery(s.id)])}
										onfocus={() => prefetch([sessionQuery(s.id)])}>{longDay(s.started_at)}</a
									>
									<p class="tnum text-xs text-muted-foreground">
										{timeSpan(s.started_at, s.ended_at)}, {int(s.plays)} plays, {int(s.skips)} skipped
									</p>
								</div>
								<p
									class="figure tnum text-right text-xl leading-none whitespace-nowrap lg:order-last"
								>
									{#each parts as p, j (j)}<span class={j > 0 ? 'ml-[0.25em]' : ''}>{p.value}</span
										><span
											class="ml-[0.1em] text-[0.55em] font-medium tracking-normal text-muted-foreground"
											>{p.unit}</span
										>{/each}
								</p>
								<!-- Length as a row of lamps, scaled to the longest session -->
								<div
									class="col-span-3 col-start-1 grid items-center sm:col-span-2 sm:col-start-2 lg:col-span-1 lg:col-start-auto"
									style="grid-template-columns: repeat({LAMPS}, minmax(0, 1fr))"
									aria-hidden="true"
								>
									{#each { length: LAMPS } as _, k (k)}
										{@const o = Math.max(0, Math.min(1, lit - k))}
										<span class="relative mx-auto size-[5px] rounded-full bg-grille">
											{#if o > 0}<span
													class="absolute inset-0 rounded-full bg-signal"
													style="opacity: {o}"
												></span>{/if}
										</span>
									{/each}
								</div>
								<div
									class="relative z-10 col-span-3 flex min-w-0 items-center gap-2 sm:col-span-2 sm:col-start-2 lg:col-span-1 lg:col-start-auto"
								>
									<span class="flex shrink-0 -space-x-2">
										{#each s.top_artists.slice(0, 3) as a (a.id)}
											<Artwork
												src={a.image_url}
												name={a.name}
												round
												class="size-6 ring-2 ring-card"
											/>
										{/each}
									</span>
									<p class="truncate text-xs text-muted-foreground">
										{s.top_artists.map((a) => a.name).join(', ')}
									</p>
								</div>
							</div>
						</li>
					{/each}
				</ol>
				<div class="mt-4">
					<Pager
						page={pageNo}
						pageSize={PAGE_SIZE}
						total={l.total}
						noun="sessions"
						href={(n) => hrefWith({ page: n > 1 ? String(n) : null })}
						warm={(n) => prefetch([sessionsQuery(view.range, PAGE_SIZE, (n - 1) * PAGE_SIZE)])}
					/>
				</div>
			{/if}
		</div>
	{:else}
		<div class="mt-6 grid gap-2" aria-busy="true">
			{#each { length: 8 } as _, i (i)}<Skeleton class="h-16 w-full rounded-lg" />{/each}
		</div>
	{/if}
</div>
