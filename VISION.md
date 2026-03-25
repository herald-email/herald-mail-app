# Vision

This document describes the long-term direction for this project. It evolves from an inbox cleanup tool into a full-featured terminal email client.

## Implementation Order

- [x] Fix responsive terminal width (hardcoded values today)
- [x] Refactor to daemon/UI split architecture
- [x] Multi-folder sidebar (collapsible tree, counts)
- [x] Status bar showing active folder, unread/total counts, selection state
- [x] Timeline/thread view + tab navigation
- [x] Compose and reply (after timeline)
- [x] AI-powered inbox classification via Ollama
- [x] Chat panel (talk to your emails with AI)
- [x] MCP server hook
- [x] SSH app mode (charmbracelet/wish)
- [x] Image rendering (iTerm2 inline images)
- [x] Search (in-folder, full-text, cross-folder, IMAP fallback, saved searches)
- [x] Semantic search (natural-language queries via local embeddings)
- [ ] Multi-account support (multiple IMAP accounts in one session)
- [x] Vendor presets (Gmail, Outlook, Fastmail, iCloud — one-line config)
- [x] Forward email with address input
- [x] Email deletion (single message, all from sender, all from domain — moves to Trash)
- [x] Delete individual email from Timeline (`D` on highlighted row or open preview)
- [x] Archive email (`e` key — moves to Archive/All Mail instead of Trash)
- [x] Deletion confirmation prompt
- [x] Automatic new-email sync (IMAP IDLE + background polling)
- [ ] Markdown compose → HTML send (write in Markdown, deliver as multipart HTML+plain)
- [ ] Attachment support (download received attachments, attach files when composing)
- [ ] Text selection (mouse selection + vim-style visual mode for copying body text)
- [ ] Full-screen email view (expand preview to fill the entire terminal)

---

## Architecture: Daemon / UI Split

### Phase 1
Single process, two goroutines: one daemon goroutine (all IMAP, cache, AI logic) and one UI goroutine (Bubble Tea). They communicate via channels and well-defined interfaces. The key discipline is that the Bubble Tea model talks only to a `Backend` interface, never to IMAP directly — this makes the later split free.

### Phase 2
Daemon becomes a real background process with IPC (Unix socket or gRPC). TUI connects to it like a client. This enables MCP hooks, SSH TUI access via `charmbracelet/wish`, and integration with Claude Code or phone apps.

---

## UI Layout

### Tabs (top-level navigation)
Keyboard (number keys) and mouse clickable.

- **Tab 1 — Cleanup**: Current sender/domain grouping view for bulk deletion
- **Tab 2 — Timeline**: Chronological thread list, standard email client layout
- Future: Tab 3 — Compose

### Timeline View
- Full-width thread list sorted by most recent email in thread
- Selecting a thread splits into: left thread list + right email preview panel
- Right panel auto-updates as user scrolls
- Fold/unfold thread replies inline
- Star/pin important threads to top
- Actions: delete thread, delete individual email, forward (before full compose is built)

### Status Bar
A persistent top/bottom bar replacing the current ad-hoc status line:

- **Active folder** — breadcrumb style, e.g. `Labels / Health`
- **Folder counts** — `12 unread / 340 total` pulled from the sidebar status cache
- **Selection state** — `3 senders selected`, `7 messages selected`, or blank when nothing is selected
- **Mode indicator** — `Domain mode` / `Sender mode`, `Logs ON` when log overlay is open
- **Deletion progress** — replaces the inline text currently in the status line: `Deleting 3/5…`
- **Key hints** — condensed one-liner that changes based on which panel is focused (sidebar / summary / details)

### Multi-Folder Sidebar
- Collapsible left panel, toggled by a keyboard shortcut
- Arrow key navigation: forward/space to expand, back to collapse
- Real IMAP folders synced from server

### Chat Panel
- Right-side slide-out panel
- User converses with their emails via a local Ollama model
- Position is fixed on the right; functionality will grow in complexity over time

---

## AI Classification (Ollama)

- Runs locally — Ollama already installed, small model preferred (Mac Mini with limited RAM)
- Qwen is a good candidate for embeddings; Gemma family for classification
- Default behavior: background tagging of new emails (fresh first, then backwards)
- Manual trigger: "Analyze everything" / "Reanalyze" button processes full history
- Categories: subscription, unnecessary, important, and others as needed

---

## Cleanup Mode (Current Core, Expanding)

### Unsubscribe System

**Hard unsubscribe** — actual unsubscription:
- Use RFC 8058 `List-Unsubscribe-Post` header for one-click machine-readable unsubscribe where supported
- Fallback: open the `List-Unsubscribe` browser URL
- Track whether emails keep arriving after unsubscribe; notify/prompt if they do

**Soft unsubscribe** — local only (Yandex-style):
- Create a "Disabled Subscriptions" IMAP folder (or user-named)
- Auto-move all future emails from that sender/domain there
- Inbox stays clean without touching the actual mailing list

Batch flow: present a list of detected subscriptions, let user select and choose mode, then execute.

### Auto-Cleanup Rules
- Per-sender rules: e.g. delete all emails from a subscription sender older than N days
- Offer to run cleanup automatically on a schedule

---

## Compose and Reply

- Write in Markdown with live Bear.app-style preview (`charmbracelet/glamour`, already in place)
- On send: convert Markdown to HTML and deliver as `multipart/alternative` — HTML part for modern clients, auto-generated plain-text part as fallback
- The plain-text fallback is stripped of Markdown syntax (not raw `**bold**`); generated by the same `htmlToText` converter used for reading
- Browser preview button: open rendered HTML in the default browser before sending
- Insert inline images by pasting or dragging a file path into the body; encoded as base64 `multipart/related` attachment
- Full reply (quotes original, Re: prefix) and forward (Fwd: prefix, forwarding header) support

---

## HTML Rendering (Received Emails)

- Best-effort rendering of HTML emails in terminal
- charmbracelet/glamour handles the Markdown path; HTML needs a separate rendering solution

---

## Image Support

- iTerm2 inline images protocol (primary target, user is on macOS/iTerm2)
- Design to be extensible to Kitty graphics protocol for other terminals

---

## Search

### In-folder search (local, fast)
- `/` key opens a search bar at the bottom of the Timeline and Cleanup tabs
- Searches cached metadata (sender, subject) instantly via SQLite `LIKE`
- Results replace the current list view; `Esc` clears the search and restores the full list
- Matched terms highlighted in the results

### Full-text search (body content)
- Extend the local cache to store a plain-text snippet or full body text per email
- SQLite FTS5 virtual table for ranked full-text search across all cached emails
- Search bar prefix `/b ` to switch into body-search mode
- Results show a one-line excerpt with the matched phrase

### Cross-folder search
- Search across all locally cached folders in a single query
- Results grouped by folder with a folder breadcrumb per row
- Selecting a result switches the active folder and highlights the email

### IMAP server-side search (fallback / deep search)
- When the local cache is incomplete (e.g. emails older than the sync window), fall back to IMAP `SEARCH` command
- Triggered explicitly with a `S` key or a "search server" prompt when local results are sparse
- Results fetched and temporarily added to the cache

### Saved searches / filters
- Save a search query as a named virtual folder in the sidebar
- Persisted in the SQLite database; re-executed on demand with `r`

---

## Contact Book

- Start simple: build from To/From/CC headers seen in sent and received mail
- Explore macOS Contacts app API
- Explore CardDAV if ProtonMail Bridge exposes it
- Evolve incrementally as compose/forward features land

---

## MCP Integration

MCP server hook exposes email operations as tools, enabling:
- Claude Code to read, search, and manage email
- Phone app integration
- Arbitrary AI agent access to the local mail store

Ties into the daemon architecture: MCP server is just another client of the daemon.

### Current tools (implemented)

| Tool | Description |
|------|-------------|
| `list_recent_emails` | Most recent emails in a folder, newest-first |
| `search_emails` | Keyword search across sender and subject |
| `get_sender_stats` | Senders ranked by email volume |
| `get_email_classifications` | AI category counts for a folder |

### Vision: full email client over MCP

The goal is to make the MCP server a complete programmatic email client — everything an AI agent needs to act as a capable email assistant without ever opening the TUI. Tools are grouped by capability area.

---

#### Read & discovery

| Tool | Key parameters | Description |
|------|---------------|-------------|
| `list_recent_emails` ✓ | `folder`, `limit` | Most recent emails, newest-first |
| `list_unread_emails` | `folder`, `limit` | Unread emails only; common first step for an agent morning briefing |
| `get_email_body` | `message_id` | Full plain-text body; fetched live from IMAP if not yet cached |
| `get_email_body_html` | `message_id` | Raw HTML body (for agents that render or further process HTML) |
| `get_email_headers` | `message_id` | All MIME headers as a key/value map (useful for List-Unsubscribe, DKIM, etc.) |
| `list_folders` | — | All IMAP folders with unread/total counts |
| `get_folder_stats` | `folder` | Message count, unread count, oldest/newest date, total size |
| `get_thread` | `message_id` | All emails in the same thread (same normalised subject), ordered by date |
| `list_attachments` | `message_id` | Attachment metadata: filename, MIME type, size — without downloading content |
| `get_attachment` | `message_id`, `filename`, `save_path` | Download a specific attachment; returns base64 content or saves to `save_path` |

---

#### Search

| Tool | Key parameters | Description |
|------|---------------|-------------|
| `search_emails` ✓ | `folder`, `query` | Keyword search across sender and subject |
| `semantic_search_emails` | `query`, `folder`, `limit`, `min_score` | Natural-language search via local embeddings; returns results ranked by cosine similarity |
| `search_by_date` | `folder`, `after`, `before` | Filter emails within a date range (ISO 8601) |
| `search_by_sender` | `sender`, `folder` | All emails from an exact sender or domain |

---

#### AI & classification

| Tool | Key parameters | Description |
|------|---------------|-------------|
| `get_email_classifications` ✓ | `folder` | Category counts for a folder |
| `classify_email` | `message_id` | Run AI classification on a single email and return (and persist) its category |
| `classify_folder` | `folder`, `limit` | Classify up to N unclassified emails in the folder; returns progress summary |
| `summarise_email` | `message_id`, `max_words` | Generate a concise summary of an email body using the local Ollama model |
| `summarise_thread` | `message_id`, `max_words` | Summarise an entire thread into a single paragraph |
| `extract_action_items` | `message_id` | List tasks, deadlines, or requests mentioned in an email body |

---

#### Compose & send

| Tool | Key parameters | Description |
|------|---------------|-------------|
| `send_email` | `to`, `subject`, `body`, `from`, `cc`, `bcc`, `attachments` | Send a new email via SMTP; body is plain text or Markdown (converted to HTML); `attachments` is a list of local file paths |
| `reply_to_email` | `message_id`, `body`, `cc`, `attachments` | Reply to an existing email; pre-fills To and `Re:` subject, quotes original |
| `forward_email` | `message_id`, `to`, `note`, `attachments` | Forward with an optional covering note and additional attachments |
| `draft_reply` | `message_id`, `instructions` | LLM drafts a reply from natural-language instructions; returns text without sending |
| `save_draft` | `to`, `subject`, `body`, `from`, `cc`, `bcc` | Save a draft to the Drafts IMAP folder without sending |
| `list_drafts` | — | List all saved drafts with subject, to, and date |
| `send_draft` | `draft_message_id` | Send a previously saved draft and remove it from Drafts |

---

#### Inbox management

| Tool | Key parameters | Description |
|------|---------------|-------------|
| `get_sender_stats` ✓ | `folder`, `top_n` | Senders ranked by email count |
| `delete_email` | `message_id` | Move a single email to Trash |
| `delete_thread` | `message_id` | Move all emails in the thread to Trash |
| `delete_sender` | `sender`, `folder` | Move all emails from a sender/domain to Trash |
| `archive_email` | `message_id` | Move to Archive (equivalent of TUI `e` key) |
| `archive_thread` | `message_id` | Archive all emails in the thread |
| `archive_sender` | `sender`, `folder` | Archive all emails from a sender/domain |
| `bulk_delete` | `message_ids[]` | Move multiple specific messages to Trash in one call |
| `bulk_move` | `message_ids[]`, `destination_folder` | Move multiple specific messages to a folder in one call |
| `move_email` | `message_id`, `destination_folder` | Move a single email to any IMAP folder |
| `mark_read` | `message_id` | Mark as read on the IMAP server |
| `mark_unread` | `message_id` | Mark as unread |
| `flag_email` | `message_id`, `flag` | Set an IMAP flag (`\Flagged`, `\Answered`, etc.) |
| `create_folder` | `name` | Create a new IMAP folder |
| `rename_folder` | `folder`, `new_name` | Rename an IMAP folder |
| `delete_folder` | `folder` | Delete an empty IMAP folder |

---

#### Cleanup & automation

These mirror the TUI Cleanup tab and its planned auto-cleanup rules — the same rules are shared between both interfaces.

| Tool | Key parameters | Description |
|------|---------------|-------------|
| `list_cleanup_rules` | — | List all saved auto-cleanup rules (sender/domain → action) |
| `add_cleanup_rule` | `sender`, `action`, `older_than_days` | Add a rule: e.g. delete all emails from `news@example.com` older than 30 days |
| `remove_cleanup_rule` | `rule_id` | Remove a cleanup rule |
| `run_cleanup_rules` | — | Execute all cleanup rules immediately; returns count of emails affected |
| `unsubscribe_sender` | `message_id` | Hard-unsubscribe via the `List-Unsubscribe` / `List-Unsubscribe-Post` header found in the email |
| `soft_unsubscribe_sender` | `sender`, `folder` | Soft-unsubscribe: auto-move all future emails from this sender to a designated folder |

---

#### Contacts

| Tool | Key parameters | Description |
|------|---------------|-------------|
| `list_contacts` | `limit` | Contacts derived from To/From/CC headers in sent and received mail |
| `search_contacts` | `query` | Find contacts by name or email address |
| `get_contact` | `email` | Full contact record: name, all seen email addresses, last seen date |

---

#### System

| Tool | Key parameters | Description |
|------|---------------|-------------|
| `sync_folder` | `folder` | Trigger incremental IMAP sync; returns count of new messages fetched |
| `sync_all_folders` | — | Sync all folders; returns per-folder new-message counts |
| `get_sync_status` | — | Cache freshness: last sync time per folder, pending embedding count |
| `get_embedding_status` | — | Semantic index progress (N indexed / M total) |
| `get_server_info` | — | Connected account, IMAP server, capabilities (IDLE, MOVE, etc.) |

---

### TUI ↔ MCP shared state

The TUI and MCP server both read from and write to the **same `email_cache.db`**. This means:

- Classifications set via `classify_email` appear immediately in the TUI Cleanup tab's Tag column
- Emails deleted via `delete_email` are removed from the TUI timeline on the next render or sync
- Cleanup rules created via `add_cleanup_rule` are enforced the next time the TUI runs a sync
- Drafts saved via `save_draft` appear in the TUI's Compose drafts list
- Contacts built up via the TUI (from viewed emails) are queryable via MCP `list_contacts`

This makes the two interfaces fully interchangeable: start a task in the TUI, continue it in Claude, or vice versa, with no manual sync step.

---

### Why this is useful

An agent with these tools can handle real-world tasks autonomously:

- *"Summarise everything I received from the finance team this week and list any action items."*
- *"Find the email about my insurance renewal, draft a reply asking for the updated quote, and send it."*
- *"Move all newsletters from the last 3 months to the Archive folder."*
- *"Classify my entire inbox and tell me how many emails need a reply."*

The combination of semantic search + summarisation + compose tools means an agent can operate as a capable inbox assistant with no human in the loop for routine tasks.

---

### Implementation notes

- Tools that require live IMAP (`get_email_body`, `reply_to_email`, `send_email`, `move_email`, flag/read tools, `sync_folder`) need the MCP server to hold an IMAP connection. Currently the server is cache-only; this requires adding an optional live backend mode (connect on demand, idle disconnect after N seconds of inactivity).
- AI tools (`summarise_email`, `draft_reply`, `extract_action_items`) call the same Ollama HTTP API already used by the TUI classifier. No new dependency.
- `✓` marks tools already implemented.

---

## SSH App Mode

`charmbracelet/wish` lets you serve the Bubble Tea TUI over SSH on a custom port. With the daemon architecture in place, this is a small addition — the TUI becomes one of several possible clients.

---

## Semantic Search

Keyword search (SQLite `LIKE`) finds exact matches. Semantic search finds *meaning* — "emails about my tax return" matches messages that say "annual filing", "IRS", "accountant sent documents", even if none contain the word "tax return".

### How it works

1. **Embedding model** — a small local model (e.g. `nomic-embed-text` via Ollama, or a bundled GGUF via `llama.cpp`) converts each email's plain-text body + subject into a dense vector (e.g. 768 floats).
2. **Vector store** — vectors are stored in a separate table in the existing SQLite database using the `sqlite-vec` extension (a single loadable `.so`/`.dylib` file, no separate process). Cosine similarity search runs entirely in SQLite.
3. **Query** — when the user types a natural-language query, the same embedding model converts it to a vector; SQLite returns the K nearest neighbours.
4. **Hybrid ranking** — results are merged with keyword matches and re-ranked: exact keyword hits score higher than semantic-only matches, so precision is not sacrificed.

### Indexing pipeline

- Triggered automatically after each TUI sync: newly cached emails are embedded in a background goroutine (rate-limited to avoid saturating the Ollama API or CPU)
- Embedding is skipped for emails with no body text
- Re-embedding on body change is detected by hashing the plain-text content
- A progress indicator in the status bar shows `✦ embedding N/M` while indexing is in progress

### UX

- In the search bar, prefix with `?` to switch to semantic mode: `? emails about my lease renewal`
- Without a prefix, the existing keyword search runs as today
- Results show a similarity score badge (`87%`) next to each row
- A "Why this result?" hint is available (shows the matched excerpt that drove the score)

### Configuration

```yaml
semantic:
  enabled: true          # default: true when Ollama is configured
  model: "nomic-embed-text"   # Ollama embedding model to use
  batch_size: 20         # emails to embed per background tick
  min_score: 0.65        # minimum cosine similarity to include in results
```

### Dependencies

| Component | Role |
|-----------|------|
| `sqlite-vec` | Vector similarity search inside SQLite (no extra process) |
| Ollama `nomic-embed-text` | Local embedding model (already a project dependency) |

`sqlite-vec` is a single dynamically-loaded extension; no schema changes beyond a new `email_embeddings` table.

### Privacy

All embeddings are computed locally. No email content is sent to any remote service. The embedding model runs inside the existing Ollama instance the user already has.

---

## Multi-Account Support

The app currently hard-codes a single IMAP+SMTP connection from one config file. The goal is to manage multiple accounts (e.g. personal ProtonMail + work Gmail) in a single session.

### Account model
- `proton.yaml` gains a top-level `accounts:` list; each entry has a `name`, IMAP credentials, SMTP credentials, and an optional `vendor` shorthand
- A default single-account config (current format) continues to work unchanged
- Each account gets its own IMAP connection, its own SQLite cache file, and its own set of folders

### UI changes
- The folder sidebar groups folders under an account header (e.g. `● personal` / `● work`)
- Switching between account folders works identically to switching folders today
- The status bar shows the active account name alongside the folder
- Compose: a "From" field lets the user pick which account to send from
- All tabs (Timeline, Cleanup, Search) can optionally show a unified view across accounts or be scoped to one

### Vendor presets
Common providers ship with sensible defaults so users only need to supply credentials:

| Vendor | IMAP host / port | SMTP host / port | Notes |
|--------|-----------------|-----------------|-------|
| `protonmail` | `127.0.0.1:1143` | `127.0.0.1:1025` | Via ProtonMail Bridge; default today |
| `gmail` | `imap.gmail.com:993` | `smtp.gmail.com:587` | App password required |
| `outlook` | `outlook.office365.com:993` | `smtp.office365.com:587` | OAuth or app password |
| `fastmail` | `imap.fastmail.com:993` | `smtp.fastmail.com:587` | App password |
| `icloud` | `imap.mail.me.com:993` | `smtp.mail.me.com:587` | App-specific password |

With a preset, config shrinks to:

```yaml
accounts:
  - name: personal
    vendor: protonmail
    credentials:
      username: me@pm.me
      password: bridge_password
  - name: work
    vendor: gmail
    credentials:
      username: me@company.com
      password: app_password
```

### OAuth2 (future)
Gmail and Outlook prefer OAuth2 over app passwords. A future phase adds a `vendor_auth` flow that opens a browser for the OAuth dance and stores the refresh token in the system keychain.

---

## Automatic New-Email Sync

### IMAP IDLE (primary mechanism)
IMAP IDLE (`RFC 2177`) is the proper push-like mechanism: the client sends an `IDLE` command and the server sends unsolicited `EXISTS` or `EXPUNGE` responses when the mailbox changes, without the client having to poll. This is far more efficient than periodic polling and is supported by all major servers (ProtonMail Bridge, Gmail, Outlook, Fastmail).

Implementation plan:
- Maintain a **dedicated IDLE connection** for the active folder alongside the existing command connection (go-imap supports this via a second `client.Client`)
- When the server sends `EXISTS` (new message count increased), trigger an incremental fetch of only the new messages and append them to the cache and timeline in-place
- The TUI receives a new message type (e.g. `NewEmailsMsg`) and inserts the rows at the top of the timeline without a full reload
- On `EXPUNGE` (message deleted elsewhere), remove the matching row from the cache and timeline
- IDLE must be re-issued every 29 minutes (server timeout); the background goroutine handles the keepalive automatically

### Background polling (fallback)
For servers or configurations where IDLE is unavailable or unreliable:
- Configurable poll interval (default: 60 seconds) in `proton.yaml` under `sync.interval`
- Polling checks only the active folder; other folders are checked less frequently (e.g. every 5 minutes) to keep the sidebar counts fresh
- Polling is also used for **non-active folders** even when IDLE is active on the current folder

### Configuration
```yaml
sync:
  idle: true          # Use IMAP IDLE when available (default: true)
  interval: 60        # Fallback poll interval in seconds (default: 60)
  background: true    # Sync other folders in background (default: true)
  notify: true        # Show a status bar flash on new email arrival (default: true)
```

### UX
- A subtle indicator in the status bar shows sync state: `↻ live` when IDLE is active, `↻ 42s` counting down to the next poll
- New emails slide into the top of the timeline with a brief highlight so the user notices them without being interrupted
- An unread badge on the folder sidebar updates automatically as new mail arrives

---

## Forward and Deletion UX

### Forward email
- `F` key in Timeline opens Compose pre-filled with:
  - `To` field empty and focused — user types the recipient address
  - Subject prefixed with `Fwd:`
  - Body quoted with a forwarding header (From / Date / Subject / original body text)
- If the email body is already loaded in the preview panel, it is included in the quote; otherwise only the metadata header is pre-filled

### Delete from Timeline
- `D` on any Timeline row (collapsed thread, expanded email, or open preview) deletes that specific email
- For a collapsed `[N]` thread header, `D` prompts: "Delete all N emails in this thread?"
- Moves to Trash, same as the Cleanup tab path

### Archive from Timeline and Cleanup
- `e` archives the highlighted email or sender (moves to `Archive` / `[Gmail]/All Mail` / `Archives` — tries known names in order)
- Archive is non-destructive: email is still searchable and accessible from the Archive folder
- Works in both Timeline (single email or full thread) and Cleanup (all emails from the selected sender)

### Deletion confirmation
- `D` never deletes immediately — it opens an inline prompt in the status bar:
  `Delete 3 senders? [y] confirm  [n/Esc] cancel`
- The prompt describes exactly what would be deleted (N message(s), N sender(s), a specific sender name, or a domain)
- `y` or `Y` confirms and proceeds; any other key or `Esc` cancels silently
- This applies to all deletion paths: single message, selected messages, single sender, selected senders, domain

---

## Attachment Support

### Receiving attachments
- Attachments are detected from `BodyStructure` during sync and shown in the email preview with a filename, MIME type, and size
- `Enter` on an attachment descriptor (or a dedicated key, e.g. `s`) opens a save-to-disk prompt with a default path (`~/Downloads/<filename>`)
- Downloaded files are saved via IMAP part fetch; no need to re-download the full message
- Inline images that are too large for iTerm2 rendering are also accessible this way

### Sending attachments
- In Compose, `Ctrl+A` opens a file picker prompt (path input with tab-completion against the filesystem)
- Multiple attachments can be added; each appears as a line below the body: `[attach] report.pdf  (42 KB)`
- `Ctrl+A` on an existing attachment line removes it
- Files are base64-encoded and sent as `multipart/mixed` wrapping the `multipart/alternative` body
- File size warning shown inline if a single attachment exceeds 10 MB

### MCP
`send_email` and `reply_to_email` tools accept an optional `attachments` list of local file paths.

---

## Text Selection

Bubble Tea's alt-screen mode captures all input, so the terminal's native mouse selection is disabled. Two complementary mechanisms restore copy-ability:

### Mouse selection mode
- `m` toggles mouse-selection mode: the TUI temporarily releases mouse capture (`tea.DisableMouse`) so the terminal's native text selection works normally
- A visible indicator in the status bar shows `[mouse] select mode — press m to return`
- Pressing `m` again re-enables Bubble Tea mouse handling and returns to normal TUI operation

### Vim-style visual selection
- In the email preview panel, `v` enters visual line mode
- `j`/`k` extend the selection; the selected lines are highlighted
- `y` yanks the selected text to the system clipboard (via `pbcopy` on macOS, `xclip`/`wl-copy` on Linux)
- `Esc` cancels visual mode without copying
- Works for any text visible in the preview — body, headers, quoted blocks

### Quick copy shortcuts
- `yy` in the preview panel copies the current line to clipboard without entering visual mode
- `Y` copies the entire visible body text (all lines, not just the viewport)

---

## Full-Screen Email View

- `z` (or `Enter` when preview is already open) expands the email preview to fill the entire terminal — tab bar, sidebar, timeline table, and status bar are all hidden
- The full-screen view shows the full email with the same scroll controls (`j`/`k`, `PgUp`/`PgDn`)
- Header block (From / Date / Subject) remains pinned at the top
- `z` or `Esc` exits full-screen and restores the previous split layout
- Full-screen mode also works in the Cleanup details panel

---

## Theming

- Dark theme default (current is acceptable)
- Inherit terminal color profile where possible
- App-level theme system as a future feature
