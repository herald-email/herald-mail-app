---
title: Cleanup
description: Bulk review, delete, archive, unsubscribe, hide, and automate sender or domain mail cleanup.
---

Cleanup is Herald's high-leverage inbox reduction screen. It groups mail by sender or domain, shows detail rows, previews individual messages, and exposes destructive actions, archive, unsubscribe, hide-future-mail, automation rules, custom prompts, and cleanup rule management.

## Overview

Press `3` to open Cleanup. Use it when you want to answer questions like "which senders have the most mail?", "what can I archive?", "what can I delete?", and "which future mail should be hidden or automated?"

## Screen Anatomy

| Area | What it shows |
| --- | --- |
| Folder sidebar | Folder tree and counts when the terminal is wide enough and no cleanup preview is hiding it. |
| Summary table | Group rows by sender or domain with columns for selection, Sender/Domain, Count, and Dates. |
| Detail table | Emails for the focused summary row with columns for selection, Date, Subject, Size, and Att. |
| Preview panel | Current cleanup email body, header metadata, action hints, unsubscribe visibility, scroll state, delete/archive progress. |
| Full-screen preview | Cleanup preview expanded across the terminal. |
| Rule editor overlay | Full-screen `Automation Rule` form opened by `W`. |
| Custom prompt overlay | Full-screen prompt editor opened by `P`. |
| Cleanup manager overlay | Full-screen saved cleanup rules manager opened by `C`. |
| Status bar | Sender/domain mode, selection counts, deletion/archive progress, dry-run mode, folder counts, and confirmations. |

<!-- HERALD_SCREENSHOT id="cleanup-main-summary" page="cleanup" alt="Cleanup tab sender summary and details" state="demo mode, 120x40, Cleanup tab, sender mode" desc="Shows sender summary rows, detail rows, selection column, sender mode status, and cleanup key hints." capture="tmux demo 120x40; ./bin/herald --demo; press 3" -->

![Cleanup tab sender summary and details](/screenshots/cleanup-main-summary.png)

## Controls

| Key | Context | Preconditions | Result |
| --- | --- | --- | --- |
| `d` | Cleanup | Not loading. | Toggles sender grouping and domain grouping. |
| `space` | Summary or details | Visible data can be interacted with. | Selects or unselects the focused sender/domain row or message row. |
| `enter` | Summary | Summary focused. | Loads detail table for the focused sender/domain. |
| `enter` | Details | Details focused and preview closed. | Opens preview for the focused message. |
| `enter` | Preview | Cleanup preview open and details focused. | Scrolls preview down one line. |
| `j` / `down` | Summary/details | Preview not intercepting scroll. | Moves selection down. |
| `k` / `up` | Summary/details | Preview not intercepting scroll. | Moves selection up. |
| `j` / `down` | Preview | Cleanup preview open and details focused. | Scrolls body down. |
| `k` / `up` | Preview | Cleanup preview open and details focused. | Scrolls body up. |
| `D` | Cleanup | Not loading, not already deleting, target exists. | Opens delete confirmation or directly queues current preview email. |
| `e` | Cleanup | Not loading, not already deleting, target exists. | Opens archive confirmation or directly queues current preview email. |
| `A` | Cleanup preview | AI configured and preview email exists. | Re-classifies the preview email. |
| `u` | Cleanup preview | Body includes `List-Unsubscribe`. | Opens unsubscribe confirmation. |
| `h` / `H` | Cleanup | Focused sender or preview email exists. | Creates hide-future-mail behavior for that sender. |
| `W` | Cleanup | Rule editor closed. | Opens automation rule editor, prefilled with focused sender or domain. |
| `P` | Main UI | Rule/prompt/settings overlays are closed. | Opens custom AI prompt editor. |
| `C` | Cleanup | Cleanup manager closed. | Opens saved cleanup rules manager. |
| `z` | Cleanup preview | Preview open. | Toggles full-screen cleanup reader. |
| `esc` | Cleanup preview/overlays | Preview, full-screen, or overlay active. | Closes the active state. |
| `tab` / `shift+tab` | Cleanup | Visible panels available. | Cycles between sidebar, summary, details, and chat when present. |

## Workflows

### Review a Sender

1. Press `3`.
2. Keep sender mode or press `d` for domain mode.
3. Move through the summary table with `j`/`k`.
4. Press `enter` to load details.
5. Press `tab` to focus details, then move through messages.
6. Press `enter` to preview a message.

### Delete or Archive a Group

1. Focus the summary table.
2. Press `space` on one or more senders or domains.
3. Press `D` to delete or `e` to archive.
4. Read the confirmation description in the status bar.
5. Press `y` to confirm or `n`/`Esc` to cancel.

### Delete or Archive Individual Messages

1. Load details for a sender or domain.
2. Press `tab` to focus details.
3. Press `space` on individual messages.
4. Press `D` or `e`.
5. Confirm only when the status description matches your selection.

### Create a Hide-Future-Mail Rule

1. Focus a sender in the summary table or open a message preview.
2. Press `h`.
3. Herald creates the backend rule/action used to hide matching future mail.

### Create Automation

1. Focus a sender or domain that should trigger automation.
2. Press `W`.
3. Choose trigger type: sender, domain, or AI category.
4. Enter trigger value.
5. Select actions: desktop notification, move, archive, delete, webhook, or shell command.
6. Fill action details such as destination folder, webhook URL/body, shell command, or notification text.
7. Complete the form to save.

### Manage Cleanup Rules

1. Press `C`.
2. Press `n` to create a cleanup rule, `enter` to edit selected, `d` to delete, or `r` to run all.
3. In the edit form, set rule name, match type, match value, action, older-than days, and enabled state.
4. Press `esc` to leave edit mode or close the manager.

## States

| State | What happens |
| --- | --- |
| Sender mode | Summary groups exact sender addresses. |
| Domain mode | Summary groups extracted sender domains. |
| Empty summary | No cleanup groups are available for the folder/cache state. |
| Selected summary rows | Status reports selected sender/domain count. |
| Selected detail rows | Status reports selected message count and sender/domain spread. |
| Delete/archive confirmation | Status bar asks for `y` confirm or `n`/`Esc` cancel. |
| Deleting/archive in progress | Requests are queued serially; status shows progress and reconnect messages when needed. |
| Cleanup preview | Sidebar hides to make room; details panel can scroll the body. |
| Full-screen preview | Cleanup preview expands and rewraps body lines. |
| AI unavailable | `A` and category-trigger workflows cannot classify new content. |
| Dry-run mode | Status shows `[DRY RUN]`; destructive rules can be exercised without live mutation when that runtime mode is active. |
| Narrow terminal | Cleanup collapses columns and can hide the sidebar or summary panel while preview is open. |

## Data And Privacy

Cleanup reads cached sender statistics, message metadata, message bodies for previews, unsubscribe headers, classifications, and rules. Delete and archive write to the IMAP mailbox and update SQLite cache. Hide-future-mail, automation rules, custom prompts, and cleanup rules are stored through Herald's backend. Webhook and shell-command automation can send or expose email-derived values outside Herald when you configure those actions.

## Troubleshooting

If delete/archive is not available, check whether a deletion is already running or the selected folder is read-only.

If a confirmation describes the wrong target, press `n` or `Esc`, clear selections with `space`, and select again.

If `u` does nothing, the previewed message does not include a usable `List-Unsubscribe` header.

If automation actions do not run, reopen `W` or `C` to verify the rule is enabled and that the match value is precise.

## Screenshot Placeholders

<!-- HERALD_SCREENSHOT id="cleanup-domain-mode" page="cleanup" alt="Cleanup domain grouping mode" state="demo mode, 120x40, Cleanup tab, domain mode" desc="Shows domain mode status, domain summary rows, detail table, and key hints after pressing d." capture="tmux demo 120x40; ./bin/herald --demo; press 3; press d" -->

![Cleanup domain grouping mode](/screenshots/cleanup-domain-mode.png)

<!-- HERALD_SCREENSHOT id="cleanup-preview" page="cleanup" alt="Cleanup message preview open" state="demo mode, 120x40, Cleanup detail preview" desc="Shows cleanup preview body, hidden sidebar behavior, action hints for unsubscribe/hide/delete/archive, and scroll state." capture="tmux demo 120x40; ./bin/herald --demo; press 3; press enter; press tab; press enter" -->

![Cleanup message preview open](/screenshots/cleanup-preview.png)

<!-- HERALD_SCREENSHOT id="cleanup-delete-confirmation" page="cleanup" alt="Cleanup delete confirmation status bar" state="demo mode, 120x40, delete confirmation active" desc="Shows destructive confirmation text with y confirm and n or Esc cancel controls." capture="tmux demo 120x40; ./bin/herald --demo; press 3; press space; press D" -->

![Cleanup delete confirmation status bar](/screenshots/cleanup-delete-confirmation.png)

<!-- HERALD_SCREENSHOT id="cleanup-rule-editor" page="cleanup" alt="Automation rule editor overlay" state="demo mode, 120x40, rule editor open" desc="Shows trigger fields, action multiselect, action detail fields, saved rules summary, and overlay framing." capture="tmux demo 120x40; ./bin/herald --demo; press 3; press W" -->

![Automation rule editor overlay](/screenshots/cleanup-rule-editor.png)

<!-- HERALD_SCREENSHOT id="cleanup-manager" page="cleanup" alt="Cleanup manager overlay" state="demo mode, 120x40, cleanup manager open" desc="Shows saved cleanup rule list, empty or populated state, run all control, and edit entry points." capture="tmux demo 120x40; ./bin/herald --demo; press 3; press C" -->

![Cleanup manager overlay](/screenshots/cleanup-manager.png)

## Related Pages

- [Destructive Actions](/features/destructive-actions/)
- [Rules and Automation](/features/rules-automation/)
- [AI Features](/features/ai/)
- [Sync and Status](/features/sync-status/)
- [Config Reference](/reference/config/)
