<!--
	A chart's window: title, a one-line reading of the data, and a Chart/Table
	switch. The table is always in the document (visually hidden while the
	chart shows), so screen readers get every value; charts mark their SVG decorative.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		title,
		summary,
		meta,
		table,
		children,
		class: className = '',
		dim = false
	}: {
		title: string;
		/** Plain-language reading of the chart, e.g. "Busiest at 9 PM". */
		summary?: string;
		meta?: Snippet;
		table: Snippet;
		children: Snippet;
		class?: string;
		dim?: boolean;
	} = $props();

	const uid = $props.id();
	let showTable = $state(false);
</script>

<section
	class="flex min-w-0 flex-col rounded-xl border bg-card {className}"
	aria-labelledby="{uid}-title"
>
	<header class="px-5 pt-4 sm:px-6">
		<div class="flex flex-wrap items-center justify-between gap-x-4 gap-y-1">
			<h2 id="{uid}-title" class="text-base font-semibold">{title}</h2>
			<div class="-mr-2 flex shrink-0 items-center gap-3">
				{@render meta?.()}
				<button
					type="button"
					aria-pressed={showTable}
					onclick={() => (showTable = !showTable)}
					class="h-7 rounded-md px-2 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground aria-pressed:bg-muted aria-pressed:text-foreground"
				>
					{showTable ? 'Chart' : 'Table'}<span class="sr-only"> view of {title}</span>
				</button>
			</div>
		</div>
		{#if summary}<p class="mt-0.5 text-xs text-muted-foreground">{summary}</p>{/if}
	</header>
	<div
		class="relative flex flex-1 flex-col transition-opacity duration-200 {dim ? 'opacity-55' : ''}"
	>
		{#if !showTable}
			{@render children()}
		{/if}
		<div class={showTable ? 'max-h-80 overflow-auto px-5 pt-3 pb-5 sm:px-6' : 'sr-only'}>
			{@render table()}
		</div>
	</div>
</section>
