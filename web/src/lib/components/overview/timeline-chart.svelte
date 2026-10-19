<!--
	Listening over the period, one bar per auto bucket (hour/day/week/month).
	In Time mode the part of each bar that is estimated from track lengths
	(recently-played backstop, D7) is hatched, so the honest number and the
	guessed number are visibly different. The peak bucket is labelled.

	With a previous series (timeline `compare=true`) each bar stands in front
	of the same position in the previous period, drawn as unlit lamps: where
	this period fell short, the dark segments above the lit ones show by how
	much.
-->
<script lang="ts">
	import { Chart, Text } from 'layerchart/svg';
	import { Tooltip } from 'layerchart';
	import * as ChartUI from '#lib/components/ui/chart/index.js';
	import {
		bucketLabel,
		axisTicks,
		compact,
		duration,
		durationTickFormatter,
		niceTicks,
		int,
		type Bucket
	} from '#lib/format.js';
	import type { Schemas } from '#lib/api/client.js';
	import type { Metric } from '#lib/ranges.js';

	let {
		timeline,
		metric
	}: {
		timeline: Schemas['Timeline'];
		/** `artists` charts different artists per bucket (Stats). */
		metric: Metric | 'artists';
	} = $props();

	const valueOf = (p: Schemas['TimelinePoint']) =>
		metric === 'plays' ? p.plays : metric === 'artists' ? p.artists : p.ms_played;

	const uid = $props.id();
	/** Lamp segment and gap height (px). */
	const SEG = 4;
	const GAP = 2;

	type Row = {
		key: string;
		value: number;
		measured: number;
		plays: number;
		ms: number;
		est: number;
		skips: number;
		artists: number;
		/** Same position in the previous period (null without a comparison). */
		prev: number | null;
		prevKey: string | null;
	};

	const prevPoints = $derived(timeline.previous ? (timeline.previous_points ?? []) : []);
	const compared = $derived(prevPoints.length > 0);
	const rows = $derived<Row[]>(
		timeline.points.map((p, i) => ({
			key: p.key,
			value: valueOf(p),
			measured: metric === 'time' ? p.ms_played - p.ms_estimated : valueOf(p),
			plays: p.plays,
			ms: p.ms_played,
			est: p.ms_estimated,
			skips: p.skips,
			artists: p.artists,
			prev: prevPoints[i] ? valueOf(prevPoints[i]) : null,
			prevKey: prevPoints[i]?.key ?? null
		}))
	);

	const bucket = $derived(timeline.bucket as Bucket);
	const max = $derived(rows.reduce((m, r) => Math.max(m, r.value), 0));
	const scaleMax = $derived(rows.reduce((m, r) => Math.max(m, r.value, r.prev ?? 0), 0));
	const peak = $derived(rows.find((r) => r.value === max && max > 0));
	const hasEstimate = $derived(metric === 'time' && rows.some((r) => r.est > 0));
	const ticks = $derived(niceTicks(scaleMax, metric === 'time' ? 'duration' : 'count'));
	const yFormat = $derived(metric === 'time' ? durationTickFormatter(ticks[1] ?? 0) : compact);
	const fmt = (n: number) =>
		metric === 'plays'
			? `${int(n)} ${n === 1 ? 'play' : 'plays'}`
			: metric === 'artists'
				? `${int(n)} ${n === 1 ? 'artist' : 'artists'}`
				: duration(n);

	type Scales = {
		xScale: ((k: string) => number | undefined) & { bandwidth?: () => number };
		yScale: (v: number) => number;
	};
	/**
	 * One path per series instead of a component per bar: a bar component
	 * per bucket cost ~40 ms to rebuild for an all-time range (M7 pass).
	 */
	function barsPath(c: Scales, top: (r: Row) => number | null, base: (r: Row) => number = () => 0) {
		const bw = c.xScale.bandwidth?.() ?? 0;
		let d = '';
		for (const r of rows) {
			const hi = top(r);
			if (hi === null) continue;
			const lo = base(r);
			if (hi <= lo) continue;
			const x = c.xScale(r.key) ?? 0;
			const y1 = c.yScale(hi);
			const y0 = c.yScale(lo);
			d += `M${x} ${y0}V${y1}h${bw}V${y0}Z`;
		}
		return d;
	}

	const config = {
		value: { label: 'Listening', color: 'var(--signal)' }
	} satisfies ChartUI.ChartConfig;
</script>

<ChartUI.Container
	{config}
	class="aspect-auto min-h-60 w-full flex-1 px-2 pb-2 sm:min-h-64 sm:px-3"
>
	<Chart
		data={rows}
		x="key"
		y="value"
		yDomain={[0, ticks[ticks.length - 1]!]}
		valueAxis="y"
		bandPadding={rows.length > 90 ? 0.15 : rows.length > 40 ? 0.3 : 0.45}
		padding={{ top: 24, left: 44, bottom: 24, right: 8 }}
		tooltipContext={{ mode: 'band' }}
		axis={false}
		grid={false}
		rule={false}
		highlight={false}
	>
		{#snippet belowMarks({ context })}
			<!-- Recessive horizontal grid, one step off the panel. -->
			{#each ticks as t (t)}
				<line
					x1={0}
					x2={context.width}
					y1={context.yScale(t)}
					y2={context.yScale(t)}
					class="stroke-border"
					stroke-width="1"
					shape-rendering="crispEdges"
				/>
				<text
					x={-10}
					y={context.yScale(t)}
					dy="0.32em"
					text-anchor="end"
					class="tnum fill-muted-foreground text-2xs">{yFormat(t)}</text
				>
			{/each}
		{/snippet}

		{#snippet marks({ context })}
			<!--
				Every bar is a level meter: a mask cuts it into lamp segments counted
				up from the baseline. Heights stay exact; the top segment is simply
				cut where the value ends.
			-->
			<defs>
				<pattern
					id="{uid}-seg"
					width="8"
					height={SEG + GAP}
					y={context.height}
					patternUnits="userSpaceOnUse"
				>
					<rect y={GAP} width="8" height={SEG} fill="white" />
				</pattern>
				<mask id="{uid}-mask" maskUnits="userSpaceOnUse">
					<rect
						x="0"
						y="-20"
						width={context.width}
						height={context.height + 20}
						fill="url(#{uid}-seg)"
					/>
				</mask>
			</defs>
			<g mask="url(#{uid}-mask)">
				{#if compared}
					<!-- Previous period: unlit lamps behind the lit ones. -->
					<path d={barsPath(context, (r) => r.prev)} class="fill-grille" />
				{/if}
				<path
					d={barsPath(context, (r) => (hasEstimate ? r.measured : r.value))}
					class="fill-signal"
					fill-opacity={context.tooltip.data ? 0.4 : 1}
				/>
				{#if hasEstimate}
					<!-- Estimated time: dim lamps above the measured part. -->
					<path
						d={barsPath(
							context,
							(r) => r.value,
							(r) => r.measured
						)}
						class="fill-signal"
						fill-opacity={context.tooltip.data ? 0.16 : 0.38}
					/>
				{/if}
				{#if context.tooltip.data}
					{@const d = context.tooltip.data as Row}
					{@const x = context.xScale(d.key) ?? 0}
					{@const bw = context.xScale.bandwidth?.() ?? 0}
					<rect
						{x}
						y={context.yScale(d.measured)}
						width={bw}
						height={context.height - context.yScale(d.measured)}
						class="fill-signal"
					/>
					{#if d.value > d.measured}
						<rect
							{x}
							y={context.yScale(d.value)}
							width={bw}
							height={context.yScale(d.measured) - context.yScale(d.value)}
							class="fill-signal"
							fill-opacity="0.38"
						/>
					{/if}
				{/if}
			</g>
		{/snippet}

		{#snippet aboveMarks({ context })}
			<!-- Baseline -->
			<line
				x1={0}
				x2={context.width}
				y1={context.height + 0.5}
				y2={context.height + 0.5}
				class="stroke-foreground/25"
			/>
			<!-- X ticks: thinned to what fits -->
			{@const xTicks = axisTicks(
				rows.map((r) => r.key),
				bucket,
				context.width / 72
			)}
			{#each rows as r, i (i)}
				{#if xTicks.has(i)}
					<text
						x={(context.xScale(r.key) ?? 0) + (context.xScale.bandwidth?.() ?? 0) / 2}
						y={context.height + 16}
						text-anchor="middle"
						class="fill-muted-foreground text-2xs">{xTicks.get(i)}</text
					>
				{/if}
			{/each}
			{#if peak}
				<Text
					x={(context.xScale(peak.key) ?? 0) + (context.xScale.bandwidth?.() ?? 0) / 2}
					y={context.yScale(peak.value) - 7}
					value="Peak"
					textAnchor="middle"
					class="fill-foreground text-2xs font-semibold"
				/>
			{/if}
		{/snippet}

		{#snippet tooltip()}
			<Tooltip.Root variant="none" anchor="bottom" y="data" yOffset={10} contained="container">
				{#snippet children({ data })}
					{@const d = data as Row}
					<div
						class="grid min-w-44 gap-1 rounded-lg border bg-popover px-3 py-2.5 text-xs shadow-[0_8px_24px_-8px_rgb(0_0_0/0.25)]"
					>
						<p class="text-muted-foreground">{bucketLabel(d.key, bucket)}</p>
						<p class="tnum text-sm font-semibold">{fmt(d.value)}</p>
						<p class="tnum text-muted-foreground">
							{#if metric === 'time'}{int(d.plays)} plays, {int(d.artists)}
								{d.artists === 1 ? 'artist' : 'artists'}{:else if metric === 'plays'}{duration(
									d.ms
								)}, {int(d.artists)}
								{d.artists === 1 ? 'artist' : 'artists'}{:else}{int(d.plays)} plays, {duration(
									d.ms
								)}{/if}, {int(d.skips)} skipped
						</p>
						{#if d.prev !== null && d.prevKey}
							<p class="tnum flex items-center gap-1.5 text-muted-foreground">
								<span class="inline-block size-2.5 rounded-[2px] bg-grille"></span>
								{fmt(d.prev)} in {bucketLabel(d.prevKey, bucket)}
							</p>
						{/if}
						{#if d.est > 0}
							<p class="flex items-center gap-1.5 text-muted-foreground">
								<span
									class="inline-block size-2.5 rounded-[2px]"
									style="background: repeating-linear-gradient(0deg, var(--signal-dim) 0 3px, transparent 3px 4.5px)"
								></span>
								{duration(d.est)} estimated
							</p>
						{/if}
					</div>
				{/snippet}
			</Tooltip.Root>
		{/snippet}
	</Chart>
</ChartUI.Container>
