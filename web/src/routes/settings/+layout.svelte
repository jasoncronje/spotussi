<!--
	Settings shell: the title and the tab list, one route per tab. A rail
	beside the content on wide screens, a scrolling strip on phones; both are
	plain links with aria-current, so every tab has a URL and back/forward work.
-->
<script lang="ts">
	import { page } from '$app/state';
	import { SETTINGS_TABS, settingsHref } from '#lib/components/settings/tabs.js';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

	const active = $derived(
		SETTINGS_TABS.find((t) => page.url.pathname.endsWith(`/settings/${t.id}`))
	);

	let strip = $state<HTMLElement>();
	// Keep the current tab in view on the phone strip without scrolling the page.
	$effect(() => {
		const id = active?.id;
		if (!id || !strip) return;
		const el = strip.querySelector<HTMLElement>(`[data-tab="${id}"]`);
		if (el && strip.scrollWidth > strip.clientWidth)
			strip.scrollTo({ left: el.offsetLeft - strip.clientWidth / 2 + el.offsetWidth / 2 });
	});
</script>

<svelte:head
	><title>{active ? `Settings, ${active.label}` : 'Settings'} – spotussi</title></svelte:head
>

<div class="mx-auto max-w-[90rem] px-4 pt-6 pb-20 sm:px-6 lg:px-10 lg:pt-10">
	<h1 class="display text-2xl sm:text-3xl">Settings</h1>

	<div
		class="mt-5 grid grid-cols-[minmax(0,1fr)] gap-6 lg:mt-6 lg:grid-cols-[12rem_minmax(0,48rem)] lg:gap-12"
	>
		<nav aria-label="Settings" class="-mx-4 min-w-0 border-b sm:-mx-6 lg:mx-0 lg:border-0">
			<ul
				bind:this={strip}
				class="flex [scrollbar-width:none] gap-1 overflow-x-auto px-4 sm:px-6 lg:sticky lg:top-6 lg:grid lg:gap-0.5 lg:overflow-visible lg:px-0 [&::-webkit-scrollbar]:hidden"
			>
				{#each SETTINGS_TABS as t (t.id)}
					{@const on = active?.id === t.id}
					<li class="shrink-0" data-tab={t.id}>
						<a
							href={settingsHref(t.id)}
							aria-current={on ? 'page' : undefined}
							class="relative flex h-11 items-center gap-2.5 px-2.5 text-sm whitespace-nowrap text-muted-foreground transition-colors hover:text-foreground aria-[current]:text-foreground max-lg:aria-[current]:after:absolute max-lg:aria-[current]:after:inset-x-2.5 max-lg:aria-[current]:after:-bottom-px max-lg:aria-[current]:after:h-0.5 max-lg:aria-[current]:after:rounded-full max-lg:aria-[current]:after:bg-signal lg:h-9 lg:rounded-md lg:hover:bg-muted lg:aria-[current]:bg-muted"
						>
							<t.icon
								class="size-4 shrink-0 {on ? 'text-signal-ink' : ''}"
								strokeWidth={on ? 2.25 : 1.75}
								aria-hidden="true"
							/>
							<span class="font-medium">{t.label}</span>
						</a>
					</li>
				{/each}
			</ul>
		</nav>

		<div class="grid min-w-0 grid-cols-[minmax(0,1fr)] content-start gap-6">
			{@render children()}
		</div>
	</div>
</div>
