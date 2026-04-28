# Compose Attachment Path Autocomplete

## Overview

This spec defines terminal-style filesystem autocomplete for the Compose attachment prompt. It matters because `Ctrl+A` already exposes a path input, and users expect `Tab` in that input to behave like a terminal path prompt.

- [x] Scope is limited to the Compose `Ctrl+A` attachment flow.
- [x] Existing To/CC/BCC contact autocomplete remains unchanged.
- [x] Sending attachments continues to use the existing staged attachment and SMTP paths.

## User Behavior

This section describes the visible interaction contract. The prompt should favor familiar terminal completion semantics over a fuzzy picker or file browser.

- [x] Pressing `Ctrl+A` opens the existing `Attach file:` path prompt.
- [x] Pressing `Tab` completes a unique match or the longest common prefix for multiple matches.
- [x] Pressing `Tab` again when no longer prefix can be inserted shows a compact suggestion list.
- [x] Pressing `Tab` while the list is visible cycles forward through matches and updates the input.
- [x] Pressing `Shift+Tab`, `up`, or `down` while the list is visible moves the selected match.
- [x] Pressing `Enter` on a file attaches it.
- [x] Pressing `Enter` on a directory keeps the prompt open with the directory path plus a trailing separator.
- [x] Pressing `Esc` cancels the attachment prompt and clears suggestions.

## Filesystem Rules

This section defines how matches are discovered and displayed. The rules intentionally stay local and predictable.

- [x] Support `~`, absolute paths, relative paths, and current-directory names.
- [x] Search only the current path segment directory, not recursively.
- [x] Hide dotfiles unless the typed filename segment starts with `.`.
- [x] Sort directories before files, then case-insensitively by display name.
- [x] Display directories with a trailing `/`.
- [x] Insert paths literally, including spaces, with no shell escaping.
- [x] On no matches or unreadable directories, leave the input unchanged and show a bounded Compose status.

## Layout And Testing

This section covers the acceptance evidence needed before the feature is considered complete. The Compose view must stay usable at normal and narrow terminal sizes.

- [x] Reserve rows for the attachment suggestions so Compose chrome and key hints stay visible at `220x50` and `80x24`.
- [x] Show at most five suggestion rows.
- [x] Add focused Go tests for completion, list visibility, navigation, hidden-file filtering, and prompt key routing.
- [x] Add a render regression test for an active suggestion list at `80x24`.
- [x] Verify with tmux in demo mode at `220x50`, `80x24`, and `50x15`.
