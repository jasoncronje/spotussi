<!--
	Copies a value and says so. Falls back to selecting text for browsers
	without clipboard access (plain-HTTP instances).
-->
<script lang="ts">
	import Copy from '@lucide/svelte/icons/copy';
	import Check from '@lucide/svelte/icons/check';

	let {
		value,
		label = 'Copy',
		class: className = ''
	}: { value: string; label?: string; class?: string } = $props();

	let status = $state<'idle' | 'copied' | 'failed'>('idle');
	let timer: ReturnType<typeof setTimeout> | undefined;

	async function copy() {
		try {
			if (navigator.clipboard && window.isSecureContext) {
				await navigator.clipboard.writeText(value);
			} else {
				const ta = document.createElement('textarea');
				ta.value = value;
				ta.setAttribute('readonly', '');
				ta.style.position = 'fixed';
				ta.style.opacity = '0';
				document.body.appendChild(ta);
				ta.select();
				const ok = document.execCommand('copy');
				ta.remove();
				if (!ok) throw new Error('copy failed');
			}
			status = 'copied';
		} catch {
			status = 'failed';
		}
		clearTimeout(timer);
		timer = setTimeout(() => (status = 'idle'), 2000);
	}
</script>

<button
	type="button"
	onclick={copy}
	class="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md border bg-card px-2.5 text-xs font-medium transition-colors hover:bg-muted {className}"
>
	{#if status === 'copied'}
		<Check class="size-3.5 text-signal-ink" aria-hidden="true" />Copied
	{:else}
		<Copy class="size-3.5" aria-hidden="true" />{status === 'failed' ? 'Select and copy' : label}
	{/if}
	<span class="sr-only" aria-live="polite">{status === 'copied' ? 'Copied to clipboard' : ''}</span>
</button>
