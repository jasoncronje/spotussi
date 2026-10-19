<!--
	Who is signed in and signing out. Shown only where it means something:
	proxy mode names the user (the proxy signs out on its own), password mode
	offers sign-out; open access has no session, so the panel is left out.
-->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import LogOut from '@lucide/svelte/icons/log-out';
	import { Button } from '#lib/components/ui/button/index.js';
	import SettingsPanel from './settings-panel.svelte';
	import { api } from '#lib/api/client.js';
	import { unwrap } from '#lib/data/queries.js';
	import { session, loadSession } from '#lib/state/session.svelte.js';

	const mode = $derived(session.current?.mode);
	const user = $derived(session.current?.user);
	let busy = $state(false);

	async function signOut() {
		busy = true;
		try {
			await unwrap(api.POST('/auth/logout'));
		} finally {
			await loadSession(true).catch(() => {});
			goto(resolve('/login'));
		}
	}
</script>

{#if mode === 'password' || (mode === 'proxy' && user)}
	<SettingsPanel id="account" title="Account">
		<div class="flex flex-wrap items-center justify-between gap-x-6 gap-y-3">
			<p class="text-sm text-muted-foreground">
				{#if mode === 'proxy'}
					Signed in as <span class="font-medium text-foreground">{user}</span> through your proxy.
				{:else}
					Signed in as the owner on this browser.
				{/if}
			</p>
			{#if mode === 'password'}
				<Button variant="outline" onclick={signOut} disabled={busy}>
					<LogOut aria-hidden="true" />Sign out
				</Button>
			{/if}
		</div>
	</SettingsPanel>
{/if}
