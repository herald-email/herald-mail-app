---
title: Compose
description: Write, preview, assist, attach, draft, reply, forward, and send mail in Herald.
---

Compose is Herald's writing screen. It supports normal message composition, replies, forwards, CC/BCC, Markdown preview, file attachments, address autocomplete, draft auto-save, AI rewrite assistance, AI subject suggestions, and send error recovery.

## Overview

Press `2` to open Compose. Compose sends through the configured SMTP server and can be opened directly or pre-filled by Timeline reply, Timeline forward, or quick reply workflows.

## Screen Anatomy

| Area | What it shows |
| --- | --- |
| To field | Primary recipients. Autocomplete searches contacts once the current token has enough characters. |
| CC field | Carbon-copy recipients with the same autocomplete behavior. |
| BCC field | Blind-copy recipients with the same autocomplete behavior. |
| Subject field | Message subject and optional AI-generated subject hint. |
| Body field | Markdown-capable message body. |
| Markdown preview | Rendered body preview using Glamour when `ctrl+p` is active. |
| Attachment lines | Attached file paths or filenames after files are added. |
| Attachment path prompt | One-line path input opened by `ctrl+a`. |
| Autocomplete dropdown | Up to several contact suggestions with selected row and hidden-count note when compressed. |
| AI assistant panel | Custom prompt input, quick rewrite actions, generated response, and accept behavior. |
| Compose status | Validation errors, send state, AI messages, attachment errors, and draft status. |

<!-- HERALD_SCREENSHOT id="compose-main-fields" page="compose" alt="Compose tab with empty message fields" state="demo mode, 120x40, Compose tab active" desc="Shows To, CC, BCC, Subject, Body, status area, and Compose key hints." capture="tmux demo 120x40; ./bin/herald --demo; press 2" -->

## Controls

| Key | Context | Preconditions | Result |
| --- | --- | --- | --- |
| `tab` | Compose fields | No autocomplete selection is being accepted and no subject hint is pending. | Moves focus To -> CC -> BCC -> Subject -> Body -> To. |
| `tab` | Subject hint | AI subject hint is visible. | Accepts the suggested subject. |
| `ctrl+s` | Compose | SMTP configured, To not empty, Subject not empty. | Sends the message with Markdown-derived HTML/plain text and attachments. |
| `ctrl+p` | Compose | Any draft. | Toggles Markdown preview. |
| `ctrl+a` | Compose | Attachment prompt not already active. | Opens file path input for adding an attachment. |
| `enter` | Attachment prompt | Attachment prompt active. | Adds the expanded path as an attachment. |
| `esc` | Attachment prompt | Attachment prompt active. | Cancels attachment input. |
| `ctrl+g` | Compose | AI backend configured. | Opens or closes the AI assistant panel. |
| `enter` | AI prompt field | AI assistant prompt has text. | Sends the custom instruction to the AI assistant. |
| `1` to `5` | AI panel | AI panel open and body is not empty. | Runs quick actions: improve, shorten, lengthen, formal, or casual. |
| `ctrl+enter` | AI panel | AI response is available. | Replaces the body with the AI response and closes the panel. |
| `ctrl+j` | Compose | AI configured and body or reply context exists. | Requests an AI subject suggestion. |
| `esc` | Compose | Subject hint, AI panel, or compose status is active. | Clears the subject hint, closes AI panel, or clears status. |
| `up` / `down` | Autocomplete dropdown | Suggestions are visible. | Moves the selected suggestion. |
| `enter` / `tab` | Autocomplete dropdown | Suggestions are visible. | Accepts the selected suggestion into the active address field. |
| `esc` | Autocomplete dropdown | Suggestions are visible. | Dismisses suggestions. |
| `1` / `3` / `4` | Compose tab | Main Compose handler active. | Switches to Timeline, Cleanup, or Contacts. |

## Workflows

### Send a New Message

1. Press `2`.
2. Enter at least one `To` recipient.
3. Press `tab` through CC, BCC, Subject, and Body as needed.
4. Write the body.
5. Press `ctrl+p` to inspect Markdown rendering when desired.
6. Press `ctrl+s`.

### Add an Attachment

1. Press `ctrl+a`.
2. Type or paste a file path. `~` is expanded.
3. Press `enter`.
4. Confirm the attachment line appears.
5. Send normally.

### Use Address Autocomplete

1. Type part of a name or email in To, CC, or BCC.
2. When suggestions appear, use `up`/`down` to select.
3. Press `enter` or `tab` to insert the contact.
4. Continue typing more recipients.

### Use AI Writing Assistance

1. Write body text first.
2. Press `ctrl+g`.
3. Press `1` through `5` for a built-in rewrite or type a custom instruction and press `enter`.
4. Review the AI response.
5. Press `ctrl+enter` to accept it into the body, or `esc` to close the panel.

### Reply or Forward From Timeline

1. In Timeline, select a message.
2. Press `R` to reply or `F` to forward.
3. Compose opens with recipient, subject, and body context pre-filled.
4. Finish the message and press `ctrl+s`.

## States

| State | What happens |
| --- | --- |
| Empty required fields | `ctrl+s` reports a send error when To or Subject is empty. |
| SMTP not configured | Send reports that SMTP is unavailable. |
| Markdown preview | Body is rendered as styled preview rather than the edit textarea. |
| Attachment input | Key input is captured by the path prompt until `enter` or `esc`. |
| Large attachment warning | Attachment add flow can warn when a file is larger than the configured threshold. |
| Autocomplete compact | Suggestions collapse to a single line when vertical space is tight. |
| AI unavailable | `ctrl+g` and `ctrl+j` report no AI backend configured. |
| AI loading | The assistant waits for provider output and then displays a response. |
| Draft saved | Compose auto-saves drafts about every 30 seconds when there is content. |
| Send success | Fields clear, saved draft is deleted, and status reports send success. |
| Send error | Draft content remains available so you can fix configuration or message fields. |

## Data And Privacy

Compose reads contacts for autocomplete and writes drafts through Herald's backend. Sending uses SMTP credentials from config and sends the full message, recipients, Markdown-derived bodies, and attachments to the SMTP server. AI assistance sends draft text and optional reply context to the configured AI backend. Attached files are read from local disk when added or sent.

## Troubleshooting

If autocomplete does not appear, keep typing until the current token is long enough and confirm contacts have been imported or learned from mail.

If `ctrl+s` does not send, read the compose status for missing To, missing Subject, SMTP configuration, or provider errors.

If an AI subject suggestion appears but you do not want it, press `esc`. If you do want it, press `tab`.

If a send error occurs after attaching files, verify the file still exists and that your provider accepts the message size.

## Screenshot Placeholders

<!-- HERALD_SCREENSHOT id="compose-markdown-preview" page="compose" alt="Compose Markdown preview mode" state="demo mode, 120x40, Compose tab, Markdown preview active" desc="Shows the rendered Markdown preview, original compose fields, and preview key state." capture="tmux demo 120x40; ./bin/herald --demo; press 2; fill body with markdown; press ctrl+p" -->

<!-- HERALD_SCREENSHOT id="compose-autocomplete" page="compose" alt="Compose address autocomplete dropdown" state="demo mode, 120x40, To field with contact suggestions" desc="Shows recipient suggestions, selected suggestion row, and accept/dismiss behavior." capture="tmux demo 120x40; ./bin/herald --demo; press 2; type two or more contact characters in To" -->

<!-- HERALD_SCREENSHOT id="compose-ai-assistant" page="compose" alt="Compose AI assistant panel" state="demo mode, 120x40, AI configured, assistant panel open" desc="Shows AI quick actions, custom instruction input, generated response area, and accept key hint." capture="tmux demo 120x40; ./bin/herald --demo with AI configured; press 2; enter body text; press ctrl+g" -->

<!-- HERALD_SCREENSHOT id="compose-attachment-input" page="compose" alt="Compose attachment path prompt" state="demo mode, 120x40, attachment input active" desc="Shows the file path input row opened by ctrl+a and the surrounding compose fields." capture="tmux demo 120x40; ./bin/herald --demo; press 2; press ctrl+a" -->

## Related Pages

- [Timeline](/using-herald/timeline/)
- [Attachments](/features/attachments/)
- [AI Features](/features/ai/)
- [Settings](/features/settings/)
- [Config Reference](/reference/config/)
