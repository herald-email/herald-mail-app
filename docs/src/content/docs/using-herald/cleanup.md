---
title: Cleanup
description: Review sender or domain cleanup candidates from Timeline grouping and manage cleanup automation from Settings.
---

Cleanup is no longer a separate top-level tab. The browse workflow now lives in Timeline grouping: press `G` to cycle from thread view to sender groups and domain groups, then use the same Timeline preview, selection, delete, archive, unsubscribe, and hide-future-mail controls.

## Overview

Press `1` for Timeline, then press `G` until the list is grouped the way you want. Use sender grouping to review high-volume people or services, domain grouping to review organizations, and `Settings > Sync & Cleanup` for automation rules, custom prompts, and saved cleanup rules.

## Screen Anatomy

| Area | What it shows |
| --- | --- |
| Timeline list | Thread, sender, or domain groups depending on the current `G` grouping mode. |
| Preview panel | The selected email body, metadata, actions, attachments, unsubscribe visibility, and scroll state. |
| Settings Sync & Cleanup | Polling, IMAP IDLE, offline cache, reclaim storage, cleanup schedule, and manager launchers. |
| Rule editor overlay | Compact centered automation-rule form launched from Settings. |
| Custom prompt overlay | Compact centered prompt editor launched from Settings. |
| Cleanup manager overlay | Compact centered saved cleanup rules manager launched from Settings. |
| Dry-run preview overlay | Compact centered preview of matched messages, folders, categories, and planned actions before a rule can be saved, enabled, or run live. |

## Controls

| Key | Context | Preconditions | Result |
| --- | --- | --- | --- |
| `G` | Timeline list | Timeline is focused. | Cycles thread, sender, and domain grouping. |
| `space` | Timeline list | Visible data can be interacted with. | Selects or unselects the highlighted message or group. |
| `d` / `backspace` | Timeline list/preview | Target exists, not read-only, not already deleting. | Opens delete confirmation for the highlighted or selected target. |
| `D` / `shift+backspace` | Timeline list/preview | Target exists, not read-only, not already deleting. | Immediately queues deletion without confirmation. |
| `a` / `e` | Timeline list/preview | Target exists, not read-only, not already deleting. | Archives the highlighted or selected target. |
| `enter` | Timeline list | A row is highlighted. | Opens a preview, expands a normal thread, or focuses grouped mail. |
| `u` | Timeline preview | Body includes `List-Unsubscribe`. | Opens unsubscribe confirmation. |
| `H` | Timeline list/preview | A current sender exists. | Creates hide-future-mail behavior for that sender. |
| `S` | Main UI | Settings closed. | Opens Settings; choose `Sync & Cleanup` for automation and cleanup managers. |

## Workflows

### Review a Sender or Domain

1. Press `1`.
2. Press `G` until Timeline is grouped by sender or domain.
3. Move through groups with `j`/`k` or arrows.
4. Press `enter` or right arrow to preview the highlighted mail.
5. Press `space` to select a group when you want to act on more than the highlighted row.

### Delete or Archive Grouped Mail

1. Group Timeline by sender or domain with `G`.
2. Highlight or select one or more groups.
3. Press `d` to delete with confirmation or `a`/`e` to archive.
4. Read the confirmation description; it should name a sender group or domain group.
5. Press `y` to confirm or `n`/`Esc` to cancel.

### Manage Automation and Cleanup Rules

1. Press `S`.
2. Choose `Sync & Cleanup`.
3. Use the automation-rule, custom-prompt, or cleanup-rule launcher.
4. Review any dry-run preview before enabling or running archive/delete rules.

## States

| State | What happens |
| --- | --- |
| Thread grouping | Timeline behaves as the normal reading list. |
| Sender grouping | Rows represent senders and destructive confirmation copy names sender groups. |
| Domain grouping | Rows represent domains and destructive confirmation copy names domain groups. |
| Selected Timeline rows | Status reports selected message count. Group rows expand to their represented messages for delete/archive. |
| Rules dry-run preview | Compact overlay lists matched messages with sender/domain/category, folder, subject/date, and planned action without mutating IMAP or SQLite run metadata. |
| Narrow terminal | At `50x15`, Herald shows the standard minimum-size guard. Compact overlays fit at `80x24`. |

## Data And Privacy

Timeline cleanup reads cached message metadata, message bodies for previews, unsubscribe headers, classifications, and rules. Delete and archive write to the IMAP mailbox and update SQLite cache. Hide-future-mail, automation rules, custom prompts, and cleanup rules are stored through Herald's backend. Webhook and shell-command automation can send or expose email-derived values outside Herald when you configure those actions.

## Related Pages

- [Timeline](/using-herald/timeline/)
- [Destructive Actions](/features/destructive-actions/)
- [Rules and Automation](/features/rules-automation/)
- [Settings](/features/settings/)
- [MCP Server](/advanced/mcp/)
