<!--
	Upload an Extended streaming history export (D10, D23): drop the zip (or
	Streaming_History JSON files) or pick it, watch the bytes go up, then the
	server's import read through it, then what it added. Every state reads
	from the shared upload (state/import-upload), so leaving the page doesn't
	stop anything and Settings and the setup guide show the same upload.
-->
<script lang="ts">
	import { tick } from 'svelte';
	import { resolve } from '$app/paths';
	import FileArchive from '@lucide/svelte/icons/file-archive';
	import ExternalLink from '@lucide/svelte/icons/external-link';
	import Lamp from '#lib/components/shell/lamp.svelte';
	import CopyButton from '#lib/components/common/copy-button.svelte';
	import SegmentMeter from './segment-meter.svelte';
	import { ACCEPT, type ImportJob } from '#lib/data/imports.js';
	import { importUpload as up, RETRYABLE } from '#lib/state/import-upload.svelte.js';
	import { bytes, int, plural } from '#lib/format.js';

	let {
		cliHint = true,
		historyLink = true,
		surface = 'inset'
	}: {
		/** Mention `spotussi import` under the drop zone. */
		cliHint?: boolean;
		/** Offer "See it in History" when done (off where the page has its own next step). */
		historyLink?: boolean;
		/** `inset` inside a panel (recessed), `raised` on the page background. */
		surface?: 'inset' | 'raised';
	} = $props();
	const bg = $derived(surface === 'inset' ? 'bg-background' : 'bg-card');

	const uid = $props.id();
	const CLI = 'spotussi import my_spotify_data.zip';
	const historyHref = `${resolve('/history')}?range=all`;

	let root = $state<HTMLElement>();
	let input = $state<HTMLInputElement>();
	let dragDepth = $state(0);
	const dragging = $derived(dragDepth > 0);

	const phase = $derived(up.phase);
	const name = $derived(
		up.files.length > 1 && phase.kind === 'done'
			? `${up.files.length} files`
			: (up.file?.name ?? '')
	);
	const batch = $derived(up.files.length > 1 ? `, file ${up.index + 1} of ${up.files.length}` : '');

	function pick(list: FileList | null | undefined) {
		const files = [...(list ?? [])];
		if (files.length && !up.busy) up.start(files);
		if (input) input.value = '';
	}

	function hasFiles(e: DragEvent) {
		return [...(e.dataTransfer?.types ?? [])].includes('Files');
	}

	// What a screen reader hears: once per change of state, never per tick.
	const announcement = $derived.by(() => {
		switch (phase.kind) {
			case 'idle':
				return up.cancelled ? 'Upload cancelled. Nothing was imported.' : '';
			case 'uploading':
				return `Uploading ${up.file?.name}${batch}.`;
			case 'processing':
				return phase.job.state === 'queued'
					? `Upload finished. ${up.file?.name} is queued for import.`
					: `Importing ${up.file?.name}.`;
			case 'done': {
				const t = up.totals;
				return t.added === 0
					? 'Import finished. Everything in it was already imported.'
					: `Import finished. ${plural(t.added, 'stream')} added, ${int(t.duplicate)} already imported.`;
			}
			case 'failed':
				return `Import failed. ${phase.problem.title}.`;
		}
	});

	// Keep keyboard focus somewhere useful as controls come and go, but only
	// when it was inside this control (or dropped to the page); never pull
	// focus from wherever the person has moved on to.
	let last: string | null = null;
	$effect(() => {
		const kind = phase.kind;
		const first = last === null;
		if (kind === last) return;
		last = kind;
		if (first) return;
		const active = document.activeElement;
		if (active && active !== document.body && !root?.contains(active)) return;
		tick().then(() => root?.querySelector<HTMLElement>('[data-focus]')?.focus());
	});

	const pct = (a: number, b: number | null) => (b ? Math.min(1, a / b) : null);

	/** "History file 3 of 15, 62 MB of 235 MB read" */
	function readLine(j: ImportJob) {
		const file =
			j.files > 1 ? `History file ${Math.min(j.files_done + 1, j.files)} of ${j.files}, ` : '';
		return j.bytes_total
			? `${file}${bytes(j.bytes_done)} of ${bytes(j.bytes_total)} read`
			: 'Opening the export';
	}
</script>

<svelte:window
	ondragover={(e) => hasFiles(e) && e.preventDefault()}
	ondrop={(e) => hasFiles(e) && e.preventDefault()}
	onbeforeunload={(e) => {
		if (phase.kind === 'uploading') e.preventDefault();
	}}
/>

<div bind:this={root} class="grid gap-3">
	<p role="status" class="sr-only">{announcement}</p>

	{#if phase.kind === 'idle'}
		<label
			for="{uid}-file"
			ondragenter={(e) => {
				if (!hasFiles(e)) return;
				e.preventDefault();
				dragDepth++;
			}}
			ondragover={(e) => hasFiles(e) && e.preventDefault()}
			ondragleave={() => (dragDepth = Math.max(0, dragDepth - 1))}
			ondrop={(e) => {
				e.preventDefault();
				dragDepth = 0;
				pick(e.dataTransfer?.files);
			}}
			class="flex cursor-pointer flex-wrap items-center gap-x-4 gap-y-3 rounded-xl border p-4 outline-offset-2 outline-ring transition-colors has-[:focus-visible]:outline-2 sm:px-5 {dragging
				? 'border-signal bg-signal-soft'
				: `${bg} hover:border-foreground/25`}"
		>
			<span
				class="grid size-11 shrink-0 place-items-center rounded-lg border bg-card transition-colors {dragging
					? 'border-signal text-signal-ink'
					: 'text-muted-foreground'}"
			>
				<FileArchive class="size-5" aria-hidden="true" />
			</span>
			<span class="grid min-w-0 flex-1 basis-56 gap-0.5">
				<span class="flex items-center gap-2 text-sm font-medium">
					<span class="pointer-coarse:hidden"
						>{dragging ? 'Drop to import' : 'Drop your export here'}</span
					>
					<span class="hidden pointer-coarse:inline">Choose your export</span>
				</span>
				<span id="{uid}-hint" class="text-xs text-muted-foreground">
					<span class="font-mono">my_spotify_data.zip</span> or its Streaming_History_Audio files
				</span>
			</span>
			<span
				aria-hidden="true"
				class="inline-flex h-9 items-center rounded-lg bg-primary px-3.5 text-sm font-medium text-primary-foreground max-sm:w-full max-sm:justify-center"
				>Choose file</span
			>
			<input
				bind:this={input}
				id="{uid}-file"
				data-focus
				type="file"
				accept={ACCEPT}
				multiple
				aria-label="Choose your Spotify export"
				aria-describedby="{uid}-hint"
				class="sr-only"
				onchange={(e) => pick(e.currentTarget.files)}
			/>
		</label>
		{#if up.cancelled}
			<p class="text-sm text-muted-foreground">Upload cancelled. Nothing was imported.</p>
		{/if}
		{#if cliHint}
			<p class="max-w-[65ch] text-xs text-muted-foreground">
				Large export? Run
				<code class="font-mono whitespace-nowrap text-foreground">spotussi import &lt;file&gt;</code
				> on the server.
			</p>
		{/if}
	{:else}
		<div class="grid gap-4 rounded-xl border {bg} p-4 sm:px-5">
			<div class="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
				<div class="grid min-w-0 gap-0.5">
					<p class="flex items-center gap-2 text-sm font-medium">
						{#if phase.kind === 'uploading'}
							<Lamp on />Uploading
						{:else if phase.kind === 'processing'}
							<Lamp on={phase.job.state === 'running'} />{phase.job.state === 'queued'
								? 'Queued'
								: 'Importing'}
						{:else if phase.kind === 'done'}
							<Lamp on />{up.totals.added === 0 ? 'Already imported' : 'Imported'}
						{:else}
							<span
								aria-hidden="true"
								class="inline-block size-[5px] shrink-0 rounded-full bg-destructive"
							></span>
							<span class="text-destructive">{phase.problem.title}</span>
						{/if}
					</p>
					<p class="truncate font-mono text-xs text-muted-foreground">
						{name}{#if phase.kind !== 'done' && batch}<span class="font-sans">{batch}</span>{/if}
					</p>
				</div>
				{#if phase.kind === 'uploading'}
					<button
						type="button"
						data-focus
						onclick={() => up.cancel()}
						class="h-8 rounded-lg border bg-card px-3 text-sm font-medium transition-colors hover:bg-muted"
						>Cancel upload</button
					>
				{/if}
			</div>

			{#if phase.kind === 'uploading'}
				<div class="grid gap-2">
					<SegmentMeter
						value={pct(phase.sent, phase.total)}
						label="Upload"
						valuetext="{bytes(phase.sent)} of {bytes(phase.total)} sent"
					/>
					<p class="flex justify-between gap-3 text-xs text-muted-foreground">
						<span class="tnum">{bytes(phase.sent)} of {bytes(phase.total)}</span>
						<span class="tnum font-medium text-foreground"
							>{Math.floor((pct(phase.sent, phase.total) ?? 0) * 100)}%</span
						>
					</p>
				</div>
				<p class="text-xs text-muted-foreground">Keep this tab open.</p>
			{:else if phase.kind === 'processing'}
				{@const j = phase.job}
				<div class="grid gap-2">
					<SegmentMeter
						value={j.state === 'queued' ? null : pct(j.bytes_done, j.bytes_total)}
						label="Import"
						valuetext={j.bytes_total
							? `${bytes(j.bytes_done)} of ${bytes(j.bytes_total)} read`
							: undefined}
					/>
					<p class="flex flex-wrap justify-between gap-x-3 text-xs text-muted-foreground">
						{#if j.state === 'queued'}
							<span>Waiting for the server</span>
						{:else}
							<span class="tnum">{readLine(j)}</span>
							<span class="tnum font-medium text-foreground"
								>{Math.floor((pct(j.bytes_done, j.bytes_total) ?? 0) * 100)}%</span
							>
						{/if}
					</p>
				</div>
				{@render readout(j.rows_done, j.rows_new, j.rows_duplicate)}
				<p class="text-xs text-muted-foreground">You can leave this page.</p>
			{:else if phase.kind === 'done'}
				{@const t = up.totals}
				{@render readout(t.read, t.added, t.duplicate)}
				<div class="flex flex-wrap gap-2">
					{#if historyLink}
						<a
							href={historyHref}
							data-focus
							class="inline-flex h-9 items-center rounded-lg bg-primary px-3.5 text-sm font-medium text-primary-foreground"
							>See it in History</a
						>
					{/if}
					<button
						type="button"
						data-focus={historyLink ? undefined : true}
						onclick={() => up.reset()}
						class="h-9 rounded-lg border bg-card px-3.5 text-sm font-medium transition-colors hover:bg-muted"
						>Import another file</button
					>
				</div>
			{:else if phase.kind === 'failed'}
				{@const p = phase.problem}
				{@const kept = up.totals.added + (phase.job?.rows_new ?? 0)}
				<div class="grid gap-2 text-sm">
					<p class="max-w-[65ch]">{p.body}</p>
					{#if p.cli}
						<div class="flex flex-wrap items-center gap-2 rounded-lg border bg-card p-2 pl-3">
							<code class="min-w-0 flex-1 font-mono text-sm break-all select-all">{CLI}</code>
							<CopyButton value={CLI} label="Copy command" />
						</div>
					{/if}
					{#if p.privacyLink}
						<p>
							<a
								href="https://www.spotify.com/account/privacy/"
								target="_blank"
								rel="noopener noreferrer"
								class="inline-flex items-center gap-1 font-medium underline decoration-signal underline-offset-4"
								>Spotify privacy settings<ExternalLink class="size-3.5" aria-hidden="true" /><span
									class="sr-only"
								>
									(opens in a new tab)</span
								></a
							>
						</p>
					{/if}
					{#if kept > 0}
						<p class="text-muted-foreground">
							{plural(kept, 'stream')} imported before this {kept === 1 ? 'was' : 'were'} kept.
						</p>
					{/if}
				</div>
				<div class="flex flex-wrap gap-2">
					{#if RETRYABLE.includes(phase.code)}
						<button
							type="button"
							data-focus
							onclick={() => up.retry()}
							class="h-9 rounded-lg bg-primary px-3.5 text-sm font-medium text-primary-foreground"
							>Upload again</button
						>
					{/if}
					<button
						type="button"
						data-focus={RETRYABLE.includes(phase.code) ? undefined : true}
						onclick={() => up.reset()}
						class="h-9 rounded-lg border bg-card px-3.5 text-sm font-medium transition-colors hover:bg-muted"
						>Choose another file</button
					>
				</div>
			{/if}
		</div>
	{/if}
</div>

{#snippet readout(read: number, added: number, duplicate: number)}
	<!-- dt/dd straight in the grid: a label that wraps on a phone pushes the
	     whole label row down, so the three figures stay on one line. -->
	<dl class="grid grid-flow-col grid-cols-3 grid-rows-[auto_auto] gap-x-4 gap-y-0.5">
		{#each [{ label: 'Streams read', value: read }, { label: 'Added', value: added }, { label: 'Already imported', value: duplicate }] as r (r.label)}
			<dt class="self-end text-xs text-muted-foreground">{r.label}</dt>
			<dd class="figure tnum min-w-0 text-lg">{int(r.value)}</dd>
		{/each}
	</dl>
{/snippet}
