<!--
	First run in password mode (D13): prove you run the server with the
	one-time setup token it printed to its log, then choose the owner
	password. Signs in and continues to the Spotify setup guide.
-->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import AuthFrame from '#lib/components/auth/auth-frame.svelte';
	import Progress from '#lib/components/onboarding/progress.svelte';
	import { field, primary } from '#lib/components/onboarding/ui.js';
	import { api, type Schemas } from '#lib/api/client.js';
	import { loadSession, setSession } from '#lib/state/session.svelte.js';

	let token = $state('');
	let password = $state('');
	let confirm = $state('');
	let busy = $state(false);
	let error = $state<{ field: 'token' | 'password' | 'confirm' | 'form'; text: string } | null>(
		null
	);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		error = null;
		const t = token.trim();
		if (!t) return (error = { field: 'token', text: 'Paste the setup token from the server log.' });
		if (password.length < 8)
			return (error = { field: 'password', text: 'Use at least 8 characters.' });
		if (password !== confirm)
			return (error = { field: 'confirm', text: "The two passwords don't match." });
		busy = true;
		try {
			const {
				data,
				error: err,
				response
			} = await api.POST('/auth/setup', {
				body: { password, setup_token: t }
			});
			if (data) {
				setSession(data);
				await goto(resolve('/welcome'), { replace: true });
				return;
			}
			const code = (err as Schemas['Error'] | undefined)?.error.code;
			if (code === 'invalid_setup_token' || response.status === 401) {
				error = {
					field: 'token',
					text: "That token doesn't match. Copy it from the latest server start."
				};
			} else if (code === 'setup_done') {
				await loadSession(true);
				await goto(resolve('/login'), { replace: true });
			} else if (response.status === 429) {
				error = { field: 'form', text: 'Too many attempts. Wait a minute and try again.' };
			} else {
				error = {
					field: 'form',
					text:
						(err as Schemas['Error'] | undefined)?.error.message ??
						`Setup failed (${response.status}).`
				};
			}
		} catch {
			error = { field: 'form', text: 'The server could not be reached.' };
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Set up – spotussi</title></svelte:head>

<AuthFrame width="wide">
	<Progress current="owner" done={{}} href={() => null} />

	<div class="mt-10 max-w-md sm:mt-14">
		<h1 class="headline text-2xl sm:text-3xl">Claim this instance</h1>

		<form onsubmit={submit} class="mt-8 grid gap-5" novalidate>
			<label class="grid gap-1.5 text-sm font-medium">
				Setup token
				<input
					bind:value={token}
					name="setup_token"
					autocomplete="off"
					autocapitalize="characters"
					spellcheck="false"
					placeholder="XXXXX-XXXXX-XXXXX-XXXXX"
					aria-invalid={error?.field === 'token' ? 'true' : undefined}
					aria-describedby="token-help{error?.field === 'token' ? ' token-error' : ''}"
					class="{field} font-mono tracking-wide uppercase placeholder:text-faint/70"
				/>
				<span id="token-help" class="text-xs font-normal text-muted-foreground">
					In the server log, for example
					<code class="rounded bg-muted px-1 py-0.5 font-mono text-foreground"
						>docker logs spotussi</code
					>
				</span>
				{#if error?.field === 'token'}<span
						id="token-error"
						role="alert"
						class="text-sm font-normal text-destructive">{error.text}</span
					>{/if}
			</label>

			<input type="text" name="username" value="owner" autocomplete="username" hidden readonly />
			<label class="grid gap-1.5 text-sm font-medium">
				Owner password
				<input
					bind:value={password}
					type="password"
					name="new-password"
					autocomplete="new-password"
					minlength="8"
					aria-invalid={error?.field === 'password' ? 'true' : undefined}
					aria-describedby="pw-help{error?.field === 'password' ? ' pw-error' : ''}"
					class={field}
				/>
				<span id="pw-help" class="text-xs font-normal text-muted-foreground"
					>8 characters or more</span
				>
				{#if error?.field === 'password'}<span
						id="pw-error"
						role="alert"
						class="text-sm font-normal text-destructive">{error.text}</span
					>{/if}
			</label>
			<label class="grid gap-1.5 text-sm font-medium">
				Password again
				<input
					bind:value={confirm}
					type="password"
					name="confirm-password"
					autocomplete="new-password"
					aria-invalid={error?.field === 'confirm' ? 'true' : undefined}
					aria-describedby={error?.field === 'confirm' ? 'confirm-error' : undefined}
					class={field}
				/>
				{#if error?.field === 'confirm'}<span
						id="confirm-error"
						role="alert"
						class="text-sm font-normal text-destructive">{error.text}</span
					>{/if}
			</label>
			{#if error?.field === 'form'}<p role="alert" class="text-sm text-destructive">
					{error.text}
				</p>{/if}
			<div class="mt-2">
				<button type="submit" disabled={busy} class={primary}
					>{busy ? 'Saving…' : 'Set password'}</button
				>
			</div>
		</form>
	</div>
</AuthFrame>
