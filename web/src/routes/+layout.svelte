<script lang="ts">
	import './layout.css';
	import { page, navigating } from '$app/state';
	import Gauge from '@lucide/svelte/icons/gauge';
	import ListOrdered from '@lucide/svelte/icons/list-ordered';
	import ChartColumn from '@lucide/svelte/icons/chart-column';
	import History from '@lucide/svelte/icons/history';
	import Timer from '@lucide/svelte/icons/timer';
	import Wordmark from '#lib/components/shell/wordmark.svelte';
	import RangePicker from '#lib/components/shell/range-picker.svelte';
	import MetricToggle from '#lib/components/shell/metric-toggle.svelte';
	import AccountMenu from '#lib/components/shell/account-menu.svelte';
	import Lamp from '#lib/components/shell/lamp.svelte';
	import ShieldAlert from '@lucide/svelte/icons/shield-alert';
	import { sectionHref } from '#lib/state/view.svelte.js';
	import { activity, Resource } from '#lib/data/resource.svelte.js';
	import { settingsQuery, statusQuery } from '#lib/data/queries.js';
	import StatusBanners from '#lib/components/shell/status-banners.svelte';
	import CatalogStrip from '#lib/components/shell/catalog-strip.svelte';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { setUnauthorizedHandler } from '#lib/api/client.js';
	import { session, loadSession, authRedirect } from '#lib/state/session.svelte.js';
	import { applySettings } from '#lib/state/prefs.svelte.js';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

	// A request that comes back 401 means the session ended (expired, signed
	// out elsewhere, password reset): re-check and route to login.
	setUnauthorizedHandler(async () => {
		try {
			const s = await loadSession(true);
			const target = authRedirect(s, page.url.pathname, page.url.search);
			if (!target) return;
			const ended = target.startsWith('/login')
				? `${target}${target.includes('?') ? '&' : '?'}ended=1`
				: target;
			// eslint-disable-next-line svelte/no-navigation-without-resolve
			goto(ended);
		} catch {
			// unreachable server: pages show their own errors
		}
	});

	/** Login, first-run setup and the onboarding wizard draw their own frame. */
	const bare = $derived(['/login', '/setup', '/welcome'].includes(page.route.id ?? ''));
	const signedIn = $derived(Boolean(session.current?.authenticated));
	const openAccess = $derived(session.current?.mode === 'none');

	// Date format and week start come from instance settings.
	const settings = new Resource(() => (signedIn ? settingsQuery() : null));
	$effect(() => {
		if (settings.current) applySettings(settings.current);
	});

	// Instance state behind the shell's banners (Spotify connection, catalog
	// progress). Polled slowly while the tab is visible, and on return to it.
	const status = new Resource(() => (signedIn && !bare ? statusQuery() : null));
	onMount(() => {
		const tick = () => document.visibilityState === 'visible' && status.refresh();
		const id = setInterval(tick, 60_000);
		document.addEventListener('visibilitychange', tick);
		return () => {
			clearInterval(id);
			document.removeEventListener('visibilitychange', tick);
		};
	});

	const sections = [
		{ path: '/', label: 'Overview', icon: Gauge },
		{ path: '/top', label: 'Top', icon: ListOrdered },
		{ path: '/stats', label: 'Stats', icon: ChartColumn },
		{ path: '/history', label: 'History', icon: History },
		{ path: '/sessions', label: 'Sessions', icon: Timer }
	] as const;

	// Views that rank or chart by a measure get the Plays/Time toggle.
	const METRIC_VIEWS = new Set([
		'/',
		'/top/[[kind]]',
		'/stats',
		'/artists/[id]',
		'/albums/[id]',
		'/tracks/[id]',
		'/genres/[id]'
	]);

	const routeId = $derived(page.route.id ?? '');
	const showRange = $derived(!routeId.startsWith('/settings') && routeId !== '/sessions/[id]');
	const showMetric = $derived(METRIC_VIEWS.has(routeId));
	const busy = $derived(activity.refreshing > 0 || navigating.to !== null);

	function isActive(path: string) {
		return path === '/' ? page.url.pathname === '/' : page.url.pathname.startsWith(path);
	}
</script>

{#if bare}
	{@render children()}
{:else}
	<a
		href="#main"
		class="sr-only z-50 rounded-md bg-card px-3 py-2 focus:not-sr-only focus:fixed focus:top-2 focus:left-2"
	>
		Skip to content
	</a>

	<div class="flex min-h-dvh flex-col pb-[calc(4rem+env(safe-area-inset-bottom))] md:pb-0">
		<StatusBanners status={status.current} />
		{#if openAccess}
			<!-- AUTH_MODE=none (D13): always visible, never dismissible. -->
			<aside
				aria-label="Access warning"
				class="border-b border-destructive/25 bg-destructive/8 text-sm"
			>
				<div
					class="mx-auto flex max-w-[90rem] items-start gap-3 px-4 py-2 sm:items-center sm:px-6 lg:px-10"
				>
					<ShieldAlert class="mt-0.5 size-4 shrink-0 text-destructive sm:mt-0" aria-hidden="true" />
					<p class="min-w-0">
						<span class="font-semibold">No sign-in required.</span>
						<span class="text-muted-foreground"
							>Anyone who can reach this address can read and change everything. Set
							<code class="rounded bg-muted px-1 py-0.5 text-xs text-foreground"
								>AUTH_MODE=password</code
							>
							unless it's on a private network.</span
						>
					</p>
				</div>
			</aside>
		{/if}
		<header class="border-b bg-background">
			<div class="mx-auto flex h-14 max-w-[90rem] items-center gap-6 px-4 sm:px-6 lg:px-10">
				<a
					href={sectionHref('/')}
					class="-mx-1 rounded-md px-1 py-1"
					aria-label="spotussi, overview"
				>
					<Wordmark />
				</a>
				<nav aria-label="Main" class="hidden md:block">
					<ul class="flex items-center gap-1">
						{#each sections as s (s.path)}
							{@const active = isActive(s.path)}
							<li>
								<a
									href={sectionHref(s.path)}
									aria-current={active ? 'page' : undefined}
									class="flex h-9 items-center gap-2 rounded-lg px-3 text-sm text-muted-foreground transition-colors hover:text-foreground aria-[current]:text-foreground"
								>
									<Lamp on={active} class={active ? '' : 'opacity-0'} />
									<span class="font-medium">{s.label}</span>
								</a>
							</li>
						{/each}
					</ul>
				</nav>
				<div class="ml-auto flex items-center">
					<AccountMenu current={isActive('/settings')} />
				</div>
			</div>
		</header>

		{#if showRange && status.current}
			<CatalogStrip catalog={status.current.catalog} />
		{/if}

		{#if showRange}
			<div
				class="sticky top-0 z-30 border-b bg-background/88 backdrop-blur-md supports-[backdrop-filter]:bg-background/75"
			>
				<section
					aria-label="View controls"
					class="mx-auto flex h-12 max-w-[90rem] items-center justify-between gap-3 px-4 sm:px-6 lg:px-10"
				>
					<RangePicker />
					{#if showMetric}<MetricToggle />{/if}
				</section>
				<div
					class="pointer-events-none absolute inset-x-0 -bottom-px h-0.5 overflow-hidden"
					aria-hidden="true"
				>
					{#if busy}<div class="progress h-full w-1/3 bg-signal"></div>{/if}
				</div>
			</div>
		{/if}

		<main id="main" class="flex-1">
			{@render children()}
		</main>

		<!-- Phone navigation -->
		<nav
			aria-label="Main"
			class="fixed inset-x-0 bottom-0 z-40 border-t bg-background/92 pb-[env(safe-area-inset-bottom)] backdrop-blur-md md:hidden"
		>
			<ul class="mx-auto grid h-16 max-w-md grid-cols-5">
				{#each sections as s (s.path)}
					{@const active = isActive(s.path)}
					<li>
						<a
							href={sectionHref(s.path)}
							aria-current={active ? 'page' : undefined}
							class="flex h-full flex-col items-center justify-center gap-1 text-2xs text-muted-foreground aria-[current]:text-foreground"
						>
							<span class="relative">
								<s.icon class="size-5" strokeWidth={active ? 2.25 : 1.75} />
								{#if active}<Lamp on class="absolute -top-1 -right-2" />{/if}
							</span>
							{s.label}
						</a>
					</li>
				{/each}
			</ul>
		</nav>
	</div>
{/if}

<style>
	.progress {
		animation: sweep 0.9s var(--ease-out-quart) infinite;
	}
	@media (prefers-reduced-motion: reduce) {
		.progress {
			animation: none;
			width: 100%;
			opacity: 0.7;
		}
	}
	@keyframes sweep {
		from {
			transform: translateX(-100%);
		}
		to {
			transform: translateX(300%);
		}
	}
</style>
