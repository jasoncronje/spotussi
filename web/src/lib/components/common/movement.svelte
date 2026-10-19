<!-- Rank movement vs the previous period: ▲ 3, ▼ 2, new, or = for unchanged. -->
<script lang="ts">
	let {
		rank,
		prevRank,
		hasPrevious = true,
		class: className = ''
	}: { rank: number; prevRank: number | null; hasPrevious?: boolean; class?: string } = $props();

	const m = $derived.by(() => {
		if (!hasPrevious) return null;
		if (prevRank === null)
			return { glyph: '', text: 'new', label: 'New since the previous period' };
		const d = prevRank - rank;
		if (d === 0) return { glyph: '=', text: '', label: 'Same rank as the previous period' };
		return d > 0
			? { glyph: '▲', text: String(d), label: `Up ${d} from #${prevRank}` }
			: { glyph: '▼', text: String(-d), label: `Down ${-d} from #${prevRank}` };
	});
</script>

{#if m}
	<span
		class="tnum inline-flex items-center gap-0.5 text-2xs leading-none {m.text === 'new'
			? 'font-semibold text-signal-ink'
			: 'text-muted-foreground'} {className}"
		title={m.label}
	>
		<span class="sr-only">{m.label}</span>
		<span aria-hidden="true" class={m.glyph === '=' ? '' : 'text-[0.7em]'}>{m.glyph}</span>
		<span aria-hidden="true">{m.text}</span>
	</span>
{/if}
