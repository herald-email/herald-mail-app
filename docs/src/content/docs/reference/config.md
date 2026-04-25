---
title: Config Reference
description: Reference for Herald YAML config fields, defaults, provider presets, and file locations.
---

Herald reads YAML config from `~/.herald/conf.yaml` by default or from the path passed with `-config`. The first-run wizard and settings panel write the same config shape.

## Core Example

```yaml
credentials:
  username: "your_email@example.com"
  password: "your-password-or-app-password"
server:
  host: "imap.example.com"
  port: 993
smtp:
  host: "smtp.example.com"
  port: 587
ollama:
  host: "http://localhost:11434"
  model: "gemma3:4b"
  embedding_model: "nomic-embed-text-v2-moe"
```

## Fields

| YAML path | Purpose |
| --- | --- |
| `vendor` | Optional provider shortcut: `gmail`, `protonmail`, `fastmail`, `outlook`, or `icloud`. |
| `cache.database_path` | SQLite cache file path. Generated per config when missing. |
| `credentials.username` | IMAP/SMTP username or provider/bridge username. |
| `credentials.password` | Password, app password, or bridge password. |
| `server.host` | IMAP host. Required for non-demo mode. |
| `server.port` | IMAP port. Required for non-demo mode. |
| `smtp.host` | SMTP host for Compose send. |
| `smtp.port` | SMTP port for Compose send. |
| `ollama.host` | Ollama base URL. |
| `ollama.model` | Local chat/classification model. Default `gemma3:4b`. |
| `ollama.embedding_model` | Local embedding model. Default `nomic-embed-text-v2-moe`. |
| `sync.interval` | Fallback poll seconds. Default `60`. |
| `sync.poll_interval_minutes` | Poll interval in minutes for settings-oriented sync. |
| `sync.idle_enabled` | Enables IMAP IDLE when supported. |
| `sync.background` | Enables background sync of other folders. |
| `sync.notify` | Enables status notification behavior. |
| `semantic.enabled` | Enables semantic features when AI is configured. |
| `semantic.model` | Embedding model name. Defaults to Ollama embedding model. |
| `semantic.batch_size` | Embedding batch size. Default `20`. |
| `semantic.min_score` | Minimum semantic result score. Default `0.30`. |
| `gmail.access_token` | Gmail OAuth access token for experimental OAuth. |
| `gmail.refresh_token` | Gmail OAuth refresh token. Treat as credential. |
| `gmail.token_expiry` | OAuth access-token expiry in RFC3339 format. |
| `gmail.email` | OAuth Gmail account email. |
| `daemon.port` | Local daemon port. Default `7272`. |
| `daemon.bind` | Local daemon bind address. Default `127.0.0.1`. |
| `daemon.pid_file` | Daemon pidfile path. |
| `daemon.log_file` | Daemon log path. |
| `classification.prompts` | YAML-defined custom prompt list. |
| `classification_actions` | YAML-defined rule/action list. |
| `cleanup.schedule_hours` | Auto-cleanup schedule interval; `0` disables schedule. |
| `claude.api_key` | Claude API key. |
| `claude.model` | Claude model. Default `claude-sonnet-4-6`. |
| `openai.api_key` | OpenAI or compatible provider key. |
| `openai.base_url` | API base URL. Default `https://api.openai.com/v1`. |
| `openai.model` | OpenAI-compatible model. Default `gpt-4o`. |
| `ai.provider` | `ollama`, `claude`, `openai`, or `disabled`. Default `ollama`. |
| `ai.local_max_concurrency` | Local AI concurrency. Default `1`. |
| `ai.external_max_concurrency` | External AI concurrency. Default `4`. |
| `ai.background_queue_limit` | Background AI queue limit. Default `64`. |
| `ai.pause_background_while_interactive` | Pauses background AI while interactive tasks run. Default true. |

## Provider Presets

| Vendor | IMAP | SMTP |
| --- | --- | --- |
| `gmail` | `imap.gmail.com:993` | `smtp.gmail.com:587` |
| `protonmail` | `127.0.0.1:1143` | `127.0.0.1:1025` |
| `fastmail` | `imap.fastmail.com:993` | `smtp.fastmail.com:587` |
| `icloud` | `imap.mail.me.com:993` | `smtp.mail.me.com:587` |
| `outlook` | `outlook.office365.com:993` | `smtp.office365.com:587` |

## File Permissions

Herald checks config permissions on startup and warns if group or other users can read the file. Use:

```sh
chmod 600 ~/.herald/conf.yaml
```

## Related Pages

- [First-run Wizard](/first-run-wizard/)
- [Settings](/features/settings/)
- [Provider Setup](/provider-setup/)
- [Daemon Commands](/advanced/daemon/)
- [Privacy and Security](/security-privacy/)
