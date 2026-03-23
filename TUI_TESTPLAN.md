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
