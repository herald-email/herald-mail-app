---
title: Text Selection
description: Copy email text from Timeline preview and full-screen reading mode.
---

Text selection is available in Timeline preview and full-screen reading. It gives keyboard-first copying for a current line, a visual range, or the full wrapped body.

## Overview

Use text selection when you need to copy a quote, reference number, address, or full email body without leaving the terminal. Mouse-selection mode is also available when you prefer terminal-native selection.

## Screen Anatomy

| Area | What it shows |
| --- | --- |
| Wrapped body lines | The preview/full-screen body after Herald wraps text to the current width. |
| Scroll offset | The current top line in the preview body. |
| Visual selection | Highlighted range between selection start and end. |
| Pending line copy | A one-key waiting state after the first `y`. |
| Mouse-selection mode | Terminal mouse mode toggle that changes whether terminal-native selection is easier. |

<!-- HERALD_SCREENSHOT id="text-selection-visual-mode" page="text-selection" alt="Timeline preview visual selection mode" state="demo mode, 120x40, preview open, visual mode active" desc="Shows selected body lines, scroll position, and copy-related key hints." capture="tmux demo 120x40; ./bin/herald --demo; press 1; press enter; press v; press j" -->

![Timeline preview visual selection mode](/screenshots/text-selection-visual-mode.png)

## Controls

| Key | Context | Preconditions | Result |
| --- | --- | --- | --- |
| `v` | Timeline preview/full-screen | Wrapped body lines are available. | Toggles visual selection and starts at current scroll line. |
| `j` / `down` | Visual mode | Visual mode active. | Extends selection downward. |
| `k` / `up` | Visual mode | Visual mode active. | Shrinks or moves selection upward. |
| `y` | Visual mode | A range is selected. | Copies selected wrapped lines and exits visual mode. |
| `y` then `y` | Preview/full-screen | Body lines exist and visual mode is not active. | Copies the current visible line. |
| `Y` | Preview/full-screen | Body lines exist. | Copies the entire wrapped body. |
| `m` | Timeline | Any Timeline state. | Toggles mouse-selection mode. |
| `esc` | Visual mode | Visual mode active. | Cancels visual mode. |

## Workflows

### Copy a Range

1. Open an email preview.
2. Scroll to the first line.
3. Press `v`.
4. Use `j`/`k` to adjust the range.
5. Press `y`.

### Copy the Current Line

1. Open an email preview.
2. Scroll until the desired line is at the current body offset.
3. Press `y`.
4. Press `y` again.

### Copy the Whole Body

1. Open an email preview or full-screen reader.
2. Press `Y`.

### Use Terminal Mouse Selection

1. Press `m`.
2. Use your terminal's native mouse selection.
3. Press `m` again to return Herald to its normal mouse mode.

## States

| State | What happens |
| --- | --- |
| No body loaded | Copy keys have nothing to copy. |
| Visual mode | Navigation changes the selected range instead of just scrolling. |
| Pending `yy` | Herald waits for the second `y`; any other key clears the pending state. |
| Full-screen | Same copy controls apply with more body lines visible. |
| Clipboard unavailable | Copy command can fail if the operating system clipboard command is unavailable. |

## Data And Privacy

Text selection reads body text that is already displayed in Herald and writes selected text to the operating system clipboard. The clipboard may be visible to other local applications according to your OS security model.

## Troubleshooting

If copying the wrong line, remember that `yy` copies the current scroll line, not necessarily the row your cursor highlighted earlier.

If selected text has unexpected wrapping, widen the terminal or use full-screen mode before copying.

If clipboard copy fails, verify the local clipboard command for your platform is installed and available.

## Screenshot Placeholders

<!-- HERALD_SCREENSHOT id="text-selection-full-screen" page="text-selection" alt="Full-screen reader with visual selection" state="demo mode, 120x40, full-screen reader, visual mode" desc="Shows full-screen body text, expanded selection range, and copy controls." capture="tmux demo 120x40; ./bin/herald --demo; press 1; press enter; press z; press v; press j; press j" -->

![Full-screen reader with visual selection](/screenshots/text-selection-full-screen.png)

<!-- HERALD_SCREENSHOT id="text-selection-mouse-mode" page="text-selection" alt="Timeline mouse-selection mode active" state="demo mode, 120x40, Timeline preview, mouse mode toggled" desc="Shows status/key hint state after toggling mouse-selection mode for terminal-native selection." capture="tmux demo 120x40; ./bin/herald --demo; press 1; press enter; press m" -->

![Timeline mouse-selection mode active](/screenshots/text-selection-mouse-mode.png)

## Related Pages

- [Timeline](/using-herald/timeline/)
- [Global UI](/using-herald/global-ui/)
- [All Keybindings](/reference/keybindings/)
