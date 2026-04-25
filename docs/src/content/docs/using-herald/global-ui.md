---
title: Global UI
description: Understand Herald's shared layout, tabs, sidebar, status bar, overlays, and focus model.
---

Global UI covers the parts of Herald that stay consistent while you move between tabs. Learn this page first if the interface feels dense: it explains where status lives, how focus moves, and why some panels appear or disappear at smaller terminal sizes.

## Overview

Herald is a Bubble Tea terminal app with a persistent header, tab bar, optional folder sidebar, main content panels, optional chat panel, bottom status bar, and context-sensitive key hints. Most work happens in one of four tabs: Timeline, Compose, Cleanup, or Contacts.

## Screen Anatomy

| Area | What it shows | Notes |
| --- | --- | --- |
| Header | `Herald` while the main TUI is active. | The loading view shows a larger startup banner before cached data is visible. |
| Tab bar | `1 Timeline`, `2 Compose`, `3 Cleanup`, `4 Contacts`. | The active tab is highlighted. Number keys switch tabs when the current overlay allows it. |
| Top sync strip | Current startup or live sync phase. | Appears when Herald is loading while some visible data is already available. |
| Folder sidebar | IMAP folder tree with unread and total counts. | Visible mainly on Timeline and Cleanup when the terminal is wide enough. |
| Main panels | The active tab content. | Timeline and Cleanup can split into list/detail/preview layouts. |
| Chat panel | Right-side AI chat input and transcript. | Opens with `c` when AI is configured and the terminal is wide enough. |
| Status bar | Folder breadcrumb, AI chip, search or cleanup state, deletion progress, sync countdown, demo/dry-run/log flags. | Confirmation prompts temporarily replace normal status. |
| Key hints | The currently valid keys for the focused tab, panel, or overlay. | Hints wrap to at most two lines. |

<!-- HERALD_SCREENSHOT id="global-main-layout" page="global-ui" alt="Herald main layout with Timeline and sidebar" state="demo mode, 120x40, Timeline tab, sidebar visible" desc="Shows header, tab bar, folder sidebar, Timeline list, status bar, and key hints together." capture="tmux demo 120x40; ./bin/herald --demo; press 1" -->

## Controls

| Key | Context | Preconditions | Result |
| --- | --- | --- | --- |
| `1` | Global | Visible data can be interacted with. | Switches to Timeline. In the quick reply picker, chooses reply 1. |
| `2` | Global | Visible data can be interacted with. | Switches to Compose. In the quick reply picker, chooses reply 2. |
| `3` | Global | Visible data can be interacted with. | Switches to Cleanup. In the quick reply picker, chooses reply 3. |
| `4` | Global | Visible data can be interacted with. | Switches to Contacts or loads Contacts if already selected. In the quick reply picker, chooses reply 4. |
| `q` | Global | Any state. | Quits Herald after cleanup. |
| `ctrl+c` | Global | Any state. | Quits Herald after cleanup, including from text inputs and overlays. |
| `tab` / `ctrl+i` | Most tabs | Visible data can be interacted with. | Cycles focus forward through visible panels. |
| `shift+tab` | Most tabs | Visible data can be interacted with. | Cycles focus backward through visible panels. |
| `f` | Timeline/Cleanup | Visible data can be interacted with. | Toggles the folder sidebar when that tab can render it. |
| `c` | Main UI | Not loading and width allows the chat panel. | Toggles AI chat and focuses its input. |
| `l` / `L` | Main UI | Visible data can be interacted with. | Toggles the log viewer overlay. |
| `r` | Main UI | Not loading. | Refreshes the current folder and clears Timeline chat filters. |
| `S` | Main UI | Settings overlay is not already open. | Opens settings as a full-screen panel. |
| `a` | Main UI | AI classifier is configured and work is available. | Starts folder classification. |
| `esc` | Main UI and overlays | A transient state is active. | Closes the most specific state first, such as quick reply, visual mode, full-screen preview, cleanup preview, chat filter, Timeline preview, search, Compose AI panel, or status message. |

## Workflows

### Move Between Tabs

1. Press `1`, `2`, `3`, or `4`.
2. Watch the tab bar highlight move.
3. Use the bottom key hints to learn the active tab's controls.

### Use Panel Focus

1. Open a screen with multiple panels, such as Timeline with the sidebar visible.
2. Press `tab` to move focus to the next panel.
3. Press `shift+tab` to move focus back.
4. Use `j`/`k` or arrow keys; navigation applies to the focused panel.

### Open Chat

1. Press `c` from a wide terminal.
2. Type a mailbox question.
3. Press `enter` to send.
4. Press `esc` or `tab` to close or leave chat focus.

### Inspect Logs

1. Press `l`.
2. Scroll with `j`/`k` or arrow keys.
3. Press `l` again to close.

## States

| State | What you see | What to do |
| --- | --- | --- |
| Startup loading | A loading banner, progress text, optional progress bar, elapsed time, and `q` quit hint. | Wait for sync or press `q`. |
| Visible-data loading | Existing cached rows remain visible and a top sync strip explains current IMAP work. | Continue reading cached data while sync completes. |
| Minimum terminal | A size guard replaces the normal UI below roughly `60x15`. | Resize the terminal. |
| Sidebar auto-hidden | Status includes a sidebar hidden notice. | Widen the terminal or press `f` when the tab supports the sidebar. |
| Chat unavailable at size | Status says chat is hidden at this size. | Widen the terminal before pressing `c` again. |
| AI unavailable | AI chip reads off/down or AI actions show a concise error. | Configure AI or continue using non-AI mail features. |
| Logs overlay | Log viewer is on top of the current tab and status includes `Logs ON`. | Press `l` to close. |
| Confirmation | Status bar asks for `y` confirm or `n`/`Esc` cancel. | Confirm only if the described action matches your intent. |

## Data And Privacy

The global UI reads cached message metadata, folder counts, sync state, deletion progress, AI scheduler state, and logs generated by the current Herald process. Opening chat can send a compact mailbox context to the configured AI provider. Opening settings can read and write config fields, credentials, tokens, and provider keys.

## Troubleshooting

If a key seems to do nothing, check the bottom key hints and focused panel. Many keys are context-sensitive: for example, `space` expands a folder when the sidebar is focused but selects a cleanup row when Cleanup is focused.

If a panel disappeared, check terminal width. Herald hides the sidebar or refuses to open chat when there is not enough room to render the remaining mail view.

If a prompt will not close, press `esc` first. If that does not apply, `q` or `ctrl+c` still quits globally.

## Screenshot Placeholders

<!-- HERALD_SCREENSHOT id="global-chat-open" page="global-ui" alt="Chat panel open beside Timeline" state="demo mode, 120x40, Timeline tab, chat visible" desc="Shows the right-side chat panel, chat input, active focus, compressed Timeline width, status bar, and chat key hints." capture="tmux demo 120x40; ./bin/herald --demo; press 1; press c" -->

<!-- HERALD_SCREENSHOT id="global-logs-overlay" page="global-ui" alt="Log viewer overlay open" state="demo mode, 120x40, logs overlay visible" desc="Shows the real-time log viewer overlay, log levels, scrollable history area, and close hints." capture="tmux demo 120x40; ./bin/herald --demo; press l" -->

<!-- HERALD_SCREENSHOT id="global-narrow-terminal" page="global-ui" alt="Narrow terminal size guard" state="demo mode, 50x15" desc="Shows the narrow terminal fallback or compressed layout used for minimum-size testing." capture="tmux demo 50x15; ./bin/herald --demo" -->

## Related Pages

- [All Keybindings](/reference/keybindings/)
- [Sync and Status](/features/sync-status/)
- [Settings](/features/settings/)
- [Chat Panel](/features/chat/)
