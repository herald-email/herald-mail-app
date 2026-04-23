# TUI Test Plan — 360 QA Matrix

This manual QA specification defines the acceptance criteria for Herald's TUI across layout, focus semantics, tab transitions, attachments, AI behavior, degraded states, and surface compatibility. It is the source of truth for tmux-driven exploratory testing after any change that touches rendering, key handling, IMAP sync, attachments, AI integration, SSH, MCP, or background work scheduling.

**Test reports** must be saved in the `reports/` folder. Use descriptive names such as `reports/TEST_REPORT_2026-04-21_360-tui.md`.

---

## Setup

### Build targets

```bash
go build -o /tmp/herald ./main.go
go build -o /tmp/herald-ssh ./cmd/herald-ssh-server
go build -o /tmp/herald-mcp ./cmd/mcp-server
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
- Key hints show sidebar actions while sidebar is focused.
- Timeline border is inactive while sidebar is focused.

### TC-07 — Stale status leakage across tabs

**Lane:** A, B  
**Sizes:** `220x50`, `80x24`

**Steps:**
1. In Cleanup, select rows and messages.
2. Switch to Timeline and Contacts.
3. Open preview and capture.

**Expect:**
- Cleanup selection counts do not appear in Timeline or Contacts.
- Tab-local status fragments stay tab-local.

### TC-08 — Compose and Contacts chrome sanity

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

### TC-09 — Narrow screen behavior

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

### TC-10 — Timeline search and clear

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

### TC-11 — Preview unwind order

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

### TC-12 — Logs and chat resilience

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

### TC-13 — Multi-attachment navigation and save

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

### TC-14 — Quick replies success and degrade

**Lane:** C  
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Open a message with body text.
2. Trigger quick replies.
3. Repeat with Ollama unavailable or misconfigured.

**Expect:**
- Success path shows reply options without freezing the UI.
- Failure path stays responsive and shows concise bounded feedback.

### TC-15 — Semantic search success and degrade

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

### TC-16 — Contact enrichment under Ollama failure

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

### TC-17 — Image description behavior

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

### TC-18 — Local AI backlog and responsiveness

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

### TC-19 — SSH render smoke

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

### TC-20 — MCP read smoke after TUI changes

**Lane:** E

**Steps:**
1. Build and run `cmd/mcp-server`.
2. Call `tools/list`.
3. Call one read tool such as `list_recent_emails`.

**Expect:**
- MCP starts successfully.
- Tool listing succeeds.
- One read operation succeeds and does not regress after TUI-affecting work.

### TC-21 — Virtual `All Mail only` inspector

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

### TC-22 — `All Mail only` unsupported state

**Lane:** B  
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Run against an account/server that does not expose `All Mail`, or use a stubbed unsupported backend.
2. Select `All Mail only`.

**Expect:**
- The view shows a clear explanation that the provider does not expose `All Mail`.
- Herald does not show an empty ambiguous Timeline.
- No destructive actions are available.

### TC-23 — `All Mail only` read-only enforcement

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

---

## Recommendations

Run the lanes in this order for bug-fix or refactor work:

1. Lane A at `220x50` and `80x24`
2. Lane B at `220x50` and `80x24`
3. Lane C with a healthy Ollama instance
4. Lane C with Ollama degraded or unavailable
5. Lane D SSH smoke
6. Lane E MCP smoke

For UI-only changes, Lane A plus the relevant focused cases may be enough during iteration, but the post-completion report still needs at least one pass each of TUI, SSH, and MCP.
