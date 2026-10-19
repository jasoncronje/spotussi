<!--
	Recent imports, newest first: uploads and `spotussi import` runs alike.
	Each says how it went, what it added and when it ran; one still running
	shows its progress.
-->
<script lang="ts">
	import Lamp from '#lib/components/shell/lamp.svelte';
	import { importProblem, type ImportJob } from '#lib/data/imports.js';
	import { ago, fullDateTime, int, plural } from '#lib/format.js';

	let { jobs, limit = 8 }: { jobs: ImportJob[]; limit?: number } = $props();

	let all = $state(false);
	const shown = $derived(all ? jobs : jobs.slice(0, limit));

	const STATE = {
		queued: 'Queued',
		running: 'Importing',
		done: 'Done',
		failed: 'Failed'
	} as const;

	function took(j: ImportJob) {
		if (!j.finished_at) return '';
		const s = Math.max(0, Math.round((j.finished_at - j.started_at) / 1000));
		return s < 60 ? `${s} s` : `${Math.floor(s / 60)} min ${s % 60} s`;
	}
</script>

<ul class="divide-y divide-border/60 rounded-lg border">
	{#each shown as j (j.id)}
		{@const when = j.finished_at ?? j.started_at}
		<li class="grid gap-1.5 px-3 py-2.5">
			<div class="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5">
				<p class="min-w-0 truncate font-mono text-xs">{j.filename}</p>
				<p
					class="flex items-center gap-2 text-xs {j.state === 'failed'
						? 'font-medium text-destructive'
						: j.state === 'done'
							? 'text-muted-foreground'
							: 'font-medium'}"
				>
					{#if j.state === 'running' || j.state === 'queued'}<Lamp
							on={j.state === 'running'}
						/>{/if}{STATE[j.state]}
				</p>
			</div>

			{#if j.state === 'running' || j.state === 'queued'}
				{@const frac = j.bytes_total ? Math.min(1, j.bytes_done / j.bytes_total) : null}
				<div
					class="h-[5px] w-full rounded-full bg-grille/70"
					role="progressbar"
					aria-label="{j.filename} import"
					aria-valuemin={0}
					aria-valuemax={100}
					aria-valuenow={frac === null ? undefined : Math.round(frac * 100)}
				>
					<div
						class="h-full rounded-full bg-signal transition-[width] duration-500"
						style="width: {(frac ?? 0) * 100}%"
					></div>
				</div>
			{/if}

			{#if j.state === 'failed'}
				<p class="text-xs text-destructive">{importProblem(j.error_code, j.error).title}.</p>
			{/if}
			<p class="tnum text-xs text-muted-foreground">
				{#if j.state === 'queued'}
					Waiting to start.
				{:else if j.state === 'running'}
					{plural(j.rows_done, 'stream')} read{#if j.files > 1}, history file {Math.min(
							j.files_done + 1,
							j.files
						)} of {j.files}{/if}.
				{:else if j.state === 'failed'}
					{j.rows_new > 0 ? `${plural(j.rows_new, 'stream')} kept.` : 'Nothing was imported.'}
				{:else if j.rows_new === 0 && j.rows_duplicate > 0}
					All {int(j.rows_duplicate)} streams already imported.
				{:else}
					{plural(j.rows_new, 'stream')} added{#if j.rows_duplicate > 0}, {int(j.rows_duplicate)}
						already imported{/if}{#if j.files > 0}, from {plural(j.files, 'file')}{/if}.
				{/if}
			</p>
			<p class="text-xs text-muted-foreground">
				{j.origin === 'cli' ? 'From spotussi import' : 'Uploaded'},
				<time datetime={new Date(when).toISOString()} title={fullDateTime(when)}
					>{j.finished_at ? 'finished' : 'started'} {ago(when)}</time
				>{#if j.state === 'done' && took(j)}, took {took(j)}{/if}
			</p>
		</li>
	{/each}
</ul>
{#if jobs.length > limit}
	<button
		type="button"
		onclick={() => (all = !all)}
		aria-expanded={all}
		class="hit w-fit text-sm text-muted-foreground underline underline-offset-2 hover:text-foreground"
		>{all ? 'Show fewer' : `Show all ${jobs.length}`}</button
	>
{/if}
