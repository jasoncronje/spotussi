<!--
	Indicator lamp: the "selected" signal used by every selector in the app
	(range, metric, navigation), like the LED over a hi-fi input button.
	Decorative; selection is also exposed via aria-current / aria-checked.
-->
<script lang="ts">
	let { on = false, class: className = '' }: { on?: boolean; class?: string } = $props();
</script>

<span
	aria-hidden="true"
	class="inline-block size-[5px] shrink-0 rounded-full transition-[background-color,box-shadow] duration-150 {on
		? 'bg-signal shadow-[0_0_0_3px_var(--signal-soft)]'
		: 'bg-grille'} {className}"
></span>
