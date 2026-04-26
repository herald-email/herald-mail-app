---
title: Attachments
description: Save received attachments from Timeline and add outgoing attachments in Compose.
---

Herald supports attachments in both reading and writing flows. Timeline detects and saves received attachments; Compose reads local files and sends them with outgoing mail.

## Overview

Use Timeline preview attachment controls when receiving files. Use Compose `ctrl+a` when sending files. Attachment behavior depends on message MIME structure, local file paths, and provider message-size limits.

## Screen Anatomy

| Area | What it shows |
| --- | --- |
| Timeline Att column | Attachment indicator for messages whose structure includes attachments. |
| Preview attachment area | Attachment list, selected attachment, filename, and save affordance. |
| Attachment save prompt | Destination path input, defaulting to `~/Downloads/<filename>`. |
| Compose attachment prompt | File path input opened with `ctrl+a`. |
| Compose attachment lines | Files attached to the outgoing draft. |
| Compose status | Attachment add errors, large-file warnings, and send errors. |

<!-- HERALD_SCREENSHOT id="attachments-timeline-list" page="attachments" alt="Timeline attachment indicator column" state="demo mode, 120x40, Timeline tab with attachment rows" desc="Shows the Att column in Timeline and messages that can expose attachments in preview." capture="tmux demo 120x40; ./bin/herald --demo; press 1" -->

![Timeline attachment indicator column](/screenshots/attachments-timeline-list.png)

## Controls

| Key | Context | Preconditions | Result |
| --- | --- | --- | --- |
| `enter` | Timeline row | Message selected. | Opens preview and fetches body/attachment metadata. |
| `[` | Timeline preview | More than one attachment and selected index greater than zero. | Selects previous attachment. |
| `]` | Timeline preview | More than one attachment and selected index is not last. | Selects next attachment. |
| `s` | Timeline preview | Preview focused and attachments exist. | Opens save prompt. |
| `enter` | Save prompt | Save prompt active. | Saves selected attachment to the entered path. |
| `esc` | Save prompt | Save prompt active. | Cancels save prompt. |
| `ctrl+a` | Compose | Compose active. | Opens outgoing attachment path prompt. |
| `enter` | Compose attachment prompt | Prompt active. | Adds the local file path to the draft. |
| `esc` | Compose attachment prompt | Prompt active. | Cancels outgoing attachment entry. |

## Workflows

### Save a Received Attachment

1. Open Timeline.
2. Select a message with the Att indicator.
3. Press `enter` to open preview.
4. Use `[`/`]` if multiple attachments are present.
5. Press `s`.
6. Confirm or edit the destination path.
7. Press `enter`.

### Attach a File to Outgoing Mail

1. Open Compose.
2. Press `ctrl+a`.
3. Enter a file path.
4. Press `enter`.
5. Confirm the attachment appears in the draft.
6. Send with `ctrl+s`.

## States

| State | What happens |
| --- | --- |
| No attachments | `s`, `[`, and `]` do not change anything. |
| Multiple attachments | Selected attachment index changes with `[` and `]`. |
| Save prompt active | Normal Timeline keys are paused until save prompt completes or cancels. |
| Save error | Status reports backend or filesystem error. |
| Large outgoing file | Compose can warn when a file is large. |
| Missing outgoing file | Compose reports add/send failure if the path cannot be read. |
| Provider size limit | SMTP provider can reject large messages after Herald builds them. |

## Data And Privacy

Saving an attachment writes a file to the local destination path. Adding an outgoing attachment reads the local file and sends it over SMTP with the message. Attachment metadata is derived from the parsed email body and cached message state.

## Troubleshooting

If the Att column is blank but you expected attachments, the provider may not expose attachment disposition in the fetched structure.

If saving fails, check path permissions and whether the filename already exists with restrictive permissions.

If sending fails with attachments, reduce file size or verify SMTP provider limits.

## Screenshot Placeholders

<!-- HERALD_SCREENSHOT id="attachments-save-prompt" page="attachments" alt="Attachment save prompt in Timeline preview" state="demo mode, 120x40, attachment save prompt active" desc="Shows selected attachment, default Downloads destination, editable save path, and cancel behavior." capture="tmux demo 120x40; ./bin/herald --demo; open a message with attachment; press s" -->

![Attachment save prompt in Timeline preview](/screenshots/attachments-save-prompt.png)

<!-- HERALD_SCREENSHOT id="attachments-compose-added" page="attachments" alt="Compose with outgoing attachment added" state="demo mode, 120x40, Compose draft with attachment" desc="Shows attachment line in Compose after adding a local file and the send/preview key hints." capture="tmux demo 120x40; ./bin/herald --demo; press 2; press ctrl+a; enter a fixture path; press enter" -->

![Compose with outgoing attachment added](/screenshots/attachments-compose-added.png)

## Related Pages

- [Timeline](/using-herald/timeline/)
- [Compose](/using-herald/compose/)
- [Destructive Actions](/features/destructive-actions/)
