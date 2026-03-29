# Architecture

This document describes the current system design (Phase 1) and the target architecture (Phase 2 daemon, Phase 3 native app). It is the technical complement to [VISION.md](VISION.md).

---

## Phase 1 — Current: Single Process, Interface Discipline

```
┌─────────────────────────────────────────────────────────────────┐
│  mail-processor (single binary)                                 │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  Bubble Tea UI  (internal/app)                           │   │
│  │  Model → Update → View                                   │   │
│  │  talks only to Backend interface, never IMAP/SQL direct  │   │
│  └────────────────────────┬─────────────────────────────────┘   │
│                           │  Backend interface                  │
│  ┌────────────────────────▼─────────────────────────────────┐   │
│  │  LocalBackend  (internal/backend/local.go)               │   │
│  │  - IMAP Client   (internal/imap)                         │   │
│  │  - SQLite Cache  (internal/cache)                        │   │
│  │  - AI Classifier (internal/ai)                           │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘

cmd/ssh-server  → runs the same TUI over SSH (charmbracelet/wish)
                  each SSH session gets its own LocalBackend
cmd/mcp-server  → JSON-RPC stdio server, reads email_cache.db directly
                  no live IMAP; cache-only for all current tools
```

### Package responsibilities

| Package | Responsibility |
|---------|---------------|
| `internal/app` | Bubble Tea model (Init/Update/View), all UI state, message types, key handling |
| `internal/backend` | `Backend` interface + `LocalBackend` implementation wiring IMAP and cache |
| `internal/imap` | IMAP connect, incremental sync, body fetch, deletion, archive, search, background reconcile |
| `internal/cache` | SQLite CRUD: emails, classifications, embeddings, saved searches, folder sync state |
| `internal/ai` | Ollama HTTP client: `Classify()`, `Embed()`, `Chat()` |
| `internal/models` | Shared data types: `EmailData`, `EmailBody`, `SenderStats`, `ProgressInfo`, etc. |
| `internal/config` | YAML config load/validate |
| `internal/smtp` | SMTP send (TLS-first, STARTTLS fallback) |
| `internal/logger` | File-based logger with TUI callback |

### Key design patterns

**Backend interface as the seam**
`internal/backend/backend.go` defines every operation the UI can perform. The Bubble Tea model imports only this interface — never `internal/imap` or `internal/cache` directly. This is the discipline that makes Phase 2 free: swap `LocalBackend` for `RemoteBackend` and the UI is unchanged.

**Progress via channels**
Long-running operations (IMAP sync, classification, reconcile) run in goroutines and send `models.ProgressInfo` values to buffered channels. The UI listens with `tea.Cmd` functions that block on the channel and return a message when something arrives. No polling, no shared state.

**Valid-ID ground truth**
After each sync, `StartBackgroundReconcile` fetches all server UIDs once (no envelopes), builds a `map[string]bool` of live message IDs, and sends it on a channel. All backend read methods filter results against this set. Stale cache rows are batch-deleted in the background (50/batch, 100ms sleep, newest UIDs first) while the UI already shows only valid data.

**Deletion worker**
`DeletionRequest` values are sent to a buffered channel. A single `deletionWorker` goroutine processes them serially (IMAP copy-to-Trash → mark Deleted → expunge → remove from cache). Results flow back via `deletionResultCh`. The UI updates immediately on result without waiting for a full reload.

**SQLite WAL mode**
`PRAGMA journal_mode=WAL` is set at cache init. This allows the TUI, SSH server, and MCP server to read and write the same `email_cache.db` simultaneously without blocking each other. No cross-process locks are held.

---

## Phase 2 — Daemon Server (target)

```
┌──────────────────────────────────────────────────────────────────────┐
│  mail-processor serve  (daemon, localhost:7272)                      │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐     │
│  │  DaemonBackend                                              │     │
│  │  - IMAP Client (one persistent connection per account)      │     │
│  │  - SQLite Cache (WAL, single writer)                        │     │
│  │  - AI Classifier                                            │     │
│  │  - Background reconcile / polling / IDLE                    │     │
│  └─────────────────────────────────────────────────────────────┘     │
│                                                                      │
│  HTTP REST API  + WebSocket push  (localhost:7272)                   │
│  ┌───────────────────────────┐  ┌──────────────────────────────┐     │
│  │  REST /v1/*               │  │  WebSocket /ws               │     │
│  │  mirrors Backend interface│  │  push: progress, new emails, │     │
│  │  (list, search, delete…)  │  │  valid-ID updates, sync state│     │
│  └───────────────────────────┘  └──────────────────────────────┘     │
└───────────┬───────────────────────────────────┬──────────────────────┘
            │                                   │
  ┌─────────┴──────────┐             ┌──────────┴───────────┐
  │  TUI client         │             │  MCP server           │
  │  RemoteBackend      │             │  daemon client        │
  │  (same app code,    │             │  (live IMAP via       │
  │   different backend │             │   daemon; no direct   │
  │   wiring)           │             │   DB access)          │
  └────────────────────┘             └──────────────────────┘
            │
  ┌─────────┴──────────┐
  │  SSH server         │   serves TUI client remotely
  └────────────────────┘
            │
  ┌─────────┴──────────┐
  │  Native app         │   connects to localhost:7272
  │  (Phase 3)          │   same HTTP + WebSocket API
  └────────────────────┘
```

### RemoteBackend

A `RemoteBackend` struct implements the same `Backend` interface as `LocalBackend`. Reads map to HTTP GET requests; writes to HTTP POST/DELETE. Push events (new emails, valid-ID updates, sync progress) arrive via WebSocket and are forwarded to the same channels the TUI already listens on. From the Bubble Tea model's perspective, nothing changes.

### Daemon API design

| Category | Method | Endpoint |
|----------|--------|----------|
| Sync | Load folder | `POST /v1/sync/{folder}` |
| Sync | Sync status | `GET  /v1/sync/status` |
| Read | Timeline | `GET  /v1/emails?folder=&limit=&offset=` |
| Read | By sender | `GET  /v1/emails/by-sender?folder=` |
| Read | Email body | `GET  /v1/emails/{id}/body` |
| Read | Sender stats | `GET  /v1/stats?folder=` |
| Read | Folders | `GET  /v1/folders` |
| Read | Folder status | `GET  /v1/folders/status` |
| Search | Local | `GET  /v1/search?folder=&q=&body=` |
| Search | Cross-folder | `GET  /v1/search/all?q=` |
| Search | Semantic | `GET  /v1/search/semantic?folder=&q=&limit=&min_score=` |
| Write | Delete email | `DELETE /v1/emails/{id}` |
| Write | Delete sender | `DELETE /v1/senders/{sender}?folder=` |
| Write | Archive | `POST /v1/emails/{id}/archive` |
| Write | Mark read | `POST /v1/emails/{id}/read` |
| Write | Classify | `POST /v1/emails/{id}/classify` |
| Write | Send | `POST /v1/send` |
| Push | WebSocket | `GET  /v1/ws` — streams `ProgressEvent`, `NewEmailsEvent`, `ValidIDsEvent` |

Authentication: bearer token in config (`daemon.token`), checked on every request. Localhost-only by default; opt-in to bind on a LAN address.

### CLI control

```
mail-processor serve [--port 7272] [--config proton.yaml]
mail-processor status          # prints running PID, uptime, connected account
mail-processor stop            # SIGTERM to daemon via pidfile
mail-processor sync [folder]   # POST /v1/sync; waits for completion
```

Daemon writes a pidfile to `~/.local/share/mail-processor/daemon.pid` and logs to `~/.local/share/mail-processor/daemon.log`. On macOS, a launchd plist enables autostart at login.

---

## Phase 3 — Native App

The native client connects to the daemon API like any other client. It does not embed IMAP logic or touch SQLite directly.

**macOS-first: SwiftUI**
- Menu bar icon showing unread count (polls `/v1/folders/status` or receives push)
- Full window: three-panel layout mirroring the TUI (folder sidebar, email list, preview)
- System notifications for new mail
- Shares keychain storage for the daemon auth token
- Distributed separately from the daemon; connects to `localhost:7272`

**Cross-platform alternative: Wails**
- Go backend (reuses existing `RemoteBackend`) + web frontend
- Single binary bundles both; no separate daemon required for the bundled mode
- Can also connect to a running daemon (switch backend at startup)

**Client/server boundary**
The daemon API is the only contract. The native app never imports any Go package from this repo — it speaks HTTP and WebSocket only. This means:
- The daemon can be updated independently of the app
- A phone app, browser extension, or Raycast plugin can use the same API
- The TUI remains the reference implementation and is always feature-complete

---

## Data Flow: New Email Arrives

```
IMAP server
    │  EXISTS unsolicited response (IDLE) or polling tick
    ▼
imap.Client.PollForNewEmails / IDLE handler
    │  fetches new envelope + UID
    ▼
cache.CacheEmail                          ← SQLite INSERT
    │
    ▼
backend.newEmailsCh  ←──────────────────  NewEmailsNotification{emails, folder}
    │
    ▼  (Phase 1: direct channel)
app.listenForNewEmails() tea.Cmd
    │
    ▼
NewEmailsMsg → Update() → prepend rows to timeline table
```

In Phase 2, the daemon emits a `NewEmailsEvent` on the WebSocket. `RemoteBackend` receives it and forwards it to the same `newEmailsCh`. The app is unchanged.

---

## Data Flow: Valid-ID Reconciliation

```
Load() completes ("complete" phase)
    │
    ├─ make(chan map[string]bool, 1) → b.validIDsChSt
    │
    └─ go StartBackgroundReconcile(folder, ch)
            │
            ├─ UidFetch("1:*", [FetchUid])       ← UID-only, no envelopes
            ├─ GetCachedUIDsAndMessageIDs(folder) ← all cache rows
            ├─ buildValidIDSet(cached, serverUIDs)
            │       uid==0 → keep conservatively (legacy rows)
            │       uid!=0 && !serverUIDs[uid] → stale
            │
            ├─ ch <- validMessageIDs              ← immediate send
            │
            └─ goroutine: batch-delete stale UIDs
                    for batches of 50, sleep 100ms between

app.listenForValidIDs() receives ValidIDsMsg
    │
    └─ m.backend.Get*() calls now filter against valid set
       stats, classifications, timeline all reload
```

---

## SQLite Schema

```sql
CREATE TABLE emails (
    message_id      TEXT PRIMARY KEY,
    uid             INTEGER,
    sender          TEXT,
    subject         TEXT,
    date            DATETIME,
    size            INTEGER,
    has_attachments INTEGER,
    folder          TEXT,
    is_read         INTEGER DEFAULT 0,
    list_unsubscribe      TEXT,
    list_unsubscribe_post TEXT,
    body_text       TEXT,
    last_updated    DATETIME
);

CREATE TABLE email_classifications (
    message_id    TEXT PRIMARY KEY,
    category      TEXT NOT NULL DEFAULT '',
    classified_at DATETIME NOT NULL
);

CREATE TABLE email_embeddings (
    message_id  TEXT PRIMARY KEY,
    embedding   BLOB NOT NULL,   -- float32 array, little-endian
    body_hash   TEXT NOT NULL,
    created_at  DATETIME NOT NULL
);

CREATE TABLE saved_searches (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    name      TEXT NOT NULL,
    query     TEXT NOT NULL,
    folder    TEXT NOT NULL,
    created_at DATETIME NOT NULL
);

CREATE TABLE folder_sync_state (
    folder      TEXT PRIMARY KEY,
    uidvalidity INTEGER NOT NULL DEFAULT 0,
    uidnext     INTEGER NOT NULL DEFAULT 0,
    updated_at  DATETIME NOT NULL
);
```

WAL mode: `PRAGMA journal_mode=WAL` set at init. FTS5 virtual table (`emails_fts`) created if the SQLite build includes the `fts5` module; gracefully skipped otherwise.
