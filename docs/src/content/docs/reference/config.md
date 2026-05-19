---
title: Config Reference
description: Reference for Herald YAML config fields, defaults, provider presets, and file locations.
---

Herald reads YAML config from `~/.herald/conf.yaml` by default or from the path passed with `-config`. The first-run wizard and settings overlay write the same config shape, but account setup writes or applies account fields only after IMAP and SMTP validation pass.

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
compose:
  signature:
    text: "Best,\nYour Name"
keyboard:
  profile: default
  custom_keymap: ""
theme:
  name: inherited
  overrides:
    chrome.tab_active:
      fg: "#ffffff"
      bg: "#1a73e8"
      bold: true
ollama:
  host: "http://localhost:11434"
  model: "gemma3:4b"
  embedding_model: "nomic-embed-text-v2-moe"
semantic:
  enabled: false
```

## Fields

| YAML path | Purpose |
| --- | --- |
| `vendor` | Optional provider shortcut: `gmail`, `protonmail`, `fastmail`, `outlook`, or `icloud`. |
| `cache.database_path` | SQLite cache file path. Generated as an absolute `<home>/.herald/cached/<config-name>.db` path when missing. |
| `compose.signature.text` | Optional default signature inserted into new Compose messages, replies, forwards, and quick replies. |
| `keyboard.profile` | Keyboard profile: `default`, `vim`, `emacs`, or `custom`. Default `default`. |
| `keyboard.custom_keymap` | Optional path to a YAML custom keymap file when `keyboard.profile` is `custom`. |
| `theme.name` | Theme name: `inherited`, `herald-dark`, `herald-light`, another built-in theme such as `jade-signal`, or a local theme installed in `~/.herald/themes`. Default `inherited`. |
| `theme.overrides` | Optional semantic role overrides keyed by role ID, e.g. `chrome.tab_active`. Colors accept `inherit`, `ansi:N`, `xterm:N`, or quoted `#RRGGBB`. |
| `credentials.username` | IMAP/SMTP username or provider/bridge username. |
| `credentials.password` | Password, app password, or bridge password. |
| `server.host` | IMAP host. Required for non-demo mode. |
| `server.port` | IMAP port. Required for non-demo mode. |
| `smtp.host` | SMTP host for Compose send. |
| `smtp.port` | SMTP port for Compose send. |
| `ollama.host` | Ollama base URL. |
| `ollama.model` | Local chat/classification model. Default `llama3.2:1b`. Setup and changed AI settings verify that the model is installed locally before saving. |
| `ollama.embedding_model` | Local embedding model. Default `nomic-embed-text`. Setup and changed AI settings verify that the model is installed locally before saving. |
| `sync.interval` | Fallback poll seconds. Default `60`. |
| `sync.poll_interval_minutes` | Poll interval in minutes for settings-oriented sync. |
| `sync.idle_enabled` | Enables IMAP IDLE when supported. |
| `sync.background` | Enables background sync of other folders. |
| `sync.notify` | Enables status notification behavior. |
| `semantic.enabled` | Enables semantic search indexing and automatic background embedding/contact enrichment when AI is configured. Keep `false` to avoid background body fetches and embedding work. |
| `semantic.model` | Embedding model name. Defaults to Ollama embedding model. When Ollama is selected, setup validates this effective embedding model and shows `ollama pull <model>` when it is missing. |
| `semantic.batch_size` | Embedding batch size. Default `20`. |
| `semantic.min_score` | Minimum semantic result score. Default `0.30`. |
| `gmail.access_token` | Gmail OAuth access token. |
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

Existing Ollama configs are checked at startup without blocking cached/offline mail. If a configured local model is unavailable, Herald shows `AI down`, disables AI actions, and lists the relevant `ollama pull <model>` commands in Settings > AI.

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

## Theme Files

Local theme files can be installed from Settings. A validated theme is copied to `~/.herald/themes/<name>.yaml` with private permissions.

```yaml
version: 1
name: quiet-slate
display_name: Quiet Slate
inherits: herald-dark
roles:
  focus.panel_border_focused:
    fg: "#55c2ff"
```

Use the [Jade Signal baseline YAML](/examples/themes/jade-signal-baseline.yaml) as a complete starter file when building your own theme. It uses `name: my-jade-signal` so it can be installed as a local theme without colliding with Herald's built-in `jade-signal`.

Use `-theme` to launch a built-in theme or theme file for one session without changing the config:

```sh
./bin/herald --demo -theme jade-signal
./bin/herald --demo -theme ./jade-signal-baseline.yaml
./bin/herald -theme ./quiet-slate.yaml
```

To install a local file from the TUI, open Settings with `S`, choose `Theme Selection`, and enter the YAML path in `Install local theme YAML`. After installation, set `theme.name` to the file's slug, for example `my-jade-signal`.

## Related Pages

- [First-run Wizard](/first-run-wizard/)
- [Settings](/features/settings/)
- [Themes](/themes/)
- [Provider Setup](/provider-setup/)
- [Daemon Commands](/advanced/daemon/)
- [Privacy and Security](/security-privacy/)
