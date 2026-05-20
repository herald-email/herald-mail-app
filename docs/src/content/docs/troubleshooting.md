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

## Large import stalls or network pressure

Large mailbox imports can expose provider, bridge, or local network stalls. Herald records each long IMAP fetch phase with folder, range, attempt, duration, retryability, and error details when debug logging is enabled.

Use debug logging while reproducing the issue:

```sh
./bin/herald -debug -config ~/.herald/conf.yaml
```

If import progress stops, include the latest log lines containing `IMAP command failed`, `imap command timed out`, `fetch envelopes`, `uid fetch new range`, `fetch message details`, or `uid fetch all flags` in the bug report. Demo mode should log that it uses offline fixtures, does not open IMAP/Ollama/external HTTP connections on startup, and disables background semantic indexing.

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

## Demo, virtual lab, or live config?

Use the smallest realistic surface that can prove the issue:

- Demo mode is synthetic and best for UI smoke checks, screenshots, and presentations.
- The internal virtual mail lab is for development tests that need realistic MIME, calendar, inline image, draft, reply, or send behavior without private mail.
- Live config is for provider-specific behavior such as OAuth, bridge quirks, server folders, throttling, or production IMAP/SMTP differences.

Bug reports and test reports should say which surface reproduced the issue. If a virtual-lab fixture exists for a failure, include its scenario name so the bug can be replayed without personal data.

## Terminal layout looks wrong

Try a larger terminal first. The TUI is responsive, but very small terminals can trigger fallback layouts.

Useful sizes for layout checks:

```sh
tmux new-session -d -s herald-doc-check -x 220 -y 50
tmux resize-window -t herald-doc-check -x 80 -y 24
tmux resize-window -t herald-doc-check -x 50 -y 15
```

If rendering artifacts appear, capture the pane and include the terminal size in the bug report.

## Terminal images do not render

Inline raster images depend on terminal graphics support. Native iTerm2, Ghostty, and Kitty checks are authoritative for exact placement. The ttyd browser harness is useful for repeatable browser-visible pixel checks, but stock ttyd can relocate or omit later images and should be treated as a smoke lane. Terminals without supported raster protocols should show safe placeholders or open-image links instead of corrupting layout.
