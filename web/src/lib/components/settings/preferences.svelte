<!--
	Instance settings: how plays are counted (D7), sessions, time zone and
	calendar, plus the per-browser theme (D22). Changes that rebuild derived
	data (rule, threshold, time zone, session gap) are saved together with an
	explicit button and reload every view afterwards; display-only ones
	(date format, week start, theme) apply at once.
-->
<script lang="ts">
	import Segmented from '#lib/components/common/segmented.svelte';
	import Hint from '#lib/components/common/hint.svelte';
	import SettingsPanel from './settings-panel.svelte';
	import { api, type Schemas } from '#lib/api/client.js';
	import { unwrap } from '#lib/data/queries.js';
	import { refreshAll } from '#lib/data/resource.svelte.js';
	import { invalidate } from '#lib/data/cache.js';
	import { applySettings, prefs } from '#lib/state/prefs.svelte.js';
	import { theme, type Theme } from '#lib/state/theme.svelte.js';

	let { settings, onchange }: { settings: Schemas['Settings']; onchange: () => void } = $props();

	// ---------- counting ----------
	let rule = $state<Schemas['PlayRule']>('fixed');
	let thresholdS = $state(30);
	let gap = $state(15);
	let tz = $state('UTC');
	let synced = $state('');
	$effect(() => {
		// Re-seed the draft whenever the server copy changes.
		const key = JSON.stringify(settings);
		if (key === synced) return;
		synced = key;
		rule = settings.play_rule;
		thresholdS = Math.round(settings.play_rule_threshold_ms / 1000);
		gap = settings.session_gap_minutes;
		tz = settings.timezone;
	});

	const dirty = $derived(
		rule !== settings.play_rule ||
			(rule === 'fixed' && thresholdS * 1000 !== settings.play_rule_threshold_ms) ||
			gap !== settings.session_gap_minutes ||
			tz !== settings.timezone
	);
	const valid = $derived(
		Number.isInteger(thresholdS) &&
			thresholdS >= 1 &&
			thresholdS <= 600 &&
			Number.isInteger(gap) &&
			gap >= 1 &&
			gap <= 240
	);
	let saving = $state(false);
	let message = $state('');
	let error = $state('');

	async function patch(body: Schemas['SettingsPatch']) {
		return unwrap(api.PATCH('/settings', { body }));
	}

	async function save() {
		if (!valid) return;
		saving = true;
		error = '';
		message = '';
		const body: Schemas['SettingsPatch'] = {};
		if (rule !== settings.play_rule) body.play_rule = rule;
		if (rule === 'fixed' && thresholdS * 1000 !== settings.play_rule_threshold_ms)
			body.play_rule_threshold_ms = thresholdS * 1000;
		if (gap !== settings.session_gap_minutes) body.session_gap_minutes = gap;
		if (tz !== settings.timezone) body.timezone = tz;
		try {
			const s = await patch(body);
			applySettings(s);
			refreshAll();
			onchange();
			message = 'Saved, recalculating.';
		} catch (e) {
			error = (e as Error).message;
		} finally {
			saving = false;
		}
	}
	function reset() {
		synced = '';
	}

	// ---------- display (instant) ----------
	let displayError = $state('');
	async function setDisplay(body: Schemas['SettingsPatch']) {
		displayError = '';
		const before = { date_format: prefs.dateFormat, week_start: prefs.weekStart };
		applySettings({ ...before, ...body } as Schemas['Settings']);
		try {
			const s = await patch(body);
			applySettings(s);
			// Week start changes week buckets; everything else is formatting.
			if (body.week_start) refreshAll();
			else invalidate('/settings');
			onchange();
		} catch (e) {
			applySettings(before);
			displayError = (e as Error).message;
		}
	}

	const browserTz = Intl.DateTimeFormat().resolvedOptions().timeZone;
	const zones = (() => {
		try {
			return (Intl as unknown as { supportedValuesOf(k: string): string[] }).supportedValuesOf(
				'timeZone'
			);
		} catch {
			return [browserTz, 'UTC'];
		}
	})();
	const zoneList = $derived(zones.includes(tz) ? zones : [tz, ...zones]);

	const sample = new Date(2026, 9, 17);
	const FORMATS = [
		{ value: 'auto' as const, label: 'Browser', hint: sample.toLocaleDateString() },
		{ value: 'dmy' as const, label: '17/10', hint: '17 Oct 2026' },
		{ value: 'mdy' as const, label: '10/17', hint: 'Oct 17, 2026' },
		{ value: 'ymd' as const, label: 'ISO', hint: '2026-10-17' }
	];

	const field =
		'h-9 rounded-lg border bg-background px-3 text-base outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/40 sm:text-sm';
</script>

<SettingsPanel id="counting" title="Counting and Time">
	<div class="grid gap-6">
		<fieldset class="grid gap-2">
			<legend class="mb-1 flex items-center gap-1.5 text-sm font-medium">
				Play Rule <Hint text="What counts as a play. Time listened counts every second." />
			</legend>
			<label
				class="flex cursor-pointer gap-3 rounded-lg border p-3 has-[:checked]:border-signal/60 has-[:checked]:bg-signal-soft"
			>
				<input
					type="radio"
					name="rule"
					value="fixed"
					bind:group={rule}
					class="mt-1 accent-[var(--signal)]"
				/>
				<span class="min-w-0">
					<span class="block text-sm font-medium"
						>Fixed Threshold <span class="font-normal text-muted-foreground">(default)</span></span
					>
					{#if rule === 'fixed'}
						<span class="mt-2 flex items-center gap-2 text-sm">
							<input
								type="number"
								min="1"
								max="600"
								step="1"
								bind:value={thresholdS}
								aria-label="Threshold in seconds"
								class="{field} w-24"
							/>
							seconds
						</span>
					{/if}
				</span>
			</label>
			<label
				class="flex cursor-pointer gap-3 rounded-lg border p-3 has-[:checked]:border-signal/60 has-[:checked]:bg-signal-soft"
			>
				<input
					type="radio"
					name="rule"
					value="half_track"
					bind:group={rule}
					class="mt-1 accent-[var(--signal)]"
				/>
				<span>
					<span class="block text-sm font-medium">Half the Track</span>
					<span class="block text-sm text-muted-foreground">
						At most 4 minutes. Tracks under 30 s never count.
					</span>
				</span>
			</label>
		</fieldset>

		<div class="grid gap-6 sm:grid-cols-2">
			<label class="grid content-start gap-1.5 text-sm">
				<span class="flex items-center gap-1.5 font-medium"
					>Session Gap <Hint text="A pause this long starts a new session." /></span
				>
				<span class="flex items-center gap-2">
					<input
						type="number"
						min="1"
						max="240"
						step="1"
						bind:value={gap}
						aria-label="Session gap in minutes"
						class="{field} w-24"
					/>
					minutes
				</span>
			</label>
			<label class="grid content-start gap-1.5 text-sm">
				<span class="font-medium">Time Zone</span>
				<select bind:value={tz} class="{field} w-full">
					{#each zoneList as z (z)}<option value={z}>{z.replaceAll('_', ' ')}</option>{/each}
				</select>
				{#if tz !== browserTz}
					<span class="text-xs text-muted-foreground">
						<button
							type="button"
							class="hit underline underline-offset-2 hover:text-foreground"
							onclick={() => (tz = browserTz)}>Use {browserTz.replaceAll('_', ' ')}</button
						>
					</span>
				{/if}
			</label>
		</div>

		{#if !valid}
			<p role="alert" class="text-sm text-destructive">
				Use whole numbers: 1–600 seconds, 1–240 minutes.
			</p>
		{/if}
		{#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
		{#if message && !dirty}<p role="status" class="text-sm text-muted-foreground">{message}</p>{/if}
		<div class="flex flex-wrap items-center gap-2">
			<button
				type="button"
				disabled={!dirty || !valid || saving}
				onclick={save}
				class="h-9 rounded-lg bg-primary px-3.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
				>{saving ? 'Saving…' : 'Save and recalculate'}</button
			>
			{#if dirty}
				<button
					type="button"
					onclick={reset}
					class="h-9 rounded-lg px-3 text-sm text-muted-foreground hover:bg-muted hover:text-foreground"
					>Discard</button
				>
			{/if}
		</div>
	</div>
</SettingsPanel>

<SettingsPanel id="display" title="Display">
	<div class="grid gap-5">
		<div class="grid gap-1.5">
			<span class="text-sm font-medium" id="fmt-l">Date Format</span>
			<Segmented
				label="Date format"
				size="md"
				value={prefs.dateFormat}
				onchange={(v) => setDisplay({ date_format: v })}
				options={FORMATS}
				class="w-fit max-w-full overflow-x-auto"
			/>
			<span class="tnum text-xs text-muted-foreground">
				{FORMATS.find((f) => f.value === prefs.dateFormat)?.hint}
			</span>
		</div>
		<div class="grid gap-1.5">
			<span class="text-sm font-medium">Weeks Start On</span>
			<Segmented
				label="Weeks start on"
				size="md"
				value={prefs.weekStart}
				onchange={(v) => setDisplay({ week_start: v })}
				options={[
					{ value: 'monday', label: 'Monday' },
					{ value: 'sunday', label: 'Sunday' }
				]}
				class="w-fit"
			/>
		</div>
		<div class="grid gap-1.5">
			<span class="text-sm font-medium"
				>Theme <span class="font-normal text-muted-foreground">(this browser)</span></span
			>
			<Segmented
				label="Theme"
				size="md"
				value={theme.choice}
				onchange={(v: Theme) => theme.set(v)}
				options={[
					{ value: 'system', label: 'System' },
					{ value: 'light', label: 'Light' },
					{ value: 'dark', label: 'Dark' }
				]}
				class="w-fit"
			/>
		</div>
		{#if displayError}<p role="alert" class="text-sm text-destructive">{displayError}</p>{/if}
	</div>
</SettingsPanel>
