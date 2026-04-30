---
title: Global UI
description: Understand Herald's shared layout, tabs, sidebar, status bar, overlays, and focus model.
---

Global UI covers the parts of Herald that stay consistent while you move between tabs. Learn this page first if the interface feels dense: it explains where status lives, how focus moves, and why some panels appear or disappear at smaller terminal sizes.

## Overview

Herald is a Bubble Tea terminal app with a persistent header, tab bar, optional folder sidebar, main content panels, optional chat panel, bottom status bar, context-sensitive key hints, and a `?` shortcut help overlay. Most browsing work happens in three tabs: Timeline, Cleanup, or Contacts. Compose is a transient writing screen launched from Timeline, and the common navigation surfaces work with both keys and mouse input.

## Screen Anatomy

| Area | What it shows | Notes |
| --- | --- | --- |
| Header | `Herald` while the main TUI is active. | The loading view shows a larger startup banner before cached data is visible. |
| Tab bar | `F1 Timeline`, `F2 Cleanup`, `F3 Contacts`. | The active tab is highlighted. `F1-F3` are the primary tab shortcuts; number keys remain browse-context aliases, `Alt+1/2/3` remain secondary aliases, and mouse clicks switch tabs when the terminal sends mouse events. |
| Top sync strip | Current startup or live sync phase. | Appears when Herald is loading while some visible data is already available. |
| Folder sidebar | IMAP folder tree with unread and total counts. | Visible mainly on Timeline and Cleanup when the terminal is wide enough. |
| Main panels | The active tab content. | Timeline and Cleanup can split into list/detail/preview layouts. |
| Chat panel | Right-side AI chat input and transcript. | Opens with `c` when AI is configured and the terminal is wide enough. |
| Status bar | Folder breadcrumb, AI chip, search or cleanup state, deletion progress, sync countdown, demo/dry-run/log flags. | Confirmation prompts temporarily replace normal status. |
| Key hints | The currently valid keys for the focused tab, panel, or overlay. | Hints wrap to at most two lines. |
| Shortcut help | A scrollable overlay opened with `?`. | Lists the fuller shortcut catalog for the current tab, pane, overlay, or Compose mode. |

<!-- HERALD_SCREENSHOT id="global-main-layout" page="global-ui" alt="Herald main layout with Timeline and sidebar" state="demo mode, 120x40, Timeline tab, sidebar visible" desc="Shows header, tab bar, folder sidebar, Timeline list, status bar, and key hints together." capture="tmux demo 120x40; ./bin/herald --demo; press F1" -->

![Herald main layout with Timeline and sidebar](/screenshots/global-main-layout.png)

## Controls

| Key | Context | Preconditions | Result |
| --- | --- | --- | --- |
| `F1` / `F2` / `F3` | Main TUI | Visible data can be interacted with. | Switches to Timeline, Cleanup, or Contacts from any main tab or Compose screen. |
| `1` / `2` / `3` | Browse contexts | Visible data can be interacted with and no text field owns the keys. | Switches tabs as compatibility aliases. In the quick reply picker, chooses replies 1-3. |
| `alt+1` / `alt+2` / `alt+3` | Main TUI | Visible data can be interacted with. | Switches tabs as secondary aliases, including when Compose text fields are focused. |
| `q` | Browse contexts | No text input is being edited. | Quits Herald after cleanup. |
| `ctrl+c` | Global | Any state. | Quits Herald after cleanup, including from text inputs and overlays. |
| `tab` / `ctrl+i` | Most tabs | Visible data can be interacted with. | Cycles focus forward through visible panels. |
| `shift+tab` | Most tabs | Visible data can be interacted with. | Cycles focus backward through visible panels. |
| `f` / `alt+f` | Timeline/Cleanup | Visible data can be interacted with. | Toggles the folder sidebar when that tab can render it; use `alt+f` while composing. |
| `c` / `alt+c` | Main UI | Not loading and width allows the chat panel. | Toggles AI chat and focuses its input; use `alt+c` while composing. |
| `l` / `L` / `alt+l` | Main UI | Visible data can be interacted with. | Toggles the log viewer overlay; use `alt+l` while composing. |
| `r` / `alt+r` | Main UI | Not loading. | Refreshes the current folder and clears Timeline chat filters; use `alt+r` while composing. |
| `S` | Main UI | Settings overlay is not already open. | Opens settings as a full-screen panel. |
| `a` | Main UI | AI classifier is configured and work is available. | Starts folder classification. |
| `?` | Main UI and Herald-owned overlays | Visible data can be interacted with. | Opens context-sensitive shortcut help; pressing `?`, `esc`, or `q` closes it. |
| `esc` | Main UI and overlays | A transient state is active. | Closes the most specific state first, such as quick reply, visual mode, full-screen preview, cleanup preview, chat filter, Timeline preview, search, Compose AI panel, Compose status message, or the Compose screen itself. |

## Mouse Controls

Mouse controls are convenience shortcuts over the same model as keyboard focus and selection. They are useful for trackpad-heavy terminal sessions, SSH clients that forward mouse events, and browser terminal sessions such as `ttyd`.

| Mouse action | Context | Result |
| --- | --- | --- |
| Click a top tab | Main UI | Switches to Timeline, Cleanup, or Contacts. |
| Click a folder/sidebar row | Timeline or Cleanup with sidebar visible | Selects the folder and loads it. |
| Click a Timeline row | Timeline table | Selects the row and opens the split preview. |
| Scroll over Timeline rows | Timeline table | Moves the Timeline cursor by small steps and refreshes the open preview. |
| Scroll over an email preview | Timeline or Cleanup preview | Scrolls the message body. |
| Click a Cleanup summary row | Cleanup summary | Selects the sender or domain and refreshes detail rows. |
| Click a Cleanup detail row | Cleanup detail table | Opens that message in the Cleanup preview. |
| Click an OSC 8 email link | Terminal-supported email preview links | Opens the original URL through the terminal. |

Press `m` in Timeline to temporarily release Herald's mouse capture for terminal-native text selection. Press `m` again to restore Herald's clickable and scrollable navigation.

![Mouse navigation and clickable email links in Herald](/screenshots/mouse-navigation-links.png)

## Workflows

### Move Between Tabs

1. Press `F1`, `F2`, or `F3`.
2. Watch the tab bar highlight move.
3. Use the bottom key hints to learn the active tab's controls.

Browse contexts also accept `1`, `2`, and `3` as compatibility aliases. When a terminal does not send function keys cleanly, `Alt+1`, `Alt+2`, and `Alt+3` remain secondary aliases that keep Compose text entry safe.

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

### Open Shortcut Help

1. Press `?`.
2. Scroll with `j`/`k`, arrow keys, page keys, or the mouse wheel when the overlay is taller than the terminal.
3. Press `?`, `esc`, or `q` to return to the same tab, panel, or overlay.

## States

| State | What you see | What to do |
| --- | --- | --- |
| Startup loading | A loading banner, progress text, optional progress bar, elapsed time, and `q` quit hint. | Wait for sync or press `q`. |
| Visible-data loading | Existing cached rows remain visible and a top sync strip explains current IMAP work. | Continue reading cached data while sync completes. |
| Minimum terminal | A size guard replaces the normal UI below roughly `60x15`. | Resize the terminal. |
| Sidebar auto-hidden | Status includes a sidebar hidden notice. | Widen the terminal or press `f` when the tab supports the sidebar. |
| Chat unavailable at size | Status says chat is hidden at this size. | Widen the terminal before pressing `c` again. |
| AI unavailable | AI chip reads off/down or AI actions show a concise error. | Configure AI or continue using non-AI mail features. |
| Logs overlay | Log viewer is on top of the current tab and status includes `Logs ON`. | Press `l` or `Alt+L` to close. |
| Shortcut help | A scrollable command reference is on top of the current tab or overlay. | Press `?`, `esc`, or `q` to close it. |
| Confirmation | Status bar asks for `y` confirm or `n`/`Esc` cancel. | Confirm only if the described action matches your intent. |

## Data And Privacy

The global UI reads cached message metadata, folder counts, sync state, deletion progress, AI scheduler state, and logs generated by the current Herald process. Opening chat can send a compact mailbox context to the configured AI provider. Opening settings can read and write config fields, credentials, tokens, and provider keys.

## Troubleshooting

If a key seems to do nothing, press `?` to open shortcut help or check the bottom key hints and focused panel. Many keys are context-sensitive: for example, `space` expands a folder when the sidebar is focused but selects a cleanup row when Cleanup is focused.

If a panel disappeared, check terminal width. Herald hides the sidebar or refuses to open chat when there is not enough room to render the remaining mail view.

If a prompt will not close, press `esc` first. If that does not apply, `ctrl+c` still quits globally. Plain `q` quits from browse contexts, but it is normal text inside Compose and search inputs.

## Screenshot Placeholders

<!-- HERALD_SCREENSHOT id="global-chat-open" page="global-ui" alt="Chat panel open beside Timeline" state="demo mode, 120x40, Timeline tab, chat visible" desc="Shows the right-side chat panel, chat input, active focus, compressed Timeline width, status bar, and chat key hints." capture="tmux demo 120x40; ./bin/herald --demo; press 1; press c" -->

![Chat panel open beside Timeline](/screenshots/global-chat-open.png)

<!-- HERALD_SCREENSHOT id="global-logs-overlay" page="global-ui" alt="Log viewer overlay open" state="demo mode, 120x40, logs overlay visible" desc="Shows the real-time log viewer overlay, log levels, scrollable history area, and close hints." capture="tmux demo 120x40; ./bin/herald --demo; press l" -->

![Log viewer overlay open](/screenshots/global-logs-overlay.png)

<!-- HERALD_SCREENSHOT id="global-narrow-terminal" page="global-ui" alt="Narrow terminal size guard" state="demo mode, 50x15" desc="Shows the narrow terminal fallback or compressed layout used for minimum-size testing." capture="tmux demo 50x15; ./bin/herald --demo" -->

![Narrow terminal size guard](/screenshots/global-narrow-terminal.png)

## Related Pages

- [All Keybindings](/reference/keybindings/)
- [Sync and Status](/features/sync-status/)
- [Settings](/features/settings/)
- [Chat Panel](/features/chat/)
