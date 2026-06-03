# Email ICS Calendar Creation

## Overview

This spec defines how Herald turns calendar invitation emails into calendar events. It keeps Proton as the mail source, uses existing configured calendar sources for writes, and makes duplicate handling explicit before any provider mutation.

- [x] Timeline previews with parseable inline `text/calendar` parts or `.ics` attachments show `i create calendar event` in the preview header action row.
- [x] Mail-only sessions show a visible explanation instead of advertising a working calendar mutation.
- [x] The flow asks the user to choose among configured calendar collections when more than one writable calendar is available.
- [x] A selected calendar is checked for an existing event with the same ICS UID before creating a new provider event.
- [x] Duplicate ICS UIDs show an `Event already exists` choice with update, skip, and cancel actions.

## Data And Provider Behavior

This section describes the minimum durable interfaces needed to support the preview flow without adding a daemon or MCP mutation API. It also explains how provider-specific operations stay behind the existing calendar source boundary.

- [x] `models.EmailBody` stores parsed calendar invitation parts so preview, cache, and full-body reads can detect invites without scraping rendered text.
- [x] IMAP full-body parsing preserves inline `text/calendar` payloads and `.ics` attachment payloads; preview parsing keeps metadata and falls back to full body when invite bytes are needed.
- [x] Google Calendar imports new ICS events with `events.import` and checks duplicates with `events.list?iCalUID=...`.
- [x] CalDAV creates deterministic `<UID>.ics` resources and updates the same resource only after the duplicate prompt confirms update.
- [x] Provider-backed cache rows update only after the provider create or update succeeds.

## TUI Interaction

This section locks the visible interaction contract so implementation workers preserve existing preview chrome while adding the new action. The prompt should stay compact and readable at normal SSH sizes.

- [x] The preview header action line keeps unsubscribe and hide-future-mail actions intact while adding `i create calendar event` only for parseable invites.
- [x] The body preview keeps existing attachment rows, inline-image hints, save prompts, and body scrolling behavior unchanged.
- [x] The invitation picker shows the invitation title, selected calendar label, duplicate state when present, saving/error states, and bounded key hints.
- [x] `Esc` cancels the invitation flow without closing unrelated previews or clearing loaded bodies.
- [x] Literal `i` and `s` remain text in Compose, search prompts, settings fields, and editor-like input surfaces.

## Verification

This section defines the regression evidence expected for the feature. The virtual mail lab is preferred for realistic mail because it avoids private mailbox mutation during automated checks.

- [x] Focused tests cover inline `text/calendar`, `.ics` attachments, preview cache preservation, provider create, duplicate update/skip, no-calendar explanations, and header action copy.
- [x] Virtual-lab or demo fixtures include a parseable invite and a duplicate-UID calendar case.
- [x] TUI evidence captures Timeline invite preview, calendar picker, duplicate prompt, and minimum-size guard at `220x50`, `80x24`, and `50x15`.
- [x] SSH and MCP smoke checks prove existing non-calendar-mutation surfaces still build and respond.
