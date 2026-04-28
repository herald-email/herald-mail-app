# TUI Test Plan — 360 QA Matrix

This manual QA specification defines the acceptance criteria for Herald's TUI across layout, focus semantics, tab transitions, attachments, AI behavior, degraded states, and surface compatibility. It is the source of truth for tmux-driven exploratory testing after any change that touches rendering, key handling, IMAP sync, attachments, AI integration, SSH, MCP, or background work scheduling.

**Test reports** must be saved in the `reports/` folder. Use descriptive names such as `reports/TEST_REPORT_2026-04-21_360-tui.md`.

---

## Setup

### Build targets

```bash
go build -o /tmp/herald ./main.go
go build -o /tmp/herald-ssh ./cmd/herald-ssh-server
go build -o /tmp/herald-mcp ./cmd/herald-mcp-server
```

### Demo tmux session

```bash
tmux kill-session -t mp 2>/dev/null || true
tmux new-session -d -s mp -x 220 -y 50
tmux send-keys -t mp '/tmp/herald --demo' Enter
sleep 5
```

### Live tmux session

```bash
tmux kill-session -t mp-live 2>/dev/null || true
tmux new-session -d -s mp-live -x 220 -y 50
tmux send-keys -t mp-live '/tmp/herald -config ~/.herald/conf.yaml' Enter
sleep 8
```

### Onboarding tmux session

```bash
rm -f /tmp/herald-onboard.yaml
tmux kill-session -t mp-onboard 2>/dev/null || true
tmux new-session -d -s mp-onboard -x 220 -y 50
tmux send-keys -t mp-onboard '/tmp/herald -config /tmp/herald-onboard.yaml' Enter
sleep 3
```

### Capture helpers

```bash
tmux capture-pane -t mp -p -e > /tmp/cap.txt
./.agents/skills/tui-test/screenshot.sh mp /tmp/cap.png
```

### Key helpers

```bash
tmux send-keys -t mp 'j' ''
tmux send-keys -t mp Enter
tmux send-keys -t mp Escape
tmux send-keys -t mp Tab
sleep 0.3
```

---

## Test Lanes

### Lane A — Demo Deterministic UI

Use `--demo` for repeatable layout, chrome, focus, and navigation checks.
Demo mode must not require IMAP credentials, SMTP credentials, Ollama, or a
private cache database. Its synthetic mailbox and AI responses should be stable
enough that demo tapes can double as lightweight smoke tests.

### Lane B — Live IMAP UX

Use the real config to validate:

- folder counts and transitions
- long subjects and real-world senders
- preview loading
- stale-state handling across tabs
- attachments with real messages
- progressive startup sync visibility and mid-sync Timeline refresh

### Lane C — Live Ollama / Semantic / Attachments

Use the real config and local Ollama to validate:

- semantic search
- quick replies
- contact enrichment
- image description
- attachment save flow
- overload and degraded-network behavior

### Lane D — SSH

Validate:

- TUI startup
- loading screen
- initial render over SSH
- no startup panic or shutdown panic

### Lane E — MCP Smoke

After TUI-affecting work, validate:

- `tools/list`
- one relevant read tool such as `list_recent_emails`
- `--demo` mode starts without loading `~/.herald/conf.yaml`

### Lane F — First-run Onboarding

Use a missing or empty temp config file to validate:

- setup wizard chrome and minimum-size handling
- recommended, supported, and experimental account messaging
- Standard IMAP credential labeling and navigation
- default-hidden Gmail OAuth, Gmail IMAP guidance, IMAP preset visibility, and hidden advanced defaults

---

## Terminal Sizes

Run every applicable case at:

| Size | Purpose |
|------|---------|
| `220x50` | wide baseline |
| `120x40` | medium split layout |
| `80x24` | standard SSH / laptop default |
| `50x15` | minimum-size guard |

---

## Core States

Check these states during every applicable lane:

- startup / loading
- sidebar visible / hidden
- tab switching
- focus cycling
- preview open / closed
- full-screen preview
- chat open / closed
- logs open / closed
- search active / cleared
- AI available / unavailable
- network healthy / degraded / unavailable

---

## Universal Assertions

### UX consistency

- Only one focused panel has the active border color.
- Row highlight language is consistent across:
  - folder sidebar
  - timeline
  - cleanup summary/details
  - contacts list/detail email list
- Key hints always match the normalized visible focus.
- Status bar never leaks stale mode or selection from another tab.
- Adjacent panels have aligned heights and closed borders.
- Tab-local overlays unwind in the right order with `Esc`.
- Copy uses consistent verbs: `open`, `close`, `preview`, `full-screen`, `back`.

### Reliability

- Semantic search has a visible success path and a visible degraded path.
- Quick replies have a visible success path and a visible degraded path.
- Contact enrichment has a visible success path and a visible degraded path.
- Image description has a visible success path and a visible degraded path.
- Attachment save supports the currently selected attachment, not only the first one.
- Local AI overload does not freeze UI and does not create runaway connection fan-out.
- Ollama-down, 404, timeout, or invalid-model failures produce bounded and deduplicated errors.
- A compact global AI chip remains visible whenever AI is configured and reflects the effective state of local AI work.
- Browser and unrelated network activity should remain usable while background Herald work runs.
- Startup live sync must visibly progress; it must not look frozen while IMAP work is still running.
- Startup live sync should progressively refresh visible Timeline rows as new mail is cached, not only after the final completion event.
- Startup sync refreshes should be microbatched so the screen feels alive without repainting on every single raw IMAP event.
- Folder switches should be latest-wins: stale sync results from an older folder request must not repaint the newly selected folder.
- The active folder visible bundle settles together: rows, unread/total counts, folder label, and folder tree presence become coherent within a 2-5 second window under normal startup conditions.
- Visible folder counts come from live IMAP folder status only; hydrated cache rows must not synthesize or overwrite sidebar, status-bar, or cleanup counts.
- The top sync strip reflects only unsettled active-folder work and disappears once the active folder bundle is settled.
- Startup must show the full folder tree as soon as the server folder list is known; loading the active folder must not collapse the sidebar to only a partial tree.
- Cleanup summary selection checkmarks and cleanup selection status text must always agree.
- Cleanup summary layout must resize cleanly without clipping the checkmark column or hiding the sender/domain column behind stale fixed-width assumptions.

### Network and backpressure

- Local AI work runs with bounded concurrency and prefers interactive requests over background tasks.
- Background embedding and enrichment defer or pause while interactive local AI work is active.
- Background work that is already in flight may finish, but no new background local-AI task starts while interactive work is queued or running.
- Queue saturation fails open: UI stays responsive and low-priority work is skipped or deferred.
- Herald must not suddenly open enough outbound connections to starve the rest of the machine.

---

## Test Cases

### TC-01 — Startup baseline

**Lane:** A, B
**Sizes:** `220x50`, `120x40`

**Steps:**
1. Start the app.
2. Wait for loading to finish.
3. Capture text and PNG.

**Expect:**
- Header and tab bar visible.
- No raw panic output.
- Status bar present.
- Key hint bar present.
- Active tab and active panel are visually obvious.
- If cached data is already visible while live sync continues, the UI explains that clearly and shows active sync progress in a human-readable way.
- The top sync strip uses stream language (`opening`, `syncing`, `reconciled`) rather than vague spinner-only wording.
- The sidebar folder tree is present as soon as the server folder list is available; it must not remain stuck on only `INBOX` and diagnostic entries while the active folder sync continues.

### TC-02 — Tab switching and hint updates

**Lane:** A, B
**Sizes:** all except `50x15`

**Steps:**
1. Press `1`, `2`, `3`, `4`.
2. Capture after each switch.

**Expect:**
- Correct tab highlight.
- Tab-specific layout appears.
- Key hints change with the tab.
- No stale status fragments from previous tabs.

### TC-03 — Focus border exclusivity

**Lane:** A, B
**Sizes:** `220x50`, `120x40`, `80x24`

**Steps:**
1. Open Timeline.
2. `Tab` focus through sidebar, timeline, preview, and chat if visible.
3. Repeat in Cleanup and Contacts.

**Expect:**
- Exactly one panel shows active border at a time.
- Timeline border deactivates when sidebar or preview owns focus.
- Inactive panels remain readable but visually secondary.

### TC-04 — Row highlight consistency

**Lane:** A, B
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Compare focused row in sidebar, timeline, cleanup list, cleanup detail list, contacts list, and attachment list.
2. Capture each.

**Expect:**
- Focused row styling uses one consistent visual language.
- Inactive selected rows use a subdued variant.
- No list uses a conflicting selection pattern.

### TC-05 — Timeline preview geometry

**Lane:** A, B
**Sizes:** `220x50`, `120x40`, `80x24`

**Steps:**
1. Open Timeline.
2. Open preview with `Enter`.
3. Capture.
4. Toggle full-screen with `z`.
5. Close with `Esc`.

**Expect:**
- Timeline and preview panels have aligned top/bottom borders in split mode.
- Preview border is active only when preview has focus.
- Key hints reflect list mode vs preview mode correctly.

### TC-06 — Sidebar focus behavior

**Lane:** A, B
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Focus sidebar with `Tab`.
2. Navigate with `j/k`.
3. Expand/collapse with `space`.
4. Open folder with `Enter`.

**Expect:**
- Sidebar row focus matches canonical row style.
- The selected folder remains visibly selected after focus leaves the sidebar.

### TC-07 — Global AI status chip

**Lane:** A, B, C  
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Start with AI configured and idle.
2. Trigger background embedding or enrichment.
3. Trigger quick reply while background AI is active.
4. Trigger semantic search while local AI is busy.
5. Repeat once with Ollama unavailable or an embedding model missing.

**Expect:**
- Status bar shows a compact `AI: ...` chip whenever AI is configured.
- Idle state reads `AI: idle`.
- Background-only work reads `AI: embedding`.
- When quick reply or semantic search is queued behind a running background call, the chip prefers the interactive intent over background progress.
- Degraded states read `AI: unavailable` or `AI: deferred`, not only log spam.

### TC-08 — Interactive-first local AI scheduling

**Lane:** C  
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Start live Herald with Ollama configured.
2. Allow background embedding or contact enrichment to begin.
3. Trigger quick reply from Timeline.
4. Trigger semantic search after that.
5. Watch status bar, logs, and UI responsiveness.

**Expect:**
- Quick reply and semantic search may wait for at most one already-running local call.
- After that call completes, queued interactive work runs before any queued background work.
- No additional background local-AI task starts while interactive work is queued or running.
- The UI remains navigable throughout.

### TC-09 — Background batch dedupe

**Lane:** B, C  
**Sizes:** `220x50`

**Steps:**
1. Load Timeline in a folder with unembedded mail and contacts needing enrichment.
2. Switch away and back to Timeline repeatedly.
3. Refresh while background embedding is still progressing.

**Expect:**
- Herald does not create duplicate background embedding bursts for the same folder.
- Herald does not create duplicate contact-enrichment bursts while one is already running or queued.
- Logs stay bounded and do not show a storm of repeated duplicate background launches.

### TC-10 — Fail-open backlog behavior

**Lane:** C  
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Create a local-AI backlog by letting background semantic work run.
2. Trigger additional background-producing actions such as folder reloads or enrichment.
3. Keep navigating the UI and open at least one interactive AI action.

**Expect:**
- Low-priority work is deferred or coalesced instead of opening a connection burst.
- The status chip can show `AI: deferred`.
- Browser and unrelated network activity on the machine remain usable.
- Herald does not wedge the terminal or chat/quick-reply flows.

### TC-11 — Compose AI assistant success and degrade

**Lane:** C  
**Sizes:** `220x50`, `120x40`, `80x24`

**Steps:**
1. Open Compose with a non-empty draft body.
2. Press `Ctrl+G` to open the AI assistant.
3. Trigger one quick action (`1`-`5`) or enter a custom prompt and press `Enter`.
4. Wait for a suggestion or bounded failure.
5. Edit the suggestion once, then press `Ctrl+Enter` to accept it into the compose body.
6. Repeat once with AI unavailable, misconfigured, or using an invalid model.

**Expect:**
- The AI panel opens without pushing the compose chrome off-screen.
- Success path shows loading state, diff/suggestion content, and an editable response area.
- `Ctrl+Enter` copies the accepted suggestion into the compose body and closes the panel cleanly.
- Narrow sizes degrade cleanly without broken borders or hidden compose inputs.
- Failure path stays responsive and shows concise bounded feedback in compose status instead of panicking or flooding logs.

### TC-12 — Compose AI subject hint accept and dismiss

**Lane:** C  
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Open Compose with either a non-empty draft body or reply context.
2. Press `Ctrl+J` to request a subject suggestion.
3. Capture the hint line when it appears.
4. Press `Tab` to accept the suggestion.
5. Trigger another suggestion and press `Esc` to dismiss it.
6. Repeat once with AI unavailable or no valid draft context.

**Expect:**
- A subject-hint line appears below the Subject field with visible accept/dismiss guidance.
- `Tab` accepts the hint only while the hint is visible and does not advance focus first.
- `Esc` dismisses the hint cleanly without disturbing the rest of the compose state.
- Missing draft context or unavailable AI produces bounded feedback such as `Write something first` or `No AI backend configured`.

### TC-13 — Stale status leakage across tabs

**Lane:** A, B
**Sizes:** `220x50`, `80x24`

**Steps:**
1. In Cleanup, select rows and messages.
2. Switch to Timeline and Contacts.
3. Open preview and capture.

**Expect:**
- Cleanup selection counts do not appear in Timeline or Contacts.
- Tab-local status fragments stay tab-local.

### TC-14 — Compose and Contacts chrome sanity

**Lane:** A, B
**Sizes:** `220x50`, `120x40`, `80x24`

**Steps:**
1. Open Compose and cycle focus through fields and preview.
2. Open Contacts and cycle list/detail/preview focus.
3. Capture each state.

**Expect:**
- Exactly one focused region looks active.
- List and detail borders are closed.
- Key hints match the focused region.

### TC-14A — Compose-safe command layer

**Lane:** A, B
**Sizes:** `220x50`, `120x40`, `80x24`

**Steps:**
1. Open Compose.
2. Type `q123` into the focused address field, then tab to the body and type `q123` again.
3. Press `Alt+1`, return to Compose with `Alt+2`, then press `Alt+3` and `Alt+4`.
4. Return to Compose with `Alt+2`, then press `Alt+L`, `Alt+L`, `Alt+C`, `Esc`, and `Alt+F`.
5. Press `Alt+R` from Compose.
6. Repeat with Timeline search open: type `q` into the query and press `Ctrl+C` only after confirming the query text is editable.

**Expect:**
- Plain `q` and digits remain in Compose text fields and do not quit or switch tabs.
- `Alt+1/2/3/4` switch tabs from Compose, and leaving a non-empty draft starts draft persistence.
- `Alt+L` opens and closes logs from Compose without typing into the draft.
- `Alt+C` opens chat from Compose when width allows, and `Esc` closes it cleanly.
- `Alt+F` toggles the sidebar preference from Compose without typing into the draft.
- `Alt+R` refreshes from Compose without typing into the draft.
- Timeline search treats plain `q` as query text while `Ctrl+C` remains the universal quit path.

### TC-15 — Narrow screen behavior

**Lane:** A, B
**Sizes:** `80x24`, `50x15`

**Steps:**
1. Resize the session to `80x24`.
2. Open Timeline, Compose, Cleanup, Contacts.
3. Resize to `50x15`.
4. Capture all screens.

**Expect:**
- `80x24` remains readable with no overflow, replacement glyphs, or broken borders.
- Optional panels degrade before borders break.
- `50x15` shows a complete actionable minimum-size message with exact required dimensions.

### TC-16 — Timeline search and clear

**Lane:** A, B, C  
**Sizes:** `220x50`, `80x24`

**Steps:**
1. In Timeline press `/`.
2. Type a query slowly, then type another quickly.
3. Press `Enter` and move into the result list.
4. Open a result with `Enter`.
5. Use a preview action such as quick reply or full-screen.
6. Press `Esc` repeatedly to unwind.
7. Repeat with a query that returns no matches.
8. Repeat and use `Ctrl+I` from the search input.

**Expect:**
- Search state is visible.
- Results do not fire on every keystroke; search waits briefly before running.
- `Enter` from search input moves into result navigation when results exist.
- Results are navigable.
- Preview opened from search behaves like normal Timeline preview.
- Default Timeline search merges keyword and semantic results when embeddings are available.
- Exact keyword matches remain predictable and appear before semantic-only tail results.
- Duplicate emails from the keyword and semantic legs appear only once.
- No-match case gives a clear fallback hint.
- `Esc` unwinds in order: preview → results → input → original timeline state.
- The original cursor position and thread expansion state are restored after the final `Esc`.
- Timeline search does not advertise or use `Ctrl+S`.

### TC-17 — Preview unwind order

**Lane:** A, B
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Open Timeline preview.
2. Enter full-screen.
3. Open visual mode if available.
4. Press `Esc` repeatedly.

**Expect:**
- `Esc` closes the nearest transient state first.
- Visual mode exits before full-screen.
- Full-screen exits before preview.
- Preview exits before broader tab state changes.

### TC-18 — Logs and chat resilience

**Lane:** A, B, C  
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Toggle chat with `c`.
2. Toggle logs with `l`.
3. Cycle focus while both are present where allowed.

**Expect:**
- Focus normalization stays correct.
- Borders remain exclusive.
- Key hints match the visible/focused overlay.
- Logs can be opened while visible startup data is already on screen and the active folder is still syncing.

### TC-19 — Multi-attachment navigation and save

**Lane:** B, C  
**Sizes:** `220x50`, `120x40`, `80x24`

**Steps:**
1. Open an email with 2+ attachments.
2. Move between attachments with `[` and `]`.
3. Press `s` to save.
4. Cancel once with `Esc`, then save again.

**Expect:**
- Selected attachment visibly changes.
- Save targets the currently selected attachment.
- Save prompt belongs to preview-local state and unwinds with `Esc`.
- Attachment hints appear only when attachments are present.

### TC-20 — Quick replies success and degrade

**Lane:** C  
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Open a message with body text.
2. Trigger quick replies.
3. Repeat with Ollama unavailable or misconfigured.

**Expect:**
- Success path shows reply options without freezing the UI.
- Failure path stays responsive and shows concise bounded feedback.

### TC-21 — Semantic search success and degrade

**Lane:** C  
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Run semantic search with a known-good embedding backend.
2. Repeat with embeddings unavailable, wrong model, or unsupported endpoint.

**Expect:**
- Success path returns sensible results.
- Semantic expansion is bounded and respects the configured similarity threshold instead of returning the whole folder.
- Degraded path reports AI unavailability or embedding deferral clearly.
- No silent failure and no runaway retries.
- Changing the configured embedding model invalidates stale embeddings and triggers clean re-embedding instead of mixing vector generations.

### TC-22 — Contact enrichment under Ollama failure

**Lane:** C  
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Open Contacts.
2. Trigger enrichment with Ollama healthy.
3. Repeat with Ollama stopped, invalid embedding endpoint, or invalid model.

**Expect:**
- Success path updates the contact.
- Failure path is concise, deduplicated, and does not flood the log viewer.
- Contacts remain navigable throughout.

### TC-23 — Image description behavior

**Lane:** C  
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Open an email with inline images.
2. Validate vision-capable backend behavior.
3. Repeat with non-vision model or unavailable AI backend.

**Expect:**
- Success path produces image descriptions or image placeholders as designed.
- Degraded path does not overflow or wedge the preview.
- Failure is surfaced cleanly.

### TC-24 — Local AI backlog and responsiveness

**Lane:** C  
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Trigger background embedding/classification/enrichment work.
2. While it runs, issue an interactive local AI action such as quick reply or semantic query.
3. Observe Herald and the rest of the machine.

**Expect:**
- Interactive work is prioritized.
- UI remains responsive to navigation and `Esc`.
- Low-priority work is deferred or paused instead of creating a connection storm.
- Browser and unrelated network activity remain usable.

### TC-25 — SSH render smoke

**Lane:** D  
**Sizes:** `120x40`, `80x24`

**Steps:**
1. Build and run `cmd/herald-ssh-server`.
2. Connect with `ssh -p 2222 localhost`.
3. Load the app, switch tabs, open one preview, and exit.

**Expect:**
- TUI renders over SSH.
- No startup panic.
- Focus, borders, and key hints remain sane at `80x24`.

### TC-26 — MCP read smoke after TUI changes

**Lane:** E

**Steps:**
1. Build and run `cmd/herald-mcp-server`.
2. Call `tools/list`.
3. Call one read tool such as `list_recent_emails`.

**Expect:**
- MCP starts successfully.
- Tool listing succeeds.
- One read operation succeeds and does not regress after TUI-affecting work.

### TC-45 — Demo mode AI and semantic search smoke

**Lane:** A
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Start `/tmp/herald --demo`.
2. Press `a` on Timeline and wait for classification to finish.
3. Press `?`, type `infrastructure budget risk`, press `Enter`, and open the first result.
4. Open quick replies from the preview with `Ctrl+Q`, then close the picker.

**Expect:**
- Classification tags appear without a real Ollama backend.
- `?` opens semantic search directly and returns deterministic demo results.
- Search results are meaningful for the query and can be opened.
- Quick replies show deterministic suggestions without blocking navigation.

### TC-46 — Demo fixtures cover public UI context

**Lane:** A
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Start `/tmp/herald --demo`.
2. Open Timeline preview for a newsletter, a billing/security message, and a message with an attachment.
3. Switch to Cleanup, open sender details, and preview one message.
4. Switch to Contacts, open one contact detail, and open a recent email inline.

**Expect:**
- Timeline shows fictional but realistic senders, subjects, folders, read/star states, and visible threads.
- Preview bodies are specific to each sender rather than placeholder lorem ipsum.
- At least one preview exposes attachments and at least one preview exposes unsubscribe actions.
- Contacts are populated from the same demo story and their recent emails open inline.

### TC-47 — MCP demo mode smoke

**Lane:** E

**Steps:**
1. Build `cmd/herald-mcp-server`.
2. Run `echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | /tmp/herald-mcp --demo`.
3. Run `list_recent_emails` against `INBOX`.
4. Run one search or sender-stat tool against the demo data.

**Expect:**
- `--demo` does not load or create user config/cache files.
- Tool listing succeeds.
- Read tools return fictional demo mailbox data.
- Output is deterministic enough for VHS recording.

### TC-48 — Canonical demo GIF generation

**Lane:** A, E

**Steps:**
1. Build `bin/herald` and `bin/herald-mcp-server`.
2. Run every tape in `demos/*.tape` with `vhs`.
3. Inspect the generated GIF durations and final paths.

**Expect:**
- GIFs are written to `assets/demo/`.
- Each GIF is between 5 and 30 seconds.
- No GIF shows a panic, unavailable AI state, missing private config, or empty demo data.
- The canonical scope is the five tapes under `demos/`; root `demo.tape` remains legacy.

### TC-27 — Virtual `All Mail only` inspector

**Lane:** B
**Sizes:** `220x50`, `120x40`, `80x24`

**Steps:**
1. Start with a live config that exposes `All Mail`.
2. Open the sidebar.
3. Select the virtual item `All Mail only`.
4. Wait for the diagnostic set to load.
5. Open one message preview and then full-screen it.

**Expect:**
- The sidebar shows `All Mail only` as a non-server diagnostic item.
- The Timeline opens a derived read-only view rather than a real IMAP folder.
- The status line and key hints explicitly indicate a diagnostic/read-only context.
- Preview and full-screen preview work normally.
- No destructive or mutating actions are advertised from this view.

### TC-28 — `All Mail only` unsupported state

**Lane:** B
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Run against an account/server that does not expose `All Mail`, or use a stubbed unsupported backend.
2. Select `All Mail only`.

**Expect:**
- The view shows a clear explanation that the provider does not expose `All Mail`.
- Herald does not show an empty ambiguous Timeline.
- No destructive actions are available.

### TC-29 — `All Mail only` read-only enforcement

**Lane:** A, B
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Select `All Mail only`.
2. Try `D`, `e`, `R`, `F`, `A`, `u`, `ctrl+q`, and star toggle.
3. Use `/` search inside the view.

**Expect:**
- Mutating actions are ignored or blocked with clear diagnostic-safe behavior.
- Key hints do not advertise delete/archive/reply/forward/re-classify/unsubscribe/quick-reply/star actions.
- Local search works within the derived set.
- `Esc` and preview navigation still behave normally.

### TC-30 — Active-folder bundle settles together

**Lane:** B
**Sizes:** `220x50`, `120x40`, `80x24`

**Steps:**
1. Start Herald against the real config.
2. Capture the first render with visible rows.
3. Watch the active folder for up to 5 seconds.
4. Capture again when the sync strip disappears or the counts settle.

**Expect:**
- The active folder becomes usable before secondary background work completes.
- Visible rows, folder counts, current folder label, and folder tree settle into one coherent state within a 2-5 second bundled window under normal conditions.
- The UI never shows `synced` while counts are still unsettled or while the visible folder tree is still incomplete.

### TC-31 — No count drift between sync hydration and live counts

**Lane:** B
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Start Herald with a folder that already has cached rows.
2. Capture sidebar counts, status-bar counts, and visible active-folder counts while sync is still running.
3. Capture again after sync settles.

**Expect:**
- Sidebar and status-bar counts reflect live IMAP folder status, not the number of hydrated visible rows.
- Hydrated cache slices do not temporarily rewrite the active folder to a smaller or contradictory count.
- Cleanup grouping counts for the active folder match the same authoritative live folder count model.

### TC-32 — Cleanup selection persistence and checkmarks

**Lane:** A, B
**Sizes:** `220x50`, `120x40`, `80x24`

**Steps:**
1. Open Cleanup in sender mode.
2. Select multiple rows with `space`.
3. Resort or refresh if available, switch tabs, then return.
4. Resize the terminal once and capture again.
5. Repeat in domain mode.

**Expect:**
- Selected rows keep visible checkmarks.
- The summary text such as `4 senders selected` or `4 domains selected` matches the visible checkmarks exactly.
- Selection survives refreshes, reordering, tab switches, and resizes because it is tied to logical sender/domain identity rather than row index.

### TC-33 — Cleanup responsive column layout

**Lane:** A, B
**Sizes:** `220x50`, `120x40`, `80x24`, `50x15`

**Steps:**
1. Open Cleanup with a wide terminal.
2. Resize down through every required size.
3. Capture after each resize.

**Expect:**
- Cleanup summary columns are exactly `✓`, `Sender/Domain`, `Count`, and `Date Range`.
- `Avg KB` and `Attach` do not appear.
- The sender/domain column reclaims freed width first.
- The first selection column remains visible and aligned at every supported size.
- At `220x50`, the date-range column expands enough to show a more specific day-level first/last range instead of being capped to the narrow fallback width.

### TC-34 — Folder tree completeness during startup

**Lane:** B
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Start Herald against the real config.
2. Observe the sidebar during the first visible render and during the next few seconds.
3. Capture once during active sync and once after settling.

**Expect:**
- The folder tree appears early and stays stable while the active folder sync continues.
- Starting a heavy `INBOX` sync does not temporarily collapse the sidebar to only the active folder and virtual entries.

### TC-35 — Sync strip honesty and disappearance

**Lane:** B
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Start Herald and observe the top sync strip.
2. Wait until the active folder settles.
3. Switch folders and repeat once.

**Expect:**
- The strip describes only real active-folder unsettled work such as opening, fetching, or refreshing counts.
- It does not advertise unrelated background reconcile or sender-stat work as the main story.
- It disappears once the active folder bundle is settled.
- It never shows a spinner glyph.

### TC-36 — Cleanup narrow controls and overlay fit

**Lane:** A, B
**Sizes:** `80x24`

**Steps:**
1. Open Cleanup with the sender summary focused.
2. Verify the hint bar, then move focus to the sidebar, select another folder, and return to the sender summary.
3. Open the rule editor (`W`) and the prompt editor (`P`) from Cleanup.
4. Capture each state.

**Expect:**
- The sender summary remains keyboard-navigable after selecting a folder from the sidebar.
- The narrow Cleanup hint bar still exposes navigation plus `W`, `C`, and `P`.
- Rule and prompt overlays stay fully inside the viewport instead of clipping off the top or bottom.
- Rule, cleanup, and prompt overlays explain what they do, what saving or running them changes, and where the user can come back to review saved items or results.

### TC-37 — Cleanup overlays explain saved-item discovery

**Lane:** A, B
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Open Cleanup.
2. Open the automation rule overlay with `W`.
3. Open the prompt overlay with `P`.
4. Open the cleanup rules manager with `C`.
5. Capture each overlay.

**Expect:**
- `W` explains that it creates future-mail automations rather than immediate cleanup.
- `W` shows a visible inventory or summary of saved automation rules in the same screen.
- `P` explains that prompts are reusable AI instructions and do nothing until used.
- `P` shows a visible inventory or summary of saved prompts in the same screen.
- `C` explains that cleanup rules run on demand or on schedule and that saved cleanup rules live in that manager.

### TC-38 — All Mail only stays folder-unassigned

**Lane:** B
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Open the `All Mail only` virtual folder.
2. Inspect several messages that are known to exist in `Sent`, `Archive`, or nested folders.
3. Compare the visible rows against live folder membership when possible.

**Expect:**
- `All Mail only` contains only mail that has no other real folder assignment.
- Mail that also belongs to `Sent`, `Archive`, or any nested subfolder is excluded.
- If live membership inspection is incomplete, the view fails closed with a visible unsupported/error explanation rather than showing a partial result.

### TC-39 — First-run wizard chrome and size guard

**Lane:** F
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch Herald with a missing temp config path.
2. Capture the account-selection step at each size.
3. Resize down to `50x15`.

**Expect:**
- Wizard uses Herald-branded chrome rather than raw unframed form output.
- Copy clearly distinguishes supported IMAP account paths from experimental OAuth onboarding.
- Default account choices include Standard IMAP, Gmail IMAP App Password, ProtonMail Bridge, Fastmail, iCloud, and Outlook, without experimental labels on IMAP-based presets.
- Default account choices do not include Gmail OAuth.
- At `220x50` and `80x24`, the form is centered and fully readable.
- At `50x15`, Herald shows the minimum-size resize message instead of clipped fields.

### TC-40 — Standard IMAP credentials stay labeled and navigable

**Lane:** F
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. From the first-run wizard, choose `Standard IMAP`.
2. Advance to the credentials step.
3. Capture before and after moving focus through the first three inputs.

**Expect:**
- The active top input still has visible context; the user can tell it is the email field.
- `Password`, `IMAP Host`, `IMAP Port`, `SMTP Host`, and `SMTP Port` remain readable.
- Hints match the current control.
- At `50x15`, Herald falls back to the minimum-size guard rather than clipping later fields off-screen.

### TC-41 — Gmail OAuth experimental gate and IMAP guidance

**Lane:** F
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Launch Herald with a missing temp config path and no `-experimental` flag.
2. Capture the account-selection step and confirm Gmail OAuth is absent.
3. Choose `Gmail (IMAP + App Password)`.
4. Capture the Gmail IMAP guidance step.
5. Toggle advanced server editing and capture again.
6. Relaunch with `-experimental` and a missing temp config path.
7. Choose `Gmail OAuth (Experimental)` and capture the OAuth account guidance and wait screen.

**Expect:**
- Gmail OAuth is hidden in default first-run onboarding.
- Gmail OAuth appears as `Gmail OAuth (Experimental)` only when launched with `-experimental`.
- The OAuth wait screen remains centered and shows an unboxed browser-auth prompt: `Click: [here] or copy this link to the browser:`, where `[here]` is an OSC 8 terminal hyperlink and a short `http://localhost:<port>/authorize` URL remains visible for copying.
- Gmail IMAP guidance includes Gmail server defaults and links or copy for IMAP/App Password setup.
- Gmail IMAP is described as the normal Gmail setup path while OAuth onboarding is experimental, with a note that Workspace may require OAuth.
- Advanced server fields are hidden until explicitly requested.

### TC-42 — Missing, empty, and malformed config startup behavior

**Lane:** F
**Sizes:** `220x50`

**Steps:**
1. Launch Herald with a missing temp config file.
2. Launch Herald with an empty temp config file.
3. Launch Herald with a non-empty malformed temp config file such as `credentials:`.

**Expect:**
- Missing config launches onboarding.
- Empty or whitespace-only config also launches onboarding.
- Non-empty malformed config fails with a direct validation/load error and does not overwrite the file with onboarding output.

### TC-43 — Timeline preview exposes unsubscribe and Hide Future Mail in context

**Lane:** A, B
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Open Timeline and select an email whose loaded body exposes `List-Unsubscribe`.
2. Open the email preview and capture the preview plus the bottom hint bar.
3. Open a second Timeline preview whose loaded body does not expose `List-Unsubscribe`.
4. Capture the preview plus the bottom hint bar again.

**Expect:**
- The preview header includes `Tags:` and `Actions:` rows.
- With `List-Unsubscribe`, the preview metadata and hint bar both advertise `u: unsubscribe` and `h: hide future mail`.
- Without `List-Unsubscribe`, the preview metadata and hint bar do not advertise `u`, but still advertise `h: hide future mail`.
- End-user copy does not use `hard unsubscribe` or `soft unsubscribe`.
- A first-time user can identify the available list/sender action from the preview itself without prior knowledge.

### TC-44 — Cleanup summary and preview use the same `u` / `h` semantics

**Lane:** A, B
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Open Cleanup with the sender summary focused and capture the hint bar.
2. Confirm the sender summary advertises `h: hide future mail`.
3. Open a Cleanup preview for an email whose loaded body exposes `List-Unsubscribe` and capture the preview plus the hint bar.
4. Open a Cleanup preview for an email whose loaded body does not expose `List-Unsubscribe` and capture again.

**Expect:**
- Cleanup sender summary advertises `h: hide future mail`.
- Cleanup sender summary does not advertise `u: unsubscribe`.
- Cleanup preview includes `Tags:` and `Actions:` rows in the preview header.
- Cleanup preview uses the same availability rules as Timeline preview: `u` appears only when `List-Unsubscribe` exists, while `h` remains visible in both cases.
- End-user copy does not use `hard unsubscribe` or `soft unsubscribe`.

### TC-49 — Email preview hides long link destinations behind OSC 8 labels

**Lane:** A
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Launch Herald in demo mode.
2. Open Timeline and search for `Link rendering stress preview`.
3. Open the Taskpad demo email preview.
4. Capture plain text and ANSI output at `220x50`.
5. Resize to `80x24`, scroll to the link section, and capture plain text and ANSI output again.

**Expect:**
- Visible preview text shows readable labels such as `Display in your browser` and `Taskpad logo`.
- Long destination fragments such as `eyJmaXJ`, `_next/static/media`, and `abcdefghijklmnopqrstuvwxyz0123456789` do not appear in visible preview text.
- ANSI captures include OSC 8 hyperlink sequences for the hidden destination URLs.
- The preview panel and hint bar still fit at both sizes, with no link text bleeding past panel borders.

### TC-50 — Mouse navigation parity

**Lane:** A
**Sizes:** `220x50`, `120x40`, `80x24`, `50x15`

**Steps:**
1. Launch Herald in demo mode with mouse capture enabled.
2. Click each top tab and confirm the active tab changes without typing into Compose fields.
3. In Timeline, click a visible row to open preview, then wheel over the list and the preview.
4. In Cleanup, click a sender/domain row, click a details row to open preview, then wheel over the summary, details, and preview regions.
5. Click the sidebar when visible, then press `m` in a preview to release mouse capture and press `m` again to restore it.
6. Resize to `50x15`, capture the minimum-size guard, then recover to a larger size.

**Expect:**
- Mouse click and wheel behavior matches the equivalent keyboard actions and never changes hidden state outside the clicked region.
- Preview wheel events scroll the body without moving the underlying list cursor.
- List wheel events move the focused list cursor and refresh an open preview when applicable.
- The `m` toggle releases and restores TUI mouse capture while keeping visual/copy modes coherent.
- The minimum-size guard still appears at `50x15` and recovery restores normal mouse-capable layouts.

---

## Recommendations

Run the lanes in this order for bug-fix or refactor work:

1. Lane A at `220x50` and `80x24`
2. Lane B at `220x50` and `80x24`
3. Lane C with a healthy Ollama instance
4. Lane C with Ollama degraded or unavailable
5. Lane D SSH smoke
6. Lane E MCP smoke
7. Lane F onboarding when the change touches setup or config discovery

For UI-only changes, Lane A plus the relevant focused cases may be enough during iteration, but the post-completion report still needs at least one pass each of TUI, SSH, and MCP.
