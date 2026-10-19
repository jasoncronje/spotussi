# API and MCP

The dashboard runs on a documented, read-only HTTP API, and the same queries are offered to AI assistants through a built-in MCP server.

## API

- **Reference:** `/api/docs` on your instance (the OpenAPI 3.1 spec is [`api/openapi.yaml`](../api/openapi.yaml)).
- **Tokens:** create read-only API tokens in **Settings → API & MCP** and send them as `Authorization: Bearer spt_…`. They can call every `GET` endpoint under `/api/v1` except credential management and the backup download.
- **Stability:** during 0.x the API can change between releases; breaking changes are listed in the [release notes](https://github.com/jasoncronje/spotussi/releases).

## MCP

spotussi has a built-in [MCP](https://modelcontextprotocol.io) server, so Claude (or any MCP client) can answer questions about your listening: "who did I play most this summer?", "when did I first hear this album?", "what were my longest sessions last month?". The tools are read-only and run on the same query layer as the dashboard, so the answers match the UI.

- **Endpoint:** `https://<your-spotussi>/mcp` (Streamable HTTP).
- **Auth:** an API token, sent as `Authorization: Bearer spt_…`. Create one per client in **Settings → API & MCP**; revoking it cuts that client off at once. The same tab shows this instance's API and MCP URLs with copy-ready setup commands. Browser sign-ins are not accepted at `/mcp`. With `AUTH_MODE=none`, no token is needed (as for the API).
- **stdio:** for clients that only launch local commands, `spotussi mcp --url https://<your-spotussi>` runs the same binary as a stdio proxy to `/mcp`. It reads the token from `SPOTUSSI_TOKEN` (or `--token`, but flags show up in `ps`).

### Claude Code

```bash
claude mcp add --transport http spotussi https://stats.example.com/mcp --header "Authorization: Bearer spt_your_token"
```

Or in a project's `.mcp.json` (the token comes from your environment):

```json
{
  "mcpServers": {
    "spotussi": {
      "type": "http",
      "url": "https://stats.example.com/mcp",
      "headers": { "Authorization": "Bearer ${SPOTUSSI_TOKEN}" }
    }
  }
}
```

### Claude Desktop

Claude Desktop launches local servers over stdio, so point it at the spotussi binary (a release binary for your machine; the server it talks to can be anywhere). In `claude_desktop_config.json` (Settings → Developer → Edit Config):

```json
{
  "mcpServers": {
    "spotussi": {
      "command": "/usr/local/bin/spotussi",
      "args": ["mcp", "--url", "https://stats.example.com"],
      "env": { "SPOTUSSI_TOKEN": "spt_your_token" }
    }
  }
}
```

Remote connectors added in Claude's settings sign in with OAuth, which spotussi does not offer (it only takes API tokens), so use the stdio route above.

### Tools

| Tool | What it answers |
|---|---|
| `get_status` | Timezone, play rule, how much history exists, whether capture, imports, catalog enrichment or a rebuild are running, now playing |
| `get_overview(range, metric)` | Plays, minutes, skips, unique tracks/artists/albums, change vs the previous period, top 5 of each kind, plays by hour of day |
| `get_top(type, range, metric, limit, offset, filter)` | Ranked tracks, artists, albums, genres or contexts (playlists/albums/artists played from), with previous rank |
| `get_entity(kind, id, range, metric)` | One artist, album, track or genre: rank and totals in the range, all-time totals, first/last play, related top 5 lists |
| `get_timeline(range, bucket, filter)` | Plays, minutes, skips and artists per hour/day/week/month/year |
| `get_history(range, filters, cursor, limit)` | Individual plays, newest first, filterable by entity, skipped, shuffle, counted, device, platform |
| `get_sessions(range, limit, offset, session_id)` | Longest listening sessions, or one session with its plays |
| `search_catalog(query, limit)` | Artists, albums and tracks by name, to get the IDs the other tools take |
| `get_library(view, played, sort, relation, playlist_id, limit, offset)` | Your saved songs, albums and playlists with their plays: a summary (never-played share, days from saving to first play and to heavy rotation), saved songs filtered by played / other release / never played, saved albums, playlists with how much of each you play, or one playlist's songs |

Ranges are the dashboard's presets (`today`, `this_week`, `this_month`, `this_year`, `last_7d`, `last_4w`, `last_6m`, `last_12m`, `prev_day`, `prev_week`, `prev_month`, `prev_year`, `all`) or `custom` with `from` and `to` (inclusive local dates). Results are compact JSON with names and IDs; durations are minutes (aggregates) or seconds (single plays), and times are RFC 3339 in your display timezone. Lists are capped (top 50, history 100, sessions 50, session plays 100, timeline 400 points, library 50).

### Behind a forward-auth proxy

In `AUTH_MODE=proxy`, MCP clients cannot pass your SSO login, so route `/mcp` and token requests around the forward-auth middleware: spotussi checks the token itself. [install.md](install.md#single-sign-on-forward-auth) has the Traefik router for it.
