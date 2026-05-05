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
- [x] Archive (`e` key — moves to an explicit archive folder; provider-aware fallback only)
- [x] Deletion confirmation prompt
- [x] Attachment support (download + attach when composing, with terminal-style path completion)
- [x] Markdown compose → HTML send (multipart HTML + plain)
- [x] Full-screen email preview (`z`)
- [x] Horizontal Timeline reading slice (right arrow / `]` preview/focus, left arrow / `[` list/fold/folders, `U` mark unread)
- [x] Text selection (mouse mode + vim visual mode + clipboard)
- [x] Mouse navigation (cell-motion click and wheel controls for tabs, lists, sidebars, and previews)
- [x] AI classification via Ollama
- [x] Chat panel (ask questions about your inbox)
- [x] Semantic search (natural-language queries via local embeddings)
- [x] Search (in-folder, full-text FTS5, cross-folder, IMAP fallback, saved searches)
- [x] MCP server (read/search/classify tools for Claude Code)
- [x] SSH app mode (`cmd/herald-ssh-server` via charmbracelet/wish)
- [x] Unified CLI subcommands (`herald mcp`, `herald ssh`) with legacy wrapper binaries preserved
- [x] Inline image placeholders (text labels with AI vision descriptions when available)
- [x] Vendor presets (Gmail, Outlook, Fastmail, iCloud — one-line config)
- [x] Background new-email polling
- [x] Unsubscribe from mailing-list emails via `List-Unsubscribe` (`u` in email preview when available)
- [x] Incremental IMAP sync (UIDNEXT-based, instant on no new mail)
- [x] Progressive startup sync UX that visibly refreshes rows and explains when the app is showing a current cache snapshot while live IMAP work continues
- [x] Stream-first folder sync with latest-wins generation invalidation so stale folder loads do not repaint the visible mailbox
- [x] Microbatched Timeline refresh during IMAP sync (`100` changes or `500ms`) so the UI flows without jittering
- [x] Background cache reconciliation (valid-ID ground truth, stale entries removed)
- [x] Cache hygiene invalidation for legacy/incomplete rows with no server UID
- [x] Config-specific SQLite cache paths persisted in YAML so separate account configs do not share one working-directory database
- [x] IMAP IDLE (real push; currently polling only)
- [x] Email preview in Cleanup tab (open individual email at 50%, panels shrink to 25%)
- [x] Hide Future Mail sender rule (`h` key; auto-moves future emails to a local folder)
- [x] Custom classification prompts (user-defined categories + data extraction)
- [x] Classification actions (notify, command, webhook, move, archive, delete)
- [ ] Classification action: flag on match (set IMAP `\Flagged`)
- [x] Auto-cleanup rules (per-sender delete/archive older than N days)
- [ ] Multi-account support
- [x] Chat tool calling (Ollama tool API + MCP tools in-process)
- [x] Filtered timeline from chat results
- [x] Multiple AI backends (Claude, OpenAI-compatible)
- [x] Compose AI assistant baseline (rewrite, tone/length adjustments, subject suggestion, accept into draft)
- [x] Quick replies (canned + AI-generated contextual options)
- [x] Contact book
- [x] First-run setup wizard (detected when no config exists; account type, credentials, AI config steps)
- [x] Settings panel accessible via `S` key (saves to `~/.herald/conf.yaml`)
- [ ] Keychain integration (passwords stored in OS keychain, not plaintext YAML)
- [x] README with MCP setup prompts for Claude / Cursor / Codex
- [x] Daemon server (`herald serve`, Ollama-style)
- [x] Source installs use the canonical Go module path and `cmd/herald` package so `go install github.com/herald-email/herald-mail-app/cmd/herald@latest` creates a `herald` binary.
- [x] Email rendering package (`internal/render` — independent, testable component)
- [x] Link tracker sanitization (strip UTM, fbclid, mc_cid, etc.)
- [ ] Link display modes (full URL, title-only clickable, sanitized)
- [ ] Native app client (Phase 3)

---

## Architecture: Daemon / Client Split

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full technical reference.

The app is designed in three phases so that each phase's work is additive and nothing needs to be rewritten.

### Phase 1 — current
Single process. The Bubble Tea model talks only to a `Backend` interface, never to IMAP or SQLite directly. All processing (IMAP, cache, AI) lives behind that interface. The discipline means the UI can be swapped or multiplied without touching the backend.

### Phase 2 — daemon server (Ollama-style)
The backend becomes a standalone daemon: `herald serve`. It runs as a persistent background process, holds the IMAP connection, owns the SQLite cache, and exposes a local HTTP + WebSocket API on a configurable port (default `localhost:7272`). Clients connect to it — they do not touch IMAP or the database directly.

CLI mirrors Ollama's UX:
```
herald serve          # start daemon (foreground; launchd/systemd for autostart)
herald status         # show running daemon info
herald stop           # graceful shutdown
herald sync           # trigger incremental IMAP sync
```

All existing client modes (TUI, SSH, MCP) become thin clients of the daemon. A `RemoteBackend` struct implements the same `Backend` interface over HTTP/WebSocket — TUI code is unchanged, only the backend wiring differs.

### Phase 3 — native app
A native desktop client (macOS-first via SwiftUI; cross-platform alternative via Wails) connects to the same daemon API. The server and client are distributed separately, just like Ollama and its frontends. Multiple clients can run simultaneously against one daemon with no data races.

---

## UI Layout

The TUI uses a fixed tab bar at the top, a collapsible folder sidebar on the left, and a main content area whose layout changes per tab.

- [x] Mouse navigation supports top tabs, sidebars, Timeline/Cleanup rows, and preview wheel scrolling while preserving keyboard parity
- [x] Keyboard layouts with physical-key reporting trigger Herald-owned shortcuts from their QWERTY positions in browse contexts, with Cyrillic and direct Japanese kana fallback aliases when terminals do not report `BaseCode`

### Tabs (top-level navigation)
Keyboard (`F1`-`F3` as the primary visible shortcuts, with browse-context number aliases) and mouse clickable. Compose is a transient writing screen launched from Timeline, not a top-level tab.

- [x] `F1` — Timeline: chronological email list with body preview split
- [x] `F2` — Cleanup: sender/domain grouping for bulk deletion
- [x] `F3` — Contacts: contact book with list+detail panels, keyword and semantic search, LLM enrichment

### Timeline View

The primary reading interface. Shows emails sorted newest-first, grouped by thread when multiple messages share the same subject. Selecting a row opens a body preview split on the right.

- [x] Full-width thread list sorted by date
- [x] Thread grouping across participants (fold/unfold inline with `Enter`, reply rows marked visibly)
- [x] Sender-cell disclosure markers make collapsed and expanded Timeline threads recognizable before opening them
- [x] Body preview split (right panel, auto-updates on navigation)
- [x] Horizontal reading movement: right arrow / `]` opens preview, then moves into it; left arrow moves preview focus back to the list, then folds threads or closes preview and focuses folders
- [x] Intentional unread affordance: `U` marks the current Timeline message unread after inspection
- [x] Full-screen preview (`z`)
- [x] Actions: delete, archive, reply, forward
- [x] Bulk selection with `Space` for Timeline delete/archive, including collapsed-thread selection
- [x] Star / pin important threads to top
- [x] Gmail/IMAP drafts are marked directly in Timeline rows and collapsed thread rows, including reply drafts, and `E` opens the draft in Compose for editing
- [x] Read-only virtual `All Mail only` inspector backed by live IMAP folder membership rather than cache guesses
- [x] Reading-first Timeline rows hide spreadsheet-only size/attachment columns, show attachments in the subject cell, use local human dates, and keep Sender/Subject dominant at `80x24`, `120x40`, and `220x50`
- [ ] `All Mail only` means mail present in `All Mail` with no other real folder assignment; `Sent`, `Archive`, and nested folders are excluded rather than treated as acceptable matches
- [ ] Unified list highlight language shared with the folder sidebar and other list-like panels
- [ ] Active border shown only on the currently focused Timeline region (sidebar, list, or preview)
- [ ] Split Timeline and preview panels keep aligned heights at common sizes including `80x24`
- [ ] Active folder visible bundle settles together within 2-5 seconds: rows, live counts, folder title, and folder tree presence
- [ ] Hydrated cache rows never overwrite authoritative live IMAP folder counts shown in the Timeline chrome

### Status Bar

A single persistent line at the bottom of the screen. Its content changes based on which panel is focused.

- [x] Active folder breadcrumb
- [x] Folder counts (unread / total)
- [x] Selection state (N senders selected, N messages selected)
- [x] Mode indicator (Domain mode, Sender mode, Logs ON)
- [x] Deletion progress (Deleting 3/5…)
- [x] Key hints (changes per panel)
- [x] Sync countdown (↻ 42s to next poll, ↻ live when IDLE active)
- [x] Global AI status chip that stays visible when AI is configured and summarizes the effective AI state (`idle`, `embedding`, `quick reply`, `semantic search`, `chat`, `deferred`, or `unavailable`)
- [x] Compose-safe command layer: `F1/F2/F3` are the primary advertised tab shortcuts, secondary `Alt+1/2/3` aliases remain supported, and `Alt+L`, `Alt+C`, `Alt+F`, and `Alt+R` keep global actions reachable while Compose text fields accept plain letters, digits, and `q`
- [x] Timeline key hints advertise `Tab` / `Shift+Tab` panel switching whenever the bottom bar has room for navigation help
- [x] Context-sensitive shortcut help overlay opens with `?`, lists every relevant key for the current tab, pane, overlay, and Compose mode in a compact centered modal over the current view, and keeps semantic search available through `/` with a `? query` prefix
- [ ] Key hints always reflect normalized visible focus rather than stale internal focus state
- [ ] Selection and mode fragments stay scoped to the active tab and never leak across tabs
- [ ] Hint copy uses one consistent verb set (`open`, `close`, `preview`, `full-screen`, `back`)
- [ ] Top sync strip is informational only, reports active-folder unsettled work honestly, and disappears once the active folder bundle is settled

### Multi-Folder Sidebar

- [x] Collapsible left panel (toggled with `f`)
- [x] Real IMAP folders synced from server
- [x] Unread / total counts per folder
- [x] Keyboard navigation (j/k, Enter to switch folder)
- [x] Auto-hides with a hint when terminal is too narrow
- [x] Current folder remains visibly selected even when the sidebar is not focused
- [x] Folder tree appears promptly during startup and does not collapse to a partial list while the active folder is still loading

### Chat Panel

The chat panel is a right-side slide-out (`c` key) that lets you have a conversation with your inbox using a local Ollama model. It currently supports Q&A over email content. The vision is to evolve it into a full agentic assistant that can search, summarise, compose, and manage email through natural conversation.

- [x] Slide-out panel (`c` key)
- [x] Conversation history (multiple turns)
- [x] Markdown rendering of assistant responses (glamour)
- [x] Context: currently open email available to the model
- [x] Tool calling via Ollama's native tool API
- [x] In-process tools (search, sender stats, threads — not the full MCP surface yet)
- [x] Filtered timeline: chat result sets pushed into Timeline as a live view
- [x] Context: active folder and selection state passed to model
- [ ] `draft_reply` / `send_email` from within chat (currently MCP-only)
- [x] Multiple AI backends (Ollama, Claude, OpenAI-compatible)

#### Tool calling

The chat uses Ollama's `tools` field in `/api/chat` to invoke tools directly in-process. The model decides which tools to call; the app executes them and feeds results back until the model produces a final reply. Current chat tools: `search_emails`, `list_emails_by_sender`, `get_thread`, `get_sender_stats`.

- [ ] Expand chat tools to mirror the full MCP surface (read body, summarise, reply, manage, classify)

#### Filtered timeline

When the chat returns a set of emails (from a search, date filter, or semantic query), those results are pushed into the Timeline tab as a live filtered view. The user can browse and act on them without leaving the chat flow. `Esc` or "show all" restores the full timeline.

#### Multiple AI backends

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
- [x] `classify_folder` MCP tool (batch, with progress)
- [x] Auto-classify new emails as they arrive (background, rate-limited)
- [x] Reanalyse / override existing tags
- [x] Local-AI work scheduler with bounded concurrency and interactive-before-background priority
- [x] Background embedding and enrichment coalescing so repeated Timeline loads do not create duplicate local-AI bursts
- [x] Degraded AI UX that surfaces concise `AI unavailable` / `deferred` states instead of noisy repeated failures
- [x] Local Ollama overload handling that fails open and preserves overall UI/network responsiveness

### Custom Classification Prompts

The built-in classification prompt assigns one of six fixed categories (`sub`, `news`, `imp`, `txn`, `soc`, `spam`). Custom prompts let users define their own categories and extraction logic tailored to their workflow — e.g. extracting order numbers from receipts, flagging emails from specific clients, or categorizing by project.

- [x] `classification_prompts` section in `~/.herald/conf.yaml` — list of named prompt definitions
- [x] Each prompt specifies: name, system prompt text, list of valid output categories, and an optional data extraction instruction (e.g. "extract the tracking number")
- [x] Default built-in prompt used when no custom prompts are configured (current behaviour preserved)
- [x] Multiple prompts can run on the same email (e.g. one for category, one for data extraction)
- [x] `custom_categories` table in SQLite storing prompt name + category + extracted data per email
- [x] TUI displays custom categories alongside the built-in tag column
- [x] MCP tools: `list_classification_prompts`, `classify_email_custom` (run a named prompt on one email)
- [x] Prompt overlay explains that prompts are reusable AI instructions, shows saved prompts in the same screen, and tells the user where prompt results will appear

### Classification Actions

When an email matches a category, the system can trigger an action automatically. Actions turn classification from passive tagging into an active assistant — sending OS notifications for important mail, running shell commands with extracted data, or auto-filing emails into folders. Actions execute in the background daemon (Phase 2) so they fire even when the TUI is not running.

- [x] Sender / domain / category triggers stored in SQLite (`email_rules` table) — rule matches on sender address, domain, or AI category
- [x] Action type: `notify` — send an OS-level notification (macOS Notification Center / `notify-send` on Linux) with sender, subject, and optional extracted data
- [x] Action type: `command` — run a shell command with template variables (`{{.Sender}}`, `{{.Subject}}`, `{{.Category}}`, `{{.MessageID}}`)
- [x] Action type: `webhook` — POST a JSON payload to a URL (for Slack, Discord, Home Assistant, etc.)
- [x] Action type: `move` — auto-move the email to a specified IMAP folder
- [x] Action type: `archive` — archive the matching email
- [x] Action type: `delete` — delete the matching email
- [x] Action execution logged to SQLite (`rule_action_log` table) with timestamp and result
- [x] Rule editor TUI form (`W` key in Cleanup tab — add/edit rules interactively)
- [x] `classification_actions` section in `~/.herald/conf.yaml` — declarative config format (current: DB-only)
- [x] Auto-classify new emails as they arrive to trigger rules in real time
- [x] Dry-run mode: `--dry-run` flag logs what actions would fire without executing them
- [ ] Rule overlay explains that `W` creates future-mail automations, shows saved rules in the same screen, and tells the user where the action results surface

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
- [ ] Stable sender/domain selection keyed by logical identity rather than row index
- [ ] Selection checkmarks and `N selected` status always agree across refreshes, re-sorts, and resizes
- [ ] Cleanup summary columns simplified to `✓`, `Sender/Domain`, `Count`, and `Date Range`
- [ ] Cleanup summary resizes responsively at `220x50`, `120x40`, `80x24`, and `50x15` without losing the selection column
- [ ] Wide Cleanup layouts expand the date-range column enough to show a more specific first/last date range instead of the narrow fallback format

### Email preview in Cleanup

The Cleanup tab has two panels side by side: the sender summary (left) and the email list for the selected sender (right). Today the email list is read-only. The goal is to make individual emails fully actionable from within Cleanup — open, read, reply, unsubscribe — without switching to the Timeline tab.

**Layout when an email is open:**
- Folder sidebar hides completely (same as full-screen mode in Timeline)
- Summary panel (sender list) shrinks to 25% of the terminal width
- Email list panel (details) shrinks to 25%
- Email preview panel opens at 50% on the right

This gives enough room to read the email while keeping both panels visible as context. `Esc` closes the preview and restores the normal two-panel layout.

- [x] `Tab` cycles focus between the summary panel and the email list panel
- [x] `Enter` on a row in the email list opens the email preview at 50% width
- [x] Folder sidebar hides when preview is open; restores on `Esc`
- [x] Summary and email list panels each shrink to 25% when preview is open
- [x] Preview panel supports the same scroll controls as Timeline (`j`/`k`, `PgUp`/`PgDn`)
- [ ] `r` / `R` — reply from within Cleanup preview (opens Compose, pre-filled)
- [x] `u` — unsubscribe from within an open email preview when the message exposes `List-Unsubscribe`
- [x] `h` — hide future mail from the open email's sender
- [x] `D` — delete the open email from within the preview
- [x] `e` — archive the open email from within the preview
- [x] `z` — expand to full-screen (same as Timeline full-screen mode)
- [x] `Esc` — close preview, restore two-panel Cleanup layout

### Unsubscribe

Unsubscribe and sender-hiding actions should be visible from the open email preview itself so the user does not have to remember hidden keybindings. `u` acts on the current email's mailing-list headers, while `h` acts on the sender and keeps future mail out of the inbox without pretending to be a real unsubscribe.

- [x] Preview metadata shows explicit `Tags:` and `Actions:` rows so list/sender actions are visible in context
- [x] `u` unsubscribes the currently open Timeline or Cleanup preview email when it exposes `List-Unsubscribe`
- [x] `h` hides future mail from the currently open email's sender by moving new mail to `Disabled Subscriptions`
- [x] Cleanup sender summary exposes `h` but not `u`, because true unsubscribe depends on message-level headers
- [x] Timeline preview and Cleanup preview share the same `u` / `h` semantics and user-facing copy
- [x] `u` performs RFC 8058 one-click POST when `List-Unsubscribe-Post` is available
- [x] `u` falls back to `List-Unsubscribe` mailto handling when the message only exposes an email-action target
- [x] `u` falls back to opening a `List-Unsubscribe` browser URL for HTTP links
- [x] Track whether emails keep arriving after unsubscribe; notify / prompt if they do
- [ ] Batch unsubscribe flow: present list of detected subscriptions, select, choose mode, execute

### Auto-Cleanup Rules

Rules let the app automatically act on email from known senders — delete newsletters older than 30 days, archive promotional email weekly, etc. Rules are defined per-sender or per-domain and stored in SQLite.

- [x] Per-sender / per-domain rules (action + older-than-days condition)
- [x] Rule storage in SQLite (`cleanup_rules` table)
- [x] Manual rule execution (`run_cleanup_rules` trigger)
- [x] Scheduled execution (configurable interval in `~/.herald/conf.yaml`; TUI-only — daemon runs rules on-demand via `/v1/cleanup-rules/run`)
- [x] TUI rule manager (list, add, remove)
- [x] MCP tools: `list_cleanup_rules`, `create_cleanup_rule`, `run_cleanup_rules`
- [ ] Cleanup rule manager explains what manual vs scheduled cleanup does, where saved rules live, and how run results become visible

---

## Compose and Reply

Write in Markdown, deliver as properly formatted HTML email. Compose is a transient full-screen editor launched from Timeline with `C`, contextual reply/forward/draft actions, or quick replies; `Esc` returns to the screen that initiated it after local Compose transient state is dismissed.

- [x] Markdown editor (textarea)
- [x] Timeline `C` opens a blank Compose screen for a new message
- [x] Live Markdown preview (`Ctrl+P`)
- [x] Send as multipart HTML + plain-text via SMTP
- [x] Reply (`R` key — pre-fills To, Re: subject, quotes original)
- [x] Forward (`F` key — pre-fills Fwd: subject, forwarding header, body quote)
- [x] Attachment support: attach files (`Ctrl+A`), attach list shown in compose
- [x] Send with attachments (`multipart/mixed`)
- [x] Plain draft entry is safe: digits, letters, and `q` type into the focused Compose field; global tab/log/chat/sidebar/refresh commands use function keys or Alt chords while composing
- [x] Preserved HTML replies and forwards: Compose edits only the user's top note, shows the original message as read-only context while composing replies and forwards, and sends the original HTML quote inline with selectable Safe/Fidelity/Privacy preservation
- [x] Forwarded attachments are included by default and individually toggleable before sending
- [x] Timeline drafts open as editable Compose messages with recipients, subject, and body restored from the saved draft; sending deletes the source draft only after SMTP success
- [x] Timeline drafts can be sent directly with `Ctrl+S` from the draft row, draft preview, or collapsed thread draft without switching to Compose
- [x] Timeline reply drafts are labelled as both draft and reply, and their preview shows the visible thread context before the draft body
- [x] Draft autosave replacement saves the new draft before deleting the previously saved draft, so a failed save cannot discard the only copy
- [ ] Browser preview (open rendered HTML in default browser before sending)
- [x] Inline images (paste / drag file path → base64 `multipart/related`)
- [x] `send_email` MCP tool
- [x] `reply_to_email` MCP tool
- [x] `forward_email` MCP tool
- [x] `draft_reply` MCP tool (LLM drafts reply from natural-language instructions)
- [x] `save_draft` / `send_draft` / `list_drafts` MCP tools

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
- [x] "No thanks" — polite decline
- [x] "Thank you for reaching out"
- [x] "Thank you, [Name]" — first name extracted from the From header
- [x] "Copy that" — simple acknowledgement
- [x] "I'll get back to you" — defer

AI-generated contextual replies (shown when body is loaded):
- [x] Model reads the email body and generates 3 short reply options relevant to the content
- [x] `Ctrl+Q` opens the quick-reply picker; arrow keys to select, `Enter` to open in Compose, `Esc` to cancel
- [x] Works in both split-preview and full-screen view

---

## HTML Rendering (Received Emails)

Received emails are converted from HTML to Markdown for display in the terminal. The conversion is best-effort — complex layouts simplify gracefully.

- [x] HTML → Markdown conversion for body preview
- [x] Shared Markdown-aware rendering across Timeline, Cleanup, Contacts, and full-screen received-email previews
- [x] Inline image text placeholders (`[image: type  size]` in split view, `[Image: AI description]` when vision model available)
- [x] Remote HTML image links render as readable OSC 8 links without auto-fetching remote image bytes
- [x] Full-screen inline image viewing uses bounded iTerm2 or Kitty rendering when supported, or localhost OSC 8 links for local MIME image bytes in local TUI sessions
- [ ] AI vision image descriptions — use a vision-capable model (e.g. gemma3:4b, gpt-4o, claude) to generate a one-line description for each inline image on demand. Show as `[Image: A promotional banner showing...]` instead of raw MIME type. Generate lazily when email is opened, cache in SQLite. Requires `HasVisionModel()` on the classifier.
- [x] Kitty graphics protocol support with Ghostty autodetection and `-image-protocol` override for non-iTerm2 terminals

---

## Email Rendering & Link Processing

Email body rendering and link handling live in `internal/render`, a standalone package with no TUI dependency. This makes it testable independently and reusable across all surfaces: TUI, MCP server, daemon API, and SSH mode. The package handles text wrapping (ANSI-aware), URL linkification (OSC 8 terminal hyperlinks), and link sanitization.

### Rendering component (`internal/render`)
- [x] `WrapText` / `WrapLines` — ANSI-aware text wrapping that correctly skips escape sequences when counting visible width
- [x] `StripInvisibleChars` — removes zero-width joiners, BOM, and invisible spacers that HTML emails embed
- [x] `LinkifyURLs` / `LinkifyWrappedLines` — converts raw URLs to OSC 8 clickable terminal hyperlinks with shortened labels
- [x] `ShortenURL` — produces human-readable labels like `example.com/path…` for long URLs
- [x] `Truncate`, `SanitizeText`, `CalculateTextWidth` — text utilities shared across all rendering surfaces
- [x] Full test suite (`internal/render/*_test.go`)

### Link tracker sanitization
Modern marketing emails embed tracking parameters in every link — UTM tags, Mailchimp click IDs, Facebook/Google/HubSpot trackers. These make URLs unreadable and leak privacy. The sanitizer strips known tracker parameters while preserving the actual destination.

- [x] `StripTrackers` — removes known tracking query parameters (UTM, fbclid, mc_cid, gclid, HubSpot, LinkedIn, etc.) from a single URL
- [x] `StripTrackersFromText` — applies tracker stripping to all URLs found in a text block
- [x] Case-insensitive parameter matching
- [x] Preserves non-tracker query parameters, path, fragment
- [ ] `L` key in email preview toggles link sanitization on/off (per-session)
- [ ] `sanitize_links` config option in `~/.herald/conf.yaml` (default: on)
- [ ] Link sanitization applied in MCP `get_email_body` tool output
- [ ] Link sanitization applied in daemon API `/v1/email/{id}/body` response

### Link display modes
Links in email bodies can be shown in different ways depending on user preference. A toggle cycles through modes so users can see clean text by default but reveal the full URL when needed.

- [x] Mode: title-only clickable — shortened label (`example.com/path…`) as visible text, full URL in OSC 8 hyperlink (current default)
- [ ] Mode: full URL visible — raw URL shown inline, no shortening
- [ ] Mode: sanitized clickable — tracker-stripped URL in both label and hyperlink target
- [ ] `ctrl+l` in email preview cycles through link display modes
- [ ] Current mode shown in status bar when preview is focused
- [ ] Mode preference saved in `~/.herald/conf.yaml`

---

## Search

Search is layered: fast local metadata search first, full-text body search next, IMAP server-side search as a fallback for emails not yet cached.

### In-folder search (local, fast)
- [x] `/` key opens search bar in Timeline and Cleanup tabs
- [x] SQLite `LIKE` search on sender + subject
- [x] Timeline `/` search merges keyword and semantic results when embeddings are available
- [x] Results replace current list view; `Esc` clears
- [x] Timeline search debounces local execution and ignores stale result responses while typing
- [x] `Enter` from Timeline search moves into a result-navigation mode
- [x] `Esc` unwinds Timeline search in steps: preview → results → input → original timeline state
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
- [ ] Timeline search no longer uses `Ctrl+S`; saved searches need a clearer dedicated entry point
- [ ] Appear as virtual folders in the sidebar

### Semantic search
- [x] `?` prefix in search bar triggers semantic mode
- [x] Local embeddings via Ollama (`nomic-embed-text-v2-moe` default)
- [x] Vectors stored in SQLite (`email_embeddings` table)
- [x] Cosine similarity ranking
- [x] `semantic_search_emails` MCP tool
- [x] Similarity score badge (`87%`) per result row
- [x] Embeddings are invalidated automatically when the configured embedding model changes
- [x] Semantic search shows an explicit unavailable or deferred message when embeddings cannot run
- [x] Hybrid ranking (keyword + semantic merged)
- [x] Semantic expansion is bounded by a configured similarity threshold and result cap
- [ ] "Why this result?" hint (matched excerpt)

---

## MCP Integration

The MCP server exposes email operations as tools, enabling Claude Code and other AI agents to read, search, classify, and eventually manage email without opening the TUI. It reads directly from the configured SQLite cache path and shares state with the TUI via SQLite WAL mode. The primary entrypoint is `herald mcp`; `herald-mcp-server` remains available as a compatibility wrapper for existing MCP configs and scripts.

### Entry points

These commands make the MCP surface discoverable from the main `herald --help` path while avoiding a breaking migration.

- [x] `herald mcp` starts the stdio MCP server.
- [x] `herald mcp --demo` serves deterministic demo mailbox tools without loading private config.
- [x] `herald-mcp-server` delegates to the same implementation for at least one compatibility release.

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
| `summarise_email` | Generate a summary via local AI |
| `list_rules` | List all enabled automation rules |
| `add_rule` | Create a new automation rule |
| `run_rules` | Dry-run: show which cached emails match rules in a folder |
| `list_contacts` | List contacts sorted by recency |
| `search_contacts` | Keyword search on name/email/company/topics |
| `semantic_search_contacts` | Natural-language contact search |
| `get_contact` | Full contact profile + recent emails |
| `list_folders` | All folders present in the local cache |
| `get_server_info` | Herald config and daemon status |
| `mark_read` | Mark an email as read (via daemon) |
| `mark_unread` | Mark an email as unread (via daemon) |
| `delete_email` | Delete an email (via daemon) |
| `archive_email` | Archive an email (via daemon) |
| `move_email` | Move an email to a folder (via daemon) |
| `sync_folder` | Trigger IMAP sync for a folder (via daemon) |
| `get_thread` | All emails in a thread by subject |
| `send_email` | Send email via SMTP (via daemon) |
| `summarise_thread` | One-paragraph thread summary via AI |
| `extract_action_items` | Extract tasks from an email body via AI |
| `draft_reply` | LLM-drafted reply in professional or casual tone |

### Additional tools

- [x] `list_attachments` — attachment metadata without downloading
- [x] `get_attachment` — download a specific attachment
- [x] `classify_folder` — batch classify with progress
- [x] `reply_to_email` — reply with pre-filled headers
- [x] `forward_email` — forward with covering note
- [x] `save_draft` / `list_drafts` / `send_draft`
- [x] `delete_thread` / `bulk_delete`
- [x] `archive_thread` / `archive_sender`
- [x] `bulk_move`
- [x] `create_folder` / `rename_folder` / `delete_folder`
- [x] `unsubscribe_sender` — hard-unsubscribe via List-Unsubscribe header
- [x] `soft_unsubscribe_sender` — auto-move future emails to a folder
- [x] `sync_all_folders` / `get_sync_status`

### TUI ↔ MCP shared state

Both processes read and write the same configured SQLite cache database via WAL mode. Classifications set via `classify_email` appear immediately in the TUI's Tag column. Emails deleted via `delete_email` disappear from the TUI on the next render.

### Simultaneous TUI + MCP operation

The TUI and MCP server are safely runnable at the same time:

- [x] SQLite WAL mode (readers never block writers)
- [x] IMAP connection isolation (each process holds its own connection)
- [x] Short atomic writes (no long-lived transactions)
- [x] Daemon architecture (Phase 2): both become clients of a single daemon that serialises all IMAP writes

---

## Automatic New-Email Sync

New emails are detected without a full restart. The current implementation uses background polling; IMAP IDLE is the target for true push behaviour.

### IMAP IDLE (target)
IMAP IDLE (`RFC 2177`) lets the server push `EXISTS` and `EXPUNGE` notifications to the client without polling. It is more efficient and eliminates the delay between arrival and display.

- [ ] Dedicated IDLE connection alongside the command connection (currently reuses the command connection via `IdleWithFallback`)
- [x] `EXISTS` → trigger incremental fetch, prepend rows to timeline (`MailboxUpdate` event → `PollForNewEmails`)
- [ ] `EXPUNGE` → remove matching row from cache and timeline immediately
- [x] 29-minute keepalive (handled by `go-imap-idle` `IdleWithFallback`)

### Background polling (current)
- [x] Configurable poll interval (`sync.interval` in `~/.herald/conf.yaml`, default 60s)
- [x] Polling detects new emails via UIDNEXT comparison
- [x] New emails cached and prepended to timeline
- [x] Sync countdown shown in status bar (↻ 42s)
- [ ] Per-folder poll frequency (active folder more often; background folders less)

---

## Multi-Account Support

The app currently supports one IMAP account per config file. Multi-account support will allow managing several inboxes (e.g. personal + work) in a single session.

- [ ] `accounts:` list in `~/.herald/conf.yaml` (current single-account format still works)
- [ ] Per-account IMAP connection, cache file, and folder tree
- [ ] Folder sidebar grouped under account headers
- [ ] Status bar shows active account name
- [ ] Compose "From" field lets user pick sending account
- [ ] Unified Timeline view across accounts (opt-in)
- [x] Gmail OAuth is experimental first-run onboarding, hidden unless Herald starts with `-experimental`
- [ ] Outlook OAuth
- [x] Vendor presets: `protonmail`, `gmail`, `outlook`, `fastmail`, `icloud`

---

## Contact Book

Contacts are derived from To/From/CC headers seen in sent and received mail — no import required. They power name completion in Compose and will feed into chat context.

- [x] `contacts` table in SQLite (built from To/CC/BCC/From headers during IMAP sync)
- [x] Name, email, company, topics, first/last-seen, email/sent counts per contact
- [x] LLM enrichment via Ollama: extracts company name and discussed topics from email subjects (`e` key)
- [x] Semantic contact embeddings for natural-language search
- [x] Tab 4 — Contacts TUI: two-panel list+detail, `/` keyword search, `/` then `? query` semantic search
- [x] Apple Contacts import via AppleScript at startup (darwin only, read-only name merge)
- [x] `list_contacts` / `search_contacts` / `semantic_search_contacts` / `get_contact` MCP tools
- [x] Common AI/model-related contact enrichment failures are deduplicated per batch and surfaced in the Contacts status area
- [ ] All contact enrichment failure modes are deduplicated and surfaced without log spam
- [x] Autocomplete in Compose `To`/`CC`/`BCC` fields
- [ ] CardDAV sync (config stubs in place; implementation deferred)

---

## SSH App Mode

`charmbracelet/wish` serves the full TUI over SSH on port 2222. Each SSH session gets its own `LocalBackend` (independent IMAP connection). The primary entrypoint is `herald ssh`; `herald-ssh-server` remains available as a compatibility wrapper for existing deployment scripts.

- [x] `herald ssh` subcommand
- [x] `cmd/herald-ssh-server` compatibility wrapper binary
- [x] Each session: independent LocalBackend + IMAP connection
- [ ] In Phase 2: each session connects to the shared daemon instead

---

## Forward and Deletion UX

- [x] `F` key in Timeline opens Compose pre-filled for forward (Fwd: subject, quoted body)
- [x] `R` key in Timeline opens Compose pre-filled for reply (Re: subject, quoted body, To pre-filled)
- [x] Reply/forward Compose shows a preserved-content summary with HTML, inline image, attachment, and preservation-mode status
- [x] Reply/forward Compose separates the editable response from a read-only original-message preview so users can keep source context visible while writing
- [x] `D` in Timeline deletes the highlighted email (single message)
- [x] `D` on a collapsed `[N]` thread prompts to delete all N emails
- [x] `Space` in Timeline selects messages for bulk delete/archive; collapsed thread rows select the represented thread
- [x] `D` / `e` in Timeline act on selected messages when selection exists, with confirmation copy showing selected counts
- [x] `D` on a draft uses discard-draft confirmation copy, while `E` is advertised as the explicit edit-draft command and `Ctrl+S` as the explicit send-draft command
- [x] `e` archives the highlighted email or sender
- [x] Inline confirmation prompt (`y` to confirm, `Esc` to cancel)

---

## Attachment Support

Attachments appear in previews and compose flows, so this section tracks both raw transport support and the user-facing interaction quality that makes them trustworthy.

- [x] Attachments detected from `BodyStructure` during sync
- [x] Attachment list shown in email preview (filename, MIME type, size)
- [x] Save attachment to disk (prompted path, default `~/Downloads/<filename>`)
- [x] Attach files when composing (`Ctrl+A`)
- [x] Terminal-style path autocomplete in the Compose attachment prompt
- [x] Multiple attachments; each listed below the body
- [x] Sent as `multipart/mixed`
- [x] Attachment downloads never silently overwrite existing local files; interactive saves suggest the next available filename
- [ ] Preview supports navigating between multiple attachments and saving the currently selected one
- [ ] Attachment selection uses the same focus/highlight language as other navigable lists
- [ ] File size warning for attachments over 10 MB

---

## Text Selection

Bubble Tea's alt-screen captures all input, so the terminal's native mouse selection is disabled. Two mechanisms restore copy-ability.

- [x] `m` toggles mouse-selection mode (releases mouse capture; status bar indicator)
- [x] `m` restores TUI mouse capture after temporary terminal-native text selection
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
- [x] Inline images stay within the viewport in full-screen mode and degrade to safe links/placeholders when graphics are unavailable
- [x] `z` or `Esc` exits and restores split layout

---

## Settings / Onboarding Screen

First-run experience and ongoing configuration should not require the user to edit a YAML file. A TUI settings screen lets users configure accounts, server details, AI, and sync preferences interactively. The YAML file remains the source of truth on disk — the settings screen reads and writes it.

### First-run wizard
- [x] Detected on startup when the config file is missing or empty / whitespace-only
- [x] Herald-styled setup shell with recommended, supported, and experimental account messaging and the same minimum-size guard used by the main TUI
- [x] Step 1 — Account type: recommended `Gmail (IMAP + App Password)`, supported `Standard IMAP` plus IMAP presets for ProtonMail Bridge, Fastmail, iCloud, and Outlook; `Gmail OAuth (Experimental)` appears only when launched with `-experimental`
- [x] Step 2 — Credentials: Gmail IMAP uses email + app password with prefilled Gmail defaults and an optional advanced-server toggle; Standard IMAP and IMAP presets keep editable server fields
- [x] Gmail setup copy links directly to Google docs for IMAP access, third-party client setup, and App Password generation
- [x] Gmail OAuth remains available as an experimental browser-based path behind `-experimental`; Homebrew/release binaries include OAuth defaults, while source builds require configured Google OAuth credentials
- [x] Step 3 — AI: enter Ollama host (default `localhost:11434`), pick model from detected list, pick embedding model; skip if Ollama not running
- [ ] Step 4 — Sync: poll interval, IMAP IDLE toggle
- [ ] Step 5 — Test connection button; shows result inline before saving
- [x] Writes `~/.herald/conf.yaml` on finish

### In-app settings panel
- [x] Accessible from the TUI with `S` key as a compact centered overlay over the current screen; it fits at `80x24` and falls back to the minimum-size guard at `50x15`
- [x] Editable fields for ALL config sections: credentials, server, SMTP, AI, sync (basic fields only done)
- [ ] Account list for multi-account (add / remove / reorder)
- [x] Changes saved on `Ctrl+S`; no restart required for most settings
- [x] Passwords always hidden; "reveal" button toggles visibility

---

## Security / Keychain

Passwords in `~/.herald/conf.yaml` are stored in plaintext, which is acceptable for a local tool but not ideal. The keychain integration stores credentials in the OS keychain and replaces the plaintext value with a reference.

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

- [x] **Claude Code** — prompt to add `herald mcp` to `~/.claude/claude.json` MCP config
- [x] **Cursor** — prompt to add the server to Cursor's MCP settings JSON
- [x] **Windsurf** — JSON snippet for `~/.codeium/windsurf/mcp_config.json`
- [x] **Codex** — env-var CLI approach
- [ ] **GitHub Copilot / VS Code** (when MCP support lands) — equivalent config snippet
- [x] **Generic** — prompt that explains how to run `./bin/herald mcp` and wire it into any MCP-compatible client

Example (Claude Code):
```
Add a local MCP server called "mail" that runs this command:
/path/to/herald/bin/herald mcp -config ~/.herald/conf.yaml
```

### README goals
- [x] One-paragraph "what this is" intro
- [x] Quick-start: build → configure → run (under 5 commands)
- [x] MCP setup section with copy-paste prompts per tool
- [x] Screenshot / GIF of the TUI in `assets/demo/` (canonical tapes written; GIFs generated)
- [x] Key bindings reference table
- [x] Link to VISION.md and ARCHITECTURE.md for deeper context

---

## Demo Mode

Demo mode lets anyone try the full TUI without a live IMAP account. It launches with a synthetic set of emails covering all supported features — threads, attachments, classifications, HTML bodies — so every panel and key binding can be exercised immediately.

- [x] `--demo` flag on the main binary starts the app with a `DemoBackend` instead of IMAP
- [x] Shared demo fixtures seed fictional senders, categories, attachments, bodies, unsubscribe headers, contacts, and threads
- [x] Deterministic demo AI powers classification, semantic search, chat, quick replies, and contact enrichment without Ollama
- [x] `herald mcp --demo` exposes the same synthetic mailbox without loading private config or cache files
- [x] Canonical demo tapes generate 5-30 second GIFs in `assets/demo/`
- [x] `[DEMO]` indicator in the status bar so the user knows they are not connected to a real account
- [x] Creative Commons image sampler email lets users test inline image hints, full-screen rendering, and local image fallback links in demo mode
- [x] Rich HTML rendering showcase email lets users verify headings, lists, links, remote image labels, and shared preview behavior in demo mode
- [ ] Demo mode accessible from the first-run wizard ("Try without an account")

---

## Testing Infrastructure

Integration tests and headless test harnesses ensure the app works correctly at the protocol level without a live server.

- [x] IMAP mock server (`internal/testutil/imap_server.go`) — in-process IMAP server for integration tests
- [x] Integration tests against the mock server (`internal/imap/integration_test.go`)
- [ ] 360 TUI manual QA matrix covering demo, live IMAP, live Ollama, SSH, and MCP lanes
- [ ] TUI snapshot tests via `teatest` or PTY (render correctness at fixed terminal sizes)
- [ ] CI pipeline running all tests on push

---

## Theming

- [ ] App-level theme system (configurable in `~/.herald/conf.yaml`)
- [ ] Inherit terminal color profile
- [ ] Dark theme (current hardcoded styles are dark; no light theme)
