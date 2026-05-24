---
title: All Keybindings
description: Complete Herald keyboard reference by global scope, tab, panel, and overlay.
---

This reference lists user-facing keys backed by Herald's current key handlers and key hints. When a key appears in multiple sections, the most specific focused overlay or panel wins.

## Global

| Key | Result |
| --- | --- |
| `q` | Quit Herald from browse contexts. In Compose and search inputs, plain `q` is text. |
| `ctrl+c` | Quit Herald from any state, including text inputs and overlays. |
| `1` | Switch to Timeline in browse contexts, or choose quick reply 1 when quick reply picker is open. |
| `2` | Switch/load Contacts in browse contexts, or choose quick reply 2 when quick reply picker is open. |
| `3` | Choose quick reply 3 when quick reply picker is open. It is not a top-level tab shortcut. |
| `F1` / `F2` / `F3` | Aliases for Timeline / Contacts / Contacts legacy. |
| `tab` / `ctrl+i` | Cycle focus forward, except in search where it can run server search. |
| `shift+tab` | Cycle focus backward where supported. |
| `h` / `j` / `k` / `l` | Navigate left / down / up / right where the active pane supports it. |
| `B` | Toggle folder sidebar. |
| `g` | Toggle the AI chat panel outside text-entry fields. |
| `L` | Toggle log viewer. |
| `ctrl+r` | Refresh the current folder. |
| `S` | Open settings. |
| `?` | Open context-sensitive shortcut help in browse and non-text contexts. When help is open, `/` searches help and `?`, `esc`, or `q` closes it. |
| `esc` | Close or unwind the active transient state. |

## Mouse

These actions work when the terminal sends mouse events to Herald. OSC 8 link clicks are handled by the terminal, so compatible terminals open the real URL while Herald keeps the preview text readable.

| Mouse action | Result |
| --- | --- |
| Click top tab | Switch to Timeline or Contacts. |
| Click folder/sidebar row | Select and load that folder. |
| Click Timeline row | Select the message or thread and open the preview. |
| Scroll Timeline rows | Move the Timeline cursor by small steps. |
| Scroll Timeline preview | Scroll the message body. |
| Click OSC 8 email link | Open the target URL through the terminal. |
| Press `m` in Timeline | Release or restore Herald mouse capture for terminal-native text selection. |

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
| `space` | Toggle Timeline message or thread selection. |
| `G` | Cycle Timeline grouping between thread, sender, and domain views. |
| `shift+up` / `shift+down` | Extend Timeline selection range when supported by the terminal; plain movement finishes the range and keeps selection. |
| `V`, then `j` / `k` | Use fallback Timeline range selection without shifted-arrow support; press `V` or `esc` when done. |
| `/` | Open Timeline search. |
| `c` | Open a blank Compose screen for a new message. |
| `*` | Toggle star on current email. |
| `r` | Reply all to current email in Compose. |
| `R` | Reply sender-only to current email in Compose. |
| `f` | Forward current email in Compose. |
| `d` / `backspace` | Delete current/selected target after confirmation. |
| `D` / `shift+backspace` | Delete current/selected target immediately, without confirmation. |
| `a` | Archive the current message immediately; bulk archive still confirms. |
| `T` | Re-classify current email with AI; `A` remains a legacy alias. |
| `ctrl+d` / `ctrl+u` | Scroll half a page down / up in scrollable list or preview contexts. |
| `ctrl+q` | Open quick reply picker. |
| `z` | Toggle full-screen reader when preview is open. |
| `s` | Save selected attachment from preview. |
| `[` | Select previous attachment. |
| `]` | Select next attachment. |
| `u` | Unsubscribe when preview body includes unsubscribe data. |
| `H` | Hide future mail from current sender. |
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
| `?` prefix after `/` | Semantic search when AI/embeddings are available. |
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
| Plain text and punctuation | Insert text into the focused Compose field, including literal `?`, `/`, and macOS Option-generated characters. |
| `tab` | Move through To, CC, BCC, Subject, and Body; accept subject hint when visible. |
| `ctrl+s` | Send message. |
| `ctrl+p` | Toggle Markdown preview. |
| `ctrl+a` | Open outgoing attachment path prompt. |
| `ctrl+k` | Focus the inline Compose AI prompt. |
| `ctrl+j` | Generate AI subject suggestion. |
| `ctrl+enter` | Accept AI response into body. |
| `esc` | Dismiss subject hint, AI panel, or compose status; then return to the screen that opened Compose. |
| `up` / `down` | Move autocomplete selection when suggestions are visible. |
| `enter` / `tab` | Accept autocomplete suggestion when visible. |
| `esc` | Dismiss autocomplete or attachment prompt when active. |

## Cleanup Via Timeline

| Key | Result |
| --- | --- |
| `G` | Cycle Timeline into sender or domain grouping. |
| `space` | Select the highlighted Timeline group or message. |
| `d` / `backspace` | Delete highlighted/selected mail after confirmation. |
| `D` / `shift+backspace` | Delete highlighted/selected mail immediately, without confirmation. |
| `a` / `e` | Archive highlighted/selected mail. |
| `S` then `Sync & Cleanup` | Open automation-rule, custom-prompt, and cleanup-rule managers. |

## Contacts

| Key | Result |
| --- | --- |
| `/` | Start keyword contact search. |
| `?` | Open context-sensitive shortcut help. |
| `?` prefix after `/` | Run semantic contact search when AI/embeddings are available. |
| Printable text | Add characters to active contact search. |
| `backspace` / `ctrl+h` | Delete a search character. |
| `enter` | Confirm search, open contact detail, or open recent email preview depending on focus. |
| `esc` | Clear search, close inline preview, or return from detail. |
| `tab` | Toggle list/detail focus when detail is open. |
| `j` / `down` | Move down contact list or recent emails. |
| `k` / `up` | Move up contact list or recent emails. |
| `e` | Enrich selected contact. |

## Keyboard Profiles

Herald resolves browse shortcuts through the active keyboard profile. Text-entry surfaces keep printable text literal in the default profile; Vim and Custom profiles can use the modal Compose field adapter.

```yaml
keyboard:
  profile: default # default | vim | emacs | custom
  custom_keymap: ~/.config/herald/keymaps/work.yaml
```

Custom keymap files extend a built-in profile and bind keys to predefined command IDs:

```yaml
extends: default
bindings:
  timeline:
    normal:
      x: compose.new
      a: mail.archive_current
      d: mail.delete_confirm
      D: mail.delete_immediate
fields:
  compose:
    default_mode: normal # insert | normal | visual
```

## Overlays

| Overlay | Keys |
| --- | --- |
| Delete/archive confirmation | `y`/`Y` confirm, `n`/`N`/`esc` cancel. |
| Unsubscribe confirmation | `y`/`Y` confirm, `n`/`N`/`esc` cancel. |
| Attachment save prompt | `enter` save, `esc` cancel, text edits path. |
| Logs | `L` or `esc` close, `j`/`k` or arrows scroll, `q` quit. |
| Chat | `enter` send, `esc` or `tab` close/leave chat, `q` quit. |
| Shortcut help | `/` search, `j`/`k`, arrows, page keys, `home`/`end`, or mouse wheel scroll; `?`/`esc`/`q` close. |
| Rule editor | Launched from Settings Sync & Cleanup; form navigation, `esc` cancel. |
| Prompt editor | Launched from Settings Sync & Cleanup; form navigation, `esc` cancel. |
| Cleanup manager | Launched from Settings Sync & Cleanup; `n` new, `enter` edit, `d`/`D` delete, `r` run all, `j`/`k` move, `esc` close or cancel edit. |
| Settings | `enter` opens categories or accepts form controls, `/` filters the menu, `tab` moves fields, and `esc` unwinds filter/category state before closing. |
| OAuth wait | `enter` opens browser to authorization URL. |

## Related Pages

- [Global UI](/using-herald/global-ui/)
- [Timeline](/using-herald/timeline/)
- [Compose](/using-herald/compose/)
- [Cleanup via Timeline](/using-herald/cleanup/)
- [Contacts](/using-herald/contacts/)
