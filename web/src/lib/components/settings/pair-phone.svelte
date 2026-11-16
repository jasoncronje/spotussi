<!--
	Pair the iPhone app (D41): creates a read-only API token named for the
	phone and shows it once as a QR code holding a `spotussi://pair` link.
	The same link is a button, for when this page is open on the phone itself,
	and the address and token can be copied for typing in by hand.
-->
<script lang="ts">
	import { page } from '$app/state';
	import Smartphone from '@lucide/svelte/icons/smartphone';
	import SettingsPanel from './settings-panel.svelte';
	import CopyButton from '#lib/components/common/copy-button.svelte';
	import { api, type Schemas } from '#lib/api/client.js';
	import { unwrap } from '#lib/data/queries.js';
	import { isLoopback, pairLink, qrPath } from '#lib/pairing.js';

	let { onchange }: { onchange: () => void } = $props();

	const origin = $derived(page.url.origin);
	const localOnly = $derived(isLoopback(page.url.hostname));

	let pairing = $state(false);
	let error = $state('');
	let created = $state<Schemas['ApiTokenCreated'] | null>(null);
	let qr = $state<{ size: number; d: string } | null>(null);
	const link = $derived(created ? pairLink(origin, created.secret) : '');

	async function pair() {
		pairing = true;
		error = '';
		try {
			const token = await unwrap(api.POST('/auth/tokens', { body: { name: 'iPhone' } }));
			qr = await qrPath(pairLink(origin, token.secret));
			created = token;
			onchange();
		} catch (err) {
			error = (err as Error).message;
		} finally {
			pairing = false;
		}
	}

	function done() {
		created = null;
		qr = null;
	}
</script>

<SettingsPanel
	id="pair-iphone"
	title="iPhone App"
	hint="Creates a read-only token named iPhone. Revoke it above to unpair."
>
	{#if created && qr}
		<div
			role="status"
			class="grid grid-cols-[minmax(0,1fr)] gap-5 sm:grid-cols-[auto_minmax(0,1fr)] sm:items-start"
		>
			<!-- Always dark on white: a QR code is read by contrast, not by theme. -->
			<svg
				viewBox="0 0 {qr.size} {qr.size}"
				role="img"
				aria-label="Pairing code for the iPhone app"
				shape-rendering="crispEdges"
				class="size-56 max-w-full rounded-xl border bg-white max-sm:mx-auto"
			>
				<path d={qr.d} fill="#121317" />
			</svg>
			<div class="grid min-w-0 grid-cols-[minmax(0,1fr)] gap-4">
				<div>
					<p class="font-medium">Scan with the iPhone camera</p>
					<p class="text-sm text-muted-foreground">
						The code is shown only once. It lets the app read your stats, nothing else.
					</p>
				</div>
				<a
					href={link}
					class="inline-flex h-9 w-fit items-center gap-2 rounded-lg border bg-card px-3.5 text-sm font-medium hover:bg-muted"
					><Smartphone class="size-4" aria-hidden="true" />Open in the app</a
				>
				<details class="group text-sm">
					<summary
						class="w-fit cursor-pointer text-muted-foreground underline underline-offset-2 hover:text-foreground"
						>Enter it by hand</summary
					>
					<dl class="mt-3 grid grid-cols-[minmax(0,1fr)] gap-3">
						<div class="grid grid-cols-[minmax(0,1fr)] gap-1.5">
							<dt class="font-medium">Address</dt>
							<dd class="flex items-center gap-2">
								<code
									class="min-w-0 flex-1 truncate rounded-md border bg-muted/40 px-3 py-2 font-mono text-xs select-all"
									title={origin}>{origin}</code
								>
								<CopyButton value={origin} label="Copy address" />
							</dd>
						</div>
						<div class="grid grid-cols-[minmax(0,1fr)] gap-1.5">
							<dt class="font-medium">Token</dt>
							<dd class="flex items-center gap-2">
								<code
									class="min-w-0 flex-1 truncate rounded-md border bg-muted/40 px-3 py-2 font-mono text-xs select-all"
									title={created.secret}>{created.secret}</code
								>
								<CopyButton value={created.secret} label="Copy token" />
							</dd>
						</div>
					</dl>
				</details>
				<button
					type="button"
					onclick={done}
					class="h-9 w-fit rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground"
					>Done</button
				>
			</div>
		</div>
	{:else}
		<div class="grid gap-3">
			<p class="text-sm text-muted-foreground">
				Shows a code to scan with your iPhone. The app connects to
				<span class="font-mono text-xs text-foreground">{origin}</span>.
			</p>
			{#if localOnly}
				<p class="rounded-lg border border-dashed px-3 py-2 text-sm text-muted-foreground">
					This address works only on this computer. To pair a phone, open Spotussi at an address the
					phone can reach.
				</p>
			{/if}
			<button
				type="button"
				onclick={pair}
				disabled={pairing}
				class="inline-flex h-9 w-fit items-center gap-2 rounded-lg bg-primary px-3.5 text-sm font-medium text-primary-foreground disabled:opacity-60 max-sm:w-full max-sm:justify-center"
				><Smartphone class="size-4" aria-hidden="true" />{pairing
					? 'Creating…'
					: 'Pair iPhone'}</button
			>
			{#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
		</div>
	{/if}
</SettingsPanel>
