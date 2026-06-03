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
enough that demo tapes can double as lightweight smoke tests. Mail-only
navigation should show Timeline and Contacts; calendar-enabled demo sessions add
a Calendar destination without replacing the existing mail keys. Timeline
grouping covers the former cleanup browse workflow.

Use `--demo --demo-keys` only for media captures that need a visible keypress
overlay. Normal demo sessions must keep the overlay hidden and must preserve
text entry in Compose, search prompts, and rule/prompt editors.

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
- Account Type first screen with Gmail OAuth, Standard IMAP, Gmail App Password, Proton Mail Bridge, Fastmail, iCloud, and Outlook visible before provider details
- compact Google account provider step with Mail enabled, Google Calendar enabled by default, no `Connect Google` button, and no IMAP/SMTP copy on the OAuth screen
- generic `Verify access` / `Checking account access...` copy on credential provider validation screens
- OAuth or validation failure returns to a populated provider setup screen so typos can be corrected without restarting
- post-validation compact `Advanced settings` review with `Enter Herald`, `Customize setup`, default preferences applied, and full preference steps available only when customizing
- Standard IMAP credential labeling and navigation
- Gmail IMAP guidance, IMAP preset visibility, provider switching without stale host/port values, and hidden advanced defaults

### Lane G — Virtual Mail Lab

Use `internal/testmail` for deterministic realistic mail scenarios before falling back to private live mail. The virtual lab starts local IMAP/SMTP servers, default `alice@herald.test` and `bob@herald.test` accounts, and sanitized corpus fixtures under `internal/testmail/testdata/corpus`.

Virtual lab scenarios should cover:

- plain two-user threads
- Calendly-like `text/calendar` invites
- table-heavy newsletters
- receipt/transactional HTML
- malformed charset/plaintext fallback messages
- inline CID images
- long safe links that preserve wrapping stress without tracking parameters
- draft, send, reply, Sent, and recipient INBOX flows

Reports that use this lane must mark `virtual lab` in `engineering/testplans/REPORT_TEMPLATE.md`. Demo mode remains the presentation lane; virtual lab is the realistic regression lane.

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
- Key hints must not silently drop primary actions at `80x24`; if a primary action cannot fit in the bottom chrome, it must remain discoverable in `?` help and the bottom chrome must still advertise `?: help`.
- Visible bottom chrome must be compared against the actual shortcuts available for the active pane. Missing primary actions are first-class bugs, not harmless copy drift.
- Timeline protected hints must stay visible whenever contextually valid: `c: compose`, `r: all`, `R: sender`, `f: forward`, `d: delete`, `D: delete now`, `a: archive`, panel `Tab`, and `?: help`.
- `?` opens context-sensitive shortcut help from every major tab, pane, and overlay where Herald owns key routing.
- While `?` help is open, the bottom hint bar belongs to help and must not continue advertising shortcuts from the underlying tab, pane, or overlay.
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
- Cached last-known folder names may seed the sidebar before live IMAP listing completes, but cached folder names must not seed unread/total counts.
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
- `Herald` and the `F1`-`F3` tab labels share one title row.
- Inactive tab labels inherit the terminal default foreground; the selected tab remains visually distinct.
- No raw panic output.
- Status bar present.
- Key hint bar present and readable in the terminal default foreground.
- Status bar and key hint bar are separated by a full-width horizontal line divider.
- Active tab and active panel are visually obvious.
- If cached data is already visible while live sync continues, the UI explains that clearly and shows active sync progress in a human-readable way.
- If the startup cache snapshot is empty, the app still shows the real Timeline chrome with an empty list and sync strip instead of staying on the standalone loading screen.
- First-run/full-resync startup progressively hydrates Timeline rows from cached IMAP batches, starting with the newest available batch, rather than waiting for every envelope scan to finish.
- The top sync strip uses stream language (`opening`, `syncing`, `reconciled`) rather than vague spinner-only wording.
- The sidebar folder tree is present as soon as the server folder list is available; it must not remain stuck on only `INBOX` and diagnostic entries while the active folder sync continues.

### TC-02 — Tab switching and hint updates

**Lane:** A, B
**Sizes:** all except `50x15`

**Steps:**
1. Press `F1`, `F2`, `F3`, and `F4` in a calendar-enabled demo session.
2. Capture after each switch.
3. In a non-text browse context, press `1`, `2`, and then `3`.
4. Repeat in a mail-only session with no calendar agenda backend.

**Expect:**
- `F1`/`1` open Timeline, `F2`/`2` open Contacts, `F3` remains a legacy Contacts alias, and `F4` opens Calendar only when Calendar is advertised.
- Plain `3` opens Calendar only when the title row advertises Calendar; otherwise it is not a top-level tab switcher outside quick-reply selection.
- `Herald` and tabs remain on one title row; no separate tab-bar row appears.
- Tab-specific layout appears.
- Key hints change with the tab and advertise `1-2: tabs` for mail-only sessions or `1-3: tabs` for calendar-enabled sessions.
- Key hints consistently include `?: help` when there is room or wrapped hint space.
- Browse-number aliases keep working but are not the primary tab hint.
- Compose is not shown as a top-level tab.
- No stale status fragments from previous tabs.

### TC-02A — Layout-independent physical-key shortcuts

**Lane:** A, D
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Start in Timeline with the app in a browse context.
2. In a terminal with keyboard enhancement support, use a non-Latin keyboard layout and press the physical keys advertised as `h`, `j`, `k`, `l`, `c`, `/`, and `?`.
3. In fallback mode or synthetic tests, send Cyrillic characters that correspond to advertised physical shortcut keys on a Russian keyboard: `р` for `h`, `о` for `j`, `л` for `k`, `д` for `l`, `с` for `c`, `.` for `/`, and `,` for `?`.
4. In fallback mode or synthetic tests, send direct Japanese kana layout characters that correspond to advertised physical shortcut keys: `く` for `h`, `ま` for `j`, `の` for `k`, `り` for `l`, `そ` for `c`, and `め` for `/`.
5. Open Timeline search with the physical `/` key, Cyrillic fallback `.`, or direct-kana fallback `め`, then type native query text such as `привет` or `まのり`.
6. Open Compose from Timeline with the physical `c` key or Cyrillic fallback lowercase `с`, type native body text, and then leave Compose with `Esc`.
7. Repeat the safe browse-key portion over SSH.

**Expect:**
- Physical `h/j/k/l` positions navigate left/down/up/right where the active view has a meaningful target.
- Physical `c` opens Compose from Timeline while literal `c` stays text in editable fields.
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
- Folder sidebar navigation skips non-selectable section headers or gives selectable section rows the same active/inactive highlight language.
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
- When the split preview has focus, the bottom hint bar still exposes read/write message actions that work from preview focus: `r: all`, `R: sender`, `f: forward`, `d: delete`, `D: delete now`, and `a: archive`.

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

### TC-05D — Timeline grouping switch

**Lane:** A, B
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Open Timeline with demo data and confirm the Timeline frame starts with `Grouped by: Thread (G to change)`.
2. Press `G` once and capture the Timeline list.
3. Press `G` again and capture the Timeline list.
4. Press `G` a third time and capture the Timeline list.
5. In sender and domain modes, press `Enter` on a collapsed grouped row, then `Esc` or left arrow / `[` to return to normal browsing.
6. Open Compose from Timeline, type a literal `G` in an editable field, then cancel back to Timeline.
7. Resize to `50x15`, then back to `80x24`.

**Expect:**
- `G` rotates `Thread -> Sender -> Domain -> Thread` without switching tabs or opening chat/log/sidebar overlays.
- Sender and domain modes use Timeline row chrome: disclosure markers, message counts, newest dates, unread/star indicators, attachment state, and tags where width allows.
- Existing Timeline actions remain available in grouped modes: preview, expand/fold, select, delete/archive confirmation, reply/forward on highlighted message, search, help, logs, and chat.
- The Timeline frame top border shows `Grouped by: Thread/Sender/Domain (G to change)` for the active grouping mode.
- Status text reflects grouping changes and stays scoped to Timeline.
- Hints and shortcut help advertise `G: group` where Timeline browse shortcuts are valid.
- Literal `G` remains text in Compose and other editable fields.
- At `50x15`, the minimum-size guard appears instead of clipped grouping UI, and resizing back restores a clean Timeline.

### TC-05E — Timeline sorting modes

**Lane:** A, B
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Open Timeline with demo data and confirm the default `When ↓` order is newest-first.
2. Press `O` repeatedly and capture the visible order and active column header for `When ↑`, `Sender ↑`, `Sender ↓`, `Subject ↓`, `Subject ↑`, and back to `When ↓`.
3. Press `G` into sender and domain grouping, then repeat enough sort changes to prove grouped rows sort by sender/domain label, group count through the `Subject` header, and newest message date through `When`.
4. Click the `Sender`, `Subject`, and `When` headers. Click the active sorted header a second time to flip the direction.
5. Open Compose from Timeline, type a literal `O` in an editable field, then cancel back to Timeline.
6. Resize to `50x15`, then back to `80x24`.

**Expect:**
- `O` cycles `When ↓ -> When ↑ -> Sender ↑ -> Sender ↓ -> Count ↓ -> Count ↑ -> When ↓` without switching grouping modes.
- The active sorted column header shows `↑` or `↓`; `Subject` is the visible click target and indicator for count sorting because group counts appear as `[N]` in that cell.
- Clicking an unsorted supported header selects its default direction: `Sender ↑`, `Subject ↓`, or `When ↓`; clicking the active sorted header flips direction.
- Starred or pinned groups remain above unstarred groups, with sorting applied inside each bucket.
- Sorting preserves message selections, expanded groups, and any open preview whenever the selected email remains visible.
- Hints and shortcut help advertise `O: sort` where Timeline browse shortcuts are valid.
- Literal `O` remains text in Compose, Timeline search, prompt, and editor fields.
- At `50x15`, the minimum-size guard appears instead of clipped sorting UI, and resizing back restores clean headers and rows.

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
4. Where the terminal reports shifted arrows, press `Shift+Down` and `Shift+Up` from the Timeline list to extend and shrink a selected range.
5. Press `V`, then `j` / `k`, then `Esc` to verify fallback range selection keeps the selected messages.
6. Press `D`, confirm the prompt copy references selected messages, then cancel with `Esc`.
7. Press `e`, confirm archive prompt copy references selected non-draft messages, then cancel with `Esc`.
8. Expand a thread with `Enter`, select one child row with `space`, resize through the required sizes, and capture.
9. Select a virtual read-only Timeline view such as `All Mail only` and try `space`, `Shift+Down`, `V`, `D`, and `e`.

**Expect:**
- Timeline rows include a leading `✓` selection column.
- Selected individual rows show `✓`; collapsed thread rows show checked or partial state based on represented messages.
- Status text shows `N messages selected` only on Timeline and does not leak into Cleanup or Contacts.
- Hints advertise `space: select`, `V: range`, and shifted-arrow range selection where space allows.
- Shifted-arrow range selection stops when plain `j/k` or arrows are used; the selected set remains selected and normal navigation resumes.
- Fallback `V` range mode stays active until `V` or `Esc`; its hints show `j/k: extend range`, `V/Esc: done`, `d: delete selected`, `D: delete now`, and `a: archive selected`.
- `d`, `D`, `Backspace`, `Shift+Backspace`, and `e` use the selected message set instead of the current cursor row while any Timeline messages are selected; lowercase/delete-Backspace paths prompt, uppercase/Shift+Backspace paths delete immediately.
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
**Sizes:** `220x50`, `120x40`, `80x24`, `50x15`

**Steps:**
1. Open Timeline and press `c` to open Compose with a non-empty draft body.
2. Confirm the AI command bar is visible by default between the message headers and body without narrowing the body editor.
3. Confirm the bar is one compact row with Translate, Style, quick actions, Undo, and an inline `Ask:` input.
4. Open the Translate dropdown with `Ctrl+T` and choose a language option.
5. Open the Style dropdown with `Ctrl+Y` and choose a style option.
6. Trigger typo-fix with `Ctrl+F`, shorten with `Ctrl+N`, and expand with `Ctrl+E` from the AI bar.
7. Press `Ctrl+K`, type a custom freeform instruction such as `make this warmer and translate it to Spanish`, then press `Enter`.
8. Wait for a suggestion or bounded failure.
9. Confirm the suggestion replaces the main body editor instead of appearing below it, and that prompt scaffolding such as `Current draft:` or model context explanations are not inserted into the suggestion text.
10. Confirm the Changes box shows inline word-level changes, not whole-line delete/add blocks, and the action hints sit directly below the Changes box without a large empty gap.
11. Press `Tab` to compare the original draft, then `Tab` again to return to the suggestion.
12. Edit the suggestion once, then press `Ctrl+Enter` to accept it into the compose body.
13. Confirm the AI command bar remains usable after accept by triggering another quick action such as `Ctrl+F`.
14. Press `Ctrl+Z` and confirm the previous body is restored.
15. Repeat once with AI unavailable, misconfigured, or using an invalid model.

**Expect:**
- The AI command bar is open by default without pushing the compose chrome off-screen or narrowing the body editor.
- The freeform instruction input is inline with the toolbar controls, not a separate `Ask:` row.
- Success path shows loading state, a bounded diff/change strip, and an editable suggestion in the main editor slot.
- AI review shows only the rewritten body in the suggestion editor; request context and `Current draft:` prompt echoes are stripped before display.
- The Changes box uses readable word-level highlighting with changed tokens separated clearly, and does not advertise unsupported change paging controls.
- The original body editor is hidden during AI review; the original draft is reachable with `Tab`, and the AI suggestion is reachable by pressing `Tab` again.
- Quick actions include typo-fix, translation dropdown, style dropdown, and length adjustments.
- Translate and Style actions use visible dropdown menus in the AI bar.
- Freeform instructions behave like a chat-style writing request over the current draft and produce an editable rewrite.
- `Ctrl+Enter` copies the accepted suggestion into the compose body, closes review mode, and leaves the AI command bar available for another action.
- `Ctrl+Z` restores the body from before the accepted rewrite.
- With AI unavailable, the default bar shows an `AI disabled` warning and does not advertise active rewrite controls.
- If the AI provider declines a rewrite or translation request, Herald shows a concise Compose status warning and does not place the refusal text in the editable suggestion area.
- Narrow sizes degrade cleanly without broken borders or hidden compose inputs.
- Failure path stays responsive and shows concise bounded feedback in compose status instead of panicking or flooding logs.

### TC-11A — Compose CC/BCC collapse and reveal

**Lane:** C
**Sizes:** `220x50`, `120x40`, `80x24`, `50x15`

**Steps:**
1. Open Timeline and press `c` to open a blank Compose screen.
2. Confirm empty CC and BCC rows are hidden and the header advertises `Ctrl+Alt+C` and `Ctrl+Alt+B`.
3. Press `Tab` repeatedly and confirm focus cycles through To, Subject, Body without stopping on hidden CC/BCC.
4. Press `Ctrl+Alt+C`, type a CC address, then press `Ctrl+Alt+B` and type a BCC address.
5. Confirm CC/BCC rows stay visible once non-empty, contact autocomplete still opens in those fields, and `Tab` includes visible CC/BCC fields.
6. Open or restore a draft/reply that already has CC and confirm the CC row is visible without pressing the hotkey.

**Expect:**
- Empty CC/BCC no longer consume vertical space.
- `Ctrl+Alt+C` and `Ctrl+Alt+B` reveal and focus their fields without interfering with `Ctrl+C` quit or plain text entry.
- Non-empty CC/BCC preserve existing send, draft-save, draft-restore, and autocomplete behavior.
- At `50x15`, the minimum-size guard is acceptable; returning to larger sizes restores the composed layout cleanly.

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
1. Open Timeline, press `c` to open Compose, and cycle focus through fields and preview.
2. Open Contacts and cycle list/detail/preview focus.
3. Capture each state.

**Expect:**
- Exactly one focused region looks active.
- List and detail borders are closed.
- Blank Compose fills the terminal height with no empty rows below the bottom key hints.
- Key hints match the focused region.

### TC-14A — Profile-aware command layer and text-entry safety

**Lane:** A, B
**Sizes:** `220x50`, `120x40`, `80x24`

**Steps:**
1. Open Timeline and press `c` to open Compose.
2. Type `q123?/` into the focused address field, then tab to the body and type `q123?/` again.
3. Type at least one macOS Option-generated character such as `™` or `¬` where available.
4. Press `Esc`, confirm Timeline returns, then verify `1` and `2` switch Timeline/Contacts in browse contexts, while `3` switches Calendar only when Calendar is advertised.
5. Return to Timeline and confirm `F1`, `F2`, and `F3` remain legacy mail tab aliases; `F4` opens Calendar only when Calendar is advertised.
6. Open Timeline search with `/`, type `q?/` into the query, and press `Ctrl+C` only after confirming the query text is editable.
7. Open Settings with `S`, choose each keyboard profile (Default, Vim, Emacs, Custom), verify the Custom Keymap path field appears only when Custom YAML is selected, and verify invalid custom keymap paths or unknown command IDs are reported without replacing the active working map.
8. Use a Custom keymap that extends Default with no `fields.compose.default_mode`, then another that sets `fields.compose.default_mode: normal`.

**Expect:**
- Plain `q`, digits, `?`, `/`, and Option-generated text remain in Compose text fields and do not quit, search, or switch tabs.
- `Esc` from Compose returns to the Timeline state that opened it after local Compose transient state is dismissed.
- `1/2` are the advertised mail tab keys in browse contexts; `3` joins the advertised tab keys only when Calendar is available, and `F1/F2/F3` remain supported as legacy mail aliases.
- Compose and browse hints use the active keyboard profile's resolved catalog instead of hand-written shortcut strings.
- A Custom keymap that remaps tab switching, Compose, reply, forward, archive, delete, re-classify, sidebar, logs, or chat shows the remapped primary keys in the bottom hint bar, title-row tabs, and `?` shortcut help.
- Timeline `c` opens blank Compose; `L` opens logs; `B` toggles the sidebar/folder browser; chat remains reachable through the advertised chat command without stealing text.
- Timeline search treats plain `q` as query text while `Ctrl+C` remains the universal quit path.
- Settings shows the Custom Keymap path field only for Custom YAML, while persisting `keyboard.profile` and any configured `keyboard.custom_keymap` without losing unrelated config fields.
- Custom keymaps that extend Default keep Compose insert-first until `fields.compose.default_mode` opts into a modal field mode.

### TC-14G — Vim profile field modes and visual selection

**Lane:** A, B
**Sizes:** `220x50`, `120x40`, `80x24`

**Steps:**
1. Set `keyboard.profile: vim`, launch demo mode, and open Compose.
2. Confirm the body field starts in normal mode when the Vim profile is active.
3. Press `i`, type text, press Escape, then press `A`, type more text, and press Escape.
4. Use `h/j/k/l` in normal mode to move within the body without inserting characters.
5. Press `v`, extend the selection, and copy with `y`.
6. Repeat the safe text-entry check in the prompt editor and Settings text fields.

**Expect:**
- The status/hint chrome shows `NORMAL`, `INSERT`, or `VISUAL` only when a modal field owns focus.
- Visual mode shows a visible anchor/cursor/highlight and does not appear as a dead mode.
- Prompt editor and Settings text fields preserve literal printable input unless their active field mode owns a Vim command.

### TC-14H — Modifier-aware key hint layers

**Lane:** A, B
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Open Timeline in demo mode and capture the default bottom hint bar.
2. In a terminal that reports key event types, press and hold Shift, then capture the bottom hint bar before releasing it.
3. Press `Shift+Down` from the Timeline list, capture the momentary Shift hint layer, then press a plain navigation key and confirm default hints return.
4. Press and hold Ctrl in a supported terminal, then capture the bottom hint bar; in unsupported terminals, press an existing Ctrl chord such as `Ctrl+D` or `Ctrl+R` and capture the short-lived fallback layer.
5. Press and hold Alt in a supported terminal, then capture the bottom hint bar; if the current context has no Alt-owned actions, confirm the default hints remain visible with a compact no-Alt-actions notice.
6. Repeat the same checks from Timeline preview, Cleanup summary, and Compose.
7. Open Compose, Timeline search, and a prompt/editor surface, type printable text including `?`, `/`, and an Option-generated character where available, and confirm no modifier hint state steals the text.
8. Open a delete or archive confirmation, press `?`, then return and confirm `y` remains the only confirming action while `Esc` cancels.

**Expect:**
- Modifier hint layers are presentation-only and never add a new shortcut.
- Default hints still include primary actions and `?: help` in browse contexts where help is available.
- The Shift layer advertises only existing shifted/uppercase actions valid for the current state.
- The Ctrl layer advertises only existing Ctrl actions valid for the current state.
- The Alt layer advertises existing Alt actions where present, or a compact no-Alt-actions note without hiding primary default actions.
- Multiple active modifiers prefer Ctrl, then Alt, then Shift.
- At `50x15`, the minimum-size guard still appears and recovers cleanly when resized larger.

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
4. Press `Esc` to return to Timeline, press `f`, and confirm Compose shows separate `Response` and `Original message` regions.
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
2. Open Timeline, press `c` to open Compose, press `Ctrl+A`, type the temp path plus the shared file prefix, and press `Tab`.
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
- Draft preview/list hints prioritize `E: edit draft`, `Ctrl+S: send draft`, `d: discard draft`, and `D: delete now`.
- `E` opens Compose from a highlighted draft, from draft preview focus, and from a collapsed thread that contains a draft.
- `Ctrl+S` sends a highlighted draft, draft preview, or collapsed thread draft without switching to Compose.
- Sending deletes the source draft only after send success; autosave replacement never deletes the previous draft before the new save succeeds.

### TC-14F — Compose signatures

**Lane:** A, B
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Configure `compose.signature.text` with a multiline signature and start Herald.
2. Open Timeline and press `c` to open blank Compose.
3. Return to Timeline, open a reply with `r`, and open a forward with `f`.
4. Open the quick-reply picker from a preview, choose a reply, and inspect the Compose body.
5. Open an existing draft with `E`.
6. Open Settings with `S`, find `Email Signature`, edit it, save, and open a new Compose screen.
7. Repeat the Compose and Settings captures at `80x24`; at `50x15`, confirm the minimum-size guard or compact recovery behavior.

**Expect:**
- Blank Compose, reply Compose, forward Compose, and quick replies visibly include the configured signature in the editable body with two empty lines before the signature.
- Blank Compose, reply Compose, forward Compose, and quick replies start the body cursor at the first editable line above the inserted signature so typing begins above it.
- Reply and forward signatures stay in the editable top-note body and do not alter the read-only original-message pane.
- Existing draft edits restore the saved body exactly and do not append another configured signature.
- Opening and leaving a blank Compose screen containing only the automatic signature does not create a draft.
- Settings exposes the multiline `Email Signature` field and saves future Compose openings without mutating the currently open draft.
- Reopening Compose does not duplicate a signature when the body already ends with the configured signature.

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

### TC-16B — Multi-recipient preview headers and reply-all

**Lane:** A
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Start `/tmp/herald --demo`.
2. Open Timeline and select the Cobalt Works reply thread.
3. Open the message preview for a Cobalt Works message whose loaded body has multiple `To` recipients and a `Cc` recipient.
4. Confirm the split preview header shows `From:`, `To:`, `Cc:`, `Date:`, `Subj:`, `Tags:`, and `Actions:` in that order when recipients exist.
5. Press `z` and confirm the full-screen preview shows the same loaded `To:` and `Cc:` recipient headers without showing `Bcc:`.
6. Press `r` from the message and confirm Compose opens as reply-all with the sender plus non-self `To` and `Cc` participants filled.
7. Return to Timeline, press `R` from the same message, and confirm Compose addresses only the original sender.

**Expect:**
- Demo fixtures include at least one Cobalt Works message with more than one `To` recipient and at least one visible `Cc` recipient.
- Split and full-screen preview headers show loaded `To:` and `Cc:` lines only after the body is loaded and only when those headers are non-empty.
- Preview headers never show `Bcc:`.
- Reply-all filters the current account and duplicate addresses while preserving sender, other primary recipients, and copied participants.
- Sender-only reply remains sender-only.
- At `50x15`, Herald shows the minimum-size guard and recovers when resized larger.

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
1. Toggle chat with `g`.
2. Toggle logs with `L`.
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
1. Press `?` from Timeline list, Timeline preview, Cleanup summary, Cleanup preview, Contacts list, chat, logs, and a confirmation prompt.
2. Scroll the help overlay with `j/k` or arrow keys when content is taller than the viewport.
3. Close the overlay with `Esc`, `?`, and `q` in separate passes.
4. In editable Compose fields, type `?` in To, Subject, Body, attachment path, and AI prompt inputs.
5. In Contacts, press `/`, type `? budget risk`, and confirm semantic results still work through the search input.

**Expect:**
- Plain `?` opens shortcut help, not semantic search, in Herald-owned browse and non-text contexts.
- Editable Compose fields preserve literal `?` as message text or path/prompt text and do not open shortcut help.
- At `220x50`, shortcut help appears as a compact centered modal over the current view, not a full-screen replacement.
- At `80x24`, shortcut help shrinks to fit without horizontal overflow.
- The overlay title names the current context and the body lists global, tab, pane, overlay, and mode-specific shortcuts.
- Compose key hints do not advertise `?: help` while editing text, and preservation mode still lists `Ctrl+O` only when reply/forward context exists.
- Overlay scroll state is bounded and resets when reopened from a different context.
- Closing help returns to the same tab/pane/overlay state without triggering the underlying key action.

### TC-18B — Settings compact overlay

**Lane:** A, B
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. From Timeline, press `S` to open settings.
2. Repeat from Compose, Cleanup, and Contacts to confirm the current screen remains visible behind the panel.
3. At `220x50`, capture the settings menu, confirm it lists `Accounts`, `AI`, `Sync & Cleanup`, `Keyboard`, `Theme Selection`, `Theme Editor`, and `Signature`, then close it with `Esc`.
4. At `80x24`, reopen settings and confirm the modal footer says `enter open` and `esc exit`, while the bottom hint bar says `enter: open category`, `/: filter`, and `esc: exit settings`.
5. Reopen settings, enter `Signature`, edit the multiline field, save, and confirm the menu returns without requiring account or AI fields.
6. Reopen settings, enter `Signature`, press `Esc`, and confirm Settings returns to the top-level menu before a second `Esc` exits.
7. Reopen settings, press `/`, confirm the filter prompt advertises `esc exit filter`, press `Esc`, and confirm the settings menu remains open.
8. Filter for `signature`, press `Esc` once to apply the filter, press `Esc` again to clear it, then press `Esc` a final time to close settings.
9. Resize to `50x15` while settings is open, then resize back to `80x24`.

**Expect:**
- At `220x50`, settings appears as a compact centered modal over the current view, not a full-screen replacement.
- The first panel state is the top-level settings menu, not the first account/source form field.
- Category selection opens only the chosen settings area.
- Menu-level hints describe `enter` as opening a category and `Esc` as exiting Settings; bottom screen hints switch away from the underlying tab while Settings is open.
- Saving a category writes settings, applies supported runtime updates, and returns to the top-level settings menu.
- At `80x24`, settings leaves a margin, fits without horizontal or vertical overflow, and the form scrolls inside the modal.
- At `50x15`, Herald shows the standard minimum-size guard instead of a clipped settings form.
- Returning from `50x15` to `80x24` restores the settings modal over the current screen without stale or clipped content.
- `Esc` exits or clears an active settings-menu filter before it exits Settings; from a category, `Esc` returns to the top-level menu without saving unsaved edits, and the next menu-level `Esc` exits Settings.
- First-run setup remains a linear fullscreen wizard and does not show the top-level settings menu.
- First-run setup's Theme step shows only the current theme picker with all available themes; local YAML install stays exclusive to in-app Theme Selection, while semantic theme roles, foreground/background fields, live preview, reset controls, and save-as-new-theme stay exclusive to in-app Theme Editor.

### TC-18C — Settings Accounts source manager

**Lane:** A, B
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch Herald with a deterministic multi-source config containing at least one mail source and one calendar source.
2. Press `S`, open `Accounts`, and capture the account/source list.
3. Select a mail account, press `Enter`, and capture the account detail form.
4. Return to `Accounts`, select a calendar-only account, press `Enter`, and capture the calendar detail form.
5. Return to `Accounts` and confirm the list includes both `Add account` and `Add calendar only` without an intermediate add-type submenu.
6. Choose `Add account`, confirm the provider-first form shows Gmail OAuth with paired Google Calendar enabled by default, and confirm the Gmail address plus connect/save action live in the same add flow without an early account-name prompt.
7. Choose `Add calendar only`, confirm the provider picker includes Google Calendar when experimental Google services are enabled, plus Fastmail, iCloud, Yahoo, and Custom CalDAV; confirm Fastmail/iCloud/Yahoo show short clickable app-password guidance near the CalDAV fields.
7. On a mail provider that supports calendar pairing, confirm the mail form includes `Also add calendar`; repeat on a mail-only provider and confirm the option is absent.
8. Attempt to delete a calendar-only account with `d` or `\` and confirm Herald asks for disconnect confirmation that says local cached mail/calendar data is removed while provider mail/calendars are not deleted.
9. Attempt fast delete with `D` or `|` on a non-final account and confirm Herald immediately removes the selected account from config and purges only that account's local cache.
10. Attempt to delete the final remaining mail account and confirm the operation is blocked with a bounded message.
10. Attempt to save an iCloud CalDAV source against a deterministic 401/403 fixture and capture the validation error modal.
11. Resize the open Accounts list or detail view to `50x15`, then resize back to `80x24`.

**Expect:**
- The Settings top-level menu uses `Accounts` instead of `Account setup`.
- `Settings > Accounts` groups configured sources by `account_id`, shows display name/provider/identity, and uses compact capability badges such as `Mail`, `Calendar`, or `Mail + Calendar`.
- The final Accounts rows are `Add account` and `Add calendar only`.
- Mail-capable account detail preserves the existing account setup fields and validates IMAP plus SMTP before saving.
- Calendar-capable account detail shows Google Calendar OAuth or CalDAV configuration fields and validates by listing calendars before saving.
- `Add account` creates a mail source and offers paired calendar setup only for supported providers; Gmail OAuth defaults Google Calendar on and validates both sources through one OAuth flow.
- `Add calendar only` creates a standalone Google Calendar OAuth or CalDAV source.
- Google Calendar setup starts Herald's OAuth flow instead of asking for Google's CalDAV endpoint, and missing Google OAuth client credentials produce a bounded OAuth configuration error without saving settings.
- CalDAV presets cover Fastmail, iCloud, and Yahoo with provider URL placeholders and app-password guidance links; Proton Calendar and Microsoft Calendar are documented but not shown as basic CalDAV presets.
- iCloud CalDAV Unauthorized validation failures keep the previous config active and explain Apple Account email, Apple app-specific password, password-reset regeneration, and two-factor authentication without exposing saved passwords.
- Disconnecting an account removes Herald config sources and local cache rows for that account only; it does not delete provider mail, provider calendars, or other accounts' cached data.
- Herald blocks deletion of the last configured mail source.
- At `50x15`, the standard minimum-size guard appears and resizing larger restores the same Accounts state cleanly.

### TC-24A — Theme selection and custom theme editing

**Lane:** A
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Start `/tmp/herald --demo`.
2. Press `S`, open `Theme Selection`, and capture the Theme Selection category.
3. Switch between `Inherited`, `Herald dark`, `Herald light`, `Jade Signal`, `Amber Furnace`, `Solar Paper`, and `Tokyo Dusk`; save each and confirm the visible chrome changes without leaving Settings.
4. Launch `env -u NO_COLOR TERM=xterm-256color COLORTERM=truecolor /tmp/herald --demo -theme jade-signal` and the same command shape with `-theme <valid-theme-file.yaml>`; confirm both start in the requested theme without writing config.
5. In `Theme Selection`, enter a valid local theme YAML path, save, reopen Theme Selection, and confirm the installed theme appears in the selector.
6. Open `Theme Editor`, edit one semantic role foreground with a hex value and one background with an `xterm:N` value, then use the foreground/background color pickers to try an xterm-grid value and an RGB value with instant preview, observe swatches/live preview, save as a new theme, then reopen Theme Editor.
7. Repeat with an invalid install path and invalid `-theme` value and confirm the bounded error stays inside Settings or fails launch loudly for the explicit CLI override.
8. Resize to `50x15` while Theme Selection is open, repeat while Theme Editor is open, then resize back to `80x24`.

**Expect:**
- Missing or inherited config keeps terminal-default foreground/background behavior.
- `Herald dark`, `Herald light`, and the diverse built-in themes visibly differ while preserving readable focused panel, status bar, hint bar, selection contrast, and full-screen background contrast.
- `scripts/regenerate-theme-screenshots.sh` refreshes both Timeline and Preview docs theme screenshots from `--demo -theme <name>` launches with `NO_COLOR` unset; `HERALD_THEME_SCREENSHOT_VIEW=preview scripts/regenerate-theme-screenshots.sh` remains available for a preview-only refresh, and both PNG sets are visually inspected rather than assumed good.
- Theme role text fields preserve literal `#`, `:`, digits, and letters; no browse shortcut fires while editing theme values.
- Theme color pickers update the same foreground/background values immediately: `/` from a manual color field opens that field's picker, xterm-grid moves emit `xterm:N`, RGB edits emit `#RRGGBB`, `m` switches picker mode, `i` restores `inherit`, and the selected swatch is marked with a contrasting in-cell marker without leaking ANSI text.
- Invalid installed themes and invalid install paths do not crash, overwrite config, or close Settings.
- At `50x15`, the standard minimum-size guard appears; returning to `80x24` restores the Theme Selection or Theme Editor modal.

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
1. Open Timeline and search for `Step 5: View inline images in full screen`.
2. Open the split preview and capture the image hint plus body links.
3. Press `z` to enter full-screen and capture the top of the document.
4. Scroll with app keys (`j`, `k`, `PgDn`, `PgUp`) until each inline image has appeared in the document flow.
5. In iTerm2 or Kitty raster mode, press `m` to release mouse capture, then use terminal-native scrollback to inspect whether image raster output displaced header/body text.
6. Repeat with the repo custom ttyd harness: `env -u NO_COLOR COLORTERM=truecolor PORT=7682 EVIDENCE_DIR=reports/ttyd-custom-image-preview tools/ttyd-image-harness/probe.sh`.
7. Repeat the custom ttyd harness with a full-screen app theme: `env -u NO_COLOR COLORTERM=truecolor HERALD_THEME=jade-signal PORT=7684 EVIDENCE_DIR=reports/ttyd-themed-image-preview tools/ttyd-image-harness/probe.sh`.
8. Repeat with stock ttyd smoke when comparing against the manual ttyd frontend: `env -u NO_COLOR COLORTERM=truecolor TTYD_MODE=stock PORT=7683 EVIDENCE_DIR=reports/ttyd-stock-image-preview tools/ttyd-image-harness/probe.sh`.
9. Repeat with `--demo -image-protocol=kitty` and confirm ANSI capture includes Kitty graphics `ESC_G` output.
10. In Kitty or Ghostty raster mode, scroll back and forth across multiple inline images and confirm old image placements are cleared before the current viewport is redrawn.
11. Repeat in Ghostty or a terminal with `TERM=xterm-ghostty` if available, a non-raster terminal, an iTerm2-compatible terminal if available, and SSH mode.
12. Run the standard resize cycle while full-screen preview is open.

**Expect:**
- The Creative Commons sampler fixture exposes four embedded inline images with different dimensions and HTML `cid:` placement.
- Split preview stays compact and does not promise image viewing when no full-screen image path is available.
- Full-screen preview renders text and inline images as one scrollable document below the pinned header.
- Raster images appear near their authored positions and do not push the header/title out of the visible app viewport or terminal scrollback.
- iTerm2-compatible terminals render bounded inline images using OSC 1337 when selected or auto-detected.
- Kitty-compatible terminals, including Ghostty, render bounded inline images using Kitty graphics protocol when selected or auto-detected.
- Kitty/Ghostty scrolling does not leave stale image placements over text or unrelated images.
- Custom ttyd + xterm image-addon mode reproduces browser-visible iTerm2 OSC 1337 image behavior more strictly and records screenshot plus pixel metrics for color-chart evidence and large raster image area.
- Custom ttyd with `HERALD_THEME=jade-signal` and `NO_COLOR` unset preserves real raster images while the app-level theme background is active; native image overlay escape tails are not styled, padded, or split by theme rendering.
- Stock ttyd smoke records screenshot plus pixel metrics for color-chart cells and at least one large photo region; its placement is not authoritative for acceptance.
- Non-raster local TUI shows OSC 8 `open image` links to localhost-served MIME inline image bytes.
- SSH auto mode avoids misleading localhost links and shows bounded placeholders unless the original email contains remote image URLs; forced `-image-protocol=iterm2` or `-image-protocol=kitty` emits the selected raster protocol.
- Remote HTML image URLs appear as readable OSC 8 links and Herald does not fetch them automatically.
- At `50x15`, the minimum-size guard appears and resizing back restores a clean full-screen preview.
- Test reports include terminal app/version, ttyd/frontend mode, selected app theme, selected image protocol mode, screenshots for raster modes, and ANSI captures where possible.

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

### TC-46 — Demo fixtures cover onboarding and public UI context

**Lane:** A
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Start `/tmp/herald --demo`.
2. Confirm a centered `Welcome to Herald Demo` overlay appears over the Timeline, explains the mailbox is synthetic, and points the user at the first onboarding email.
3. Press `j` and confirm the Timeline cursor does not move while the overlay remains visible.
4. Press `Enter`, `Space`, or `Esc` and confirm the overlay closes without opening the first email or toggling selection.
5. Confirm the top Timeline messages are `✉ Welcome to Herald` followed by `Step 1:` through `Step 9:` from Herald senders.
6. Open the welcome email and confirm the body explains what Herald is and that demo mode is synthetic.
7. Open Step 1 and confirm the body teaches `j/k` or up/down movement, `h/l`, arrows, `Tab`, and `Shift+Tab` between folders/Timeline/preview, preview opening with `Enter`, right arrow, `l`, or `Tab`, and mouse scrolling/clicking.
8. Open Step 2 and confirm the body explains reply, Markdown preview, preserved original formatting, rendered HTML, and plain-text fallback.
9. Open Step 3 and confirm at least two attachments are available and selection hints appear.
10. Open Step 4 and confirm the body explains text selection, full-screen preview, and `m` to release/restore mouse capture.
11. Open Step 5 and confirm inline image/full-screen instructions are present.
12. Open Step 7 and confirm cleanup rules, automation rules, custom prompts, and dry-run previews are explained.
13. Confirm supporting demo fixtures below the onboarding course use `Example:` subjects and avoid repetitive filler.
14. Switch to Cleanup, open sender details, and preview one message.
15. Switch to Contacts, open one contact detail, and open a recent email inline.

**Expect:**
- Demo startup shows a compact welcome overlay only in `--demo`, and dismissing it does not route the dismiss key to the underlying Timeline.
- Timeline starts with a Herald welcome message followed by explicit onboarding messages ordered Step 1 through Step 9.
- Supporting demo messages below the onboarding course are labeled as `Example:` fixtures and the mailbox remains focused.
- Preview bodies are specific instructional docs rather than generic lorem ipsum.
- Attachment, unsubscribe, HTML, inline image, cleanup, AI, semantic search, contacts, and MCP demo coverage remains represented in the fixture set.
- Contacts are populated from demo data and their recent emails open inline.

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
- The canonical scope is the five tapes under `demos/`; `demos/legacy/demo.tape` remains legacy.

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
2. Try `D`, `a`/`e`, `r`/`R`, `f`/`F`, `T`/`A`, `u`, `ctrl+q`, and star toggle.
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
3. Delete or archive one visible Cleanup message, confirming the prompt when using `d` or `Backspace`; reserve `D` and `Shift+Backspace` for approved immediate-delete checks.
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
- If a previous cache contains a last-known folder list, the sidebar may show that complete cached tree immediately while live IMAP status is still unsettled.
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
1. Open Timeline sender grouping with `G`.
2. Verify the hint bar, then move focus to the sidebar, select another folder, and return to Timeline.
3. Open `Settings > Sync & Cleanup`.
4. Launch the automation rule editor, prompt editor, and cleanup rules manager from that Settings category.
5. From the rule editor, complete a rule far enough to open dry-run preview; from Cleanup manager, open dry-run preview for cleanup rules.
6. Capture each state.

**Expect:**
- Timeline grouped rows remain keyboard-navigable after selecting a folder from the sidebar.
- The narrow hint bar points cleanup managers to Settings rather than `W`, `C`, or `P`.
- Rule and prompt overlays appear as compact centered modals launched from Settings and stay fully inside the viewport instead of clipping off the top or bottom.
- Rule, cleanup, and prompt overlays explain what they do, what saving or running them changes, and where the user can come back to review saved items or results.
- Dry-run preview overlays appear as compact centered modals, stay fully inside the viewport, show `[DRY RUN]`, and keep planned action/message rows readable.

### TC-37 — Cleanup overlays explain saved-item discovery

**Lane:** A, B
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Open `Settings > Sync & Cleanup`.
2. Launch the automation rule overlay.
3. Launch the prompt overlay.
4. Launch the cleanup rules manager.
5. Capture each overlay.

**Expect:**
- The automation rule editor explains that it creates future-mail automations rather than immediate cleanup.
- The automation rule editor appears as a compact centered modal, not a full-screen replacement.
- The automation rule editor shows a visible inventory or summary of saved automation rules in the same screen.
- `P` explains that prompts are reusable AI instructions and do nothing until used.
- `P` appears as a compact centered modal, not a full-screen replacement.
- `P` uses otherwise empty space for practical guidance: example prompt ideas, supported template variables, and a clear next step.
- `P` tells users to attach a prompt from Settings for automation or run one manually through MCP `classify_email_custom`.
- `P` tells users that prompt results are stored per email in custom category storage/MCP results.
- `P` shows a visible inventory or summary of saved prompts in the same screen.
- `C` explains that cleanup rules run on demand or on schedule and that saved cleanup rules live in that manager.
- `C` appears as a compact centered modal, not a full-screen replacement.

### TC-37A — Rules dry-run previews gate live actions

**Lane:** A, B
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Start Herald in demo mode and open `Settings > Sync & Cleanup`.
2. Open the automation rule overlay from Settings, create or use a prefilled rule, and open the dry-run preview.
3. Verify the preview rows, then save disabled with `s` or enable with `E` only after reading the confirmation.
4. Open the cleanup rules manager from `Settings > Sync & Cleanup`.
5. Press `p` to preview the selected cleanup rule and `r` to preview all enabled cleanup rules.
6. In normal mode, press `R` from the cleanup dry-run preview and stop at the live-run confirmation.
7. Repeat with `--dry-run`; press `R` and confirm the UI refuses live execution.

**Expect:**
- Automation and cleanup dry-run previews show matched messages, count, sender/domain/category, folder, and planned action.
- Automation and cleanup dry-run previews use the same compact centered modal treatment as Settings and Help.
- Dry-run previews never mutate mail, update `last_run`, write rule action logs, or call external actions.
- Cleanup archive/delete/move execution is unavailable until a preview is visible and the user explicitly confirms.
- In global `--dry-run` mode, the preview remains available but live cleanup run is blocked with relaunch guidance.
- At `50x15`, the minimum-size guard appears and resizing larger restores the dry-run preview cleanly.

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

### TC-38A — Multi-account active switching

**Lane:** A
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch Herald in deterministic multi-account demo mode.
2. Capture the Timeline with the sidebar visible.
3. Expand/collapse the Favorites rows for `All Inboxes`, `All Drafts`, and `All Sent`.
4. Move into a per-account child folder such as an account Inbox or Drafts row, press `Enter`, and capture the reloaded account-scoped folder.
5. Press `A` to open the account switcher overlay.
6. Move to another account, press `Enter`, and capture the reloaded account folder tree.
7. Repeat in normal single-account demo mode.

**Expect:**
- Multi-account demo shows a `Favorites` section with aggregate rows plus account children, followed by per-account folder sections.
- Account rows and favorite child rows render as `Name (email)` when the source address is known, truncating without losing the display name first.
- Favorites, per-account sections, and custom folder groups are collapsible, and navigating up/down never leaves the cursor on an unhighlighted header row.
- Per-account child folder rows switch to that concrete source and mailbox; same-named folders remain isolated by account.
- `All Inboxes` selects the existing All Accounts `INBOX` scope; ambiguous aggregate parents such as `All Drafts` and `All Sent` expand/collapse unless they can safely map to one concrete path.
- The switcher overlay names each source, shows status/error state, and `Esc` closes it without changing accounts.
- `Enter` switches the active account, restores that account's selected folder, and keeps same-named folders isolated.
- Stale async folder, status, and Timeline responses from a previous All Accounts or concrete-account scope cannot repaint the newly selected scope.
- Single-account demo keeps the existing sidebar/status chrome and does not advertise account switching.
- At `50x15`, the minimum-size guard appears and resizing larger restores the multi-account sidebar or switcher state cleanly.

### TC-38B — Unified inbox/search and account badges

**Lane:** A
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch Herald in deterministic multi-account demo mode.
2. Select `All Accounts` from the account rail or account switcher overlay.
3. Capture the unified Timeline and open a preview for messages from at least two accounts.
4. Run a search while `All Accounts` is selected and capture the search results.
5. Repeat in normal single-account demo mode.

**Expect:**
- [x] Multi-account demo shows an `All Accounts` entry; selecting it loads a unified inbox without changing single-account chrome.
- [x] Unified Timeline and search rows show a compact account badge or `Acct` column at `220x50` and `80x24`, and collapse cleanly at `50x15`.
- [x] Same-named folders and duplicate Message-IDs from different accounts remain distinct for selection, preview, reply/forward body loading, star/read/archive/delete routing, and stale-result filtering.
- [x] Search in the unified scope aggregates the visible account set and preserves account identity on results.
- [x] Single-account demo does not render account badges, all-account chrome, or changed key hints.

### TC-38C — Multi-account Compose From selection

**Lane:** A
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch Herald in deterministic multi-account demo mode.
2. Open blank Compose from an individual account and capture the `From` picker.
3. Switch to `All Accounts`, open a message from each account, and start reply/forward Compose from those messages.
4. Focus the `From` picker, change accounts, type literal `A`/`a` in the `To`, `Subject`, and body fields, and capture the result.
5. Repeat blank Compose in normal single-account demo mode.

**Expect:**
- [x] Multi-account Compose renders a compact `From` row with the selected account name/address.
- [x] Blank Compose defaults to the active real account; when browsing `All Accounts`, replies and forwards default to the selected message's source account.
- [x] Changing `From` routes send and draft operations through that selected source without changing the active browse account.
- [x] Literal `A`/`a` typed in Compose text fields remains text and does not open account switching or change the sending account.
- [x] Single-account Compose keeps the existing header layout and does not render account picker chrome.

### TC-38D — Calendar agenda read-only foundation

**Lane:** A, B when calendar cache rows are available
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch Herald in deterministic demo mode and dismiss the welcome overlay.
2. Press `3` or `F4` to open Calendar.
3. Capture the Agenda List with the selected event detail visible.
4. Move through events with `j/k` or arrow keys.
5. Press `Enter` to open the full Event Detail view, then `Esc` to return to the agenda without losing position.
6. Press `S` from Calendar, confirm Settings opens as the same compact overlay used elsewhere, press `Esc`, and confirm Calendar returns to the same view and selection.
7. Switch back to Timeline and Contacts, then repeat in a mail-only session with no calendar agenda backend.

**Expect:**
- Calendar appears as a durable title-row destination only when an agenda backend is available; mail-only sessions keep the existing Timeline/Contacts title row and `1-2: tabs` hints.
- The Agenda List is sorted by start time, shows each event's calendar/source label, and never exposes provider event IDs, CalDAV URLs, sync tokens, ETags, or OAuth details.
- The Agenda List uses a deliberate local calendar-month range, such as `May 1 - May 31`, falls back to the nearest valid event's calendar month when the current month is empty, and never renders malformed, zero-time, or absurdly long stale provider spans as historical rows such as `Dec 31` or `1950`.
- The Agenda List hides events that ended before the current local day by default; when hidden rows exist, it shows a compact `past events hidden` notice with a `[p] Show past` affordance, and pressing `p` reveals those rows until `p` hides them again.
- Google and CalDAV date-only/all-day provider events appear on their intended local calendar date, and exclusive all-day end dates do not duplicate the event on the following day.
- Switching from Agenda to Day, Week, or 3-Day anchors on the selected/current date instead of jumping to the first cached event; returning to Agenda uses the anchor's calendar month.
- The selected event detail shows title, time range, location, status, calendar/source, and notes in a structured read-only surface.
- `Enter` opens a full Event Detail view and `Esc` returns to the same selected agenda row.
- `S` opens Settings from Calendar, Settings hints replace the Calendar hints while the overlay is open, and closing Settings preserves the active Calendar view and selected event.
- Calendar hints are read-only and do not advertise RSVP, edit, create, or provider mutation actions.
- Timeline, Contacts, Compose, account switching, chat, settings, SSH, and MCP behavior remain unchanged.
- At `50x15`, the minimum-size guard appears instead of clipped calendar chrome, and resizing larger restores the agenda or detail state cleanly.

### TC-38E — Calendar day agenda drawer

**Lane:** A, B when calendar cache rows are available
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch Herald in deterministic demo mode and dismiss the welcome overlay.
2. Press `3` or `F4` to open Calendar, then press `d` to switch from Agenda List to Day Agenda.
3. Capture the Day Agenda with the selected event drawer visible.
4. Move through events on the current day with `j/k` or arrow keys, then move to the previous/next day with `h/l` or left/right arrows.
5. Press `a` to return to Agenda List, then press `d` again to confirm the selected event's day is restored.
6. Press `Enter` from Day Agenda to open the full Event Detail view, then `Esc` to return to the Day Agenda without losing position.

**Expect:**
- `d` switches to a read-only Day Agenda and `a` returns to Agenda List without changing the Calendar destination.
- Day Agenda shows only events for the selected day using the shared calendar time-grid foundation, preserves source/calendar labels through the selected-event drawer, and never exposes provider event IDs, CalDAV URLs, sync tokens, ETags, or OAuth details.
- On tall terminals, Day Agenda shows explicit `:30` rows using the same density rule as Week Time-Grid; standard-height terminals can keep hourly density when the half-hour grid would crowd the schedule.
- Long timed events render as continuous blocks across their occupied slots; all-day and multi-day events stay separate from the timed grid so they do not paint every hour of the day.
- The drawer shows title, local time, event timezone, location, status, calendar/source, mode, and notes for the selected event.
- `h/l` and left/right move between days without invoking mail navigation or mutation behavior.
- `Enter` opens the existing full Event Detail reader and `Esc` returns to the Day Agenda state.
- Hints advertise `d: day`, `a: agenda`, and `h/l: day` only in Calendar contexts, and no RSVP, edit, create, or provider mutation keys appear.
- At `50x15`, the minimum-size guard appears instead of clipped Day Agenda chrome, and resizing larger restores the Day Agenda or detail state cleanly.

### TC-38F — Calendar week time-grid

**Lane:** A, B when calendar cache rows are available
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch Herald in deterministic demo mode and dismiss the welcome overlay.
2. Press `3` or `F4` to open Calendar, then press `w` to switch from Agenda List to Week Time-Grid.
3. Capture the Week Time-Grid with the selected event inspector visible.
4. Move through visible week events with `j/k` or arrow keys, then move to the previous/next week with `h/l` or left/right arrows.
5. Press `d` to switch to Day Agenda for the selected event, then press `w` again to return to the selected event's week.
6. Press `Enter` from Week Time-Grid to open the full Event Detail view, then `Esc` to return to the Week Time-Grid without losing position.

**Expect:**
- `w` switches to a read-only Week Time-Grid and `a` returns to Agenda List without changing the Calendar destination.
- Week Time-Grid shows weekday columns or compact day bands with visible time labels, selected event state, and source/calendar labels without exposing provider event IDs, CalDAV URLs, sync tokens, ETags, or OAuth details.
- On tall terminals, Week Time-Grid shows explicit `:30` rows; standard-height terminals can keep hourly density when the half-hour grid would crowd the schedule.
- Long timed events render as continuous blocks across their occupied slots; guide dots remain visible in empty cells but do not cut through active event cells.
- Week Time-Grid shows a current-time marker with the local `HH:MM` only when the visible week contains today; past and future weeks, including deterministic demo weeks outside today, do not show a fake marker.
- Week Time-Grid uses Monday-Sunday calendar-week windows, such as `Mon May 25 - Sun May 31`, rather than a rolling seven-day range from the selected day.
- Settings can switch Week Time-Grid to Sunday-Saturday windows while Monday-Sunday remains the default.
- All-day and multi-day events render in a compact top row or summary, not as repeated hourly blocks across every day.
- The inspector shows title, local time, event timezone, location, status, calendar/source, mode, and notes for the selected event.
- `h/l` and left/right move between weeks without invoking mail navigation or mutation behavior.
- `Enter` opens the existing full Event Detail reader and `Esc` returns to the Week Time-Grid state.
- Hints advertise `w: week`, `d: day`, `a: agenda`, and `h/l: week` only in Calendar contexts, and no RSVP, edit, create, or provider mutation keys appear.
- At `50x15`, the minimum-size guard appears instead of clipped Week Time-Grid chrome, and resizing larger restores the Week Time-Grid or detail state cleanly.

### TC-38G — Calendar 3-Day Command view

**Lane:** A, B when calendar cache rows are available
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch Herald in deterministic demo mode and dismiss the welcome overlay.
2. Press `3` or `F4` to open Calendar, then press `t` to switch from Agenda List to 3-Day Command.
3. Capture the 3-Day Command view with the right command panel visible.
4. Move through visible events with `j/k` or arrow keys, then slide the three-day window with `h/l` or left/right arrows.
5. Press `w`, `d`, and `a` to confirm Week Time-Grid, Day Agenda, and Agenda List are still reachable, then press `t` to return to 3-Day Command.
6. Press `Enter` from 3-Day Command to open the full Event Detail view, then `Esc` to return without losing position.

**Expect:**
- `t` switches to a read-only 3-Day Command view and `a` returns to Agenda List without changing the Calendar destination.
- 3-Day Command shows today, tomorrow, and the next day with the same shared calendar time-grid foundation used by Day and Week, including visible time labels, selected event state, continuous long-event blocks, and source/calendar labels without exposing provider event IDs, CalDAV URLs, sync tokens, ETags, or OAuth details.
- On tall terminals, 3-Day Command shows explicit `:30` rows using the same density rule as Week Time-Grid; standard-height terminals can keep hourly density when the half-hour grid would crowd the schedule.
- The command panel shows the selected event, next-up event, open-slot summary, conflict summary, mode, and notes where available.
- `h/l` and left/right slide the three-day window without invoking mail navigation or mutation behavior.
- `Enter` opens the existing full Event Detail reader and `Esc` returns to the 3-Day Command state.
- Hints advertise `t: 3-day`, `w: week`, `d: day`, `a: agenda`, and `h/l: 3-day` only in Calendar contexts, and no RSVP, edit, create, or provider mutation keys appear.
- At `50x15`, the minimum-size guard appears instead of clipped 3-Day chrome, and resizing larger restores the 3-Day Command or detail state cleanly.

### TC-38H — Calendar full Event Detail and timezone foundation

**Lane:** A, B when calendar cache rows are available
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch Herald in deterministic demo mode and dismiss the welcome overlay.
2. Press `3` or `F4` to open Calendar, then press `Enter` from Agenda, Day, Week, or 3-Day to open full Event Detail.
3. Capture the full Event Detail at wide and standard sizes.
4. Move back with `Esc`, switch to another Calendar spatial view, and open the same selected event again.
5. Repeat with a mail-only session and confirm Calendar remains hidden.

**Expect:**
- Full Event Detail is read-only and shows event title, local time, canonical event timezone, at least one alternate timezone, location, status, calendar/source, organizer, attendees with RSVP state, recurrence, attachments, and notes where data is available.
- The detail view summarizes recurrence and attachment metadata without exposing provider event IDs, CalDAV URLs, raw sync tokens, raw ETags, OAuth details, or RSVP/create/provider-mutation actions.
- Event detail opened from Agenda, Day, Week, or 3-Day returns to the originating view without losing the selected event.
- Timezone rows make local time and event timezone distinct when they differ, and the alternate timezone row makes date-crossing cases visible.
- At `50x15`, the minimum-size guard appears instead of clipped Event Detail chrome, and resizing larger restores the full detail state cleanly.

### TC-38I — Calendar Search foundation

**Lane:** A, B when calendar cache rows are available
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch Herald in deterministic demo mode and dismiss the welcome overlay.
2. Press `3` or `F4` to open Calendar, then press `/` to switch to Calendar Search.
3. Type a query that matches an event title, location, attendee, organizer, notes, or recurrence text.
4. Capture the Search view with filtered results and the selected-event detail visible.
5. Move through results with `j/k`, press `Enter` to open full Event Detail, then `Esc` to return to the same Search result.
6. Press `Esc` again to clear Search and confirm Agenda List returns without losing the calendar destination.
7. Repeat in a mail-only session and confirm Calendar Search is not advertised.

**Expect:**
- Calendar Search is read-only, cache-backed, and source-scoped; it searches cached/demo calendar event metadata without direct provider fetches.
- Search results include matching event title, time, calendar/source label, and a compact matched-field hint without exposing provider event IDs, CalDAV URLs, sync tokens, raw ETags, OAuth details, or mutation controls.
- Searches match title, location, notes, organizer, attendee email/name, recurrence summary, attachment title, and calendar/source labels.
- `Enter` opens the existing full Event Detail reader and `Esc` returns to Search first, then clears Search back to Agenda List.
- Hints advertise `/ search`, `esc: clear search`, and read-only Calendar navigation only in Calendar contexts; literal `/`, query text, and `s` typed in Compose/search/editor text-entry surfaces remain text.
- At `50x15`, the minimum-size guard appears instead of clipped Search chrome, and resizing larger restores the Search state cleanly.

### TC-38J — Cross-source Search foundation

**Lane:** A, B when mail and calendar cache rows are available
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch Herald in deterministic demo mode or a deterministic multi-source fixture with at least one cached email and one cached calendar event sharing a search term.
2. Press `3` or `F4` to open Calendar, then press `x` to open Cross-Source Search.
3. Type a query that matches both a mail row and an event row.
4. Capture the Cross-Source Search view with blended results and the selected-result detail visible.
5. Move through results with `j/k`; confirm the detail pane switches between mail metadata and event metadata without exposing provider IDs.
6. Press `/` from the normal Calendar view and confirm Calendar Search remains event-only.
7. Repeat in a mail-only session and confirm the Calendar tab and cross-source search entry point are not advertised.

**Expect:**
- Cross-Source Search is read-only and cache-backed; it searches cached/demo mail and calendar metadata without direct IMAP, Google Calendar, or CalDAV provider fetches.
- Results include both `mail` and `event` typed rows when both caches match, with source/account context preserved.
- Event results keep the existing Event Detail reader pattern; mail results show sender, subject, folder, account/source, time, and match context without provider UIDs, raw event IDs, CalDAV URLs, sync tokens, raw ETags, OAuth details, or mutation controls.
- Stale search responses cannot repaint a newer query.
- Existing Timeline `/`, `/*`, `/b`, `?`, and Calendar `/` search behavior remains unchanged.
- At `50x15`, the minimum-size guard appears instead of clipped cross-source search chrome, and resizing larger restores the search state cleanly.

### TC-38K — Calendar Event Edit timezone foundation

**Lane:** A, B when calendar cache rows are available
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch Herald in deterministic demo mode or a cache-backed calendar fixture.
2. Press `3` or `F4` to open Calendar, then press `Enter` to open full Event Detail.
3. Press `e` to open Event Edit, change title, location, start/end, and event timezone fields, then capture the edit view.
4. Confirm the preview shows local time, event timezone, at least one alternate timezone, and a visible date-crossing note when applicable.
5. Press `Esc` to cancel once, then reopen Event Edit and press `Ctrl+S` to save.
6. Confirm the updated cached event appears in the Calendar list/detail without requiring a provider fetch.

**Expect:**
- Event Edit uses Herald's form/settings pattern with focused fields, validation rows, compact controls, and a live timezone preview.
- The primary event timezone field is near the start/end fields and saving a timezone is explicit; alternate display timezones are preview-only.
- Unsaved changes, validation errors, cancel, and cache-backed save success are visible to the user.
- The edit boundary writes to demo/cache state only in this stage; it does not advertise RSVP, recurrence-provider writes, create-event, Google Calendar mutation, CalDAV mutation, or daemon/MCP mutation APIs.
- Literal `e` typed in Compose, search prompts, and editor-like text fields remains text instead of opening Event Edit.
- At `50x15`, the minimum-size guard appears instead of clipped Event Edit chrome, and resizing larger restores the edit state cleanly.

### TC-38L — Calendar provider save and RSVP foundation

This case covers the first provider-backed mutation slice after the local/cache Event Edit model is stable. It proves edited events and RSVP responses write through the selected calendar provider before updating Herald's cache, while keeping failure and recurrence scope visible.

**Lane:** A, B when calendar provider fixtures are available
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch demo mode and a deterministic provider-backed calendar fixture using the local Google Calendar and CalDAV test harnesses.
2. Press `3` or `F4` to open Calendar, then press `Enter` to open full Event Detail.
3. Press `e`, change title/location/timezone fields, and press `Ctrl+S`.
4. Confirm the provider mutation succeeds before the cached event detail/list/search rows update.
5. Repeat with a fixture that forces a provider failure; confirm Event Edit stays open, the unsaved changes remain visible, and cached event rows do not change.
6. From Event Detail, press the RSVP response key and confirm the selected attendee response updates through the provider and then the cache.
7. Repeat text-entry checks in Compose, search prompts, and editor-like fields to confirm literal RSVP/edit shortcut keys remain text there.

**Expect:**
- Successful Event Edit saves call the provider mutation boundary first, refresh the cached scoped event only after success, and show a compact success status.
- Provider failures are explicit to the user, keep the edit open with unsaved values intact, and do not overwrite cached event rows.
- RSVP changes update attendee response state through the provider, then repaint Event Detail and source-scoped search rows from the saved event.
- Recurring events show an explicit `this event` recurrence-scope label for the first mutation slice; broader recurrence editing remains unavailable.
- Event Detail and Event Edit still hide provider event IDs, CalDAV URLs, raw sync tokens, raw ETags, OAuth details, and daemon/MCP mutation APIs.
- Literal `e` and the RSVP response shortcut typed in Compose, search prompts, and editor-like text fields remain text instead of firing calendar mutations.

### TC-38M — Calendar mutation conflict and recurrence-scope safety

This case covers the next provider-backed mutation hardening slice. It proves stale provider revisions and unsupported recurrence scopes fail visibly without rewriting cached calendar state or silently applying a broader recurrence edit than the user requested.

**Lane:** A, B when calendar provider fixtures are available
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch demo mode and a deterministic provider-backed calendar fixture whose cached event ETag is stale relative to the provider.
2. Press `3` or `F4` to open Calendar, press `Enter` for Event Detail, then press `e` to open Event Edit.
3. Change any editable field and press `Ctrl+S`.
4. Confirm Event Edit stays open, the unsaved values remain visible, the status names a provider conflict, and the cached event row remains unchanged.
5. Repeat the conflict path for RSVP from Event Detail.
6. Exercise a recurring event and confirm the only available mutation scope is explicitly labeled `this event`; unsupported wider scopes are rejected instead of being silently downgraded.

**Expect:**
- Provider `409 Conflict` or `412 Precondition Failed` responses map to a typed calendar mutation conflict that callers can detect without exposing raw provider URLs, ETags, sync tokens, or OAuth details.
- Conflict failures keep Event Edit open with unsaved values intact and leave cached agenda/detail/search rows unchanged.
- RSVP conflict failures show a compact conflict status and leave cached attendee response state unchanged.
- Unsupported recurrence scopes fail before provider mutation and do not update cache.
- `this event` remains the only advertised recurring-event mutation scope until broader recurrence editing is deliberately implemented.
- At `50x15`, the minimum-size guard appears instead of clipped conflict or recurrence-scope chrome, and resizing larger restores the edit state cleanly.

### TC-38N — Calendar selected attendee and recurrence edits

This case covers the selected calendar mutation slice that extends Event Edit beyond title, time, timezone, location, and notes. It proves attendee-list and this-event recurrence-rule edits stay visible, provider-backed, cache-safe, and bounded.

**Lane:** A, B when calendar provider fixtures are available
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch demo mode or a deterministic provider-backed calendar fixture with at least one event that has attendees and an `RRULE`.
2. Press `3` or `F4` to open Calendar, press `Enter` for Event Detail, then press `e` to open Event Edit.
3. Confirm Event Edit shows editable `Attendees` and `Recurrence` fields without exposing provider IDs, ETags, CalDAV URLs, raw sync tokens, OAuth details, or daemon/MCP mutation APIs.
4. Change the attendees field using semicolon-separated entries such as `Mina Park <mina@example.com> accepted; ops@example.com tentative optional`.
5. Change the recurrence field using provider-style recurrence lines such as `RRULE:FREQ=WEEKLY;BYDAY=TU,TH`, then press `Ctrl+S`.
6. Confirm the saved Event Detail, Calendar list/search rows, and provider-backed cache state reflect the edited attendee list and recurrence summary only after provider success.
7. Repeat with provider failure or stale revision and confirm unsaved attendee/recurrence edits remain visible while cached rows stay unchanged.

**Expect:**
- Event Edit renders attendee and recurrence fields in the same compact form language as the existing title/time/timezone fields.
- Attendee edits preserve name, email, RSVP state, and optional markers in a keyboard-editable text format.
- Recurrence edits are limited to this-event recurrence rules; broader recurrence-scope edits and create-event flows remain deferred.
- Save success, provider failure, recurrence scope, and timezone preview remain explicit to the user.
- Literal attendee/recurrence text typed in Compose, search prompts, and editor-like text fields remains text instead of firing calendar edit actions.
- At `50x15`, the minimum-size guard appears instead of clipped selected-mutation chrome, and resizing larger restores the edit state cleanly.

### TC-38O — Calendar selected reminder edits

This case covers the selected reminder mutation slice in Event Edit. It proves reminder override edits use the same provider-backed, cache-after-success path as other calendar mutations without introducing create-event UI or broader recurrence editing.

**Lane:** A, B when calendar provider fixtures are available
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch demo mode or a deterministic provider-backed calendar fixture with at least one event that has reminder overrides.
2. Press `3` or `F4` to open Calendar, press `Enter` for Event Detail, then press `e` to open Event Edit.
3. Confirm Event Edit shows an editable `Reminders` field without exposing provider IDs, ETags, CalDAV URLs, raw sync tokens, OAuth details, or daemon/MCP mutation APIs.
4. Change the reminders field using semicolon-separated entries such as `popup 10m; email 1h`, then press `Ctrl+S`.
5. Confirm the saved Event Detail, Calendar list/search rows, and provider-backed cache state reflect the edited reminder overrides only after provider success.
6. Repeat with provider failure or stale revision and confirm unsaved reminder edits remain visible while cached rows stay unchanged.

**Expect:**
- Event Edit renders reminder overrides in the same compact form language as title/time/timezone/attendee/recurrence fields.
- Reminder edits preserve method and minutes-before-event in a keyboard-editable text format.
- Reminder editing stays scoped to existing events; create-event flows and broader recurrence-scope edits remain deferred.
- Save success, provider failure, recurrence scope, and timezone preview remain explicit to the user.
- Literal reminder text typed in Compose, search prompts, and editor-like text fields remains text instead of firing calendar edit actions.
- At `50x15`, the minimum-size guard appears instead of clipped reminder-edit chrome, and resizing larger restores the edit state cleanly.

### TC-38P — Calendar Meeting Prep command-center foundation

This case covers the first read-only command-center slice after Cross-Source Search. It proves Event Detail can open a Meeting Prep view that blends selected-event context with related cached mail and nearby cached events without direct provider reads or mutations.

**Lane:** A, B when mail and calendar cache rows are available
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch demo mode or a deterministic fixture with a calendar event and at least one cached email sharing the event title, organizer, attendee, or location.
2. Press `3` or `F4` to open Calendar, then press `Enter` for Event Detail.
3. Press `p` to open Meeting Prep and capture the view.
4. Confirm the view lists the selected event, related cached mail, nearby cached events, and the visible query terms used for matching.
5. Press `Esc` and confirm Herald returns to Event Detail without losing the selected event.
6. Type literal `p` in Compose, Calendar Search, and editor-like prompts and confirm it stays text.

**Expect:**
- Meeting Prep is read-only and cache-backed; it does not directly call IMAP, Google Calendar, or CalDAV providers.
- Related mail shows sender, subject, folder/account context, and time without exposing provider UIDs, raw event IDs, CalDAV URLs, sync tokens, raw ETags, OAuth details, or mutation controls.
- Nearby events show event title, time, and calendar/source labels without exposing provider internals.
- `Esc` returns to Event Detail, and `r` refreshes Meeting Prep context for the same selected event.
- Literal `p` typed in Compose, search prompts, and editor-like text fields remains text instead of firing Meeting Prep.
- At `50x15`, the minimum-size guard appears instead of clipped Meeting Prep chrome, and resizing larger restores the prep state cleanly.

### TC-38Q — Calendar Travel Buffer command-center foundation

This case covers the next read-only command-center slice after Meeting Prep. It proves Event Detail can open a Travel Buffer view that blends selected-event context with cached travel-related mail and nearby event gaps without direct provider reads or calendar mutations.

**Lane:** A, B when mail and calendar cache rows are available
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch demo mode or a deterministic fixture with a calendar event that has a real location plus cached travel mail such as itinerary, train, hotel, airport, or rideshare context.
2. Press `3` or `F4` to open Calendar, then press `Enter` for Event Detail.
3. Press `b` to open Travel Buffer and capture the view.
4. Confirm the view lists the selected event, buffer suggestions, travel-related cached mail, nearby event gaps, and the visible query terms used for matching.
5. Press `Esc` and confirm Herald returns to Event Detail without losing the selected event.
6. Type literal `b` in Compose, Calendar Search, and editor-like prompts and confirm it stays text.

**Expect:**
- Travel Buffer is read-only and cache-backed; it does not directly call IMAP, Google Calendar, or CalDAV providers.
- Buffer suggestions name the reason for extra time, such as travel mail signals or tight nearby-event gaps, without silently changing the calendar.
- Related mail shows sender, subject, folder/account context, and time without exposing provider UIDs, raw event IDs, CalDAV URLs, sync tokens, raw ETags, OAuth details, or mutation controls.
- Nearby events show event title, time, and calendar/source labels without exposing provider internals.
- `Esc` returns to Event Detail, and `r` refreshes Travel Buffer context for the same selected event.
- Literal `b` typed in Compose, search prompts, and editor-like text fields remains text instead of firing Travel Buffer.
- At `50x15`, the minimum-size guard appears instead of clipped Travel Buffer chrome, and resizing larger restores the buffer state cleanly.

### TC-38R — Calendar AI Summary command-center foundation

This case covers the final read-only command-center slice after Travel Buffer. It proves Event Detail can open an AI Summary view that blends selected-event context with cached related mail and nearby cached events without direct provider reads or calendar mutations.

**Lane:** A, B when mail and calendar cache rows are available
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch demo mode or a deterministic fixture with a calendar event, cached related mail, and nearby cached events.
2. Press `3` or `F4` to open Calendar, then press `Enter` for Event Detail.
3. Press `s` to open AI Summary and capture the view.
4. Confirm the view lists the selected event, summary bullets, action items, source counts, and the visible query terms used for matching.
5. Press `Esc` and confirm Herald returns to Event Detail without losing the selected event.
6. Type literal `s` in Compose, Calendar Search, and editor-like prompts and confirm it stays text.

**Expect:**
- AI Summary is read-only and cache-backed; it may use the configured AI client to summarize cached context, but it does not directly call IMAP, Google Calendar, or CalDAV providers.
- When AI is unavailable or errors, the deterministic cached fallback still renders bounded summary bullets instead of leaving the view blank.
- Summary bullets and action items reference cached mail/event context without exposing provider UIDs, raw event IDs, CalDAV URLs, sync tokens, raw ETags, OAuth details, or mutation controls.
- `Esc` returns to Event Detail, and `r` refreshes AI Summary context for the same selected event.
- Literal `s` typed in Compose, search prompts, and editor-like text fields remains text instead of firing AI Summary.
- At `50x15`, the minimum-size guard appears instead of clipped AI Summary chrome, and resizing larger restores the summary state cleanly.

### TC-38S — Calendar design parity rail, range headers, and screen comparisons

This case covers the design-parity pass for the Calendar reference screens `01` through `04`. It proves the real Herald app visually tracks the reference mockups closely enough to compare side-by-side while preserving Herald terminal constraints and source-platform redaction.

**Lane:** A, B when calendar cache rows are available
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch Herald in deterministic demo mode with `HERALD_THEME=sonokai-signal`.
2. Press `3` or `F4` to open Calendar, then capture Agenda List, Week Time-Grid, Day Agenda, and 3-Day Command.
3. For each screen, capture a real app PNG and place it beside the corresponding reference mockup from `docs/superpowers/specs/2026-05-23-calendar-tui-roadmap-assets/`.
4. In each view, confirm the left calendar rail groups calendars by account/provider, shows colored calendar markers, and allows toggling visible calendars without exposing provider internals.
5. In each view, confirm the mini month bolds days with visible events while empty days stay regular weight and the selected day remains readable.
6. In each view, confirm the top range header states the active day, week, 3-day window, or agenda range and advertises `h/l` plus left/right movement.
7. Press `h/l`, left/right, `tab`, `shift+tab`, `ctrl+u`, and `ctrl+d` in the main schedule and inspector/drawer panels.
8. Resize to `50x15`, then back to `80x24` and `220x50`.

**Expect:**
- Side-by-side report evidence shows the real Sonokai Signal app is visually close to the reference mock in layout, density, panel proportions, source rail, range header, and color-marker treatment.
- Intentional differences are limited to terminal constraints, Herald navigation conventions, available provider data, and accessibility/readability requirements.
- Calendar rails never expose provider event IDs, CalDAV URLs, sync tokens, ETags, OAuth details, or raw scoped refs.
- The mini month bolds event-bearing days, keeps no-event days at regular weight, and hidden calendar filters remove their event-day emphasis.
- `up/down` and `j/k` traverse visible events across day boundaries where the active screen shows multiple days; Day Agenda can move to the next or previous day with visible events from the boundary row.
- `ctrl+u/ctrl+d` page the focused panel, and optional PageUp/PageDown aliases behave the same when the terminal reports them.
- `q`, `ctrl+c`, settings, logs, chat, help, and tab switching keep the global Herald behavior from Calendar focus.
- At `50x15`, the minimum-size guard or compact fallback appears instead of clipped calendar chrome, and resizing larger restores the same Calendar view.

### TC-38T — Calendar notes, RSVP, and invitation actions

This case covers the interaction polish required after the design-parity screens are visible. It proves HTML notes are readable, RSVP is explicit, pending events are highlighted, and email invitations can be routed into a chosen writable calendar.

**Lane:** A, B when calendar cache rows and invitation mail fixtures are available
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch demo mode or a deterministic fixture with HTML calendar notes, Markdown notes, a pending RSVP event, and an email containing `text/calendar` or an `.ics` attachment.
2. Open Calendar and navigate through Week, Day, 3-Day, Agenda, full Event Detail, Search, and Cross-Source Search.
3. Confirm the event with attendee RSVP `needs-action` is visibly marked in every calendar list/detail surface.
4. Open the RSVP action picker and choose accept, tentative, and decline against the deterministic provider or demo mutation boundary.
5. Open the event with HTML notes and confirm notes render as readable terminal text with paragraphs, lists, links, meeting URLs, dial-in numbers, and emphasis.
6. Return to Timeline, open the invitation email, and confirm the preview header action row advertises `i create calendar event` beside the existing unsubscribe, attachment, and hide-future-mail affordances.
7. Trigger Create Calendar Event, choose a configured writable calendar from the picker when more than one exists, and confirm duplicate ICS UIDs offer update or skip instead of silently duplicating events.

**Expect:**
- Raw HTML tags do not appear in completed calendar notes surfaces.
- RSVP actions are explicit, provider-backed where available, and update cached state only after provider/demo success.
- Read-only calendars show RSVP state but disable response actions with a visible reason.
- Invitation emails expose Create Calendar Event only when a parseable invitation exists, and mail-only or no-writable-calendar sessions explain why the action is unavailable.
- ICS parsing preserves summary, description, location, start/end, timezone, organizer, attendees, recurrence, attachments, reminders, UID, and sequence/revision where present.
- Literal RSVP and invitation shortcut characters typed in Compose, search prompts, and editor-like text fields remain text instead of firing calendar actions.
- At `50x15`, RSVP and invitation modals or pickers show the minimum-size guard or compact fallback instead of clipped controls.

### TC-39 — First-run wizard chrome and size guard

**Lane:** F
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch Herald with a missing temp config path.
2. Capture the account-selection step at each size.
3. Resize down to `50x15`.

**Expect:**
- Wizard uses Herald-branded chrome rather than raw unframed form output.
- Copy clearly distinguishes browser-based Google setup from credential-based mail providers.
- Default account choices include Gmail OAuth, Standard IMAP, Gmail IMAP App Password, ProtonMail Bridge, Fastmail, iCloud, and Outlook, without experimental labels on IMAP-based presets.
- At `220x50` and `80x24`, the form is centered and fully readable.
- At `50x15`, Herald shows the minimum-size resize message instead of clipped fields.

### TC-40 — Standard IMAP credentials stay labeled and navigable

**Lane:** F
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. From the first-run wizard, choose `Standard IMAP`.
2. Advance to the credentials step.
3. Capture before and after moving focus through the first three inputs.
4. Press `Esc` from the empty credentials step and capture the result.
5. Return to the credentials step, press `Shift+Tab` from the first control, and capture the result.

**Expect:**
- The active top input still has visible context; the user can tell it is the email field.
- `Password`, `IMAP Host`, `IMAP Port`, `SMTP Host`, and `SMTP Port` remain readable.
- Hints match the current control.
- `Esc` returns to the previous wizard screen without showing required-field validation errors for the empty credentials form.
- `Shift+Tab` can cross back to the previous wizard screen without showing required-field validation errors for the empty credentials form.
- At `50x15`, Herald falls back to the minimum-size guard rather than clipping later fields off-screen.

### TC-40A — IMAP presets are pre-populated

**Lane:** F
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. From the first-run wizard, choose `ProtonMail Bridge`.
2. Capture the credentials step.
3. Repeat for Fastmail, iCloud, Outlook, and Gmail advanced server settings.

**Expect:**
- ProtonMail Bridge shows `127.0.0.1`, IMAP port `1143`, and SMTP port `1025` in editable fields before the user types.
- Fastmail, iCloud, Outlook, and Gmail advanced fields show their known IMAP/SMTP host and port defaults before the user types.
- Editing any preset field keeps the user's manual value unless the field is blank or still matches the previous preset.

### TC-41 — Gmail OAuth express path and IMAP guidance

**Lane:** F
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch Herald with a missing temp config path.
2. Capture the account-selection step and confirm Gmail OAuth is visible alongside IMAP provider choices.
3. Choose `Gmail (IMAP + App Password)`.
4. Capture the Gmail IMAP guidance step.
5. Toggle advanced server editing and capture again.
6. Return to Account Type, choose `Gmail OAuth`, and capture the compact Google account guidance and wait screen.
8. Simulate or perform Google consent cancellation and capture the resulting setup error state.
9. Simulate an OAuth wait timeout and capture the guidance state.

**Expect:**
- Gmail OAuth is visible in default first-run onboarding.
- The Gmail OAuth provider step shows Mail enabled, Google Calendar enabled by default, optional Google identity, and no IMAP/SMTP host/port fields.
- The Gmail OAuth provider step does not add a separate `Connect Google` button; completing the step starts the access verification flow.
- The OAuth wait screen remains centered and shows an unboxed browser-auth prompt: `Click: [here] or copy this link to the browser:`, where `[here]` is an OSC 8 terminal hyperlink and a short `http://localhost:<port>/authorize` URL remains visible for copying.
- OAuth wait hints include local cancel behavior, and `Esc`/`q` cancellation returns a clear "settings were not saved" result.
- Google consent cancellation reports authorization cancelled, does not write the config file, and returns to the populated Google account setup so the email can be corrected.
- OAuth timeout mentions that Google test-app screens require choosing `Continue` and that `Back to safety` does not authorize Herald.
- Gmail IMAP guidance includes Gmail server defaults and links or copy for IMAP/App Password setup.
- Gmail IMAP is described as the fallback app-password setup path, with a note that Workspace may require OAuth.
- Advanced server fields are hidden until explicitly requested.

### TC-41G — Gmail API OAuth core source

**Lane:** F
**Sizes:** `220x50`, `80x24`, `50x15`

This case covers the Gmail API mail source behind Gmail OAuth. It proves the transport swap remains invisible to normal mail workflows while preserving Gmail IMAP app-password setup as a compatibility path.

**Steps:**
1. Configure a Gmail OAuth source that normalizes to a Gmail mail source with `provider: gmail` and Google token metadata. Older `provider: gmail_api` configs remain a compatibility alias.
2. Launch Herald and open Timeline on `INBOX`.
3. Open a message preview, mark it read/unread, star/unstar it, archive or trash a test message, and send a harmless self-addressed test message.
4. Repeat setup with Gmail IMAP App Password, and verify older `provider: gmail_api` configs still open through the Gmail API compatibility alias.

**Expect:**
- Gmail API OAuth uses narrower Gmail API mail access for core sync, body reads, mutations, and send.
- Gmail API OAuth draft autosave, list, edit, discard, and direct send use Gmail API draft endpoints without exposing raw draft IDs.
- Gmail API OAuth repeats Timeline sync using Gmail history polling when a cursor exists, falls back cleanly when Gmail reports the cursor is too old, and reflects smoke-created read/star/trash changes without exposing provider cursor IDs.
- Gmail API OAuth handles paginated list/draft/history responses, bounded provider retry for 429/5xx, and CC/BCC plus attachment MIME sends without changing visible Compose behavior.
- Timeline, preview, search, compose send, and scoped refs behave the same as other mail sources and do not expose raw Gmail message IDs, label IDs, OAuth tokens, or provider URLs.
- Delete moves the message to Gmail Trash rather than permanently deleting it.
- Gmail App Password still routes through the IMAP adapter; `provider: gmail_api` remains accepted only as a compatibility alias for the Gmail API adapter.

### TC-41A — First-run validates account before preferences

**Lane:** F
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch first-run setup with a missing temp config path.
2. Configure a provider with intentionally failing IMAP credentials and valid-looking SMTP fields.
3. Attempt to connect the account and capture the validation result before any AI, sync, theme, keyboard, or signature steps appear.
4. Repeat with valid-looking IMAP fields and intentionally failing SMTP credentials.
5. Repeat with mock or live valid IMAP and SMTP credentials.

**Expect:**
- The wizard shows a validation-in-progress state immediately after the account details step.
- Validation progress and failure states render inside the Herald setup box, not as unstyled terminal text.
- If IMAP fails, the config path remains missing or empty, the user sees a clear IMAP failure, and Enter returns to the populated credential step.
- If SMTP fails, the config path remains missing or empty, the user sees a clear SMTP failure, and Enter returns to the populated credential step.
- If both pass, Herald advances to optional preferences; the final save writes the validated config and proceeds to the inbox.
- At `50x15`, validation and error states use the minimum-size guard rather than clipped modal content.

### TC-41B — First-run AI defaults and advanced preference scoping

**Lane:** F
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Complete or mock a successful account validation so the first-run preferences step opens.
2. Capture the compact Advanced settings review with Theme, AI, Keyboard, Offline Cache, Signature, `Enter Herald`, and `Customize setup`.
3. Choose `Customize setup`, capture the default Ollama AI step, then select custom Ollama and capture the chat and embedding model selectors.
4. Attempt to save Ollama settings with one missing chat model and one missing embedding model.
5. Repeat with both models installed or mocked as installed.
6. Continue through customized first-run preferences and capture the offline-cache, keyboard, theme, and signature steps.
7. Launch the in-app Settings panel and open `AI` and `Sync & Cleanup`.
8. Simulate an existing saved Ollama config whose model is no longer installed.

**Expect:**
- The default Ollama step names `gemma3:4b` and `nomic-embed-text-v2-moe`, warns that the recommended defaults are comfortable with at least 16GB RAM, and says 8GB can work more slowly.
- Custom Ollama setup offers curated chat options including `gemma3:4b`, `qwen3.5:0.8b`, `llama3.2:1b`, `llama3.2:3b`, and a freeform custom model name, with downgrade guidance that flags `llama3.x` as weaker for translation.
- Custom Ollama setup offers curated embedding options including `nomic-embed-text-v2-moe`, `nomic-embed-text`, `all-minilm`, `mxbai-embed-large`, `bge-m3`, and a freeform custom model name.
- Missing first-run Ollama models block the final save, leave the config path missing or unchanged, and show exact `ollama pull <model>` commands in the setup chrome.
- Installed or mocked-installed Ollama models allow first-run setup to continue and save normally.
- A previously saved but now unavailable Ollama config does not block cached/offline startup; the status chip shows `AI down`, AI actions are disabled, Settings > AI shows install commands, and Save Disabled writes AI-off config.
- First-run preferences do not show `Poll Interval`, `Enable IMAP IDLE`, `Reclaim offline cache storage`, or `Auto-Cleanup Schedule`.
- In-app Settings still exposes those advanced controls under `Sync & Cleanup`.

### TC-41C — In-app account settings keep the previous account on validation failure

**Lane:** F
**Sizes:** `220x50`, `80x24`, `50x15`

**Steps:**
1. Launch Herald with a working demo or test config and open Settings with `S`.
2. Open `Account setup`, change the account to failing IMAP or SMTP values, and save.
3. Capture the error modal and then dismiss it.
4. Confirm the previous account view remains active and the original config file contents are unchanged.

**Expect:**
- Account settings show a validation-in-progress state before replacing runtime account state.
- Failed validation shows a compact centered error modal over the current Herald screen.
- The previous config/backend/SMTP client remain active after failure.
- Non-account settings categories still save normally without account validation.

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

**Lane:** A, B, G
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Open Timeline and select an email whose loaded body exposes `List-Unsubscribe`.
2. Open the email preview and capture the preview plus the bottom hint bar.
3. Open a second Timeline preview whose loaded body does not expose `List-Unsubscribe`.
4. Capture the preview plus the bottom hint bar again.
5. In automated virtual-lab coverage, use `ScenarioUnsubscribeHeaders` and assert the `one-click`, `mailto`, and `no-header` fixture variants.

**Expect:**
- The preview header includes `Tags:` and `Actions:` rows.
- With `List-Unsubscribe`, the preview metadata and hint bar both advertise `u: unsubscribe` and `H: hide future mail`.
- Without `List-Unsubscribe`, the preview metadata and hint bar do not advertise `u`, but still advertise `H: hide future mail`.
- End-user copy does not use `hard unsubscribe` or `soft unsubscribe`.
- Virtual-lab assertions do not contact live or external unsubscribe endpoints; they only prove parsed-header availability and visible affordances.
- A first-time user can identify the available list/sender action from the preview itself without prior knowledge.

### TC-44 — Timeline sender/domain groups preserve cleanup actions

**Lane:** A, B, G
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Open Timeline and press `G` until sender grouping is active.
2. Capture the grouped list and hint bar.
3. Press `G` again until domain grouping is active and capture the grouped list.
4. Open a grouped row preview for an email whose loaded body exposes `List-Unsubscribe` and capture the preview plus the hint bar.
5. Open a grouped row preview for an email whose loaded body does not expose `List-Unsubscribe` and capture again.
6. In automated virtual-lab coverage, use `ScenarioUnsubscribeHeaders` and assert the same `one-click`, `mailto`, and `no-header` availability rules as normal Timeline.

**Expect:**
- Sender and domain grouped rows are visible in Timeline without a Cleanup tab.
- Grouped Timeline previews include `Tags:` and `Actions:` rows in the preview header.
- Grouped Timeline previews use the same availability rules as normal Timeline preview: `u` appears only when `List-Unsubscribe` exists, while `H` remains visible when a sender exists.
- Delete/archive confirmations describe sender/domain groups instead of threads.
- End-user copy does not use `hard unsubscribe` or `soft unsubscribe`.
- Virtual-lab assertions do not contact live or external unsubscribe endpoints; daemon/MCP one-click execution is covered separately with a local test server.

### TC-49 — Email preview hides long link destinations behind OSC 8 labels

**Lane:** A
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Launch Herald in demo mode.
2. Open Timeline and search for `Example: Link rendering stress preview`.
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
2. Open Timeline and search for `Example: Rich HTML rendering showcase`.
3. Open the split preview, capture it, then press `z` and capture full-screen mode.
4. Press `G` to cycle sender/domain grouping, locate the same sender/message, and capture the grouped Timeline preview.
5. Open Contacts, select `Preview Lab`, open the `Example: Rich HTML rendering showcase` email inline, and capture the preview.
6. Resize to `80x24` and `50x15`, then back to `220x50`, capturing the affected preview surface at each size.

**Expect:**
- Timeline, grouped Timeline, Contacts, and full-screen previews all show the HTML-derived body rather than stale plain-text fallback.
- Visible preview text preserves readable structure: `HTML preview quality`, list bullets, `Open dashboard`, and `Remote status chart`.
- Long tracking URL fragments such as `abcdefghijklmnopqrstuvwxyz0123456789` and `utm_source=email` do not appear in visible preview text.
- `50x15` shows the minimum-size guard and resizing back restores a clean preview.

### TC-50 — Mouse navigation parity

**Lane:** A
**Sizes:** `220x50`, `120x40`, `80x24`, `50x15`

**Steps:**
1. Launch Herald in demo mode with mouse capture enabled.
2. Click each top tab and confirm the active tab changes without typing into Compose fields.
3. Confirm the top tab row contains Timeline and Contacts in mail-only sessions, and adds Calendar only in calendar-enabled sessions.
3. In Timeline, click a visible single-message row to open preview, then wheel over the list and the preview.
4. In Timeline, click a collapsed thread root whose top email is not selected; confirm the preview opens for the top email and the thread stays collapsed. Click the same selected root again; confirm the thread expands.
5. In Timeline, click an expanded thread root whose top email is not selected; confirm the preview opens for the top email and the thread stays expanded. Click the same selected root again; confirm the thread folds.
6. In Timeline sender/domain grouping, click a grouped row and wheel over the list and preview regions.
7. Open Calendar, click a mini-month day, click an agenda/day/week/3-day event, then double-click the same selected event to open detail.
8. Click a Calendar rail checkbox to hide a calendar, restart with the same config, and confirm the visible-calendar selection is restored from `calendar.selected_calendars`.
9. Click the sidebar when visible, then press `m` in Timeline and Calendar to release mouse capture and press `m` again to restore it.
10. Resize to `50x15`, capture the minimum-size guard, then recover to a larger size.

**Expect:**
- Mouse click and wheel behavior matches the equivalent keyboard actions and never changes hidden state outside the clicked region.
- Timeline thread-root mouse clicks use two-step semantics: select/update preview first, then fold/unfold only when the top thread email is already selected.
- Preview wheel events scroll the body without moving the underlying list cursor.
- List wheel events move the focused list cursor and refresh an open preview when applicable.
- Calendar mouse clicks match keyboard parity: mini-month clicks move the active date/range, event clicks select rows without leaking provider IDs, double-clicking the same selected event opens detail, and rail checkbox clicks show/hide calendars.
- Calendar rail visibility persists in YAML as selected calendar keys and does not expose provider URLs, OAuth tokens, sync tokens, ETags, or event IDs.
- The `m` toggle releases and restores TUI mouse capture while keeping visual/copy modes coherent.
- The minimum-size guard still appears at `50x15` and recovery restores normal mouse-capable layouts.

### TC-51 — Themed VHS demo media and keypress overlay

**Lane:** A
**Sizes:** `220x50`, `80x24`, `50x15`, plus VHS `1920x1080`

**Steps:**
1. Build Herald and launch `./bin/herald --demo`; confirm no `Keys:` overlay is visible.
2. Launch `./bin/herald --demo --demo-keys`, then press `S`, `?`, `/` in help, `2`, `G`, `c`, `V`, down-arrow range extension, real shifted down-arrow when the terminal can send it, right arrow, left arrow, and `z` across the Timeline and grouped cleanup flows.
3. Confirm Compose text, Timeline search text, and rule/prompt editor text do not appear in the key overlay.
4. Run the focused media set with `HERALD_DOC_MEDIA_ONLY=showcase-settings-dark-pastel,showcase-help-dark-pastel,showcase-cleanup-manager-red-alert,showcase-cleanup-rule-editor-red-alert,showcase-range-selection-pastel-dark,showcase-large-preview-pastel-dark demos/generate-doc-media.sh`.
5. Run `vhs demos/guided-tour-dark-pastel.tape` and `vhs demos/cleanup-rules-red-alert.tape`.
6. Run `scripts/regenerate-theme-screenshots.sh jade-signal amber-furnace solar-paper tokyo-dusk` after visual theme changes; use `HERALD_THEME_SCREENSHOT_VIEW=timeline` or `HERALD_THEME_SCREENSHOT_VIEW=preview` only when refreshing one gallery lane, then inspect the generated PNGs before publishing docs.

**Expect:**
- The overlay is opt-in and appears only when demo media explicitly requests `--demo-keys`.
- Key labels are compact and normalized, including `S`, `?`, `/`, `2`, `G`, `c`, `Shift+Down`, `Right`, `Left`, and `z`.
- Text-entry surfaces preserve literal text and do not leak draft/search/editor contents into the overlay.
- The selected screenshots render with `Dark Pastel`, `Red Alert`, and `Builtin Pastel Dark`; the two GIFs render at high resolution without replacing every existing docs asset.
- Theme gallery screenshots render through Herald's own `-theme` flag and show readable Timeline and Preview chrome for each refreshed palette.
- Existing docs media instructions use `1` Timeline, `2` Contacts, `G` Timeline grouping, Settings Sync & Cleanup launchers, and `c` to open Compose.

### TC-52 — Preview load telemetry and offline cache policy

**Lane:** A/F
**Sizes:** `120x40`, `80x24`, `50x15`

**Steps:**
1. Launch Herald in demo mode, dismiss onboarding, and open a Timeline preview.
2. Capture the split preview at `120x40` and `80x24`.
3. Press `G` to cycle sender/domain grouping and capture the grouped Timeline preview.
4. Open Settings, enter `Sync & Cleanup`, and inspect the `Offline Cache` selector. Confirm the selector uses compact policy labels without the longer helper paragraph and shows cleanup manager launchers.
5. Save each policy in turn: `Lightweight previews`, `Message bodies without attachments`, and `Full offline archive`.
6. In `Sync & Cleanup`, enable `Reclaim offline cache storage`, save, and confirm the reclaim prompt shows before/after byte estimates plus the preserved-data explanation.
7. Press `n`, repeat the action, then press `y` and confirm the status bar reports the reclaimed bytes and compaction result.
8. With debug logging enabled and without `-unsafe-logs`, wait for the active Timeline folder to finish loading and confirm the log records preview prewarming progress with private values masked, such as `Preview cache: 0/50 warming folder=?????????` followed by a completion summary. Use `-unsafe-logs` only when explicitly collecting local unredacted diagnostics.
9. Switch folders during prewarming and confirm stale prewarm progress does not continue warming the old folder.
10. Open a message with an attachment, then save the selected attachment.
11. Resize to `50x15`, then recover to `80x24`.

**Expect:**
- Timeline preview headers include a compact `Load:` row such as `Load: 42ms imap` or `Load: 2ms cache`.
- The `Load:` row never wraps or pushes body text outside the preview border at supported sizes.
- Setup wizard and Settings show the compact policy choices `Lightweight previews`, `Message bodies without attachments`, and `Full offline archive` without redundant explanatory copy inside the selector.
- Setup wizard keeps reclaim, poll interval, IMAP IDLE, and auto-cleanup out of onboarding; those advanced controls remain in Settings.
- Settings defaults to `Message bodies without attachments` for new configs and preserves the selected policy after saving.
- The reclaim action does not run silently: it estimates removable cached preview bytes, explains that preview text, headers, and attachment metadata stay cached, and waits for `y` before pruning and compacting.
- Reclaim under `lightweight` removes inline image and attachment bytes; reclaim under `no_attachments` removes only attachment bytes; reclaim under `preserve_all` reports no removable policy bytes.
- Lightweight cached previews render body text from cache without downloading attachment bytes.
- The preview prewarmer warms active-folder preview misses conservatively, one message at a time, skips already cached messages, and stops scheduling additional old-folder work after a folder switch.
- Attachment save fetches bytes on demand when preview only has metadata.
- `50x15` shows the minimum-size guard and resizing back restores a clean preview.

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
