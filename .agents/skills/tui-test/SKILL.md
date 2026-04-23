---
name: tui-test
description: "Battle-test terminal UIs such as Herald via tmux: reset to known states, compare visible hotkeys with actual behavior, capture ANSI and PNG evidence, and run resize soak and thrash loops to catch stale layout, focus, and overlay bugs."
disable-model-invocation: true
allowed-tools: Bash Read Write Glob Grep Edit Agent TodoWrite
argument-hint: "[app: herald | generic] [focus: full | hotkeys | resize | timeline | compose | cleanup | contacts | ai]"
---

# TUI Battle Testing

Use this skill to audit terminal UIs through tmux. It is generic by default, with a ready-made Herald profile for this repo.

## Deliverables

- A bug ledger with exact repro steps, terminal size, cycle count, expected vs actual behavior, and evidence paths.
- A short stable-observations section for areas you exercised that did not regress.
- An evidence folder with plain-text captures for every critical state and PNG screenshots for baseline, first broken state, and final recovery state.

## Non-Negotiables

- Use one tmux session per audited surface. Do not restart the app inside a resize soak.
- Before every scenario: reset, confirm reset, then capture the baseline.
- Build the action inventory from the visible hint bar first. Use docs or code only to supplement missing or suspicious actions.
- Use both text and PNG evidence. Text is for grep and diffs; PNG is for real visual judgment.
- Treat these as first-class bugs:
  - missing header or tab bar
  - stale or incorrect hint bar
  - silent hotkey no-op
  - focus drift or wrong active border
  - overlay persistence into unrelated states
  - stale status text leaking across tabs
  - resize recovery failure after returning to a large terminal
- On live configs, stop at the last non-mutating state for destructive actions unless the user explicitly approved the exact target.

## Step 1: Build A Lightweight App Manifest

For any app, note these before going deep:

- launch command
- reset recipe
- tabs or major screens
- global keys
- risky keys
- minimum-size guard text
- high-value states to open before resize testing

For Herald in this repo, use this default profile:

- Launch live:
  ```bash
  go build -o /tmp/herald ./main.go
  tmux kill-session -t test 2>/dev/null || true
  tmux new-session -d -s test -x 220 -y 50
  tmux send-keys -t test '/tmp/herald -config ~/.herald/conf.yaml' Enter
  sleep 10
  ```
- Launch demo:
  ```bash
  go build -o /tmp/herald ./main.go
  tmux kill-session -t test 2>/dev/null || true
  tmux new-session -d -s test -x 220 -y 50
  tmux send-keys -t test '/tmp/herald --demo' Enter
  sleep 5
  ```
- Reset recipe:
  ```bash
  tmux send-keys -t test Escape
  sleep 0.5
  tmux send-keys -t test Escape
  sleep 0.5
  tmux send-keys -t test '1'
  sleep 1
  ```
- Confirm reset by capture, not by assumption.
- Tabs:
  - `1` Timeline
  - `2` Compose
  - `3` Cleanup
  - `4` Contacts
- Global keys to try from each tab when visible and safe:
  - `1`, `2`, `3`, `4`
  - `Esc`
  - `Tab`
  - `f`
  - `c`
  - `l`
- High-value Herald states:
  - Timeline baseline
  - Timeline preview open
  - Timeline logs open
  - Compose blank form
  - Compose autocomplete dropdown open
  - Cleanup baseline
  - Cleanup preview open
  - Contacts list baseline
  - Contacts inline email preview open
- Live-risk keys:
  - `D`, `e`, `u`, `ctrl+s`
  - In live mode, stop at the confirmation or pre-send state unless explicitly allowed.
- Minimum-size guard:
  - At `50x15`, Herald should show `Terminal too narrow` and recover cleanly when resized larger.

## Step 2: Set Up Repeatable Helpers

Use helpers like these in the shell you control:

```bash
ROOT=$(pwd)
SKILL_DIR="$ROOT/.agents/skills/tui-test"
SESSION=test
EVIDENCE_DIR="$ROOT/reports/tui-audit_$(date +%F_%H%M%S)"
mkdir -p "$EVIDENCE_DIR"

cap() {
  tmux capture-pane -t "$SESSION" -p > "$EVIDENCE_DIR/$1.txt"
}

shot() {
  "$SKILL_DIR/screenshot.sh" "$SESSION" "$EVIDENCE_DIR/$1.png" "${2:-1600x900}" >/dev/null
}

cap_ansi() {
  tmux capture-pane -t "$SESSION" -p -e > "$EVIDENCE_DIR/$1.ansi.txt"
}
```

Use the bundled resize helper for soak and thrash loops:

```bash
"$SKILL_DIR/scripts/resize_cycle.sh" "$SESSION" "$EVIDENCE_DIR" timeline-preview
"$SKILL_DIR/scripts/resize_cycle.sh" "$SESSION" "$EVIDENCE_DIR" timeline-preview-thrash \
  "220x50,80x24,220x50,80x24,220x50,80x24,220x50,50x15,80x24,50x15,80x24,220x50"
```

The resize helper writes one plain-text capture per size transition. Add PNG screenshots on:

- the baseline
- the first broken step
- the final recovery step

## Step 3: The Core Loop

Run this loop for every tab, overlay, or risky hotkey cluster:

1. Reset.
2. Capture and confirm the reset actually worked.
3. Open one target state.
4. Capture text and PNG for the baseline state.
5. Exercise one advertised hotkey or one small cluster of related keys.
6. Capture again.
7. Compare the visible hint bar to what actually happened.
8. If the state is high value, run the resize cycle.
9. If the state is fragile, run the thrash cycle too.
10. Capture the first broken step and the final large-size recovery step.
11. Reset again and confirm reset before moving on.

## What To Assert

### Chrome

- Header is still visible when above minimum size.
- Tab bar is still visible when above minimum size.
- Status bar exists and matches the current tab or overlay.
- Hint bar exists and matches the current tab or overlay.

### Hint vs Behavior

- Every visible hotkey you try should either work or produce a bounded, user-visible fallback.
- If a panel or overlay opens, the hint bar should change with it.
- If a key is hidden by the current state, the UI should not continue advertising it.

### Focus and Overlays

- Only one panel should look focused.
- `Tab` and `Shift+Tab` should move focus predictably.
- `Esc` should unwind overlays in the correct order.
- Opening a new overlay should not silently leave another overlay active off-screen.

### Resize Recovery

- Shrinking to the minimum-size guard should not corrupt later large-size renders.
- Returning to `220x50` should clear stale `too narrow`, clipped, or collapsed layout state.
- A state that is open before the resize cycle should either remain open correctly or fall back cleanly with visible explanation.

### Data Density

- Dense contact autocomplete lists should not push the active field off-screen.
- Long senders, long subjects, and real-world contact/company data should not destroy the layout.
- Column truncation should still leave the screen understandable.

## Herald Hotspots

Prioritize these in this repo:

- Timeline:
  - baseline
  - `Enter` preview open and `Esc` close
  - `/` search open and `Esc` close
  - `l` logs
  - `c` chat
  - delete or archive confirmation entry only in live mode
- Compose:
  - field traversal with `Tab`
  - autocomplete with real contacts
  - `ctrl+p` preview
  - `ctrl+g` AI panel when configured
  - attachment path prompt open and cancel
- Cleanup:
  - summary/detail focus movement
  - preview open and close
  - `d` domain toggle
  - destructive keys only up to the last safe state in live mode
- Contacts:
  - incremental search with real dense results
  - contact detail
  - contact email list
  - inline email preview

## Resize Soak Rules

Default cycle:

```text
220x50 -> 120x40 -> 80x24 -> 50x15 -> 80x24 -> 120x40 -> 220x50
```

Default thrash:

```text
220x50 <-> 80x24 three times, then 80x24 <-> 50x15 twice, then back to 220x50
```

Run the full cycle at least twice for high-value states. Log the first cycle count or transition count where the UI diverges.

## Reporting

Write the report under `reports/` with a stable name such as:

```text
reports/TEST_REPORT_YYYY-MM-DD_<short-description>.md
```

Use this structure:

```markdown
# TUI Audit Report — YYYY-MM-DD

## Session
- Mode: live / demo
- Binary:
- Config:
- Sizes:
- Resize cycle:
- Thrash cycle:

## Findings
1. [Severity] Title
   Repro:
   Expected:
   Actual:
   First observed on cycle:
   Evidence:

## Stable Or Lower-Risk Observations
- ...

## Testability Notes
- ...
```

Every finding should include:

- title
- severity
- tab or state
- terminal size
- cycle count if resize-related
- exact key sequence
- expected behavior
- actual behavior
- text evidence path
- screenshot evidence path

## Generic Adaptation Notes

If the current app is not Herald:

- discover the manifest first
- derive actions from the visible hint bar
- replace the Herald reset recipe with the app's actual reset recipe
- keep the same core loop, resize soak, evidence format, and safety model

The harness should stay stable even when the app changes. The app profile is the part that should be easy to swap.
