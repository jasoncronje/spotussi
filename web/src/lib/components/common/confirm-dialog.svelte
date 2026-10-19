<!--
	A modal question before an action that is hard to take back (revoke a
	token, disconnect the account). Native <dialog>: focus is trapped and
	Escape cancels. The action runs here so a failure stays in the dialog.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		open = $bindable(false),
		title,
		confirmLabel,
		destructive = false,
		onconfirm,
		children
	}: {
		open?: boolean;
		title: string;
		confirmLabel: string;
		destructive?: boolean;
		onconfirm: () => Promise<void> | void;
		children: Snippet;
	} = $props();

	const uid = $props.id();
	let dialog = $state<HTMLDialogElement>();
	let busy = $state(false);
	let error = $state('');

	$effect(() => {
		if (!dialog) return;
		if (open && !dialog.open) {
			error = '';
			dialog.showModal();
		} else if (!open && dialog.open) dialog.close();
	});

	async function confirm() {
		busy = true;
		error = '';
		try {
			await onconfirm();
			open = false;
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy = false;
		}
	}
</script>

<dialog
	bind:this={dialog}
	aria-labelledby="{uid}-t"
	onclose={() => (open = false)}
	class="m-auto w-[min(28rem,calc(100vw-2rem))] rounded-xl border bg-popover p-0 text-popover-foreground shadow-[0_24px_64px_-16px_rgb(0_0_0/0.45)] backdrop:bg-black/50"
>
	<div class="grid gap-4 p-6">
		<h2 id="{uid}-t" class="text-lg font-semibold">{title}</h2>
		<div class="text-sm text-muted-foreground">{@render children()}</div>
		{#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
		<div class="flex flex-wrap justify-end gap-2">
			<button
				type="button"
				onclick={() => (open = false)}
				class="h-9 rounded-lg border bg-card px-3.5 text-sm font-medium hover:bg-muted"
				>Cancel</button
			>
			<button
				type="button"
				onclick={confirm}
				disabled={busy}
				class="h-9 rounded-lg px-3.5 text-sm font-medium disabled:opacity-60 {destructive
					? 'bg-destructive text-background'
					: 'bg-primary text-primary-foreground'}">{busy ? 'Working…' : confirmLabel}</button
			>
		</div>
	</div>
</dialog>
