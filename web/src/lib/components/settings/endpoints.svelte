<!--
	Where API tokens are used (D16, D29): the /api/v1 base with its docs, and
	the MCP endpoint with ready-to-paste setup for Claude Code (HTTP) and
	stdio-only clients such as Claude Desktop (the `spotussi mcp` proxy).
	URLs come from the address the browser is using.
-->
<script lang="ts">
	import { page } from '$app/state';
	import CopyButton from '#lib/components/common/copy-button.svelte';
	import SettingsPanel from './settings-panel.svelte';

	const origin = $derived(page.url.origin);
	const apiURL = $derived(`${origin}/api/v1`);
	const mcpURL = $derived(`${origin}/mcp`);
	const claudeCode = $derived(
		`claude mcp add --transport http spotussi ${mcpURL} --header "Authorization: Bearer <token>"`
	);
	const stdio = $derived(`SPOTUSSI_TOKEN=<token> spotussi mcp --url ${origin}`);
</script>

{#snippet line(label: string, value: string, copyLabel: string)}
	<div class="grid grid-cols-[minmax(0,1fr)] gap-1.5">
		<p class="text-sm font-medium">{label}</p>
		<div class="flex items-center gap-2">
			<code
				class="w-full min-w-0 flex-1 truncate rounded-md border bg-muted/40 px-3 py-2 font-mono text-xs whitespace-nowrap select-all"
				title={value}>{value}</code
			>
			<CopyButton {value} label={copyLabel} />
		</div>
	</div>
{/snippet}

<SettingsPanel id="endpoints" title="Endpoints" hint="Authorization: Bearer <token>">
	<div class="grid grid-cols-[minmax(0,1fr)] gap-5">
		<div class="grid grid-cols-[minmax(0,1fr)] gap-1.5">
			{@render line('REST API', apiURL, 'Copy API URL')}
			<p class="text-xs text-muted-foreground">
				<a
					href="/api/docs"
					target="_blank"
					rel="noopener"
					class="underline underline-offset-2 hover:text-foreground">API docs</a
				>
			</p>
		</div>

		{@render line('MCP server', mcpURL, 'Copy MCP URL')}

		<div class="grid grid-cols-[minmax(0,1fr)] gap-4 border-t pt-4">
			{@render line('Claude Code', claudeCode, 'Copy command')}
			<div class="grid grid-cols-[minmax(0,1fr)] gap-1.5">
				{@render line('Claude Desktop (stdio)', stdio, 'Copy command')}
				<p class="text-xs text-muted-foreground">Needs spotussi installed locally.</p>
			</div>
		</div>
	</div>
</SettingsPanel>
