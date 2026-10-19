<!--
	API tokens for scripts and assistants (D13): name one, see its secret
	exactly once with a copy button, list them with when they were last
	used, revoke any.
-->
<script lang="ts">
	import KeyRound from '@lucide/svelte/icons/key-round';
	import SettingsPanel from './settings-panel.svelte';
	import CopyButton from '#lib/components/common/copy-button.svelte';
	import ConfirmDialog from '#lib/components/common/confirm-dialog.svelte';
	import { api, type Schemas } from '#lib/api/client.js';
	import { unwrap } from '#lib/data/queries.js';
	import { ago, dateAt } from '#lib/format.js';

	let { tokens, onchange }: { tokens: Schemas['ApiToken'][] | undefined; onchange: () => void } =
		$props();

	let name = $state('');
	let creating = $state(false);
	let error = $state('');
	let created = $state<Schemas['ApiTokenCreated'] | null>(null);

	async function create(e: SubmitEvent) {
		e.preventDefault();
		const n = name.trim();
		if (!n) {
			error = 'Enter a name.';
			return;
		}
		creating = true;
		error = '';
		try {
			created = await unwrap(api.POST('/auth/tokens', { body: { name: n } }));
			name = '';
			onchange();
		} catch (err) {
			error = (err as Error).message;
		} finally {
			creating = false;
		}
	}

	let revoking = $state<Schemas['ApiToken'] | null>(null);
	let confirmOpen = $state(false);
	async function revoke() {
		if (!revoking) return;
		await unwrap(api.DELETE('/auth/tokens/{id}', { params: { path: { id: revoking.id } } }));
		if (created?.token.id === revoking.id) created = null;
		onchange();
	}
	const field =
		'h-9 rounded-lg border bg-background px-3 text-base outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/40 sm:text-sm';
</script>

<SettingsPanel
	id="tokens"
	title="API Tokens"
	hint="Read-only. One per client, so you can revoke one alone."
>
	<div class="grid gap-5">
		{#if created}
			<div
				role="status"
				class="grid gap-4 rounded-xl border border-signal/45 bg-signal-soft p-4 sm:p-5"
			>
				<div class="flex items-start gap-3">
					<span
						class="grid size-9 shrink-0 place-items-center rounded-full bg-signal text-[var(--window)]"
						aria-hidden="true"><KeyRound class="size-4" /></span
					>
					<div class="min-w-0">
						<p class="truncate font-medium">{created.token.name}</p>
						<p class="text-sm text-muted-foreground">Copy it now. It's shown only once.</p>
					</div>
				</div>
				<div class="flex items-center gap-2 rounded-lg border bg-background py-1.5 pr-1.5 pl-3">
					<code class="min-w-0 flex-1 truncate font-mono text-sm select-all" title={created.secret}
						>{created.secret}</code
					>
					<CopyButton value={created.secret} label="Copy token" />
				</div>
				<button
					type="button"
					onclick={() => (created = null)}
					class="h-9 w-fit rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground"
					>Done</button
				>
			</div>
		{:else}
			<form onsubmit={create} class="grid gap-1.5" novalidate>
				<label for="token-name" class="text-sm font-medium">New Token Name</label>
				<div class="flex flex-wrap gap-2">
					<input
						id="token-name"
						bind:value={name}
						maxlength="80"
						placeholder="Claude Desktop"
						autocomplete="off"
						aria-invalid={error ? 'true' : undefined}
						class="{field} min-w-0 flex-1 sm:max-w-sm"
					/>
					<button
						type="submit"
						disabled={creating}
						class="h-9 rounded-lg bg-primary px-3.5 text-sm font-medium text-primary-foreground disabled:opacity-60 max-sm:w-full"
						>{creating ? 'Creating…' : 'Create Token'}</button
					>
				</div>
				{#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
			</form>
		{/if}

		{#if tokens && tokens.length === 0}
			<p class="text-sm text-muted-foreground">No tokens yet.</p>
		{:else if tokens}
			<div class="overflow-hidden rounded-lg border">
				<table class="w-full table-fixed text-sm">
					<thead class="bg-muted/50 text-left text-xs text-muted-foreground max-sm:sr-only">
						<tr>
							<th scope="col" class="px-4 py-2 font-medium">Name</th>
							<th scope="col" class="w-36 px-4 py-2 font-medium">Created</th>
							<th scope="col" class="w-36 px-4 py-2 font-medium">Last Used</th>
							<th scope="col" class="w-24 px-4 py-2"><span class="sr-only">Actions</span></th>
						</tr>
					</thead>
					<tbody class="divide-y divide-border/70">
						{#each tokens as t (t.id)}
							<tr class="align-middle">
								<td class="min-w-0 px-4 py-3">
									<p class="truncate font-medium">{t.name}</p>
									<p class="truncate font-mono text-xs text-muted-foreground">{t.prefix}…</p>
									<p class="mt-0.5 text-xs text-muted-foreground sm:hidden">
										Created {dateAt(t.created_at)}, {t.last_used_at
											? `used ${ago(t.last_used_at)}`
											: 'never used'}
									</p>
								</td>
								<td class="px-4 py-3 text-muted-foreground max-sm:hidden">{dateAt(t.created_at)}</td
								>
								<td class="px-4 py-3 max-sm:hidden">
									{#if t.last_used_at}{ago(t.last_used_at)}{:else}<span
											class="text-muted-foreground">Never</span
										>{/if}
								</td>
								<td class="py-3 pr-3 text-right max-sm:w-24">
									<button
										type="button"
										onclick={() => {
											revoking = t;
											confirmOpen = true;
										}}
										class="h-8 rounded-md border px-2.5 text-xs font-medium text-destructive transition-colors hover:border-destructive/40 hover:bg-destructive/10"
										>Revoke<span class="sr-only"> {t.name}</span></button
									>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>
</SettingsPanel>

<ConfirmDialog
	bind:open={confirmOpen}
	title="Revoke this token?"
	confirmLabel="Revoke token"
	destructive
	onconfirm={revoke}
>
	<p>
		Anything using “{revoking?.name}” stops working. This can't be undone.
	</p>
</ConfirmDialog>
