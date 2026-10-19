<!--
	Marks listening time that is estimated rather than measured (D7): plays
	caught only by the recently-played backstop count their full track length.
-->
<script lang="ts">
	import * as Tooltip from '#lib/components/ui/tooltip/index.js';
	import { duration } from '#lib/format.js';

	let { ms, total }: { ms: number; total: number } = $props();
	const pct = $derived(total > 0 ? (ms / total) * 100 : 0);
</script>

<Tooltip.Provider delayDuration={150}>
	<Tooltip.Root>
		<Tooltip.Trigger
			class="inline-flex w-fit items-center gap-1.5 rounded text-left text-xs text-muted-foreground underline decoration-faint decoration-dotted underline-offset-[3px] hover:text-foreground"
		>
			<span
				aria-hidden="true"
				class="inline-block size-2.5 rounded-[2px]"
				style="background: repeating-linear-gradient(0deg, var(--signal-dim) 0 3px, transparent 3px 4.5px)"
			></span>
			≈ {duration(ms)} estimated
		</Tooltip.Trigger>
		<Tooltip.Content side="bottom" class="max-w-72">
			{pct < 1 ? 'Under 1%' : `${Math.round(pct)}%`} of this time is estimated. Some plays were only seen
			after they finished, so they count the full track length instead of the time you actually listened.
		</Tooltip.Content>
	</Tooltip.Root>
</Tooltip.Provider>
