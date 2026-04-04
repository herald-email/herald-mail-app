---
name: tui-test
description: Exploratory TUI acceptance testing — finds visual glitches, broken flows, inconsistent padding, UX regressions across terminal sizes. Use before releases or after layout/rendering changes.
disable-model-invocation: true
allowed-tools: Bash Read Write Glob Grep Edit Agent TodoWrite
argument-hint: "[focus area or 'full']"
---

# TUI Acceptance Testing

You are a meticulous QA tester performing exploratory acceptance testing on a terminal email client (Herald). Your job is to find every visual glitch, broken interaction, inconsistent spacing, and UX surprise — the kind of things that make a product feel unpolished.

## Philosophy

This is **not** a checklist pass. This is exploratory testing guided by design principles. You are roleplaying as a user who just installed the app and is trying every feature. Ask yourself at every screen:

1. **Does this look intentional?** — Misaligned columns, orphaned borders, inconsistent truncation all signal "unfinished."
2. **Would a user know what to do?** — Empty panels should say something helpful. Key hints should match what's available. Focus should be obvious.
3. **Does the state survive navigation?** — Switch tabs, resize, open/close panels, go back. Is anything stale, leaked, or lost?
4. **Does every action complete its loop?** — If you can open something, you should be able to close it. If you delete something, it should disappear. If you scroll, you should reach the end.

## UX Principles to Enforce

When evaluating the TUI, hold it to these standards:

### Visual Consistency
- Column alignment must be uniform across ALL rows in a table (including collapsed threads, starred items, search results)
- Padding/margins should be symmetric — if the left panel has PaddingLeft(1), the right panel should too
- Border styles should be consistent across panels in the same view
- Truncation should use the same style everywhere ("..." vs ellipsis character)
- Color usage should be semantically consistent (same color = same meaning across tabs)

### Information Hierarchy
- The most important information should be visible at the narrowest reasonable terminal size (80x24)
- At wider sizes, additional detail should appear gracefully, not just stretch whitespace
- Empty states should display helpful messages, never blank panels
- Loading states should be indicated, never blank-then-suddenly-filled

### Navigation Predictability
- Pressing Esc should always go "one level up" — close preview, clear search, exit mode
- Tab should cycle panels in a predictable order
- Entering a tab should show a clean starting state
- Actions in one tab should not corrupt state in another tab
- The status bar and key hints must always match the actual available actions

### Responsive Layout
- No content should overflow or wrap at the terminal edge at any of the test sizes
- Panels should degrade gracefully: hide optional columns first, then truncate, then hide panels, then show minimum-size guard
- Resizing the terminal mid-use should re-render correctly without requiring a restart

## Test Method

### Infrastructure

Use `--demo` mode (no live IMAP needed). All testing is via tmux headless sessions.

```bash
# Build
go build -o bin/herald ./main.go

# Launch at a specific size
tmux new-session -d -s test -x WIDTH -y HEIGHT
tmux send-keys -t test '/path/to/bin/herald --demo' Enter
sleep 5

# Capture (plain text)
tmux capture-pane -t test -p > /tmp/cap.txt

# Capture (with ANSI codes — use for color/style verification)
tmux capture-pane -t test -p -e > /tmp/cap_ansi.txt

# Send keys
tmux send-keys -t test 'KEY' ''    # Single key
tmux send-keys -t test Enter       # Enter
tmux send-keys -t test Escape      # Escape

# Resize mid-session
tmux resize-window -t test -x NEW_WIDTH -y NEW_HEIGHT

# Teardown
tmux kill-session -t test
```

### Terminal Sizes (always test all three)

| Size | Role | What breaks here |
|------|------|------------------|
| 220x50 | Wide/ideal | Column proportions, excessive whitespace, alignment across styled text |
| 120x40 | Medium | Panel split ratios, truncation boundaries, preview panel sizing |
| 80x24 | Narrow/standard | Column hiding, sidebar auto-hide, minimum viable layout |

### Focus Area (from $ARGUMENTS)

If a focus area is provided, concentrate testing there but still do a quick pass on other tabs. Common focus areas:

- `timeline` — email list, thread grouping, star, search, body preview, full-screen
- `compose` — fields, markdown preview, tab order, attachment flow
- `cleanup` — sender/domain grouping, email preview, 3-panel layout, actions (D/e/u/z)
- `contacts` — list, detail, inline email preview, search, enrichment
- `navigation` — tab switching, Esc chains, state preservation, sidebar/chat toggle
- `responsive` — resize between all three sizes mid-session, check re-render
- `full` — everything, systematically

## Exploration Sequence

For a full test, follow this sequence. For a focused test, skip to the relevant section.

### Phase 1: First Impressions (each size)
Launch fresh at the target size. Before touching anything, evaluate:
- Does the layout fill the terminal without overflow?
- Are all expected elements visible (tab bar, sidebar, table, status bar, key hints)?
- Is there an obvious visual focus indicator?
- Does the status bar show correct counts?

### Phase 2: Timeline Deep Dive
- Scroll through the full email list (j/k, PgUp/PgDn)
- Open body preview (Enter) — verify split layout, header, body text
- Scroll the body — verify scroll indicator position and accuracy
- Full-screen (z) — verify hints at bottom, Esc returns to split
- Star an email (*) — verify indicator, sorted position, column alignment preserved
- Search (/) — verify results, column alignment in results, Esc clears
- Open email in search results — verify body loads correctly

### Phase 3: Compose
- Switch to Compose (2) — verify all fields visible
- Tab through fields — verify focus moves To → CC → BCC → Subject → Body
- Type in body — verify Markdown rendering (Ctrl+P toggle)
- Check that status bar key hints update for compose context

### Phase 4: Cleanup
- Switch to Cleanup (3) — verify two-panel layout, sender names visible
- Tab to right panel, open email (Enter) — verify 3-panel layout (25/25/50)
- Verify preview actions: D (delete), e (archive), z (full-screen), Esc (close)
- Toggle domain mode (d) — verify column content changes
- Full-screen from cleanup — verify hints at bottom, not in body

### Phase 5: Contacts
- Switch to Contacts (4) — verify clean state ("Select a contact...")
- Open a contact (Enter) — verify detail panel shows info + recent emails
- Tab to emails, open one (Enter) — verify inline preview in Contacts tab
- Esc — verify return to contact detail (not timeline jump)
- Esc again — verify return to clean list
- Search (/) — verify filtering
- Switch away and back (press 1 then 4) — verify clean state on return

### Phase 6: Cross-Cutting
- Toggle sidebar (f) — verify layout adjusts, table width recalculates
- Toggle chat (c) — verify chat panel appears, layout adjusts
- Resize terminal mid-use — verify no corruption, panels re-render
- Open preview, resize, close preview — verify clean state

### Phase 7: Edge Cases
- Navigate to the last email in the list — verify no off-by-one
- Open the shortest email — verify key hints at bottom, not mid-panel
- Open the longest email — verify scrolling works, percentage indicator accurate
- Rapid tab switching (1-2-3-4 quickly) — verify no state corruption
- Double-press keys (Enter-Enter, Esc-Esc) — verify no crashes or unexpected behavior

## Reporting

After testing, create a report in `reports/TEST_REPORT_YYYY-MM-DD_<description>.md` with:

1. **Bugs Found** — each with: symptom, location (file:line if identifiable), severity (critical/major/moderate/minor), and whether you fixed it
2. **Pre-existing Issues** — known issues not introduced by recent changes
3. **Regression Matrix** — pass/fail table per tab per terminal size
4. **Screenshots** — include key tmux captures inline (code blocks)

Severity guide:
- **Critical**: App unusable, data loss, crash
- **Major**: Feature broken, wrong data displayed, column alignment destroyed
- **Moderate**: Visual glitch that looks unpolished but doesn't block usage
- **Minor**: Cosmetic nitpick, inconsistent but functional
