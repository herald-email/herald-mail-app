---
title: Security & Privacy
description: Understand what Herald stores locally and when data leaves your machine.
---

Herald is designed around local-first email access. The TUI connects to your configured IMAP and SMTP servers, caches metadata and selected body text in SQLite, and does not require AI for the core app.

AI is optional. Mail sync, reading, compose, search, cleanup, Calendar, and settings work without Ollama or cloud model keys. When AI is enabled, Ollama is the local default; external providers are opt-in.

## Local files

Default config:

```sh
~/.herald/conf.yaml
```

The config can contain credentials, app passwords, OAuth refresh tokens, AI provider keys, and the configured cache path. Keep it private:

```sh
chmod 600 ~/.herald/conf.yaml
```

SQLite cache paths are stored in the config. By default, generated cache files live under the current user's `~/.herald/cached/` directory and are written to YAML as absolute paths.

Herald Memories are local immutable records under `~/.herald/memories` by default. They store compact claims, evidence pointers, bounded snippets, prompt version, confidence, freshness, and optional Obsidian target metadata.

## Logs

Herald writes logs to files only, never to the terminal. Default locations:

| Platform | Location |
| --- | --- |
| macOS | `~/Library/Logs/Herald` |
| Linux/BSD | `${XDG_STATE_HOME:-~/.local/state}/herald/logs` |
| Windows | `%LOCALAPPDATA%\Herald\Logs` |

Set `HERALD_LOG_DIR` to override the log directory.

## AI behavior

Ollama runs locally and is the default path when you enable AI features. Semantic search, classification, chat, quick replies, and AI draft help require configured AI.

If you configure Claude or an OpenAI-compatible provider, prompts and relevant email context may be sent to that provider for the feature you invoke. Herald does not send mail content to external AI providers unless you configure one and use an AI-backed feature.

Semantic search stores embeddings in the local SQLite cache. Embeddings are tied to the configured embedding model so Herald can invalidate stale vectors when the model changes.

Research Mode is explicit. By default, external research queries use public identifiers such as person name, company, domain, role, or URL, and do not include private email bodies, private note text, attachments, or full thread summaries.

Herald Memories source expansion is opt-in. Calendar memories use cached events, Obsidian note ingestion is bounded to configured destination folders in the vault, and saved research-note ingestion requires explicit URLs in local Markdown notes.

## MCP behavior

The MCP server runs over stdio. It exposes cached email data to whatever AI client you connect it to. Configure it only in clients you trust, and remember that the client may include returned email data in its own model requests.

The current UI-first Herald Memories release does not expose a memory API through MCP or the daemon.

## Deletion and archive

Delete operations copy mail to a Trash folder when possible, mark the original message deleted, expunge it, and remove the corresponding cache row. Archive operations move mail through the configured IMAP backend and update local state.
