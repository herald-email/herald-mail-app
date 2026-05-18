# Timeline Grouping Switch

## Goal

This slice begins moving cleanup-oriented browsing into Timeline without removing the existing Cleanup tab yet. Users can keep reading chronologically by default, then press one key to group the same Timeline data by sender or domain when they want to triage repeated mail.

- [x] Timeline starts in the existing thread grouping mode.
- [x] Pressing `G` in Timeline browse contexts rotates `Thread -> Sender -> Domain -> Thread`.
- [x] Sender and domain modes use the current Timeline working set, including search or chat-filtered results when those views are active.
- [x] Cleanup remains available and unchanged in this slice.

## User Behavior

The grouping switch should feel like a view mode, not a new destructive cleanup surface. Grouped rows are browseable with the same Timeline navigation, preview, selection, delete, and archive affordances that already exist for collapsed thread rows.

- [x] Sender mode groups messages by normalized sender address and shows the newest message for each sender first.
- [x] Domain mode groups messages by normalized sender domain, including common compound public suffixes such as `co.uk` and `com.au`.
- [x] Collapsed sender and domain groups show disclosure markers, message counts, newest dates, attachment state, unread/star indicators, and classification tags using the existing Timeline row language.
- [x] `Enter`, right arrow, and `]` keep their existing collapsed-row behavior: expand/open with `Enter`, preview newest message with horizontal preview movement.
- [x] Switching grouping mode closes any open Timeline preview and keeps selection state keyed by message ID.

## Hints And Status

Grouping must be discoverable from the same chrome that teaches other Timeline actions. The Timeline frame names the active grouping mode at a glance, while hints and shortcut help expose the `G` command without stealing text-entry input.

- [x] The Timeline frame top border shows `Grouped by: Thread/Sender/Domain (G to change)`.
- [x] Timeline status shows `Group: Thread`, `Group: Sender`, or `Group: Domain`.
- [x] Timeline hints include `G: group` when Timeline browse actions are active.
- [x] Shortcut help includes `G` for cycling Timeline grouping.
- [x] Compose, search, prompt, settings, and editor-like text fields keep literal `G` input.

## Verification

The feature changes table rendering, command routing, and visible chrome, so it needs both focused Go coverage and tmux evidence. Demo mode is sufficient for visual proof because it exercises Timeline rows without live IMAP risk.

- [x] Focused Go tests cover default thread grouping, `G` rotation order, sender grouping, domain grouping, frame notice/status/hint/help copy, and text-entry safety.
- [x] Demo tmux captures cover Timeline grouping at `220x50`, `80x24`, and the `50x15` minimum-size guard.
- [x] Input-routing evidence proves Compose, prompt, and editor surfaces do not intercept literal `G`.
