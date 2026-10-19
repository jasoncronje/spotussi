<!--
	Plays / Time: decides how lists rank and what charts plot on this view.
	Remembered per view (D7) and written to the URL.
-->
<script lang="ts">
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.js';
	import Lamp from './lamp.svelte';
	import { view, setMetric } from '#lib/state/view.svelte.js';
	import { prefetchMetric } from '#lib/data/prefetch.js';
	import type { Metric } from '#lib/ranges.js';

	const options: { value: Metric; label: string; hint: string }[] = [
		{ value: 'plays', label: 'Plays', hint: 'Rank and chart by counted plays' },
		{ value: 'time', label: 'Time', hint: 'Rank and chart by time listened' }
	];
</script>

<ToggleGroup.Root
	type="single"
	value={view.metric}
	onValueChange={(v) => {
		if (v === 'plays' || v === 'time') setMetric(v);
	}}
	aria-label="Measure"
	class="rounded-lg bg-muted p-[3px]"
>
	{#each options as o (o.value)}
		{@const active = view.metric === o.value}
		<ToggleGroup.Item
			value={o.value}
			title={o.hint}
			onpointerenter={() => !active && prefetchMetric(o.value)}
			onfocus={() => !active && prefetchMetric(o.value)}
			class="h-7 min-w-0 gap-1.5 rounded-md! px-2.5 text-xs text-muted-foreground hover:bg-transparent hover:text-foreground data-[state=on]:bg-card data-[state=on]:text-foreground data-[state=on]:shadow-[0_1px_0_var(--border),0_0_0_1px_var(--border)]"
		>
			<Lamp on={active} />
			{o.label}
		</ToggleGroup.Item>
	{/each}
</ToggleGroup.Root>
