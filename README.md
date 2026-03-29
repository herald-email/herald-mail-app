# Herald

Herald is a terminal email client for power users. Fast inbox cleanup, AI classification, semantic search, and an MCP server that lets AI agents read and manage your email without opening the TUI. Supports Gmail (OAuth2), ProtonMail Bridge, Fastmail, iCloud, Outlook, and standard IMAP.

---

## Quick Start

```bash
# Build from source (Go 1.23+ required)
git clone <repo>
cd mail-processor
make build

# Run (first launch shows setup wizard)
./bin/herald
```

---

## Gmail OAuth2

Herald supports native Gmail OAuth2 — no app passwords needed.

1. Create a Google Cloud project and enable the Gmail API
2. Create OAuth2 credentials (Desktop app type) and note the client ID and secret
3. Set environment variables:

```bash
export HERALD_GOOGLE_CLIENT_ID="your-client-id.apps.googleusercontent.com"
export HERALD_GOOGLE_CLIENT_SECRET="your-client-secret"
```

4. Run `./bin/herald` — the setup wizard will guide you through authorization

---

## MCP Setup

Herald includes an MCP server that exposes your email to AI tools (Claude Code, Cursor, etc.). It is built as a separate binary from `cmd/mcp-server`.

```bash
# Build the MCP server binary
go build -o bin/mcp-server ./cmd/mcp-server
```

### Claude Code

Give this prompt to Claude Code:

```
Add a local MCP server called "herald" that runs this command:
/path/to/herald/bin/mcp-server -config ~/.herald/conf.yaml
```

### Cursor / VS Code

Add to your MCP settings JSON:

```json
{
  "mcpServers": {
    "herald": {
      "command": "/path/to/herald/bin/mcp-server",
      "args": ["-config", "~/.herald/conf.yaml"]
    }
  }
}
```

### Available MCP Tools

| Tool | Description |
|------|-------------|
| `list_recent_emails` | Most recent emails in a folder |
| `list_unread_emails` | Unread emails only |
| `search_emails` | Keyword search on sender + subject |
| `search_by_sender` | All emails from a sender or domain |
| `search_by_date` | Filter by date range |
| `semantic_search_emails` | Natural-language search via local embeddings |
| `get_email_body` | Cached plain-text body |
| `get_sender_stats` | Senders ranked by email volume |
| `get_email_classifications` | AI category counts for a folder |
| `classify_email` | Run AI classification on one email |
| `summarise_email` | Generate a summary via Ollama |

---

## Key Bindings

| Key | Action |
|-----|--------|
| `1` / `2` / `3` | Switch to Timeline / Compose / Cleanup tab |
| `j` / `k` | Navigate up / down |
| `Enter` | Open email preview |
| `D` | Delete selected email or sender |
| `e` | Archive |
| `R` | Reply |
| `F` | Forward |
| `u` | Unsubscribe |
| `z` | Full-screen preview |
| `S` | Open settings |
| `c` | Toggle AI chat panel |
| `a` | Run AI classification on current folder |
| `f` | Toggle folder sidebar |
| `/` | Search |
| `?` | Semantic search |
| `q` | Quit |

---

## Configuration

Config file: `~/.herald/conf.yaml`

For manual configuration (advanced):

```yaml
credentials:
  username: "your@email.com"
  password: "your-password"
server:
  host: "imap.fastmail.com"
  port: 993
smtp:
  host: "smtp.fastmail.com"
  port: 587
ollama:
  host: "http://localhost:11434"
  model: "gemma3:4b"
```

Vendor presets (auto-fill IMAP/SMTP settings): `gmail`, `protonmail`, `fastmail`, `icloud`, `outlook`

---

## Architecture

See [VISION.md](VISION.md) for the full feature roadmap and [ARCHITECTURE.md](ARCHITECTURE.md) for the technical design.
