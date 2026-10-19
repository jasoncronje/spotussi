<!--
	Route-level error page: unknown URLs and anything that escapes a section's
	own error handling. The shell (nav, range) stays usable around it.
-->
<script lang="ts">
	import { page } from '$app/state';
	import { sectionHref } from '#lib/state/view.svelte.js';

	const notFound = $derived(page.status === 404);
</script>

<svelte:head><title>{notFound ? 'Not found' : 'Error'} – spotussi</title></svelte:head>

<div class="mx-auto max-w-[90rem] px-4 pt-10 pb-20 sm:px-6 lg:px-10 lg:pt-14">
	<div class="flex max-w-2xl gap-6 rounded-xl border border-dashed bg-card/60 p-6 sm:p-8">
		<svg viewBox="0 0 28 28" class="hidden size-12 shrink-0 sm:block" aria-hidden="true">
			{#each { length: 16 } as _, n (n)}
				<circle
					cx={Math.floor(n / 4) * 7 + 3.5}
					cy={(3 - (n % 4)) * 7 + 3.5}
					r="2.2"
					class={n === 0 ? 'fill-signal' : 'fill-grille'}
				/>
			{/each}
		</svg>
		<div class="min-w-0">
			<h1 class="display text-xl">
				{notFound ? 'There is nothing at this address' : 'This page stopped working'}
			</h1>
			<p class="mt-2 text-sm text-muted-foreground">
				{#if notFound}
					The link may be out of date.
				{:else}
					{page.error?.message ? `${page.error.message}. ` : ''}Reloading usually fixes it.
				{/if}
			</p>
			<div class="mt-5 flex flex-wrap gap-2">
				{#if !notFound}
					<button
						type="button"
						onclick={() => location.reload()}
						class="h-9 rounded-lg bg-primary px-3.5 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-90"
					>
						Reload
					</button>
				{/if}
				<a
					href={sectionHref('/')}
					class="grid h-9 place-items-center rounded-lg border bg-card px-3.5 text-sm font-medium transition-colors hover:bg-muted"
				>
					Go to Overview
				</a>
			</div>
		</div>
	</div>
</div>
