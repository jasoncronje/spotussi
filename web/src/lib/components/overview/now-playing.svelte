<!--
	The display window: what is playing right now (live poller), with progress
	interpolated between polls. Falls back to the last play when idle.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import Artwork from '#lib/components/common/artwork.svelte';
	import { trackLength, ago } from '#lib/format.js';
	import { artistHref, trackHref } from '#lib/state/view.svelte.js';
	import type { Schemas } from '#lib/api/client.js';

	let { now, last }: { now: Schemas['NowPlaying'] | undefined; last: Schemas['Play'] | undefined } =
		$props();

	let tick = $state(Date.now());
	onMount(() => {
		const id = setInterval(() => (tick = Date.now()), 1000);
		return () => clearInterval(id);
	});

	const playing = $derived(now?.track ? now : null);
	const progress = $derived.by(() => {
		if (!playing || playing.progress_ms == null) return null;
		const dur = playing.track!.duration_ms ?? 0;
		const extra = playing.is_playing && playing.observed_at ? tick - playing.observed_at : 0;
		return { at: Math.min(dur, playing.progress_ms + Math.max(0, extra)), dur };
	});
	const track = $derived(playing?.track ?? last?.track ?? null);
</script>

<section
	aria-label={playing ? 'Now playing' : 'Last played'}
	class="flex min-h-26 min-w-0 items-center gap-4 rounded-xl border bg-card p-3 pr-4 sm:w-[23rem]"
>
	{#if track}
		<Artwork src={track.image_url} name={track.name} class="size-16" eager />
		<div class="min-w-0 flex-1">
			<p class="flex items-center gap-1.5 text-2xs text-muted-foreground">
				{#if playing}
					<span class="relative flex size-[5px]" aria-hidden="true">
						{#if playing.is_playing}<span
								class="absolute inset-0 animate-ping rounded-full bg-signal opacity-60"
							></span>{/if}
						<span
							class="relative size-[5px] rounded-full {playing.is_playing
								? 'bg-signal'
								: 'bg-faint'}"
						></span>
					</span>
					<span class="truncate">
						{playing.is_playing ? 'Now playing' : 'Paused'}{playing.device
							? ` on ${playing.device.name}`
							: ''}
					</span>
				{:else if last}
					<span>Last played {ago(last.ended_at, tick)}</span>
				{/if}
			</p>
			<a
				href={trackHref(track.id)}
				class="hit-above mt-0.5 block truncate font-semibold hover:underline hover:decoration-signal hover:underline-offset-4"
				>{track.name}</a
			>
			<p class="narrow truncate text-xs text-muted-foreground">
				{#each track.artists as a, k (a.id)}{k > 0 ? ', ' : ''}<a
						href={artistHref(a.id)}
						class="hit-below hover:text-foreground hover:underline">{a.name}</a
					>{/each}
			</p>
			{#if progress && progress.dur > 0}
				<div class="tnum mt-2 flex items-center gap-2 text-2xs text-muted-foreground">
					<div
						class="h-[3px] flex-1 overflow-hidden rounded-full bg-grille"
						role="progressbar"
						aria-label="Track progress"
						aria-valuemin={0}
						aria-valuemax={Math.round(progress.dur / 1000)}
						aria-valuenow={Math.round(progress.at / 1000)}
						aria-valuetext="{trackLength(progress.at)} of {trackLength(progress.dur)}"
					>
						<div
							class="h-full rounded-full bg-signal"
							style="width: {(progress.at / progress.dur) * 100}%"
						></div>
					</div>
					<span aria-hidden="true">{trackLength(progress.at)} / {trackLength(progress.dur)}</span>
				</div>
			{/if}
		</div>
	{:else}
		<div class="size-16 rounded-[4px] border border-dashed"></div>
		<p class="text-sm text-muted-foreground">Nothing playing</p>
	{/if}
</section>
