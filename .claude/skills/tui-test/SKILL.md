---
name: tui-test
description: Battle-test the Herald TUI — automated exploratory testing that captures screens, detects visual defects programmatically, and digs into anything suspicious. Simulates prolonged real usage.
disable-model-invocation: true
allowed-tools: Bash Read Write Glob Grep Edit Agent TodoWrite
argument-hint: "[focus: full | stress | ai | contacts | timeline | cleanup | compose | navigation]"
---

# TUI Battle Testing

You are testing Herald, a terminal email client. Your job is to systematically USE the app via tmux, capture every screen, ANALYZE each capture for defects, and dig deeper into anything that looks wrong.

## The Core Loop

This is what you do, over and over, for every feature you test:

```
1. CAPTURE the screen (tmux capture-pane)
2. READ the capture carefully — every line
3. CHECK against the defect checklist below
4. If something is wrong → investigate (try variations, sizes, repetitions)
5. If something is fine → do more operations, then capture and check again
6. After 10+ operations in an area → capture and compare against your first capture
```

**You must actually read and analyze every capture you take.** Don't just capture and move on. Look at the output. Count the columns. Check if borders are closed. Verify the header is present.

## How to Detect Defects in a tmux Capture

When you capture a screen with `tmux capture-pane -t test -p > /tmp/cap.txt`, you get plain text. Here is exactly what to check:

### 1. Structural Integrity

```
EXPECTED (first 3 lines of any tab):
 ProtonMail Analyzer                          ← Line 1: app title
  1  Timeline    2  Compose    3  Cleanup...  ← Line 2: tab bar
                                              ← Line 3: blank or panel start
```

**Defect: Missing header.** If line 1 doesn't contain "Analyzer" or line 2 doesn't contain tab numbers, the header has been pushed off-screen. This happens when height is too small or panel height calculation is wrong. Check `windowHeight` math.

**Defect: Missing tab indicator.** The active tab should be visually distinct. If all tabs look the same in the ANSI capture (`-e` flag), the highlight is broken.

### 2. Border Completeness

Every panel must have matching corners:
- Top-left `┌` must pair with top-right `┐` on the same line
- Bottom-left `└` must pair with bottom-right `┘` on the same line
- Left `│` and right `│` must appear on every line between top and bottom

**Defect: Unclosed border.** If you see `┌───...` without a matching `┐`, the panel overflows the terminal width. Count the characters between `│` markers — they should equal the panel's intended width.

**Defect: Missing bottom border.** If `└` never appears, the panel extends beyond the visible terminal height.

### 3. Column Alignment

For any table (Timeline, Cleanup, Contact list), pick a column header and check that every row's data for that column starts at the same character position.

**How to check programmatically:**
```bash
# Capture and check if all "Date" values start at the same column
tmux capture-pane -t test -p > /tmp/cap.txt
# Find the column position of "Date" in the header
grep -n "Date" /tmp/cap.txt | head -1
# Then check that date-like patterns (26-0X-XX) appear at that same position in data rows
```

**Defect: Column drift.** If row 5's date starts at column 60 but row 12's date starts at column 58, something is wrong with the sender or subject cell width for that row. Common cause: ANSI codes counted as visible width, or inconsistent indicator characters.

### 4. Content Presence

- **Sender column**: Should never be entirely empty. If you see rows where the space between `│` and the subject is blank, the sender rendering is broken.
- **Status bar**: Bottom line(s) should contain folder name, counts, and key hints. If the status bar is missing or shows previous tab's hints, state didn't update.
- **Right panel**: When showing "Select a contact..." or email detail, should never be completely empty (no text at all between borders).

### 5. Layout at Different Sizes

At **80x24**:
- Sidebar should auto-hide (status bar says "sidebar hidden")
- App header may be compressed but tab bar must still be present
- Contact list: names will be heavily truncated but company brackets and counts must still be on each row

At **120x40**:
- Sidebar visible, panels proportional
- No column should be 0-width (invisible)

At **220x50**:
- Everything visible, full sender names with email addresses
- No excessive whitespace gaps between columns

### 6. Hint Placement

Key hints and scroll indicators must appear at the **bottom** of their panel, never floating mid-panel or mixed into body content. If you see "Esc: back" or "D: delete" with body text above AND below it, the hint is in the wrong place.

## Setup

```bash
# Build
go build -o bin/herald ./main.go

# Launch at starting size
tmux kill-session -t test 2>/dev/null; sleep 0.5
tmux new-session -d -s test -x 220 -y 50
tmux send-keys -t test "$(pwd)/bin/herald --demo" Enter
sleep 5

# For live IMAP (AI features): omit --demo
```

### Key Reference

```bash
tmux send-keys -t test 'j' ''       # Down
tmux send-keys -t test 'k' ''       # Up
tmux send-keys -t test Enter        # Open/confirm
tmux send-keys -t test Escape       # Back/close
tmux send-keys -t test Tab          # Next panel
tmux send-keys -t test '1'          # Timeline tab
tmux send-keys -t test '2'          # Compose tab
tmux send-keys -t test '3'          # Cleanup tab
tmux send-keys -t test '4'          # Contacts tab
tmux send-keys -t test 'f'          # Toggle sidebar
tmux send-keys -t test 'c'          # Toggle chat
tmux send-keys -t test 'z'          # Full-screen toggle
tmux send-keys -t test '*'          # Star
tmux send-keys -t test 'D'          # Delete
tmux send-keys -t test 'e'          # Archive/enrich
tmux send-keys -t test '/' ''       # Search
tmux send-keys -t test C-q          # Quick replies
tmux send-keys -t test C-p          # Compose preview
tmux resize-window -t test -x W -y H  # Resize (DON'T restart)
```

## Exploration Plan

Default to `full`. If $ARGUMENTS names a focus, spend 80% of time there, 20% on a quick pass of other areas.

### Step 1: Baseline at 220x50

Capture the initial screen. This is your reference. Verify:
- [ ] Header present ("Analyzer" on line 1, tab bar on line 2)
- [ ] Sidebar with folder counts
- [ ] Table with populated Sender and Subject columns
- [ ] Status bar at bottom with counts and key hints
- [ ] Border boxes complete (all corners present)

### Step 2: Exercise Every Tab (still 220x50)

For each tab (1, 2, 3, 4):
1. Switch to the tab, wait 1s, capture
2. Run the defect checklist above on the capture
3. Interact: open something (Enter), capture, check
4. Close it (Esc), capture, compare with the pre-interaction capture
5. Note any differences

### Step 3: Deep Dive — Repetition (pick the most complex tab first)

Stay in one tab. Do 15-20 operations:
- **Timeline**: Open email, close, move down, open next, close, repeat for 15+ emails. Every 5th one, capture and run the full defect checklist.
- **Contacts**: Open contact, tab to emails, open email, Esc back, Esc back, next contact, repeat for ALL contacts. Capture after each round.
- **Cleanup**: Tab to emails, open preview, close, move down, open next, repeat. Try full-screen on every 3rd email.
- **Compose**: Tab through all fields, type something, toggle preview, switch away and back. Is the draft preserved?

After the repetition burst, capture and compare against the Step 1 baseline for that tab. **Character-for-character comparison of the structural elements** (borders, header, status bar).

### Step 4: The Resize Gauntlet (single session, no restart)

With something open (email preview, contact detail, etc.):
1. Capture at 220x50
2. `tmux resize-window -t test -x 120 -y 40` → wait 1s → capture → defect check
3. `tmux resize-window -t test -x 80 -y 24` → wait 1s → capture → defect check
4. `tmux resize-window -t test -x 220 -y 50` → wait 1s → capture → compare with step 1
5. `tmux resize-window -t test -x 60 -y 18` → capture → should show size guard or degrade gracefully

At each size, explicitly verify:
- Header + tab bar present?
- Borders complete?
- Columns aligned?
- Status bar with correct content?

### Step 5: Cross-Tab State

1. Open something in Timeline (email preview). Switch to Compose (2). Switch back (1). Is the preview still there?
2. Open something in Contacts. Switch to Cleanup (3). Switch back (4). Is the state clean? (It should be — clean entry.)
3. Type something in Compose. Switch away. Switch back. Is the text preserved?
4. Open same email from Timeline, Cleanup, and Contacts. Compare the body text — must be identical.

### Step 6: Edge Cases

- `tmux send-keys -t test 'jjjjjjjjjjjjjjjjjjjj' ''` — 20 rapid keys. Capture. Valid state?
- Esc 10 times from any state. Clean baseline?
- Enter on empty (no contact selected, 0 search results). No crash?
- Navigate to last email → j (stay put). First email → k (stay put).
- Star → unstar → star → unstar 5 times. Aligned?

### Step 7: AI Features (if Ollama running or live IMAP)

- `a` in Timeline for classification → wait → check Tag column
- `c` for chat → ask a question → check response rendering
- `Ctrl+Q` on open email → quick reply picker → select one → check Compose
- `e` on contact → enrichment → check Company/Topics persist

After each AI operation, go back to Timeline and verify the table is still rendering correctly.

## The Iteration Loop

After your first full pass:

1. **What area had the most issues?** Go back there. Dig deeper. Try 30 operations instead of 15. Try at the size where things looked worst.
2. **Can you reproduce each bug?** Try the exact sequence again. Nail down the boundary (works for N, breaks at N+1).
3. **Do bugs from one area appear in others?** If contacts had a rendering issue, does the same data render wrong in cleanup?
4. **After fixing a bug, re-run the full sequence** that found it AND its neighbors.
5. **Capture "hunches"** — things that feel slightly off but you can't pinpoint. Write them in the report. They're worth re-checking next session.

## Reporting

Save to `reports/TEST_REPORT_YYYY-MM-DD_<description>.md`.

```markdown
# Battle Test Report — YYYY-MM-DD

## Session
- Mode: --demo / live
- Sizes tested: 220x50, 120x40, 80x24
- Iterations: N (how many times you circled back)

## Bugs Found
### [BUG-N] Title (Severity)
- **Symptom**: exact visual description
- **Capture**: (paste the relevant lines from tmux capture)
- **Reproduction**: key sequence
- **Boundary**: works until Nth operation / only at width < X
- **Root cause**: file:line if found
- **Fixed**: yes/no

## Regression Matrix
| Feature | 220x50 | 120x40 | 80x24 | After 15+ ops |
|---------|--------|--------|-------|---------------|
| Timeline table | | | | |
| Email preview | | | | |
| Full-screen | | | | |
| Cleanup 2-panel | | | | |
| Cleanup preview | | | | |
| Contacts list | | | | |
| Contact detail | | | | |
| Contact email preview | | | | |
| Compose fields | | | | |
| Sidebar toggle | | | | |
| Tab switching | | | | |
| Resize recovery | | | | |

## Hunches
Things that felt off but weren't confirmed. Check next time.
```
