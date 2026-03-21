# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A terminal-based email client and inbox cleanup tool written in Go, connecting to IMAP servers via ProtonMail Bridge. Built with Bubble Tea TUI framework.

## Architecture

### Go Implementation

The app initializes in `main.go` -> `app.New()` -> starts a Bubble Tea program with alt-screen. On startup it connects to IMAP, processes new emails into SQLite cache, runs `CleanupCache` to sync stale entries, then renders the main view.

A persistent IMAP connection is held open for the lifetime of the app (not reconnected per operation). A background `deletionWorker` goroutine reads from a buffered channel and processes deletions serially, sending results back through a result channel for UI updates.

### Project Structure

```
internal/
├── app/
│   ├── app.go       # Bubble Tea Model: Init/Update/View, state, message types
│   ├── helpers.go   # Table updates, deletion queue, navigation, domain toggle
│   └── logs.go      # LogViewer TUI component (viewport-based)
├── cache/
│   └── cache.go     # SQLite CRUD: GetCachedIDs, CacheEmail, GetAllEmails, Delete*
├── config/
│   └── config.go    # YAML config load/validate, file permission check
├── imap/
│   ├── client.go    # IMAP connect, ProcessEmails, GetSenderStatistics
│   └── delete.go    # DeleteSenderEmails, DeleteDomainEmails, DeleteEmail, CleanupCache
├── logger/
│   └── logger.go    # File-based logger with callback for TUI log viewer
└── models/
    └── email.go     # EmailData, SenderStats, ProgressInfo, DeletionRequest/Result
```

### Key Features

- **Email Grouping**: Groups emails by sender or domain for bulk analysis
- **SQLite Caching**: Caches email metadata in `email_cache.db`; only fetches new messages on subsequent launches
- **Interactive Deletion**: Single email, selected senders, or domain-wide deletion (copies to Trash then expunges)
- **TUI Interface**: Split-pane view (summary table | details table) with real-time log viewer overlay
- **Domain Mode**: `d` key toggles grouping by domain (e.g. `example.com`) vs full email address

## Common Commands

### Go Version
```bash
# Build and run
make build && ./bin/mail-processor

# Or run directly
make run

# CLI flags
./bin/mail-processor -debug              # Enable debug logging
./bin/mail-processor -config custom.yaml # Custom config file
./bin/mail-processor -help

# Development
make deps     # Install dependencies
make fmt      # Format code
make test     # Run tests
```

### Configuration
Both versions use `proton.yaml`:
```yaml
credentials:
  username: "your_email@mail.com"
  password: "your_password"
server:
  host: "imap.mail.com"
  port: 993
```

Config file permissions are checked at startup; warns if group/others have access (chmod 600 recommended).

### Dependencies

Go 1.23+ required. `go-sqlite3` requires CGO (`gcc`/`clang` must be present).

| Library | Version | Purpose |
|---------|---------|---------|
| `charmbracelet/bubbletea` | v1.3.4 | TUI framework (Elm architecture) |
| `charmbracelet/bubbles` | v0.21.0 | Table and viewport components |
| `charmbracelet/lipgloss` | v1.1.0 | Terminal styling |
| `emersion/go-imap` | v1.2.1 | IMAP client |
| `mattn/go-sqlite3` | v1.14.18 | SQLite driver (CGO) |
| `gopkg.in/yaml.v3` | v3.0.1 | Config parsing |

## Key TUI Bindings

| Key | Action |
|-----|--------|
| `q` / `ctrl+c` | Quit |
| `d` | Toggle domain/sender grouping mode |
| `r` | Refresh (reconnect + re-process) |
| `D` | Delete selected or current sender/message |
| `space` | Toggle selection (sender row or individual message) |
| `tab` | Switch focus between summary and details tables |
| `enter` | Load details for currently highlighted sender |
| `up`/`k`, `down`/`j` | Navigate |
| `l` / `L` | Toggle real-time log viewer overlay |

## Development Notes

### Database Schema
```sql
CREATE TABLE emails (
    message_id      TEXT PRIMARY KEY,
    uid             INTEGER,
    sender          TEXT,
    subject         TEXT,
    date            DATETIME,
    size            INTEGER,
    has_attachments INTEGER,
    folder          TEXT,
    last_updated    DATETIME
)
```
Cache file: `email_cache.db` (created in working directory).

### Logging
- Log file: `mail_processor_YYYYMMDD_HHMMSS.log` (created in working directory)
- Always writes to file only (no console output, preserves TUI)
- TUI log viewer receives logs via `logger.SetLogCallback`
- `-debug` or `-verbose` flags enable DEBUG-level entries

### Deletion Flow
1. `deleteSelected()` sends `DeletionRequest` structs to `deletionRequestCh`
2. `deletionWorker()` goroutine calls the appropriate `imap.Client.Delete*` method
3. Delete methods: fetch all envelope headers, match by sender/domain/message-ID, copy to Trash (tries `Trash`, `Deleted Items`, `[Gmail]/Trash`, `INBOX.Trash`), mark `\Deleted`, expunge, then delete from SQLite cache
4. Result sent to `deletionResultCh`; UI updates immediately, then reloads stats after all pending deletions finish

### Code Patterns

**Go Version:**
- **Progress updates**: IMAP goroutine sends `models.ProgressInfo` to a buffered channel; `listenForProgress()` blocks until a message arrives then triggers re-render
- **Domain extraction**: `cache.extractDomain()` handles compound TLDs (`co.uk`, `com.au`, etc.)
- **Text sanitization**: `sanitizeText()` strips emoji/symbols while preserving Unicode letters (table display)
- **TLS**: `InsecureSkipVerify: true` is intentional for local IMAP bridge (e.g. ProtonMail Bridge)
- **Message-ID fallback**: If envelope `Message-Id` is empty, uses `uid-{UID}` as cache key
- **Attachment detection**: Recursively checks `BodyStructure.Disposition == "attachment"`

**Python Version:**
- Async/await pattern for IMAP operations to avoid blocking the TUI
- Worker threads in Textual for background email processing
- SQLite transactions for cache consistency
