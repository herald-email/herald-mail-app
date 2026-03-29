# Vision

This document describes the long-term direction for this project. It evolves from an inbox cleanup tool into a full-featured terminal email client with a native app and a local daemon server.

---

## Implementation Status

High-level milestones. Detailed feature status is in each section below.

- [x] Responsive terminal layout (no hardcoded widths)
- [x] Backend interface discipline (UI never touches IMAP/SQL directly)
- [x] Multi-folder sidebar (collapsible tree, unread/total counts)
- [x] Status bar (folder, counts, selection state, mode, key hints)
- [x] Timeline / thread view + tab navigation
- [x] Compose, reply, forward
- [x] Email deletion (single, by sender, by domain — moves to Trash)
- [x] Archive (`e` key — moves to Archive / All Mail)
- [x] Deletion confirmation prompt
- [x] Attachment support (download + attach when composing)
- [x] Markdown compose → HTML send (multipart HTML + plain)
- [x] Full-screen email preview (`z`)
- [x] Text selection (mouse mode + vim visual mode + clipboard)
- [x] AI classification via Ollama
- [x] Chat panel (ask questions about your inbox)
- [x] Semantic search (natural-language queries via local embeddings)
- [x] Search (in-folder, full-text FTS5, cross-folder, IMAP fallback, saved searches)
- [x] MCP server (read/search/classify tools for Claude Code)
- [x] SSH app mode (`cmd/ssh-server` via charmbracelet/wish)
- [x] iTerm2 inline image rendering
- [x] Vendor presets (Gmail, Outlook, Fastmail, iCloud — one-line config)
- [x] Background new-email polling
- [x] Hard unsubscribe via List-Unsubscribe headers (`u` key)
- [x] Incremental IMAP sync (UIDNEXT-based, instant on no new mail)
- [x] Background cache reconciliation (valid-ID ground truth, stale entries removed)
- [ ] IMAP IDLE (real push; currently polling only)
- [ ] Email preview in Cleanup tab (open individual email at 50%, panels shrink to 25%)
- [ ] Soft unsubscribe (auto-move future emails to a local folder)
- [ ] Custom classification prompts (user-defined categories + data extraction)
- [ ] Classification actions (notify, command, webhook, move, flag on match)
- [ ] Auto-cleanup rules (per-sender delete/archive older than N days)
- [ ] Multi-account support
- [ ] Chat tool calling (Ollama tool API + MCP tools in-process)
- [ ] Filtered timeline from chat results
- [ ] Multiple AI backends (Claude, OpenAI-compatible)
- [ ] AI writing assistant in Compose (style, tone, grammar, subject suggest)
- [ ] Quick replies (canned + AI-generated contextual options)
- [ ] Contact book
- [ ] Settings / onboarding screen (no YAML editing required)
- [ ] Keychain integration (passwords stored in OS keychain, not plaintext YAML)
- [ ] README with MCP setup prompts for Claude / Cursor / Codex
- [ ] Daemon server (`mail-processor serve`, Ollama-style)
- [ ] Native app client (Phase 3)

---

## Architecture: Daemon / Client Split

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full technical reference.

The app is designed in three phases so that each phase's work is additive and nothing needs to be rewritten.

### Phase 1 — current
Single process. The Bubble Tea model talks only to a `Backend` interface, never to IMAP or SQLite directly. All processing (IMAP, cache, AI) lives behind that interface. The discipline means the UI can be swapped or multiplied without touching the backend.

### Phase 2 — daemon server (Ollama-style)
The backend becomes a standalone daemon: `mail-processor serve`. It runs as a persistent background process, holds the IMAP connection, owns the SQLite cache, and exposes a local HTTP + WebSocket API on a configurable port (default `localhost:7272`). Clients connect to it — they do not touch IMAP or the database directly.

CLI mirrors Ollama's UX:
```
mail-processor serve          # start daemon (foreground; launchd/systemd for autostart)
mail-processor status         # show running daemon info
mail-processor stop           # graceful shutdown
mail-processor sync           # trigger incremental IMAP sync
```

All existing client modes (TUI, SSH, MCP) become thin clients of the daemon. A `RemoteBackend` struct implements the same `Backend` interface over HTTP/WebSocket — TUI code is unchanged, only the backend wiring differs.

### Phase 3 — native app
A native desktop client (macOS-first via SwiftUI; cross-platform alternative via Wails) connects to the same daemon API. The server and client are distributed separately, just like Ollama and its frontends. Multiple clients can run simultaneously against one daemon with no data races.

---

## UI Layout

The TUI uses a fixed tab bar at the top, a collapsible folder sidebar on the left, and a main content area whose layout changes per tab.

### Tabs (top-level navigation)
Keyboard (number keys) and mouse clickable.

- [x] Tab 1 — Timeline: chronological email list with body preview split
- [x] Tab 2 — Compose: write and send email
- [x] Tab 3 — Cleanup: sender/domain grouping for bulk deletion

### Timeline View

The primary reading interface. Shows emails sorted newest-first, grouped by thread when multiple messages share the same subject. Selecting a row opens a body preview split on the right.

- [x] Full-width thread list sorted by date
- [x] Thread grouping (fold/unfold inline with `Enter`)
- [x] Body preview split (right panel, auto-updates on navigation)
- [x] Full-screen preview (`z`)
- [x] Actions: delete, archive, reply, forward
- [ ] Star / pin important threads to top

### Status Bar

A single persistent line at the bottom of the screen. Its content changes based on which panel is focused.

- [x] Active folder breadcrumb
- [x] Folder counts (unread / total)
- [x] Selection state (N senders selected, N messages selected)
- [x] Mode indicator (Domain mode, Sender mode, Logs ON)
- [x] Deletion progress (Deleting 3/5…)
- [x] Key hints (changes per panel)
- [x] Sync countdown (↻ 42s to next poll, ↻ live when IDLE active)

### Multi-Folder Sidebar

- [x] Collapsible left panel (toggled with `f`)
- [x] Real IMAP folders synced from server
- [x] Unread / total counts per folder
- [x] Keyboard navigation (j/k, Enter to switch folder)
- [x] Auto-hides with a hint when terminal is too narrow

### Chat Panel

The chat panel is a right-side slide-out (`c` key) that lets you have a conversation with your inbox using a local Ollama model. It currently supports Q&A over email content. The vision is to evolve it into a full agentic assistant that can search, summarise, compose, and manage email through natural conversation.

- [x] Slide-out panel (`c` key)
- [x] Conversation history (multiple turns)
- [x] Markdown rendering of assistant responses (glamour)
- [x] Context: currently open email available to the model
- [ ] Tool calling via Ollama's native tool API
- [ ] In-process MCP tools (no stdio round-trip)
- [ ] Filtered timeline: chat result sets pushed into Timeline as a live view
- [ ] Context: active folder and selection state passed to model
- [ ] `draft_reply` / `send_email` from within chat
- [ ] Multiple AI backends (Ollama, Claude, OpenAI-compatible)

#### Tool calling (planned)

When tool calling is implemented, the chat will use Ollama's `tools` field in `/api/chat` to invoke the same functions exposed by the MCP server, directly in-process. The model decides which tools to call; the app executes them and feeds results back until the model produces a final reply.

Planned tools mirror the MCP surface: search, read body, summarise, reply, manage, classify.

#### Filtered timeline (planned)

When the chat returns a set of emails (from a search, date filter, or semantic query), those results are pushed into the Timeline tab as a live filtered view. The user can browse and act on them without leaving the chat flow. `Esc` or "show all" restores the full timeline.

#### Multiple AI backends (planned)

| Backend | How | When to use |
|---------|-----|-------------|
| Ollama (local) | `/api/chat` with tools | Default; fully offline, no cost |
| Claude | `claude-sonnet-4-6` via Anthropic SDK | Stronger reasoning, better tool use |
| OpenAI-compatible | Any server speaking OpenAI chat completions | Flexibility |

---

## AI Classification

The app can automatically tag emails with categories (subscription, important, unnecessary, etc.) using a local Ollama model. Classification runs in the background after sync — it never blocks the UI. The `a` key triggers a full classification pass on the current folder.

- [x] Background classification via Ollama (`a` key)
- [x] Category tags stored in SQLite (`email_classifications` table)
- [x] Tag column visible in Timeline and Cleanup tabs
- [x] MCP tool: `classify_email` (single message)
- [ ] `classify_folder` MCP tool (batch, with progress)
- [ ] Auto-classify new emails as they arrive (background, rate-limited)
- [ ] Reanalyse / override existing tags

### Custom Classification Prompts

The built-in classification prompt assigns one of six fixed categories (`sub`, `news`, `imp`, `txn`, `soc`, `spam`). Custom prompts let users define their own categories and extraction logic tailored to their workflow — e.g. extracting order numbers from receipts, flagging emails from specific clients, or categorizing by project.

- [ ] `classification_prompts` section in `proton.yaml` — list of named prompt definitions
- [ ] Each prompt specifies: name, system prompt text, list of valid output categories, and an optional data extraction instruction (e.g. "extract the tracking number")
- [ ] Default built-in prompt used when no custom prompts are configured (current behaviour preserved)
- [ ] Multiple prompts can run on the same email (e.g. one for category, one for data extraction)
- [ ] `custom_categories` table in SQLite storing prompt name + category + extracted data per email
- [ ] TUI displays custom categories alongside the built-in tag column
- [ ] MCP tools: `list_classification_prompts`, `classify_email_custom` (run a named prompt on one email)

### Classification Actions

When an email matches a category, the system can trigger an action automatically. Actions turn classification from passive tagging into an active assistant — sending OS notifications for important mail, running shell commands with extracted data, or auto-filing emails into folders. Actions execute in the background daemon (Phase 2) so they fire even when the TUI is not running.

- [ ] `classification_actions` section in `proton.yaml` — list of rules mapping category → action
- [ ] Each rule specifies: category match (built-in or custom), action type, and action config
- [ ] Action type: `notify` — send an OS-level notification (macOS Notification Center / `notify-send` on Linux) with sender, subject, and optional extracted data
- [ ] Action type: `command` — run a shell command with template variables (`{{.Sender}}`, `{{.Subject}}`, `{{.Category}}`, `{{.ExtractedData}}`, `{{.MessageID}}`)
- [ ] Action type: `webhook` — POST a JSON payload to a URL (for Slack, Discord, Home Assistant, etc.)
- [ ] Action type: `move` — auto-move the email to a specified IMAP folder
- [ ] Action type: `flag` — set IMAP flags (e.g. `\Flagged`, `\Seen`)
- [ ] Actions run in the daemon (Phase 2) on every newly classified email, even when TUI/UI is offline
- [ ] Action execution logged to SQLite (`classification_action_log` table) with timestamp and result
- [ ] Dry-run mode: `--dry-run` flag logs what actions would fire without executing them
- [ ] MCP tools: `list_classification_actions`, `add_classification_action`, `remove_classification_action`, `get_action_log`

#### Example configuration

```yaml
classification_prompts:
  - name: project-tagger
    prompt: |
      Given this email, respond with exactly one project name:
      infra, backend, frontend, hiring, other
      Sender: {sender}
      Subject: {subject}
      Tag:
    categories: [infra, backend, frontend, hiring, other]

  - name: order-extractor
    prompt: |
      If this email contains an order or tracking number, extract it.
      Respond with ONLY the number, or "none" if not found.
      Sender: {sender}
      Subject: {subject}
      Number:
    extract: true

classification_actions:
  - category: imp
    action: notify
    title: "Important email"
    body: "From {{.Sender}}: {{.Subject}}"

  - category: txn
    prompt: order-extractor
    action: command
    command: "echo '{{.ExtractedData}}' >> ~/orders.log"

  - category: infra
    prompt: project-tagger
    action: webhook
    url: "https://hooks.slack.com/services/XXX"
    body: '{"text": "Infra email from {{.Sender}}: {{.Subject}}"}'
```

---

## Cleanup Mode

The Cleanup tab groups emails by sender or domain and shows volume statistics, making it easy to identify and bulk-delete noise. It is the original core of the app and the area where unsubscribe and auto-cleanup rules will land.

### Sender / Domain grouping

- [x] Group by sender (default)
- [x] Group by domain (`d` key toggle)
- [x] Stats per sender: count, size, date range
- [x] Details panel: individual emails for selected sender
- [x] Bulk delete: all from sender, all from domain
- [x] Bulk archive: all from sender

### Email preview in Cleanup

The Cleanup tab has two panels side by side: the sender summary (left) and the email list for the selected sender (right). Today the email list is read-only. The goal is to make individual emails fully actionable from within Cleanup — open, read, reply, unsubscribe — without switching to the Timeline tab.

**Layout when an email is open:**
- Folder sidebar hides completely (same as full-screen mode in Timeline)
- Summary panel (sender list) shrinks to 25% of the terminal width
- Email list panel (details) shrinks to 25%
- Email preview panel opens at 50% on the right

This gives enough room to read the email while keeping both panels visible as context. `Esc` closes the preview and restores the normal two-panel layout.

- [x] `Tab` cycles focus between the summary panel and the email list panel
- [ ] `Enter` on a row in the email list opens the email preview at 50% width
- [ ] Folder sidebar hides when preview is open; restores on `Esc`
- [ ] Summary and email list panels each shrink to 25% when preview is open
- [ ] Preview panel supports the same scroll controls as Timeline (`j`/`k`, `PgUp`/`PgDn`)
- [ ] `r` / `R` — reply from within Cleanup preview (opens Compose, pre-filled)
- [ ] `u` — unsubscribe from within Cleanup preview
- [ ] `D` — delete the open email from within the preview
- [ ] `e` — archive the open email from within the preview
- [ ] `z` — expand to full-screen (same as Timeline full-screen mode)
- [ ] `Esc` — close preview, restore two-panel Cleanup layout

### Unsubscribe

Triggered with `u` on any email in the Timeline or Cleanup detail view. The app reads `List-Unsubscribe` and `List-Unsubscribe-Post` headers stored during sync.

- [x] Hard unsubscribe via RFC 8058 one-click POST (`List-Unsubscribe-Post`)
- [x] Fallback: open `List-Unsubscribe` mailto link (opens in default mail client)
- [ ] Fallback: open `List-Unsubscribe` browser URL (for HTTP links)
- [ ] Track whether emails keep arriving after unsubscribe; notify / prompt if they do
- [ ] Soft unsubscribe: auto-move all future emails from sender to a "Disabled Subscriptions" IMAP folder (local-only, inbox stays clean without touching the actual list)
- [ ] Batch unsubscribe flow: present list of detected subscriptions, select, choose mode, execute

### Auto-Cleanup Rules

Rules let the app automatically act on email from known senders — delete newsletters older than 30 days, archive promotional email weekly, etc. Rules are defined per-sender or per-domain and stored in SQLite.

- [ ] Per-sender / per-domain rules (action + older-than-days condition)
- [ ] Rule storage in SQLite (`cleanup_rules` table)
- [ ] Manual rule execution (`run_cleanup_rules` trigger)
- [ ] Scheduled execution (configurable interval in `proton.yaml`)
- [ ] TUI rule manager (list, add, remove)
- [ ] MCP tools: `list_cleanup_rules`, `add_cleanup_rule`, `remove_cleanup_rule`, `run_cleanup_rules`

---

## Compose and Reply

Write in Markdown, deliver as properly formatted HTML email. The compose tab is a full-screen editor with a live preview mode and attachment support.

- [x] Markdown editor (textarea)
- [x] Live Markdown preview (`Ctrl+P`)
- [x] Send as multipart HTML + plain-text via SMTP
- [x] Reply (`R` key — pre-fills To, Re: subject, quotes original)
- [x] Forward (`F` key — pre-fills Fwd: subject, forwarding header, body quote)
- [x] Attachment support: attach files (`Ctrl+A`), attach list shown in compose
- [x] Send with attachments (`multipart/mixed`)
- [ ] Browser preview (open rendered HTML in default browser before sending)
- [ ] Inline images (paste / drag file path → base64 `multipart/related`)
- [ ] `send_email` MCP tool
- [ ] `reply_to_email` MCP tool
- [ ] `forward_email` MCP tool
- [ ] `draft_reply` MCP tool (LLM drafts reply from natural-language instructions)
- [ ] `save_draft` / `send_draft` / `list_drafts` MCP tools

### AI Writing Assistant (Compose)

While composing, the local Ollama model acts as an inline writing assistant — no cloud, no round-trip, just a keystroke. The assistant operates on the current draft and replaces or annotates it in place. All rewrites are diff-previewed before applying: old text greyed out, new text highlighted; `y` accepts, `n` keeps the original.

- [ ] **Spell / grammar check** (`Ctrl+G`) — highlights errors inline; `Tab` to accept suggestion, `Esc` to dismiss
- [ ] **Style rewrite** (`Ctrl+W`) — rewrites the selected paragraph in a cleaner, more concise style while preserving meaning
- [ ] **Tone adjuster** — cycle through tones: `professional → friendly → direct → formal`; model rewrites the draft to match
- [ ] **Subject line suggest** — when To is filled and the body has content, offer 3 subject line options ranked by clarity
- [ ] **Length adjust** — `shorten` (condense to key points) / `expand` (add context and detail)

### Quick Replies

When reading an email, a set of contextual one-click replies is offered below the preview. The user picks one, it opens pre-filled in Compose ready to edit or send immediately. Eliminates the most repetitive writing without removing the human in the loop.

Built-in canned replies (always shown, no model needed):
- [ ] "No thanks" — polite decline
- [ ] "Thank you for reaching out"
- [ ] "Thank you, [Name]" — first name extracted from the From header
- [ ] "Copy that" — simple acknowledgement
- [ ] "I'll get back to you" — defer

AI-generated contextual replies (shown when body is loaded):
- [ ] Model reads the email body and generates 3 short reply options relevant to the content
- [ ] `Ctrl+Q` opens the quick-reply picker; arrow keys to select, `Enter` to open in Compose, `Esc` to cancel
- [ ] Works in both split-preview and full-screen view

---

## HTML Rendering (Received Emails)

Received emails are converted from HTML to Markdown for display in the terminal. The conversion is best-effort — complex layouts simplify gracefully.

- [x] HTML → Markdown conversion for body preview
- [x] Inline image rendering (iTerm2 protocol)
- [ ] Kitty graphics protocol support (for non-iTerm2 terminals)

---

## Search

Search is layered: fast local metadata search first, full-text body search next, IMAP server-side search as a fallback for emails not yet cached.

### In-folder search (local, fast)
- [x] `/` key opens search bar in Timeline and Cleanup tabs
- [x] SQLite `LIKE` search on sender + subject
- [x] Results replace current list view; `Esc` clears
- [ ] Matched term highlighting in results

### Full-text search (body content)
- [x] Body text cached per email (`body_text` column)
- [x] SQLite FTS5 virtual table (when available)
- [x] `/b ` prefix switches to body-search mode
- [ ] One-line excerpt with matched phrase in results

### Cross-folder search
- [x] Searches all locally cached folders in one query
- [ ] Results grouped by folder with breadcrumb per row
- [ ] Selecting a result switches folder and highlights the email

### IMAP server-side search (fallback)
- [x] Falls back to IMAP `SEARCH` when local results are sparse
- [ ] Explicit `S` key to force a server search

### Saved searches / filters
- [x] Save named searches persisted in SQLite
- [x] Listed and executable from the TUI
- [ ] Appear as virtual folders in the sidebar

### Semantic search
- [x] `?` prefix in search bar triggers semantic mode
- [x] Local embeddings via `nomic-embed-text` (Ollama)
- [x] Vectors stored in SQLite (`email_embeddings` table)
- [x] Cosine similarity ranking
- [x] `semantic_search_emails` MCP tool
- [ ] Similarity score badge (`87%`) per result row
- [ ] Hybrid ranking (keyword + semantic merged)
- [ ] "Why this result?" hint (matched excerpt)

---

## MCP Integration

The MCP server exposes email operations as tools, enabling Claude Code and other AI agents to read, search, classify, and eventually manage email without opening the TUI. It reads directly from `email_cache.db` and shares state with the TUI via SQLite WAL mode.

### Implemented tools

| Tool | Description |
|------|-------------|
| `list_recent_emails` | Most recent emails in a folder, newest-first |
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

### Planned tools

- [ ] `get_thread` — all emails in a thread ordered by date
- [ ] `list_attachments` — attachment metadata without downloading
- [ ] `get_attachment` — download a specific attachment
- [ ] `list_folders` — all IMAP folders with counts
- [ ] `classify_folder` — batch classify with progress
- [ ] `extract_action_items` — tasks and deadlines from an email body
- [ ] `summarise_thread` — one-paragraph thread summary
- [ ] `send_email` — send via SMTP
- [ ] `reply_to_email` — reply with pre-filled headers
- [ ] `forward_email` — forward with covering note
- [ ] `draft_reply` — LLM drafts a reply from instructions
- [ ] `save_draft` / `list_drafts` / `send_draft`
- [ ] `delete_email` / `delete_thread` / `bulk_delete`
- [ ] `archive_email` / `archive_thread` / `archive_sender`
- [ ] `move_email` / `bulk_move`
- [ ] `mark_read` / `mark_unread`
- [ ] `create_folder` / `rename_folder` / `delete_folder`
- [ ] `unsubscribe_sender` — hard-unsubscribe via List-Unsubscribe header
- [ ] `soft_unsubscribe_sender` — auto-move future emails to a folder
- [ ] `list_cleanup_rules` / `add_cleanup_rule` / `run_cleanup_rules`
- [ ] `list_contacts` / `search_contacts` / `get_contact`
- [ ] `sync_folder` / `sync_all_folders` / `get_sync_status`
- [ ] `get_server_info`

### TUI ↔ MCP shared state

Both processes read and write the same `email_cache.db` via SQLite WAL mode. Classifications set via `classify_email` appear immediately in the TUI's Tag column. Emails deleted via `delete_email` disappear from the TUI on the next render.

### Simultaneous TUI + MCP operation

The TUI and MCP server are safely runnable at the same time:

- [x] SQLite WAL mode (readers never block writers)
- [x] IMAP connection isolation (each process holds its own connection)
- [x] Short atomic writes (no long-lived transactions)
- [ ] Daemon architecture (Phase 2): both become clients of a single daemon that serialises all IMAP writes

---

## Automatic New-Email Sync

New emails are detected without a full restart. The current implementation uses background polling; IMAP IDLE is the target for true push behaviour.

### IMAP IDLE (target)
IMAP IDLE (`RFC 2177`) lets the server push `EXISTS` and `EXPUNGE` notifications to the client without polling. It is more efficient and eliminates the delay between arrival and display.

- [ ] Dedicated IDLE connection alongside the command connection
- [ ] `EXISTS` → trigger incremental fetch, prepend rows to timeline
- [ ] `EXPUNGE` → remove matching row from cache and timeline immediately
- [ ] 29-minute keepalive (re-issue IDLE before server timeout)

### Background polling (current)
- [x] Configurable poll interval (`sync.interval` in `proton.yaml`, default 60s)
- [x] Polling detects new emails via UIDNEXT comparison
- [x] New emails cached and prepended to timeline
- [x] Sync countdown shown in status bar (↻ 42s)
- [ ] Per-folder poll frequency (active folder more often; background folders less)

---

## Multi-Account Support

The app currently supports one IMAP account per config file. Multi-account support will allow managing several inboxes (e.g. personal + work) in a single session.

- [ ] `accounts:` list in `proton.yaml` (current single-account format still works)
- [ ] Per-account IMAP connection, cache file, and folder tree
- [ ] Folder sidebar grouped under account headers
- [ ] Status bar shows active account name
- [ ] Compose "From" field lets user pick sending account
- [ ] Unified Timeline view across accounts (opt-in)
- [ ] OAuth2 for Gmail / Outlook (future; current: app passwords only)
- [x] Vendor presets: `protonmail`, `gmail`, `outlook`, `fastmail`, `icloud`

---

## Contact Book

Contacts are derived from To/From/CC headers seen in sent and received mail — no import required. They power name completion in Compose and will feed into chat context.

- [ ] `contacts` table in SQLite (built from email headers during sync)
- [ ] Name + all seen addresses + last-seen date per contact
- [ ] Autocomplete in Compose `To`/`CC`/`BCC` fields
- [ ] `list_contacts` / `search_contacts` / `get_contact` MCP tools
- [ ] macOS Contacts app integration (future)
- [ ] CardDAV sync if ProtonMail Bridge exposes it (future)

---

## SSH App Mode

`charmbracelet/wish` serves the full TUI over SSH on port 2222. Each SSH session gets its own `LocalBackend` (independent IMAP connection).

- [x] `cmd/ssh-server` binary
- [x] Each session: independent LocalBackend + IMAP connection
- [ ] In Phase 2: each session connects to the shared daemon instead

---

## Forward and Deletion UX

- [x] `F` key in Timeline opens Compose pre-filled for forward (Fwd: subject, quoted body)
- [x] `R` key in Timeline opens Compose pre-filled for reply (Re: subject, quoted body, To pre-filled)
- [x] `D` in Timeline deletes the highlighted email (single message)
- [x] `D` on a collapsed `[N]` thread prompts to delete all N emails
- [x] `e` archives the highlighted email or sender
- [x] Inline confirmation prompt (`y` to confirm, `Esc` to cancel)

---

## Attachment Support

- [x] Attachments detected from `BodyStructure` during sync
- [x] Attachment list shown in email preview (filename, MIME type, size)
- [x] Save attachment to disk (prompted path, default `~/Downloads/<filename>`)
- [x] Attach files when composing (`Ctrl+A`, tab-completion)
- [x] Multiple attachments; each listed below the body
- [x] Sent as `multipart/mixed`
- [ ] File size warning for attachments over 10 MB

---

## Text Selection

Bubble Tea's alt-screen captures all input, so the terminal's native mouse selection is disabled. Two mechanisms restore copy-ability.

- [x] `m` toggles mouse-selection mode (releases mouse capture; status bar indicator)
- [x] `v` in preview enters vim-style visual line mode
- [x] `y` yanks selected lines to system clipboard (`pbcopy` / `xclip`)
- [x] `Esc` cancels visual mode
- [x] `yy` copies current line; `Y` copies entire visible body

---

## Full-Screen Email View

- [x] `z` (or `Enter` when preview is already open) expands preview to fill the terminal
- [x] Tab bar, sidebar, timeline table hidden in full-screen
- [x] Header (From / Date / Subject) pinned at top
- [x] Same scroll controls (`j`/`k`, `PgUp`/`PgDn`)
- [x] `z` or `Esc` exits and restores split layout

---

## Settings / Onboarding Screen

First-run experience and ongoing configuration should not require the user to edit a YAML file. A TUI settings screen lets users configure accounts, server details, AI, and sync preferences interactively. The YAML file remains the source of truth on disk — the settings screen reads and writes it.

### First-run wizard
- [ ] Detected on startup when no config file exists
- [ ] Step 1 — Account type: pick a vendor preset (ProtonMail, Gmail, Outlook, Fastmail, iCloud) or "Custom"
- [ ] Step 2 — Credentials: username + password fields (masked); password optionally saved to keychain
- [ ] Step 3 — AI: enter Ollama host (default `localhost:11434`), pick model from detected list, skip if Ollama not running
- [ ] Step 4 — Sync: poll interval, IMAP IDLE toggle
- [ ] Step 5 — Test connection button; shows result inline before saving
- [ ] Writes `proton.yaml` on finish

### In-app settings panel
- [ ] Accessible from the TUI with `?` or `,` key (or a Settings tab)
- [ ] Editable fields for all config sections: credentials, server, SMTP, AI, sync
- [ ] Account list for multi-account (add / remove / reorder)
- [ ] Changes saved on `Ctrl+S`; no restart required for most settings
- [ ] Passwords always hidden; "reveal" button toggles visibility

---

## Security / Keychain

Passwords in `proton.yaml` are stored in plaintext, which is acceptable for a local tool but not ideal. The keychain integration stores credentials in the OS keychain and replaces the plaintext value with a reference.

- [ ] macOS: Keychain Services API (`security` CLI or native Go binding)
- [ ] Linux: Secret Service API (via `libsecret` / D-Bus)
- [ ] Opt-in: `credentials.use_keychain: true` in config (default off to avoid breaking existing setups)
- [ ] Settings screen always uses keychain when available
- [ ] First-run wizard offers keychain storage by default
- [ ] Fallback to plaintext YAML if keychain unavailable or declined

---

## Developer Integration (MCP Setup)

The MCP server lets Claude Code, Cursor, Codex, and other AI tools read and manage email without opening the TUI. Setting it up requires a few lines of config that vary by tool. The README will include a ready-to-paste prompt for each supported tool so users can enable their AI assistant in under a minute.

### README prompts (planned)

Each prompt below instructs the AI tool to register the MCP server. Users copy the prompt, run it in the relevant tool, and the server is live.

- [ ] **Claude Code** — prompt to add `cmd/mcp-server` to `~/.claude/claude.json` MCP config
- [ ] **Cursor** — prompt to add the server to Cursor's MCP settings JSON
- [ ] **GitHub Copilot / VS Code** (when MCP support lands) — equivalent config snippet
- [ ] **Generic** — prompt that explains how to run `./bin/mcp-server` and wire it into any MCP-compatible client

Example (Claude Code):
```
Add a local MCP server called "mail" that runs this command:
/path/to/mail-processor/bin/mcp-server -config /path/to/proton.yaml
```

### README goals
- [ ] One-paragraph "what this is" intro
- [ ] Quick-start: build → configure → run (under 5 commands)
- [ ] MCP setup section with copy-paste prompts per tool
- [ ] Screenshot / GIF of the TUI
- [ ] Key bindings reference table
- [ ] Link to VISION.md and ARCHITECTURE.md for deeper context

---

## Theming

- [ ] App-level theme system (configurable in `proton.yaml`)
- [ ] Inherit terminal color profile
- [ ] Dark theme (current hardcoded styles are dark; no light theme)
