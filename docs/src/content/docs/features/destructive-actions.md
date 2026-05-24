---
title: Destructive Actions
description: Understand delete, archive, unsubscribe, hide-future-mail, confirmations, progress, retries, and read-only states.
---

Destructive actions change your mailbox or future-mail behavior. Herald routes these through confirmations, progress reporting, read-only checks, and a serialized worker so you can see what is happening.

## Overview

Delete, archive, unsubscribe, and hide-future-mail are available from Timeline. Sender/domain cleanup is handled by Timeline grouping, so bulk delete/archive uses the same selected-row model as normal reading.

## Screen Anatomy

| Area | What it shows |
| --- | --- |
| Confirmation status bar | Action description plus `y` confirm and `n`/`Esc` cancel. |
| Delete/archive progress | Current sender or message and completed/total request count. |
| Preview action line | `u unsubscribe` when available and `h hide future mail`. |
| Selection columns | Timeline selection markers that define bulk targets. |
| Read-only status | Diagnostic mode that blocks mutations. |
| Dry-run flag | `[DRY RUN]` when runtime mode avoids live mutation for supported cleanup paths. |

<!-- HERALD_SCREENSHOT id="destructive-delete-confirm" page="destructive-actions" alt="Delete confirmation status bar" state="demo mode, 120x40, Timeline sender group delete confirmation active" desc="Shows confirmation description, y confirm, n or Esc cancel, selected rows, and status override." capture="tmux demo 120x40; ./bin/herald --demo; press 1; press G; press space; press d" -->

![Delete confirmation status bar](/screenshots/destructive-delete-confirm.png)

## Controls

| Key | Context | Preconditions | Result |
| --- | --- | --- | --- |
| `d` / `backspace` | Timeline | Target exists, not read-only, not already deleting. | Opens delete confirmation. |
| `D` / `shift+backspace` | Timeline | Target exists, not read-only, not already deleting. | Queues deletion immediately without confirmation. |
| `a` / `e` | Timeline | Target exists, not read-only, not already deleting. | Opens archive confirmation or queues current preview email. |
| `y` / `Y` | Confirmation | Delete/archive/unsubscribe confirmation active. | Confirms the pending action. |
| `n` / `N` | Confirmation | Confirmation active. | Cancels the pending action. |
| `esc` | Confirmation | Confirmation active. | Cancels the pending action. |
| `u` | Timeline preview | Body includes `List-Unsubscribe` and not read-only. | Opens unsubscribe confirmation. |
| `h` / `H` | Timeline | Current sender exists. | Creates hide-future-mail behavior for the sender. |
| `space` | Timeline | Timeline list focused. | Selects bulk delete/archive targets. |

## Workflows

### Delete Selected Timeline Groups

1. Open Timeline with `1`.
2. Press `G` until sender or domain grouping is active.
3. Use `space` to select groups or messages.
4. Press `d`.
5. Read the confirmation text.
6. Press `y` only if the target is correct.

Use `D` only when you intentionally want to delete immediately without the confirmation step.

### Archive Selected Timeline Groups

1. Group Timeline by sender or domain with `G`.
2. Select groups or messages.
3. Press `a` or `e`.
4. Confirm with `y`.
5. Watch progress in the status bar.

### Unsubscribe From a Sender

1. Open a Timeline preview.
2. Confirm the action line includes unsubscribe.
3. Press `u`.
4. Read the confirmation.
5. Press `y` to run the unsubscribe method.

### Hide Future Mail

1. Focus a sender row or open a preview.
2. Press `h`.
3. Herald saves backend behavior to hide matching future mail.

## States

| State | What happens |
| --- | --- |
| Confirmation active | Normal status is replaced until `y`, `n`, or `esc`. |
| Worker queue | Delete/archive requests are processed serially. |
| Retry | Connection errors can be retried with backoff. |
| Reconnecting | Status reports reconnecting during deletion work. |
| Archive | Message is moved through backend archive behavior and cache state updates. |
| Delete | Message is copied/moved toward Trash when possible, marked deleted, expunged, and removed from cache. |
| Unsubscribe unavailable | `u` does nothing unless the body has `List-Unsubscribe`. |
| Read-only diagnostic | Mutations are blocked, especially in `All Mail only`. |
| Dry-run | Status marks dry-run mode for supported cleanup paths. |

## Data And Privacy

Delete/archive operations mutate the configured IMAP mailbox and SQLite cache. Delete attempts Trash-folder semantics before expunging. Archive moves mail according to backend behavior. Unsubscribe may open a URL, copy a URL or mailto target, or perform a one-click unsubscribe method depending on header data. Hide-future-mail writes a local/backend rule for matching future mail.

## Troubleshooting

If `d`, `D`, or `e` appears inactive, check for read-only folder mode, active deletion progress, or missing selection.

If deletion progress stalls, open logs with `l` after the UI is usable and check provider connectivity.

If unsubscribe opens a browser or copies a URL instead of silently completing, that behavior comes from the message's unsubscribe header method.

If mail reappears after delete/archive, refresh with `r` and verify provider Trash/Archive semantics.

## Screenshot Placeholders

<!-- HERALD_SCREENSHOT id="destructive-archive-confirm" page="destructive-actions" alt="Archive confirmation status bar" state="demo mode, 120x40, Timeline grouped archive confirmation active" desc="Shows archive target description, confirmation controls, and selection context." capture="tmux demo 120x40; ./bin/herald --demo; press 1; press G; press space; press e" -->

![Archive confirmation status bar](/screenshots/destructive-archive-confirm.png)

<!-- HERALD_SCREENSHOT id="destructive-unsubscribe-confirm" page="destructive-actions" alt="Unsubscribe confirmation status bar" state="demo mode, 120x40, unsubscribe confirmation active" desc="Shows sender unsubscribe confirmation from a preview that includes List-Unsubscribe data." capture="tmux demo 120x40; ./bin/herald --demo; open a message with List-Unsubscribe; press u" -->

![Unsubscribe confirmation status bar](/screenshots/destructive-unsubscribe-confirm.png)

<!-- HERALD_SCREENSHOT id="destructive-progress" page="destructive-actions" alt="Delete or archive progress in status bar" state="demo mode, 120x40, deletion worker active" desc="Shows serialized worker progress, completed/total count, sender label, and reconnecting state if applicable." capture="tmux demo 120x40; ./bin/herald --demo; press 1; press G; select rows; press d; press y; capture during progress" -->

![Delete or archive progress in status bar](/screenshots/destructive-progress.png)

## Related Pages

- [Cleanup via Timeline](/using-herald/cleanup/)
- [Timeline](/using-herald/timeline/)
- [Rules and Automation](/features/rules-automation/)
- [Privacy and Security](/security-privacy/)
