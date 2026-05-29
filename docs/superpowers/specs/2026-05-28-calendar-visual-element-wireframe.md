# Calendar Visual Element Wireframe

This companion spec breaks the rejected calendar visual-parity pass into independently testable UI elements. The goal is to make every screenshot comparison explainable: left rail, main schedule body, inspector, and chrome each have a concrete terminal wireframe before being judged against the reference mocks.

## Shared Chrome

The calendar surface needs calendar-specific chrome while preserving Herald's global shell behavior. This chrome should make the current calendar mode obvious and reserve the bottom rows for state and key hints like the reference.

- [x] Render Calendar inside the shared Herald title-row tab strip with numbered top-level tab labels; do not advertise calendar-only `F1`-`F5` view tabs.
- [x] Render a calendar status row with active range, timezone, update time, and visible-event count.
- [x] Render a calendar hint row with pane cycling, navigation, open/detail, edit/new, today, timezone, and quit hints.
- [x] Keep global quit, logs, chat, settings, and text-entry routing behavior compatible with the rest of Herald.

## Left Panel

The left panel is the anchor for screens `01` through `04`. It should no longer be only a flat calendar list: it needs a border-level date range, range movement, a mini month, colored calendar toggles, and a filter footer.

- [x] Show the normalized date range as the left panel border title, without duplicating the range or view name in the body.
- [x] Show `<  Today  >` movement affordance below the range.
- [x] Render a mini month grid with weekday headers and highlight the active day, week, or 3-day window depending on the current screen.
- [x] Bold mini month days with visible events differently from regular-weight empty days, while keeping the selected day readable.
- [x] Render calendar toggles with visible colored checkboxes or swatches, grouped by account/provider when multiple accounts are present.
- [x] Render a filter footer with the current filter label such as `All Events`.
- [x] Preserve non-Latin calendar names and avoid exposing provider IDs.

## Week Body

The Week screen must become a real time grid, not a grouped list. The reference uses time rows, day columns, colored event blocks, a current-time line, and a focused event inspector.

- [x] Render weekday columns with a fixed time-axis column on the left.
- [x] Render hourly grid lines for the visible day range.
- [x] Render colored event blocks for timed events, including title and time range.
- [x] Show an explicit current-time marker line using the local wall clock only when the visible week contains today.
- [x] Keep the selected event visually distinct without relying on color alone.

## Day Body

The Day screen should read as a dense agenda timeline with a drawer, not as a generic list. It needs the same left panel and a right drawer that can show readable notes and RSVP actions.

- [x] Render the selected day as an hourly agenda with timed blocks and all-day items separated when present.
- [x] Highlight RSVP-needed events inline.
- [x] Keep the right drawer focused on selected-event detail, notes, and actions.
- [x] Preserve up/down movement across neighboring event days.

## Three Day Body

The 3-Day screen should expose three adjacent command lanes. It should feel related to the Week grid while keeping the command panel actionable.

- [x] Render three day lanes with stable day labels and colored event blocks.
- [x] Keep chronological navigation across all visible lanes.
- [x] Render next-up, open slots, conflicts, and RSVP-needed action summary in the command panel.

## Agenda Body

The Agenda screen is the compact list version of the same model. It should share the rail and inspector while making date grouping and source colors obvious.

- [x] Render `Agenda (N) for <date/range>` as the main panel border title with the range-switch hint right-aligned in dim text.
- [x] Render local date separators in the list.
- [x] Render colored source markers and RSVP-needed markers on every row.
- [x] Keep the preview/detail panel readable at standard width, with `Event Detail` on the panel border instead of repeated as a body heading.
- [x] Fall back cleanly at the minimum guard size.

## Evidence

Every element should have real app evidence before it is considered visually complete. The reference mock alone is not enough; the captured screen must show actual Herald output with the intended theme visible.

- [x] Capture Sonokai Signal screenshots with visible ANSI foreground/background colors.
- [x] Capture the same screen state at `220x50`, `80x24`, and `50x15`.
- [x] Place reference and real screenshots side-by-side in the report.
- [x] Mark any remaining visual differences as explicit follow-up checkboxes instead of accepting weak evidence.
