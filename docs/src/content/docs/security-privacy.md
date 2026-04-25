---
title: Security & Privacy
description: Understand what Herald stores locally and when data leaves your machine.
---

Herald is designed around local-first email access. The TUI connects to your configured IMAP and SMTP servers, caches metadata and selected body text in SQLite, and uses local AI by default when AI is enabled.

## Local files

Default config:

```sh
~/.herald/conf.yaml
```

The config can contain credentials, app passwords, OAuth refresh tokens, AI provider keys, and the configured cache path. Keep it private:

```sh
chmod 600 ~/.herald/conf.yaml
```

SQLite cache paths are stored in the config. By default, generated cache files live under `herald/cached/` relative to the working directory.

## Logs

Herald writes logs to files only, never to the terminal. Default locations:

| Platform | Location |
| --- | --- |
| macOS | `~/Library/Logs/Herald` |
| Linux/BSD | `${XDG_STATE_HOME:-~/.local/state}/herald/logs` |
| Windows | `%LOCALAPPDATA%\Herald\Logs` |

Set `HERALD_LOG_DIR` to override the log directory.

## AI behavior

Ollama runs locally and is the default path for classification, chat, quick replies, and semantic search. If you configure Claude or an OpenAI-compatible provider, prompts and relevant email context may be sent to that provider for the feature you invoke.

Semantic search stores embeddings in the local SQLite cache. Embeddings are tied to the configured embedding model so Herald can invalidate stale vectors when the model changes.

## MCP behavior

The MCP server runs over stdio. It exposes cached email data to whatever AI client you connect it to. Configure it only in clients you trust, and remember that the client may include returned email data in its own model requests.

## Deletion and archive

Delete operations copy mail to a Trash folder when possible, mark the original message deleted, expunge it, and remove the corresponding cache row. Archive operations move mail through the configured IMAP backend and update local state.
