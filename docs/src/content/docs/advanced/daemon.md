---
title: Daemon Commands
description: Run, inspect, stop, and sync through Herald's local HTTP daemon.
---

The Herald daemon is a local HTTP server that owns a backend connection, cache access, SSE events, and write endpoints used by MCP and optional SSH remote mode. It is available through subcommands on the main Herald binary.

## Commands

```sh
./bin/herald serve -config ~/.herald/conf.yaml
./bin/herald status
./bin/herald stop
./bin/herald sync
./bin/herald sync Archive
```

| Command | Result |
| --- | --- |
| `serve` | Starts the daemon in the foreground, writes pidfile, initializes backend, and listens on configured bind/port. |
| `status` | Reads pidfile, checks process state, and calls `/v1/status`. |
| `stop` | Sends SIGTERM to the pidfile process and waits for pidfile removal. |
| `sync` | Calls `/v1/sync` for `INBOX` by default or a folder argument when provided. |

<!-- HERALD_SCREENSHOT id="daemon-status-output" page="daemon" alt="Herald daemon status output" state="local shell, daemon running" desc="Shows status command output with pid, uptime, version, or HTTP reachability details." capture="terminal; ./bin/herald serve in one terminal; ./bin/herald status in another" deferred="true" reason="requires live daemon process" -->

## HTTP Surface

The daemon exposes local endpoints for:

- Status, sync, SSE events, and folders.
- Email list, body, read/unread, star/unstar, delete, archive, move, classify, reply, forward, attachments, and unsubscribe.
- Threads, bulk delete/move, sender archive/delete/soft-unsubscribe.
- Stats, search, semantic search, classifications, rules, prompts, cleanup rules, dry-run previews, and drafts.

Dry-run planning is available through `POST /v1/rules/dry-run` for automation rules and `POST /v1/cleanup-rules/dry-run` for cleanup rules. These endpoints return matched messages and planned actions without mutating IMAP mail or updating rule metadata; live cleanup execution still goes through `POST /v1/cleanup-rules/run`.

The default bind address is `127.0.0.1` and the default port is `7272`.

## TUI and MCP Behavior

The local TUI tries to connect to a running daemon first and falls back to direct local backend if the daemon is unavailable. The MCP server probes the daemon and uses it for live or mutating tools.

## Data And Privacy

The daemon keeps IMAP/SMTP access on the machine where it runs. It exposes a local HTTP API, writes a pidfile and log file according to config, uses the same SQLite cache, and can mutate mail through HTTP requests. Keep the bind address on loopback unless you intentionally protect and expose it.

## Troubleshooting

If `status` reports a stale pidfile, remove it through `stop` behavior or delete the configured pidfile after verifying no daemon is running.

If `sync` fails, confirm the daemon is running and that `daemon.bind`/`daemon.port` match the config used by the command.

If MCP write tools fail, start the daemon before starting the MCP client.

If `serve` crashes with `panic: parsing "POST /v1/folders/{name...}/rename": ... wildcard not at end`, upgrade Herald. That panic came from an older invalid Go `ServeMux` route pattern and prevents every daemon-backed surface from starting.

## Related Pages

- [MCP Server](/advanced/mcp/)
- [SSH Mode](/advanced/ssh-mode/)
- [Sync and Status](/features/sync-status/)
- [Config Reference](/reference/config/)
