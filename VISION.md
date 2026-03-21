# Vision

This document describes the long-term direction for this project. It evolves from an inbox cleanup tool into a full-featured terminal email client.

## Implementation Order

1. Fix responsive terminal width (hardcoded values today) ✓
2. Refactor to daemon/UI split architecture ✓
3. Multi-folder sidebar (collapsible tree, counts) ✓
4. Status bar showing active folder, unread/total counts, selection state
5. Timeline/thread view + tab navigation
6. Compose and reply (after timeline)
7. AI-powered inbox classification via Ollama
8. Chat panel (talk to your emails with AI)
9. MCP server hook
10. SSH app mode (charmbracelet/wish)
11. Image rendering (iTerm2 inline images)

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

## Compose and Reply (Later Phase)

- Write in Markdown with live Bear.app-style preview (charmbracelet/glamour)
- Convert to HTML on send
- Browser preview button for checking HTML rendering before sending
- Insert images inline in compose
- Full reply and forward support

---

## HTML Rendering (Received Emails)

- Best-effort rendering of HTML emails in terminal
- charmbracelet/glamour handles the Markdown path; HTML needs a separate rendering solution

---

## Image Support

- iTerm2 inline images protocol (primary target, user is on macOS/iTerm2)
- Design to be extensible to Kitty graphics protocol for other terminals

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

---

## SSH App Mode

`charmbracelet/wish` lets you serve the Bubble Tea TUI over SSH on a custom port. With the daemon architecture in place, this is a small addition — the TUI becomes one of several possible clients.

---

## Theming

- Dark theme default (current is acceptable)
- Inherit terminal color profile where possible
- App-level theme system as a future feature
