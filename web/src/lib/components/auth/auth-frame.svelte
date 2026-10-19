<!--
	The frame for pages outside the app shell (sign-in, first-run setup, the
	setup guide): the mark and the theme switch, then one column on the
	faceplate. `wide` fits the setup meter and guide; `narrow` a single form.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import Wordmark from '#lib/components/shell/wordmark.svelte';
	import ThemeToggle from '#lib/components/shell/theme-toggle.svelte';

	let {
		width = 'narrow',
		children
	}: {
		width?: 'narrow' | 'wide';
		children: Snippet;
	} = $props();
</script>

<div class="flex min-h-dvh flex-col">
	<header
		class="mx-auto flex w-full max-w-[90rem] items-center justify-between px-4 py-4 sm:px-6 lg:px-10"
	>
		<Wordmark />
		<ThemeToggle />
	</header>
	<main
		id="main"
		class="mx-auto w-full flex-1 px-4 pt-4 pb-16 sm:px-6 sm:pt-10 {width === 'wide'
			? 'max-w-[44rem]'
			: 'max-w-md'}"
	>
		{@render children()}
	</main>
</div>
