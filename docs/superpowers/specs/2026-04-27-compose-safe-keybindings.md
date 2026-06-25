# Compose-Safe Keybindings

## Overview

This spec defines Herald's compose-safe command layer. It keeps global navigation and overlay actions reachable while preventing normal draft characters from being stolen by global shortcuts.

## User-Visible Behavior

These requirements describe what a user can observe in the main TUI after the change.

- [x] Plain `q`, letters, and digits type into focused Compose fields instead of quitting, switching tabs, or opening overlays.
- [x] `Ctrl+C` quits from every state, including Compose, search inputs, chat, and overlays.
- [x] Plain `q` still quits from browse contexts where no text input is being edited.
- [x] Plain `1/2`, `l`, `c`, `f`, and `r` continue to work in browse contexts where they do not conflict with text entry; plain `3` is reserved for quick replies.
- [x] `F1/F2/F3` switch to Timeline, Contacts, and Contacts legacy alias from anywhere in the main TUI, including Compose.
- [x] Bottom key hints and the tab bar use `1-2` as the primary visible tab-switching annotation across tabs.
- [x] `Alt+1/2` remain supported as secondary aliases for terminals that reliably send Alt-modified digits.
- [x] Printable Alt chords such as `Alt+L`, `Alt+C`, `Alt+F`, and `Alt+R` remain local to Compose text entry and do not toggle logs, chat, sidebar, or refresh while a draft field is focused.

## Routing Rules

The key router should make the most specific text-entry state safe before falling back to browse-mode shortcuts. Overlay and editor forms that already own their own full-screen key handling continue to do so.

- [x] A centralized global command layer handles `Ctrl+C` and function-key tab switching before tab-specific handlers, while printable Alt chords stay local to active text fields.
- [x] Text-entry modes such as Compose and Timeline search receive plain printable characters.
- [x] Logs act like an overlay: they can close with the advertised logs command or `Esc`, and scroll without leaking keys into the underlying draft.
- [x] Leaving a non-empty Compose draft through `F1/F3/F4` or `Alt+1/3/4` starts the existing draft persistence path.

## Verification

These checks define the minimum acceptance evidence for the implementation.

- [x] Unit tests cover Compose text insertion for `q` and digits, browse-mode `q` quit, F-key tab switching from Compose, printable Alt chord text safety, refresh text safety, and Timeline search text safety.
- [x] Focused Go tests pass with the requested key/navigation/compose/log/chat pattern.
- [x] A broader `go test ./internal/app -count=1` pass confirms no adjacent app behavior regressed.
- [x] Demo-mode tmux captures at `220x50`, `120x40`, and `80x24` show the Compose command layer and key hints behaving coherently.
