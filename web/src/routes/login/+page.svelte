<!--
	Sign in with the owner password (D13, password mode). After signing in,
	return to the page that sent you here (`next`, same-origin paths only).
-->
<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import AuthFrame from '#lib/components/auth/auth-frame.svelte';
	import { field, primary } from '#lib/components/onboarding/ui.js';
	import { api, type Schemas } from '#lib/api/client.js';
	import { setSession, session } from '#lib/state/session.svelte.js';

	let password = $state('');
	let busy = $state(false);
	let error = $state('');
	let input = $state<HTMLInputElement>();

	const next = $derived.by(() => {
		const n = page.url.searchParams.get('next') ?? '/';
		// Only same-origin paths; never //host or a scheme.
		return n.startsWith('/') && !n.startsWith('//') ? n : '/';
	});

	// Sent here by a 401 mid-session (expired, signed out elsewhere).
	const ended = $derived(page.url.searchParams.get('ended') === '1');

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!password) {
			error = 'Enter the owner password.';
			input?.focus();
			return;
		}
		busy = true;
		error = '';
		try {
			const { data, error: err, response } = await api.POST('/auth/login', { body: { password } });
			if (data) {
				setSession(data);
				// eslint-disable-next-line svelte/no-navigation-without-resolve
				await goto(next, { replace: true });
				return;
			}
			const code = (err as Schemas['Error'] | undefined)?.error.code;
			if (response.status === 429) {
				const wait = Number(response.headers.get('Retry-After')) || 60;
				error = `Too many attempts. Try again in ${wait < 90 ? `${wait} seconds` : `${Math.ceil(wait / 60)} minutes`}.`;
			} else if (response.status === 401) {
				error = "That password isn't right.";
			} else if (code === 'wrong_auth_mode') {
				error = "This instance doesn't use a password. Reload the page.";
			} else {
				error =
					(err as Schemas['Error'] | undefined)?.error.message ??
					`Sign-in failed (${response.status}).`;
			}
			password = '';
			input?.focus();
		} catch {
			error = 'The server could not be reached.';
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Sign in – spotussi</title></svelte:head>

<AuthFrame>
	<h1 class="headline text-2xl sm:mt-6 sm:text-3xl">Sign in</h1>
	{#if ended}
		<p role="status" class="mt-3 text-muted-foreground">
			Your session ended. Sign in to pick up where you were.
		</p>
	{/if}
	{#if session.current?.mode === 'proxy'}
		<p class="mt-3 text-muted-foreground">Open this instance through its reverse proxy.</p>
	{:else}
		<form onsubmit={submit} class="mt-7 grid gap-4" novalidate>
			<!-- Lets password managers store the entry against a username. -->
			<input type="text" name="username" value="owner" autocomplete="username" hidden readonly />
			<label class="grid gap-1.5 text-sm font-medium">
				Owner password
				<input
					bind:this={input}
					bind:value={password}
					type="password"
					name="password"
					autocomplete="current-password"
					required
					aria-invalid={error ? 'true' : undefined}
					aria-describedby={error ? 'login-error' : undefined}
					class={field}
				/>
			</label>
			{#if error}<p id="login-error" role="alert" class="text-sm text-destructive">{error}</p>{/if}
			<button type="submit" disabled={busy} class="{primary} sm:w-full"
				>{busy ? 'Signing in…' : 'Sign in'}</button
			>
		</form>
		<p class="mt-8 text-xs text-muted-foreground">
			Forgot it? Run <code class="rounded bg-muted px-1.5 py-0.5 font-mono text-foreground"
				>spotussi reset-password</code
			> on the server.
		</p>
	{/if}
</AuthFrame>
