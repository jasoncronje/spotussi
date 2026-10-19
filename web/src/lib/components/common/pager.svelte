<!--
	Page links for offset-paginated lists. Real links (the page lives in the
	URL), with the neighbouring page prefetched on hover/focus.
-->
<script lang="ts">
	import ChevronLeft from '@lucide/svelte/icons/chevron-left';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import { int } from '#lib/format.js';

	let {
		page,
		pageSize,
		total,
		noun,
		href,
		warm
	}: {
		/** 1-based. */
		page: number;
		pageSize: number;
		total: number;
		noun: string;
		href: (page: number) => string;
		warm?: (page: number) => void;
	} = $props();

	const pages = $derived(Math.max(1, Math.ceil(total / pageSize)));
	const from = $derived(total === 0 ? 0 : (page - 1) * pageSize + 1);
	const to = $derived(Math.min(total, page * pageSize));

	/** 1 … 4 5 6 … 30 */
	const numbers = $derived.by(() => {
		const want = new Set([1, pages, page - 1, page, page + 1]);
		const out: (number | null)[] = [];
		let last = 0;
		for (let n = 1; n <= pages; n++) {
			if (!want.has(n)) continue;
			if (n - last > 1) out.push(null);
			out.push(n);
			last = n;
		}
		return out;
	});

	const btn =
		'grid h-9 min-w-9 place-items-center rounded-lg px-2 text-sm transition-colors hover:bg-muted';
</script>

<nav
	aria-label="Pages"
	class="flex flex-wrap items-center justify-between gap-x-6 gap-y-3 border-t pt-4"
>
	<p class="tnum text-sm text-muted-foreground">
		{int(from)}–{int(to)} of {int(total)}
		{noun}
	</p>
	{#if pages > 1}
		<ul class="flex items-center gap-1">
			<li>
				{#if page > 1}
					<a
						href={href(page - 1)}
						class={btn}
						aria-label="Previous page"
						onpointerenter={() => warm?.(page - 1)}
						onfocus={() => warm?.(page - 1)}><ChevronLeft class="size-4" /></a
					>
				{:else}
					<span class="{btn} opacity-35 hover:bg-transparent" aria-hidden="true"
						><ChevronLeft class="size-4" /></span
					>
				{/if}
			</li>
			{#each numbers as n, i (n ?? `gap${i}`)}
				<li class={n === null || n === page ? '' : 'max-sm:hidden'}>
					{#if n === null}
						<span class="px-1 text-faint max-sm:hidden" aria-hidden="true">…</span>
					{:else if n === page}
						<span
							aria-current="page"
							class="{btn} tnum bg-card font-semibold shadow-[0_0_0_1px_var(--border)] hover:bg-card"
							>{n}<span class="sr-only"> of {pages}</span></span
						>
					{:else}
						<a
							href={href(n)}
							class="{btn} tnum text-muted-foreground"
							onpointerenter={() => warm?.(n)}
							onfocus={() => warm?.(n)}><span class="sr-only">Page </span>{n}</a
						>
					{/if}
				</li>
			{/each}
			<li>
				{#if page < pages}
					<a
						href={href(page + 1)}
						class={btn}
						aria-label="Next page"
						onpointerenter={() => warm?.(page + 1)}
						onfocus={() => warm?.(page + 1)}><ChevronRight class="size-4" /></a
					>
				{:else}
					<span class="{btn} opacity-35 hover:bg-transparent" aria-hidden="true"
						><ChevronRight class="size-4" /></span
					>
				{/if}
			</li>
		</ul>
	{/if}
</nav>
