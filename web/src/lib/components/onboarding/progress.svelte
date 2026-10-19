<!--
	Setup progress as the station scale of a tuner: a graphite glass window
	with one station per step, printed like frequencies, minor ticks between,
	and an amber needle on the current step. The scale behind the needle is
	lit. Changing step glides the needle there with a dial-cord overshoot;
	nothing else moves. The lamp is the receiver's "tuned" light, lit once
	Spotify is connected.

	The stations are the step links (aria-current "step"); finished and
	reachable steps can be revisited. Reduced motion: the needle jumps.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import Lamp from '#lib/components/shell/lamp.svelte';
	import { ONBOARDING, type OnboardingStep } from './steps.js';

	let {
		current,
		done,
		tuned = false,
		href,
		ownerLabel = 'Password'
	}: {
		current: OnboardingStep;
		done: Partial<Record<OnboardingStep, boolean>>;
		/** Spotify connected: lights the tuned lamp. */
		tuned?: boolean;
		/** Link for a step, or null when it can't be revisited. */
		href: (s: OnboardingStep) => string | null;
		ownerLabel?: string;
	} = $props();

	const N = ONBOARDING.length;
	const MINOR = 5;
	const index = $derived(ONBOARDING.findIndex((s) => s.id === current));
	// Station i sits at the centre of column i, under its link.
	const at = (i: number) => ((i + 0.5) / N) * 100;
	const ticks: { x: number; major: boolean }[] = [];
	for (let i = 0; i < N; i++) {
		ticks.push({ x: at(i), major: true });
		if (i < N - 1)
			for (let m = 1; m < MINOR; m++)
				ticks.push({ x: at(i) + (m / MINOR) * (100 / N), major: false });
	}
	const needle = $derived(at(index));

	// No glide in from nowhere on first paint.
	let ready = $state(false);
	onMount(() => {
		const id = requestAnimationFrame(() => (ready = true));
		return () => cancelAnimationFrame(id);
	});
</script>

<div class="grid gap-2.5">
	<div class="flex justify-end">
		<span
			class="inline-flex items-center gap-2 text-xs transition-colors duration-500 {tuned
				? 'text-foreground'
				: 'text-muted-foreground'}"
			><Lamp on={tuned} />Spotify<span class="sr-only">{tuned ? 'connected' : 'not connected'}</span
			></span
		>
	</div>
	<nav aria-label="Setup steps" class="scale relative overflow-hidden rounded-2xl" class:ready>
		<div
			aria-hidden="true"
			class="pointer-events-none absolute inset-x-0 top-[calc(50%+0.5rem)] h-8 -translate-y-1/2 sm:top-1/2 sm:h-10"
		>
			<span class="rail absolute inset-x-0 top-1/2 h-px"></span>
			{#each ticks as t, i (i)}
				<span
					class="tick absolute top-1/2 w-px -translate-x-1/2 -translate-y-1/2 {t.major
						? 'h-full'
						: 'h-[38%]'}"
					class:lit={t.x <= needle + 0.01}
					style="left: {t.x}%"
				></span>
			{/each}
			<span
				class="needle absolute -inset-y-1.5 w-[3px] -translate-x-1/2 rounded-full"
				style="left: {needle}%"
			></span>
		</div>
		<ol class="relative grid" style="grid-template-columns: repeat({N}, minmax(0, 1fr))">
			{#each ONBOARDING as s, i (s.id)}
				{@const link = href(s.id)}
				{@const state = i === index ? 'now' : done[s.id] ? 'done' : 'todo'}
				{@const label = s.id === 'owner' ? ownerLabel : s.label}
				<li class="min-w-0">
					<svelte:element
						this={link ? 'a' : 'span'}
						href={link ?? undefined}
						aria-current={i === index ? 'step' : undefined}
						data-state={state}
						class="station flex flex-col items-center gap-12 rounded-xl px-1 pt-3 pb-[3.5rem] text-center outline-offset-[-3px] sm:gap-14 sm:pt-3.5 sm:pb-4 {link
							? 'hover:[--ink:var(--scale-on)]'
							: ''}"
					>
						<span class="figure tnum text-sm leading-none sm:text-base">{i + 1}</span>
						<span class="narrow w-full truncate text-xs leading-tight max-sm:sr-only">{label}</span>
						{#if state === 'done'}<span class="sr-only">(done)</span>{/if}
					</svelte:element>
				</li>
			{/each}
		</ol>
	</nav>
</div>

<style>
	.scale {
		background: var(--window);
		box-shadow:
			inset 0 0 0 1px var(--window-edge),
			inset 0 2px 6px -2px oklch(0 0 0 / 0.5),
			0 1px 0 0 var(--window-lip);
	}
	/* The glass: a faint sheen across the top. */
	.scale::after {
		content: '';
		position: absolute;
		inset: 0;
		background: linear-gradient(176deg, oklch(1 0 0 / 0.055), transparent 46%);
		pointer-events: none;
	}
	.station {
		--ink: var(--scale-off-ink);
		color: var(--ink);
		transition: color 0.2s;
	}
	.station[data-state='done'] {
		--ink: var(--scale-on);
	}
	.station[data-state='now'] {
		--ink: var(--scale-lit);
	}
	.station:focus-visible {
		outline-color: var(--scale-lit);
	}
	.rail {
		background: var(--scale-tick);
	}
	.tick {
		background: var(--scale-tick);
	}
	.tick.lit {
		background: var(--scale-lit);
		opacity: 0.85;
	}
	.needle {
		background: var(--scale-lit);
		box-shadow:
			0 0 6px 0 var(--scale-glow),
			0 0 22px 3px var(--scale-glow);
	}
	.ready .needle {
		transition: left 0.8s cubic-bezier(0.3, 1.45, 0.45, 1);
	}
	.ready .tick {
		transition: background-color 0.3s ease 0.2s;
	}
	@media (prefers-reduced-motion: reduce) {
		.ready .needle,
		.ready .tick {
			transition: none;
		}
	}
</style>
