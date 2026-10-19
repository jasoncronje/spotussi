<!--
	Global time-range picker. Quick picks are real links (linkable, back/forward,
	middle-click), prefetching the page's data on hover/focus so the switch
	itself is instant. Every other preset and the custom range live in "More".
-->
<script lang="ts">
	import * as Popover from '#lib/components/ui/popover/index.js';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import Check from '@lucide/svelte/icons/check';
	import Lamp from './lamp.svelte';
	import { view, hrefWithRange, setRange } from '#lib/state/view.svelte.js';
	import { resolvedRange } from '#lib/state/resolved.svelte.js';
	import { prefetchRange } from '#lib/data/prefetch.js';
	import { dateSpan } from '#lib/format.js';
	import {
		PRESET_GROUPS,
		PRESET_LABEL,
		QUICK_PRESETS,
		sameRange,
		type Preset,
		type RangeSel
	} from '#lib/ranges.js';

	let open = $state(false);
	const sel = $derived(view.range);
	const inQuick = $derived(QUICK_PRESETS.some((q) => q.preset === sel.preset));
	const currentLabel = $derived(
		sel.preset === 'custom' ? dateSpan(sel.from, sel.to) : PRESET_LABEL[sel.preset]
	);

	function presetSel(p: Preset): RangeSel {
		return { preset: p as Exclude<Preset, 'custom'> };
	}

	// Custom range form, seeded from the range on screen.
	let from = $state('');
	let to = $state('');
	const today = new Date().toLocaleDateString('en-CA');
	$effect(() => {
		if (!open) return;
		from = sel.preset === 'custom' ? sel.from : resolvedRange.from || today;
		to = sel.preset === 'custom' ? sel.to : resolvedRange.to || today;
	});
	const customValid = $derived(Boolean(from && to && from <= to));

	function applyCustom(e: SubmitEvent) {
		e.preventDefault();
		if (!customValid) return;
		open = false;
		setRange({ preset: 'custom', from, to });
	}

	function warm(s: RangeSel) {
		if (!sameRange(s, sel)) prefetchRange(s);
	}
</script>

<div class="flex min-w-0 items-center gap-1.5">
	<!-- Quick picks: md and up -->
	<nav aria-label="Time range" class="hidden md:block">
		<ul class="flex items-center rounded-lg bg-muted p-[3px]">
			{#each QUICK_PRESETS as q (q.preset)}
				{@const s = presetSel(q.preset)}
				{@const active = sel.preset === q.preset}
				<li>
					<a
						href={hrefWithRange(s)}
						data-sveltekit-reset="false"
						aria-current={active ? 'true' : undefined}
						title={PRESET_LABEL[q.preset]}
						onpointerenter={() => warm(s)}
						onfocus={() => warm(s)}
						class="flex h-7 items-center gap-1.5 rounded-md px-2.5 text-xs font-medium whitespace-nowrap text-muted-foreground transition-colors hover:text-foreground aria-[current]:bg-card aria-[current]:text-foreground aria-[current]:shadow-[0_1px_0_var(--border),0_0_0_1px_var(--border)]"
					>
						<Lamp on={active} />
						{q.short}
					</a>
				</li>
			{/each}
		</ul>
	</nav>

	<Popover.Root bind:open>
		<Popover.Trigger
			class="group flex h-8 max-w-full min-w-0 items-center gap-1.5 rounded-lg px-2.5 text-xs font-medium transition-colors hover:bg-muted aria-expanded:bg-muted max-md:border max-md:bg-card max-md:text-sm {!inQuick
				? 'md:bg-card md:text-foreground md:shadow-[0_0_0_1px_var(--border)]'
				: 'md:text-muted-foreground md:hover:text-foreground'}"
			aria-label="Choose time range, current: {currentLabel}"
		>
			<span class="md:hidden"><Lamp on /></span>
			{#if !inQuick}<span class="hidden md:inline"><Lamp on /></span>{/if}
			<span class="truncate md:hidden">{currentLabel}</span>
			<span class="hidden truncate md:inline">{inQuick ? 'More' : currentLabel}</span>
			<ChevronDown
				class="size-3.5 shrink-0 opacity-60 transition-transform group-aria-expanded:rotate-180"
			/>
		</Popover.Trigger>
		<Popover.Content
			align="end"
			sideOffset={8}
			class="w-[min(30rem,calc(100vw-1.5rem))] gap-0 rounded-xl border p-0 shadow-[0_12px_32px_-12px_rgb(0_0_0/0.28)] ring-0"
		>
			<div class="grid grid-cols-1 gap-x-2 p-2 sm:grid-cols-2">
				{#each PRESET_GROUPS as group (group.label)}
					<div class="py-1">
						<p class="px-2 pt-1 pb-1.5 text-2xs text-faint">{group.label}</p>
						<ul>
							{#each group.presets as p (p)}
								{@const s = presetSel(p)}
								{@const active = sel.preset === p}
								<li>
									<a
										href={hrefWithRange(s)}
										data-sveltekit-reset="false"
										aria-current={active ? 'true' : undefined}
										onclick={() => (open = false)}
										onpointerenter={() => warm(s)}
										onfocus={() => warm(s)}
										class="flex h-8 items-center justify-between rounded-md px-2 text-sm transition-colors hover:bg-muted aria-[current]:font-semibold"
									>
										{PRESET_LABEL[p]}
										{#if active}<Check class="size-4 text-signal-ink" strokeWidth={3} />{/if}
									</a>
								</li>
							{/each}
						</ul>
					</div>
				{/each}
			</div>
			<form onsubmit={applyCustom} class="border-t px-4 pt-3 pb-4">
				<p class="mb-2 text-2xs text-faint">Custom range</p>
				<div class="flex flex-wrap items-end gap-2">
					<label class="grid min-w-0 flex-1 gap-1 text-2xs text-muted-foreground">
						From
						<input
							type="date"
							bind:value={from}
							max={to || today}
							min={resolvedRange.firstDate || undefined}
							class="tnum h-9 rounded-md border bg-background px-2 text-sm text-foreground"
						/>
					</label>
					<label class="grid min-w-0 flex-1 gap-1 text-2xs text-muted-foreground">
						To
						<input
							type="date"
							bind:value={to}
							min={from || undefined}
							max={today}
							class="tnum h-9 rounded-md border bg-background px-2 text-sm text-foreground"
						/>
					</label>
					<button
						type="submit"
						disabled={!customValid}
						class="h-9 rounded-md bg-primary px-3.5 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-90 disabled:opacity-40 max-sm:basis-full"
					>
						Show range
					</button>
				</div>
			</form>
		</Popover.Content>
	</Popover.Root>
</div>
