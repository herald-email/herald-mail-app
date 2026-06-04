---
title: What's New in v0.6
description: User-facing changes in the v0.6 beta release line through v0.6.2.
---

The v0.6 beta line turns Herald into a source-aware mail and calendar workspace. `v0.6.2-beta.1` is the current beta, building on the `v0.6.0-beta.1` Gmail OAuth graduation and the `v0.6.1-beta.1` multi-account/calendar hardening patch.

## Release Delta

These are the changes most visible to people using Herald every day. The theme of the release is provider-aware setup, safer mutations, and richer reading workflows without leaving the terminal.

- [x] Gmail OAuth now uses Herald's Gmail API mail source for core sync, body reads, drafts, mailbox mutations, and send, while Gmail App Password remains the IMAP/SMTP fallback.
- [x] Google Calendar OAuth and CalDAV calendar sources are first-class account sources, with cache-backed Week, Day, 3-Day, Agenda, Search, and Event Detail views.
- [x] Calendar mutations are provider-backed: create, edit, delete, RSVP, attendee editing, reminders, recurrence edits, and endpoint-specific travel timezones write through the provider before cache updates.
- [x] Email previews can import `text/calendar` parts and `.ics` attachments into a selected writable calendar, including duplicate UID update/skip handling.
- [x] Multi-account mail behavior is source-aware, including account-scoped folder trees, source badges, From routing in Compose, draft handling, and scoped MCP/daemon mutation guards.
- [x] Preview rendering is safer: bounded native image previews, opt-in remote HTML image reveal with tracker sanitization, sanitized OSC 8 link targets, and local fallback links/placeholders.
- [x] Read-only previews gained practical cursor selection, mouse drag selection, visual-range copy, line/body copy, and richer clipboard payloads where the platform supports them.
- [x] Compose gained an external-editor body flow, preserved reply/forward context, account-scoped signatures, AI toolbar polish, and Gmail API draft/send parity.
- [x] Contacts can import from native macOS Contacts where supported, and layout-correct keyboard routing keeps printable text safe in Compose, search, prompts, settings, and editor-like fields.
- [x] Local notifications and `herald://mail/...` deep links can return to folder, message, sender, search, or compose context where platform support exists.
- [x] First-run and Settings flows are faster and safer, with Google account setup, account/calendar source management, AI repair states, privacy-safe logs, and explicit `-unsafe-logs` diagnostics.

## Beta Notes

Each beta in the v0.6 line tightened a different part of the new source platform. This breakdown helps testers decide what to re-check after upgrading.

- [x] `v0.6.0-beta.1` graduates Google OAuth onboarding and the Gmail API mail source.
- [x] `v0.6.1-beta.1` fixes calendar agenda clock drift, live multi-account folder refresh, and two-account mail operations.
- [x] `v0.6.2-beta.1` adds the latest calendar create guidance and preview cursor-selection/rich-copy polish.

## Demo Highlights

The new demo media focuses on flows that changed after v0.5. These GIFs run against deterministic demo data, so they do not touch a real inbox or provider account.

![Calendar workspace demo](/demo/calendar-workspace.gif)

![Calendar invitation import demo](/demo/calendar-invitation.gif)

![Preview selection and linked image demo](/demo/preview-selection-images.gif)

![Preserved reply Compose demo](/demo/compose-preserved-reply.gif)

## How To Try It

Demo mode is the fastest way to explore the v0.6 workflows without connecting mail. Use a release or Homebrew binary for OAuth defaults; source builds need local OAuth defaults before Gmail OAuth or Google Calendar OAuth can open a browser flow.

```sh
make build
./bin/herald --demo
./bin/herald --demo --demo-keys
./bin/herald --demo --open 'herald://mail/search?folder=INBOX&q=invoice'
```

Press `3` for Calendar, `n` to create an event, `e` to edit one, `/` to search events, and `i` from an email preview containing an invitation to create or update a calendar event.

## Previous Release

The v0.5 release introduced Calendar as a top-level surface and landed the first broad source-identity work. See [What's New in v0.5](/whats-new-in-v0-5/) for that historical release checklist.
