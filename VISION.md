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
- [x] Account-scoped default Compose signature configured in YAML and editable from Settings
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
- [x] Inline image rendering in email previews (bounded side-panel and full-screen raster viewing where supported, with safe links/placeholders otherwise)
- [x] Opt-in remote HTML image reveal in email preview (`o` reveals linked images for the current message without auto-loading remote content)
- [x] Vendor presets (Gmail, Outlook, Fastmail, iCloud — one-line config)
- [x] Background new-email polling
- [x] macOS notifications with Herald deep links for new mail and sync failures
- [x] Unsubscribe from mailing-list emails via `List-Unsubscribe` (`u` in email preview when available)
- [x] Incremental IMAP sync (UIDNEXT-based, instant on no new mail)
- [x] Progressive startup sync UX that visibly refreshes rows and explains when the app is showing a current cache snapshot while live IMAP work continues
- [x] Stream-first folder sync with latest-wins generation invalidation so stale folder loads do not repaint the visible mailbox
- [x] Microbatched Timeline refresh during IMAP sync (`100` changes or `500ms`) so the UI flows without jittering
- [x] Background cache reconciliation (valid-ID ground truth, stale entries removed)
- [x] Cache hygiene invalidation for legacy/incomplete rows with no server UID
- [x] Config-specific SQLite cache paths persisted in YAML so separate account configs do not share one working-directory database
- [x] IMAP IDLE (real push; currently polling only)
- [x] Timeline grouped cleanup replaces the retired top-level cleanup browse surface for sender/domain workflows
- [x] Hide Future Mail sender rule (`h` key; auto-moves future emails to a local folder)
- [x] Custom classification prompts (user-defined categories + data extraction)
- [x] Classification actions (notify, command, webhook, move, archive, delete)
- [ ] Classification action: flag on match (set IMAP `\Flagged`)
- [x] Auto-cleanup rules (per-sender delete/archive older than N days)
- [ ] Multi-account support
- [x] Legacy chat tool calling exists today, but is scheduled for Gollem replacement rather than expansion
- [x] Legacy filtered timeline from chat results exists today, but is scheduled for typed Gollem Timeline intents
- [x] Gollem UI chat-agent replacement with typed search, summary, Timeline, and Compose intents enabled as the default chat runtime
- [ ] Obsidian-friendly email memories with source-backed tracks, contact dossiers, planned company/thread dossiers, Compose Radar nudges, and Markdown vault sync
- [x] Multiple AI backends (Claude, OpenAI-compatible)
- [x] External embedding provider/model selection for OpenAI-compatible vendors, including Settings > AI controls
- [x] Compact AI setup presets with separate chat and embedding role assignments in first-run customization and Settings > AI
- [x] Compose AI assistant baseline (rewrite, tone/length adjustments, subject suggestion, accept into draft)
- [x] Quick replies (canned + AI-generated contextual options)
- [x] Contact book
- [x] First-run setup wizard (detected when no config exists; account type, credentials, AI config steps)
- [x] Settings panel accessible via `S` key (saves to `~/.herald/conf.yaml`)
- [x] Configurable keyboard profiles (Default, Vim, Emacs, Custom) with a central command catalog and git-friendly custom keymap files
- [x] App-level theme system with terminal inheritance, Herald dark/light built-ins, local YAML installs, and Settings-based role editing
- [x] Privacy-safe logging by default, with `-unsafe-logs` as an explicit local opt-in for unredacted diagnostics
- [ ] Keychain integration (passwords stored in OS keychain, not plaintext YAML)
- [x] README with MCP setup prompts for Claude / Cursor / Codex
- [x] Daemon server (`herald serve`, Ollama-style)
- [x] Source installs use the canonical Go module path and `cmd/herald` package so `go install github.com/herald-email/herald-mail-app/cmd/herald@latest` creates a `herald` binary.
- [x] Email rendering package (`internal/render` — independent, testable component)
- [x] Link tracker sanitization (strip UTM, fbclid, mc_cid, etc. from visible labels, terminal hyperlink targets, and opt-in remote image reveal URLs)
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

The TUI uses a title-row tab strip beside the `Herald` title, a collapsible folder sidebar on the left, and a main content area whose layout changes per top-level view.

- [x] Mouse navigation supports top tabs, sidebars, Timeline/Contacts/Calendar rows, and preview wheel scrolling while preserving keyboard parity
- [x] Keyboard layouts with physical-key reporting prefer layout-correct printable characters for Latin/ASCII shortcuts, while Cyrillic and direct Japanese kana physical fallback aliases remain available when terminals do not report `BaseCode`
- [x] Default keyboard profile uses calmer GUI-mail-style preferred shortcuts while preserving literal text entry in Compose, search, prompts, settings, and editor-like fields
- [x] Delete shortcuts use a safe/fast split: `Delete` asks for confirmation, while `Shift+Delete` deletes immediately in browse contexts; `d`/`D` and Backspace variants remain legacy aliases

### Tabs (top-level navigation)
Keyboard (`1`-`3` as the primary visible shortcuts, with `F1`-`F2` supported as function aliases, `F3` as a temporary Contacts alias, and `F4` as a Calendar alias) and mouse clickable from the title row. Compose is a transient writing screen launched from Timeline, not a top-level tab.

- [x] `1` — Timeline: chronological email list with body preview split
- [x] `2` — Contacts: contact book with list+detail panels, keyword and semantic search, LLM enrichment
- [x] `3` — Calendar: source-backed schedule workspace with rail, schedule views, search, detail, RSVP, and provider-backed create/edit/delete flows
- [x] `F3` — Contacts legacy alias while existing muscle memory sunsets

### Timeline View

The primary reading interface. Shows emails sorted newest-first, grouped by thread when multiple messages share the same subject. Selecting a row opens a body preview split on the right.

- [x] Full-width thread list sorted by date
- [x] Thread grouping across participants (fold/unfold inline with `Enter`, reply rows marked visibly)
- [x] Sent replies linked by provider thread IDs or RFC reply headers appear inline with their original active-folder Timeline thread while keeping their real Sent folder identity for preview and mutations
- [x] Sender-cell disclosure markers make collapsed and expanded Timeline threads recognizable before opening them
- [x] Body preview split (right panel, auto-updates on navigation, with bounded inline image rendering when supported)
- [x] Horizontal reading movement: right arrow / `]` opens preview, then moves into it; left arrow moves preview focus back to the list, then folds threads or closes preview and focuses folders
- [x] Intentional unread affordance: `U` marks the current Timeline message unread after inspection
- [x] Full-screen preview (`z`)
- [x] macOS preview printing (`p`) opens the standard print dialog for the loaded email in Original Visual mode or a themed Rendered Markdown mode while preserving remote-image privacy
- [x] Actions: delete, archive, reply, forward
- [x] Preview load telemetry shows the last body load duration/source and logs timing for Timeline previews
- [x] Offline cache policy controls whether previews keep lightweight body text only, non-attachment body data, or full attachment data for offline work
- [x] Changing to a stricter offline cache policy prunes disallowed cached attachment or inline-image bytes while preserving preview text, headers, and attachment metadata
- [x] Settings exposes a manual offline-cache reclaim action that estimates removable preview bytes, explains preserved data, prunes disallowed binary payloads, and compacts SQLite storage
- [x] Background preview prewarming fills the active folder's newest preview-cache misses one message at a time after Timeline data loads
- [x] Bulk selection with `Space` for Timeline delete/archive, including collapsed-thread selection
- [x] Mail-style range selection with `Shift+Up` / `Shift+Down` where terminals support it, plus `V` then `j`/`k` fallback range mode
- [x] Star / pin important threads to top
- [x] Gmail/IMAP drafts are marked directly in Timeline rows and collapsed thread rows, including reply drafts, and `E` opens the draft in Compose for editing
- [x] Read-only virtual `All Mail only` inspector backed by live IMAP folder membership rather than cache guesses
- [x] Reading-first Timeline rows hide spreadsheet-only size/attachment columns, show attachments in the subject cell, use local human dates, and keep Sender/Subject dominant at `80x24`, `120x40`, and `220x50`
- [x] Timeline grouping switch cycles the reading list between default thread, sender, and domain grouping with `G`, preserving Timeline actions and fully covering the cleanup browse workflow
- [x] Timeline sorting cycles with `O` and sortable `Sender` / `Subject` count / `When` headers, showing direction indicators while preserving starred pinning
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
- [x] Global AI status chip reflects startup-detected missing or unreachable Ollama models as `AI down`, disables AI actions until repaired, and keeps repair details available from Settings > AI
- [x] Profile-aware command layer: `1/2` are the advertised tab shortcuts, `Alt+1/Alt+2/Alt+3` are optional aliases, `F1/F2` mirror those tabs, `F3` remains a temporary Contacts alias, Default prefers GUI-mail-style shortcuts, Vim preserves `h/j/k/l`, and text fields keep printable input including `?`, `/`, and macOS Option-generated characters
- [x] Timeline key hints advertise `Tab` / `Shift+Tab` panel switching whenever the bottom bar has room for navigation help
- [x] Context-sensitive shortcut help overlay opens with `?` in browse and non-text contexts, lists every relevant key for the current tab, pane, overlay, and Compose mode in a compact centered modal over the current view, keeps editable Compose fields free to type literal `?`, and keeps semantic search available through `/` with a `? query` prefix
- [x] Modifier-aware key hints: when the terminal reports Shift, Ctrl, or Alt key state, the bottom hint bar temporarily pivots to existing commands for that modifier without changing shortcut behavior; terminals without key-release support fall back to a brief modified-keypress hint
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
- [x] Multi-account sessions show Mail.app-style Favorites plus per-account folder sections, with single-account sessions keeping the legacy folder tree
- [x] Multi-account folder navigation keeps a bounded, scrollable sidebar, skips non-selectable section headers, preserves visible selection styling, and labels account rows as `Name (email)` where the address is known
- [x] Startup recovers the last known per-account folder tree from the cache before live IMAP folder listing completes, while live IMAP status remains the only source of unread/total counts

### Chat Panel

The chat panel is a right-side slide-out (`g` key, with `c` retained as a legacy alias outside Timeline) that will become Herald's UI-only email agent surface. The current hand-written Ollama `/api/chat` loop is legacy scaffolding, not the roadmap foundation; the replacement path uses a Gollem runner that treats Ollama as only one possible local provider while explicit global AI disable keeps chat unavailable.

- [x] Slide-out panel (`g` key, with `c` retained as a legacy alias outside Timeline)
- [x] Conversation history (multiple turns)
- [x] Markdown rendering of assistant responses (glamour)
- [x] Context: currently open email available to the model
- [x] Context: active folder and selection state passed to model
- [x] Remove legacy in-process chat tools for search, sender stats, and threads from the chat runtime
- [x] Remove prose `<filter>` block parsing from chat result handling
- [x] Replace the legacy Ollama chat loop with a Gollem-only default chat runner before adding risky chat capabilities
- [x] Remove the legacy chat tool registry and `<filter>` response parser once Gollem can return a plain assistant reply and a typed Timeline intent
- [x] Keep the chat drawer as the user-facing entry point while moving agent execution into an `internal/agent` boundary
- [x] Use typed Gollem outputs for `reply`, `timeline_intent`, `summary`, and `compose_intent` instead of prompt-injected control markup
- [x] Make chat a UI-mode feature only; do not add daemon, MCP, or background-agent entrypoints in the first Gollem iteration
- [x] Do not allow chat to send, delete, archive, or mutate calendar events in the first Gollem iteration

#### Gollem replacement roadmap

The next chat architecture uses Gollem agents and typed tools. The model can search, inspect, summarize, and propose UI intents, while Bubble Tea remains the only layer allowed to mutate Timeline or Compose state.

- [x] Add a Gollem provider factory for `ollama`, `anthropic`, `kimi`, and `fireworks`
- [x] Add `find_emails` with keyword, semantic, hybrid, current-folder, unread, sender, date-hint, and result-limit parameters
- [ ] Extend `find_emails` with cross-folder search and semantic score disclosure in tool rows
- [x] Add `get_email_context` for bounded subject/sender/date/body-snippet context by message ID
- [x] Add `summarize_email_set` for search-result summaries with cited message IDs, involved people, dates, open questions, and action items
- [x] Add `explain_people` to identify senders and likely owners from a bounded email set
- [ ] Extend `explain_people` with recipients, mentioned people, and organizer inference when richer cached context is available
- [x] Add `TimelineIntent` so chat can show explicit selections, keyword results, semantic results, or hybrid results in Timeline
- [x] Add `ComposeIntent` so chat can propose subject/body edits when Compose is active, routed through the existing review/accept flow
- [ ] Add progress states for `searching`, `reading`, `summarizing`, and `draft edit ready` so long-running chat requests do not look frozen
- [x] Add deterministic provider-contract tests for plain reply, one tool call, two-step search-plus-summary, typed Timeline intent, typed Compose intent, malformed args, and provider failure
- [ ] Add live provider smoke results for Ollama/local, Anthropic, OpenAI, Kimi, and Fireworks before publishing provider recommendations

#### Filtered timeline

When the Gollem agent returns a Timeline intent, the TUI pushes those results into the Timeline tab as a live filtered view. The user can browse and act on the messages without leaving the chat flow, and `Esc` or "show all" restores the full timeline.

- [x] `explicit_ids` shows exactly the message IDs returned by the agent after local validation
- [x] `keyword`, `semantic`, and `hybrid` intents reuse the existing Timeline search pipeline instead of duplicating ranking logic in the agent
- [x] Result labels appear as compact Timeline state such as `Agent: onboarding issue`
- [x] Empty explicit result sets show a bounded notice and keep the previous Timeline recoverable
- [ ] Large result sets are capped, deterministic, and disclose how many messages were searched versus summarized

#### Provider policy

Gollem is the chat-agent framework. Ollama/local, Anthropic, OpenAI, Kimi, and Fireworks are providers behind Gollem, not separate Herald chat architectures.

| Provider | Gollem route | First use |
|---------|-----|-------------|
| Ollama/local | Gollem OpenAI-compatible/Ollama provider | Offline local agent experiments |
| Anthropic | Gollem Anthropic provider | Highest-quality remote reasoning and tool use |
| OpenAI | Gollem OpenAI-compatible provider | Global OpenAI-configured chat and hosted model baseline |
| Kimi | Gollem OpenAI-compatible provider | Long-context or Kimi-specific model experiments |
| Fireworks | Gollem OpenAI-compatible provider | Hosted open-model and Kimi variants |

- [x] Config selects one Gollem chat provider and model for UI chat without changing the existing embedding provider contract
- [x] Ollama remains supported for local chat through Gollem only; the old direct Ollama chat loop is removed
- [x] Provider-specific quirks are isolated in the Gollem provider factory, not scattered through Bubble Tea update logic

---

## Obsidian-Friendly Memories

Herald Memories turns important email history into local, source-backed tracks that can be used in chat, Contacts, and Compose without locking the user into Herald. The product wedge is "Obsidian-friendly email memories": machine-readable enough for retrieval, human-readable enough to live as editable Markdown in an existing vault.

- [x] Add a dedicated roadmap spec for Obsidian-friendly email memories at `docs/superpowers/specs/2026-06-06-obsidian-friendly-email-memories-roadmap.md`
- [x] Treat the Gollem chat-agent replacement as the required foundation before adding memory-aware chat tools
- [x] Add immutable local memory records under `~/.herald/memories` by default with JSON evidence, prompt version, confidence, freshness, and Obsidian target metadata
- [x] Add default memory configuration for source folders, per-memory destinations, prompt templates, update rules, confidence thresholds, and Obsidian output profile modes
- [x] Add deterministic cached-mail extraction for last contact, last user reply, open questions, commitments, deadlines, and job/work track status from Inbox and Sent body snippets
- [x] Add read-only Gollem memory tools for memory search, contact history, company tracks, open loops, and reply-prep context
- [x] Add reusable track lifecycle assembly for active, waiting, stale, resolved, backlog, and done views over immutable source-backed memory records
- [x] Add Compose Radar v1 as a compact source-backed reply context panel with at most three high-confidence nudges and no draft mutation
- [x] Refresh Compose Radar after reply recipient, subject, or body changes with debounce and stale-result protection
- [x] Add typed Compose Radar nudge metadata for conflicts, callbacks, open loops, relationship context, research updates, draft risks, action state, and dismissal scope
- [x] Add Obsidian preview/merge rendering that preserves user-authored note sections and supports minimal YAML, hidden YAML, link, and tag modes
- [x] Build local email memories from cached Inbox, Sent, thread headers, contacts, body snippets, classifications, and semantic embedding/cache signals
- [x] Store every memory with evidence pointers to email message refs, folders, dates, snippets, note paths, or research URLs
- [x] Validate evidence pointers by source type, including email, sent replies, Obsidian notes, calendar events, attachments, and research URLs, while bounding snippets so memory files do not copy full private content
- [x] Add memory-aware chat tools for contact history, company status, related replies, open loops, and "what should I remember before replying?"
- [x] Add explicit local Compose Radar actions for open source, dismiss, insert, resolve, save memory, and research person/company intent
- [x] Add Contact dossiers that combine local email memories, active tracks, Obsidian links, and source evidence
- [x] Add Company dossiers that combine local email memories, active tracks, Obsidian links, and optional web research placeholders
- [x] Expose memory configuration with strong defaults for vault targets, generated sections, Obsidian output profile, prompt templates, update rules, and safe research behavior
- [ ] Add opt-in Obsidian sync that writes and updates Markdown notes under configured `People/`, `Job search/`, and `Scheduled Task Artifacts/` folders while preserving user edits
- [ ] Add explicit Research Mode for person/company enrichment that never sends private email content to external research queries by default
- [ ] Add a daily memory briefing that updates track status, open questions, and stale threads as a diff rather than a full inbox recap

---

## AI Classification

The app can automatically tag emails with categories (subscription, important, unnecessary, etc.) using a local Ollama model. Classification runs in the background after sync — it never blocks the UI. The `t` key triggers a full classification pass on the current folder, while `T` re-classifies the focused email.

- [x] Background classification via Ollama (`t` key for folder pass, `T` for current-message re-classification)
- [x] Category tags stored in SQLite (`email_classifications` table)
- [x] Tag column visible in Timeline, including sender/domain grouped cleanup workflows
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
- [x] Rule editor TUI form in `Settings > Sync & Cleanup` — add/edit rules interactively
- [x] `classification_actions` section in `~/.herald/conf.yaml` — declarative config format (current: DB-only)
- [x] Auto-classify new emails as they arrive to trigger rules in real time
- [x] Dry-run mode: `--dry-run` flag logs what actions would fire without executing them
- [x] Rule overlay explains that automation rules handle future matching mail, shows saved rules in the same compact centered overlay, and tells the user where the action results surface
- [x] Rule editor dry-run preview shows matched cached messages and planned actions before users save or enable move/archive/delete automation
- [x] Automation events carry source/account identity so mail rules stay scoped per source and future calendar change events can enter the lane without enabling calendar mutations

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

## Cleanup Via Timeline

Cleanup browsing now lives in Timeline instead of a separate top-level view. The `G` grouping switch lets users review mail by thread, sender, or domain in one list, then use the same Timeline selection, preview, delete, archive, unsubscribe, and hide-future-mail controls.

### Sender / Domain grouping

- [x] Group Timeline rows by sender with `G`
- [x] Group Timeline rows by domain with `G`
- [x] Delete or archive a highlighted sender/domain group through Timeline destructive actions
- [x] Bulk-select grouped Timeline rows with `Space` before delete/archive
- [x] Confirmation copy names sender/domain groups rather than calling them threads
- [x] Sender/domain grouping uses the same preview, reply, forward, attachment, unsubscribe, and hide-future-mail behavior as normal Timeline rows
- [x] Top-level Cleanup browse tab, Cleanup preview, Cleanup row selection, Cleanup mouse flows, and direct `W`/`P`/`C` browse shortcuts are intentionally retired

### Unsubscribe

Unsubscribe and sender-hiding actions should be visible from the open email preview itself so the user does not have to remember hidden keybindings. `u` acts on the current email's mailing-list headers, while `H` acts on the sender and keeps future mail out of the inbox without pretending to be a real unsubscribe.

- [x] Preview metadata shows explicit `Tags:` and `Actions:` rows so list/sender actions are visible in context
- [x] `u` unsubscribes the currently open Timeline preview email when it exposes `List-Unsubscribe`
- [x] `H` hides future mail from the currently open email's sender by moving new mail to `Disabled Subscriptions`
- [x] Timeline sender/domain groups expose `H` for the highlighted sender while `u` remains tied to an open message preview with real unsubscribe headers
- [x] `u` performs RFC 8058 one-click POST when `List-Unsubscribe-Post` is available
- [x] `u` falls back to `List-Unsubscribe` mailto handling when the message only exposes an email-action target
- [x] `u` falls back to opening a `List-Unsubscribe` browser URL for HTTP links
- [x] Track whether emails keep arriving after unsubscribe; notify / prompt if they do
- [ ] Batch unsubscribe flow: present list of detected subscriptions, select, choose mode, execute

### Auto-Cleanup Rules

Rules let the app automatically act on email from known senders — delete newsletters older than 30 days, archive promotional email weekly, etc. Rules are defined per-sender or per-domain, stored in SQLite, and managed from `Settings > Sync & Cleanup` rather than a browse tab.

- [x] Per-sender / per-domain rules (action + older-than-days condition)
- [x] Rule storage in SQLite (`cleanup_rules` table)
- [x] Manual rule execution (`run_cleanup_rules` trigger)
- [x] Scheduled execution (configurable interval in `~/.herald/conf.yaml`; TUI-only — daemon runs rules on-demand via `/v1/cleanup-rules/run`)
- [x] TUI rule manager (list, add, remove) launched from `Settings > Sync & Cleanup`
- [x] MCP tools: `list_cleanup_rules`, `create_cleanup_rule`, `dry_run_cleanup_rules`, `run_cleanup_rules`
- [x] Cleanup rule manager appears as a compact centered overlay and explains what manual vs scheduled cleanup does, where saved rules live, and how run results become visible
- [x] Cleanup rule dry-run preview shows selected/all rule matches and requires confirmation before live archive/delete execution

---

## Compose and Reply

Write in Markdown, deliver as properly formatted HTML email. Compose is a transient full-screen editor launched from Timeline with `Ctrl+N` (and legacy `c`), contextual reply/forward/draft actions, or quick replies; `Esc` returns to the screen that initiated it after local Compose transient state is dismissed.

- [x] Markdown editor (textarea)
- [x] Timeline `Ctrl+N` opens a blank Compose screen for a new message; `c` remains a legacy alias
- [x] Live Markdown preview (`Ctrl+P`)
- [x] External editor handoff (`Ctrl+X`) writes the Compose body through `$VISUAL` or `$EDITOR` and restores the edited text when the editor exits
- [x] Send as multipart HTML + plain-text via SMTP
- [x] Reply (`Ctrl+R` sender-only and `Ctrl+Shift+R` reply-all keys — pre-fill To, Re: subject, quotes original; `r`/`R` remain legacy aliases)
- [x] Loaded preview headers show visible `To` and `Cc` recipients when a message body includes them, so reply-all participants can be checked before composing
- [x] Forward (`Ctrl+F` key — pre-fills Fwd: subject, forwarding header, body quote; `f` remains a legacy alias)
- [x] Attachment support: attach files (`Ctrl+A`), attach list shown in compose
- [x] Send with attachments (`multipart/mixed`)
- [x] Plain draft entry is safe: digits, letters, `q`, `/`, `?`, and Option-generated characters type into the focused Compose field; global tab/log/chat/sidebar/refresh commands do not steal printable text while composing
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

While composing, the configured AI model acts as an inline writing assistant. The assistant defaults open as a compact command bar between the header fields and body, with Translate and Style dropdowns, quick actions, undo, and a small inline custom-instruction field in the same row. When a rewrite returns, Compose enters a review mode where the cleaned editable suggestion replaces the main body editor until the user accepts or dismisses it, after which the command bar remains available.

- [x] **Default-open AI bar** — Compose shows the compact AI command bar immediately; `Ctrl+K` focuses the inline instruction field
- [x] **Disabled AI warning** — when no AI provider is configured, Compose shows an `AI disabled` warning in the bar instead of active rewrite controls
- [x] **Spell / grammar fix** (`Ctrl+F`) — rewrites the current draft to fix typos, grammar, punctuation, and clarity issues while preserving meaning
- [x] **Translation dropdown** (`Ctrl+T`) — opens a language menu in the AI bar and rewrites the current draft into the selected language
- [x] **Style dropdown** (`Ctrl+Y`) — opens a style menu in the AI bar and rewrites the current draft in the selected style
- [x] **Undo accepted rewrite** (`Ctrl+Z`) — restores the body from before the last accepted AI suggestion
- [x] **Subject line suggest** (`Ctrl+J`) — when a draft or reply context exists, offer a concise subject hint that can be accepted with `Tab`
- [x] **Length adjust** — AI bar quick actions can shorten the draft to key points or expand it with useful context
- [x] **Freeform instruction chat** (`Ctrl+K`) — the user can type natural-language directions inline, such as "make this warmer and translate it to Spanish", and press `Enter` for an editable rewrite
- [x] **Review-in-place rewrite** — AI suggestions occupy the main editor slot instead of being appended below it; `Tab` toggles Original/Suggestion while reviewing
- [x] **Clean review surface** — AI review strips `Current draft:` prompt echoes, uses readable word-level Changes without fake pager hints, and keeps the action row attached to the Changes box

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
- [x] Shared Markdown-aware rendering across Timeline, Contacts, grouped cleanup rows, and full-screen received-email previews
- [x] Inline image text placeholders (`[image: type  size]` in split view, `[Image: AI description]` when vision model available)
- [x] Remote HTML image links render as readable OSC 8 links without auto-fetching remote image bytes
- [x] Full-screen inline image viewing uses bounded iTerm2 or Kitty rendering when supported, or localhost OSC 8 links for local MIME image bytes in local TUI sessions
- [ ] AI vision image descriptions — use a vision-capable model (e.g. gemma3:4b, gpt-5-mini, claude) to generate a one-line description for each inline image on demand. Show as `[Image: A promotional banner showing...]` instead of raw MIME type. Generate lazily when email is opened, cache in SQLite. Requires `HasVisionModel()` on the classifier.
- [x] Kitty graphics protocol support with Ghostty autodetection and `-image-protocol` override for non-iTerm2 terminals

---

## Email Rendering & Link Processing

Email body rendering and link handling live in `internal/render`, a standalone package with no TUI dependency. This makes it testable independently and reusable across all surfaces: TUI, MCP server, daemon API, and SSH mode. The package handles text wrapping (ANSI-aware), URL linkification (OSC 8 terminal hyperlinks), and link sanitization.

### Rendering component (`internal/render`)
- [x] `WrapText` / `WrapLines` — ANSI-aware text wrapping that correctly skips escape sequences when counting visible width
- [x] `StripInvisibleChars` — removes zero-width joiners, BOM, and invisible spacers that HTML emails embed
- [x] `LinkifyURLs` / `LinkifyWrappedLines` — converts raw URLs to OSC 8 clickable terminal hyperlinks with shortened labels
- [x] `ShortenURL` — produces human-readable labels like `example.com/path…` for long URLs
- [x] OSC 8 link hover affordance — when terminal mouse motion is available, hovered links brighten in place and the status bar shows a shortened sanitized destination preview
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
- [x] `/` key opens search bar in Timeline
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
- [x] Cross-source search blends cached mail and calendar event results in one read-only view while preserving Timeline and Calendar Search behavior
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
- [x] Embeddings via local Ollama or OpenAI-compatible providers, with `nomic-embed-text-v2-moe` as the local default and `text-embedding-3-small` as the OpenAI-compatible default
- [x] Vectors stored in SQLite (`email_embeddings` table)
- [x] Cosine similarity ranking
- [x] `semantic_search_emails` MCP tool
- [x] Similarity score badge (`87%`) per result row
- [x] Embeddings are invalidated automatically when the configured embedding provider or model changes
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

### Notifications and deep links
Notifications should bring attention back to the right mailbox context without turning every background action into noise. The first target is macOS native notifications with Herald deep links, while other platforms must degrade cleanly.

- [x] Native macOS notifications for new mail and sync failures are enabled by default.
- [x] Notification activation can open folder, message, sender, search, or compose contexts through `herald://mail/...` links.
- [x] Deletion/archive completion, classification completion, and chat-result notifications are supported but disabled by default.
- [x] Linux keeps delivery-only notification behavior and unsupported platforms no-op without misleading click-through claims.

---

## Multi-Account Support

The app currently supports one IMAP account per config file. Multi-account support will allow managing several inboxes (e.g. personal + work) in a single session.

- [x] `sources:` list in `~/.herald/conf.yaml` supports multiple mail sources while the current single-account format still works
- [x] Active account switching uses one backend connection per mail source and restores an account-scoped folder tree
- [x] Source identity foundation (`source_id`, `account_id`, and scoped message refs) lands before multi-account UI
- [x] Work coordinator policies preserve latest UI intent, coalesce duplicate resource fetches, serialize source mutations, and keep background work fair
- [x] Cache-first mail body and preview services use scoped message refs, coalesce duplicate provider fetches, replay safe completed work, and keep direct provider reads behind explicit `NoCache` methods
- [x] Current IMAP provider access is isolated behind `IMAPMailSource` before visible account switching
- [x] Folder sidebar exposes an account rail and account-scoped folder tree when multiple mail sources are configured
- [x] Status bar shows active account name when multiple mail sources are configured
- [x] Compose "From" field lets user pick sending account and routes sends/drafts through that account
- [x] Unified Timeline and search view across accounts (opt-in) shows account badges and routes selected-message reads/writes by scoped refs
- [x] Gmail OAuth is the supported default Google mail onboarding path, with app-password Gmail retained as the non-OAuth Gmail fallback
- [x] `provider: gmail` OAuth mail uses the Gmail API source and narrower Gmail API access for core sync, body reads, mailbox mutations, and send; `provider: gmail_api` remains a compatibility alias
- [x] Gmail API draft create/list/delete/send parity supports Herald autosave, draft lists, edit, discard, and direct send flows
- [x] Gmail API history polling uses cached provider cursors for delta sync and falls back to bounded list/get when the cursor is missing or expired
- [x] Gmail API list/draft/history reads paginate, retry bounded 429/5xx responses, and Compose sends preserve CC/BCC plus attachment MIME parts through the API path
- [ ] Outlook OAuth
- [x] Vendor presets: `protonmail`, `gmail`, `outlook`, `fastmail`, `icloud`

---

## Calendar Sources

Calendar sources extend Herald's source platform beyond mail while keeping provider-specific sync details out of the TUI. The current milestone includes source-scoped sync, search, and provider-backed create/update/delete mutations that update Herald's cache only after the provider succeeds.

- [x] `CalendarSource` capability shared by Google Calendar and CalDAV providers
- [x] Source-scoped calendar and event cache with provider freshness metadata such as ETag, revision, or sync token
- [x] Read-only Google Calendar and CalDAV adapters list calendars/events and fetch event details against deterministic local provider harnesses before live-provider checks
- [x] Read-only cache-backed agenda and event detail view before RSVP, edit, or create flows
- [x] Read-only Day Agenda + Drawer view with Agenda/Day switching and selected-event context before Week view or mutations
- [x] Read-only Week Time-Grid view with selected-event inspector before calendar search or mutations
- [x] Read-only 3-Day Command view with next-up, conflict, and open-slot summaries before calendar search or mutations
- [x] Full read-only Event Detail shows attendees, RSVP state, recurrence, attachments, local time, event timezone, and an alternate timezone before calendar search or mutations
- [x] Calendar search view before RSVP, edit, or create flows
- [x] Cross-source search view blends cached mail and calendar event results before command-center summaries or mutations
- [x] Meeting Prep view opens from Calendar Event Detail and blends the selected event with related cached mail and nearby cached events without provider fetches or mutations
- [x] Travel Buffer view opens from Calendar Event Detail and blends the selected event with cached travel-related mail and nearby event gaps without provider fetches or mutations
- [x] AI Summary view opens from Calendar Event Detail and summarizes the selected event, cached related mail, and cached nearby events without provider fetches or mutations
- [x] Local/cache-backed Event Edit form with explicit save/cancel state and timezone preview
- [x] Provider-backed Event Edit saves write through Google Calendar/CalDAV before updating cache, and provider failures keep unsaved edits visible
- [x] Provider-backed Event Create opens from Calendar on a writable collection, lets the Calendar field choose any writable configured calendar with current-account calendars listed first, falls back from read-only browse/filter selections, and writes through Google Calendar/CalDAV before adding cached rows
- [x] Provider-backed Event Delete requires confirmation, writes through Google Calendar/CalDAV before invalidating cached rows, and is available from Calendar browse/detail/edit contexts
- [x] Event Create/Edit supports different start and end timezones for travel events and renders HTML/Markdown notes in the live preview
- [x] Event Create/Edit uses cursor-aware modal fields plus keyboard pickers for timezones, attendees, recurrence, reminders, and date selection without stealing text-entry shortcuts
- [x] RSVP response changes write through Google Calendar/CalDAV before updating cached attendee state
- [x] Provider mutation conflicts and unsupported recurrence scopes fail visibly without rewriting cached event rows
- [x] Event Edit can mutate attendee lists and this-event recurrence rules through the existing provider save-through flow
- [x] Event Edit can mutate reminder overrides through the existing provider save-through flow
- [x] Calendar create/update/delete routes are exposed through the daemon and MCP tools for agent-driven calendar workflows
- [x] Google Calendar source using OAuth and provider sync tokens
- [x] Google Calendar account setup uses Herald's supported Google OAuth flow instead of asking users to configure Google's CalDAV URL with an app password
- [x] CalDAV source using discovery, ETag, and sync-token or polling fallback
- [x] CalDAV account setup offers provider-specific guidance for Fastmail, iCloud, and Yahoo app-password flows while keeping Proton Calendar and Microsoft Calendar out of the basic CalDAV preset list.
- [x] Calendar screens `01` through `04` match the reference mockups closely enough to require side-by-side real-app Sonokai Signal screenshots in implementation reports.
- [x] Calendar Week, Day, 3-Day, Agenda, Search, and invitation picker surfaces share an Apple Calendar-style multi-account calendar rail with colored calendar toggles.
- [x] Calendar range headers clearly state the active day, week, 3-day window, or agenda range and advertise arrow movement first while preserving `h/l` aliases.
- [x] Calendar Agenda uses whole calendar-month windows, and Week uses Monday-Sunday windows, instead of rolling ranges from the current day.
- [x] Calendar Agenda hides events before the current local day by default, while exposing a reversible show-past affordance and preserving whole-month range navigation.
- [x] Calendar date-only and all-day provider events render on their intended local calendar dates, with exclusive all-day end dates kept out of the following day.
- [x] Calendar view switching preserves the selected/current date across Agenda, Day, Week, and 3-Day instead of jumping to the first cached event.
- [x] Calendar week start can be changed in Settings, with Monday as the default and Sunday available for Apple Calendar-style US layouts.
- [x] Calendar rail mini-month bolds days with visible events while leaving empty days at regular weight.
- [x] Calendar mouse navigation supports mini-month day selection, event selection/detail opening, and calendar rail toggles, with the visible-calendar selection persisted in YAML.
- [x] Calendar notes render HTML and Markdown into readable terminal text instead of exposing raw tags.
- [x] Calendar RSVP is explicit through accept, tentative, and decline actions, and events requiring a response are visibly highlighted.
- [x] Emails with `text/calendar` parts or `.ics` attachments expose a preview-header Create Calendar Event action, ask which configured writable calendar should receive the invitation when more than one exists, and offer update or skip when the selected calendar already has the same ICS UID.

---

## Contact Book

Contacts are derived from To/From/CC headers seen in sent and received mail — no import required. They power name completion in Compose and will feed into chat context.

- [x] `contacts` table in SQLite (built from To/CC/BCC/From headers during IMAP sync)
- [x] Name, email, company, topics, first/last-seen, email/sent counts per contact
- [x] LLM enrichment via Ollama: extracts company name and discussed topics from email subjects (`e` key)
- [x] Semantic contact embeddings for natural-language search
- [x] Tab 2 — Contacts TUI: two-panel list+detail, `/` keyword search, `/` then `? query` semantic search
- [x] Apple Contacts import via native Contacts.framework API at startup (darwin+cgo only, read-only name merge)
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

- [x] `f` key in Timeline opens Compose pre-filled for forward (Fwd: subject, quoted body; `F` remains a legacy alias)
- [x] `r` / `R` keys in Timeline open Compose pre-filled for reply-all or sender-only reply (Re: subject, quoted body, To pre-filled)
- [x] Reply/forward Compose shows a preserved-content summary with HTML, inline image, attachment, and preservation-mode status
- [x] Reply/forward Compose separates the editable response from a read-only original-message preview so users can keep source context visible while writing
- [x] `d` / `Backspace` in Timeline prompts before deleting the highlighted email, selected messages, or collapsed thread
- [x] `D` / `Shift+Backspace` in Timeline immediately deletes the highlighted email, selected messages, or collapsed thread
- [x] `Space` in Timeline selects messages for bulk delete/archive; collapsed thread rows select the represented thread
- [x] `Shift+Up` / `Shift+Down` and `V` range mode extend Timeline bulk selection across visible rows
- [x] `d` / `e` in Timeline act on selected messages when selection exists, with confirmation copy showing selected counts
- [x] `d` on a draft uses discard-draft confirmation copy, while `E` is advertised as the explicit edit-draft command and `Ctrl+S` as the explicit send-draft command
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

Bubble Tea's alt-screen captures input and mouse events, so Herald needs explicit copy paths that work even when the terminal cursor is hidden. Text selection should be visible, keyboard-first, and still leave terminal-native selection available as a fallback.

- [x] `m` toggles mouse-selection mode (releases mouse capture; status bar indicator)
- [x] `m` restores TUI mouse capture after temporary terminal-native text selection
- [x] `m` uses the same release/restore behavior from Calendar so calendar users can temporarily use terminal-native text selection.
- [x] First `v` in read-only email previews shows a visible row/column cursor without starting a text range
- [x] Second `v` anchors the current cursor position and starts/stops range selection from there
- [x] `h/j/k/l` and arrow keys move the preview cursor by character or line, extending the selected range only after selection has started
- [x] `y` copies the selected preview range to the system clipboard, preferring rich HTML/image payloads where the platform supports them and plain text everywhere else
- [x] `Esc` cancels preview cursor/selection mode before closing the enclosing preview
- [x] `yy` copies current line; `Y` copies entire visible body
- [x] Contacts inline previews and Compose original-message previews share the same visible selection behavior as Timeline split and full-screen previews
- [x] Read-only Timeline and Contacts email previews support in-app mouse drag selection across visible header rows, email addresses, and body text; `y` copies the highlighted range.

---

## Full-Screen Email View

- [x] `z` (or `Enter` when preview is already open) expands preview to fill the terminal
- [x] Tab bar, sidebar, timeline table hidden in full-screen
- [x] Header (From / Date / Subject) pinned at top
- [x] Same scroll controls (`j`/`k`, `PgUp`/`PgDn`)
- [ ] Native inline images clip to the visible preview bounds while scrolling so partially visible images appear from the top or bottom without overflowing
- [x] `z` or `Esc` exits and restores split layout

---

## Settings / Onboarding Screen

First-run experience and ongoing configuration should not require the user to edit a YAML file. A TUI settings screen lets users configure accounts, server details, AI, and sync preferences interactively. The YAML file remains the source of truth on disk — the settings screen reads and writes it.

### First-run wizard
- [x] Detected on startup when the config file is missing or empty / whitespace-only
- [x] Herald-styled setup shell with recommended, supported, and fallback account messaging and the same minimum-size guard used by the main TUI
- [x] Step 1 — Account Type chooser remains first: Gmail OAuth, Standard IMAP, Gmail App Password, Proton Mail Bridge, Fastmail, iCloud, and Outlook are visible before provider details
- [x] Step 2 — Provider express paths: Gmail OAuth opens a compact Google account step with optional email identity, Mail enabled, and Google Calendar enabled by default; credential providers keep editable host/port fields where relevant
- [x] Connection gate: Google OAuth verifies selected Google access through one browser flow; credential-based providers use generic `Verify access` copy while retaining protocol-aware validation internally
- [x] Gmail setup copy links directly to Google docs for IMAP access, third-party client setup, and App Password generation
- [x] Gmail OAuth is available by default as a browser-based path; Homebrew/release binaries include OAuth defaults, while source builds require configured Google OAuth credentials to run OAuth
- [x] Gmail OAuth writes and normalizes `provider: gmail` to the Gmail API mail source for core mail operations using the Gmail API `gmail.modify` OAuth scope
- [x] `Settings > Accounts` and first-run setup validate selected mail/calendar provider access before saving or applying account changes; normal startup for existing configs still opens cached/offline data when live connectivity is unavailable
- [x] Gmail OAuth setup treats browser consent as a candidate config, validates Gmail API read/send capability before saving, and makes Google cancel/timeout states explicit
- [x] Back navigation: `Esc` and `Shift+Tab` can return to previous first-run wizard screens without being blocked by required-field validation on the current screen
- [x] Step 3 — Enter or customize: after account validation, first-run shows a compact Advanced settings review with AI off/deferred, cache set to message bodies without attachments, keyboard Default, theme Inherited/Default, and empty signature, then offers `Enter Herald` with `Customize setup`
- [x] AI setup defaults to quality-first local models (`gemma3:4b` and `nomic-embed-text-v2-moe`), warns that 16GB RAM is comfortable while 8GB can work more slowly, and keeps custom Ollama downgrade choices plus freeform model names
- [x] First-run Ollama setup validates that the selected chat/classification and embedding models are installed before saving; missing models show exact `ollama pull` commands and keep the config unwritten
- [x] Offline Cache choices use compact labels for lightweight previews, message bodies without attachments, and full offline archives
- [x] Theme step shows a current-theme picker for inherited, built-in, and installed themes; local YAML install stays in Theme Selection and semantic role editing stays in Theme Editor
- [x] Advanced Sync & Cleanup preferences such as poll interval, IMAP IDLE, reclaim, and auto-cleanup stay out of first-run onboarding and remain available in in-app Settings
- [x] Final save writes `~/.herald/conf.yaml` only after the account connection gate has passed

### In-app settings panel
- [x] Accessible from the TUI with `S` key as a compact centered overlay over the current screen; it fits at `80x24` and falls back to the minimum-size guard at `50x15`
- [x] Top-level category menu for `Accounts`, `Calendar`, `AI`, `Sync & Cleanup`, `Keyboard`, `Theme Selection`, `Theme Editor`, and `Signature` so users can change one settings area without stepping through unrelated fields
- [x] Editable fields for ALL config sections: credentials, server, SMTP, AI, sync (basic fields only done)
- [x] AI settings expose curated Ollama chat and embedding model recommendations, including downgrade guidance for constrained machines and a translation-quality warning for `llama3.x` choices
- [x] AI settings expose OpenAI-compatible embedding provider and model controls, including external embeddings for Claude/OpenAI chat and optional local Ollama embeddings for external chat
- [x] AI settings warn when a previously configured Ollama model is no longer installed or reachable, disable AI actions, show install commands, and offer a Save Disabled action without blocking cached/offline startup
- [x] Sync & Cleanup includes an explicit reclaim action for preview-cache storage with a before/after byte estimate and confirmation before pruning
- [x] Sync & Cleanup defaults to message bodies without attachments and keeps Offline Cache policy labels compact
- [x] Theme Selection switches between inherited, Herald dark, Herald light, and installed YAML themes and installs local YAML files; Theme Editor edits semantic color roles with swatches, xterm-256/hex inputs, xterm-grid and RGB color pickers, live preview, reset controls, and save-as-new-theme support
- [x] `Settings > Accounts` lists configured mail/calendar accounts plus first-class `Add account` and `Add calendar only` rows on one manager page
- [x] `Settings > Accounts` can add mail accounts, add standalone calendar sources, and pair supported mail providers with calendars without a separate add-type submenu
- [x] Gmail OAuth account setup can add Gmail and Google Calendar in one flow, defaulting calendar on, deriving calendar identity from the Gmail address, and validating both sources before saving
- [x] `Settings > Accounts` exposes Google Calendar through the supported Google OAuth path and keeps CalDAV provider presets as username/password alternatives
- [x] `Settings > Accounts` shows short clickable setup links for provider-specific CalDAV app passwords without exposing saved calendar passwords.
- [x] `Settings > Accounts` can disconnect configured accounts/sources and purge their local cache without deleting provider-side mail or calendars
- [ ] Account reorder remains planned after add/remove/account detail management
- [x] Category saves write the config, apply supported runtime updates, and return to the settings menu; menu hints say `enter open` and `esc exit`, and `Esc` unwinds filter/category state before exiting without saving unsaved edits
- [x] `Settings > Accounts` saves validate selected mail/calendar provider access before replacing the active config/backend/source graph; failed validation leaves the previous account active and shows a compact error modal
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
- [x] v0.6 demo tapes cover Calendar workspace, invitation import, preview selection/image reveal, and preserved reply Compose flows
- [x] High-resolution themed showcase tapes cover Dark Pastel, Red Alert, and softer dark terminal profiles without multiplying every docs capture
- [x] Demo media can opt into a compact keypress overlay with `--demo-keys` so presentation GIFs show the shortcuts being pressed
- [x] `[DEMO]` indicator in the status bar so the user knows they are not connected to a real account
- [x] Demo mode can launch a deterministic multi-account mailbox for account-switching visual checks without live IMAP
- [x] Demo TUI startup shows a compact dismissible welcome overlay that points users at the first onboarding email before they explore freely
- [x] `Step 5: View inline images in full screen` Herald Image Lab email lets users test inline image hints, full-screen rendering, and local image fallback links in demo mode
- [x] Rich HTML rendering showcase email lets users verify headings, lists, links, remote image labels, and shared preview behavior in demo mode
- [ ] Demo mode accessible from the first-run wizard ("Try without an account")

---

## Testing Infrastructure

Integration tests and headless test harnesses ensure the app works correctly at the protocol level without a live server.

- [x] IMAP mock server (`internal/testutil/imap_server.go`) — in-process IMAP server for integration tests
- [x] Integration tests against the mock server (`internal/imap/integration_test.go`)
- [x] Repo-local pre-release test skill runs deterministic demo gates for Go tests, theme/image raster proof, SSH, and MCP before beta tagging
- [ ] 360 TUI manual QA matrix covering demo, live IMAP, live Ollama, SSH, and MCP lanes
- [ ] TUI snapshot tests via `teatest` or PTY (render correctness at fixed terminal sizes)
- [ ] CI pipeline running all tests on push

---

## Theming

- [x] App-level theme system (configurable in `~/.herald/conf.yaml`)
- [x] Inherit terminal color profile by default via `theme.name: inherited`
- [x] Herald dark and Herald light built-in themes, with `adaptive` and `legacy-dark` accepted as backward-compatible aliases
- [x] Diverse built-in app theme catalog covering red/black, crimson, emerald, ember, blue, violet, paper/light, and terminal-inspired palettes
- [x] `-theme` launch override accepts a built-in theme name or theme YAML file without saving config
- [x] Local YAML theme install from Settings into `~/.herald/themes`
- [x] Custom theme creation and semantic role editing from Settings without adding a global shortcut
- [x] Theme gallery docs screenshots can be regenerated with `scripts/regenerate-theme-screenshots.sh` for Timeline and `HERALD_THEME_SCREENSHOT_VIEW=preview` for open preview captures
