---
title: MCP Setup
description: Expose Herald email search and management tools to AI clients over stdio.
---

Herald ships a standalone MCP server in `cmd/herald-mcp-server`. It reads the configured SQLite cache and exposes email tools over stdio.

## Build

```sh
go build -o bin/herald-mcp-server ./cmd/herald-mcp-server
```

Use the same config path as the TUI:

```sh
./bin/herald-mcp-server -config ~/.herald/conf.yaml
```

## Quick smoke test

```sh
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | ./bin/herald-mcp-server -config ~/.herald/conf.yaml
```

If the cache is empty, open the TUI first and let Herald sync at least one folder.

## Readiness checklist

Use the same `-config` path for the TUI, daemon, and MCP server. Cache-only tools can run after Herald has synced mail; live IMAP/SMTP tools need the daemon.

| Capability | Requirement |
| --- | --- |
| Recent/unread mail, keyword search, sender stats, contacts, rules, and stored classifications | Run the TUI or daemon long enough to populate the SQLite cache. |
| Body lookup, summaries, action items, and draft replies | Open the email in the TUI first so body text is cached. Listing outputs include `message_id=...` for follow-up calls. |
| Semantic search, summaries, classification, and action-item extraction | Configure an AI provider such as Ollama, Claude, or OpenAI-compatible settings. |
| Sync, drafts, attachments, sending, folder changes, and mail mutations | Start `herald serve -config ~/.herald/conf.yaml`; `herald status` should report a running daemon. |

## Claude Code

```sh
claude mcp add herald -- "$(pwd)/bin/herald-mcp-server" -config ~/.herald/conf.yaml
```

## Cursor

Add to `.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "herald": {
      "command": "/path/to/herald/bin/herald-mcp-server",
      "args": ["-config", "~/.herald/conf.yaml"]
    }
  }
}
```

## Windsurf

Add to `~/.codeium/windsurf/mcp_config.json`:

```json
{
  "mcpServers": {
    "herald": {
      "command": "/path/to/herald/bin/herald-mcp-server",
      "args": ["-config", "~/.herald/conf.yaml"]
    }
  }
}
```

## Codex

```sh
CODEX_MCP_SERVERS='{"herald":{"command":"/path/to/herald/bin/herald-mcp-server","args":["-config","~/.herald/conf.yaml"]}}' codex
```

## Available tool categories

Herald includes tools for recent mail, unread mail, sender search, date search, semantic search, body lookup, classification, summaries, contacts, draft replies, and rule inspection. AI-powered tools require an AI backend, and body-based tools require cached body text.

If `herald serve` crashes with `wildcard not at end`, upgrade Herald. Older binaries had an invalid daemon route pattern and cannot unlock daemon-backed MCP tools.
