# TUI Test Plan — mail-processor

Manual QA checklist for verifying layout, navigation, and feature correctness across terminal sizes.
Run this after any change that touches rendering, layout math, key handling, or IMAP/cache logic.

**Test reports** must be saved in the `reports/` folder (gitignored). Name them descriptively, e.g. `reports/TEST_REPORT_2026-03-24.md`.

---

## Setup

### 1. Build a fresh test binary

```bash
go build -o /tmp/mail-processor-test .
```

### 2. Start a tmux session at the target size

```bash
# Replace WxH with the size you are testing (see sizes below)
tmux new-session -d -s mp -x 220 -y 50
tmux send-keys -t mp '/tmp/mail-processor-test -config proton.yaml' Enter
sleep 6   # wait for IMAP connect + initial load
```

### 3. Capture screenshots

```bash
tmux capture-pane -t mp -p -e > /tmp/cap.txt
cat /tmp/cap.txt
```

### 4. Send keystrokes

```bash
# Single key (no newline)
tmux send-keys -t mp 'j' ''

# Enter key
tmux send-keys -t mp '' ''

# Escape
tmux send-keys -t mp '' ''

# Sleep briefly after navigation to let the TUI re-render
sleep 0.3
```

### 5. Resize and re-test

```bash
tmux resize-window -t mp -x 80 -y 24
sleep 0.3
tmux capture-pane -t mp -p -e > /tmp/cap_80.txt
```

### 6. Teardown

```bash
tmux kill-session -t mp
```

---

## Terminal Sizes

Test every scenario at all three sizes unless noted otherwise.

| Size    | Why                                                            |
|---------|----------------------------------------------------------------|
| 220×50  | Wide — all columns, panels, and sidebars fully visible         |
| 80×24   | Standard SSH/default — most common real-world size             |
| 50×15   | Narrow/small — minimum-size guard should trigger               |

---

## What to Look for in Every Capture

- **Overflow** — columns or panels bleed past the terminal right edge (lines wrap)
- **Truncation** — useful text cut off too early; ellipsis (`...`) appears in the wrong place
- **Blank panels** — empty area where a message, spinner, or hint should appear
- **Missing key hints** — the bottom hint bar does not reflect the available keys for the active tab
- **Alignment** — table borders misaligned, columns shifting between rows
- **Crash / panic output** — any raw Go stack trace visible in the terminal

---

## Test Cases

### TC-01 — App startup and initial render

**Steps:**
1. Start app at 220×50.
2. Wait for the loading bar to finish.
3. Capture screenshot.

**Expect:**
- Tab bar visible: `1 Timeline  2 Compose  3 Cleanup`
- Summary table populated with senders and email counts
- Folder sidebar visible on the left
- Status bar at the bottom shows current folder name
- No overflow, no blank panels

---

### TC-02 — Tab switching

**Steps:**
1. Press `1` → capture
2. Press `2` → capture
3. Press `3` → capture

**Expect:**
- Each tab highlights correctly in the tab bar
- Timeline tab: chronological email list with Date, Sender, Subject columns
- Compose tab: To / Subject fields + body textarea visible
- Cleanup tab: summary table (senders) on the left, details table on the right
- Key hint bar updates for each tab

---

### TC-03 — Timeline navigation and body preview

**Steps:**
1. Switch to Timeline (`1`).
2. Press `j` five times to navigate down.
3. Press Enter to open the body preview.
4. Wait 2 seconds for body to load.
5. Capture screenshot.
6. Press Escape to close preview.
7. Capture screenshot.

**Expect (preview open):**
- Screen splits: timeline table on the left, preview panel on the right
- Preview shows email body text (or "Loading..." briefly)
- No column overflow; both panels fit within terminal width
- Sender / subject visible in preview header

**Expect (preview closed):**
- Layout returns to single-panel timeline
- Cursor remains on the previously selected email

---

### TC-04 — Iterate through first 20 emails in Timeline

This test stress-tests layout and IMAP body fetching across varied real-world email content (long subjects, Unicode, attachments, inline images, large HTML-only messages).

**Steps:**
1. Switch to Timeline (`1`).
2. For i = 1 to 20:
   a. Press Enter to open body preview.
   b. Wait 2 seconds.
   c. Capture screenshot → save as `/tmp/cap_email_<i>.txt`.
   d. Scroll down in the preview a few times (`j` × 3).
   e. Press Escape to close.
   f. Press `j` to advance to the next email.
   g. Wait 0.3 seconds.
3. Review all 20 captures.

```bash
for i in $(seq 1 20); do
  tmux send-keys -t mp '' ''   # Enter
  sleep 2
  tmux capture-pane -t mp -p -e > /tmp/cap_email_${i}.txt
  tmux send-keys -t mp 'jjj' ''
  sleep 0.3
  tmux send-keys -t mp '' ''   # Escape
  tmux send-keys -t mp 'j' ''
  sleep 0.3
done
```

**Expect for each capture:**
- Preview panel fills its allocated width without overflow
- Subject line in preview header is truncated cleanly (ends with `...`, no garbled characters)
- Non-ASCII subjects (Japanese, Arabic, accented Latin, emoji) render without broken bytes
- Body text wraps within the panel — no lines extend past the right edge
- "Loading..." appears briefly if the fetch is slow; panel is never permanently blank
- Tab bar and status bar remain intact throughout
- No crashes or panic output in any frame

---

### TC-05 — Sidebar toggle

**Steps:**
1. Switch to Timeline (`1`) at 80×24.
2. Press `f` to hide the sidebar → capture.
3. Press `f` again to show the sidebar → capture.

**Expect:**
- With sidebar hidden: timeline table expands to fill the full width
- With sidebar shown: sidebar reappears; table shrinks back without overflow
- No abrupt layout artifacts or misaligned borders on either toggle

---

### TC-06 — Folder navigation via sidebar

**Steps:**
1. Ensure sidebar is visible (`f` to toggle if needed).
2. Press `Tab` until the sidebar is focused (highlighted border).
3. Press `j` to move down to a different folder.
4. Press Enter to switch folders.
5. Wait 3 seconds for load.
6. Capture screenshot.

**Expect:**
- Status bar updates with the new folder name
- Email list repopulates for the new folder
- If the folder is empty, a "No emails in this folder" message is shown (not blank rows)

---

### TC-07 — Cleanup tab — sender grouping and details

**Steps:**
1. Switch to Cleanup (`3`).
2. Navigate to the first sender with `j`.
3. Press Enter to load the details panel.
4. Press `Tab` to move focus to the details panel.
5. Press `j` several times to scroll through individual emails.
6. Press Space on two different emails to select them.
7. Capture screenshot.

**Expect:**
- Details panel shows individual emails for the selected sender
- Selected emails have a checkmark (`✓`) in the first column
- Status bar shows `2 messages selected` (or `N messages from M senders` if applicable)
- Subject lines truncate cleanly — no garbled characters for non-ASCII subjects

---

### TC-08 — Domain mode toggle (Cleanup tab)

**Steps:**
1. Switch to Cleanup (`3`).
2. Press `d` to toggle domain grouping.
3. Capture screenshot.
4. Press `d` again to return to sender grouping.

**Expect:**
- Tab bar or status bar indicates the current mode
- Summary table re-groups by domain (e.g., `example.com` instead of `user@example.com`)
- Toggling back restores the original per-sender view

---

### TC-09 — AI chat panel

**Steps:**
1. Press `c` to open the chat panel.
2. Capture screenshot.
3. Type a question: `how many emails do I have?`
4. Press Enter.
5. Wait 5 seconds for Ollama response.
6. Capture screenshot.
7. Press `c` to close the chat panel.

**Expect (panel open):**
- Chat panel appears on the right side; main content narrows proportionally
- No overflow; total width still fits within terminal
- Input field at the bottom of the chat panel is active

**Expect (after response):**
- "You: how many emails do I have?" visible in chat history
- AI response visible below it
- Long messages wrap within the chat panel width

**Expect (closed):**
- Chat panel disappears; main content re-expands to full width

---

### TC-10 — Log viewer overlay

**Steps:**
1. Press `l` to open the log viewer.
2. Capture screenshot.
3. Press `l` again to close it.

**Expect:**
- Log overlay renders over the main content
- Log entries are colour-coded by level (green INFO, orange WARN, red ERROR)
- Timestamps are present on each line
- Closing with `l` restores the normal view cleanly

---

### TC-11 — Compose tab basics

**Steps:**
1. Switch to Compose (`2`).
2. Type a recipient address in the To field.
3. Press Tab to move to Subject, type a subject.
4. Press Tab to move to the body, type a few lines.
5. Press Ctrl+P to toggle Markdown preview.
6. Capture screenshot.
7. Press Ctrl+P again to return to edit mode.

**Expect:**
- Tab focus cycles correctly through To → Subject → Body
- Markdown preview renders the body content with formatting
- Toggling preview on/off does not resize or break adjacent panels

---

### TC-12 — Minimum size guard (50×15)

**Steps:**
1. Resize session to 50×15: `tmux resize-window -t mp -x 50 -y 15`
2. Capture screenshot.

**Expect:**
- App displays a plain-text message: `Terminal too narrow (N cols). Please resize to at least 60 columns.` or similar
- No broken table borders, overlapping panels, or garbled output

---

### TC-13 — Resize from wide to narrow and back

**Steps:**
1. Start at 220×50, capture.
2. Resize to 80×24 with Timeline open and body preview active, capture.
3. Resize back to 220×50, capture.

**Expect:**
- Each resize re-flows the layout cleanly
- Column widths recalculate; no overflow at 80×24
- Preview panel closes or shrinks gracefully if insufficient space
- Returning to 220×50 restores all columns and panels

---

### TC-14 — Refresh (`r`)

**Steps:**
1. From any tab, press `r`.
2. Wait for the progress bar to complete.
3. Capture screenshot.

**Expect:**
- Progress bar or status message shows "Scanning..." → "Processing..." → completed
- After refresh, email counts are consistent with the previous view (no data lost)
- No crash or blank screen

---

### TC-15 — Reply pre-fill (Timeline tab)

**Steps:**
1. Switch to Timeline (`1`).
2. Navigate to an email and open its preview (Enter).
3. Press `R` to open Compose pre-filled with reply data.
4. Capture screenshot.

**Expect:**
- Compose tab becomes active
- To field is populated with the original sender's address
- Subject field is prefilled with `Re: <original subject>`
- Body contains a quoted snippet or is empty (acceptable either way)

---

### TC-16 — Compose: Markdown email and send

**Prerequisites:** Live SMTP configured in `proton.yaml`.

**Steps:**
1. Switch to Compose (`2`).
2. Tab to the To field, type a recipient address.
3. Tab to Subject, type a subject line.
4. Tab to Body, type text with Markdown: `**bold word** and a [link](https://example.com)`.
5. Press `Ctrl+P` to open the Markdown preview.
6. Capture screenshot.
7. Press `Ctrl+P` again to return to edit mode.
8. Press `Ctrl+S` to send.
9. Capture screenshot after send attempt.

**Expect (preview):**
- Rendered output shows `**bold word**` as bold and `[link](https://example.com)` as a clickable-style link
- Preview occupies the body area without overflowing

**Expect (send):**
- Status bar shows a send confirmation message (e.g. `Email sent!`) or an error if SMTP is misconfigured
- Compose fields are cleared (or retained — either is acceptable; note actual behaviour)

---

### TC-17 — AI chat panel interactions

**Prerequisites:** Ollama running at configured host with the configured model loaded.

**Steps:**
1. Open Timeline (`1`), wait for load.
2. Press `c` to open the chat panel.
3. Capture screenshot — panel open, empty history.
4. Type a query: `summarise my recent emails` and press Enter.
5. Wait up to 10 seconds for a response.
6. Capture screenshot — response visible.
7. Press `c` to close the chat panel.
8. Capture screenshot — panel closed.

**Expect (panel open):**
- Chat panel renders on the right; main content narrows proportionally
- Input field is focused; typing is echoed correctly

**Expect (after response):**
- "You: summarise my recent emails" visible in chat history
- AI response rendered below it; long lines wrap within the panel
- No layout corruption in the main panel

**Expect (closed):**
- Chat panel disappears; main content re-expands to original width
- Focus returns to timeline table (cursor still on the same row)

---

### TC-18 — AI classification (`a` key)

**Prerequisites:** Ollama running with the configured model.

**Steps:**
1. Switch to Cleanup tab (`3`).
2. Press `a` to trigger AI classification on the current folder.
3. Watch the status bar for progress messages.
4. Wait for classification to complete (status bar returns to normal).
5. Press `l` to open the log viewer and inspect entries.
6. Press `l` to close.

**Expect:**
- Status bar cycles through progress messages (e.g. `Classifying 1/N…`)
- No crash or freeze; progress completes
- Log viewer shows `INFO` entries indicating categories assigned
- Classified emails in the Cleanup table show a category tag (if the table includes that column)

---

### TC-19 — Panel focus cycling with Tab / Shift+Tab (Timeline)

**Steps:**
1. Switch to Timeline (`1`), wait for load.
2. Press Enter on any email to open the body preview.
3. Press `Tab` once — capture screenshot.
4. Press `Tab` again — capture screenshot.
5. Press `Shift+Tab` — capture screenshot (should go back one step).
6. While preview panel is focused (step 3), press `j` twice — capture.
7. Press `Tab` to return focus to timeline table — press `j` twice — capture.

**Expect (step 3 — preview focused):**
- Preview panel border and header become noticeably brighter / different colour
- Status bar hints update to show `j/k: scroll`

**Expect (step 4 — timeline focused again):**
- Preview border returns to dim colour
- Status bar hints return to `j/k: navigate`

**Expect (step 5 — Shift+Tab):**
- Focus moves back to preview panel (reverse cycle)

**Expect (step 6 — j while preview focused):**
- Preview body scrolls down; timeline cursor does NOT move

**Expect (step 7 — j while timeline focused):**
- Timeline cursor moves down; preview body does NOT scroll

---

### TC-20 — Preview stays open during timeline navigation

**Steps:**
1. Switch to Timeline (`1`), wait for load.
2. Navigate to email N (use `j`/`k`) and press Enter — note the sender/subject in the preview.
3. Capture screenshot — preview shows email N.
4. Press `j` to move cursor to email N+1 — capture screenshot.
5. Press `j` again to email N+2 — capture screenshot.
6. Press Enter on email N+2 — wait for body load — capture screenshot.
7. Press Escape — capture screenshot.

**Expect (step 4 and 5):**
- Timeline cursor moves to N+1 / N+2
- Preview panel **remains visible** and still shows email N's content (not closed, not replaced)

**Expect (step 6):**
- Preview reloads and shows email N+2's From/Subject/body
- Loading indicator appears briefly if body fetch is slow

**Expect (step 7):**
- Preview panel closes; layout returns to full-width timeline
- Cursor remains on email N+2

---

### TC-21 — Thread expand/collapse

**Steps:**
1. Switch to Timeline (`1`), wait for load.
2. Use `j`/`k` to find a row whose subject begins with `[N]` (collapsed thread, N ≥ 2).
3. Capture screenshot — note the subject and N value.
4. Press Enter to expand the thread.
5. Capture screenshot.
6. Press `j` once or twice to move to a child email row (prefixed `↳`).
7. Capture screenshot.
8. Navigate back to the `[N]` header row and press Enter again to collapse.
9. Capture screenshot.

**Expect (step 3 — collapsed):**
- Single row shows `[N] Subject`, sender from the newest email, date of newest email

**Expect (step 5 — expanded):**
- The `[N]` header row is gone; N individual rows appear in its place
- First row shows the full sender and subject without `[N]` prefix
- Subsequent rows show `  ↳ sender` in the sender column
- No overflow; table width unchanged

**Expect (step 7 — navigating child rows):**
- Each `↳` row has its own date, size, and attachment indicator
- Pressing Enter on a `↳` row opens the body preview for that specific email

**Expect (step 9 — collapsed again):**
- All individual rows are replaced by the single `[N]` header row
- Timeline length shrinks by N-1 rows

---

### TC-22 — Deletion confirmation prompt

**Steps:**
1. Switch to Timeline (`1`), navigate to an email.
2. Press `D`.
3. Capture screenshot.
4. Press `n`.
5. Capture screenshot (email still present).
6. Press `D` again, then press `y`.
7. Capture screenshot.

**Expect (step 3):**
- Status bar turns red/highlighted with the email subject and `[y] confirm  [n/Esc] cancel`
- Key hint bar shows `[y] confirm  │  [n/Esc] cancel`
- No deletion has started yet

**Expect (step 5):**
- Status bar returns to normal
- Email is still present in the timeline

**Expect (step 7):**
- Email is removed from the timeline after deletion completes
- Status bar returns to normal after reload

---

### TC-23 — Delete individual email from Timeline

**Steps:**
1. Switch to Timeline (`1`), navigate to any email row.
2. Press `D` → `y` to confirm.
3. Wait for reload.
4. Capture screenshot.

**Expect:**
- The specific email row disappears
- Timeline reloads without the deleted email
- Cursor stays near its previous position

---

### TC-24 — Archive email (`e` key)

**Steps:**
1. Switch to Timeline (`1`), navigate to an email.
2. Press `e`.
3. Status bar shows archive confirmation prompt.
4. Press `y`.
5. Wait for reload, capture screenshot.

**Expect:**
- Confirmation prompt mentions "Archive" not "Delete"
- After `y`: email removed from INBOX timeline
- No crash; server-side: email should appear in Archive folder
- Sidebar folder counts update after completion (Archive count increases)

Also test in Cleanup tab (panelDetails focused on a message):
6. Switch to Cleanup (`3`), focus on a message row, press `e` → `y`.

**Expect:**
- Same confirmation + archive behavior
- Sidebar counts update after completion

---

### TC-25 — Forward email (`F` key)

**Steps:**
1. Switch to Timeline (`1`), navigate to an email.
2. Press `F`.
3. Capture screenshot.

**Expect:**
- Compose tab opens automatically
- Subject prefixed with `Fwd: ` (original subject preserved)
- Body contains `--- Forwarded message ---` header with From/Date/Subject
- If the email body was previously loaded in preview, it appears in the compose body
- Cursor focus is on the To: field (empty, ready to type)

---

### TC-26 — In-folder search (`/` key)

**Steps:**
1. Switch to Timeline (`1`).
2. Press `/`.
3. Type a sender domain (e.g. `github`).
4. Capture screenshot.
5. Press Escape.
6. Capture screenshot.
7. Press `/`, type a term that does not exist in this folder (e.g. `zzznomatch`).
8. Capture screenshot.

**Expect (step 4):**
- Key hint bar shows search input with query text
- Timeline shows only matching emails (filtered in real time)
- Status bar shows result count

**Expect (step 6):**
- Timeline restores all emails
- Search input cleared

**Expect (step 8 — zero results):**
- Status bar shows `Search: 0 results`
- Key hint bar shows: `No results in this folder — try: /* zzznomatch`

---

### TC-27 — Full-text body search (`/b` prefix)

**Prerequisites:** Open a few emails in the preview to cache their body text.

**Steps:**
1. Press `/`, type `/b invoice`.
2. Capture screenshot.

**Expect:**
- Results include emails where `invoice` appears in the body (not just subject/sender)
- Source tag in status bar shows `fts`

---

### TC-28 — Cross-folder search (`/*` prefix)

**Steps:**
1. Press `/`, type `/* github`.
2. Capture screenshot.

**Expect:**
- Results include emails from all folders, not just the current one
- Emails from different folders appear mixed in the results

---

### TC-29 — Saved search (`Ctrl+S` in search mode)

**Steps:**
1. Press `/`, type `newsletter`.
2. Press `Ctrl+S`.
3. Press Escape.
4. Capture screenshot.

**Expect:**
- No error/crash
- Search saved silently (can be verified in database: `sqlite3 email_cache.db "SELECT * FROM saved_searches"`)

---

### TC-30 — Semantic search (`?` prefix)

**Prerequisites:** Ollama running with `nomic-embed-text` model available.

**Steps:**
1. Press `/`, type `?meeting tomorrow`.
2. Wait for results.
3. Capture screenshot.

**Expect:**
- Results ranked by semantic similarity (not keyword match)
- Status bar shows `semantic` source tag
- If Ollama not available: graceful error in status, no crash

---

### TC-31 — Background polling / new email notification

**Prerequisites:** Another email client or script that can send a new email to INBOX.

**Steps:**
1. Start the app, wait for load to complete.
2. Check status bar for `↻ 60s` countdown (or similar).
3. Send a new email to the account from an external client.
4. Wait for the polling interval (up to 60 seconds).
5. Capture screenshot.

**Expect:**
- Status bar shows `↻ Ns` countdown ticking down each second
- After poll fires: new email appears at the top of the Timeline without manual refresh
- No crash or freeze during poll

---

### TC-32 — Vendor preset config (`vendor: gmail`)

**Steps:**
1. Create a test config file with only:
   ```yaml
   vendor: gmail
   credentials:
     username: test@gmail.com
     password: testpass
   ```
2. Run: `./bin/mail-processor -config test-vendor.yaml`

**Expect:**
- App connects to `imap.gmail.com:993` (visible in log)
- No "missing server.host" validation error
- Connection fails (wrong credentials) but with an IMAP auth error, not a config error

---

### TC-33 — Deletion confirmation in Cleanup tab

**Steps:**
1. Switch to Cleanup (`3`), navigate to a sender row.
2. Press `D`.
3. Capture screenshot (confirmation prompt).
4. Press `Esc`.
5. Confirm no deletion occurred (sender still present).

**Expect:**
- Red status bar with sender name in the description
- Press `Esc` cancels cleanly

---

### TC-34 — Thread collapse by pressing Enter on open thread root

**Steps:**
1. Switch to Timeline (`1`), wait for load.
2. Use `j`/`k` to find a row whose subject begins with `[N]` (collapsed thread, N ≥ 2).
3. Press Enter to expand the thread.
4. Capture screenshot — thread is expanded (N individual rows visible).
5. Navigate back to the thread root row (the first row of the expanded thread, no `↳` prefix).
6. Press Enter to collapse the thread.
7. Capture screenshot.

**Expect (step 4 — expanded):**
- N individual email rows appear where the `[N]` header was
- First row has no `↳` prefix; subsequent rows are prefixed with `↳`

**Expect (step 7 — collapsed):**
- All N individual rows are replaced by a single `[N] Subject` header row
- Timeline length shrinks by N−1 rows
- Cursor sits on the collapsed `[N]` header row
- No layout corruption or blank rows left behind

---

### TC-35 — Full-screen email view (`z` key)

**Steps:**
1. Switch to Timeline (`1`), open body preview on any email (Enter).
2. Wait for body to load.
3. Press `z` to enter full-screen mode.
4. Capture screenshot.
5. Press `j` several times to scroll.
6. Capture screenshot.
7. Press `z` again to exit.
8. Capture screenshot.
9. Repeat: re-enter full-screen with `z`, then press Escape to exit.
10. Capture screenshot after Escape.

**Expect (step 4 — full-screen):**
- Tab bar, sidebar, and timeline table are all hidden
- Email body fills the entire terminal width and height
- From / Date / Subject header visible at the top
- Scroll indicator at the bottom: `z/esc: exit full-screen`

**Expect (step 6 — scrolled):**
- Body scrolls; scroll indicator updates (`line N/M  XX%`)

**Expect (steps 7 and 10 — exit):**
- Layout restores to the split timeline + preview view
- No blank panels or rendering artifacts

---

### TC-36 — Mouse mode (`m` key)

**Steps:**
1. Open an email body preview (Enter on Timeline).
2. Tab to focus the preview panel.
3. Press `m` to enter mouse mode.
4. Capture screenshot.
5. Press `m` again to restore TUI mouse handling.
6. Capture screenshot.

**Expect (step 4 — mouse mode):**
- Status bar prepends `[mouse] select mode — m: restore TUI`
- Terminal mouse events are released to the OS (native text selection active)

**Expect (step 6 — restored):**
- Status bar returns to normal (no mouse-mode prefix)
- TUI mouse handling is re-enabled

---

### TC-37 — Visual selection (`v`, `y`, `Y`, `yy`)

**Prerequisites:** Email with multi-line body open in preview.

**Steps:**
1. Open an email body preview; wait for body to load.
2. Tab to focus the preview panel.
3. Press `v` to enter visual mode.
4. Capture screenshot — first line should be highlighted (purple).
5. Press `j` three times to extend the selection.
6. Capture screenshot — four lines highlighted.
7. Press `y` to copy the selection to clipboard.
8. Open a terminal editor or text area and paste to verify.
9. Open the preview again; focus preview panel; press `yy`.
10. Paste and verify the current line was copied.
11. Press `Y` to copy the entire body.
12. Paste and verify all body lines are present.
13. Press `v` to enter visual mode, press `j` once, then press Esc.
14. Capture screenshot — visual mode exits; no highlight.

**Expect (step 4):**
- Key hint bar changes to: `j/k: extend selection  │  y: copy selection  │  Y: copy all  │  esc: cancel visual`
- First visible line highlighted with purple background

**Expect (step 6):**
- Lines 1–4 highlighted; remaining lines normal

**Expect (steps 8, 10, 12):**
- Clipboard content matches the selected text exactly

**Expect (step 14):**
- Visual mode exits cleanly; body renders normally (no highlight)
- Key hint bar returns to normal preview hints

---

### TC-38 — Compose: Markdown → HTML send

**Prerequisites:** Live SMTP configured in `proton.yaml`.

**Steps:**
1. Switch to Compose (`2`).
2. Fill To, Subject, and Body (include `**bold**` and `_italic_` in body).
3. Press `Ctrl+P` to preview — verify Markdown renders.
4. Press `Ctrl+P` again to return to edit mode.
5. Press `Ctrl+S` to send.
6. Capture screenshot after send.
7. In a separate email client, open the sent message and view in HTML mode.

**Expect (step 6):**
- Status bar shows `Email sent!` or similar confirmation

**Expect (step 7):**
- Received email renders `bold` in bold and `italic` in italic in the HTML view
- A plain-text fallback is also present (multipart/alternative)

---

### TC-39 — Receive attachments: display in preview

**Prerequisites:** An email with at least one attachment (PDF, image, or similar) present in the mailbox.

**Steps:**
1. Switch to Timeline (`1`), locate an email with attachment indicator.
2. Press Enter to open preview; wait for body to load.
3. Capture screenshot.

**Expect:**
- Preview panel shows `[attach] filename.pdf  application/pdf  X KB` (or similar) below the body
- Key hint bar shows `s: save attachment` when preview is focused
- No crash or blank panel

---

### TC-40 — Save attachment (`s` key)

**Prerequisites:** TC-39 prerequisite met; email with attachment open in preview.

**Steps:**
1. Open the email with attachment (TC-39 steps 1–2).
2. Tab to focus the preview panel.
3. Press `s`.
4. Capture screenshot — save-path input should appear.
5. Type a destination path (e.g. `/tmp/test-att.pdf`).
6. Press Enter.
7. Capture screenshot.
8. Verify the file exists: `ls -la /tmp/test-att.pdf`

**Expect (step 4):**
- `Save to:` input appears pre-filled with `~/Downloads/<filename>`
- Esc cancels the prompt without saving

**Expect (step 7):**
- Status bar or preview shows a confirmation message
- No crash or error

**Expect (step 8):**
- File exists at the specified path with non-zero size

---

### TC-41 — Send with attachment (`ctrl+a`)

**Prerequisites:** Live SMTP configured; a small test file at a known path (e.g. `/tmp/test.txt`).

**Steps:**
1. Switch to Compose (`2`).
2. Fill To and Subject.
3. Press `Ctrl+A` to open the attachment path input.
4. Capture screenshot.
5. Type `/tmp/test.txt`, press Enter.
6. Capture screenshot — file should appear in staged attachment list.
7. Press `Ctrl+A` again and type a path to a file >10 MB (if available) — verify size warning.
8. Press `Ctrl+S` to send.
9. In a separate email client, open the sent message.

**Expect (step 4):**
- Attachment path input appears below the body area

**Expect (step 6):**
- Staged attachment list shows `[attach] test.txt  (N KB)`
- Compose status shows no error

**Expect (step 7):**
- Compose status shows a size warning if file >10 MB

**Expect (step 9):**
- Received email has the attachment intact with correct filename and size

---

### TC-42 — Read/unread tracking

**Steps:**
1. Build and launch at 220×50.
2. Switch to Timeline (`1`).
3. Look for unread emails — they should show `●` as the first character of the Sender column.
4. Press Enter on an unread email to open the body preview.
5. Wait ~1 second for the body to load.
6. Capture screenshot.
7. Press Esc to close.
8. Capture screenshot.

**Expect (step 3):**
- Unread emails have `●` prefix in Sender column; read emails have ` ` (space) prefix
- No layout overflow from the extra character

**Expect (step 6 — preview open):**
- Body displays normally; no visual change in preview itself

**Expect (step 8 — after close):**
- The `●` has been replaced by ` ` (space) for the email just opened, confirming it was marked read
- No crash or error output

---

### TC-43 — Unsubscribe (`u` key)

**Prerequisites:** An email with a `List-Unsubscribe` header loaded in the preview panel.

**Steps:**
1. Switch to Timeline (`1`), navigate to a newsletter or marketing email.
2. Press Enter to open the body preview; wait for it to load.
3. Press Tab to focus the preview panel.
4. Check key hints — should show `u: unsubscribe` if `List-Unsubscribe` header is present.
5. Press `u`.
6. Capture screenshot — should show orange confirmation bar.
7. Press `n` to cancel; capture screenshot.
8. Press `u` again, then press `y` to confirm.
9. Capture screenshot — status bar should show unsubscribe result.

**Expect (step 4):**
- `u: unsubscribe` appears in key hints when body has `List-Unsubscribe` header
- Hint absent for emails without the header

**Expect (step 6 — confirmation):**
- Status bar turns orange with `Unsubscribe from <sender>?  [y] confirm  [n/Esc] cancel`
- No layout corruption

**Expect (step 7 — cancelled):**
- Status bar returns to normal; no action taken

**Expect (step 9 — confirmed):**
- Status bar shows one of:
  - `Unsubscribed (one-click POST sent)` — if RFC 8058 one-click
  - `Unsubscribe URL copied to clipboard` — if HTTPS URL copied
  - `Unsubscribe address copied to clipboard` — if mailto only
- No crash

---

### TC-44 — First-run wizard (no config file)

**Prerequisites:** `~/.herald/conf.yaml` does not exist. Herald binary is built (`make build`).

**Steps:**
1. Rename or remove `~/.herald/conf.yaml` if present.
2. Run `./bin/herald`.
3. Capture screenshot — wizard step 1 (account type selector) should appear immediately.
4. Navigate to **Standard IMAP** with `↓`, press Enter.
5. Fill in email address and password fields; leave IMAP/SMTP fields at defaults.
6. Press Enter to advance to step 3 (AI).
7. Accept default Ollama settings; press Enter to save & launch.
8. Capture screenshot — app should load with the inbox.

**Expect (step 3):**
- Full-screen TUI wizard visible (no config error, no crash)
- Step indicator shows `Step 1 of 3 — Account`
- Provider list includes Gmail, Standard IMAP, ProtonMail Bridge, Fastmail, iCloud, Outlook

**Expect (step 8):**
- `~/.herald/conf.yaml` created with correct IMAP credentials
- App loads inbox normally

---

### TC-45 — S key settings panel

**Prerequisites:** App running with a valid config.

**Steps:**
1. From any tab, press `S`.
2. Capture screenshot — settings overlay should appear full-screen.
3. Navigate to the AI step (step 3) using the form navigation.
4. Change the Ollama model field.
5. Press Enter to save.
6. Capture screenshot — settings should close; inbox visible again.
7. Press `S` again; press Escape.
8. Capture screenshot — settings should close without saving.

**Expect (step 2):**
- Settings overlay fills the terminal
- Form pre-filled with current config values
- Provider matches what is in the active config

**Expect (step 6):**
- Status bar briefly shows "Settings saved."
- Inbox (or previous view) is visible again

**Expect (step 8):**
- Settings close; no "Settings saved." message

---

### TC-46 — Gmail OAuth flow from settings panel

**Prerequisites:** `HERALD_GOOGLE_CLIENT_ID` and `HERALD_GOOGLE_CLIENT_SECRET` set to real Google OAuth2 credentials. App running.

**Steps:**
1. Press `S` to open settings.
2. Select **Gmail** as provider; enter a Gmail address; advance to step 2.
3. Press Enter on the OAuth confirmation.
4. Capture screenshot — OAuth wait screen should appear with the authorization URL.
5. Open the URL in a browser and authorize.
6. Capture screenshot — OAuth wait screen should show success and transition back to inbox.
7. Check `~/.herald/conf.yaml` for `gmail.access_token`, `gmail.refresh_token`, `gmail.token_expiry`.

**Expect (step 4):**
- Authorization URL displayed; spinner visible
- `Open Browser` option visible; URL is a valid `accounts.google.com/o/oauth2/auth` URL

**Expect (step 6):**
- App returns to inbox
- Status bar shows "Gmail account authorized. Reconnecting…"

**Expect (step 7):**
- `gmail.refresh_token` is non-empty in the saved config

---

### TC-47 — Rule Editor (W key)

**Preconditions:** App running in Cleanup tab with at least one sender visible

**Steps:**
1. Press `W` while a sender row is highlighted
2. The rule editor form opens (huh form overlay)
3. Fill in: Trigger = "sender", Action = "archive", any details
4. Tab through the remaining fields to reach the Submit button, then press Enter to submit
5. Press `W` again — confirm the new rule appears in the editor pre-filled

**Expect:**
- Rule is saved; re-opening the editor (W key) shows a pre-filled form with the saved trigger and actions; a status message confirms 'Rule saved'; `go test ./...` passes

---

### TC-48 — Demo Mode (--demo flag)

**Preconditions:** Binary built with `make build`

**Steps:**
1. Run `./bin/herald --demo`
2. Observe status bar shows `[DEMO]` badge
3. Navigate Timeline, Cleanup, Compose tabs — all show synthetic data
4. Press `D` to delete a sender — the sender row disappears from the table and a status message confirms the deletion
5. Press `a` to classify — classification runs on demo emails

**Expected:**
- App functions fully with synthetic data; no IMAP connection needed; `[DEMO]` visible in status bar

---

### TC-49 — Soft Unsubscribe (u key)

**Preconditions:** App running in Cleanup tab with at least one sender visible

**Steps:**
1. Press `u` on a sender row
2. A 3-choice prompt appears (unsubscribe / move / cancel)
3. Choices are navigable with arrow keys; pressing Escape cancels without taking action
4. Choose "move to Disabled Subscriptions"
5. Confirm the sender's emails moved

**Expected:**
- Emails from that sender moved to "Disabled Subscriptions" folder; rule created; no crash; the sender's emails no longer appear in the Cleanup tab; the move rule is visible via the `W` key

---

### TC-50 — Cleanup Preview (Enter key)

**Preconditions:** App running in Cleanup tab with at least one sender that has emails

**Steps:**
1. Press `Enter` on a sender row (not `D` for delete)
2. A three-column layout appears (sender list | email list | body preview); verify at 220×50 and 80×24 terminal sizes
3. Navigate between emails in the middle column — body updates in right panel
4. Press `Escape` to close the preview and return to normal 2-column layout

**Expected:**
- Preview opens cleanly; body loads asynchronously (loading indicator shown); layout returns to normal on Escape; no layout overflow; at 80×24 the layout degrades gracefully (no content overflow beyond terminal edge)

---

### TC-51 — Daemon serve subcommand

**Preconditions:** `herald` binary built (`make build`); no daemon already running on the configured port

**Steps:**
1. Run `./bin/herald serve --config proton.yaml` in a terminal
2. Verify the process starts and logs indicate it is listening (e.g. `listening on 127.0.0.1:<port>`)
3. Run `./bin/herald status --config proton.yaml` in a second terminal
4. Run `./bin/herald stop --config proton.yaml`
5. Confirm the serve process exits cleanly

**Expected:**
- `serve` starts without error and writes a pidfile; `status` reports the daemon as running with its PID; `stop` sends a shutdown signal and the serve process exits with code 0; pidfile is removed after stop

---

### TC-52 — Daemon sync subcommand

**Preconditions:** Daemon running via `herald serve --config proton.yaml`

**Steps:**
1. Run `./bin/herald sync --config proton.yaml` (optionally with `--folder INBOX`)
2. Observe output or daemon log for sync activity
3. Run `./bin/herald sync --config proton.yaml --folder INBOX` explicitly
4. Stop the daemon with `./bin/herald stop --config proton.yaml`

**Expected:**
- `sync` triggers an IMAP fetch on the daemon; the daemon log shows new emails processed (or "no new mail" if inbox is empty); command exits 0; running sync against a stopped daemon produces a clear error message (not a panic)

---

### TC-53 — TUI auto-detects running daemon (RemoteBackend)

**Preconditions:** Daemon running via `herald serve --config proton.yaml`

**Steps:**
1. Launch the TUI: `./bin/herald --config proton.yaml` (no `serve` subcommand)
2. Verify the status bar shows a remote/daemon indicator (not "local")
3. Navigate through Timeline and Cleanup tabs — data should load via the daemon
4. Stop the daemon with `./bin/herald stop --config proton.yaml` in another terminal
5. Observe TUI behaviour after daemon disappears (reconnect attempt or error message)

**Expected:**
- TUI detects the running daemon and connects as a RemoteBackend client; the status bar reflects this; email data loads normally; when the daemon stops, the TUI shows a reconnect or error state rather than crashing

---

### TC-54 — Compose CC/BCC fields render at full width

**Goal:** Verify CC and BCC text inputs span the full compose column width (regression for the "tiny box" bug where they only showed a single character).

**Setup:** `./bin/herald --demo`, navigate to Compose tab (`2` key).

**Steps:**
1. Open Compose tab
2. Observe the To, CC, and BCC input fields

**Expected:**
- All three input fields (To, CC, BCC) span the same full width as each other
- No field shows as a tiny one-character box
- Typing into CC and BCC fields shows text as expected

**Automated coverage:** `TestComposeCCBCCWidth_MatchesToField` in `internal/app/compose_layout_test.go`

---

### TC-55 — Sender name and email address shown in distinct colors

**Goal:** Verify that in the Cleanup sender list and Timeline sender column, the display name and `<email>` address are rendered in visually distinct colors (near-white name, dim gray email).

**Setup:** `./bin/herald --demo`, use a 220×50 terminal.

**Steps:**
1. Open Cleanup tab (`3` key) — observe sender column
2. Open Timeline tab (`1` key) — observe sender column in email list
3. Look for senders in `"Display Name <email@domain>"` format

**Expected:**
- Display name appears brighter/near-white
- Email address appears dimmer/gray
- Both are readable and don't overflow their column width
- Plain email-only addresses (no name) render gracefully without errors

**Automated coverage:** `TestStyledSender_*` tests in `internal/app/wordDiff_test.go`

---

### TC-56 — AI Writing Assistant panel (Compose tab)

**Goal:** Verify the Compose AI assistant panel opens, quick actions work, subject suggestion works, and accept/discard function correctly.

**Setup:** `./bin/herald --config proton.yaml` with Ollama running, Compose tab.

**Steps:**
1. Open Compose tab (`2` key), type some body text
2. Press `Ctrl+G` — AI panel should appear on the right, compose body narrows
3. Press a quick action (e.g. arrow to "Improve", Enter) — panel shows loading, then suggestion
4. Observe word-level diff: deleted words in red strikethrough, additions in green
5. Edit the suggestion text directly in the panel
6. Press `Ctrl+Enter` — suggestion replaces compose body; panel closes
7. Open panel again (`Ctrl+G`), press `Esc` — panel closes, body unchanged
8. Press `Ctrl+J` — subject hint appears below Subject field
9. Press `Tab` — hint accepted into Subject field; or `Esc` to dismiss

**Expected:**
- `Ctrl+G` toggles panel; compose body narrows to ~60% width
- Loading spinner shows while AI is working
- Word-level diff highlights only changed words
- `Ctrl+Enter` copies edited suggestion to body
- `Ctrl+J` + `Tab` accepts subject hint
- No AI configured → status bar shows "No AI backend configured", no panic

**Automated coverage:** `TestAiAssistCmd_NilClassifier`, `TestAiSubjectCmd_NilClassifier`, `TestComposeAIFields_Initialized`

---

### TC-57 — Reply pre-fills compose with thread context for AI

**Goal:** Verify that pressing `R` on a Timeline email pre-fills Compose and captures reply context so the AI assistant uses thread context.

**Setup:** `./bin/herald --demo`, Timeline tab with emails loaded.

**Steps:**
1. Navigate to an email in Timeline (`j`/`k`)
2. Press `R` — Compose opens pre-filled with To, Subject (Re: ...), quoted body
3. Press `Ctrl+G` to open AI panel
4. Observe context toggle shows "Thread" (not "Draft only") when replying
5. Fire a quick action — AI should include thread context in its rewrite

**Expected:**
- Compose pre-filled correctly (To, Subject with Re:, quoted reply)
- AI panel shows Thread context enabled when replying
- Draft-only context toggle switches to "Draft only" mode
- After sending or navigating away, reply context is cleared for next compose

---

## Result Format

After completing all test cases, write up findings using this structure:

```
## Test Run — <date> — <terminal size(s)>

### Bugs

| ID  | Severity | TC  | Description                          | Steps to reproduce |
|-----|----------|-----|--------------------------------------|--------------------|
| B1  | Medium   | 04  | Subject garbled for Japanese text    | TC-04, email #7    |

### UX Issues

| ID  | TC  | Description                              | Suggestion                        |
|-----|-----|------------------------------------------|-----------------------------------|
| U1  | 06  | Empty folder shows no message, just rows | Show "No emails in this folder"   |

### All Good

List test cases that passed with no issues:
- TC-01 Startup: PASS
- TC-02 Tab switching: PASS
- ...
```

If a test case passes completely and nothing is noteworthy, list it under **All Good**.
Only open a bug or UX item when something is clearly wrong or clearly improvable.
