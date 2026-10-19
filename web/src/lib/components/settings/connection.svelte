<!--
	The Spotify account that live capture uses: state, account, when it was
	connected, granted scopes, and why a reconnect is needed. Reconnecting
	and changing the app go through the setup guide; disconnecting asks first.
-->
<script lang="ts">
	import { settingsHref } from './tabs.js';
	import { resolve } from '$app/paths';
	import SettingsPanel from './settings-panel.svelte';
	import Lamp from '#lib/components/shell/lamp.svelte';
	import ConfirmDialog from '#lib/components/common/confirm-dialog.svelte';
	import { api, type Schemas } from '#lib/api/client.js';
	import { unwrap } from '#lib/data/queries.js';
	import { dateAt } from '#lib/format.js';

	let { data, onchange }: { data: Schemas['SpotifyConnection'] | undefined; onchange: () => void } =
		$props();

	let confirmOpen = $state(false);
	async function disconnect() {
		await unwrap(api.DELETE('/spotify/connection'));
		onchange();
	}

	const STATE = {
		connected: { label: 'Connected', on: true },
		reconnect_required: { label: 'Reconnect needed', on: false },
		not_configured: { label: 'Not connected', on: false }
	} as const;
	const btn = 'inline-flex h-9 items-center rounded-lg px-3.5 text-sm font-medium';
</script>

<SettingsPanel id="spotify" title="Spotify Account">
	{#if data}
		{@const st = STATE[data.state]}
		<div class="grid gap-4">
			<p class="flex items-center gap-2 text-sm font-medium">
				<Lamp on={st.on} />
				<span class={data.state === 'reconnect_required' ? 'text-destructive' : ''}>{st.label}</span
				>
			</p>
			{#if data.state === 'reconnect_required' && data.last_error}
				<p class="rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm">
					{data.last_error} Capture is paused until you reconnect.
				</p>
			{/if}
			<dl class="grid gap-x-8 gap-y-2 text-sm sm:grid-cols-[auto_1fr]">
				{#if data.account_name || data.account_id}
					<dt class="text-muted-foreground">Account</dt>
					<dd>
						{data.account_name ?? data.account_id}{#if data.account_name && data.account_id}<span
								class="text-muted-foreground"
							>
								({data.account_id})</span
							>{/if}
					</dd>
				{/if}
				{#if data.connected_at}
					<dt class="text-muted-foreground">Connected</dt>
					<dd>{dateAt(data.connected_at)}</dd>
				{/if}
				<dt class="text-muted-foreground">App Client ID</dt>
				<dd class="font-mono text-xs break-all">{data.client_id ?? 'Not set'}</dd>
				<dt class="text-muted-foreground">Redirect URI</dt>
				<dd class="font-mono text-xs break-all">{data.callback.redirect_uri}</dd>
				{#if data.scopes.length}
					<dt class="text-muted-foreground">Permissions</dt>
					<dd class="text-xs text-muted-foreground">{data.scopes.join(', ')}</dd>
				{/if}
			</dl>
			<div class="flex flex-wrap gap-2">
				{#if data.state === 'not_configured'}
					<a href={resolve('/welcome')} class="{btn} bg-primary text-primary-foreground"
						>Set up Spotify</a
					>
				{:else}
					<a
						href="{resolve('/welcome')}?step=connect&reconnect=1&next={encodeURIComponent(
							settingsHref('spotify')
						)}"
						class="{btn} {data.state === 'reconnect_required'
							? 'bg-primary text-primary-foreground'
							: 'border bg-card hover:bg-muted'}">Reconnect</a
					>
					<a href="{resolve('/welcome')}?step=app" class="{btn} border bg-card hover:bg-muted"
						>Change app</a
					>
					<button
						type="button"
						onclick={() => (confirmOpen = true)}
						class="{btn} text-destructive hover:bg-destructive/10">Disconnect</button
					>
				{/if}
			</div>
		</div>
	{:else}
		<p class="text-sm text-muted-foreground">Loading…</p>
	{/if}
</SettingsPanel>

<ConfirmDialog
	bind:open={confirmOpen}
	title="Disconnect Spotify?"
	confirmLabel="Disconnect"
	destructive
	onconfirm={disconnect}
>
	<p>
		Capture stops and the Spotify sign-in is deleted. Recorded plays stay. Plays while disconnected
		come back only from a later export.
	</p>
</ConfirmDialog>
