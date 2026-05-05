# TUI Test Plan — 360 QA Matrix

This manual QA specification defines the acceptance criteria for Herald's TUI across layout, focus semantics, tab transitions, attachments, AI behavior, degraded states, and surface compatibility. It is the source of truth for tmux-driven exploratory testing after any change that touches rendering, key handling, IMAP sync, attachments, AI integration, SSH, MCP, or background work scheduling.

**Test reports** must be saved in the `reports/` folder. Use descriptive names such as `reports/TEST_REPORT_2026-04-21_360-tui.md`.

---

## Setup

### Build targets

```bash
go build -o /tmp/herald ./cmd/herald
go build -o /tmp/herald-ssh ./cmd/herald-ssh-server  # compatibility wrapper
go build -o /tmp/herald-mcp ./cmd/herald-mcp-server  # compatibility wrapper
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
- `?` opens context-sensitive shortcut help from every major tab, pane, and overlay where Herald owns key routing.
- Keyboard layouts with physical-key reporting can use Herald-owned browse shortcuts from the same physical keys as the advertised Latin shortcuts; printable fallback aliases cover Cyrillic and direct Japanese kana layouts when `BaseCode` is unavailable, while search and Compose text inputs still receive the actual native characters.
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
- Timeline unread dots reflect live IMAP `\Seen` flags after refresh/sync, including messages read externally in Gmail when no new mail arrived.
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
1. Press `F1`, `F2`, `F3`.
2. Capture after each switch.
3. In a non-text browse context, press `1`, `2`, `3` as compatibility aliases.

**Expect:**
- Correct tab highlight.
- Tab-specific layout appears.
- Key hints change with the tab and consistently advertise `F1-F3: tabs`.
- Key hints consistently include `?: help` when there is room or wrapped hint space.
- Browse-number aliases keep working but are not the primary tab hint.
- Compose is not shown as a top-level tab.
- No stale status fragments from previous tabs.

### TC-02A — Layout-independent physical-key shortcuts

**Lane:** A, D
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Start in Timeline with the app in a browse context.
2. In a terminal with keyboard enhancement support, use a non-Latin keyboard layout and press the physical keys advertised as `j`, `k`, `l`, `c`, `/`, and `?`.
3. In fallback mode or synthetic tests, send Cyrillic characters that correspond to advertised physical shortcut keys on a Russian keyboard: `о` for `j`, `л` for `k`, `д` for `l`, `с` for `c`, `.` for `/`, and `,` for `?`.
4. In fallback mode or synthetic tests, send direct Japanese kana layout characters that correspond to advertised physical shortcut keys: `ま` for `j`, `の` for `k`, `り` for `l`, `そ` for `c`, and `め` for `/`.
5. Open Timeline search with the physical `/` key, Cyrillic fallback `.`, or direct-kana fallback `め`, then type native query text such as `привет` or `まのり`.
6. Open Compose from Timeline with the physical `Shift+C` key or Cyrillic fallback uppercase `С`, type native body text, and then leave Compose with `Esc`.
7. Repeat the safe browse-key portion over SSH.

**Expect:**
- Physical `j` and `k` positions move the active row regardless of the active text layout when the terminal reports `BaseCode`.
- Physical `l` opens and closes the log overlay, and physical `c` toggles the chat panel where chat is available.
- Physical `/` opens Timeline search and physical `?` opens shortcut help.
- The Cyrillic and direct-kana fallback aliases continue to behave the same way when `BaseCode` is unavailable and the terminal sends one committed character per keypress.
- Search and Compose text fields preserve the typed native characters instead of converting them to Latin shortcut names.
- Japanese romaji IME pre-edit is not treated as command input before the IME commits text; those sessions need terminal `BaseCode`/keyboard-enhancement support for true physical shortcuts while composing.
- SSH supports the fallback aliases everywhere it receives normal UTF-8 key messages; physical-key support depends on the SSH client and terminal reporting keyboard enhancements.

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
- The Timeline bottom hint bar advertises `Tab` / `Shift+Tab` panel switching when navigation hints are visible.

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
- When the split preview has focus, the bottom hint bar still exposes read/write message actions that work from preview focus: `R: reply`, `F: forward`, `D: delete`, and `e: archive`.

### TC-05C — Timeline reading-first row layout

**Lane:** A, B
**Sizes:** `220x50`, `120x40`, `80x24`, `50x15`

**Steps:**
1. Open Timeline with demo data and capture the list.
2. Confirm the focused row includes a sender, subject, local human date, and tag when width allows.
3. Select an email with an attachment and confirm the subject cell carries the attachment marker.
4. Open the split preview, then repeat the captures at `220x50` and `80x24`.
5. Resize to `50x15`, then back to `80x24`.

**Expect:**
- Timeline headers do not include `Size KB` or standalone `Att`.
- Attachment state appears next to the subject for single rows and collapsed threads that contain attachments.
- Sender and Subject remain the dominant columns; at `80x24` with preview open, optional `Tag` and then `When` may hide before Sender/Subject collapse.
- Dates use local human labels in the list and a full local human timestamp in the preview header.
- Header rows are visually distinct from body rows without breaking row selection styling.
- At `50x15`, the minimum-size guard appears instead of clipped row chrome, and resizing back restores a clean Timeline.

### TC-05B — Timeline horizontal reading movement

**Lane:** A, B
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Open Timeline with the sidebar visible and focus the Timeline list.
2. Press right arrow on a single-message row.
3. With that preview open and Timeline still focused, press right arrow or `]`.
4. Move to a collapsed thread row and press right arrow.
5. Focus the preview panel and press left arrow.
6. Move Timeline focus to an expanded thread parent row and press left arrow or `[`.
7. Move Timeline focus to a collapsed thread row or single-email row and press left arrow or `[`.
8. With no preview open, press left arrow or `[` from Timeline focus.
9. Focus the folder sidebar and press right arrow or `]`.
10. Open a preview with multiple attachments, focus the preview panel, and press `[` / `]`.
11. Press `U` from a previewed message, then repeat in a virtual read-only Timeline view such as `All Mail only`.

**Expect:**
- Right arrow and `]` open the split preview without moving focus out of the Timeline list when no preview is open.
- With a preview already open, right arrow and `]` from Timeline focus move focus into the preview without changing the previewed message.
- Collapsed thread rows preview the newest thread message and do not unfold.
- Left arrow from preview focus moves focus back to the Timeline list without closing the preview.
- Left arrow and `[` from Timeline focus fold an expanded thread parent row before moving focus farther left.
- Left arrow and `[` from Timeline focus close an open preview and focus folders when the current row is a single email or collapsed thread.
- With no preview open, left arrow and `[` show and focus the folder sidebar when the terminal can render it.
- Right arrow and `]` from folder focus return focus to the Timeline list.
- When preview focus is active and the email has multiple attachments, brackets navigate attachments instead of closing/opening panes.
- `U` marks the current previewed or focused Timeline message unread, updates the unread dot immediately, and is blocked in read-only diagnostic views.
- At `50x15`, the minimum-size guard appears instead of clipped horizontal-navigation UI.

### TC-05A — Timeline bulk selection for delete and archive

**Lane:** A, B
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Open Timeline.
2. Press `space` on a single-message row.
3. Move to a collapsed thread row and press `space`.
4. Press `D`, confirm the prompt copy references selected messages, then cancel with `Esc`.
5. Press `e`, confirm archive prompt copy references selected non-draft messages, then cancel with `Esc`.
6. Expand a thread with `Enter`, select one child row with `space`, resize through the required sizes, and capture.
7. Select a virtual read-only Timeline view such as `All Mail only` and try `space`, `D`, and `e`.

**Expect:**
- Timeline rows include a leading `✓` selection column.
- Selected individual rows show `✓`; collapsed thread rows show checked or partial state based on represented messages.
- Status text shows `N messages selected` only on Timeline and does not leak into Cleanup or Contacts.
- Hints advertise `space: select`, and selected-state hints advertise `D: delete selected` and `e: archive selected`.
- `D` and `e` use the selected message set instead of the current cursor row while any Timeline messages are selected.
- Read-only diagnostic views do not allow selection or destructive actions.
- At `50x15`, the minimum-size guard appears instead of clipped selection UI.

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
1. Open Timeline and press `C` to open Compose with a non-empty draft body.
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
1. Open Timeline and press `C` for a non-empty draft body, or press `R` from a Timeline message for reply context.
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
1. Open Timeline, press `C` to open Compose, and cycle focus through fields and preview.
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
1. Open Timeline and press `C` to open Compose.
2. Type `q123` into the focused address field, then tab to the body and type `q123` again.
3. Press `Esc`, confirm Timeline returns, press `C` again, then press `F1`, `F2`, and `F3` from Compose in separate passes.
4. Return to Timeline and press `C`, then repeat the same tab switching with `Alt+1/2/3` where the terminal supports Alt-modified digits.
5. Return to Timeline and press `C`, then press `Alt+L`, `Alt+L`, `Alt+C`, `Esc`, and `Alt+F`.
6. Press `Alt+R` from Compose.
7. Repeat with Timeline search open: type `q` into the query and press `Ctrl+C` only after confirming the query text is editable.

**Expect:**
- Plain `q` and digits remain in Compose text fields and do not quit or switch tabs.
- `Esc` from Compose returns to the Timeline state that opened it after local Compose transient state is dismissed.
- `F1/F2/F3` switch to Timeline/Cleanup/Contacts from Compose, and leaving a non-empty draft starts draft persistence.
- `Alt+1/2/3` keep working as secondary aliases when the terminal sends those chords.
- Compose and browse hints use `F1-F3: tabs` as the visible tab-switching annotation rather than mixing number-key and Alt-key tab labels.
- Timeline `C` opens blank Compose; lowercase `c` still opens chat.
- `Alt+L` opens and closes logs from Compose without typing into the draft.
- `Alt+C` opens chat from Compose when width allows, and `Esc` closes it cleanly.
- `Alt+F` toggles the sidebar preference from Compose without typing into the draft.
- `Alt+R` refreshes from Compose without typing into the draft.
- Timeline search treats plain `q` as query text while `Ctrl+C` remains the universal quit path.

### TC-14B — Demo Compose send is offline

**Lane:** A
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Start `/tmp/herald --demo` with no SMTP credentials configured.
2. Open Timeline, press `C`, then fill To, Subject, and Body.
3. Press `Ctrl+S` and capture the resulting status line.
4. Repeat at `80x24`.

**Expect:**
- Demo mode shows `Message sent!` after `Ctrl+S`.
- Demo mode does not show `Send failed` or require `smtp.host`.
- The success status remains visible and readable at `80x24`.

### TC-14C — Preserved HTML reply and forward compose

**Lane:** A, B
**Sizes:** `220x50`, `120x40`, `80x24`

**Steps:**
1. Open a Timeline message that has an HTML body, inline image, and attachment.
2. Press `R` and confirm Compose opens as a top-note editor with a read-only `Original message` pane rather than pasting the original body into the textarea.
3. Press `Ctrl+O` repeatedly and confirm the preservation mode cycles through Safe, Fidelity, and Privacy.
4. Press `Esc` to return to Timeline, press `F`, and confirm Compose shows separate `Response` and `Original message` regions.
5. Confirm forwarded attachments appear below Compose as included original attachments.
6. Focus the forwarded attachment list, move with `j`/`k`, press `x` to mark one attachment removed, then press `x` again to include it.
7. Repeat at `80x24` and confirm the response/original split and summary remain readable without overflow.

**Expect:**
- Replies and forwards show a concise preserved-content summary with mode, original HTML status, inline image count, and forwarded attachment count.
- The body textarea contains only the user's new note, not the converted original message.
- Reply Compose labels the editable top note as `Response` and shows a read-only `Original message` preview rendered from preserved original content.
- Forward Compose labels the editable top note as `Response` and shows a read-only `Original message` preview rendered from preserved original content.
- When the original-message pane has focus, `j`/`k` or arrow keys scroll the source without changing the editable response.
- The forwarded attachment focused row uses the same active focus color language as other navigable lists.
- Reply sends preserve the original HTML quote and threading headers.
- Forward sends preserve original HTML, preserve referenced inline images, include original attachments by default, and omit attachments toggled off with `x`.
- Missing HTML falls back to escaped plain-text quote without blocking send.

### TC-14D — Compose attachment path autocomplete

**Lane:** A, B
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Create a temp directory with two files that share a prefix, one subdirectory, one filename containing a space, and one dotfile.
2. Open Timeline, press `C` to open Compose, press `Ctrl+A`, type the temp path plus the shared file prefix, and press `Tab`.
3. Press `Tab` again when the common prefix is exhausted.
4. Cycle suggestions with `Tab`, `Shift+Tab`, `up`, and `down`.
5. Select a directory and press `Enter`, then select a file and press `Enter`.
6. Repeat the prompt and press `Esc` to cancel.

**Expect:**
- First `Tab` completes a unique match or longest common prefix.
- Repeated `Tab` shows a compact suggestion list only when multiple matches remain.
- Suggestions list directories before files, appends `/` to directories, and hides dotfiles until the typed segment starts with `.`.
- Selecting a directory keeps the prompt open with a trailing `/`.
- Selecting a file stages exactly that attachment and closes the prompt.
- Paths with spaces are inserted literally.
- At `50x15`, the minimum-size guard appears and the Compose view recovers cleanly when resized back to `80x24`.

### TC-14E — Timeline draft edit workflow

**Lane:** A, B
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Start `/tmp/herald --demo`.
2. Open Timeline and find a thread with at least one draft message.
3. Confirm the collapsed thread row marks the draft count as `Draft` or `Draft N`.
4. Expand the thread and confirm the individual reply draft row is marked `Draft reply` without using the classification Tag column.
5. Open the draft preview and capture the header plus the thread context above the body.
6. Press `Ctrl+S` from the draft preview, confirm the send prompt, and confirm the draft is removed only after send success.
7. Repeat from a draft row without opening Compose.
8. Press `E` from the draft row or preview.
9. Confirm Compose opens with the draft recipients, subject, and body restored.
10. Send the message in demo mode and press `Esc` to return to Timeline.
11. Repeat at `80x24`; at `50x15`, confirm the minimum-size guard or compact layout does not render overlapping draft labels.

**Expect:**
- Draft state is visible in both Timeline thread rows and individual message rows, including rows that are also marked as replies.
- Reply drafts show `Draft reply` in Timeline and preview state text.
- Draft reply preview shows compact thread context with the other visible messages in the conversation before the draft body.
- Preview header shows `State: Draft - E edit draft - Ctrl+S send` for plain drafts or `State: Draft reply - E edit draft - Ctrl+S send` for reply drafts.
- Draft preview/list hints prioritize `E: edit draft`, `Ctrl+S: send draft`, and `D: discard draft`.
- `E` opens Compose from a highlighted draft, from draft preview focus, and from a collapsed thread that contains a draft.
- `Ctrl+S` sends a highlighted draft, draft preview, or collapsed thread draft without switching to Compose.
- Sending deletes the source draft only after send success; autosave replacement never deletes the previous draft before the new save succeeds.

### TC-15 — Narrow screen behavior

**Lane:** A, B
**Sizes:** `80x24`, `50x15`

**Steps:**
1. Resize the session to `80x24`.
2. Open Timeline, Timeline-launched Compose, Cleanup, Contacts.
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
9. Repeat with `/`, type `? infrastructure budget risk`, press `Enter`, and open a semantic result.

**Expect:**
- Search state is visible.
- Results do not fire on every keystroke; search waits briefly before running.
- `Enter` from search input moves into result navigation when results exist.
- Results are navigable.
- Preview opened from search behaves like normal Timeline preview.
- Default Timeline search merges keyword and semantic results when embeddings are available.
- Semantic search remains available from search input with a leading `? query` prefix; plain `?` opens shortcut help instead.
- Exact keyword matches remain predictable and appear before semantic-only tail results.
- Duplicate emails from the keyword and semantic legs appear only once.
- No-match case gives a clear fallback hint.
- `Esc` unwinds in order: preview → results → input → original timeline state.
- The original cursor position and thread expansion state are restored after the final `Esc`.
- Timeline search does not advertise or use `Ctrl+S`.

### TC-16A — Timeline cross-participant reply threads

**Lane:** A, B
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Start `/tmp/herald --demo`.
2. Open Timeline and select a thread that contains a reply between `demo@demo.local` and another participant.
3. Confirm the collapsed row shows a `▸` disclosure marker and multiple participants, including `me`.
4. Press `Enter` to expand the thread.
5. Confirm the expanded root row shows a `▾` disclosure marker.
6. Move through the expanded rows.

**Expect:**
- Messages with the same normalized subject appear as one thread even when participants differ.
- The collapsed sender cell starts with `▸` after unread/star indicators and shows the newest unique participants rather than only the newest sender.
- The expanded root sender cell starts with `▾` after unread/star indicators.
- Rows whose subject starts with a reply prefix show a visible `↩` reply marker at the beginning of the sender cell; an expanded reply root shows `▾ ↩`.
- Non-reply child rows still use the existing `↳` indentation marker.

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

### TC-18A — Context-sensitive shortcut help overlay

**Lane:** A, B  
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Press `?` from Timeline list, Timeline preview, Compose, Cleanup summary, Cleanup preview, Contacts list, chat, logs, and a confirmation prompt.
2. Scroll the help overlay with `j/k` or arrow keys when content is taller than the viewport.
3. Close the overlay with `Esc`, `?`, and `q` in separate passes.
4. In Compose reply or forward mode, press `?` after `Ctrl+O` is available.
5. In Contacts, press `/`, type `? budget risk`, and confirm semantic results still work through the search input.

**Expect:**
- Plain `?` opens shortcut help, not semantic search, in Herald-owned contexts.
- At `220x50`, shortcut help appears as a compact centered modal over the current view, not a full-screen replacement.
- At `80x24`, shortcut help shrinks to fit without horizontal overflow.
- The overlay title names the current context and the body lists global, tab, pane, overlay, and mode-specific shortcuts.
- Compose help explains what preservation mode means and lists `Ctrl+O` only when reply/forward context exists.
- Overlay scroll state is bounded and resets when reopened from a different context.
- Closing help returns to the same tab/pane/overlay state without triggering the underlying key action.

### TC-19 — Multi-attachment navigation and save

**Lane:** B, C  
**Sizes:** `220x50`, `120x40`, `80x24`

**Steps:**
1. Open an email with 2+ attachments.
2. Move between attachments with `[` and `]`.
3. Press `s` to save.
4. Cancel once with `Esc`, then save again.
5. Create a file at the prompted save path, press `s` again, and press `Enter` without editing the colliding path.

**Expect:**
- Selected attachment visibly changes.
- Save targets the currently selected attachment.
- Save prompt belongs to preview-local state and unwinds with `Esc`.
- Attachment hints appear only when attachments are present.
- If the default save path already exists, the prompt is pre-filled with the next available filename such as `report (1).pdf` and shows a warning.
- Pressing `Enter` on any existing save path keeps the prompt open, changes the input to a non-conflicting suggestion, warns about the existing file, and does not overwrite file contents.

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

### TC-23A — Full-screen inline image rendering and fallback links

**Lane:** A, B, C
**Sizes:** `220x50`, `120x40`, `80x24`, `50x15`

**Steps:**
1. Open Timeline and search for `Creative Commons image sampler for terminal previews`.
2. Open the split preview and capture the image hint plus body links.
3. Press `z` to enter full-screen and capture the top of the document.
4. Scroll with app keys (`j`, `k`, `PgDn`, `PgUp`) until each inline image has appeared in the document flow.
5. In iTerm2 or Kitty raster mode, press `m` to release mouse capture, then use terminal-native scrollback to inspect whether image raster output displaced header/body text.
6. Repeat in stock ttyd and confirm the browser flow reaches full-screen; stock ttyd may show fallback `open image` links instead of raster output.
7. Repeat in a custom ttyd xterm.js frontend with `@xterm/addon-image` enabled and `TERM_PROGRAM=iTerm.app` on the Herald process; see `TUI_TESTING.md` for the required `/token` + websocket handshake details.
8. Repeat with `--demo -image-protocol=kitty` and confirm ANSI capture includes Kitty graphics `ESC_G` output.
9. In Kitty or Ghostty raster mode, scroll back and forth across multiple inline images and confirm old image placements are cleared before the current viewport is redrawn.
10. Repeat in Ghostty or a terminal with `TERM=xterm-ghostty` if available, a non-raster terminal, an iTerm2-compatible terminal if available, and SSH mode.
11. Run the standard resize cycle while full-screen preview is open.

**Expect:**
- The Creative Commons sampler fixture exposes four embedded inline images with different dimensions and HTML `cid:` placement.
- Split preview stays compact and does not promise image viewing when no full-screen image path is available.
- Full-screen preview renders text and inline images as one scrollable document below the pinned header.
- Raster images appear near their authored positions and do not push the header/title out of the visible app viewport or terminal scrollback.
- iTerm2-compatible terminals render bounded inline images using OSC 1337 when selected or auto-detected.
- Kitty-compatible terminals, including Ghostty, render bounded inline images using Kitty graphics protocol when selected or auto-detected.
- Kitty/Ghostty scrolling does not leave stale image placements over text or unrelated images.
- Custom ttyd + xterm image-addon mode can reproduce browser-visible iTerm2 OSC 1337 image behavior; if the custom page is blank, the test report records whether the initial ttyd websocket handshake was sent.
- Non-raster local TUI shows OSC 8 `open image` links to localhost-served MIME inline image bytes.
- SSH auto mode avoids misleading localhost links and shows bounded placeholders unless the original email contains remote image URLs; forced `-image-protocol=iterm2` or `-image-protocol=kitty` emits the selected raster protocol.
- Remote HTML image URLs appear as readable OSC 8 links and Herald does not fetch them automatically.
- At `50x15`, the minimum-size guard appears and resizing back restores a clean full-screen preview.
- Test reports include terminal app/version, ttyd/frontend mode, selected image protocol mode, screenshots for raster modes, and ANSI captures where possible.

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
1. Build and run `/tmp/herald ssh`.
2. Connect with `ssh -p 2222 localhost`.
3. Load the app, switch tabs, open one preview, and exit.

**Expect:**
- TUI renders over SSH.
- No startup panic.
- Focus, borders, and key hints remain sane at `80x24`.

### TC-26 — MCP read smoke after TUI changes

**Lane:** E

**Steps:**
1. Build and run `/tmp/herald mcp`.
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
3. Press `/`, type `? infrastructure budget risk`, press `Enter`, and open the first result.
4. Open quick replies from the preview with `Ctrl+Q`, then close the picker.

**Expect:**
- Classification tags appear without a real Ollama backend.
- `/` plus `? query` opens semantic search and returns deterministic demo results.
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
1. Build `/tmp/herald`.
2. Run `echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | /tmp/herald mcp --demo`.
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
1. Build `bin/herald` and the compatibility `bin/herald-mcp-server`.
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

### TC-32A — Cleanup delete/archive propagates to Timeline

**Lane:** A
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch Herald in demo mode.
2. Open Cleanup, focus a sender with visible messages, and capture the sender/details state.
3. Delete or archive one visible Cleanup message, confirming the prompt when shown.
4. Switch immediately to Timeline and search or navigate to the same sender/subject.
5. Repeat for a Cleanup sender/domain batch when demo data makes a safe target obvious.
6. Resize to `50x15`, then recover to `80x24` and Timeline.

**Expect:**
- The deleted or archived Cleanup message disappears from Timeline on the next render, without waiting for a later refresh.
- Stale Timeline search results, chat-filtered rows, selections, and open previews for the affected message are cleared.
- Cleanup details, Timeline rows, and folder/status counts settle coherently after the follow-up reload.
- `50x15` shows the minimum-size guard and resizing back restores a clean Timeline without stale deleted rows.

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
- `P` uses otherwise empty space for practical guidance: example prompt ideas, supported template variables, and a clear next step.
- `P` tells users to attach a prompt with `W` for automation or run one manually through MCP `classify_email_custom`.
- `P` tells users that prompt results are stored per email in custom category storage/MCP results.
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

### TC-49A — Shared HTML Markdown previews across surfaces

**Lane:** A
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch Herald in demo mode.
2. Open Timeline and search for `Rich HTML rendering showcase`.
3. Open the split preview, capture it, then press `z` and capture full-screen mode.
4. Open Cleanup, locate the same sender/message, open its preview, capture split and full-screen modes.
5. Open Contacts, select `Preview Lab`, open the `Rich HTML rendering showcase` email inline, and capture the preview.
6. Resize to `80x24` and `50x15`, then back to `220x50`, capturing the affected preview surface at each size.

**Expect:**
- Timeline, Cleanup, Contacts, and full-screen previews all show the HTML-derived body rather than stale plain-text fallback.
- Visible preview text preserves readable structure: `HTML preview quality`, list bullets, `Open dashboard`, and `Remote status chart`.
- Long tracking URL fragments such as `abcdefghijklmnopqrstuvwxyz0123456789` and `utm_source=email` do not appear in visible preview text.
- `50x15` shows the minimum-size guard and resizing back restores a clean preview.

### TC-50 — Mouse navigation parity

**Lane:** A
**Sizes:** `220x50`, `120x40`, `80x24`, `50x15`

**Steps:**
1. Launch Herald in demo mode with mouse capture enabled.
2. Click each top tab and confirm the active tab changes without typing into Compose fields.
3. Confirm the top tab row contains Timeline, Cleanup, and Contacts only.
3. In Timeline, click a visible single-message row to open preview, then wheel over the list and the preview.
4. In Timeline, click a collapsed thread root whose top email is not selected; confirm the preview opens for the top email and the thread stays collapsed. Click the same selected root again; confirm the thread expands.
5. In Timeline, click an expanded thread root whose top email is not selected; confirm the preview opens for the top email and the thread stays expanded. Click the same selected root again; confirm the thread folds.
6. In Cleanup, click a sender/domain row, click a details row to open preview, then wheel over the summary, details, and preview regions.
7. Click the sidebar when visible, then press `m` in a preview to release mouse capture and press `m` again to restore it.
8. Resize to `50x15`, capture the minimum-size guard, then recover to a larger size.

**Expect:**
- Mouse click and wheel behavior matches the equivalent keyboard actions and never changes hidden state outside the clicked region.
- Timeline thread-root mouse clicks use two-step semantics: select/update preview first, then fold/unfold only when the top thread email is already selected.
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
