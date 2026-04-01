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
cmd/
├── ssh-server/main.go  # Serve the TUI over SSH via charmbracelet/wish (port 2222)
└── mcp-server/main.go  # MCP JSON-RPC stdio server exposing email tools to Claude
internal/
├── ai/
│   └── ollama.go    # Ollama HTTP client: Classify() + Chat() via /api/generate and /api/chat
├── app/
│   ├── app.go       # Bubble Tea Model: Init/Update/View, state, all message types
│   ├── helpers.go   # Tables, deletion queue, navigation, render functions, classify/chat
│   └── logs.go      # LogViewer TUI component (viewport-based)
├── backend/
│   ├── backend.go   # Backend interface decoupling UI from IMAP
│   └── local.go     # LocalBackend: direct IMAP + SQLite cache
├── cache/
│   └── cache.go     # SQLite CRUD: emails table + email_classifications table
├── config/
│   └── config.go    # YAML config load/validate; fields: credentials, server, smtp, ollama
├── imap/
│   ├── body.go      # FetchEmailBody: MIME parse text/plain + inline images by UID
│   ├── client.go    # IMAP connect, ProcessEmails, GetSenderStatistics, FetchEmailBody
│   └── delete.go    # DeleteSenderEmails, DeleteDomainEmails, DeleteEmail, CleanupCache
├── iterm2/
│   └── render.go    # iTerm2 inline image protocol (OSC 1337); IsSupported() + Render()
├── logger/
│   └── logger.go    # File-based logger with callback for TUI log viewer
├── models/
│   └── email.go     # EmailData, EmailBody, InlineImage, SenderStats, ProgressInfo, DeletionRequest/Result
└── smtp/
    └── client.go    # SMTP send (TLS-first, then STARTTLS fallback)
```

### Key Features

- **Tab 1 — Cleanup**: Groups emails by sender or domain for bulk analysis and deletion
- **Tab 2 — Timeline**: Chronological email list; press Enter to open body preview (split view)
- **Tab 3 — Compose**: Write and send email with Markdown preview (glamour) via SMTP
- **SQLite Caching**: `email_cache.db` — only fetches new messages on subsequent launches
- **Interactive Deletion**: Single email, selected senders, or domain-wide (copies to Trash then expunges)
- **AI Classification**: Ollama-powered `Classify()` tags emails; `a` runs on current folder
- **Chat Panel**: Right-side slide-out (`c` key) — converse with your emails via Ollama
- **Multi-folder Sidebar**: Collapsible IMAP folder tree (`f` key)
- **MCP Server**: `cmd/mcp-server` exposes list/search/stats/classify tools over stdio to Claude
- **SSH App Mode**: `cmd/ssh-server` serves the full TUI over SSH on port 2222
- **iTerm2 Images**: Inline image rendering in the email body preview on iTerm2

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

### TUI Testing with tmux

> **Use both files together:**
> - [TUI_TESTPLAN.md](TUI_TESTPLAN.md) — the full manual QA checklist (what to test and what to expect at each step)
> - [TUI_TESTING.md](TUI_TESTING.md) — programmatic/agent harness guide using `teatest` or PTY + virtual terminal (how to automate TUI interactions in Go tests)
>
> When writing automated TUI tests, consult `TUI_TESTING.md` for the harness pattern, then use the test cases in `TUI_TESTPLAN.md` as the specification for what to assert.

**Always verify visual/layout changes using tmux.** The TUI renders differently at different terminal sizes; a change that looks correct in code can break layout at 80×24 or produce garbage at 50×15. tmux lets you spin up headless sessions at exact dimensions, send keystrokes, and capture rendered output — all without interrupting your working terminal.

**Test reports** must be saved in the `reports/` folder (gitignored). Name them descriptively, e.g. `reports/TEST_REPORT_2026-03-24.md`.

#### Quick workflow

```bash
# 1. Build a test binary
go build -o /tmp/mail-processor-test .

# 2. Start a headless session at a specific size (WIDTHxHEIGHT)
tmux new-session -d -s test -x 220 -y 50

# 3. Launch the app inside it
tmux send-keys -t test './tmp/mail-processor-test -config proton.yaml' Enter
sleep 5   # wait for IMAP connect + initial load

# 4. Capture a screenshot (plain text, ANSI escape codes included with -e)
tmux capture-pane -t test -p -e > /tmp/cap.txt
cat /tmp/cap.txt

# 5. Send keystrokes to navigate (no Enter suffix = no newline)
tmux send-keys -t test '2' ''   # switch to Timeline tab
sleep 0.5
tmux send-keys -t test 'jjjj' ''   # navigate down
sleep 0.3
tmux send-keys -t test '' ''    # open body preview (Enter)
sleep 2
tmux capture-pane -t test -p -e > /tmp/cap_body.txt

# 6. Resize and re-test
tmux resize-window -t test -x 80 -y 24
sleep 0.3
tmux capture-pane -t test -p -e > /tmp/cap_80.txt

# 7. Kill the session when done
tmux kill-session -t test
```

#### Sizes to always check

| Size | Why |
|------|-----|
| 220×50 | Wide terminal — all columns and panels should be fully visible |
| 80×24 | Standard SSH/default — most common real-world size |
| 50×15 | Narrow/small — layout stress test; minimum-size guard should trigger |

#### Key sequences

| Action | Keys |
|--------|------|
| Tab 1 Timeline | `1` |
| Tab 2 Compose | `2` |
| Tab 3 Cleanup | `3` |
| Open body preview | `Enter` (send as `''`) |
| Close preview | `Escape` |
| Toggle sidebar | `f` |
| Toggle chat | `c` |
| Toggle logs | `l` |
| Navigate | `j` / `k` |
| Domain mode | `d` (Cleanup tab) |
| Markdown preview | `C-p` (Compose tab) |

#### What to look for in captures

- **Overflow**: content bleeding past terminal edge (columns sum > terminal width)
- **Truncation**: useful text cut off too aggressively vs too late
- **Empty states**: blank areas where a helpful message should appear
- **Loading indicators**: panels that stay blank while an async fetch is in progress
- **Key hints**: status bar correctly reflects available keys for the active tab/panel

### Configuration
`proton.yaml` (all sections):
```yaml
credentials:
  username: "your_email@mail.com"
  password: "your_password"
server:
  host: "127.0.0.1"   # IMAP host (ProtonMail Bridge default)
  port: 1143           # IMAP port (use 993 for standard TLS)
smtp:
  host: "127.0.0.1"   # SMTP host (ProtonMail Bridge default)
  port: 1025           # SMTP port
ollama:
  host: "http://localhost:11434"  # Ollama API base URL
  model: "gemma2"                 # Model for classification and chat
```

Config file permissions are checked at startup; warns if group/others have access (chmod 600 recommended).

### Dependencies

Go 1.25+ required. `go-sqlite3` requires CGO (`gcc`/`clang` must be present).

| Library | Purpose |
|---------|---------|
| `charmbracelet/bubbletea` | TUI framework (Elm architecture) |
| `charmbracelet/bubbles` | Table, viewport, textinput, textarea components |
| `charmbracelet/lipgloss` | Terminal styling |
| `charmbracelet/glamour` | Markdown rendering in Compose preview |
| `charmbracelet/wish` | SSH server wrapping the TUI (`cmd/ssh-server`) |
| `emersion/go-imap` | IMAP client |
| `mattn/go-sqlite3` | SQLite driver (CGO) |
| `gopkg.in/yaml.v3` | Config parsing |
| `mark3labs/mcp-go` | MCP JSON-RPC server (`cmd/mcp-server`) |

## Key TUI Bindings

| Key | Action |
|-----|--------|
| `q` / `ctrl+c` | Quit |
| `1` / `2` / `3` | Switch to Timeline / Compose / Cleanup tab |
| `d` | Toggle domain/sender grouping mode (Cleanup tab) |
| `r` | Refresh (reconnect + re-process) |
| `D` | Delete selected or current sender/message |
| `space` | Toggle selection (sender row or individual message) |
| `tab` | Cycle focus between panels |
| `enter` | Load details (Cleanup) or open body preview (Timeline) |
| `esc` | Close email preview (Timeline) |
| `up`/`k`, `down`/`j` | Navigate |
| `f` | Toggle folder sidebar |
| `l` / `L` | Toggle real-time log viewer overlay |
| `c` | Toggle AI chat panel |
| `a` | Run AI classification on current folder |
| `R` | Reply: open Compose pre-filled from highlighted Timeline email |
| `ctrl+s` | Send email (Compose tab) |
| `ctrl+p` | Toggle Markdown preview (Compose tab) |

## Development Notes

### Development Practices

#### Documentation conventions

When adding or updating features in `VISION.md` or any other planning/design document:

- Every `##` section heading must be followed by a 1–3 sentence description of what the section covers and why it matters — before any bullet points or subsections.
- All features within a section must be expressed as checkboxes: `- [x]` for implemented, `- [ ]` for planned.
- Never use plain bullet points for feature lists — always checkboxes so the document is also a progress tracker.
- Keep checkbox descriptions concrete and testable (what a user can observe), not vague intentions.

#### Large feature workflow
1. **Update [TUI_TESTPLAN.md](TUI_TESTPLAN.md) first** — add or update the relevant TC-xx test case(s) before writing any implementation code. This defines the acceptance criteria.
2. **Update [VISION.md](VISION.md)** — add the feature as a `- [ ]` checkbox in the relevant section with a brief description.
3. **Update [ARCHITECTURE.md](ARCHITECTURE.md)** if the change affects package responsibilities, data flows, the Backend interface, the SQLite schema, or the Phase 2/3 design. Update the relevant diagram, table, or section before writing implementation code.
4. Implement the feature.
5. Mark the checkbox `- [x]` in VISION.md when done.
6. Run the post-completion checklist below.

#### Post-completion checklist (bugs and large features)

After a bug fix or large feature is complete, run all three surface tests and save a report in `reports/`:

| Surface | How to test |
|---------|-------------|
| **TUI** | tmux workflow (see below) + relevant `TUI_TESTPLAN.md` test cases |
| **SSH** | `go build ./cmd/ssh-server && ./bin/ssh-server`, then `ssh -p 2222 localhost` in a second terminal and exercise the affected flows |
| **MCP** | `go build ./cmd/mcp-server && echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' \| ./bin/mcp-server` — then invoke the relevant tool(s) and verify output |

Save the report as `reports/TEST_REPORT_<YYYY-MM-DD>_<short-description>.md`.

---

### Bug Fix Workflow

**Always write a failing test before fixing a bug.**

#### Internal / non-TUI logic
1. Write a test in the relevant `*_test.go` file that reproduces the bug and fails.
2. Confirm it fails (`go test ./...`).
3. Fix the bug.
4. Confirm the test now passes.
5. Run the post-completion checklist above.

#### TUI logic
When the buggy behavior can be exercised through pure Go (e.g. a helper function, a model update function, state transitions, rendering logic with mock data):
1. Write a test using mock `EmailData` / fake state — no live IMAP or SMTP needed.
2. Confirm it fails, fix the bug, confirm it passes.
3. Run the post-completion checklist above.

When the bug is genuinely only observable via terminal rendering (layout, ANSI codes, key routing), fall back to the tmux workflow described below and document the repro steps in the test report.

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
);

CREATE TABLE email_classifications (
    message_id    TEXT PRIMARY KEY,
    category      TEXT NOT NULL DEFAULT '',
    classified_at DATETIME NOT NULL
);
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

- **Progress updates**: IMAP goroutine sends `models.ProgressInfo` to a buffered channel; `listenForProgress()` blocks until a message arrives then triggers re-render
- **Classification channel**: `classifyCh chan ClassifyProgressMsg` (buffered 50); `listenForClassification()` reads one result at a time per Cmd, same pattern as progress
- **Body fetch**: `FetchEmailBody()` does `UidFetch` with `imap.BodySectionName{}` (full message), then MIME-parses text/plain + inline images; dispatched as a `tea.Cmd`, result is `EmailBodyMsg`
- **iTerm2 images**: `iterm2.Render()` emits `\033]1337;File=...\a`; only called when `iterm2.IsSupported()` (`$TERM_PROGRAM` contains "iTerm"); non-iTerm2 terminals get empty string
- **Domain extraction**: `cache.extractDomain()` handles compound TLDs (`co.uk`, `com.au`, etc.)
- **Text sanitization**: `sanitizeText()` strips emoji/symbols while preserving Unicode letters (table display)
- **TLS**: `InsecureSkipVerify: true` is intentional for local IMAP bridge (e.g. ProtonMail Bridge)
- **Message-ID fallback**: If envelope `Message-Id` is empty, uses `uid-{UID}` as cache key
- **Attachment detection**: Recursively checks `BodyStructure.Disposition == "attachment"`
- **SSH server**: Each `wish` SSH session gets its own `LocalBackend` (own IMAP connection + shared Ollama classifier)
- **MCP server**: Reads directly from `email_cache.db` — no live IMAP needed; 4 tools: `list_recent_emails`, `search_emails`, `get_sender_stats`, `get_email_classifications`

## Generating Demo GIFs

Demo tapes live in `demos/*.tape`. Generated GIFs go to `static/*.gif`.

**Prerequisites:**
```bash
brew install vhs
```

**Regenerate all GIFs:**
```bash
make build   # tapes launch ./bin/herald --demo
for f in demos/*.tape; do vhs "$f"; done
```

**Individual tape:**
```bash
vhs demos/overview.tape   # generates static/overview.gif
```

**Notes:**
- All tapes use `--demo` mode — no live IMAP or credentials needed
- Output paths are set inside each `.tape` file (`Output static/xxx.gif`)
- After changing a feature, regenerate the relevant tape to keep demos current
- Keep tapes under 30 seconds — focused demos convert better
- Tapes must be run from the project root (they reference `./bin/herald`)
