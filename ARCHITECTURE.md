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
| `internal/render` | Email body rendering: shared HTML-to-Markdown conversion, ANSI-aware text wrapping, URL linkification, and preview URL target sanitization for visible labels plus OSC 8 hyperlink targets. No TUI dependency — usable from MCP, daemon, SSH |
| `internal/mcpserver` | Shared MCP stdio server implementation used by `herald mcp` and the legacy `herald-mcp-server` wrapper |
| `internal/sshserver` | Shared SSH server implementation used by `herald ssh` and the legacy `herald-ssh-server` wrapper |
| `internal/logger` | File-based logger with TUI callback; writes `herald_*.log` under the platform user log/state directory and masks private mailbox/config data unless `-unsafe-logs` is explicitly enabled |
| `internal/deeplink` | Parses and builds `herald://mail/...` links for folder, message, sender, search, and compose contexts |
| `internal/notifications` | Platform notification boundary: macOS native activation, Linux delivery-only fallback, unsupported-platform no-op, and fake recorder for tests |

### First-run configuration flow

Startup resolves the config path before logging or backend setup and distinguishes between three states: missing config, empty or whitespace-only config, and existing non-empty config. Missing or empty configs launch the standalone onboarding wizard, while existing non-empty configs still go through normal YAML load and validation so malformed user configs fail loudly instead of being replaced.

The standalone wizard reuses `internal/app.Settings` in a dedicated fullscreen shell rather than the in-app settings overlay. First-run onboarding keeps the Account Type chooser first, then opens compact provider-specific express paths. Gmail OAuth shows a Google account step with optional email identity, Mail enabled, and Google Calendar enabled by default; credential provider paths keep Gmail IMAP with an App Password, ProtonMail Bridge, Fastmail, iCloud, Outlook, and Standard IMAP available with editable preset host/port fields where relevant. Gmail OAuth hands off to `OAuthWaitModel`, which uses the same centered modal treatment as the in-app overlay path and shows generic Google authorization copy. OAuth completion produces a candidate config with token data, not a saved config. First-run Google OAuth writes explicit `sources[]`: a Gmail mail source with `provider: gmail` and, when selected, a Google Calendar source with `provider: google_calendar`. The Gmail source opens the Gmail API adapter for core mail operations and narrower Gmail API access; `provider: gmail_api` remains accepted as a compatibility alias, and credential/app-password Gmail remains the IMAP adapter path. First-run account setup passes the account candidate through `internal/accountcheck` immediately after the account details step, before optional preferences are shown. First-run back navigation treats `Esc` and boundary-crossing `Shift+Tab` as navigation commands, so they do not run required-field validation for the current screen. If OAuth or account validation fails, the wizard keeps the candidate values and returns the user to the populated provider setup screen instead of restarting from the chooser. After validation, the fast path shows a compact Advanced settings review with default preferences: AI disabled/deferred, offline cache set to message bodies without attachments, keyboard Default, theme Inherited/Default, and empty signature. The post-validation screen offers `Enter Herald` plus `Customize setup`; customization opens the full AI, cache, keyboard, theme, and signature preference steps. In-app account management starts from `Settings > Accounts`, which groups configured `sources[]` by `account_id`, shows mail/calendar capability badges, opens source-scoped account detail forms, and offers an `Add account` flow for mail or standalone calendar sources. Google Calendar sources use the same supported Google OAuth flow as Gmail instead of Google's CalDAV endpoint, while Fastmail, iCloud, Yahoo, and Custom CalDAV remain URL/username/password paths. Mail source saves reuse the account validation gate before config writes, backend replacement, or SMTP-client replacement; calendar source saves validate by opening the configured calendar source and listing calendars without mutating events. Normal startup for an already configured account does not use the account gate, so cached/offline startup behavior remains available during provider outages. For existing Ollama configs, startup model checks are advisory: missing or unreachable models set the AI status to down, disable in-memory AI actions, and expose repair details in `Settings > AI` without blocking cached mail. The in-app settings panel can still expose the full provider list and advanced Sync & Cleanup controls for existing configured users, including automation rules, custom prompts, and cleanup rules now that Cleanup is no longer a top-level browse view, but it renders as a compact centered modal over the current Herald view so opening `S` preserves screen context instead of replacing the app chrome. In panel mode, Settings starts at a top-level category menu (`Accounts`, `AI`, `Sync & Cleanup`, `Keyboard`, `Theme Selection`, `Theme Editor`, `Signature`); saving non-account categories writes the same YAML config shape and returns to that menu, while account/source saves validate before replacing the active account graph and show a compact error modal when validation fails. AI saves that newly select or change Ollama use the same model-inventory validation before applying the config; if a previously configured model later disappears, `Settings > AI` shows the install commands and offers a Save Disabled action. Theme Selection owns choosing the active theme and installing local YAML files, while Theme Editor owns semantic role overrides, reset controls, live preview, and save-as-new-theme. `Esc` unwinds focused filter state and category depth before exiting Settings without saving unsaved edits.

### Key design patterns

**Connection validation before account persistence**
`internal/accountcheck` owns setup-time connection validation for candidate configs. It creates short-lived provider clients, authenticates the required read and send surfaces, closes them, and returns independent check results plus a combined user-facing summary. Detailed provider errors are logged; UI copy stays bounded and points users to the log file and config path behavior. Gmail OAuth validates through the Gmail API mail source with provider-aware scopes and can include Google Calendar scope only when the first-run or Settings flow selected a Calendar source; Gmail App Password and other credential-based Gmail configs still use the IMAP/SMTP validation gate behind generic user-facing setup copy.

**Backend interface as the seam**
`internal/backend/backend.go` defines every operation the UI can perform. The Bubble Tea model imports only this interface — never `internal/imap` or `internal/cache` directly. This is the discipline that makes Phase 2 free: swap `LocalBackend` for `RemoteBackend` and the UI is unchanged.

**Preserved reply and forward composition**
Compose treats replies and forwards as two pieces of state: the editable top note and a read-only preserved original-message context fetched from IMAP. Reply and forward Compose screens render that source context in an `Original message` pane while keeping the textarea limited to the user's new note. The context carries the original HTML, plain fallback, inline CID images, attachments, and threading headers; `internal/smtp` assembles the final outgoing MIME so Herald does not round-trip rich messages through Markdown. The TUI, daemon, RemoteBackend, and MCP entrypoints all route reply/forward sends through this preserved-context path, while new messages continue to use the regular Markdown compose flow.

**Timeline thread context**
Cached `EmailData` rows carry provider thread IDs plus RFC `In-Reply-To` and `References` metadata from IMAP and Gmail API sources. Active-folder Timeline reads can add cached Sent-folder replies only when those metadata values link the Sent row to a visible active-folder message; subject text alone is not enough. The grafted rows keep their original source and folder identity so preview, read/star/archive/delete, reply, forward, SSH, daemon, and MCP paths still route mutations through the correct provider collection.

**Draft composition workflow**
IMAP `\Draft` flags, Gmail `X-GM-LABELS` `\Draft` labels, and canonical draft folder membership populate `models.EmailData.IsDraft` in the cache, and active-folder reconcile refreshes that flag for existing rows. Cache startup also backfills canonical draft-folder rows so older caches do not leave `[Gmail]/Drafts` messages unmarked after upgrade. Timeline and preview layer draft state on top of reply/thread markers for labels and key hints, and draft reply previews derive compact visible-thread context from the same Timeline grouping so the conversation is visible before the draft body. Compose fetches the draft body plus editable headers (`To`, `CC`, `BCC`, `Subject`) before opening the message for editing. Compose tracks the source draft UID/folder so sending deletes the source only after send success, and Timeline direct send routes through the backend draft-send path to send the saved draft without opening Compose. IMAP-backed sources send drafts through SMTP, while Gmail API sources use `users.drafts.*` for create, list, delete, and send while keeping raw draft IDs behind Herald UIDs and scoped refs. Autosave replacement saves the new draft before deleting the previous saved copy.

**Transient Compose origin**
Compose is an internal full-screen writing state rather than a top-level tab. Timeline `C`, reply, forward, draft edit, and quick reply paths record the originating Timeline state before entering Compose so `Esc` can restore the initiating list, preview, or search context after Compose-local prompts, AI panels, suggestions, and status messages have been dismissed. Successful sends return directly to Timeline with the send status visible there. Leaving a non-empty blank Compose or draft edit still routes through the existing draft persistence path, while reply and forward Compose exits first ask whether to keep the response draft or discard it.

**Compose AI writing assistant**
Compose owns the writing-assistant UI state and sends draft snapshots to the configured `ai.AIClient` through the existing chat interface. The AI command bar defaults open between the message headers and body as a compact single row, with Translate and Style dropdowns, quick actions, undo, and an inline freeform instruction input; if no AI provider is configured, the same bar renders a disabled warning instead of actionable controls. Rewrites return cleaned editable suggestions and word-level diffs inside an AI review mode that temporarily replaces the main body editor instead of appending another panel below it, stripping prompt scaffolding such as `Current draft:` echoes before display. The draft body, SMTP send path, and draft persistence are not mutated until the user explicitly accepts the suggestion, the command bar remains available after review closes, and the previous body is retained for a one-step AI undo.

**Compose signatures**
The profile config owns one optional legacy default signature at `compose.signature.text`, and each mail source can override it at `sources[].compose.signature.text`. The Settings panel still edits the profile default, while multi-account Compose uses the selected source's signature when opening blank messages, replies, forwards, and quick replies. Compose entrypoints append the resolved signature to the editable textarea, leave two empty lines before the signature, and place the cursor at the first editable line so users type above it; draft editing restores saved body text exactly. Signatures are never appended at send time, and a signature-only blank Compose body does not count as content for draft autosave.

**Shortcut command catalog**
The TUI owns a structured, context-sensitive command catalog in `internal/app` because key routing, visible focus normalization, text-entry safety, and overlay state all live there. Commands have stable IDs, profile bindings, scope/mode availability, safety metadata, and display text; the bottom hint bar stays abbreviated while the `?` help overlay renders the active profile's resolved catalog for the current tab, pane, overlay, and field mode in a compact centered modal over the current view. The bottom hint bar may pivot to Shift, Ctrl, or Alt presentation layers when terminal key-event support exposes modifier state, but those layers are informational and still derive from existing context-valid commands. Editable text-entry surfaces, including Compose fields, search fields, prompts, settings, and editor-like fields, keep literal printable input unless their active field adapter explicitly owns a modal command.

**Keyboard profiles and custom keymaps**
The account config selects a keyboard profile at `keyboard.profile` (`default`, `vim`, `emacs`, or `custom`) and optionally points `keyboard.custom_keymap` at a separate YAML file. Custom keymaps extend a built-in profile, map keys only to predefined command IDs, and can opt Compose into `insert`, `normal`, or `visual` default field modes. The default profile keeps text fields insert-first but uses Vim-compatible browse navigation (`h/j/k/l`), contextual `/` search, `r` reply-all, `R` sender-only reply, `f` forward, `a` archive-current, and `D` confirmed delete; Vim profile adds minimal normal/insert/visual Compose field adapters, while Emacs remains insert-first with Emacs-style movement aliases.

**Theme catalog and role resolution**
The account config selects a theme at `theme.name` and may include `theme.overrides` keyed by semantic role IDs such as `chrome.status_bar` or `focus.selection_active`. `internal/config` preserves the YAML shape, while `internal/app` resolves built-ins, installed local YAML themes from `~/.herald/themes`, one-session `-theme` file/name overrides, and per-config overrides into a model-owned `Theme`. Missing config defaults to `inherited`, `adaptive` aliases to `inherited`, `legacy-dark` aliases to `herald-dark`, and `crymsom` aliases to `crimson`; invalid installed themes fall back to inherited with bounded status/log feedback instead of crashing startup, while invalid `-theme` values fail launch loudly because they are explicit CLI requests.

**Layout-correct shortcut matching**
Herald-owned command routing treats printable associated text from Bubble Tea v2 `KeyPressMsg` as authoritative for Latin/ASCII command keys, so non-US layouts such as QWERTZ and AZERTY produce the character the user actually typed. `BaseCode` remains available for non-printable navigation keys and for non-Latin physical shortcut compatibility, while printable fallback aliases preserve Cyrillic and direct Japanese kana behavior when terminals do not report physical keys. Text-entry surfaces such as Compose, Timeline search, Contacts search, attachment paths, settings, prompt editors, and AI prompts keep raw key text so users can type native characters normally, including macOS Option-generated text; Japanese romaji IME pre-edit remains outside the app until the IME commits text.

**Progress via channels**
Long-running operations (IMAP sync, classification, reconcile) run in goroutines and send channel events back to the Bubble Tea model. The UI listens with `tea.Cmd` functions that block on those channels and return a message when something arrives. No polling, no shared state.

Startup sync should feel live, not frozen. When cached Timeline data is already available, the TUI stays usable and renders an explicit top-of-screen sync strip explaining that current rows are visible while live IMAP work continues. Large IMAP fetches should cache and publish progress in batches so the Timeline and sender stats can refresh incrementally during startup instead of only at the final completion event.

The current recovery target narrows that streaming model into two timing classes. The visible bundle is the active folder rows, the active folder unread/total counts, the current folder title/status, and the folder tree presence; it should settle together within a 2-5 second window under normal startup conditions. Secondary background work such as classifications, embeddings, enrichment, reconcile cleanup, and non-critical refreshes may continue afterward, but they must not block the visible bundle or claim ownership of the main status message.

The folder-sync stream remains generation-tagged and latest-wins, but its role is intentionally narrow: it reports progress and triggers row hydration refreshes. `FolderSyncEvent` values such as `sync_started`, `snapshot_ready`, `rows_cached`, `counts_updated`, `reconcile_started`, `sync_complete`, and `sync_error` should never synthesize authoritative folder totals from the visible row slice. Live IMAP folder status is the only source of truth for sidebar, status-bar, and Timeline counts. Visible snapshot refreshes remain microbatched at `100` changes or `500ms` so the UI moves forward smoothly without repaint churn.

The app therefore tracks the active folder explicitly as four pieces of state: current folder rows, current folder live counts, current folder sync phase, and whether the current folder bundle is settled. Cached rows can be shown early, but they are provisional until the live counts settle. Background reconcile, sender-stat refreshes, retries, or cache-hydration updates must not repaint the current folder with contradictory counts or a premature `synced` state.

**Valid-ID ground truth**
During each active-folder sync, Herald refreshes cached IMAP flags from `UID + FLAGS` so cached `is_read`, starred, and draft state follows server truth even when no new UID arrived. After sync completion, `StartBackgroundReconcile` fetches all server UIDs once (no envelopes), builds a folder-scoped `map[string]bool` of live message IDs, and sends it on a channel that the UI can already see by the time `sync_complete` arrives. All backend read methods filter results against the set for each row's own folder; folders without a reconcile result remain unfiltered until their own server truth arrives. Stale cache rows are batch-deleted in the background (50/batch, 100ms sleep, newest UIDs first) while the UI already shows only valid data. Legacy or incomplete cache rows with no server UID are also invalidated automatically by message ID so they do not linger as half-openable search results.

Gmail API mail sources store per-source/per-folder `historyId` cursors in cache metadata after a bounded list/get sync. Subsequent normal syncs call `users.history.list` with the stored cursor and selected label, fetch affected messages for add/label-change events, remove cached rows for provider delete/trash events, and update the cursor only after provider events are applied. If Gmail returns an expired cursor or the cursor is missing, the source falls back to the existing bounded list/get sync and records a fresh cursor from the fetched messages.

Gmail API transport hardening lives inside `GmailAPIMailSource`: list, label, draft, and history endpoints consume page tokens until exhausted, provider 429/5xx responses retry with short bounded backoff before returning a trimmed UI-safe error, and Compose sends assemble a raw RFC 2822 message with CC/BCC, Markdown HTML/plain alternatives, inline images where available, and staged attachments before posting `users.messages.send`. Provider IDs, page tokens, retry details, and raw OAuth tokens remain behind the source boundary.

**Virtual diagnostic folders**
Some investigative views should not pretend to be real IMAP mailboxes. The first example is `All Mail only`, a read-only virtual folder derived from live IMAP folder membership rather than the current cache row’s single `folder` value. The source of truth is the server: start from the `All Mail` message-ID set, subtract every other real folder assignment, and only keep mail that is otherwise folder-unassigned. Messages that also live in `INBOX`, `Sent`, `Archive`, or any nested subfolder are not part of this view. If `All Mail` is unavailable or any required membership fetch fails, the view fails closed with an explicit unsupported or error state rather than showing a partial unsafe result set.

**Stable selection identity**
Timeline selection is treated as message identity, not row position. The UI stores selected Timeline message IDs, renders the checkmark column from those IDs, expands collapsed thread/sender/domain group selections to every represented message, and prunes the set when the active Timeline working set changes or a bulk action completes.

**Deletion worker**
`DeletionRequest` values are sent to a buffered channel. A single `deletionWorker` goroutine processes them serially (IMAP copy-to-Trash → mark Deleted → expunge → remove from cache). Results flow back via `deletionResultCh`. The UI updates immediately on result without waiting for a full reload.

**Config-specific SQLite cache**
The SQLite database path is part of configuration. If YAML already contains a cache database path, every local cache reader and writer uses it as authoritative. If it is missing, startup generates an absolute `<home>/.herald/cached/<config-name>.db` path from the config filename, disambiguates with a date and short random suffix when that file already exists, writes the chosen absolute path back to YAML, and then opens the cache.

**Cached folder tree recovery**
Successful live folder-list responses are persisted in the configured SQLite cache as last-known folder names. Local startup primes the in-memory folder tree from those cached names before the IMAP connection finishes, then replaces the names only after a fresh server `LIST` succeeds. Cached folder names are navigation scaffolding only; sidebar, status-bar, and Timeline unread/total counts continue to come from live IMAP status responses.

**Offline preview cache policy**
`cache.storage_policy` controls how much message body data Herald persists for offline reading. `no_attachments` is the default and keeps message bodies plus inline preview data where available, while stripping downloadable attachment bytes. `lightweight` stores preview text/HTML, useful headers, and attachment metadata without inline image or attachment bytes. `preserve_all` stores fetched preview bodies with attachment bytes so later saves can use local data. When users save a stricter policy, Herald normalizes existing preview-cache rows to that target policy so old attachment or inline-image bytes do not linger while preview text, headers, and attachment metadata remain available. The Settings panel also exposes an explicit reclaim action that estimates removable cached preview bytes for the current policy, confirms that text/headers/attachment metadata stay cached, prunes disallowed binary payloads, and runs SQLite `VACUUM` as best-effort compaction. Timeline preview loaders first try the persistent preview cache, then use a preview-specific IMAP fetch for lightweight/no-attachment misses, and only fall back to full body fetches where the backend does not expose a preview fetch path. After non-read-only Timeline data loads, an app-side preview prewarmer snapshots the active folder's newest messages and warms uncached previews one message at a time; folder switches advance the background generation so stale prewarm results are ignored before they can schedule more IMAP work.

**SQLite WAL mode**
`PRAGMA journal_mode=WAL` is set at cache init. This allows the TUI, SSH server, daemon, and MCP server to read and write the same configured SQLite database simultaneously without blocking each other. No cross-process locks are held.

### AI work scheduling and network safety

AI work now needs its own resource model because local Ollama capacity behaves very differently from external APIs. The UI must stay responsive even when embeddings, enrichment, classification, chat, and image description are all active, so the scheduler treats local AI as scarce machine capacity and explicitly prefers interactive work over background throughput.

### Gollem chat-agent replacement

Chat is a UI-mode agent surface, not a daemon, MCP, or background automation surface in the first Gollem iteration. The existing right-side chat drawer remains the user-facing shell, but the hand-written Ollama `/api/chat` loop, legacy chat tool registry, and prose `<filter>` response contract are not architecture foundations; they should be removed once a Gollem runner can return a plain reply and typed UI intents.

The replacement boundary should live in an `internal/agent` package with a small runner interface that receives a TUI snapshot and returns a typed result. Bubble Tea owns all state mutation: the agent can propose a Timeline filter, search query, summary, or Compose edit, but `internal/app` applies those intents through existing Timeline search/filter and Compose AI review paths.

**Agent runner contract**

- [ ] `internal/app` builds a bounded `ChatAgentInput` snapshot with current folder, active tab, selected/visible message IDs, optional Compose draft state, and the user's latest chat message.
- [ ] `internal/agent` owns Gollem agent construction, provider selection, typed tool definitions, structured result parsing, and provider error normalization.
- [ ] `internal/backend` remains the source of email reads and searches; Gollem tools adapt to existing backend/search methods instead of opening separate IMAP, SQLite, daemon, or MCP paths.
- [ ] `internal/app` applies typed `TimelineIntent` values through existing Timeline search and chat-filter state so result browsing, `Esc`, selection, and preview behavior stay consistent.
- [ ] `internal/app` applies typed `ComposeIntent` values only through the existing Compose AI review/accept flow; chat never silently mutates drafts and cannot send mail in the first iteration.
- [ ] The first agent tool set is read-only: `find_emails`, `get_email_context`, `summarize_email_set`, and `explain_people`.
- [ ] The first provider set is Gollem-backed Ollama/local, Anthropic, Kimi, and Fireworks; Ollama is only a provider behind Gollem, not a separate chat runtime.
- [ ] Agent requests publish concise progress states such as `searching`, `reading`, `summarizing`, and `draft edit ready` so the chat drawer does not appear stuck during slow local or remote provider calls.
- [ ] The first implementation does not add memory, autonomous daily summaries, calendar mutations, delete/archive, send mail, or MCP mirroring.

### Inline image preview safety

Timeline previews keep image bytes local to the TUI process. Split and full-screen previews render bounded iTerm2 OSC 1337 images or Kitty graphics images when auto-detected or selected with `-image-protocol`; otherwise local TUI sessions expose current MIME inline images through random, in-memory `127.0.0.1` URLs wrapped in OSC 8 links. Native raster overlays carry row geometry so Herald can crop and re-encode the visible image slice when a scroll position or split-preview panel boundary shows only part of the image; if cropping cannot be done safely, the overlay is suppressed rather than allowed to overflow. SSH sessions default to placeholders because a user's browser would not be on the same host, but explicit `-image-protocol=iterm2` or `-image-protocol=kitty` opts into raster output over SSH. Remote HTML image URLs render as readable placeholders and OSC 8 links by default; known tracker query params are stripped from placeholder hyperlink targets and the opt-in fetch URL while non-tracker params are preserved. Pressing `o` in the current Timeline preview fetches only the current message's remote images with bounded, no-cookie, no-referrer HTTP(S) requests, blocks local/private/link-local destinations and unsafe redirects, keeps bytes in memory only, and then sends revealed images through the same renderer/fallback path as MIME inline images.

### Preview selection and clipboard payloads

Read-only email preview selection is a TUI-layer reader mode over the shared preview document. Preview rows carry copy metadata such as plain text, HTML fragments, and image identity so the cursor/highlight renderer does not need to scrape ANSI output; clipboard writes go through an internal payload writer that always supports plain text and can add HTML or image data on platforms that expose richer clipboard APIs.

**Interactive-before-background priority**

- Highest priority: user-blocking interactive work such as chat replies, semantic query embeddings, quick replies, current-email image description, and user-triggered single-contact enrichment
- Medium priority: explicit user-triggered folder classification or other visible batch actions
- Lowest priority: background email embeddings and background contact enrichment
- Strict interactive-first means no new background local-AI task is dispatched while interactive work is queued or running
- The scheduler is intentionally non-preemptive: one already-running background Ollama call may finish before the waiting interactive task begins

**Quality-first local AI defaults**

- New configs default local Ollama chat/classification to `gemma3:4b` and embeddings to `nomic-embed-text-v2-moe` so translation, writing assistance, and semantic search start from stronger local models
- Setup warns that these recommended defaults are comfortable with at least 16GB RAM; 8GB can still work, but users should expect slower local AI responses
- Custom Ollama setup uses curated selectors for common chat and embedding model sizes, while preserving freeform model names and a downgrade path for constrained machines; the downgrade copy calls out that `llama3.x` is weaker for translation
- Existing explicit model values remain authoritative; default changes only fill blank config values
- Setup and changed AI settings validate local Ollama model names against `/api/tags` before saving, while existing configs use non-blocking warning state so offline startup remains available

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

## Source Platform Direction

The next refactor layer turns the current single-account backend discipline into a source platform for multi-account mail and calendar integrations. The detailed architecture lives in [docs/superpowers/specs/2026-05-22-source-platform-architecture.md](docs/superpowers/specs/2026-05-22-source-platform-architecture.md), and the first implementation roadmaps live in [docs/superpowers/plans/2026-05-22-work-coordinator-foundation.md](docs/superpowers/plans/2026-05-22-work-coordinator-foundation.md) and [docs/superpowers/plans/2026-05-22-source-identity-foundation.md](docs/superpowers/plans/2026-05-22-source-identity-foundation.md).

- [x] Introduce `SourceID`, `AccountID`, `CollectionRef`, and `MessageRef` before exposing multi-account UI.
- [x] Introduce `EventRef` before exposing calendar UI or cross-source event APIs.
- [x] Keep source plugins provider-specific but small: IMAP, Google Calendar, and CalDAV handle transport details while Herald owns cache policy, queue policy, stale-result filtering, and UI priority.
- [x] Extract current IMAP provider operations from `LocalBackend` into `IMAPMailSource` behind a mail capability interface.
- [x] Add an active-account backend wrapper so the TUI can switch between configured mail sources while legacy `Backend` callers continue to use folder/message-ID methods.
- [x] Add an opt-in `All Accounts` TUI scope that aggregates Timeline/search through source backends, renders account badges, and routes selected-message reads/writes by `MessageRef`.
- [x] Add account-aware Compose routing so blank messages, replies, forwards, drafts, and sends can select a real mail source while single-account sessions keep the legacy Compose surface.
- [x] Add an additive account-folder snapshot API so the TUI sidebar can render Mail.app-style Favorites and per-account folder sections without changing legacy folder methods.
- [x] Move latest-user-intent, duplicate resource coalescing, serial mutations, and fair background work into reusable coordination primitives before migrating existing queues.
- [x] Keep mail body and preview reads cache-first so callers ask once while the service decides persistent cache, in-flight join, completed replay, or provider fetch.
- [x] Extend the same cache-first service boundary to calendar event reads after `EventRef` and calendar cache storage exist.
- [x] Add an optional cache-backed Calendar Agenda TUI destination with read-only Event Detail; mail-only sessions do not advertise the Calendar tab.
- [x] Add a read-only Day Agenda + Drawer calendar view that reuses cache-backed agenda rows and preserves the full Event Detail reader.
- [x] Add a read-only Week Time-Grid calendar view that reuses cache-backed agenda rows and preserves the full Event Detail reader.
- [x] Add a read-only 3-Day Command calendar view that reuses cache-backed agenda rows and preserves the full Event Detail reader.
- [x] Enrich the read-only Event Detail reader with attendees, RSVP state, recurrence, attachments, and explicit local/event/alternate timezone rows before calendar mutation UI.
- [x] Add a read-only Calendar Search view that searches cached, scoped event rows without exposing provider identifiers or mutation controls.
- [x] Add a read-only Cross-Source Search view that blends cached mail and calendar event rows without changing Timeline search, Calendar Search, or mutation surfaces.
- [x] Add a read-only Calendar Meeting Prep view that reuses cache-backed cross-source search to show selected-event context, related mail, and nearby events without provider fetches or mutations.
- [x] Add a read-only Calendar Travel Buffer view that reuses cache-backed cross-source search to show selected-event travel context, cached travel mail, nearby event gaps, and buffer suggestions without provider fetches or mutations.
- [x] Add a read-only Calendar AI Summary view that reuses cache-backed cross-source search plus the configured AI client when available, with deterministic cached fallback and no provider fetches or mutations.
- [x] Add source-aware automation event lanes so existing mail rules run with scoped message identity while calendar change events can enter the lane as read-only groundwork.
- [x] Add a local/cache-backed Calendar Event Edit boundary that proves timezone-safe save/cancel UI through the shared mutation path.
- [x] Add provider-backed Calendar Event Create, Edit, Delete, and RSVP mutation boundaries that write through Google Calendar/CalDAV before cache updates.
- [x] Add endpoint-specific calendar event timezones so flights and other travel events can start in one timezone and end in another across TUI, cache, providers, daemon, and MCP.
- [x] Add Google Calendar source OAuth refresh and provider sync-token persistence so cache-backed event sync can use incremental provider reads without exposing provider tokens to the TUI.
- [x] Add a Gmail API mail source for core OAuth Gmail operations so sync, body reads, mailbox mutations, and send can use provider-aware Gmail API scopes without exposing provider message IDs or labels to the TUI.
- [x] Add CalDAV principal/home-set discovery plus sync-collection incremental reads with calendar-query polling fallback when a provider does not support sync tokens.
- [x] Add selected Calendar Event Edit mutations for attendee lists and this-event recurrence rules while preserving explicit provider conflict and recurrence-scope failures.
- [x] Add selected Calendar Event Edit mutations for reminder overrides while preserving provider save-through/cache-after-success behavior.
- [x] Add a shared calendar-source rail model for calendar views and invitation import pickers that groups calendars by account/provider, filters visible events, preserves scoped refs internally, and renders only user-visible calendar/account labels.
- [x] Add a normalized visible calendar event row layer so Week, Day, 3-Day, Agenda, Search, and invitation flows share date-range filtering, sorting, all-day handling, RSVP markers, calendar colors, and provider-internal redaction.
- [x] Normalize provider date-only and all-day event dates as local calendar days before display, treating provider all-day end dates as exclusive and preserving date anchors across Calendar view switches.
- [x] Add `calendar.week_start` as a profile setting so Week can render Monday-first by default or Sunday-first when the user chooses Apple Calendar-style US week layout.
- [x] Add `calendar.selected_calendars` as a profile-level allow-list of scoped collection keys; the Calendar rail uses it to restore visible calendars after restart while keeping provider URLs, tokens, ETags, and event IDs out of YAML.
- [x] Add a shared calendar notes renderer that converts provider HTML or Markdown notes into terminal-readable text before drawers, inspectors, full detail, command panels, and previews wrap content.
- [x] Add explicit calendar invitation intake from mail previews by parsing `text/calendar` parts and `.ics` attachments into the existing `CalendarEvent` model, then routing create/update/skip decisions through the selected writable calendar source after checking the selected calendar for an existing ICS UID.
- [x] Add optional source/account scoped daemon read filters and MCP listing refs while preserving legacy folder/message-ID compatibility.
- [x] Add source/account/collection/item identity to daemon progress, sync, valid-ID, new-email, and mutation events while preserving legacy event names.
- [x] Add read-only scoped calendar MCP tools for cached agenda, search, and event detail results.
- [x] Add scoped daemon and MCP calendar create/update/delete tools so write callers can pass `EventRef`/`local_id` identity instead of provider-internal IDs alone.
- [x] Add scoped single-message daemon/MCP mail mutation refs so multi-account writes cannot silently target the wrong account while legacy single-account calls still work.
- [x] Add scoped bulk/thread/sender/draft daemon/MCP mail mutation guards so multi-account writes fail clearly without source refs or per-message local IDs.
- [x] Preserve legacy folder/message-ID APIs until daemon, MCP, TUI, and SSH callers can pass scoped refs safely.

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
    │
    ├─ if notifications.new_mail: notify with message or folder deep link
    │
    └─ if notification activated: DeepLinkMsg → route TUI to folder/message/search/compose
```

In Phase 2, the daemon emits a `NewEmailsEvent` on the WebSocket. `RemoteBackend` receives it and forwards it to the same `newEmailsCh`. The app is unchanged.

Sync failures follow the same boundary: a `sync_error` event keeps the existing visible TUI status and additionally asks the notifier for a folder-scoped deep link when `notifications.sync_failures` is enabled. macOS uses `UNUserNotificationCenter` response callbacks to feed the deep link back into the running TUI; Linux and unsupported platforms do not claim activation support.

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

CREATE TABLE email_preview_bodies (
    message_id            TEXT PRIMARY KEY,
    from_addr             TEXT,
    to_addr               TEXT,
    cc                    TEXT,
    bcc                   TEXT,
    subject               TEXT,
    text_plain            TEXT,
    text_html             TEXT,
    is_from_html          INTEGER NOT NULL DEFAULT 0,
    list_unsubscribe      TEXT,
    list_unsubscribe_post TEXT,
    inline_images_json    TEXT NOT NULL DEFAULT '[]',
    attachments_json      TEXT NOT NULL DEFAULT '[]',
    cached_at             DATETIME NOT NULL
);

CREATE TABLE email_embeddings (
    message_id  TEXT PRIMARY KEY,
    source_id   TEXT NOT NULL DEFAULT 'default-mail',
    account_id  TEXT NOT NULL DEFAULT 'default',
    local_id    TEXT,
    embedding   BLOB NOT NULL,   -- float32 array, little-endian
    body_hash   TEXT NOT NULL,
    created_at  DATETIME NOT NULL
);

CREATE TABLE email_embedding_chunks (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    message_id   TEXT NOT NULL,
    source_id    TEXT NOT NULL DEFAULT 'default-mail',
    account_id   TEXT NOT NULL DEFAULT 'default',
    local_id     TEXT,
    chunk_index  INTEGER NOT NULL DEFAULT 0,
    embedding    BLOB NOT NULL,
    content_hash TEXT NOT NULL,
    embedded_at  DATETIME NOT NULL,
    UNIQUE(message_id, chunk_index)
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
