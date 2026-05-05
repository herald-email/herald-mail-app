# Cleanup Discoverability Polish

This spec defines the intended behavior for the Cleanup-adjacent overlays and the `All Mail only` virtual folder so the UI stops relying on prior knowledge. It focuses on discoverability, honest wording, and wide-terminal readability rather than adding a new mail-processing engine.

## Goals

- Make `W`, `P`, and `C` understandable on first open.
- Clarify where saved rules or prompts can be reviewed again.
- Clarify where cleanup results show up after a manual or scheduled run.
- Tighten `All Mail only` so it means folder-unassigned mail, not a vague diagnostic bucket.
- Use wide Cleanup layouts to show a more informative sender date range.

## `All Mail only`

`All Mail only` is a read-only diagnostic view for messages that exist in `All Mail` and have no other real IMAP folder assignment. If a message is also present in `INBOX`, `Sent`, `Archive`, or any nested subfolder, it must be excluded from this view.

The view remains fail-closed:

- if `All Mail` is unavailable, show an unsupported state
- if folder-membership inspection is incomplete, show an unsupported state
- never show a partial "best guess" result set

## Cleanup Overlay Copy

The Cleanup-adjacent overlays must explain three things in the viewport itself before the user starts filling fields:

1. What this tool is for.
2. What saving or running it will do.
3. Where the user can come back to review saved items or see results.

The `W`, `P`, `C`, and dry-run preview surfaces render as compact centered overlays over the current Cleanup view, matching Settings `S` and shortcut help `?`. At `80x24`, each overlay must fit inside the viewport with internal truncation or scrolling where needed; at `50x15`, Herald's standard minimum-size guard remains in charge.

### `W` — automation rule editor

The rule overlay must explain that it creates a real-time automation rule for future matching mail. It must also show a compact inventory of existing saved automation rules in the same compact centered overlay so reopening `W` answers "where did my rule go?"

### `P` — custom prompt editor

The prompt overlay must explain that prompts are reusable AI instructions, not actions by themselves. It must show a compact inventory of saved prompts in the same compact centered overlay so reopening `P` answers "where did my prompt go?"

### `C` — cleanup rules manager

The cleanup rules overlay must explain that cleanup rules operate on older sender/domain mail and can be run manually or on the configured schedule. The list view must tell the user that saved cleanup rules live in this compact centered manager and that manual run results surface through the status message plus visible archive/delete effects.

## Cleanup Date Range Presentation

On wide terminals, the Cleanup summary date-range column should not stay artificially capped to the narrow format needed for `80x24`. It should expand enough to show a more specific first/last date range while keeping the sender column readable.

The intended presentation is responsive:

- narrow layouts may use a compact month-oriented range
- wide layouts should show a more specific day-level range
- the date column should gain width on large terminals instead of freezing at the narrow cap
