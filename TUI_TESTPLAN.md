# TUI Test Plan — mail-processor

Manual QA checklist for verifying layout, navigation, and feature correctness across terminal sizes.
Run this after any change that touches rendering, layout math, key handling, or IMAP/cache logic.

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
