# Herald

**Fast terminal email for power users.** AI classification, semantic search, bulk cleanup, quick replies, and an MCP server for AI agents — all from your terminal.

![Herald overview](static/overview.gif)

---

## Features

| Feature | Status |
|---------|--------|
| Gmail (OAuth2), Protonmail Bridge, Fastmail, iCloud, Outlook, IMAP | ✅ |
| Chronological timeline with split-view email preview | ✅ |
| Bulk cleanup — delete by sender or domain in one keystroke | ✅ |
| AI classification via Ollama (gemma3, llama3, etc.) | ✅ |
| Semantic search with nomic-embed-text + chunked body embeddings | ✅ |
| Quick replies — 5 canned + 3 AI-generated suggestions (Ctrl+Q) | ✅ |
| Contact book with LLM enrichment and Apple Contacts import | ✅ |
| Compose + reply + forward with Markdown preview | ✅ |
| MCP server — AI agents read and manage email over stdio | ✅ |
| SSH server — run the full TUI over SSH | ✅ |
| IMAP IDLE push sync — new mail appears instantly | ✅ |

---

## Quick Start

```bash
# Build (Go 1.23+ required)
git clone https://github.com/your-org/herald
cd herald
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

## Configuration

Config file: `~/.herald/conf.yaml`

```yaml
credentials:
  username: "your@email.com"
  password: "your-password"     # or leave blank for OAuth2
server:
  host: "imap.fastmail.com"
  port: 993
smtp:
  host: "smtp.fastmail.com"
  port: 587
ollama:
  host: "http://localhost:11434"
  model: "gemma3:4b"             # for classification, chat, quick replies
  embedding_model: "nomic-embed-text"  # for semantic search
```

Vendor presets (auto-fill IMAP/SMTP): `gmail`, `protonmail`, `fastmail`, `icloud`, `outlook`

---

## MCP Setup

Herald ships a standalone MCP server binary (`cmd/mcp-server`) that exposes your email to AI tools over stdio.

```bash
go build -o bin/mcp-server ./cmd/mcp-server
```

### Claude Code

```
Add a local MCP server called "herald" that runs this command:
/path/to/herald/bin/mcp-server -config ~/.herald/conf.yaml
```

Or run this from the herald directory:
```bash
claude mcp add herald -- "$(pwd)/bin/mcp-server" -config ~/.herald/conf.yaml
```

### Cursor

Add to `.cursor/mcp.json`:

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

### Windsurf

Add to `~/.codeium/windsurf/mcp_config.json`:

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

### Codex

```bash
CODEX_MCP_SERVERS='{"herald":{"command":"/path/to/herald/bin/mcp-server","args":["-config","~/.herald/conf.yaml"]}}' codex
```

### Generic (any stdio MCP client)

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | ./bin/mcp-server -config ~/.herald/conf.yaml
```

### Available MCP Tools

| Tool | Description |
|------|-------------|
| `list_recent_emails` | Most recent emails in a folder |
| `list_unread_emails` | Unread emails only |
| `search_emails` | Keyword search on sender + subject |
| `search_by_sender` | All emails from a sender or domain |
| `search_by_date` | Filter by date range |
| `semantic_search_emails` | Natural-language search via chunked body embeddings |
| `get_email_body` | Cached plain-text body |
| `get_sender_stats` | Senders ranked by email volume |
| `get_email_classifications` | AI category counts for a folder |
| `classify_email` | Run AI classification on one email |
| `summarise_email` | Generate a summary via Ollama |
| `list_contacts` | Paginated contact list |
| `search_contacts` | Keyword search on name/email/company/topics |
| `semantic_search_contacts` | Natural-language contact search |
| `get_contact` | Full profile + recent emails |

---

## Key Bindings

| Key | Action |
|-----|--------|
| `1` / `2` / `3` / `4` | Timeline / Compose / Cleanup / Contacts tab |
| `j` / `k` | Navigate down / up |
| `Enter` | Open email preview |
| `Escape` | Close preview / picker |
| `D` | Delete selected email or sender |
| `e` | Archive |
| `R` | Reply |
| `F` | Forward |
| `Ctrl+Q` | Quick reply picker (in preview) |
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

## Run in Browser

Herald can run in a browser tab via [ttyd](https://github.com/nicholasgasior/ttyd):

```bash
brew install ttyd
ttyd -W ./bin/herald
```

Open [http://localhost:7681](http://localhost:7681). The `-W` flag makes the terminal writable (required for keyboard input). All key bindings work as in a normal terminal.

Options:

```bash
ttyd -W -p 8080 ./bin/herald                  # Custom port
ttyd -W -c user:pass ./bin/herald              # Basic auth
ttyd -W ./bin/herald --demo                    # Demo mode (no IMAP needed)
```

---

## Architecture

See [VISION.md](VISION.md) for the full feature roadmap and [ARCHITECTURE.md](ARCHITECTURE.md) for the technical design.
