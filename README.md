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

**Threads:**
- A thread header showing `[3] Subject` means 3 emails share that subject
- `Enter` on a thread header — expands it to show individual emails
- `Enter` again on the header — collapses it back

**Replying:**
- `R` — opens Compose pre-filled with the sender's address and `Re:` subject

**Deleting:**
- `D` — moves the highlighted email to Trash

---

## Tab 2 — Compose

Write and send email.

- `Tab` cycles between: **To**, **Subject**, **Body** fields
- `Ctrl+P` — toggle Markdown preview (body is rendered with formatting)
- `Ctrl+S` — send the email via SMTP
- `Esc` — cancel and return to Timeline

---

## Tab 3 — Cleanup

Find and delete bulk senders. The left panel groups all emails by sender (or domain), sorted by volume. The right panel shows individual messages for the highlighted sender.

**Grouping:**
- `d` — toggle between grouping by full sender address vs. by domain
  (e.g. `news@promo.example.com` grouped under `example.com`)

**Selecting and deleting:**
- `Space` — select/deselect the highlighted sender or individual message
- `Enter` — load individual messages for the highlighted sender
- `D` — delete all emails from selected senders (or selected individual messages)

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
- `Enter` to send, the response streams back in the panel

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

---

## Running Over SSH

A built-in SSH server serves the full TUI on port 2222:

```bash
./bin/ssh-server
ssh localhost -p 2222
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
