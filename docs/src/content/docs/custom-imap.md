---
title: Custom IMAP
description: Configure Herald for Proton Mail Bridge, Fastmail, iCloud, Outlook, and other IMAP providers.
---

Herald connects to mail through IMAP and sends mail through SMTP. The TUI talks to a backend interface; provider-specific details live in the config and IMAP/SMTP clients.

## Provider presets

Known presets fill IMAP and SMTP host and port values when you have not provided explicit values:

| Provider | IMAP | SMTP |
| --- | --- | --- |
| Gmail | `imap.gmail.com:993` | `smtp.gmail.com:587` |
| Proton Mail Bridge | `127.0.0.1:1143` | `127.0.0.1:1025` |
| Fastmail | `imap.fastmail.com:993` | `smtp.fastmail.com:587` |
| iCloud | `imap.mail.me.com:993` | `smtp.mail.me.com:587` |
| Outlook | `outlook.office365.com:993` | `smtp.office365.com:587` |

## Manual config

```yaml
credentials:
  username: "your@email.com"
  password: "your-password-or-app-password"
server:
  host: "imap.example.com"
  port: 993
smtp:
  host: "smtp.example.com"
  port: 587
```

Use provider-specific app passwords when your mail service requires them. Proton Mail users should run Proton Mail Bridge locally and use the Bridge-generated username and password.

## Cache path

Herald stores a SQLite cache path in config:

```yaml
cache:
  database_path: "~/.herald/cached/conf.db"
```

If the path is missing, Herald creates a per-config absolute cache path under `~/.herald/cached/` and writes it back to the YAML file. This prevents different account configs from sharing one database and keeps the cache usable no matter which directory Herald starts from.

## Sync behavior

Herald uses IMAP IDLE when available and falls back to polling. Cached rows can render quickly while live IMAP work continues, so startup stays usable even when a mailbox is large.
