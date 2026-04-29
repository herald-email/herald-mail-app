# Context-Sensitive Shortcut Help Overlay

## Goal
This feature gives users an in-app rescue path when a key does something surprising. Plain `?` opens a lazygit-style shortcut overlay for the current Herald context, while semantic search moves behind the existing search input as `/` followed by a `? query` prefix.

- [x] Pressing `?` opens help from Timeline, Compose, Cleanup, Contacts, chat, logs, confirmations, and preview states that Herald owns.
- [x] The bottom hint bar advertises `?: help` without overflowing at `80x24`.
- [x] Timeline and Contacts semantic search remain reachable through `/` with a leading `? query`.

## Shortcut Catalog
The catalog is owned by `internal/app` because the Bubble Tea model already owns focus normalization and key routing. Entries should be generated from the current visible context instead of copied from README-style static docs.

- [x] Global shortcuts include tab switching, quit, refresh, sidebar, chat, logs, and help close/open behavior.
- [x] Context shortcuts include the active tab and focused pane, plus overlays such as logs, chat, delete/subscribe confirmation, search, quick replies, and previews.
- [x] Compose help explains current field navigation, send/preview/attach/AI commands, and preservation mode when reply/forward context exists.
- [x] Help content avoids destructive ambiguity by naming delete/archive as actions that may ask for confirmation.

## Interaction
The overlay behaves like a read-only modal over the current screen. It must not mutate the underlying tab, pane, draft, search query, selected message, or confirmation state when opened or closed.

- [x] `Esc`, `?`, and `q` close help.
- [x] `j/k`, `up/down`, `pgup/pgdown`, `home/end`, and mouse wheel where available scroll help content.
- [x] Reopening help starts at the top and recomputes content from the latest visible context.
- [x] At minimum terminal size, the normal size guard remains in charge instead of trying to draw help.

## Verification
Acceptance is split between focused Go tests and tmux visual evidence because this is both key-routing behavior and terminal layout. Demo mode is sufficient for visual checks because shortcut help is local TUI state.

- [x] Unit tests cover `?` routing, close keys, context content, footer hints, and semantic-search migration.
- [x] Tmux captures show before/after behavior at `220x50` and `80x24`.
- [x] MCP has no behavior change beyond build/read smoke after TUI-affecting work.
