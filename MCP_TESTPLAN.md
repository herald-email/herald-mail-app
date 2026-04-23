# MCP Server Test Plan — mail-processor

Manual QA checklist for verifying that the MCP server correctly exposes email data as tools to AI agents and Claude Code.
Run this after any change to `cmd/mcp-server/`, `internal/cache/`, or the SQLite schema.

---

## Setup

### 1. Populate the cache

The MCP server reads from `email_cache.db` — it does not connect to IMAP.
If the cache is empty, run the TUI first and wait for it to finish syncing.

```bash
./bin/mail-processor -config proton.yaml
# Press q once sync is complete
```

### 2. Build the MCP server binary

```bash
go build -o /tmp/mcp-server-test ./cmd/mcp-server
```

### 3a. Test with MCP Inspector (recommended — browser UI)

```bash
npx @modelcontextprotocol/inspector /tmp/mcp-server-test -config proton.yaml
# Opens http://localhost:5173 — use the browser to call tools interactively
```

### 3b. Test with raw JSON-RPC via stdin

The MCP protocol is newline-delimited JSON-RPC 2.0. Each request must be sent as a single line.

```bash
# List available tools
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' \
  | /tmp/mcp-server-test -config proton.yaml

# Call a tool
echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_recent_emails","arguments":{"folder":"INBOX"}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

### 4. Claude Code integration test

Add to `~/.claude/settings.json` (or project `.claude/settings.json`):

```json
{
  "mcpServers": {
    "mail": {
      "command": "/absolute/path/to/mcp-server-test",
      "args": ["-config", "/absolute/path/to/proton.yaml"]
    }
  }
}
```

Restart Claude Code, then ask: *"List my 5 most recent emails in INBOX."*

### 5. Teardown

No persistent process to kill — the MCP server exits after stdin closes.
Remove the test binary: `rm /tmp/mcp-server-test`

---

## Prerequisites

| Item | Required |
|------|----------|
| `proton.yaml` present | Yes |
| `email_cache.db` populated | Yes (except TC-MCP-13) |
| Node.js / `npx` available | For TC-MCP-01 via Inspector |
| Claude Code with MCP support | For TC-MCP-14 |
| AI classifications present | TC-MCP-11 only (press `a` in TUI first) |
| Ollama with the configured embedding model available | TC-MCP-19 and TC-MCP-20 |

---

## What to Look for in Every Response

- **Well-formed JSON** — every response is valid JSON-RPC 2.0 with an `id` matching the request
- **No crash output** — no Go stack trace mixed into the JSON response
- **Correct field names** — `date`, `sender`, `subject` present in email results
- **Sort order** — list/stats results respect documented ordering (newest-first, count-descending)
- **Error shape** — errors use `{"jsonrpc":"2.0","id":N,"error":{"code":...,"message":"..."}}`, not a panic

---

## Test Cases

### TC-MCP-01 — Server starts and registers tools

**Steps:**
```bash
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect:**
- Response contains a non-empty `result.tools` array with the current catalog, not only the legacy 4-tool subset
- Tool names present include at least:
  - `list_recent_emails`, `search_emails`, `get_sender_stats`, `get_email_classifications`
  - `semantic_search_emails`, `semantic_search_contacts`
  - `classify_email`, `summarise_email`, `draft_reply`
- Each tool has a `description` and `inputSchema` field
- No error field in response

---

### TC-MCP-02 — list_recent_emails basic

**Steps:**
```bash
echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_recent_emails","arguments":{"folder":"INBOX"}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect:**
- Response `result.content[0].text` contains a list of emails
- Up to 20 rows returned (default limit)
- Each row includes date, sender, and subject
- Rows ordered newest-first

---

### TC-MCP-03 — list_recent_emails with limit

**Steps:**
```bash
echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_recent_emails","arguments":{"folder":"INBOX","limit":5}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect:**
- Exactly 5 rows returned (or fewer if folder has < 5 emails)
- Same format as TC-MCP-02

---

### TC-MCP-04 — list_recent_emails unknown folder

**Steps:**
```bash
echo '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"list_recent_emails","arguments":{"folder":"DoesNotExist"}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect:**
- Either empty list with a "no emails found" message, or a descriptive error
- No crash; valid JSON-RPC response

---

### TC-MCP-05 — search_emails by sender

**Steps:**
1. Pick a sender domain that exists in your cache (e.g. `github.com`).
2. Run:
```bash
echo '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"search_emails","arguments":{"folder":"INBOX","query":"github.com"}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect:**
- Results only contain emails whose sender includes `github.com`
- No emails from unrelated senders appear
- Up to 100 results returned

---

### TC-MCP-06 — search_emails by subject keyword

**Steps:**
1. Pick a keyword that appears in at least one subject (e.g. `invoice`).
2. Run:
```bash
echo '{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"search_emails","arguments":{"folder":"INBOX","query":"invoice"}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect:**
- Results contain emails with `invoice` in sender or subject
- Case-insensitive match (e.g. `Invoice`, `INVOICE` both match)

---

### TC-MCP-07 — search_emails no results

**Steps:**
```bash
echo '{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"search_emails","arguments":{"folder":"INBOX","query":"xyzzy_no_match_12345"}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect:**
- Empty result list with a "no emails found" message
- No crash; valid JSON-RPC response

---

### TC-MCP-08 — search_emails special characters (SQL safety)

**Steps:**
```bash
# Test % wildcard
echo '{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"search_emails","arguments":{"folder":"INBOX","query":"%"}}}' \
  | /tmp/mcp-server-test -config proton.yaml

# Test backslash
echo '{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"search_emails","arguments":{"folder":"INBOX","query":"\\\\"}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect:**
- Both return valid JSON-RPC responses (empty results or matches)
- No SQL error embedded in response
- No crash or panic output

---

### TC-MCP-09 — get_sender_stats basic

**Steps:**
```bash
echo '{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"get_sender_stats","arguments":{"folder":"INBOX"}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect:**
- Response lists senders with their email counts
- Ordered by count descending (highest-volume sender first)
- Up to 20 rows by default

---

### TC-MCP-10 — get_sender_stats with top_n

**Steps:**
```bash
echo '{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"get_sender_stats","arguments":{"folder":"INBOX","top_n":3}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect:**
- Exactly 3 rows returned (or fewer if folder has < 3 distinct senders)
- Same ordering as TC-MCP-09

---

### TC-MCP-11 — get_email_classifications with data

**Prerequisites:** Open TUI, switch to INBOX, press `a` and wait for classification to finish.

**Steps:**
```bash
echo '{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"get_email_classifications","arguments":{"folder":"INBOX"}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect:**
- Response contains a category summary (e.g. `{"subscription": 42, "important": 5, …}`)
- All categories assigned by the TUI are represented
- Counts are positive integers

---

### TC-MCP-12 — get_email_classifications with no data

**Steps:**
```bash
# Use a folder that has never been classified
echo '{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"get_email_classifications","arguments":{"folder":"Sent"}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect:**
- Empty classification object `{}` or a message indicating no classifications
- No crash; valid JSON-RPC response

---

### TC-MCP-13 — Missing cache file

**Steps:**
```bash
# Run with a config pointing to a non-existent DB location
/tmp/mcp-server-test -config /nonexistent/proton.yaml
```
Then attempt any tool call over stdin.

**Expect:**
- Server either exits with a clear error message before accepting input, or
- Accepts the connection and returns a JSON-RPC error on the first tool call
- No panic or unhandled exception output

---

### TC-MCP-14 — Claude Code integration (end-to-end)

**Prerequisites:** Claude Code installed and `settings.json` configured per Setup step 4.

**Steps:**
1. Restart Claude Code to pick up the new MCP server config.
2. Open a new conversation.
3. Ask: *"Using the mail MCP server, list my 5 most recent emails in INBOX."*
4. Observe the tool call and response.

**Expect:**
- Claude invokes `list_recent_emails` with `folder:"INBOX"` and `limit:5`
- Tool result is displayed in the conversation
- Claude summarises the emails in natural language
- No MCP protocol errors appear in the Claude Code output

---

### TC-MCP-15 — get_email_body

**Steps:**
```bash
# Use a message_id from a previous list_recent_emails call that has been opened in the TUI
echo '{"jsonrpc":"2.0","id":15,"method":"tools/call","params":{"name":"get_email_body","arguments":{"message_id":"<some-id>"}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect:**
- Returns the cached plain-text body of the email
- If body is not cached, returns a helpful message to open the email in the TUI

---

### TC-MCP-16 — list_unread_emails

**Steps:**
```bash
echo '{"jsonrpc":"2.0","id":16,"method":"tools/call","params":{"name":"list_unread_emails","arguments":{"folder":"INBOX","limit":5}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect:**
- Returns only unread emails (those not yet opened in TUI)
- Respects `limit` parameter

---

### TC-MCP-17 — search_by_date

**Steps:**
```bash
echo '{"jsonrpc":"2.0","id":17,"method":"tools/call","params":{"name":"search_by_date","arguments":{"folder":"INBOX","after":"2024-01-01","before":"2024-12-31"}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect:**
- Returns emails within the specified date range only
- Invalid date format returns a clear error

---

### TC-MCP-18 — search_by_sender

**Steps:**
```bash
echo '{"jsonrpc":"2.0","id":18,"method":"tools/call","params":{"name":"search_by_sender","arguments":{"sender":"github.com"}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect:**
- Returns emails from senders matching `github.com` across all folders
- Each row shows folder name alongside date, sender, subject

---

### TC-MCP-19 — semantic_search_emails (Ollama required)

**Prerequisites:** Ollama running with the configured embedding model available (default: `nomic-embed-text-v2-moe`); emails with cached body vectors.

**Steps:**
```bash
echo '{"jsonrpc":"2.0","id":19,"method":"tools/call","params":{"name":"semantic_search_emails","arguments":{"query":"invoice or billing","folder":"INBOX","limit":5}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect (Ollama running):**
- Returns semantically similar emails, ranked by similarity

**Expect (Ollama not configured):**
- Returns: `Ollama not configured — set ollama.host in proton.yaml`

---

### TC-MCP-20 — semantic_search_contacts (Ollama required)

**Prerequisites:** Ollama running with the configured embedding model available; contacts already enriched/indexed.

**Steps:**
```bash
echo '{"jsonrpc":"2.0","id":20,"method":"tools/call","params":{"name":"semantic_search_contacts","arguments":{"query":"people I discuss swiftui performance with","limit":5}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect (Ollama running):**
- Returns semantically similar contacts, ranked by relevance
- Result text includes contact identity plus score/context fields when available

**Expect (Ollama unavailable or model missing):**
- Returns a bounded embedding/config error rather than crashing or hanging

---

### TC-MCP-21 — classify_email (Ollama required)

**Prerequisites:** Ollama running; a known message_id.

**Steps:**
```bash
echo '{"jsonrpc":"2.0","id":21,"method":"tools/call","params":{"name":"classify_email","arguments":{"message_id":"<some-id>"}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect (Ollama running):**
- Returns `Classified as: <category>`
- Classification persisted in cache (visible in TUI Cleanup tab)

**Expect (Ollama not configured):**
- Returns: `Ollama not configured — set ollama.host in proton.yaml`

---

### TC-MCP-22 — summarise_email (Ollama required)

**Prerequisites:** Ollama running; a message_id whose body has been cached (opened in TUI).

**Steps:**
```bash
echo '{"jsonrpc":"2.0","id":22,"method":"tools/call","params":{"name":"summarise_email","arguments":{"message_id":"<some-id>","max_words":50}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect (body cached, Ollama running):**
- Returns a concise summary in ≤50 words

**Expect (body not cached):**
- Returns: `Body not cached. Open the email in the TUI to load its body first.`

**Expect (Ollama not configured):**
- Returns: `Ollama not configured — set ollama.host in proton.yaml`

---

### TC-MCP-23 — draft_reply (AI required)

**Prerequisites:** AI backend running; a known `message_id`; body cached preferred for higher-quality replies.

**Steps:**
```bash
echo '{"jsonrpc":"2.0","id":23,"method":"tools/call","params":{"name":"draft_reply","arguments":{"message_id":"<some-id>","tone":"professional"}}}' \
  | /tmp/mcp-server-test -config proton.yaml
```

**Expect (AI running):**
- Returns reply-body text only, without headers or JSON wrapper text
- Output tone roughly matches the requested `professional` style

**Expect (body not cached):**
- Still returns a bounded draft using sender/subject context, or a clear cached-body guidance message

**Expect (AI not configured):**
- Returns: `AI not configured`

---

## Result Format

After completing all test cases, write up findings using this structure:

```
## Test Run — <date> — MCP server <version>

### Bugs

| ID  | Severity | TC      | Description                              | Steps to reproduce       |
|-----|----------|---------|------------------------------------------|--------------------------|
| B1  | High     | MCP-08  | % query returns all emails (not escaped) | TC-MCP-08, first command |

### UX Issues

| ID  | TC      | Description                                     | Suggestion                        |
|-----|---------|-------------------------------------------------|-----------------------------------|
| U1  | MCP-04  | Unknown folder returns no message, just empty   | Add "folder not found" note       |

### All Good

List test cases that passed with no issues:
- TC-MCP-01 Server starts: PASS
- TC-MCP-02 list_recent_emails basic: PASS
- ...
```

Only open a bug or UX item when something is clearly wrong or clearly improvable.
