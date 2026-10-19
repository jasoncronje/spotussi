<!--
	One listening session: when it ran, how long, and every row in order. The
	tape across the top places each row at its real time between the first
	start and the last end, so gaps and skips are visible at a glance:
	counted plays are lit, skipped or uncounted rows are unlit.
-->
<script lang="ts">
	import { page } from '$app/state';
	import ArrowLeft from '@lucide/svelte/icons/arrow-left';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import SectionError from '#lib/components/common/section-error.svelte';
	import PlayRow from '#lib/components/history/play-row.svelte';
	import { Resource } from '#lib/data/resource.svelte.js';
	import { sessionQuery } from '#lib/data/queries.js';
	import { artistHref, sectionHref } from '#lib/state/view.svelte.js';
	import { clockTime, duration, durationParts, int, longDay, timeSpan } from '#lib/format.js';

	const id = $derived(page.params.id!);
	const res = new Resource(() => sessionQuery(id));
	const d = $derived(res.current);
	const s = $derived(d?.session);
	const notFound = $derived(
		res.error && 'status' in res.error && (res.error as { status: number }).status === 404
	);

	const span = $derived(s ? Math.max(1, s.ended_at - s.started_at) : 1);
	const pos = (at: number) => (s ? ((at - s.started_at) / span) * 100 : 0);
	const ticks = $derived.by(() => {
		if (!s) return [];
		const out: number[] = [];
		const stepMs =
			span > 4 * 3_600_000 ? 3_600_000 : span > 90 * 60_000 ? 30 * 60_000 : 15 * 60_000;
		const first = Math.ceil(s.started_at / stepMs) * stepMs;
		for (let t = first; t < s.ended_at; t += stepMs) out.push(t);
		return out;
	});
	const readouts = $derived(
		s
			? [
					{ label: 'Length', parts: durationParts(s.duration_ms) },
					{ label: 'Listened', parts: durationParts(s.ms_played) },
					{ label: 'Plays', parts: [{ value: int(s.plays), unit: '' }] },
					{ label: 'Skipped', parts: [{ value: int(s.skips), unit: '' }] }
				]
			: []
	);
</script>

<svelte:head
	><title>{s ? `Session, ${longDay(s.started_at)}` : 'Session'} – spotussi</title></svelte:head
>

<div class="mx-auto max-w-[90rem] px-4 pt-6 pb-20 sm:px-6 lg:px-10 lg:pt-10">
	<a
		href={sectionHref('/sessions')}
		class="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
		><ArrowLeft class="size-4" aria-hidden="true" />All sessions</a
	>

	{#if notFound}
		<section class="mt-6 grid gap-2 rounded-xl border border-dashed bg-card/50 px-6 py-10 sm:px-10">
			<h1 class="display text-2xl">Session not found</h1>
			<p class="max-w-[60ch] text-muted-foreground">
				Sessions are recalculated when the gap or your history changes. Find it in the list.
			</p>
		</section>
	{:else if res.error && !d}
		<SectionError class="mt-6" what="this session" error={res.error} retry={res.retry} />
	{:else}
		<header class="mt-4">
			<p class="text-sm text-muted-foreground">Session</p>
			<h1 class="display text-2xl text-balance sm:text-3xl">
				{#if s}{longDay(s.started_at)}{:else}<Skeleton class="h-9 w-80" />{/if}
			</h1>
			<p class="mt-2 min-h-5 text-sm text-muted-foreground">
				{#if s}
					<span class="tnum text-foreground">{timeSpan(s.started_at, s.ended_at)}.</span>
					{int(s.rows)} rows, mostly
					{#each s.top_artists.slice(0, 3) as a, k (a.id)}{k > 0
							? k === Math.min(3, s.top_artists.length) - 1
								? ' and '
								: ', '
							: ''}<a href={artistHref(a.id)} class="inline-link hit">{a.name}</a>{/each}.
				{/if}
			</p>
		</header>

		<section aria-label="Totals" class="mt-6 overflow-hidden rounded-xl border bg-card">
			<dl class="grid grid-cols-2 lg:grid-cols-4">
				{#each readouts as r, i (r.label)}
					<div
						class="flex flex-col gap-2 px-5 py-5 sm:px-6 {i % 2 ? 'border-l' : ''} {i >= 2
							? 'border-t lg:border-t-0'
							: ''} {i === 2 ? 'lg:border-l' : ''}"
					>
						<dt class="text-sm text-muted-foreground">{r.label}</dt>
						<dd class="figure text-2xl leading-none whitespace-nowrap sm:text-3xl">
							{#each r.parts as p, j (j)}<span class={j > 0 ? 'ml-[0.3em]' : ''}>{p.value}</span
								>{#if p.unit}<span
										class="ml-[0.12em] text-[0.5em] font-medium tracking-normal text-muted-foreground"
										>{p.unit}</span
									>{/if}{/each}
						</dd>
					</div>
				{:else}
					{#each { length: 4 } as _, i (i)}
						<div class="grid gap-2 px-5 py-5 sm:px-6">
							<Skeleton class="h-4 w-16" /><Skeleton class="h-8 w-24" />
						</div>
					{/each}
				{/each}
			</dl>

			<!-- Tape -->
			{#if d && s}
				<div class="border-t px-5 pt-5 pb-4 sm:px-6">
					<div class="relative h-8 rounded-md bg-muted" aria-hidden="true">
						{#each d.plays as p (p.id)}
							<span
								class="absolute inset-y-1 rounded-[2px] {p.counts_as_play && !p.skipped
									? 'bg-signal'
									: 'bg-grille'}"
								style="left: {pos(p.played_at)}%; width: max(2px, calc({Math.max(
									0,
									pos(p.ended_at) - pos(p.played_at)
								)}% - 1px))"
								title="{p.track.name}, {clockTime(p.played_at)}"
							></span>
						{/each}
					</div>
					<div class="tnum relative mt-1.5 h-4 text-2xs text-muted-foreground" aria-hidden="true">
						{#each ticks as t (t)}
							{#if pos(t) > 6 && pos(t) < 94}
								<span class="absolute -translate-x-1/2" style="left: {pos(t)}%">{clockTime(t)}</span
								>
							{/if}
						{/each}
						<span class="absolute left-0">{clockTime(s.started_at)}</span>
						<span class="absolute right-0">{clockTime(s.ended_at)}</span>
					</div>
					<p class="mt-2 text-xs text-muted-foreground">
						Lit: counted. Unlit: skipped or too short. Gaps: silence, {duration(
							Math.max(0, s.duration_ms - s.ms_played)
						)} in all.
					</p>
				</div>
			{/if}
		</section>

		<section aria-labelledby="plays-h" class="mt-10">
			<header class="mb-1 flex items-baseline justify-between gap-3 border-b pb-2.5">
				<h2 id="plays-h" class="text-base font-semibold">In order</h2>
				{#if d}<span class="tnum text-xs text-muted-foreground">{int(d.plays.length)} rows</span
					>{/if}
			</header>
			{#if d}
				<ol class="divide-y divide-border/60">
					{#each d.plays as p, i (p.id)}<PlayRow play={p} index={i + 1} />{/each}
				</ol>
			{:else}
				<div class="grid gap-2 pt-2" aria-busy="true">
					{#each { length: 8 } as _, i (i)}<Skeleton class="h-12 w-full" />{/each}
				</div>
			{/if}
		</section>
	{/if}
</div>
