<!--
	Stand-in for views that arrive in UI milestone U1: names the question the
	page will answer and what it will show, so navigation and drill-down links
	can be tried end to end today.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { sectionHref } from '#lib/state/view.svelte.js';

	let {
		title,
		question,
		planned,
		kicker,
		children
	}: {
		title: string;
		question: string;
		planned: string[];
		kicker?: string;
		children?: Snippet;
	} = $props();

	// An unlit 12×4 meter: the empty instrument waiting for its signal.
	const cols = 12;
	const rows = 4;
</script>

<svelte:head><title>{title} – spotussi</title></svelte:head>

<div class="mx-auto max-w-[90rem] px-4 pt-8 pb-16 sm:px-6 lg:px-10 lg:pt-12">
	{#if kicker}<p class="mb-2 text-sm text-muted-foreground">{kicker}</p>{/if}
	<h1 class="display text-2xl text-balance sm:text-3xl">{title}</h1>
	<p class="mt-3 max-w-[60ch] text-base text-muted-foreground">{question}</p>

	{@render children?.()}

	<section
		class="mt-10 grid max-w-3xl gap-8 rounded-xl border border-dashed bg-card/50 p-6 sm:grid-cols-[auto_1fr] sm:p-8"
		aria-labelledby="planned-heading"
	>
		<svg viewBox="0 0 {cols * 12} {rows * 12}" class="h-12 w-auto" aria-hidden="true">
			{#each { length: cols } as _, c (c)}
				{#each { length: rows } as _, r (r)}
					<circle cx={c * 12 + 6} cy={r * 12 + 6} r="3.2" class="fill-grille" />
				{/each}
			{/each}
		</svg>
		<div>
			<h2 id="planned-heading" class="text-base font-semibold">Coming in U1</h2>
			<p class="mt-1 text-muted-foreground">This page is next in line. It will show:</p>
			<ul class="mt-4 grid gap-2">
				{#each planned as item (item)}
					<li class="flex gap-3">
						<span class="mt-[0.55em] size-1 shrink-0 rounded-full bg-faint"></span>
						<span>{item}</span>
					</li>
				{/each}
			</ul>
			<a
				href={sectionHref('/')}
				class="mt-6 inline-flex h-9 items-center rounded-lg border bg-card px-3.5 text-sm font-medium transition-colors hover:bg-muted"
			>
				Back to Overview
			</a>
		</div>
	</section>
</div>
