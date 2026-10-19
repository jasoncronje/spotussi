<!--
	Desktop section navigation: icon and label for each section, the current
	one on a raised key with its icon lit. The key is one element under the
	links that glides to the new section on each navigation, so the move reads
	as one control changing position. Icons drop out below lg to fit.
-->
<script lang="ts">
	import { untrack, type Component } from 'svelte';

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

	let nav = $state<HTMLElement>();
	let key = $state({ x: 0, w: 0 });
	let ready = $state(false);
	const current = $derived(sections.find((s) => isActive(s.path))?.path);

	$effect(() => {
		if (!nav) return;
		void current;
		const el = nav;
		const measure = () => {
			const a = el.querySelector<HTMLElement>('a[aria-current="page"]');
			key = a ? { x: a.offsetLeft, w: a.offsetWidth } : { x: untrack(() => key.x), w: 0 };
		};
		measure();
		// Let the first placement land before the glide is switched on.
		const raf = requestAnimationFrame(() => (ready = true));
		const ro = new ResizeObserver(measure);
		ro.observe(el);
		return () => {
			cancelAnimationFrame(raf);
			ro.disconnect();
		};
	});
</script>

<nav bind:this={nav} aria-label="Main" class="relative hidden md:block">
	<span
		aria-hidden="true"
		class="pointer-events-none absolute top-0 left-0 h-9 rounded-lg bg-card shadow-[0_0_0_1px_var(--border),0_1px_2px_rgb(0_0_0/0.06)] motion-reduce:transition-none {ready
			? 'transition-[translate,width,opacity] duration-300 ease-(--ease-out-quart)'
			: ''}"
		style:width="{key.w}px"
		style:translate="{key.x}px 0"
		style:opacity={key.w ? 1 : 0}
	></span>
	<ul class="flex items-center gap-1">
		{#each sections as s (s.path)}
			{@const active = isActive(s.path)}
			<li>
				<a
					href={href(s.path)}
					aria-current={active ? 'page' : undefined}
					class="relative flex h-9 items-center gap-2 rounded-lg px-3 text-sm text-muted-foreground transition-colors duration-200 hover:text-foreground aria-[current]:text-foreground"
				>
					<s.icon
						class="size-4 transition-colors duration-300 max-lg:hidden {active
							? 'text-signal'
							: 'opacity-80'}"
						strokeWidth={active ? 2.25 : 1.75}
					/>
					<span class="font-medium">{s.label}</span>
				</a>
			</li>
		{/each}
	</ul>
</nav>
