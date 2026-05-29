# Calendar Design Parity Roadmap

This spec turns the existing Calendar TUI reference mockups into an implementation roadmap. It intentionally ignores the concept board and starts with `01-week-time-grid.png`, then proceeds one screen at a time through `04-agenda-list.png` before polishing detail, notes, RSVP, settings, and invitation flows.

## Problem Statement

The current Calendar implementation proves provider, cache, edit, and RSVP plumbing, but it does not yet match the planned product shape. The next work should treat design parity as unfinished, not as polish on an already-complete calendar surface.

- [x] Rebuild the calendar surface around the reference screens `01` through `04`, with one independently verifiable implementation slice per screen.
- [x] Add the shared calendar-source rail that appears in `01` through `04`, adapted for Herald's multi-account source platform and Apple Calendar-style calendar grouping.
- [x] Replace raw HTML event notes with readable terminal-rendered notes before judging Event Detail or inspectors complete.
- [x] Fix date normalization, range headers, sorting, and range movement so views do not show broken or surprising historical date windows.
- [ ] Preserve global Herald behavior in Calendar: `q` and `ctrl+c` quit, settings opens and returns to the same calendar context, logs/chat still route correctly, and mail tabs keep their existing behavior.

## Reference Screens

The reference images are the source of truth for layout direction, while Herald's terminal constraints remain the source of truth for interaction and resize behavior. Each screen below should land as its own slice with tmux evidence before the next screen starts.

- [x] Screen `01`: Week Time-Grid, based on `docs/superpowers/specs/2026-05-23-calendar-tui-roadmap-assets/01-week-time-grid.png`.
- [x] Screen `02`: Day Agenda with Drawer, based on `docs/superpowers/specs/2026-05-23-calendar-tui-roadmap-assets/02-day-agenda-drawer.png`.
- [x] Screen `03`: 3-Day Command view, based on `docs/superpowers/specs/2026-05-23-calendar-tui-roadmap-assets/03-three-day-command.png`.
- [x] Screen `04`: Agenda List, based on `docs/superpowers/specs/2026-05-23-calendar-tui-roadmap-assets/04-agenda-list.png`.
- [ ] Screens `05` and `06` remain follow-up polish surfaces after the shared rail, range model, notes renderer, and invitation actions are proven in `01` through `04`.

## Shared Calendar Rail

Screens `01` through `04` need the same left-side calendar source rail before any individual view can be called design-close. The rail should feel like Apple Calendar translated into Herald's terminal language: source groups, color swatches, enabled calendars, pending counts, and compact account identity.

- [x] Render a persistent left rail for Calendar views at wide and standard sizes, with grouped sections for providers/accounts such as `iCloud`, an account email, `Google`, and `Other`.
- [x] Show each calendar as a row with a colored swatch, enabled/disabled marker, display name, and optional count for pending invitations or visible events.
- [x] Support account-scoped and all-calendar states without exposing provider IDs, CalDAV URLs, Google event IDs, sync tokens, or internal `EventRef` values.
- [x] Use account/source display names when available and fall back through calendar display name, account label, provider label, and stable source ID only as a last resort.
- [x] Let users toggle calendars from the rail, with the main view immediately filtering to enabled calendars while preserving the selected date range and selected event when still visible.
- [x] Keep non-Latin calendar names readable, including examples such as `Праздники России`, without sanitizing them out of the rail.
- [x] Collapse the rail into a compact source picker or hidden rail at narrow sizes, with an explicit key hint to reopen it instead of clipping the schedule.
- [ ] Make the rail reusable by Week, Day, 3-Day, Agenda, Calendar Search, invitation-add flows, and Settings calendar-source pickers.

## Range Header

Every calendar screen needs a clear range cue above the schedule body so the user knows what window is being shown and how to move it. This cue should be integrated into panel borders, matching Timeline's framed grouping treatment, and must be visible even when the detail/inspector panel is open.

- [x] Use a main-panel frame title such as `Agenda (4) for Tue May 28, 2026`, with `(<-/->/h/l to switch)` right-aligned in dim text.
- [x] Use week wording for week views, such as `Week Agenda for Mon May 25 - Sun May 31, 2026`, with range movement kept in the right-side frame hint.
- [x] Use 3-day wording for the command view, such as `3-Day Window for Tue May 28 - Thu May 30, 2026`, with range movement kept in the right-side frame hint.
- [x] Keep the header independent of provider sync status; loading and error messages belong in a compact status row below it, while successful load counts are represented in the frame title rather than a body status row.
- [x] Format dates from normalized local calendar days, not raw provider timestamps, so ranges never render as obviously wrong windows like the current 1997 example unless the underlying event data is genuinely from 1997.
- [ ] Include enabled-calendar scope in compact form when filtering is active, such as `4 calendars` or `Work + Family`.

## Date And Sorting Model

The date bugs should be fixed as a foundation before the screen work, because every visual target depends on credible ranges and chronological traversal. The implementation should treat provider timestamps, all-day values, recurrence instances, and timezone conversions as data normalization problems before rendering.

- [x] Normalize all visible event rows into local display intervals with start day, end day, all-day flag, event timezone label, and stable source/calendar identity before sorting.
- [x] Sort timed events by local start time, then local end time, then all-day status, then title, then scoped event ref for deterministic ties.
- [ ] Sort all-day and multi-day events into predictable top rows or day-spanning rows instead of mixing them into timed rows by midnight artifacts.
- [x] Make Agenda List use a deliberate local 30-day range, defaulting to today when that window has events and otherwise falling back to the nearest valid event window.
- [x] Hide Agenda List events that ended before the current local day by default, with a visible `[p] Show past` affordance that restores those rows without changing Day, Week, 3-Day, Search, or Event Detail behavior.
- [x] Reject or hide calendar events with invalid or missing start timestamps, including stale cache rows with absurdly long historic spans, so parser failures never render as zero-time historical dates.
- [ ] Make Week Time-Grid default to the week containing today or the selected event, not the first cached event when that produces surprising historical windows.
- [ ] Make Day Agenda default to today when possible, then nearest upcoming event, then first cached event only as a final fallback.
- [x] Preserve selected-event identity across refresh and filter changes when the same scoped event is still in the visible set.

## Navigation Contract

Calendar should inherit Timeline's panel discipline while keeping calendar-specific range movement obvious. The schedule surface should not trap arrow navigation within one day when the visible view contains more days.

- [x] Use `tab` and `shift+tab` to cycle focus across calendar rail, main schedule, and inspector/drawer/detail panels.
- [x] Use `h/l` and left/right arrows for date-range movement when the main schedule owns focus: previous/next day, week, or 3-day window depending on the active screen.
- [x] Use `ctrl+u` and `ctrl+d` as the canonical page-up/page-down actions for the focused panel, with PageUp/PageDown as optional terminal aliases when available.
- [x] Make `up/down` and `k/j` traverse visible events across day boundaries in Week and 3-Day views.
- [x] In Day Agenda, make `down` on the last event move to the first event on the next day with visible events, and make `up` on the first event move to the last event on the previous day with visible events.
- [x] Keep `enter` opening detail for the selected event and `esc` returning one level without losing the selected date range.
- [x] Keep `q` and `ctrl+c` as global quit actions even when Calendar, detail, notes, RSVP picker, or invitation-add picker has focus.
- [x] Keep settings available from Calendar with the same key used elsewhere, and return to the same calendar view, selected range, and selected event after settings closes.

## Screen 01 Week Time-Grid

Week Time-Grid is the first implementation target because it exposes the largest design gap: the current view is a grouped list, not a week-oriented schedule surface. This slice should build the shared rail, range header, range normalization, and inspector behavior that later screens reuse.

- [x] Layout the screen as calendar rail, week grid, and week inspector at wide sizes.
- [x] Render a true week surface with day columns or terminal-native lanes instead of a single vertical list grouped by day.
- [x] Keep the current week range visible in the top range header and keep `h/l` or left/right movement to previous/next week.
- [x] Show event blocks or rows with time, title, calendar color/source marker, and RSVP/pending state without exposing internal IDs.
- [x] Highlight events requiring a response with a distinct marker and theme-aware color, not color alone.
- [x] Move selection across days with `up/down` and `k/j`, including crossing from the last event of one day to the first event of the next visible day.
- [x] Render the week inspector with selected event time, local/event timezone, location, organizer, RSVP state, calendar name, and rendered notes preview.
- [x] Preserve minimum-size behavior at `50x15` by showing a clear guard or compact summary rather than clipped columns.

## Screen 02 Day Agenda With Drawer

Day Agenda should be the fast daily command view, with the shared rail on the left and a persistent drawer on the right. This slice should refine day-boundary navigation and prove that event notes can be read without opening full detail.

- [x] Layout the screen as calendar rail, day agenda, and day drawer at wide and standard sizes.
- [x] Use the frame-title range format `Day Agenda for <day>` and keep previous/next day movement visible as the right-side frame hint.
- [x] Render selected-day events through the shared calendar time-grid foundation, with the Week-style half-hour density rule on tall terminals and all-day/multi-day events separated from timed events.
- [x] Let `up/down` move across day boundaries when the user reaches the first or last event for the current day.
- [x] Show pending invitations and RSVP-needed events inline in the day list with an explicit marker such as `RSVP`.
- [x] Render drawer notes from HTML or Markdown into clean terminal text, preserving lists, links, meeting URLs, and important emphasis.
- [x] Keep `enter` opening full Event Detail and `esc` returning to the same day, event, and scroll position.

## Screen 03 3-Day Command

The 3-Day Command view should be Herald's terminal-native differentiator rather than another list. It should help the user bridge today, tomorrow, and the next day with conflicts, next-up context, and open slots.

- [x] Layout the screen as calendar rail, shared three-day time grid, and command panel.
- [x] Use the frame-title range format `3-Day Window for <start> - <end>` and let `h/l` or left/right slide the window by one day.
- [x] Render three visible days through the shared calendar time-grid foundation, with stable day labels, per-calendar color/source markers, visible selected-event focus, and Week-style half-hour density on tall terminals.
- [x] Traverse all visible events chronologically with `up/down`, skipping empty days but preserving the lane context.
- [ ] Show pending RSVP events in the command panel as a first-class action group when any are visible.
- [x] Show next-up, conflicts, and open slots based only on the currently enabled calendars.
- [x] Keep command-panel actions read-only except explicit RSVP actions and explicit open-detail actions.

## Screen 04 Agenda List

Agenda List should become the compact chronological calendar timeline once the spatial views are reliable. It should reuse the same rail and range header while behaving like Timeline where possible.

- [x] Layout the screen as calendar rail, agenda list, and event preview/detail panel.
- [x] Use the frame-title range format `Agenda (N) for <start> - <end>` with explicit previous/next range movement in the right-side frame hint.
- [ ] Group events by local date with readable date separators and stable chronological sorting inside each group.
- [x] Keep past Agenda rows hidden before today's local date until the user toggles `[p] Show past`; when hidden rows exist, show a compact count and keep `p` advertised only on the Agenda surface.
- [x] Include calendar color/source markers, RSVP-needed markers, and account badges without leaking provider internals.
- [x] Support page movement with `ctrl+u/ctrl+d` in the agenda list.
- [ ] Add independent preview scrolling when the preview/detail panel is focused.
- [x] Preserve Timeline-like search entry and unwind behavior when Agenda search is added or reused.
- [x] Keep the preview panel readable at `80x24`; at `50x15`, show the minimum-size guard or a one-panel compact agenda.

## Notes Rendering

Raw HTML in event descriptions makes the current detail and inspector surfaces feel broken even when the underlying data is correct. Notes rendering should be treated as a shared calendar reader capability, not a one-off formatter in a single panel.

- [x] Detect HTML notes from provider data and convert them into terminal-readable text or Markdown before wrapping.
- [x] Preserve paragraph breaks, ordered and unordered lists, emphasis, links, meeting URLs, dial-in numbers, and code-like tokens.
- [ ] Collapse or de-emphasize machine-generated calendar footers such as repeated separator lines and `Please do not edit this section` when doing so does not hide join details.
- [x] Reuse Herald's existing URL linkification and tracker-sanitization behavior where links are shown.
- [x] Keep Markdown notes readable with the same renderer path so providers that supply plain Markdown do not regress.
- [ ] Add fixtures for Shopify-style interview notes, Google Meet dial-in blocks, plain text descriptions, and mixed HTML plus plain text descriptions.

## RSVP And Invitation Actions

Calendar event response should be explicit and visible, not a hidden cycle action. Events that require a response should stand out in every calendar screen where they appear.

- [x] Mark events with attendee RSVP `needs-action` as requiring response in Week, Day, 3-Day, Agenda, Detail, Search, and Cross-Source surfaces.
- [x] Replace RSVP cycling as the primary interaction with explicit accept, tentative, and decline actions.
- [ ] Add an explicit reset-to-needs-action action where providers support it, instead of relying on the legacy cycle action.
- [x] Use a compact action picker or button row in inspectors/detail, such as `[y] Accept  [m] Maybe  [n] Decline`, while keeping key conflicts with view switching out of the main schedule list.
- [x] Require confirmation or clear status feedback before provider-backed RSVP mutation when the selected provider can fail or has stale revision data.
- [x] Update the cache only after provider success and keep conflict/unsupported recurrence errors visible in the same panel.
- [ ] Keep read-only calendar sources honest by showing RSVP state but disabling response actions with a clear reason.

## Email Invitation Intake

Mail and calendar should meet at invitation emails, especially when the email contains `text/calendar` parts or `.ics` attachments. This workflow should be designed after the core calendar screens have the shared rail and calendar picker.

- [x] Detect invitations in email preview from `text/calendar` MIME parts, `.ics` attachments, and recognizable calendar invite metadata.
- [x] Show an `Add to calendar` action in email preview when an invitation can be parsed, routing duplicate UIDs to update/skip behavior instead of silently appending another event.
- [x] When adding an invitation, open a calendar picker based on the shared calendar rail and ask which writable calendar should receive the event.
- [x] Parse ICS fields into the calendar event model, including summary, description, location, start/end, timezone, organizer, attendees, recurrence, attachments, reminders, UID, and sequence/revision.
- [x] Detect duplicate ICS UID values and offer update/skip behavior instead of silently creating duplicates.
- [ ] Preserve RSVP semantics for invitations, including accept/decline/tentative when the user's attendee identity can be matched.
- [x] Keep mail-only sessions honest by showing why the invitation cannot be added when no writable calendar source exists.

## Settings And Global UI

Calendar design parity depends on the rest of Herald still feeling coherent. Settings and global controls should work while Calendar has focus, and calendar settings should share the same source vocabulary as the rail.

- [x] Opening Settings from Calendar should preserve active calendar screen, focused panel, selected range, selected event, and enabled calendar filters.
- [ ] Settings > Accounts should expose mail and calendar sources using the same account labels and calendar display names shown in the calendar rail.
- [ ] Calendar source changes should validate provider credentials and then refresh the rail without forcing the user back to Timeline.
- [x] `q`, `ctrl+c`, logs, chat, help, and theme/settings controls should keep their global behavior in Calendar unless a modal explicitly documents a local override.
- [x] Calendar key hints should show only actions that work in the current focus context and should include range movement, panel cycling, paging, RSVP, settings, and detail/open actions.

## Verification Roadmap

Each slice should prove design parity with visual evidence before the next screen begins. The screenshots should be treated as acceptance references, not as loose inspiration.

- [x] Add or update TUI test plan cases before implementing each screen slice.
- [x] Run focused Go tests for date normalization, sorting, filter state, key routing, RSVP action selection, ICS parsing, and notes rendering.
- [x] Capture tmux evidence at `220x50`, `80x24`, and `50x15` for every screen slice.
- [x] Capture at least one screenshot from the real Herald app for every completed screen slice using `HERALD_THEME=sonokai-signal`, because Sonokai Signal exercises many calendar colors and contrast combinations.
- [x] Place the real Sonokai Signal screenshot side-by-side with the matching reference mockup in the implementation report so reviewers can judge visual closeness without hunting through files.
- [x] Document intentional differences from the reference mockup next to the comparison, limited to terminal constraints, Herald navigation conventions, available provider data, and accessibility/readability requirements.
- [x] Save reports in `reports/` with explicit references to the screen number, source mockup, terminal sizes, and any known intentional differences from the mockup.
- [x] Include fixtures for multiple accounts, iCloud calendars, Google calendars, Other calendars, non-Latin names, pending invitations, HTML notes, ICS attachments, all-day events, recurring events, and timezone-crossing events.
- [x] Do not mark a screen complete until the shared rail, header, sorting, navigation, and resize behavior required by that screen all pass.
- [x] Do not mark a screen visually complete if the real app screenshot is only functionally correct but no longer visually close to the reference layout, density, panel proportions, source rail, range header, or color-marker treatment.

## Implementation Order

The work should move from shared foundations into one screen at a time. This avoids another broad calendar pass that checks boxes while leaving the user-facing surface far from the design.

- [x] Phase 0: Calendar parity foundation. Build the shared calendar rail model, normalized visible event rows, range header helpers, notes-rendering helper, and calendar key-routing contract.
- [x] Phase 1: Screen `01` Week Time-Grid. Implement the shared rail in the first real screen, fix week selection/range movement, and capture visual evidence against the `01` mockup.
- [x] Phase 2: Screen `02` Day Agenda with Drawer. Reuse the rail and header, add day-boundary navigation, and prove rendered notes in the drawer.
- [x] Phase 3: Screen `03` 3-Day Command. Reuse the rail and normalized rows, then add the command panel's next-up, conflicts, and open slots.
- [ ] Phase 3 follow-up: Promote visible pending RSVP events into a first-class command panel action group.
- [x] Phase 4: Screen `04` Agenda List. Reuse the rail and normalized rows, then make the compact timeline-style agenda and preview panel match the design direction.
- [x] Phase 5: Event Detail and notes polish. Apply the notes renderer, RSVP action picker, and source labels to full detail and search/cross-source detail surfaces.
- [x] Phase 6: Email invitation intake. Add invitation detection, ICS parsing, duplicate handling, and calendar picker flow from email preview.
- [x] Phase 7: Global behavior hardening. Verify quit/log/chat/help behavior, input-routing safety, and calendar key hints from calendar screens.
- [ ] Phase 7 follow-up: Preserve settings return state and validate calendar source changes from Calendar without forcing a return to Timeline.

## Non-Goals

This roadmap is intentionally scoped to design parity and invitation workflows. It should not become a general calendar expansion pass until the existing reference screens are credible.

- [ ] Do not implement month view before screens `01` through `04` match the reference direction.
- [x] Do not expose provider IDs, event IDs, CalDAV URLs, Google IDs, sync tokens, or raw scoped refs in the calendar UI.
- [ ] Do not replace the mail Timeline, Contacts, Compose, settings, chat, or logs behavior as part of this roadmap.
- [x] Do not treat RSVP cycling alone as sufficient; explicit accept/tentative/decline actions are required.
- [x] Do not ship raw HTML notes in any completed calendar detail, drawer, inspector, command panel, or preview surface.
- [x] Do not collapse the multi-account calendar rail into a single flat list except as a narrow-terminal fallback.

## Open Decisions

These decisions should be answered before or during Phase 0 and then carried consistently through all screen slices. They should not block writing the first implementation plan, but each decision needs an explicit default.

- [ ] Decide whether the calendar rail toggle key should reuse the existing sidebar key or get a calendar-specific key when the mail folder sidebar is not visible.
- [ ] Decide whether `PageUp/PageDown` should be hard aliases for `ctrl+u/ctrl+d` on every terminal that reports them.
- [ ] Decide the exact default Agenda range length, with 30 days as the proposed starting point.
- [ ] Decide whether pending invitation counts in the rail represent visible filtered events, all cached needs-action events, or provider-reported invitation counts.
- [ ] Decide whether email invitation intake should support importing to read-only calendars as a disabled row with an explanation or hide read-only calendars from the picker.
