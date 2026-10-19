<!--
	The explanation behind a label, kept out of the way: a small info mark
	that shows the text on hover or focus. Screen readers get the text as the
	button's name, so nothing is hidden from them.
-->
<script lang="ts">
	import Info from '@lucide/svelte/icons/info';
	import * as Tooltip from '#lib/components/ui/tooltip/index.js';

	let {
		text,
		side = 'top',
		class: className = ''
	}: { text: string; side?: 'top' | 'bottom' | 'left' | 'right'; class?: string } = $props();
</script>

<Tooltip.Provider delayDuration={120}>
	<Tooltip.Root>
		<Tooltip.Trigger
			type="button"
			class="-m-1 inline-grid size-6 shrink-0 place-items-center rounded-full align-middle text-faint transition-colors hover:text-foreground {className}"
		>
			<Info class="size-3.5" aria-hidden="true" />
			<span class="sr-only">{text}</span>
		</Tooltip.Trigger>
		<Tooltip.Content {side} class="max-w-72">{text}</Tooltip.Content>
	</Tooltip.Root>
</Tooltip.Provider>
