<!--
	A small choice between a few options (a chart's measure, a History filter),
	drawn like the shell's selectors: a recessed track, the chosen option
	raised and lit. Native radio inputs, so arrow keys and forms just work.
-->
<script lang="ts" generics="T extends string">
	import Lamp from '#lib/components/shell/lamp.svelte';

	let {
		label,
		options,
		value,
		onchange,
		size = 'sm',
		class: className = ''
	}: {
		/** Accessible name of the group. */
		label: string;
		options: { value: T; label: string; hint?: string }[];
		value: T;
		onchange: (v: T) => void;
		size?: 'sm' | 'md';
		class?: string;
	} = $props();

	const name = $props.id();
</script>

<fieldset class="inline-flex min-w-0 rounded-lg bg-muted p-[3px] {className}">
	<legend class="sr-only">{label}</legend>
	{#each options as o (o.value)}
		{@const on = o.value === value}
		<label
			title={o.hint}
			class="relative flex cursor-pointer items-center gap-1.5 rounded-md px-2.5 font-medium whitespace-nowrap text-muted-foreground transition-colors select-none hover:text-foreground has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-ring {size ===
			'sm'
				? 'h-7 text-xs'
				: 'h-8 text-sm'} {on
				? 'bg-card text-foreground shadow-[0_1px_0_var(--border),0_0_0_1px_var(--border)]'
				: ''}"
		>
			<input
				type="radio"
				{name}
				value={o.value}
				checked={on}
				onchange={() => onchange(o.value)}
				class="sr-only"
			/>
			<Lamp {on} />
			{o.label}
		</label>
	{/each}
</fieldset>
