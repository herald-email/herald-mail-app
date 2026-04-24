# Herald

**Fast terminal email for power users.** AI classification, semantic search, bulk cleanup, quick replies, and an MCP server for AI agents — all from your terminal.

![Herald overview](static/overview.gif)

---

## Features

| Feature | Status |
|---------|--------|
| Standard IMAP + personal Gmail IMAP onboarding | ✅ |
| Experimental presets: Gmail OAuth, Protonmail Bridge, Fastmail, iCloud, Outlook | ⚠️ |
| Chronological timeline with split-view email preview | ✅ |
| Bulk cleanup — delete by sender or domain in one keystroke | ✅ |
| AI classification via Ollama (gemma3, llama3, etc.) | ✅ |
| Semantic search with `nomic-embed-text-v2-moe` + chunked body embeddings | ✅ |
| Quick replies — 5 canned + 3 AI-generated suggestions (Ctrl+Q) | ✅ |
| Contact book with LLM enrichment and Apple Contacts import | ✅ |
| Compose + reply + forward with Markdown preview | ✅ |
| MCP server — AI agents read and manage email over stdio | ✅ |
| SSH server — run the full TUI over SSH | ✅ |
| IMAP IDLE push sync — new mail appears instantly | ✅ |

---

## Quick Start

```bash
# Build (Go 1.25+ required)
git clone https://github.com/herald-email/herald-mail-app.git
cd herald-mail-app
make build

# Run (first launch shows setup wizard)
./bin/herald
```

---

## Gmail Setup

Herald's stable Gmail onboarding path targets personal Gmail over IMAP with an App Password. The wizard prefills `imap.gmail.com:993` and `smtp.gmail.com:587`, explains the App Password step, and keeps Gmail OAuth available only as an explicitly experimental path.

1. For personal Gmail, IMAP is generally already on as of January 2025. For Google Workspace Gmail, your admin may need to enable IMAP and Workspace accounts may require OAuth instead of a username/password flow.
2. Turn on Google 2-Step Verification, then create an App Password for Herald.
3. Run `./bin/herald` and choose `Gmail (IMAP + App Password)` in the setup wizard.

Helpful references:

- [Google Workspace Help: Set up Gmail with a third-party email client](https://knowledge.workspace.google.com/admin/sync/set-up-gmail-with-a-third-party-email-client)
- [Gmail Help: Add Gmail to another email client](https://support.google.com/mail/answer/75726?hl=en)
- [Gmail Help: Sign in with app passwords](https://support.google.com/mail/answer/185833?hl=en)

If you want to try the experimental Gmail OAuth flow instead, set:

```bash
export HERALD_GOOGLE_CLIENT_ID="your-client-id.apps.googleusercontent.com"
export HERALD_GOOGLE_CLIENT_SECRET="your-client-secret"
```

---

## Configuration

Config file: `~/.herald/conf.yaml`

```yaml
credentials:
  username: "your@email.com"
  password: "your-password-or-app-password"
server:
  host: "imap.fastmail.com"
  port: 993
smtp:
  host: "smtp.fastmail.com"
  port: 587
ollama:
  host: "http://localhost:11434"
  model: "gemma3:4b"             # for classification, chat, quick replies
  embedding_model: "nomic-embed-text-v2-moe"  # for semantic search
```

Known server presets (auto-fill IMAP/SMTP): `gmail`, `protonmail`, `fastmail`, `icloud`, `outlook`

---

## MCP Setup

Herald ships a standalone MCP server binary (`cmd/herald-mcp-server`) that exposes your email to AI tools over stdio.

```bash
go build -o bin/herald-mcp-server ./cmd/herald-mcp-server
```

### Claude Code

```
Add a local MCP server called "herald" that runs this command:
/path/to/herald/bin/herald-mcp-server -config ~/.herald/conf.yaml
```

Or run this from the herald directory:
```bash
claude mcp add herald -- "$(pwd)/bin/herald-mcp-server" -config ~/.herald/conf.yaml
```

### Cursor

Add to `.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "herald": {
      "command": "/path/to/herald/bin/herald-mcp-server",
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
      "command": "/path/to/herald/bin/herald-mcp-server",
      "args": ["-config", "~/.herald/conf.yaml"]
    }
  }
}
```

### Codex

```bash
CODEX_MCP_SERVERS='{"herald":{"command":"/path/to/herald/bin/herald-mcp-server","args":["-config","~/.herald/conf.yaml"]}}' codex
```

### Generic (any stdio MCP client)

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | ./bin/herald-mcp-server -config ~/.herald/conf.yaml
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

## License

Herald is source-available under the Functional Source License, Version 1.1,
ALv2 Future License (`FSL-1.1-ALv2`). You may use, copy, modify, redistribute,
and run Herald for any permitted purpose other than a competing commercial use.

Each version converts to the Apache License, Version 2.0 on the second
anniversary of the date that version is made available, as described in
[LICENSE](LICENSE).
