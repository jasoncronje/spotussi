#!/usr/bin/env bash
# Claude Code PreToolUse hook (Bash): the marketing site (site/, the `spotussi`
# Worker in the root wrangler.jsonc) deploys only through Cloudflare Workers
# Builds from main (D35, owner 2026-11-20), the same as the other Ussi sites.
# Blocks `wrangler deploy` (and `versions deploy`, `triggers deploy`,
# `publish`) anywhere in the repo: wrangler.jsonc is the only Worker here.
# `--dry-run`, `rollback` and `deployments` stay allowed.
cmd=$(jq -r '.tool_input.command // empty')
[ -z "$cmd" ] && exit 0
if printf '%s' "$cmd" | grep -Ezq '(^|[;&|(]|npx|bunx|dlx|\.bin/)[[:space:]]*(--yes[[:space:]]+|-y[[:space:]]+)?wrangler(@[^[:space:]]+)?[[:space:]]+((versions|triggers)[[:space:]]+)?(deploy|publish)([[:space:]]|$|;|&|\|)' \
   && ! printf '%s' "$cmd" | grep -q -- '--dry-run'; then
  echo "Blocked: the spotussi Worker (spotussi.app) deploys only through Cloudflare Workers Builds (land the change on main). Do not run wrangler deploy by hand. To undo a bad deploy: npx wrangler@4 rollback <version-id> -m \"why\". See site/README.md, \"Deploy\"." >&2
  exit 2
fi
exit 0
