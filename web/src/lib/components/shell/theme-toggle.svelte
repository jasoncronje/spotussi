<!--
	Light/dark switch for the pages outside the app shell (sign-in, setup).
	It shows the theme a click switches to, based on what is on screen, so
	it never shows a third "system" state. The app's menu offers System too.
-->
<script lang="ts">
	import Sun from '@lucide/svelte/icons/sun';
	import Moon from '@lucide/svelte/icons/moon';
	import { theme } from '#lib/state/theme.svelte.js';

	const to = $derived(theme.dark ? 'light' : 'dark');
</script>

<button
	type="button"
	onclick={() => theme.set(to)}
	title="Switch to {to} theme"
	aria-label="Switch to {to} theme"
	class="grid size-9 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
>
	{#if to === 'light'}<Sun class="size-4" />{:else}<Moon class="size-4" />{/if}
</button>
