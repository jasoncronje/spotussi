<!--
	Instance-wide states every page shares, above the header (M8):
	- the server can't be reached: one banner instead of every section
	  failing on its own; probes /health and refetches everything once it
	  answers again;
	- Spotify needs reconnecting (refresh token revoked or expired, D12):
	  live capture has stopped, so this stays until it's fixed;
	- Spotify isn't connected yet but there's history to look at.
	Banners are not dismissible: each one describes something still true.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import Unplug from '@lucide/svelte/icons/unplug';
	import { api, type Schemas } from '#lib/api/client.js';
	import { refreshAll } from '#lib/data/resource.svelte.js';
	import { link, serverAnswered } from '#lib/state/link.svelte.js';
	import { ago } from '#lib/format.js';

	let { status }: { status: Schemas['SystemStatus'] | undefined } = $props();

	// ---------- server unreachable ----------
	let probing = $state(false);
	let attempt = 0;
	let timer: ReturnType<typeof setTimeout> | undefined;

	async function probe() {
		clearTimeout(timer);
		probing = true;
		try {
			const r = await api.GET('/health');
			if (serverAnswered(r.response.status, r.error)) {
				link.down = false;
				attempt = 0;
				refreshAll();
				return;
			}
		} catch {
			// still down
		} finally {
			probing = false;
		}
		schedule();
	}
	function schedule() {
		clearTimeout(timer);
		if (!link.down) return;
		// 2 s, 4 s, 8 s … capped at 30 s.
		timer = setTimeout(probe, Math.min(30_000, 2000 * 2 ** attempt++));
	}
	$effect(() => {
		if (link.down) schedule();
		else clearTimeout(timer);
	});
	onMount(() => {
		const online = () => link.down && probe();
		window.addEventListener('online', online);
		return () => {
			window.removeEventListener('online', online);
			clearTimeout(timer);
		};
	});

	// ---------- Spotify ----------
	const spotify = $derived(status?.spotify.state);
	const here = $derived(page.url.pathname + page.url.search);
	const reconnectHref = $derived(
		`${resolve('/welcome')}?step=connect&reconnect=1&next=${encodeURIComponent(here)}`
	);
	const hasHistory = $derived((status?.data.rows ?? 0) > 0);
	const onSettings = $derived(page.url.pathname.startsWith('/settings'));
</script>

{#if link.down}
	<aside
		aria-label="Server unreachable"
		class="border-b border-destructive/25 bg-destructive/8 text-sm"
	>
		<div
			class="mx-auto flex max-w-[90rem] flex-wrap items-center gap-x-3 gap-y-2 px-4 py-2 sm:px-6 lg:px-10"
		>
			<Unplug class="size-4 shrink-0 text-destructive" aria-hidden="true" />
			<p class="min-w-0 flex-1" role="status">
				<span class="font-semibold">Can’t reach the server.</span>
				<span class="text-muted-foreground"
					>Showing what was already loaded. {probing
						? 'Checking…'
						: 'Retrying automatically.'}</span
				>
			</p>
			<button
				type="button"
				onclick={() => {
					attempt = 0;
					probe();
				}}
				disabled={probing}
				class="h-8 rounded-lg border bg-card px-3 text-sm font-medium hover:bg-muted disabled:opacity-60"
				>Try now</button
			>
		</div>
	</aside>
{/if}

{#if spotify === 'reconnect_required'}
	<aside aria-label="Spotify connection" class="reconnect border-b text-sm">
		<div
			class="mx-auto flex max-w-[90rem] flex-wrap items-center gap-x-3 gap-y-2 px-4 py-2.5 sm:px-6 lg:px-10"
		>
			<span class="beacon relative grid size-[7px] shrink-0 place-items-center" aria-hidden="true">
				<span class="size-[7px] rounded-full bg-signal"></span>
			</span>
			<p class="min-w-0 flex-1">
				<span class="font-semibold">Spotify needs reconnecting.</span>
				<span class="text-muted-foreground"
					>Live capture stopped{#if status?.capture.last_play_at}; last play caught {ago(
							status.capture.last_play_at
						)}{/if}. Your history is safe.</span
				>
			</p>
			<a
				href={reconnectHref}
				class="inline-flex h-8 items-center rounded-lg bg-primary px-3 text-sm font-medium text-primary-foreground hover:opacity-90"
				>Reconnect Spotify</a
			>
		</div>
	</aside>
{:else if spotify === 'not_configured' && hasHistory && !onSettings}
	<aside aria-label="Spotify connection" class="border-b bg-muted/40 text-sm">
		<div
			class="mx-auto flex max-w-[90rem] flex-wrap items-center gap-x-3 gap-y-1 px-4 py-2 sm:px-6 lg:px-10"
		>
			<span class="size-[5px] shrink-0 rounded-full bg-grille" aria-hidden="true"></span>
			<p class="min-w-0 flex-1 text-muted-foreground">
				<span class="font-medium text-foreground">Spotify isn’t connected.</span>
				New plays aren’t being captured.
			</p>
			<a href="{resolve('/welcome')}?step=app" class="inline-link hit text-sm font-medium"
				>Connect Spotify</a
			>
		</div>
	</aside>
{/if}

<style>
	.reconnect {
		background: var(--signal-soft);
		border-color: color-mix(in oklab, var(--signal) 40%, transparent);
	}
	/* One beacon on the screen: the only lamp that pulses. */
	.beacon::after {
		content: '';
		position: absolute;
		inset: -4px;
		border-radius: 999px;
		background: var(--signal);
		opacity: 0;
		animation: beacon 1.8s var(--ease-out-quart) infinite;
	}
	@media (prefers-reduced-motion: reduce) {
		.beacon::after {
			animation: none;
		}
	}
	@keyframes beacon {
		0% {
			transform: scale(0.4);
			opacity: 0.55;
		}
		100% {
			transform: scale(1.6);
			opacity: 0;
		}
	}
</style>
