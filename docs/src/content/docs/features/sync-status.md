---
title: Sync and Status
description: Read Herald's sync strip, status bar, progress indicators, folder counts, and degraded states.
---

Sync and status messaging explains what Herald is doing in the background. The bottom status bar and optional top sync strip are the best places to understand loading, folder counts, AI tasks, deletion/archive progress, demo mode, dry-run mode, and narrow terminal decisions.

## Overview

Herald keeps a persistent IMAP connection open while the app runs. It processes new mail into SQLite, updates folder counts, listens for changes when possible, and falls back to polling based on config.

## Screen Anatomy

| Area | What it shows |
| --- | --- |
| Loading view | Startup phase, elapsed time, spinner, optional progress counts, progress bar, ETA, and quit hint. |
| Top sync strip | Human-readable current sync action when cached data is visible during live work. |
| Folder breadcrumb | Current folder path using a breadcrumb-style separator. |
| Folder counts | Unread and total count, with an unsettled marker while counts are still loading. |
| AI chip/progress | AI status and classification/embedding progress. |
| Cleanup mode | Sender/domain mode and selection counts. |
| Deletion progress | Delete/archive queue progress and reconnect status. |
| Sync timer | `live` for active IDLE or countdown seconds for polling. |
| Demo/dry-run/log flags | `[DEMO]`, `[DRY RUN]`, and `Logs ON` when active. |
| Sidebar notice | Auto-hidden sidebar explanation at narrow widths. |

<!-- HERALD_SCREENSHOT id="sync-status-main-bar" page="sync-status" alt="Herald status bar with folder counts and sync mode" state="demo mode, 120x40, Timeline tab active" desc="Shows status message, folder breadcrumb, AI chip, folder counts, sync live/countdown state, demo flag, and key hints." capture="tmux demo 120x40; ./bin/herald --demo; press 1" -->

## Controls

| Key | Context | Preconditions | Result |
| --- | --- | --- | --- |
| `r` | Main UI | Not loading. | Refreshes current folder and starts sync progress. |
| `l` / `L` | Main UI | Visible data can be interacted with. | Opens log viewer for more detailed runtime messages. |
| `q` | Loading view or main UI | Any state. | Quits. |
| `f` | Timeline/Cleanup | Sidebar supported. | Can hide/show sidebar; status reports when width auto-hides it. |
| `S` | Main UI | Settings closed. | Opens settings where sync interval, IDLE, and cleanup schedule can be changed. |

## Workflows

### Understand Startup

1. Launch Herald.
2. Watch the loading banner while no cached data is visible.
3. Once cached rows appear, use the top sync strip and status bar to track remaining IMAP work.
4. Continue using cached rows while sync completes.

### Refresh a Folder

1. Open the folder in the sidebar.
2. Press `r`.
3. Watch the top sync strip and bottom status.
4. Resume normal work after loading clears.

### Investigate Status With Logs

1. Press `l`.
2. Review INFO, WARN, ERROR, and DEBUG lines.
3. Press `l` again to close.

## States

| State | What happens |
| --- | --- |
| Opening folder | Sync strip says Herald is opening the mailbox, possibly waiting on another mail client. |
| Checking sync state | Herald compares cached state with live mailbox state. |
| Fetching new mail | Progress shows count of new rows being cached. |
| Finalizing | Sender stats and folder counts are refreshed. |
| No new mail | Status reports the current folder is up to date. |
| IDLE live | Status shows live sync. |
| Polling | Status shows countdown seconds until next poll. |
| Counts unsettled | Folder count includes an ellipsis until loading settles. |
| Connection lost during delete | Delete progress notes reconnecting. |
| Logs on | Status includes `Logs ON`. |

## Data And Privacy

Sync reads IMAP folder state, message metadata, flags, body structure, and new messages. It writes cached rows, classifications, contacts, and folder status to SQLite/backend storage. Logs are written to local log files only, not to terminal output.

## Troubleshooting

If counts look stale, press `r` and wait for sync state to settle.

If the app seems idle but expected mail is missing, verify the selected folder in the sidebar and check whether polling or IDLE is configured.

If startup takes a long time, cached data should appear as soon as possible; open logs with `l` after the main UI appears for details.

## Screenshot Placeholders

<!-- HERALD_SCREENSHOT id="sync-top-strip" page="sync-status" alt="Top sync strip during folder refresh" state="demo mode, 120x40, refresh in progress" desc="Shows the top sync strip with a folder refresh message while cached rows remain visible." capture="tmux demo 120x40; ./bin/herald --demo; press 1; press r" -->

<!-- HERALD_SCREENSHOT id="sync-loading-view" page="sync-status" alt="Startup loading view" state="demo mode, 120x40, startup loading" desc="Shows loading banner, spinner, progress text, optional progress bar, elapsed time, and quit hint before visible data." capture="tmux demo 120x40; ./bin/herald --demo; capture immediately after launch" -->

## Related Pages

- [Global UI](/using-herald/global-ui/)
- [Settings](/features/settings/)
- [Troubleshooting](/troubleshooting/)
- [Config Reference](/reference/config/)
