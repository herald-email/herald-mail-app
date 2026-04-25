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
