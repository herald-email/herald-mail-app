---
name: tui-test
description: Battle-test the Herald TUI — prolonged stress testing, state accumulation bugs, AI features, and UX degradation over extended use. Not a quick pass — simulates days of real usage.
disable-model-invocation: true
allowed-tools: Bash Read Write Glob Grep Edit Agent TodoWrite
argument-hint: "[focus: full | stress | ai | contacts | timeline | cleanup | compose | navigation]"
---

# TUI Battle Testing

You are a power user who has been using Herald daily for weeks. You open dozens of emails per session, switch between tabs constantly, use AI features, resize your terminal, and expect everything to just work. Your goal is to break the app — find the bugs that only surface after prolonged use, repeated operations, and unusual interaction sequences.

## What Makes This Different from a Quick Pass

A quick pass opens one email and checks alignment. A battle test:

- Opens **20+ emails in sequence** and checks whether the 20th renders as cleanly as the 1st
- Performs the **same action from different entry points** (open email from Timeline vs Cleanup vs Contacts vs search results — same email, same rendering?)
- **Accumulates state** — star 5 emails, delete 3, search, clear search, star 2 more — is the list still correct?
- Exercises **AI round-trips** — classification, chat, quick replies, contact enrichment — and checks that stale AI state doesn't leak into subsequent views
- **Interleaves operations** — open email, switch to compose, come back, is the preview still there? Open chat, close it, is the layout restored exactly?
- Tests **the same flow at all three sizes without restarting** — resize from 220 to 80 mid-operation and check nothing corrupts

## Infrastructure

### Building and Launching

```bash
# Always rebuild before testing
go build -o bin/herald ./main.go

# Launch in tmux at target size
tmux new-session -d -s test -x 220 -y 50
tmux send-keys -t test '/path/to/bin/herald --demo' Enter
sleep 5
```

For live IMAP testing (when testing AI features or real email rendering):
```bash
tmux send-keys -t test '/path/to/bin/herald' Enter
sleep 8  # IMAP connect takes longer
```

### Capturing and Comparing

```bash
# Plain text capture
tmux capture-pane -t test -p > /tmp/cap.txt

# ANSI capture (for color/style verification)
tmux capture-pane -t test -p -e > /tmp/cap_ansi.txt

# Diff two captures to detect drift
diff /tmp/cap_before.txt /tmp/cap_after.txt
```

### Key Sequences

```bash
tmux send-keys -t test 'KEY' ''     # Single key (j, k, f, etc.)
tmux send-keys -t test Enter        # Enter
tmux send-keys -t test Escape       # Escape
tmux send-keys -t test Tab          # Tab
tmux send-keys -t test C-p          # Ctrl+P
tmux send-keys -t test C-q          # Ctrl+Q
tmux send-keys -t test C-s          # Ctrl+S
```

### Terminal Sizes

| Size | When to use |
|------|------------|
| 220x50 | Start here — full visibility, verify everything works at ideal size first |
| 120x40 | Mid-session resize — catch layout recalculation bugs |
| 80x24 | Stress the layout — columns hide, panels shrink, sidebar auto-hides |

**Critical**: Do not restart the app between size changes. Resize the live session to catch re-render bugs.

## Focus Areas ($ARGUMENTS)

- **`full`** — Run all phases below, end-to-end. Takes the longest, finds the most.
- **`stress`** — Phases 2-4 only (repetition, accumulation, interleaving). Skip first impressions.
- **`ai`** — Phase 5 only (classification, chat, quick replies, enrichment). Requires live IMAP or Ollama running.
- **`timeline`** / **`cleanup`** / **`contacts`** / **`compose`** / **`navigation`** — Targeted deep dive on one area.

If no argument, default to `full`.

## Phase 1: First Impressions & Baseline

Launch fresh at 220x50. Capture the initial screen as a baseline.

**Check:**
- Tab bar visible with all 4 tabs, correct highlighting
- Sidebar shows folders with unread/total counts
- Timeline table: Sender column populated, Subject column populated, columns aligned
- Status bar: folder name, counts, [DEMO] indicator, key hints matching the active tab
- No blank panels, no loading spinners that never resolve

**Capture this as `/tmp/baseline_220.txt`.** You'll compare later screens against this.

## Phase 2: Repetition Stress Testing

The goal is to find bugs that only appear after repeated operations — state that accumulates, cursors that drift, rendering that degrades.

### 2a. Open 20+ Emails in Sequence

Starting from the top of the timeline:
1. Press Enter to open email #1, wait 2s, capture
2. Press j (next email), wait 0.5s
3. Repeat steps 1-2 for at least 20 emails
4. After every 5th email, capture and compare column alignment to baseline
5. After the 20th email, press Esc to close preview

**Watch for:**
- Sender column going blank after N opens (ANSI corruption accumulating)
- Body preview showing the wrong email (stale state from previous open)
- Scroll position drifting — are you still on email #20 or did cursor jump?
- Memory of previously opened emails leaking into new preview
- Scroll indicator showing wrong line counts

### 2b. Star/Unstar Rapid Cycling

1. Navigate to email #5, press * to star
2. Note its new position (should sort to top)
3. Press * again to unstar, note it returns
4. Repeat for 10 different emails
5. Star 3 emails, then navigate through them

**Watch for:**
- Thread group sorting breaking after many star/unstar cycles
- Starred indicator (star) column misaligned after repeated toggling
- Cursor jumping to wrong position after star re-sort
- Duplicate rows appearing in the table

### 2c. Search/Clear Cycling

1. Press /, type "AWS", Enter — note result count
2. Open an email from results, close it
3. Press Esc to clear search — verify full list restored
4. Repeat with different search terms 5+ times
5. After 5 cycles, verify the full list matches the baseline capture

**Watch for:**
- Search results count drifting (showing wrong number)
- Previous search results bleeding into new search
- Full list not fully restoring after Esc (missing emails)
- Column widths changing between search and full view

### 2d. Delete and Archive Accumulation

1. Navigate to an email, press D, confirm with y
2. Verify the email disappears from the list
3. Press e on another email (archive)
4. Repeat delete/archive 5+ times
5. Check: does the total count in the status bar update correctly each time?
6. Check: are there phantom rows where deleted emails were?

## Phase 3: State Accumulation & Cross-Tab Corruption

### 3a. Tab Switching Under Load

With a preview open in Timeline:
1. Press 2 (Compose) — does compose show clean?
2. Type something in the To field
3. Press 1 (Timeline) — is the preview still open with the correct email?
4. Press 3 (Cleanup) — clean state?
5. Press 4 (Contacts) — clean state?
6. Press 1 (Timeline) — preview still correct?
7. Press 2 (Compose) — is the To field content still there?

**Repeat this cycle 3 times.** State leaks often appear on the 2nd or 3rd round.

### 3b. Panel Toggle Stress

In Timeline with preview open:
1. Press f (toggle sidebar) — preview stays, layout adjusts
2. Press c (toggle chat) — layout adjusts again
3. Press f again (sidebar back) — three panels visible
4. Press c again (chat closes) — back to timeline + preview
5. Compare to baseline — same layout as before toggles?
6. Resize to 80x24 with sidebar + chat + preview all trying to display — what happens?

### 3c. Cleanup → Contacts → Timeline Round-Trip

1. Go to Cleanup (3), open a sender's email in preview
2. Note which email is showing
3. Go to Contacts (4), open the same sender, open the same email
4. Verify the body matches
5. Go to Timeline (1), find the same email, open it
6. Verify the body matches
7. The same email opened from 3 different tabs should look identical

### 3d. Resize Gauntlet (DO NOT RESTART)

Without restarting the app, in a single session:
1. Start at 220x50, open email preview, capture
2. Resize to 120x40 — capture, check alignment
3. Resize to 80x24 — capture, check that preview still shows (may be narrow)
4. Resize to 220x50 — capture, compare with step 1
5. Resize to 60x20 — check minimum-size behavior
6. Resize back to 120x40 — verify recovery

**Watch for:**
- Column widths not recalculating after resize
- Preview panel not resizing (stuck at old width)
- Sidebar appearing/disappearing incorrectly
- Text wrapping corrupted after width change
- Status bar content overflowing or truncating incorrectly

## Phase 4: Interaction Sequence Stress

These are specific multi-step flows that historically break.

### 4a. The 20-Email Contacts Gauntlet

1. Go to Contacts (4)
2. Open contact #1, Tab to emails, open email — verify inline preview
3. Esc back to detail, Esc back to list
4. Open contact #2, repeat
5. Do this for ALL contacts in the list
6. After the last contact, go back to contact #1 — does it still work?
7. Open email from contact, Esc, switch to Timeline (1), back to Contacts (4) — clean state?

### 4b. Cleanup Full-Screen → Back → Different Email

1. Go to Cleanup (3), Tab to emails, open email #1 (Enter)
2. Go full-screen (z)
3. Exit full-screen (z)
4. Close preview (Esc)
5. Navigate to email #2, open it (Enter)
6. Does email #2 show? Or is email #1's body still cached?
7. Go full-screen (z), check body is email #2

### 4c. Compose Draft Persistence

1. Go to Compose (2)
2. Tab to To field, type "test@example.com"
3. Tab to Subject, type "Test Subject"
4. Tab to Body, type "Hello world"
5. Switch to Timeline (1), open an email, read it
6. Switch back to Compose (2) — is the draft still there?
7. Switch to Cleanup (3), do some browsing
8. Switch back to Compose (2) — still there?

### 4d. Search → Open → Full-Screen → Back

1. In Timeline, press /, search for something
2. From results, open an email (Enter)
3. Go full-screen (z)
4. Exit (z or Esc)
5. Press Esc to close preview
6. Press Esc to clear search
7. Verify: full email list restored, no phantom search state

## Phase 5: AI Features (requires Ollama or live backend)

Skip this phase in `--demo` mode unless AI features have demo stubs.

### 5a. Classification

1. In Timeline, press `a` to run AI classification
2. Watch the status bar for progress
3. After completion, verify Tag column shows categories
4. Open a classified email — does the tag match the content?
5. Press `A` to re-classify — does the tag update?
6. Switch to Cleanup — are tags visible there too?

### 5b. Chat Panel

1. Press `c` to open chat
2. Type a question about emails (e.g., "show me emails from AWS")
3. Wait for response — does it render in Markdown?
4. If it returns email results, does the filtered timeline appear?
5. Press Esc or type "show all" to restore full timeline
6. Verify: full list restored, no phantom filter

### 5c. Quick Replies

1. Open an email in Timeline (Enter)
2. Press Ctrl+Q to open quick reply picker
3. Navigate with arrow keys — are all options visible?
4. Press Enter on one — does it open Compose pre-filled?
5. Esc back to Timeline
6. Open a DIFFERENT email, Ctrl+Q — are the replies different? (They should be if AI-generated)

### 5d. Contact Enrichment

1. Go to Contacts (4)
2. Select a contact, press `e` to enrich
3. Wait for enrichment to complete
4. Verify: Company and Topics fields update
5. Esc, re-open the same contact — is enrichment data persisted?

## Phase 6: Edge Cases & Adversarial Inputs

### 6a. Rapid-Fire Keys

Send 20+ keys in under a second:
```bash
tmux send-keys -t test 'jjjjjjjjjjjjjjjjjjjj' ''
sleep 1
```
Capture — is the cursor at a valid position? Any rendering artifacts?

### 6b. Esc Spam

```bash
tmux send-keys -t test Escape Escape Escape Escape Escape
sleep 0.5
```
From any state — does repeated Esc eventually reach a clean baseline?

### 6c. Enter on Empty States

- Contacts tab with no contact selected: press Enter — should do nothing
- Timeline with 0 emails (empty folder): press Enter — should do nothing
- Search with 0 results: press Enter — should do nothing

### 6d. Boundary Emails

- Navigate to the VERY LAST email in the list — press j — cursor should stay
- Navigate to the VERY FIRST email — press k — cursor should stay
- Open the last email, close it, open the first — body renders correctly?

### 6e. Long Content

If any email has a very long body:
- Open it, scroll to the very bottom — does percentage reach 100%?
- Scroll back up — does percentage track accurately?
- Go full-screen, scroll to bottom — same behavior?

## Reporting

Save report to `reports/TEST_REPORT_YYYY-MM-DD_<description>.md`.

### Report Structure

```markdown
# Battle Test Report — YYYY-MM-DD

## Test Configuration
- Mode: --demo / live IMAP
- Duration: how long the session ran
- Phases completed: 1-6 / subset

## Bugs Found
### [BUG-N] Short title (Severity: critical/major/moderate/minor)
- **Symptom**: What the user sees
- **Reproduction**: Exact key sequence
- **After how many operations**: 1st time? Only after 20th email open?
- **Root cause**: file:line if identified
- **Fixed**: yes/no (if yes, describe fix)

## State Accumulation Issues
Any behavior that degrades over time but works on first try.

## AI Feature Results
Classification accuracy, chat responsiveness, quick reply relevance.

## Regression Matrix
| Tab/Feature | 220x50 | 120x40 | 80x24 | After 20+ ops |
|-------------|--------|--------|-------|---------------|
| Timeline list | | | | |
| Email preview | | | | |
| Full-screen | | | | |
| Star/unstar | | | | |
| Search | | | | |
| Compose | | | | |
| Cleanup 2-panel | | | | |
| Cleanup preview | | | | |
| Contacts list | | | | |
| Contact preview | | | | |
| Sidebar toggle | | | | |
| Chat panel | | | | |

## Key Captures
Include captures that show bugs (before/after pairs are especially useful).
```

## UX Principles (Reference)

When evaluating any screen, hold it to these standards:

- **Visual consistency**: Every row in a table must align. Same styling for same semantics.
- **Information hierarchy**: Most important info visible at 80x24. More detail at wider sizes.
- **Navigation predictability**: Esc always goes up one level. Tab entry = clean state. No cross-tab corruption.
- **Responsive layout**: Resize never corrupts. Graceful degradation, not broken layout.
- **State honesty**: If an email was deleted, it's gone. If search was cleared, full list is back. No ghosts.
- **Temporal stability**: The 20th operation should render identically to the 1st. No drift, no accumulation.
