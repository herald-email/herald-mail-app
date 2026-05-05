# Architecture

This document describes the current system design (Phase 1) and the target architecture (Phase 2 daemon, Phase 3 native app). It is the technical complement to [VISION.md](VISION.md).

---

## Phase 1 — Current: Single Process, Interface Discipline

```
┌─────────────────────────────────────────────────────────────────┐
│  Herald (single binary)                                         │
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
│  │  DemoBackend    (internal/backend/demo.go + fixtures)    │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘

cmd/herald       → primary Go-installable CLI package (`go install .../cmd/herald@latest`)
                  delegates to the shared internal CLI runner
root main.go     → local development wrapper around the same shared CLI runner
herald ssh       → runs the same TUI over SSH (charmbracelet/wish)
                  each SSH session gets its own LocalBackend
cmd/herald-ssh-server  → compatibility wrapper for `herald ssh`
herald mcp       → JSON-RPC stdio server, reads the configured SQLite cache directly
                  no live IMAP; cache-only for normal tools; `--demo` serves fixtures
cmd/herald-mcp-server  → compatibility wrapper for `herald mcp`
```

### Package responsibilities

| Package | Responsibility |
|---------|---------------|
| `internal/cli` | Shared primary CLI runner used by `cmd/herald` and the repository-root development wrapper; dispatches TUI, daemon, MCP, and SSH subcommands |
| `internal/app` | Bubble Tea model (Init/Update/View), all UI state, message types, key handling |
| `internal/backend` | `Backend` interface + `LocalBackend` implementation wiring IMAP and cache |
| `internal/demo` | Shared fictional demo fixtures and deterministic AI used by TUI demo mode and MCP `--demo` |
| `internal/imap` | IMAP connect, incremental sync, body fetch, deletion, archive, search, background reconcile |
| `internal/cache` | SQLite CRUD: emails, classifications, embeddings, saved searches, folder sync state |
| `internal/ai` | Ollama HTTP client: `Classify()`, `Embed()`, `Chat()` |
| `internal/models` | Shared data types: `EmailData`, `EmailBody`, `SenderStats`, `ProgressInfo`, etc. |
| `internal/config` | YAML config load/validate plus onboarding-readiness checks such as vendor presets and empty-config detection |
| `internal/smtp` | SMTP send (TLS-first, STARTTLS fallback) |
| `internal/render` | Email body rendering: shared HTML-to-Markdown conversion, ANSI-aware text wrapping, URL linkification, link sanitization. No TUI dependency — usable from MCP, daemon, SSH |
| `internal/mcpserver` | Shared MCP stdio server implementation used by `herald mcp` and the legacy `herald-mcp-server` wrapper |
| `internal/sshserver` | Shared SSH server implementation used by `herald ssh` and the legacy `herald-ssh-server` wrapper |
| `internal/logger` | File-based logger with TUI callback; writes `herald_*.log` under the platform user log/state directory |

### First-run configuration flow

Startup resolves the config path before logging or backend setup and distinguishes between three states: missing config, empty or whitespace-only config, and existing non-empty config. Missing or empty configs launch the standalone onboarding wizard, while existing non-empty configs still go through normal YAML load and validation so malformed user configs fail loudly instead of being replaced.

The standalone wizard reuses `internal/app.Settings` in a dedicated fullscreen shell rather than the in-app settings overlay. First-run onboarding defaults to IMAP-based paths: Standard IMAP, Gmail IMAP with an App Password, ProtonMail Bridge, Fastmail, iCloud, and Outlook presets. Gmail OAuth is an experimental first-run branch hidden unless the process starts with `-experimental`; when selected it hands off to `OAuthWaitModel`, which uses the same centered modal treatment as the in-app overlay path. The in-app settings panel can still expose the full provider list for existing configured users, but it renders as a compact centered modal over the current Herald view so opening `S` preserves screen context instead of replacing the app chrome.

### Key design patterns

**Backend interface as the seam**
`internal/backend/backend.go` defines every operation the UI can perform. The Bubble Tea model imports only this interface — never `internal/imap` or `internal/cache` directly. This is the discipline that makes Phase 2 free: swap `LocalBackend` for `RemoteBackend` and the UI is unchanged.

**Preserved reply and forward composition**
Compose treats replies and forwards as two pieces of state: the editable top note and a read-only preserved original-message context fetched from IMAP. Reply and forward Compose screens render that source context in an `Original message` pane while keeping the textarea limited to the user's new note. The context carries the original HTML, plain fallback, inline CID images, attachments, and threading headers; `internal/smtp` assembles the final outgoing MIME so Herald does not round-trip rich messages through Markdown. The TUI, daemon, RemoteBackend, and MCP entrypoints all route reply/forward sends through this preserved-context path, while new messages continue to use the regular Markdown compose flow.

**Draft composition workflow**
IMAP `\Draft` flags, Gmail `X-GM-LABELS` `\Draft` labels, and canonical draft folder membership populate `models.EmailData.IsDraft` in the cache, and active-folder reconcile refreshes that flag for existing rows. Cache startup also backfills canonical draft-folder rows so older caches do not leave `[Gmail]/Drafts` messages unmarked after upgrade. Timeline and preview layer draft state on top of reply/thread markers for labels and key hints, and draft reply previews derive compact visible-thread context from the same Timeline grouping so the conversation is visible before the draft body. Compose fetches the draft body plus editable headers (`To`, `CC`, `BCC`, `Subject`) before opening the message for editing. Compose tracks the source draft UID/folder so sending deletes the source only after SMTP success, and Timeline direct send routes through the backend draft-send path to send the saved draft without opening Compose before deleting the source draft. Autosave replacement saves the new draft before deleting the previous saved copy.

**Transient Compose origin**
Compose is an internal full-screen writing state rather than a top-level tab. Timeline `C`, reply, forward, draft edit, and quick reply paths record the originating Timeline state before entering Compose so `Esc` can restore the initiating list, preview, or search context after Compose-local prompts, AI panels, suggestions, and status messages have been dismissed. Leaving a non-empty Compose screen still routes through the existing draft persistence path.

**Shortcut help catalog**
The TUI owns a structured, context-sensitive shortcut catalog in `internal/app` because key routing, visible focus normalization, and overlay state all live there. The bottom hint bar stays abbreviated, while the `?` help overlay renders the full catalog for the current tab, pane, overlay, and Compose mode in a compact centered modal over the current view; semantic search is reached through the Timeline/Contacts search input with a `? query` prefix so plain `?` can remain globally reserved for help. Cleanup configuration surfaces follow the same compact modal pattern: `W` automation rules, `C` cleanup rules, `P` prompts, and dry-run previews compose over the current Herald view instead of replacing the app chrome.

**Layout-independent shortcut matching**
Herald-owned command routing uses Bubble Tea v2 `KeyPressMsg` physical-key data before matching shortcuts, but only in command contexts. `BaseCode` makes browse shortcuts layout-independent on terminals with keyboard enhancement support, while printable fallbacks preserve Cyrillic and direct Japanese kana behavior for terminals that do not report physical keys. Text-entry surfaces such as Compose, Timeline search, Contacts search, attachment paths, and AI prompts keep raw key text so users can type native characters normally; Japanese romaji IME pre-edit remains outside the app until the IME commits text.

**Progress via channels**
Long-running operations (IMAP sync, classification, reconcile) run in goroutines and send channel events back to the Bubble Tea model. The UI listens with `tea.Cmd` functions that block on those channels and return a message when something arrives. No polling, no shared state.

Startup sync should feel live, not frozen. When cached Timeline data is already available, the TUI stays usable and renders an explicit top-of-screen sync strip explaining that current rows are visible while live IMAP work continues. Large IMAP fetches should cache and publish progress in batches so the Timeline and sender stats can refresh incrementally during startup instead of only at the final completion event.

The current recovery target narrows that streaming model into two timing classes. The visible bundle is the active folder rows, the active folder unread/total counts, the current folder title/status, and the folder tree presence; it should settle together within a 2-5 second window under normal startup conditions. Secondary background work such as classifications, embeddings, enrichment, reconcile cleanup, and non-critical refreshes may continue afterward, but they must not block the visible bundle or claim ownership of the main status message.

The folder-sync stream remains generation-tagged and latest-wins, but its role is intentionally narrow: it reports progress and triggers row hydration refreshes. `FolderSyncEvent` values such as `sync_started`, `snapshot_ready`, `rows_cached`, `counts_updated`, `reconcile_started`, `sync_complete`, and `sync_error` should never synthesize authoritative folder totals from the visible row slice. Live IMAP folder status is the only source of truth for sidebar, status-bar, and Cleanup unread/total counts. Visible snapshot refreshes remain microbatched at `100` changes or `500ms` so the UI moves forward smoothly without repaint churn.

The app therefore tracks the active folder explicitly as four pieces of state: current folder rows, current folder live counts, current folder sync phase, and whether the current folder bundle is settled. Cached rows can be shown early, but they are provisional until the live counts settle. Background reconcile, sender-stat refreshes, retries, or cache-hydration updates must not repaint the current folder with contradictory counts or a premature `synced` state.

**Valid-ID ground truth**
During each active-folder sync, Herald refreshes cached IMAP flags from `UID + FLAGS` so cached `is_read`, starred, and draft state follows server truth even when no new UID arrived. After sync completion, `StartBackgroundReconcile` fetches all server UIDs once (no envelopes), builds a folder-scoped `map[string]bool` of live message IDs, and sends it on a channel that the UI can already see by the time `sync_complete` arrives. All backend read methods filter results against the set for each row's own folder; folders without a reconcile result remain unfiltered until their own server truth arrives. Stale cache rows are batch-deleted in the background (50/batch, 100ms sleep, newest UIDs first) while the UI already shows only valid data. Legacy or incomplete cache rows with no server UID are also invalidated automatically by message ID so they do not linger as half-openable search results.

**Virtual diagnostic folders**
Some investigative views should not pretend to be real IMAP mailboxes. The first example is `All Mail only`, a read-only virtual folder derived from live IMAP folder membership rather than the current cache row’s single `folder` value. The source of truth is the server: start from the `All Mail` message-ID set, subtract every other real folder assignment, and only keep mail that is otherwise folder-unassigned. Messages that also live in `INBOX`, `Sent`, `Archive`, or any nested subfolder are not part of this view. If `All Mail` is unavailable or any required membership fetch fails, the view fails closed with an explicit unsupported or error state rather than showing a partial unsafe result set.

**Stable selection identity**
Cleanup summary selection is treated as logical sender/domain identity, not row position. Checkmarks, selection counts, bulk actions, and resize/re-sort behavior must all derive from the same stable key set so refreshes and resizes cannot desynchronize what the user sees from what the app thinks is selected.

Timeline bulk selection follows the same identity rule at message granularity. The UI stores selected Timeline message IDs separately from Cleanup selections, renders the checkmark column from those IDs, expands collapsed thread-row selections to every represented message, and prunes the set when the active Timeline working set changes or a bulk action completes.

**Deletion worker**
`DeletionRequest` values are sent to a buffered channel. A single `deletionWorker` goroutine processes them serially (IMAP copy-to-Trash → mark Deleted → expunge → remove from cache). Results flow back via `deletionResultCh`. The UI updates immediately on result without waiting for a full reload.

**Config-specific SQLite cache**
The SQLite database path is part of configuration. If YAML already contains a cache database path, every local cache reader and writer uses it as authoritative. If it is missing, startup generates an absolute `<home>/.herald/cached/<config-name>.db` path from the config filename, disambiguates with a date and short random suffix when that file already exists, writes the chosen absolute path back to YAML, and then opens the cache.

**SQLite WAL mode**
`PRAGMA journal_mode=WAL` is set at cache init. This allows the TUI, SSH server, daemon, and MCP server to read and write the same configured SQLite database simultaneously without blocking each other. No cross-process locks are held.

### AI work scheduling and network safety

AI work now needs its own resource model because local Ollama capacity behaves very differently from external APIs. The UI must stay responsive even when embeddings, enrichment, classification, chat, and image description are all active, so the scheduler treats local AI as scarce machine capacity and explicitly prefers interactive work over background throughput.

### Inline image preview safety

Timeline and Cleanup previews keep image bytes local to the TUI process. Full-screen preview renders bounded iTerm2 OSC 1337 images or Kitty graphics images when auto-detected or selected with `-image-protocol`; otherwise local TUI sessions expose current MIME inline images through random, in-memory `127.0.0.1` URLs wrapped in OSC 8 links. SSH sessions default to placeholders because a user's browser would not be on the same host, but explicit `-image-protocol=iterm2` or `-image-protocol=kitty` opts into raster output over SSH. Remote HTML image URLs are rendered as links without being fetched by Herald.

**Interactive-before-background priority**

- Highest priority: user-blocking interactive work such as chat replies, semantic query embeddings, quick replies, current-email image description, and user-triggered single-contact enrichment
- Medium priority: explicit user-triggered folder classification or other visible batch actions
- Lowest priority: background email embeddings and background contact enrichment
- Strict interactive-first means no new background local-AI task is dispatched while interactive work is queued or running
- The scheduler is intentionally non-preemptive: one already-running background Ollama call may finish before the waiting interactive task begins

**Bounded queue, fail-open behavior**

- Local AI work uses a bounded queue and a low default concurrency so Herald does not exhaust local sockets or starve the rest of the machine
- Low-priority duplicate work is dropped or coalesced instead of opening more concurrent requests
- Background work pauses while interactive local AI work is active when `pause_background_while_interactive` is enabled
- Queue saturation must fail open: the UI remains responsive, low-priority work is deferred or skipped, and the user gets concise status/log feedback instead of a connection storm
- The TUI exposes this state through a compact global `AI:` chip so users can tell whether Herald is idle, busy, deferred, or unavailable without opening logs

**Transport policy**

- Local AI uses a shared HTTP transport with strict per-host connection caps
- Local queued work should not depend on one blanket short timeout because large local models can be slow but still healthy
- External AI may use a higher bounded concurrency because remote providers tolerate parallelism better, but it remains config-driven

**Embedding model identity**

- Semantic vectors are tied to the configured embedding model, not just to the message body hash
- Cache startup records the active embedding model in SQLite metadata
- If the configured embedding model changes, Herald invalidates cached email and contact embeddings before new semantic work starts
- Legacy caches without recorded embedding-model metadata are treated as compatible only with the historical default `nomic-embed-text`; switching to another model forces invalidation

**Hybrid Timeline search**

- Plain Timeline search combines fast local sender/subject keyword matches with semantic expansion when embeddings are available
- Keyword hits remain the stable head of the result set while semantic-only candidates are appended in similarity order
- Semantic expansion is bounded by a configured score threshold and a fixed result cap so one query does not fan out into an unbounded whole-folder ranking pass
- Duplicate message IDs from the keyword and semantic legs are coalesced before results reach the TUI
- Timeline search is modeled as explicit input and result-navigation submodes rather than a single global text overlay
- Local search dispatch is debounced and tagged with sequence tokens so stale responses are ignored when newer typing supersedes them
- Search unwind is step-based: `Esc` returns from preview to results, from results to the input, and only then restores the original timeline snapshot

**Configuration**

These controls live under `ai:` in config:

- `provider`
- `local_max_concurrency`
- `external_max_concurrency`
- `background_queue_limit`
- `pause_background_while_interactive`

---

## Phase 2 — Daemon Server (target)

```
┌──────────────────────────────────────────────────────────────────────┐
│  herald serve  (daemon, localhost:7272)                      │
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

A `RemoteBackend` struct implements the same `Backend` interface as `LocalBackend`. Reads map to HTTP GET requests; writes to HTTP POST/DELETE. Push events (new emails, valid-ID updates, sync progress, and folder sync stream events) arrive via WebSocket and are forwarded to the same channels the TUI already listens on. From the Bubble Tea model's perspective, nothing changes.

### Daemon API design

| Category | Method | Endpoint |
|----------|--------|----------|
| Sync | Load folder | `POST /v1/sync/{folder}` |
| Sync | Sync status | `GET  /v1/sync/status` |
| Read | Timeline | `GET  /v1/emails?folder=&limit=&offset=` |
| Read | By sender | `GET  /v1/emails/by-sender?folder=` |
| Read | Email body | `GET  /v1/emails/{id}/body` |
| Read | Attachment list | `GET  /v1/emails/{id}/attachments` |
| Read | Attachment download | `GET  /v1/emails/{id}/attachments/{filename}?dest_path=` |
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

Attachment downloads that include `dest_path` use create-exclusive local writes. If the requested file already exists, the daemon returns `409 Conflict` with `error`, `path`, and `suggested_path` fields so TUI, MCP, and native clients can avoid silent overwrites.

### CLI control

```
herald serve [--port 7272] [--config proton.yaml]
herald status          # prints running PID, uptime, connected account
herald stop            # SIGTERM to daemon via pidfile
herald sync [folder]   # POST /v1/sync; waits for completion
```

Daemon writes a pidfile to `~/.local/share/herald/daemon.pid` and logs to `~/.local/share/herald/daemon.log`. On macOS, a launchd plist enables autostart at login.

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
            │       uid==0 → stale legacy/no-uid row
            │       uid!=0 && !serverUIDs[uid] → stale
            │
            ├─ ch <- validMessageIDs              ← immediate send
            │
            └─ goroutine: batch-delete stale rows
                    delete stale UIDs by uid
                    delete legacy rows by message_id
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
    is_draft        INTEGER DEFAULT 0,
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

CREATE TABLE cache_metadata (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    updated_at  DATETIME NOT NULL
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
