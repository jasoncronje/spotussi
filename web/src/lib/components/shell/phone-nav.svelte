<!--
	Phone section navigation: a tab bar along the bottom, matching main-nav.
	The current tab's icon sits on a raised pill that glides between tabs.
-->
<script lang="ts">
	import type { Component } from 'svelte';

	type Section = {
		path: string;
		label: string;
		icon: Component<{ class?: string; strokeWidth?: number }>;
	};
	let {
		sections,
		href,
		isActive
	}: {
		sections: readonly Section[];
		href: (path: string) => string;
		isActive: (path: string) => boolean;
	} = $props();

	const index = $derived(sections.findIndex((s) => isActive(s.path)));
</script>

<nav
	aria-label="Main"
	class="fixed inset-x-0 bottom-0 z-40 border-t bg-background/92 pb-[env(safe-area-inset-bottom)] backdrop-blur-md md:hidden"
>
	<div class="relative mx-auto h-16 max-w-lg">
		<span
			aria-hidden="true"
			class="pointer-events-none absolute inset-y-0 left-0 transition-[translate,opacity] duration-300 ease-(--ease-out-quart) motion-reduce:transition-none"
			style:width="{100 / sections.length}%"
			style:translate="{Math.max(index, 0) * 100}% 0"
			style:opacity={index < 0 ? 0 : 1}
		>
			<span
				class="absolute top-2 left-1/2 h-7 w-11 -translate-x-1/2 rounded-full bg-card shadow-[0_0_0_1px_var(--border)]"
			></span>
		</span>
		<ul class="grid h-full" style:grid-template-columns="repeat({sections.length}, minmax(0, 1fr))">
			{#each sections as s (s.path)}
				{@const active = isActive(s.path)}
				<li>
					<a
						href={href(s.path)}
						aria-current={active ? 'page' : undefined}
						class="flex h-full flex-col items-center justify-center gap-1 text-2xs text-muted-foreground aria-[current]:text-foreground"
					>
						<span class="relative flex h-7 w-11 items-center justify-center">
							<s.icon
								class="size-5 transition-colors duration-300 {active ? 'text-signal' : ''}"
								strokeWidth={active ? 2.25 : 1.75}
							/>
						</span>
						{s.label}
					</a>
				</li>
			{/each}
		</ul>
	</div>
</nav>
