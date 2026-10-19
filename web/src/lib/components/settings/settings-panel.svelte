<!-- A titled settings section: heading (with an optional info hint), then the body. -->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import Hint from '#lib/components/common/hint.svelte';

	let {
		id,
		title,
		hint,
		children,
		actions
	}: {
		id: string;
		title: string;
		/** The why behind the section, in a tooltip. */
		hint?: string;
		children: Snippet;
		actions?: Snippet;
	} = $props();
</script>

<section {id} aria-labelledby="{id}-h" class="scroll-mt-24 rounded-xl border bg-card">
	<header
		class="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 border-b px-5 py-4 sm:px-6"
	>
		<div class="flex min-w-0 items-center gap-1.5">
			<h2 id="{id}-h" class="text-base font-semibold">{title}</h2>
			{#if hint}<Hint text={hint} />{/if}
		</div>
		{@render actions?.()}
	</header>
	<div class="px-5 py-5 sm:px-6">
		{@render children()}
	</div>
</section>
