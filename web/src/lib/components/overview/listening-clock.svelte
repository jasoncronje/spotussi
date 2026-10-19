<!--
	Listening clock: 24 spokes of meter dots, midnight at the top, running
	clockwise. Each spoke lights in proportion to that hour's listening (the
	outermost lit dot is partially lit, so nothing is rounded away). The centre
	is the readout: busiest hour by default, the hovered/selected hour otherwise.

	Keyboard: the dial is a slider over the 24 hours (arrow keys, Home/End).
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { duration, hourLabel, hourSpan, int } from '#lib/format.js';
	import type { Schemas } from '#lib/api/client.js';
	import type { Metric } from '#lib/ranges.js';

	let { clock, metric }: { clock: Schemas['Clock']; metric: Metric } = $props();

	const SIZE = 320;
	const C = SIZE / 2;
	const RINGS = 8;
	const R0 = 64;
	const R1 = 128;
	const STEP = (R1 - R0) / (RINGS - 1);
	const DOT = 3.4;

	const hours = $derived(clock.hours);
	const val = (h: Schemas['ClockHour']) => (metric === 'plays' ? h.plays : h.ms_played);
	const max = $derived(Math.max(1, ...hours.map(val)));
	const peakHour = $derived(
		hours.reduce((best, h) => (val(h) > val(best) ? h : best), hours[0]!).hour
	);
	const total = $derived(hours.reduce((s, h) => s + val(h), 0));

	let active = $state<number | null>(null);
	let focused = $state(false);
	const shown = $derived(active ?? peakHour);

	function angle(hour: number) {
		return (hour / 24) * Math.PI * 2 - Math.PI / 2;
	}
	function pos(hour: number, r: number) {
		const a = angle(hour);
		return { x: C + Math.cos(a) * r, y: C + Math.sin(a) * r };
	}
	/** 0..1 brightness of ring k on an hour's spoke. */
	function lit(h: Schemas['ClockHour'], k: number) {
		const v = val(h);
		if (v <= 0) return 0;
		const level = Math.max((v / max) * RINGS, 0.35);
		return Math.max(0, Math.min(1, level - k));
	}

	function wedge(hour: number) {
		const a0 = angle(hour - 0.5);
		const a1 = angle(hour + 0.5);
		const ri = R0 - 12;
		const ro = R1 + 14;
		const p = (a: number, r: number) =>
			`${(C + Math.cos(a) * r).toFixed(1)} ${(C + Math.sin(a) * r).toFixed(1)}`;
		return `M${p(a0, ri)} L${p(a0, ro)} A${ro} ${ro} 0 0 1 ${p(a1, ro)} L${p(a1, ri)} A${ri} ${ri} 0 0 0 ${p(a0, ri)}Z`;
	}

	function describe(h: Schemas['ClockHour']) {
		return `${hourSpan(h.hour)}: ${int(h.plays)} plays, ${duration(h.ms_played)}`;
	}

	function onkeydown(e: KeyboardEvent) {
		const cur = active ?? peakHour;
		let next: number | null = null;
		if (e.key === 'ArrowRight' || e.key === 'ArrowUp') next = (cur + 1) % 24;
		else if (e.key === 'ArrowLeft' || e.key === 'ArrowDown') next = (cur + 23) % 24;
		else if (e.key === 'Home') next = 0;
		else if (e.key === 'End') next = 23;
		else if (e.key === 'Escape') active = null;
		if (next !== null) {
			e.preventDefault();
			active = next;
		}
	}

	// One orchestrated moment: the dial sweeps on when it first appears.
	let entered = $state(false);
	let settled = $state(false);
	onMount(() => {
		requestAnimationFrame(() => (entered = true));
		const t = setTimeout(() => (settled = true), 1400);
		return () => clearTimeout(t);
	});

	const shownHour = $derived(hours[shown]!);
	const share = $derived(total > 0 ? val(shownHour) / total : 0);
</script>

<div
	class="relative mx-auto aspect-square w-full max-w-[22rem] rounded-full outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-4 focus-visible:ring-offset-card"
	role="slider"
	tabindex="0"
	aria-label="Listening by hour of day"
	aria-valuemin={0}
	aria-valuemax={23}
	aria-valuenow={shown}
	aria-valuetext={describe(shownHour)}
	{onkeydown}
	onfocus={() => (focused = true)}
	onblur={() => {
		focused = false;
		active = null;
	}}
	onpointerleave={() => {
		if (!focused) active = null;
	}}
>
	<svg viewBox="-24 -8 {SIZE + 48} {SIZE + 16}" class="size-full" aria-hidden="true">
		<!-- Quarter labels -->
		{#each [0, 6, 12, 18] as q (q)}
			{@const p = pos(q, R1 + 22)}
			<text
				x={p.x}
				y={p.y}
				dy="0.35em"
				text-anchor="middle"
				class="fill-muted-foreground text-[10.5px] font-medium">{hourLabel(q)}</text
			>
		{/each}

		{#each hours as h (h.hour)}
			{@const isShown = shown === h.hour && (active !== null || focused)}
			<g class:dim={active !== null && active !== h.hour}>
				{#each { length: RINGS } as _, k (k)}
					{@const p = pos(h.hour, R0 + k * STEP)}
					{@const l = lit(h, k)}
					<circle cx={p.x} cy={p.y} r={DOT} class="fill-grille" />
					{#if l > 0}
						<circle
							cx={p.x}
							cy={p.y}
							r={DOT}
							class="dot fill-signal"
							class:on={entered}
							class:settled
							style="--o:{l.toFixed(3)};--d:{h.hour * 22 + k * 14}ms"
						/>
					{/if}
				{/each}
				{#if isShown}
					{@const p = pos(h.hour, R1 + 9)}
					<circle cx={p.x} cy={p.y} r="2" class="fill-foreground" />
				{/if}
			</g>
			<path
				d={wedge(h.hour)}
				fill="transparent"
				role="presentation"
				onpointerenter={() => (active = h.hour)}
			/>
		{/each}
	</svg>

	<!-- Centre readout -->
	<div
		class="pointer-events-none absolute inset-0 grid place-content-center text-center"
		aria-hidden="true"
	>
		<p class="text-2xs text-muted-foreground">
			{active === null ? 'Busiest hour' : hourSpan(shown)}
		</p>
		<p class="figure mt-0.5 text-xl leading-none">
			{active === null ? hourLabel(peakHour) : metric === 'plays' ? int(shownHour.plays) : ''}
			{#if active !== null && metric === 'time'}{duration(shownHour.ms_played)}{/if}
		</p>
		<p class="tnum mt-1 text-2xs text-muted-foreground">
			{Math.round(share * 100)}% of {metric === 'plays' ? 'plays' : 'time'}
		</p>
	</div>
</div>

<style>
	.dot {
		fill-opacity: 0;
		transition: fill-opacity 220ms ease;
	}
	.dot.on {
		fill-opacity: var(--o);
		transition: fill-opacity 380ms var(--ease-out-quart) var(--d);
	}
	.dot.on.settled {
		transition-delay: 0ms;
	}
	g {
		transition: opacity 150ms ease;
	}
	g.dim {
		opacity: 0.4;
	}
</style>
