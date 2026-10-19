<!--
	The setup guide (D12, D13), after the owner password: create a Spotify
	developer app, paste its Client ID, connect the account (direct callback
	over HTTPS, paste-back over plain HTTP), confirm the time zone, then
	request and import the extended history export. The step lives in the
	URL so the direct OAuth callback returns to the right place and Settings
	can link to one step. The tuner scale above marks progress; its lamp
	lights once Spotify is connected.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import ExternalLink from '@lucide/svelte/icons/external-link';
	import Check from '@lucide/svelte/icons/check';
	import AuthFrame from '#lib/components/auth/auth-frame.svelte';
	import Progress from '#lib/components/onboarding/progress.svelte';
	import {
		ONBOARDING,
		type GuideStep,
		type OnboardingStep
	} from '#lib/components/onboarding/steps.js';
	import { field, primary, quiet, secondary } from '#lib/components/onboarding/ui.js';
	import CopyButton from '#lib/components/common/copy-button.svelte';
	import SectionError from '#lib/components/common/section-error.svelte';
	import Lamp from '#lib/components/shell/lamp.svelte';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import ImportUpload from '#lib/components/import/import-upload.svelte';
	import { importUpload } from '#lib/state/import-upload.svelte.js';
	import { api } from '#lib/api/client.js';
	import { Resource, refreshAll } from '#lib/data/resource.svelte.js';
	import { connectionQuery, nowPlayingQuery, settingsQuery, unwrap } from '#lib/data/queries.js';
	import { applySettings } from '#lib/state/prefs.svelte.js';
	import { session } from '#lib/state/session.svelte.js';
	import { settingsHref } from '#lib/components/settings/tabs.js';

	const GUIDE = ONBOARDING.filter((s) => s.id !== 'owner') as {
		id: GuideStep;
		label: string;
	}[];
	const TITLES: Record<GuideStep, string> = {
		app: 'Create a Spotify app',
		client: 'Paste the Client ID',
		connect: 'Connect Spotify',
		timezone: 'Your time zone',
		history: 'Bring in your past'
	};

	const conn = new Resource(() => connectionQuery());
	const settings = new Resource(() => settingsQuery());
	const c = $derived(conn.current);
	// The port Spotify must see, if the redirect URI has one (D21).
	const redirectPort = $derived.by(() => {
		try {
			return c ? new URL(c.callback.redirect_uri).port : '';
		} catch {
			return '';
		}
	});

	const step = $derived.by<GuideStep>(() => {
		const s = page.url.searchParams.get('step');
		return GUIDE.find((x) => x.id === s)?.id ?? 'app';
	});
	const order = (s: OnboardingStep) => ONBOARDING.findIndex((x) => x.id === s);
	const href = (s: GuideStep) => `${resolve('/welcome')}?step=${s}`;
	function go(s: GuideStep) {
		// eslint-disable-next-line svelte/no-navigation-without-resolve
		return goto(href(s));
	}

	const connected = $derived(c?.state === 'connected');
	// Granted later than the rest (D36); older connections lack it.
	const REQUIRED_SCOPES = ['user-library-read'];

	// Reconnect (M8): the shell's banner links here with ?reconnect=1 and the
	// page you were on; once connected again you go straight back to it.
	const reconnect = $derived(page.url.searchParams.get('reconnect') === '1');
	const next = $derived.by(() => {
		const n = page.url.searchParams.get('next') ?? '';
		return n.startsWith('/') && !n.startsWith('//') && !n.startsWith('/\\') ? n : resolve('/');
	});
	const connectReturn = $derived(
		reconnect ? `${href('connect')}&reconnect=1&next=${encodeURIComponent(next)}` : href('connect')
	);
	// A reconnect always offers consent again, even while connected: that's
	// how an account picks up a new permission (user-library-read, D36). The
	// connected panel shows once Spotify sends you back, or the paste-back lands.
	const returned = $derived(page.url.searchParams.get('spotify'));
	let pastedBack = $state(false);
	const showConnected = $derived(
		connected && (!reconnect || returned === 'connected' || pastedBack)
	);
	const missingScopes = $derived(
		connected ? REQUIRED_SCOPES.filter((s) => !c?.scopes.includes(s)) : []
	);
	let cleared = false;
	$effect(() => {
		// The status cached before the reconnect still says it's needed.
		if (reconnect && showConnected && !cleared) {
			cleared = true;
			refreshAll();
		}
	});
	const done = $derived<Partial<Record<OnboardingStep, boolean>>>({
		owner: true,
		app: Boolean(c?.client_id) || order(step) > order('app'),
		client: Boolean(c?.client_id),
		connect: connected,
		timezone: order(step) > order('timezone'),
		history: importUpload.phase.kind === 'done'
	});
	const ownerLabel = $derived(session.current?.mode === 'password' ? 'Password' : 'Access');

	// First play: once connected, watch the player until something plays.
	const player = new Resource(() => (connected ? nowPlayingQuery() : null));
	const heard = $derived(player.current?.track ? player.current : null);
	onMount(() => {
		const id = setInterval(() => {
			if (connected && document.visibilityState === 'visible') player.refresh();
		}, 5000);
		return () => clearInterval(id);
	});

	// ---------- client ID ----------
	let clientId = $state('');
	let clientSeeded = false;
	$effect(() => {
		if (!clientSeeded && c) {
			clientSeeded = true;
			clientId = c.client_id ?? '';
		}
	});
	let clientError = $state('');
	let savingClient = $state(false);
	async function saveClient(e: SubmitEvent) {
		e.preventDefault();
		const v = clientId.trim();
		if (!/^[0-9A-Za-z]{32}$/.test(v)) {
			clientError =
				'A Client ID is 32 letters and digits. Copy it from Basic Information, not the client secret.';
			return;
		}
		savingClient = true;
		clientError = '';
		try {
			if (v !== c?.client_id) {
				await unwrap(api.PUT('/spotify/client-id', { body: { client_id: v } }));
				conn.refresh();
			}
			await go('connect');
		} catch (err) {
			clientError = (err as Error).message;
		} finally {
			savingClient = false;
		}
	}

	// ---------- connect ----------
	function errorText(code: string | undefined | null): string | undefined {
		switch (code) {
			case 'invalid_callback_url':
				return `That isn’t the address Spotify sent you to. Copy the whole address from the tab with the connection error; it starts with ${c?.callback.redirect_uri ?? 'the redirect URI'}.`;
			case 'invalid_state':
				return 'That approval expired or was already used. Open Spotify again.';
			case 'spotify_denied':
				return 'Access wasn’t approved. Open Spotify again and choose Agree.';
			case 'spotify_error':
				return 'Spotify refused the connection. Check the redirect URI matches exactly. Signed in with another Spotify account? Add it under User Management in your app.';
			case 'client_id_required':
				return 'Add the Client ID first.';
		}
	}
	const returnedError = $derived(
		returned === 'error'
			? (errorText(page.url.searchParams.get('spotify_error')) ??
					'Connecting didn’t finish. Try again.')
			: ''
	);
	let authorizeUrl = $state<string | null>(null);
	let pasted = $state('');
	let connectError = $state('');
	let connecting = $state(false);
	async function startConnect() {
		connecting = true;
		connectError = '';
		try {
			const r = await unwrap(
				api.POST('/spotify/authorize', { body: { return_to: connectReturn } })
			);
			if (r.callback.mode === 'direct') {
				window.location.assign(r.authorize_url);
				return;
			}
			authorizeUrl = r.authorize_url;
			window.open(r.authorize_url, '_blank', 'noopener');
		} catch (err) {
			connectError = errorText((err as { code?: string }).code) ?? (err as Error).message;
		} finally {
			connecting = false;
		}
	}
	async function finishPaste(e: SubmitEvent) {
		e.preventDefault();
		const url = pasted.trim();
		if (!url) {
			connectError = 'Paste the full address from the tab Spotify sent you to.';
			return;
		}
		connecting = true;
		connectError = '';
		try {
			await unwrap(api.POST('/spotify/paste-back', { body: { url } }));
			pasted = '';
			authorizeUrl = null;
			pastedBack = true;
			conn.refresh();
		} catch (err) {
			connectError = errorText((err as { code?: string }).code) ?? (err as Error).message;
		} finally {
			connecting = false;
		}
	}

	// ---------- time zone ----------
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
	let tz = $state(browserTz);
	const zoneList = $derived(zones.includes(tz) ? zones : [tz, ...zones]);
	let tzError = $state('');
	let savingTz = $state(false);
	async function saveTz(e: SubmitEvent) {
		e.preventDefault();
		savingTz = true;
		tzError = '';
		try {
			if (tz !== settings.current?.timezone) {
				const s = await unwrap(api.PATCH('/settings', { body: { timezone: tz } }));
				applySettings(s);
				refreshAll();
			}
			await go('history');
		} catch (err) {
			tzError = (err as Error).message;
		} finally {
			savingTz = false;
		}
	}
	const zoneParts = $derived(tz.replaceAll('_', ' ').split('/'));
	const localTime = $derived.by(() => {
		try {
			return new Intl.DateTimeFormat(undefined, {
				hour: 'numeric',
				minute: '2-digit',
				timeZone: tz
			}).format(new Date());
		} catch {
			return '';
		}
	});
</script>

<svelte:head
	><title>Set up, {GUIDE.find((s) => s.id === step)?.label} – spotussi</title></svelte:head
>

{#snippet external(url: string, label: string)}
	<a
		href={url}
		target="_blank"
		rel="noopener noreferrer"
		class="inline-flex items-center gap-1 font-medium underline decoration-signal decoration-2 underline-offset-4 hover:decoration-foreground"
		>{label}<ExternalLink class="size-3.5" aria-hidden="true" /><span class="sr-only">
			(opens in a new tab)</span
		></a
	>
{/snippet}

{#snippet code(value: string, label: string)}
	<div
		class="flex flex-wrap items-center justify-end gap-2 rounded-xl border bg-card p-1.5 pl-3.5 max-sm:py-3"
	>
		<code class="min-w-0 flex-1 font-mono text-sm break-all select-all max-sm:basis-full"
			>{value}</code
		>
		<CopyButton {value} {label} />
	</div>
{/snippet}

{#snippet n(i: number)}
	<span
		class="figure tnum grid size-7 shrink-0 place-items-center rounded-full border bg-card text-xs"
		aria-hidden="true">{i}</span
	>
{/snippet}

<AuthFrame width="wide">
	{#if !reconnect}
		<Progress
			current={step}
			{done}
			tuned={connected}
			{ownerLabel}
			href={(s) => (s === 'owner' ? null : href(s as GuideStep))}
		/>
	{/if}

	<div class="mt-10 sm:mt-14">
		{#if conn.error && !c}
			<SectionError what="the Spotify connection" error={conn.error} retry={conn.retry} />
		{:else if !c}
			<Skeleton class="h-11 w-2/3" /><Skeleton class="mt-6 h-56 w-full rounded-xl" />
		{:else}
			{#key step}
				<section
					aria-labelledby="step-h"
					class="animate-in duration-300 fade-in slide-in-from-bottom-1"
				>
					<h1 id="step-h" class="headline text-2xl sm:text-3xl">
						{reconnect && step === 'connect' && !showConnected ? 'Reconnect Spotify' : TITLES[step]}
					</h1>

					{#if step === 'app'}
						<ol class="mt-8 grid gap-7">
							<li class="grid grid-cols-[1.75rem_1fr] gap-x-3.5">
								{@render n(1)}
								<p class="pt-0.5">
									{@render external(
										'https://developer.spotify.com/dashboard',
										'Spotify developer dashboard'
									)}, then <strong>Create app</strong>. Any name.
								</p>
							</li>
							<li class="grid grid-cols-[1.75rem_1fr] gap-x-3.5">
								{@render n(2)}
								<div class="grid min-w-0 gap-3 pt-0.5">
									<p>Add this <strong>Redirect URI</strong>:</p>
									{@render code(c.callback.redirect_uri, 'Copy redirect URI')}
									<p class="border-l-2 border-signal pl-3 text-sm text-muted-foreground">
										<span class="font-medium text-foreground">Exact match</span>: same scheme, host,
										{#if redirectPort}port <code class="font-mono text-foreground"
												>{redirectPort}</code
											>,{:else}no port,{/if} path, no trailing slash.
									</p>
								</div>
							</li>
							<li class="grid grid-cols-[1.75rem_1fr] gap-x-3.5">
								{@render n(3)}
								<p class="pt-0.5">
									Tick <strong>Web API</strong>, accept the terms, <strong>Save</strong>.
								</p>
							</li>
						</ol>
						<div class="mt-10 flex flex-wrap gap-2">
							<a href={href('client')} class={primary}>Next</a>
						</div>
					{:else if step === 'client'}
						<p class="mt-3 text-muted-foreground">
							In your app: <strong class="font-medium text-foreground">Settings</strong>, then Basic
							Information.
						</p>
						<form onsubmit={saveClient} class="mt-8 grid max-w-lg gap-3" novalidate>
							<label class="grid gap-1.5 text-sm font-medium">
								Client ID
								<input
									bind:value={clientId}
									autocomplete="off"
									spellcheck="false"
									maxlength="40"
									placeholder="32 letters and digits"
									aria-invalid={clientError ? 'true' : undefined}
									aria-describedby="client-help{clientError ? ' client-error' : ''}"
									class="{field} font-mono"
								/>
								<span id="client-help" class="text-xs font-normal text-muted-foreground"
									>Not the client secret. spotussi never needs it.</span
								>
							</label>
							{#if clientError}<p id="client-error" role="alert" class="text-sm text-destructive">
									{clientError}
								</p>{/if}
							{#if c.client_id && connected}
								<p class="text-sm text-muted-foreground">
									A new Client ID disconnects the current account.
								</p>
							{/if}
							<div class="mt-5 flex flex-wrap gap-2">
								<button type="submit" disabled={savingClient} class={primary}
									>{savingClient ? 'Saving…' : 'Save'}</button
								>
								<a href={href('app')} class={quiet}>Back</a>
							</div>
						</form>
					{:else if step === 'connect'}
						{#if showConnected}
							<div role="status" class="mt-8 grid gap-6">
								<div class="flex items-center gap-4">
									<span
										class="pop grid size-12 shrink-0 place-items-center rounded-full bg-signal text-[var(--window)]"
										><Check class="size-6" strokeWidth={3} aria-hidden="true" /></span
									>
									<div class="min-w-0">
										<p class="text-sm text-muted-foreground">Connected</p>
										<p class="display truncate text-xl">
											{c.account_name ?? c.account_id ?? 'Spotify'}
										</p>
									</div>
								</div>
								<div
									class="flex items-center gap-3 rounded-xl border bg-card px-4 py-3.5"
									aria-live="polite"
								>
									{#if heard?.track}
										<Lamp on />
										<p class="min-w-0 text-sm">
											<span class="text-muted-foreground">Now playing</span>
											<span class="font-medium">{heard.track.name}</span><span
												class="text-muted-foreground"
												>, {heard.track.artists.map((a) => a.name).join(', ')}</span
											>
										</p>
									{:else}
										<span
											class="listen relative grid size-[5px] place-items-center"
											aria-hidden="true"
											><span class="size-[5px] rounded-full bg-signal"></span></span
										>
										<p class="text-sm text-muted-foreground">Play anything on Spotify</p>
									{/if}
								</div>
							</div>
							<div class="mt-10 flex flex-wrap gap-2">
								{#if reconnect}
									<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
									<a href={next} class={primary}>Done</a>
								{:else}
									<a href={href('timezone')} class={primary}>Next</a>
								{/if}
							</div>
						{:else if !c.client_id}
							<p class="mt-3 text-muted-foreground">Add the Client ID first.</p>
							<div class="mt-8"><a href={href('client')} class={primary}>Add Client ID</a></div>
						{:else}
							{#if returnedError}<p
									role="alert"
									class="mt-6 rounded-xl border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm"
								>
									{returnedError}
								</p>{/if}
							{#if connected}
								<p class="mt-6 rounded-xl border bg-card px-4 py-3 text-sm">
									{#if missingScopes.length}
										<span class="font-medium">This connection is missing a permission.</span>
										Approve again to grant
										<code class="font-mono text-xs">{missingScopes.join(', ')}</code>. Live capture
										keeps running meanwhile.
									{:else}
										<span class="font-medium"
											>Connected as {c.account_name ?? c.account_id ?? 'Spotify'}.</span
										> Approve again to refresh the sign-in. Live capture keeps running meanwhile.
									{/if}
								</p>
							{/if}
							{#if c.state === 'reconnect_required'}
								<p
									class="mt-6 rounded-xl border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm"
								>
									<span class="font-medium">Spotify no longer accepts the saved sign-in.</span>
									Approve access again to restart live capture.{#if c.last_error}
										<span class="mt-1 block text-xs text-muted-foreground">{c.last_error}</span
										>{/if}
								</p>
							{/if}
							{#if c.callback.mode === 'direct'}
								<p class="mt-3 text-muted-foreground">
									Approve on Spotify; you'll come straight back.
								</p>
								<div class="mt-10 flex flex-wrap gap-2">
									<button
										type="button"
										onclick={startConnect}
										disabled={connecting}
										class={primary}
									>
										{connecting
											? 'Opening Spotify…'
											: reconnect
												? 'Reconnect Spotify'
												: 'Connect Spotify'}
									</button>
									{#if reconnect}
										<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
										<a href={next} class={quiet}>Not now</a>
									{:else}
										<a href={href('client')} class={quiet}>Back</a>
									{/if}
								</div>
							{:else}
								<p class="mt-3 text-muted-foreground">
									Plain HTTP, so you'll copy one address back by hand.
								</p>
								<ol class="mt-8 grid gap-7">
									<li class="grid grid-cols-[1.75rem_1fr] gap-x-3.5">
										{@render n(1)}
										<div class="grid gap-3 pt-0.5">
											<p>Approve in a new tab: <strong>Agree</strong>.</p>
											<div class="flex flex-wrap items-center gap-x-4 gap-y-2">
												<button
													type="button"
													onclick={startConnect}
													disabled={connecting}
													class={authorizeUrl ? secondary : primary}
												>
													{authorizeUrl ? 'Open Spotify again' : 'Open Spotify'}<ExternalLink
														class="size-4"
														aria-hidden="true"
													/>
												</button>
												{#if authorizeUrl}
													<a
														href={authorizeUrl}
														target="_blank"
														rel="noopener noreferrer"
														class="hit text-sm text-muted-foreground underline underline-offset-2 hover:text-foreground"
														>Tab didn't open?</a
													>
												{/if}
											</div>
										</div>
									</li>
									<li class="grid grid-cols-[1.75rem_1fr] gap-x-3.5">
										{@render n(2)}
										<p class="min-w-0 pt-0.5">
											It lands on <strong>“This site can't be reached”</strong> or
											<strong>“Unable to connect”</strong>. That's expected: the address bar holds
											the approval, starting with
											<code class="font-mono text-sm break-all">{c.callback.redirect_uri}</code>.
										</p>
									</li>
									<li class="grid grid-cols-[1.75rem_1fr] gap-x-3.5">
										{@render n(3)}
										<form onsubmit={finishPaste} class="grid min-w-0 gap-3 pt-0.5" novalidate>
											<label class="grid gap-1.5">
												<span>Paste that whole address:</span>
												<input
													bind:value={pasted}
													autocomplete="off"
													spellcheck="false"
													placeholder="{c.callback.redirect_uri}?code=…"
													aria-invalid={connectError ? 'true' : undefined}
													aria-describedby={connectError ? 'connect-error' : undefined}
													class="{field} font-mono text-sm"
												/>
											</label>
											<div class="flex flex-wrap gap-2">
												<button
													type="submit"
													disabled={connecting}
													class={authorizeUrl || pasted ? primary : secondary}
													>{connecting ? 'Connecting…' : 'Connect'}</button
												>
												{#if reconnect}
													<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
													<a href={next} class={quiet}>Not now</a>
												{:else}
													<a href={href('client')} class={quiet}>Back</a>
												{/if}
											</div>
										</form>
									</li>
								</ol>
							{/if}
							{#if connectError}<p
									id="connect-error"
									role="alert"
									class="mt-4 text-sm text-destructive"
								>
									{connectError}
								</p>{/if}
						{/if}
					{:else if step === 'timezone'}
						<form onsubmit={saveTz} class="mt-8 grid max-w-lg gap-5">
							<p class="flex flex-wrap items-baseline gap-x-4 gap-y-1" aria-hidden="true">
								<span class="figure tnum text-2xl leading-none">{localTime}</span>
								<span class="text-muted-foreground"
									>{zoneParts.at(-1)}{#if zoneParts.length > 1}, {zoneParts[0]}{/if}</span
								>
							</p>
							<label class="grid gap-1.5 text-sm font-medium">
								Time zone
								<select bind:value={tz} aria-describedby="tz-help" class={field}>
									{#each zoneList as z (z)}<option value={z}>{z.replaceAll('_', ' ')}</option
										>{/each}
								</select>
								<span id="tz-help" class="text-xs font-normal text-muted-foreground"
									>{tz === browserTz ? 'From this browser. ' : ''}Days and the listening clock
									follow it.</span
								>
							</label>
							{#if tzError}<p role="alert" class="text-sm text-destructive">{tzError}</p>{/if}
							<div class="mt-3 flex flex-wrap gap-2">
								<button type="submit" disabled={savingTz} class={primary}
									>{savingTz ? 'Saving…' : 'Save'}</button
								>
								<a href={href('connect')} class={quiet}>Back</a>
							</div>
						</form>
					{:else}
						<p class="mt-3 text-muted-foreground">
							{connected
								? 'Every play from now on is recorded.'
								: 'Live capture starts once Spotify is connected.'}
							Years before that come from Spotify's export.
						</p>
						<ol class="mt-8 grid gap-7">
							<li class="grid grid-cols-[1.75rem_1fr] gap-x-3.5">
								{@render n(1)}
								<p class="pt-0.5">
									In {@render external(
										'https://www.spotify.com/account/privacy/',
										'Spotify privacy settings'
									)}, tick <strong>Extended streaming history</strong> and request it.
								</p>
							</li>
							<li class="grid grid-cols-[1.75rem_1fr] gap-x-3.5">
								{@render n(2)}
								<p class="pt-0.5">
									Spotify emails it within <strong>30 days</strong>, often sooner.
								</p>
							</li>
							<li class="grid grid-cols-[1.75rem_1fr] gap-x-3.5">
								{@render n(3)}
								<div class="grid min-w-0 gap-3 pt-0.5">
									<p>
										Drop the zip here, or later in <a
											href={settingsHref('import')}
											class="inline-link hit">Settings</a
										>.
									</p>
									<ImportUpload surface="raised" cliHint={false} historyLink={false} />
								</div>
							</li>
						</ol>

						<div class="mt-10 flex flex-wrap gap-2">
							{#if importUpload.phase.kind === 'done'}
								<a href={resolve('/')} class={primary}>Open your overview</a>
							{:else}
								<a href={resolve('/')} class={importUpload.busy ? primary : secondary}
									>{importUpload.busy ? 'Open your overview' : 'Finish, import later'}</a
								>
							{/if}
							<a href={href('timezone')} class={quiet}>Back</a>
						</div>
					{/if}
				</section>
			{/key}
		{/if}
	</div>
</AuthFrame>

<style>
	/* The check lands with a little overshoot. */
	.pop {
		animation: pop 0.5s var(--ease-out-quart) both;
	}
	@keyframes pop {
		0% {
			transform: scale(0.4);
			opacity: 0;
		}
		60% {
			transform: scale(1.08);
			opacity: 1;
		}
		100% {
			transform: scale(1);
		}
	}
	/* Waiting for a play: a slow ring around the lamp. */
	.listen::before {
		content: '';
		position: absolute;
		inset: -5px;
		border-radius: 9999px;
		border: 1.5px solid var(--signal);
		animation: listen 1.8s var(--ease-out-quart) infinite;
	}
	@keyframes listen {
		from {
			transform: scale(0.4);
			opacity: 0.9;
		}
		to {
			transform: scale(1.6);
			opacity: 0;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.listen::before {
			animation: none;
			opacity: 0.4;
		}
	}
</style>
