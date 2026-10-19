<!--
	The header menu: Settings, the theme (system, light, dark) and signing
	out. Who is signed in shows at the top in proxy mode; sign-out is offered
	only in password mode (a proxy signs out on its own, open access has no
	session).
-->
<script lang="ts">
	import { DropdownMenu } from 'bits-ui';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import SettingsIcon from '@lucide/svelte/icons/settings';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import Monitor from '@lucide/svelte/icons/monitor';
	import Sun from '@lucide/svelte/icons/sun';
	import Moon from '@lucide/svelte/icons/moon';
	import Check from '@lucide/svelte/icons/check';
	import LogOut from '@lucide/svelte/icons/log-out';
	import { api } from '#lib/api/client.js';
	import { unwrap } from '#lib/data/queries.js';
	import { settingsHref } from '#lib/components/settings/tabs.js';
	import { session, loadSession } from '#lib/state/session.svelte.js';
	import { theme, type Theme } from '#lib/state/theme.svelte.js';

	let { current = false }: { /** On a Settings page. */ current?: boolean } = $props();

	const mode = $derived(session.current?.mode);
	const THEMES = [
		{ value: 'system', label: 'System', icon: Monitor },
		{ value: 'light', label: 'Light', icon: Sun },
		{ value: 'dark', label: 'Dark', icon: Moon }
	] as const;

	async function signOut() {
		try {
			await unwrap(api.POST('/auth/logout'));
		} finally {
			await loadSession(true).catch(() => {});
			goto(resolve('/login'));
		}
	}

	const item =
		'flex h-9 cursor-default items-center gap-2.5 rounded-md px-2.5 text-sm outline-none select-none data-highlighted:bg-muted';
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger
		aria-label="Menu"
		class="flex h-9 items-center gap-1 rounded-lg px-2 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground data-[state=open]:bg-muted data-[state=open]:text-foreground {current
			? 'bg-muted text-foreground'
			: ''}"
	>
		<SettingsIcon class="size-[18px]" aria-hidden="true" />
		<ChevronDown class="size-3.5 opacity-70" aria-hidden="true" />
	</DropdownMenu.Trigger>
	<DropdownMenu.Portal>
		<DropdownMenu.Content
			align="end"
			sideOffset={6}
			class="z-50 w-56 origin-(--bits-dropdown-menu-content-transform-origin) rounded-xl border bg-popover p-1.5 text-popover-foreground shadow-[0_16px_40px_-16px_rgb(0_0_0/0.3)] outline-hidden duration-150 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-[0.98]"
		>
			{#if mode === 'proxy' && session.current?.user}
				<p class="truncate px-2.5 pt-1.5 pb-2 text-xs text-muted-foreground">
					Signed in as <span class="font-medium text-foreground">{session.current.user}</span>
				</p>
				<DropdownMenu.Separator class="-mx-1.5 my-1 h-px bg-border" />
			{/if}
			<DropdownMenu.Item class={item}>
				{#snippet child({ props })}
					<a {...props} href={settingsHref('general')}>
						<SettingsIcon class="size-4 text-muted-foreground" aria-hidden="true" />Settings
					</a>
				{/snippet}
			</DropdownMenu.Item>
			<DropdownMenu.Separator class="-mx-1.5 my-1 h-px bg-border" />
			<DropdownMenu.Group>
				<DropdownMenu.GroupHeading class="px-2.5 pt-1.5 pb-1 text-xs text-muted-foreground"
					>Theme</DropdownMenu.GroupHeading
				>
				<DropdownMenu.RadioGroup value={theme.choice} onValueChange={(v) => theme.set(v as Theme)}>
					{#each THEMES as t (t.value)}
						<DropdownMenu.RadioItem value={t.value} closeOnSelect={false} class={item}>
							{#snippet children({ checked })}
								<t.icon class="size-4 text-muted-foreground" aria-hidden="true" />
								<span class="flex-1">{t.label}</span>
								{#if checked}<Check class="size-4 text-signal-ink" aria-hidden="true" />{/if}
							{/snippet}
						</DropdownMenu.RadioItem>
					{/each}
				</DropdownMenu.RadioGroup>
			</DropdownMenu.Group>
			{#if mode === 'password'}
				<DropdownMenu.Separator class="-mx-1.5 my-1 h-px bg-border" />
				<DropdownMenu.Item class={item} onSelect={signOut}>
					<LogOut class="size-4 text-muted-foreground" aria-hidden="true" />Sign out
				</DropdownMenu.Item>
			{/if}
		</DropdownMenu.Content>
	</DropdownMenu.Portal>
</DropdownMenu.Root>
