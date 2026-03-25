# Mail Processor

A fast terminal email client built for inbox management. Read emails, clean up subscriptions, compose replies, and use AI to classify and chat with your inbox — all from the terminal.

## Quick Start

### Build from Source

**Prerequisites**: Go 1.23+, GCC/Clang (for SQLite)

```bash
git clone https://github.com/zoomacode/mail-processor.git
cd mail-processor
make build
./bin/mail-processor
```

## Configuration

Create `proton.yaml` in the same directory as the binary:

```yaml
credentials:
  username: "your_email@mail.com"
  password: "your_bridge_password"
server:
  host: "127.0.0.1"   # ProtonMail Bridge default
  port: 1143           # Use 993 for standard IMAP TLS
smtp:
  host: "127.0.0.1"   # For sending email
  port: 1025
ollama:
  host: "http://localhost:11434"   # For AI features (optional)
  model: "gemma2"
```

**Vendor presets** — skip the `server` block by using a preset:

```yaml
vendor: protonmail   # sets host=127.0.0.1 port=1143 smtp.port=1025
# or
vendor: gmail        # sets host=imap.gmail.com port=993
```

Secure the file: `chmod 600 proton.yaml`

On first launch, all emails are fetched into a local SQLite cache. Subsequent launches only sync new messages, so startup is fast.

---

## The Interface

The app has three tabs. Switch between them with `1`, `2`, `3`.

```
 1  Timeline    2  Compose    3  Cleanup
```

---

## Tab 1 — Timeline

Your inbox as a chronological list. Emails with the same subject are grouped into collapsed threads.

**Reading email:**
- `↑`/`↓` or `k`/`j` — navigate the list
- `Enter` — open the email body (splits the screen; plain text or converted from HTML)
- `Esc` — close the preview
- `z` — toggle full-screen email view (hides tab bar, sidebar, and timeline; fills the terminal)

**Full-screen mode keys:** `j`/`k` scroll, `z` or `Esc` exit.

**Threads:**
- A thread header showing `[3] Subject` means 3 emails share that subject
- `Enter` on a thread header — expands it to show individual emails
- `Enter` again on the header — collapses it back

**Attachments:**
- Emails with attachments show an attachment count in the timeline
- In the body preview, each attachment is listed as `[attach] filename  mime/type  size`
- `s` — save the highlighted attachment (prompts for a destination path; defaults to `~/Downloads/`)

**Text selection and clipboard:**
- `v` — enter visual mode; the current line is highlighted
- `j`/`k` — extend the selection down/up
- `y` — copy the selection to the system clipboard (macOS: pbcopy; Wayland: wl-copy; X11: xclip)
- `yy` — copy the current line (press `y` twice without entering visual mode)
- `Y` — copy the entire email body
- `Esc` — cancel visual mode
- `m` — release TUI mouse handling so you can select text with the terminal's native cursor;
         press `m` again to restore TUI interaction

**Replying and forwarding:**
- `R` — opens Compose pre-filled with the sender's address and `Re:` subject
- `F` — opens Compose pre-filled with `Fwd:` subject and a forwarded-message block

**Deleting and archiving:**
- `D` — confirmation prompt → `y` to delete, `n`/`Esc` to cancel
- `e` — confirmation prompt → `y` to move to Archive

**Search:**
- `/` — in-folder search (filters by sender and subject in real time)
- `/b <query>` — full-text body search (uses cached body text)
- `/* <query>` — cross-folder search (all folders)
- `? <query>` — semantic similarity search (requires Ollama with `nomic-embed-text`)
- `Ctrl+I` — IMAP server-side search (for terms not yet cached locally)
- `Ctrl+S` (in search mode) — save the current search with a name
- `Esc` — clear search and restore the full email list

---

## Tab 2 — Compose

Write and send email. The body is interpreted as Markdown and sent as both HTML and plain text.

- `Tab` — cycle between **To**, **Subject**, **Body** fields
- `Ctrl+P` — toggle Markdown preview (rendered with glamour)
- `Ctrl+A` — add a file attachment (prompts for a path; the file is staged below the body)
- `Ctrl+S` — send the email via SMTP (multipart/alternative HTML + plain text; multipart/mixed if attachments are staged)
- `Esc` — cancel and return to Timeline

Staged attachments are listed below the body as `[attach] filename (N KB)`. A warning appears if any attachment exceeds 10 MB.

---

## Tab 3 — Cleanup

Find and delete bulk senders. The left panel groups all emails by sender (or domain), sorted by volume. The right panel shows individual messages for the highlighted sender.

**Grouping:**
- `d` — toggle between grouping by full sender address vs. by domain
  (e.g. `news@promo.example.com` grouped under `example.com`)

**Selecting and deleting:**
- `Space` — select/deselect the highlighted sender or individual message
- `Enter` — load individual messages for the highlighted sender
- `D` — delete all emails from selected senders (or selected individual messages); confirmation prompt
- `e` — archive all emails from the highlighted sender; confirmation prompt

Deletion moves emails to Trash and runs in the background. The status bar shows progress; you can keep navigating while it runs.

---

## Folder Sidebar

Press `f` to open/close a folder tree on the left. Navigate with `↑`/`↓`, press `Enter` to switch to a folder. The app syncs and displays the selected folder's emails.

---

## AI Features

Requires [Ollama](https://ollama.com) running locally with the configured model pulled.

**Classification:**
- `a` — classify all unclassified emails in the current folder
- Categories appear as a label next to each email in the Cleanup tab

**Chat panel:**
- `c` — open/close a chat panel on the right side
- Ask questions about your inbox in plain language (e.g. "Which senders have the most emails?")
- `Enter` to send; the response streams back in the panel

**Semantic search:**
- `/? <query>` — find emails by meaning rather than keyword
- Requires the `nomic-embed-text` model: `ollama pull nomic-embed-text`

---

## Global Keys

| Key | Action |
|-----|--------|
| `1` / `2` / `3` | Switch to Timeline / Compose / Cleanup |
| `q` / `Ctrl+C` | Quit |
| `r` | Refresh (reconnect and sync new emails) |
| `f` | Toggle folder sidebar |
| `c` | Toggle AI chat panel |
| `a` | Run AI classification on current folder |
| `l` / `L` | Toggle live log viewer |
| `↑`/`k`, `↓`/`j` | Navigate |
| `Tab` | Cycle focus between panels |

### Timeline / Preview Keys

| Key | Action |
|-----|--------|
| `Enter` | Open body preview (or expand/collapse thread) |
| `Esc` | Close preview |
| `z` | Toggle full-screen email view |
| `R` | Reply (opens Compose) |
| `F` | Forward (opens Compose) |
| `D` | Delete with confirmation |
| `e` | Archive with confirmation |
| `/` | Start search |
| `v` | Enter visual selection mode |
| `y` (in visual mode) | Copy selection to clipboard |
| `yy` | Copy current line to clipboard |
| `Y` | Copy entire body to clipboard |
| `m` | Toggle mouse mode (release/restore TUI mouse handling) |
| `s` | Save highlighted attachment |

### Compose Keys

| Key | Action |
|-----|--------|
| `Tab` | Next field (To → Subject → Body) |
| `Ctrl+P` | Toggle Markdown preview |
| `Ctrl+A` | Add file attachment |
| `Ctrl+S` | Send email |

---

## SSH Server

The full TUI can be served over SSH so you can connect from any terminal — remote machine, iPad, phone, etc.

**Build:**
```bash
go build -o bin/ssh-server ./cmd/ssh-server
```

**Start:**
```bash
./bin/ssh-server                                  # defaults: port 2222, proton.yaml
./bin/ssh-server -addr :2222 -config proton.yaml -host-key .ssh/host_ed25519
```

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:2222` | Listen address |
| `-config` | `proton.yaml` | Path to config file |
| `-host-key` | `.ssh/host_ed25519` | SSH host private key (auto-created if missing) |

**Connect:**
```bash
ssh localhost -p 2222
```

- No username or password needed — the server accepts any connection
- Each SSH session gets its own independent IMAP connection
- All sessions share the same local email cache and Ollama classifier
- The host key is generated on first run and reused on restart; subsequent connections will not trigger a host-key warning

---

## MCP Server

The MCP (Model Context Protocol) server exposes your email data as tools that Claude and other AI agents can call directly. It reads from the local SQLite cache — no live IMAP connection is required.

**Prerequisites:** run the TUI at least once to populate `email_cache.db`.

**Build:**
```bash
go build -o bin/mcp-server ./cmd/mcp-server
```

**Claude Code integration** — add to your `.claude/settings.json`:
```json
{
  "mcpServers": {
    "mail": {
      "command": "/absolute/path/to/bin/mcp-server",
      "args": ["-config", "/absolute/path/to/proton.yaml"]
    }
  }
}
```

Once registered, Claude can call your email tools in any conversation.

**Available tools:**

| Tool | Parameters | Description |
|------|-----------|-------------|
| `list_recent_emails` | `folder` (req), `limit` (opt, default 20) | Most recent emails newest-first |
| `search_emails` | `folder` (req), `query` (req) | Case-insensitive search across sender and subject |
| `get_sender_stats` | `folder` (req), `top_n` (opt, default 20) | Senders ranked by email count |
| `get_email_classifications` | `folder` (req) | AI category counts (requires prior `a` run in TUI) |

**Direct invocation** (for testing without Claude):
```bash
# List available tools
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | ./bin/mcp-server

# Or use the MCP inspector UI
npx @modelcontextprotocol/inspector ./bin/mcp-server
```

---

## Troubleshooting

**No emails appear on first launch** — the initial sync can take a moment for large mailboxes. Watch the progress bar at the bottom.

**"(image)" in email body** — inline images are shown as text descriptors; the app is terminal-only.

**Deletion not working** — press `l` to open the log viewer for details. The app looks for a Trash/Deleted Items folder automatically.

**AI features not working** — ensure Ollama is running (`ollama serve`) and the model is pulled (`ollama pull gemma2`).

**Debug mode:**
```bash
./bin/mail-processor -debug
```

## Architecture

### Key Components

1. **IMAP Client** ([internal/imap/](internal/imap/)) - Handles email server communication
2. **Cache System** ([internal/cache/](internal/cache/)) - SQLite-based email metadata storage
3. **TUI Interface** ([internal/app/](internal/app/)) - Bubble Tea-based user interface
4. **Configuration** ([internal/config/](internal/config/)) - YAML config file handling
5. **Models** ([internal/models/](internal/models/)) - Data structures and types

### Performance Features

- **Concurrent Processing**: Efficient goroutine usage for email processing
- **Smart Caching**: Only processes new emails on subsequent runs
- **Memory Efficient**: Streams email data rather than loading everything
- **Async Operations**: Non-blocking deletion queue with worker goroutines

### Security Features

- **Secure Deletion**: Moves to Trash folder instead of permanent deletion
- **Message-ID Based**: Uses IMAP Message-ID for reliable deletion
- **Input Validation**: All email data is validated before processing
- **Secure Connections**: TLS encryption for IMAP connections
- **Config Security**: File permission checks for credential files

## Development

### Project Structure
```
.
├── main.go                 # Application entry point
├── internal/
│   ├── app/               # TUI application logic
│   │   ├── app.go         # Main app model and Update loop
│   │   ├── helpers.go     # Helper functions and workers
│   │   └── logs.go        # Log viewer component
│   ├── cache/             # SQLite caching
│   │   └── cache.go       # Cache implementation
│   ├── config/            # Configuration handling
│   │   └── config.go      # Config parsing
│   ├── imap/              # IMAP client
│   │   ├── client.go      # IMAP operations
│   │   └── delete.go      # Email deletion (Message-ID based)
│   ├── logger/            # Logging system
│   │   └── logger.go      # Structured logging
│   └── models/            # Data models
│       └── email.go       # Email structures
├── go.mod                 # Go modules
├── Makefile              # Build automation
└── proton.yaml           # Configuration file
```

### Development Commands

```bash
# Install dependencies
make deps

# Format code
make fmt

# Run linter
make vet

# Run tests
make test

# Build
make build

# Run
make run
```

### Building for Multiple Platforms

Releases are automated via GitHub Actions using GoReleaser. To create a release:

```bash
git tag v1.0.0
git push origin v1.0.0
```

This will automatically build and publish binaries for:
- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64)

## Dependencies

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Terminal styling
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components (table)
- [go-imap](https://github.com/emersion/go-imap) - IMAP client library
- [go-sqlite3](https://github.com/mattn/go-sqlite3) - SQLite driver
- [yaml.v3](https://github.com/go-yaml/yaml) - Configuration parsing

## Troubleshooting

### Common Issues

**Connection Failed**:
- Check ProtonMail Bridge is running
- Verify host/port in `proton.yaml`
- Check firewall settings

**Permission Denied**:
- Ensure config file has correct permissions: `chmod 600 proton.yaml`

**SQLite Error**:
- Check disk space
- Verify file permissions on `email_cache.db`

**Deletion Not Working**:
- Check logs with `l` key
- Verify Trash folder exists on IMAP server
- Check IMAP server permissions

### Debugging

Enable debug logging:
```bash
./bin/mail-processor -debug
```

Monitor log files:
```bash
tail -f mail_processor_*.log
```

Or use the built-in log viewer by pressing `l` in the TUI.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `make test`
5. Submit a pull request

## License

MIT License - see LICENSE file for details
