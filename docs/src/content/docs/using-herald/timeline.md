---
title: Timeline
description: Read, search, preview, reply to, forward, star, and copy messages in the Timeline tab.
---

Timeline is Herald's primary reading view. It shows chronological mail rows, thread grouping, search results, split or full-screen previews, quick replies, attachment saving, text selection, and actions that operate on the current message.

## Overview

Press `1` to open Timeline. Use it when you want to scan mail, switch folders, search across cached content, read a message, reply or forward, save attachments, star important threads, unsubscribe, or copy message text. You can drive the same flow with keys or by clicking rows and scrolling the list or preview with a mouse or trackpad.

## Screen Anatomy

| Area | What it shows |
| --- | --- |
| Folder sidebar | Folder tree, unread counts, total counts, expandable parents, and virtual `All Mail only` when available. |
| Timeline table | One row per visible thread or email. Columns are Sender, Subject, Date, Size KB, Att, and Tag. |
| Sender cell | Sender name plus unread indicator, star indicator, thread count, and child-row prefix for expanded threads. |
| Subject cell | Subject for the newest thread message or the individual email row. |
| Date and size | Message date and approximate message size in KB. |
| Att column | Attachment indicator when Herald detected attachments. |
| Tag column | AI classification/category when present. |
| Preview panel | Message header, body text, tags, unsubscribe/hide actions, attachments, inline image notes, loading/error state, and scroll position. |
| Full-screen preview | Same message content expanded across the terminal, with bounded inline images in iTerm2-compatible terminals and safe OSC 8 fallback links in local TUI sessions. |
| Search input/results | Search prompt, search mode, result count, and focused result rows. |
| Quick reply picker | A small choice list of canned and optional AI-generated replies. |

<!-- HERALD_SCREENSHOT id="timeline-main-list" page="timeline" alt="Timeline tab with inbox rows" state="demo mode, 120x40, sidebar visible" desc="Shows the main Timeline list, unread/star indicators, tag column, status bar, and key hints." capture="tmux demo 120x40; ./bin/herald --demo; press 1" -->

![Timeline tab with inbox rows](/screenshots/timeline-main-list.png)

## Controls

| Key | Context | Preconditions | Result |
| --- | --- | --- | --- |
| `enter` | Timeline row | A row is selected. | Opens the email preview. If the row is a collapsed thread, expands the thread first. |
| `enter` | Search results | Search result focus is active. | Opens the selected result. If preview focus is active, returns focus to results. |
| `j` / `down` | Timeline list | Timeline list is focused. | Moves down and updates preview if one is open. |
| `k` / `up` | Timeline list | Timeline list is focused. | Moves up and updates preview if one is open. |
| `j` / `down` | Preview/full-screen | Preview or full-screen is focused. | Scrolls body down, or extends visual selection. |
| `k` / `up` | Preview/full-screen | Preview or full-screen is focused. | Scrolls body up, or shrinks visual selection. |
| `/` | Timeline | Not loading and search is closed. | Opens Timeline search. |
| `ctrl+i` / `tab` | Search input | Search query is non-empty. | Runs server IMAP search instead of local search. |
| `esc` | Search/preview states | Search, preview, full-screen, quick reply, visual mode, or chat filter is active. | Unwinds the active state in the safest order. |
| `*` | Timeline row | Not read-only. | Toggles star on the current row email. |
| `R` | Timeline row | Not loading and not read-only. | Opens Compose as a reply to the selected email. |
| `F` | Timeline row | Not read-only. | Opens Compose as a forward of the selected email. |
| `D` | Timeline or selected message | Not read-only. | Starts delete confirmation for the current or selected target. |
| `e` | Timeline or selected message | Not read-only. | Starts archive confirmation for the current or selected target. |
| `A` | Timeline row | AI configured. | Re-classifies the current email. |
| `ctrl+q` | Timeline row | A current email exists. | Opens or closes the quick reply picker. |
| `z` | Preview | A selected email is open. | Toggles full-screen reading. |
| `s` | Preview | Preview has attachments. | Opens attachment save prompt with a default Downloads path. |
| `[` / `]` | Preview | Message has more than one attachment. | Selects previous or next attachment. |
| `u` | Preview | Body has `List-Unsubscribe` data and tab is not read-only. | Opens unsubscribe confirmation. |
| `h` / `H` | Preview | A selected email is open and tab is not read-only. | Creates a hide-future-mail rule for the sender. |
| `v` | Preview/full-screen | Body wrapped lines are available. | Toggles visual text selection. |
| `y` | Preview/full-screen | Visual selection is active. | Copies selected lines and exits visual mode. |
| `y` then `y` | Preview/full-screen | Body wrapped lines are available. | Copies the current visible body line. |
| `Y` | Preview/full-screen | Body wrapped lines are available. | Copies the full wrapped body. |
| `m` | Timeline | Any Timeline state. | Toggles mouse-selection mode for terminal copy behavior. |
| Click row | Timeline list | Terminal sends mouse events and a row is visible. | Selects the row and opens the split preview. |
| Wheel/trackpad scroll | Timeline list | Terminal sends mouse wheel events. | Moves through Timeline rows in small steps and refreshes the open preview. |
| Wheel/trackpad scroll | Preview/full-screen | Preview content is scrollable. | Scrolls the message body. |
| Click OSC 8 link | Preview/full-screen | Terminal supports OSC 8 hyperlinks. | Opens the linked URL through the terminal. |

## Workflows

### Read a Message

1. Press `1`.
2. Use `j`/`k` to select a row.
3. Press `enter`.
4. Read in the split preview. Press `z` for full-screen.
5. Press `esc` to leave full-screen or close the preview.

### Read with a Mouse

1. Click a Timeline row to select it and open the split preview.
2. Scroll over the Timeline list to move between messages, or scroll over the preview to read more body text.
3. Click readable email links such as `Display in your browser` when your terminal supports OSC 8 hyperlinks.
4. Press `m` if you want to temporarily hand the mouse back to the terminal for native text selection.

### Search Timeline

1. Press `/`.
2. Type a query. Herald debounces local search while you type.
3. Use prefixes when needed: `/b ` for body search, `/*` for cross-folder search, or `?` for semantic search.
4. Press `enter` to run or focus existing results.
5. Press `ctrl+i` or `tab` from the search input to run server IMAP search.
6. Press `esc` once to leave results or twice to clear search.

### Reply or Forward

1. Select an email row.
2. Press `R` to reply or `F` to forward.
3. Herald switches to Compose and prefills To/Subject/body context.
4. Finish the draft and send with `ctrl+s`.

### Use Quick Replies

1. Select or open an email.
2. Press `ctrl+q`.
3. Move with `j`/`k` or press number `1` through `8`.
4. Press `enter` to open the selected reply in Compose.
5. Press `esc` to cancel.

### Save an Attachment

1. Open an email with attachments.
2. Use `[` and `]` to choose the attachment when there are several.
3. Press `s`.
4. Edit the destination path if needed.
5. Press `enter` to save or `esc` to cancel.

## States

| State | What happens |
| --- | --- |
| Loading | Existing cached rows remain visible when possible; new IMAP work is reflected in the top sync strip and status bar. |
| Empty folder | Timeline shows an empty row area and key hints still expose refresh, sidebar, chat, and tabs when available. |
| Preview loading | Header/preview area opens while Herald fetches and MIME-parses body text and attachments. |
| Preview error | The preview reports a body fetch or parse failure without crashing the tab. |
| Read-only diagnostic | `All Mail only` disables destructive actions, server/body/cross/semantic search, and mailbox mutations. |
| AI unavailable | Tags may be absent; semantic search, AI replies, and re-classification show concise unavailable messages. |
| Attachment prompt | The save-path input captures keys until `enter` or `esc`. |
| Unsubscribe confirmation | Status asks for `y` confirm or `n`/`Esc` cancel before running the unsubscribe method. |
| Narrow terminal | Sidebar and chat may hide; preview and table widths shrink. Below the minimum size, the global size guard appears. |

## Data And Privacy

Timeline reads message metadata from SQLite and IMAP-backed cache. Opening a message fetches the full message body by UID, parses text/plain content, inline images, attachments, and unsubscribe headers, and can cache body text for later use. Marking read, starring, deleting, archiving, unsubscribing, hiding future mail, and attachment saving write to IMAP, SQLite, local files, or rules depending on the action.

Inline MIME image bytes are never written to disk for preview. In local TUI sessions that cannot render inline graphics, Herald serves the currently previewed images from a random localhost URL and exposes short OSC 8 links; those links are revoked when the preview changes. Remote HTML image URLs are shown as links only and are not fetched by Herald.

AI actions such as semantic search, classification, image descriptions, and quick replies send selected query or message context to the configured AI backend. Ollama is local by default; external providers receive the context required for the requested action.

## Troubleshooting

If search appears stuck, press `esc` to return from results to input, then press `esc` again to clear the search state.

If delete/archive/star does nothing, check whether you are in `All Mail only` read-only diagnostic mode.

If attachments do not save, verify the destination path is writable. The default path expands to `~/Downloads/<filename>`.

If quick replies only show canned choices, the AI backend is unavailable or still generating; the canned replies remain usable.

## Screenshot Placeholders

<!-- HERALD_SCREENSHOT id="timeline-split-preview" page="timeline" alt="Timeline split preview open" state="demo mode, 120x40, email preview open" desc="Shows selected row, preview header fields, body text, action hint line, attachment area if present, and split layout." capture="tmux demo 120x40; ./bin/herald --demo; press 1; press enter" -->

![Timeline split preview open](/screenshots/timeline-split-preview.png)

<!-- HERALD_SCREENSHOT id="timeline-search-results" page="timeline" alt="Timeline search results mode" state="demo mode, 120x40, local search active" desc="Shows search input, result rows, result focus behavior, and search-related status fragments." capture="tmux demo 120x40; ./bin/herald --demo; press 1; press /; type newsletter; press enter" -->

![Timeline search results mode](/screenshots/timeline-search-results.png)

<!-- HERALD_SCREENSHOT id="timeline-quick-reply-picker" page="timeline" alt="Quick reply picker on Timeline" state="demo mode, 120x40, quick reply picker visible" desc="Shows canned replies, optional AI replies, selection cursor, number shortcuts, and cancellation hint." capture="tmux demo 120x40; ./bin/herald --demo; press 1; press ctrl+q" -->

![Quick reply picker on Timeline](/screenshots/timeline-quick-reply-picker.png)

<!-- HERALD_SCREENSHOT id="timeline-full-screen-reader" page="timeline" alt="Full-screen email reader" state="demo mode, 120x40, full-screen preview" desc="Shows expanded body reading mode, scrollable wrapped body, action hints, and text selection affordances." capture="tmux demo 120x40; ./bin/herald --demo; press 1; press enter; press z" -->

![Full-screen email reader](/screenshots/timeline-full-screen-reader.png)

## Related Pages

- [Search](/features/search/)
- [Attachments](/features/attachments/)
- [Text Selection](/features/text-selection/)
- [Destructive Actions](/features/destructive-actions/)
- [Compose](/using-herald/compose/)
