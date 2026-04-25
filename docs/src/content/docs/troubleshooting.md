---
title: Troubleshooting
description: Fix common setup, build, IMAP, AI, and terminal rendering problems.
---

## Config permission warning

Herald warns when `~/.herald/conf.yaml` is readable by group or other users.

```sh
chmod 600 ~/.herald/conf.yaml
```

## SQLite or CGO build errors

Herald uses `go-sqlite3`, which requires CGO and a C compiler.

On macOS, install command line tools:

```sh
xcode-select --install
```

Then rebuild:

```sh
make build
```

## IMAP connection fails

Check:

- IMAP host and port
- App password or provider-specific password
- Whether IMAP is enabled for the account
- Whether a local bridge, such as Proton Mail Bridge, is running
- Whether the config path passed with `-config` is the one you edited

Use debug logging for more detail:

```sh
./bin/herald -debug
```

## SMTP send fails

Verify `smtp.host`, `smtp.port`, username, and password. Herald tries TLS-first behavior and can fall back to STARTTLS for providers that expect it.

## AI unavailable

For Ollama, check that the server is running:

```sh
curl http://localhost:11434/api/tags
```

Then confirm your config includes:

```yaml
ollama:
  host: "http://localhost:11434"
  model: "gemma3:4b"
  embedding_model: "nomic-embed-text-v2-moe"
```

If AI is unavailable, Herald should keep the TUI responsive and show concise AI status instead of blocking mailbox use.

## Body not cached in MCP

Some MCP tools need cached body text. Open the email in the TUI first so Herald fetches and caches the body, then retry the MCP tool.

## Terminal layout looks wrong

Try a larger terminal first. The TUI is responsive, but very small terminals can trigger fallback layouts.

Useful sizes for layout checks:

```sh
tmux new-session -d -s herald-doc-check -x 220 -y 50
tmux resize-window -t herald-doc-check -x 80 -y 24
tmux resize-window -t herald-doc-check -x 50 -y 15
```

If rendering artifacts appear, capture the pane and include the terminal size in the bug report.
