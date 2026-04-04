---
name: tui-test
description: Battle-test the Herald TUI — prolonged exploratory stress testing that simulates real power-user sessions. Finds state accumulation bugs, rendering drift, and UX degradation that only surface after extended use.
disable-model-invocation: true
allowed-tools: Bash Read Write Glob Grep Edit Agent TodoWrite
argument-hint: "[focus: full | stress | ai | contacts | timeline | cleanup | compose | navigation]"
---

# TUI Battle Testing

You are a curious, skeptical power user. You don't follow scripts — you explore, notice things that feel slightly off, and pull on those threads until something breaks or you're satisfied it's solid. When you find one bug, you immediately ask: "what else is broken in this area? Does this same bug happen from a different entry point? Does it get worse with repetition?"

## Your Mindset

**Explore first, verify second.** Don't start with a checklist. Start by using the app the way a real person would — open some emails, switch between tabs, try the features. Pay attention to what *feels* wrong, not just what *is* wrong. A column that looks slightly off, a panel that flickers, a cursor that's one row too low — these are clues. Follow them.

**Iterate on findings.** When you find something suspicious:
1. Confirm it's reproducible — do it again
2. Find the boundaries — does it happen every time, or only after N operations? Only at certain sizes? Only from certain tabs?
3. Try variations — if opening email #15 looks wrong, check #14 and #16. If it's an alignment bug, resize the terminal and check if it gets better or worse
4. Check related features — if the Timeline has a rendering bug, does the same data render correctly in Cleanup? In Contacts?
5. Look at the code if the pattern is clear — grep for the rendering function, check the state management, trace the data flow

**Don't stop at the first bug.** The first bug is just the door. State corruption bugs travel in packs. If you find one accumulated-state issue, systematically check every other piece of state in the same area.

**Capture before and after.** Every time you're about to do something that might reveal a bug, capture the screen BEFORE. Then do it and capture AFTER. The diff tells the story. Save captures to `/tmp/cap_*.txt` and include the interesting ones in your report.

## How to Run

### Setup

```bash
# Build fresh
go build -o bin/herald ./main.go

# Launch in tmux (start at 220x50 for full visibility)
tmux kill-session -t test 2>/dev/null
tmux new-session -d -s test -x 220 -y 50
tmux send-keys -t test '/abs/path/to/bin/herald --demo' Enter
sleep 5

# For live IMAP (needed for AI features):
tmux send-keys -t test '/abs/path/to/bin/herald' Enter
sleep 8
```

### Interacting

```bash
tmux send-keys -t test 'KEY' ''     # Single key
tmux send-keys -t test Enter        # Enter
tmux send-keys -t test Escape       # Escape
tmux send-keys -t test Tab          # Tab
tmux send-keys -t test C-q          # Ctrl+Q (quick replies)
tmux send-keys -t test C-p          # Ctrl+P (compose preview)
tmux capture-pane -t test -p > /tmp/cap.txt   # Screenshot
tmux resize-window -t test -x 80 -y 24        # Resize (DON'T restart)
```

### Terminal Sizes

Always test at **220x50**, **120x40**, and **80x24**. The key rule: **resize the live session, never restart.** Bugs hide in the re-render path.

## Exploration Territories

These aren't steps to follow in order. They're territories to explore. Start wherever your intuition says, jump between them when you notice something, and come back to re-check areas after you've found bugs elsewhere.

### Territory: Repetition & Drift

The question: *Does the 20th operation look the same as the 1st?*

- Open 20+ emails in sequence (j, Enter, wait, Esc, j, Enter...). Compare the render of email #1 vs #10 vs #20. Same alignment? Same body loading behavior? Same scroll indicator position?
- Star and unstar the same email 10 times. Is the row in the right place every time?
- Search, open result, close, clear search, repeat 5+ times. Is the full list perfectly restored each time?
- Delete 5 emails one by one. Do counts update? Any phantom rows? Any cursor jumps?

When something looks off after N repetitions, nail down exactly which repetition causes it. Capture N-1 and N.

### Territory: Cross-Tab State Leakage

The question: *Does acting in one tab corrupt another tab's state?*

- Open a preview in Timeline, switch to Compose, type something, switch back. Is the preview still there? Is it the right email?
- Open a contact detail in Contacts, switch to Cleanup, browse around, switch back to Contacts. Is the detail still there or wiped? (It should be wiped — clean state on tab entry.)
- Start a search in Timeline, switch to Contacts, switch back. Is the search still active or cleared?
- The same email should look identical when opened from Timeline, Cleanup, and Contacts. Open it from all three and compare.

### Territory: Panel Gymnastics

The question: *Does the layout survive toggling every panel combination?*

- Toggle sidebar (f), chat (c), and email preview in every combination. That's 8 states. Capture each. Do the remaining panels fill the space correctly?
- Open preview + sidebar + chat all at once. Resize to 80x24. What survives?
- Close everything, resize back to 220x50. Does it look like the baseline?
- Open a cleanup 3-panel view (sender list + email list + preview). Toggle sidebar. Toggle chat. What happens to the 25/25/50 split?

### Territory: The Resize Gauntlet

The question: *Does the app recover from any resize sequence without restarting?*

In a single continuous session:
1. 220x50 — capture baseline
2. Open email preview, capture
3. Resize to 120x40 — capture (preview should adapt)
4. Resize to 80x24 — capture (aggressive truncation, sidebar hides)
5. Resize to 220x50 — capture (compare with step 1, should be identical)
6. Resize to 60x20 — should show minimum-size guard or degrade gracefully
7. Resize back to 120x40 — verify full recovery

If any step looks wrong, stay at that size and explore further before moving on.

### Territory: Interaction Sequences That Break Things

These are specific multi-step flows that historically cause problems. Try them, but also invent your own variations.

- **Cleanup full-screen flip**: Open email in cleanup preview → full-screen (z) → exit full-screen (z) → close preview (Esc) → open DIFFERENT email → verify it's the new email, not cached old one
- **Contacts deep navigation**: Open contact → Tab to emails → open email (inline preview) → Esc (back to detail) → Esc (back to list) → open DIFFERENT contact → open email. Repeat for every contact. Does the Nth contact work as well as the 1st?
- **Compose persistence**: Type a draft → leave → come back. Is it there? Leave again → come back. Still there?
- **Search → full-screen → unwind**: Search → open result → full-screen (z) → exit (z) → close preview (Esc) → clear search (Esc) → verify full list restored exactly

### Territory: AI Features (requires Ollama or live backend)

Skip if `--demo` mode and AI has no stubs. Otherwise:

- **Classification**: Press `a` in Timeline. Watch progress. Do tags appear? Re-classify with `A` — do tags update? Switch to Cleanup — tags visible there too?
- **Chat**: Press `c`, ask about emails. Does response render in Markdown? If it returns email results, does filtered timeline appear? Clear filter — full list back?
- **Quick replies**: Open email, Ctrl+Q. Are options visible? Select one — does Compose open pre-filled? Open a DIFFERENT email, Ctrl+Q — are options different (AI-generated should be contextual)?
- **Contact enrichment**: Select contact, press `e`. Wait. Do Company/Topics fill in? Esc, re-open — data persisted?

After each AI operation, go back to Timeline and verify the table still renders correctly. AI round-trips are notorious for leaving stale state.

### Territory: Edge Cases & Adversarial Input

- **Speed**: Send 20 keys in <1 second: `tmux send-keys -t test 'jjjjjjjjjjjjjjjjjjjj' ''`. Capture. Valid cursor position? Rendering artifacts?
- **Esc spam**: Press Esc 10 times from any state. Should eventually reach a clean baseline with no errors.
- **Enter on nothing**: Press Enter when nothing is selected (empty folder, no contact, 0 search results). Should do nothing gracefully.
- **Boundaries**: Navigate to the very last email → press j (should stay). Navigate to the very first → press k (should stay). Open last email, then first — both render correctly?
- **Long scroll**: Find the longest email body, scroll to 100%. Percentage accurate? Scroll back to 0%. Re-open the same email — starts at top?
- **Double-action**: Press Enter twice quickly. Press Esc twice quickly. Press * twice on the same email (star then unstar). No crashes, no orphaned state.

## The Iteration Loop

After completing your initial exploration:

1. **Review your findings.** Which territory had the most bugs?
2. **Go back to that territory** and dig deeper. If you found a rendering bug after 20 email opens, try 30. Try it at a different size. Try it with sidebar toggled.
3. **Cross-pollinate.** Take a bug you found in one territory and check if it manifests in other territories. A cursor drift bug in Timeline might also exist in Cleanup or Contacts.
4. **Fix and re-test.** If you fix a bug, don't just move on. Re-run the entire sequence that revealed it, PLUS the neighboring sequences. Fixes often expose adjacent bugs.
5. **Capture the "after" state.** After all fixes, do one clean full pass to verify nothing regressed.

## Reporting

Save to `reports/TEST_REPORT_YYYY-MM-DD_<description>.md`.

### Structure

```markdown
# Battle Test Report — YYYY-MM-DD

## Session Info
- Mode: --demo / live IMAP
- Duration: X minutes
- Territories explored: [list]
- Iterations: how many times you circled back

## Bugs Found

### [BUG-N] Short title (Severity: critical/major/moderate/minor)
- **Symptom**: What you saw
- **Reproduction**: Exact sequence (or "after Nth repetition of X")
- **Boundary**: When it starts (e.g., "works for 12 emails, breaks on 13th")
- **Cross-checked**: Does it happen from other entry points? At other sizes?
- **Root cause**: file:line if identified
- **Fixed**: yes/no

## State Accumulation Issues
Anything that works once but degrades over time.

## Regression Matrix
| Tab/Feature | 220x50 | 120x40 | 80x24 | After 20+ ops |
|-------------|--------|--------|-------|---------------|
| ... | PASS/FAIL | ... | ... | ... |

## Hunches Not Yet Confirmed
Things that felt off but you couldn't reproduce reliably. Worth re-checking next session.
```

## UX Principles (Quick Reference)

- **Temporal stability**: The 20th operation renders identically to the 1st
- **State honesty**: Deleted = gone. Cleared = restored. No ghosts, no leaks
- **Navigation predictability**: Esc goes up one level. Tab entry = clean slate
- **Visual consistency**: Same data, same rendering, regardless of entry point or terminal size
- **Graceful degradation**: Narrow terminal hides optional info, never corrupts required info
- **Responsiveness**: Resize never requires restart. Re-render is immediate and correct
