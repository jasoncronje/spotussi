<!--
	One section that failed to load, contained where it would have been so
	the rest of the page keeps working. Says what kind of failure it was:
	the server is unreachable (the shell's banner retries for you), the
	server failed (5xx, worth retrying), or the request was refused.
-->
<script lang="ts">
	import { ApiError } from '#lib/data/cache.js';
	import { unreachable } from '#lib/state/link.svelte.js';

	let {
		what,
		error,
		retry,
		class: className = ''
	}: { what: string; error: Error; retry: () => void; class?: string } = $props();

	const status = $derived(error instanceof ApiError ? error.status : 0);
	const detail = $derived.by(() => {
		if (unreachable(error)) return 'The server isn’t answering. This retries when it’s back.';
		if (status >= 500) return `The server failed (${status}): ${error.message}`;
		if (status === 403) return 'Not allowed for this sign-in.';
		return error.message;
	});
</script>

<div
	role="alert"
	class="flex flex-wrap content-center items-center justify-between gap-3 rounded-xl border border-destructive/30 bg-destructive/5 px-5 py-4 text-sm {className}"
>
	<p class="min-w-0">
		<span class="font-semibold">Couldn't load {what}.</span>
		<span class="text-muted-foreground">{detail}</span>
	</p>
	<button
		type="button"
		onclick={retry}
		class="h-8 rounded-lg border bg-card px-3 text-sm font-medium transition-colors hover:bg-muted"
	>
		Try again
	</button>
</div>
