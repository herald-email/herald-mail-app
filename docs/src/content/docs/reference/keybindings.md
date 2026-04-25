---
title: All Keybindings
description: Complete Herald keyboard reference by global scope, tab, panel, and overlay.
---

This reference lists user-facing keys backed by Herald's current key handlers and key hints. When a key appears in multiple sections, the most specific focused overlay or panel wins.

## Global

| Key | Result |
| --- | --- |
| `q` | Quit Herald. |
| `ctrl+c` | Quit Herald from any state, including text inputs and overlays. |
| `1` | Switch to Timeline, or choose quick reply 1 when quick reply picker is open. |
| `2` | Switch to Compose, or choose quick reply 2 when quick reply picker is open. |
| `3` | Switch to Cleanup, or choose quick reply 3 when quick reply picker is open. |
| `4` | Switch/load Contacts, or choose quick reply 4 when quick reply picker is open. |
| `tab` / `ctrl+i` | Cycle focus forward, except in search where it can run server search. |
| `shift+tab` | Cycle focus backward where supported. |
| `f` | Toggle folder sidebar on tabs that support it. |
| `c` | Toggle chat panel. |
| `l` / `L` | Toggle log viewer. |
| `r` | Refresh the current folder. |
| `S` | Open settings. |
| `a` | Start AI classification for the current folder. |
| `esc` | Close or unwind the active transient state. |

## Sidebar

| Key | Result |
| --- | --- |
| `j` / `down` | Move down folder tree. |
| `k` / `up` | Move up folder tree. |
| `space` | Expand or collapse a folder tree node. |
| `enter` | Select folder or toggle a synthetic parent node. |

## Timeline

| Key | Result |
| --- | --- |
| `enter` | Open selected email, expand a collapsed thread, or open selected search result. |
| `j` / `down` | Move down list, scroll preview, or move quick reply selection depending on focus. |
| `k` / `up` | Move up list, scroll preview, or move quick reply selection depending on focus. |
| `/` | Open Timeline search. |
| `*` | Toggle star on current email. |
| `R` | Reply to current email in Compose. |
| `F` | Forward current email in Compose. |
| `D` | Delete current/selected target after confirmation. |
| `e` | Archive current/selected target after confirmation. |
| `A` | Re-classify current email with AI. |
| `ctrl+q` | Open quick reply picker. |
| `z` | Toggle full-screen reader when preview is open. |
| `s` | Save selected attachment from preview. |
| `[` | Select previous attachment. |
| `]` | Select next attachment. |
| `u` | Unsubscribe when preview body includes unsubscribe data. |
| `h` / `H` | Hide future mail from current sender. |
| `v` | Toggle visual text selection in preview/full-screen. |
| `y` | Start `yy` line copy or copy visual selection. |
| `Y` | Copy full wrapped body. |
| `m` | Toggle mouse-selection mode. |

## Timeline Search

| Key or prefix | Result |
| --- | --- |
| Plain query | Local search while typing. |
| `/b ` | Body/FTS search over cached bodies. |
| `/*` | Cross-folder cached search. |
| `?` prefix | Semantic search when AI/embeddings are available. |
| `enter` | Run search or focus existing results. |
| `tab` / `ctrl+i` | Run server IMAP search from search input. |
| `esc` | Close preview, leave results, or clear search depending on current search state. |

## Quick Reply Picker

| Key | Result |
| --- | --- |
| `j` / `down` | Move to next reply. |
| `k` / `up` | Move to previous reply. |
| `enter` | Open selected reply in Compose. |
| `1` through `8` | Choose reply by number. |
| `esc` | Close picker. |

## Compose

| Key | Result |
| --- | --- |
| `tab` | Move through To, CC, BCC, Subject, and Body; accept subject hint when visible. |
| `ctrl+s` | Send message. |
| `ctrl+p` | Toggle Markdown preview. |
| `ctrl+a` | Open outgoing attachment path prompt. |
| `ctrl+g` | Toggle Compose AI assistant panel. |
| `ctrl+j` | Generate AI subject suggestion. |
| `ctrl+enter` | Accept AI response into body. |
| `esc` | Dismiss subject hint, AI panel, or compose status. |
| `up` / `down` | Move autocomplete selection when suggestions are visible. |
| `enter` / `tab` | Accept autocomplete suggestion when visible. |
| `esc` | Dismiss autocomplete or attachment prompt when active. |

## Cleanup

| Key | Result |
| --- | --- |
| `d` | Toggle sender/domain grouping. |
| `space` | Select summary row or detail message. |
| `enter` | Load details from summary, open preview from details, or scroll preview. |
| `j` / `down` | Move rows or scroll preview. |
| `k` / `up` | Move rows or scroll preview. |
| `D` | Delete selected/current target. |
| `e` | Archive selected/current target. |
| `A` | Re-classify preview email. |
| `u` | Unsubscribe when preview body supports it. |
| `h` / `H` | Hide future mail for focused sender. |
| `W` | Open automation rule editor. |
| `P` | Open custom prompt editor. |
| `C` | Open cleanup manager. |
| `z` | Toggle full-screen cleanup preview. |
| `esc` | Close preview/full-screen/overlay. |

## Contacts

| Key | Result |
| --- | --- |
| `/` | Start keyword contact search. |
| `?` | Start semantic contact search. |
| Printable text | Add characters to active contact search. |
| `backspace` / `ctrl+h` | Delete a search character. |
| `enter` | Confirm search, open contact detail, or open recent email preview depending on focus. |
| `esc` | Clear search, close inline preview, or return from detail. |
| `tab` | Toggle list/detail focus when detail is open. |
| `j` / `down` | Move down contact list or recent emails. |
| `k` / `up` | Move up contact list or recent emails. |
| `e` | Enrich selected contact. |

## Overlays

| Overlay | Keys |
| --- | --- |
| Delete/archive confirmation | `y`/`Y` confirm, `n`/`N`/`esc` cancel. |
| Unsubscribe confirmation | `y`/`Y` confirm, `n`/`N`/`esc` cancel. |
| Attachment save prompt | `enter` save, `esc` cancel, text edits path. |
| Logs | `l` close, `j`/`k` or arrows scroll, `q` quit. |
| Chat | `enter` send, `esc` or `tab` close/leave chat, `q` quit. |
| Rule editor | Form navigation, `esc` cancel. |
| Prompt editor | Form navigation, `esc` cancel. |
| Cleanup manager | `n` new, `enter` edit, `d`/`D` delete, `r` run all, `j`/`k` move, `esc` close or cancel edit. |
| Settings | Form navigation, `enter` accept/save, `esc` cancel where supported. |
| OAuth wait | `enter` opens browser to authorization URL. |

## Related Pages

- [Global UI](/using-herald/global-ui/)
- [Timeline](/using-herald/timeline/)
- [Compose](/using-herald/compose/)
- [Cleanup](/using-herald/cleanup/)
- [Contacts](/using-herald/contacts/)
