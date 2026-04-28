# Timeline Bulk Selection

This spec defines Timeline message selection for bulk delete and archive actions. It keeps Timeline cleanup fast while preserving the existing confirmation prompt and serial deletion worker safety model.

## User Behavior

Timeline users need to select multiple visible messages without switching to Cleanup. Selection must be discoverable, stable across normal table movement, and scoped to the current Timeline working set.

- [x] `Space` toggles selection for the focused Timeline row when the Timeline list or search results list owns focus.
- [x] A selected individual email row shows a checkmark in the leading selection column.
- [x] A collapsed thread row toggles every message represented by that row; the row shows checked when all represented messages are selected and partial when only some are selected.
- [x] Expanded thread child rows toggle only the individual visible message.
- [x] `Space` is ignored in read-only diagnostic Timeline views such as `All Mail only`.
- [x] Mouse row activation remains unchanged.

## Bulk Actions

Bulk actions reuse the existing destructive-action confirmation flow and deletion worker. This avoids introducing a second IMAP write path for Timeline actions.

- [x] `D` deletes selected Timeline messages when at least one selectable Timeline message is selected.
- [x] `e` archives selected Timeline messages when at least one non-draft Timeline message is selected.
- [x] With no Timeline selection, `D` and `e` keep the existing current-row behavior.
- [x] A collapsed thread row with no selection targets the full represented thread for delete and archive confirmation.
- [x] Deleting selected drafts is allowed and confirmation copy says drafts will be discarded when any selected target is a draft.
- [x] Archiving skips selected drafts; if all selected Timeline targets are drafts, no archive confirmation opens and the status explains that drafts cannot be archived.

## Selection State And Copy

Selection state is keyed by message ID, not row position. This lets row checks survive sorting, search result navigation, refreshes, and terminal resizing as long as the same message remains in the current Timeline working set.

- [x] Timeline selection is stored separately from Cleanup selection.
- [x] The Timeline status bar shows `N messages selected` only while the Timeline tab is active.
- [x] Timeline hints advertise `space: select` when selection is available.
- [x] Timeline hints use selected-action copy such as `D: delete selected` and `e: archive selected` when Timeline messages are selected.
- [x] Selection is pruned when selected messages disappear from the active working set or after a bulk delete/archive batch completes.

## Verification

The feature must be covered by focused Go tests and tmux captures because it changes table layout, key routing, and destructive-action prompts. Demo mode is sufficient for visual acceptance because it exercises the Timeline UI without live IMAP risk.

- [x] Go tests cover collapsed-thread selection, expanded-row selection, selected-target priority over current row, confirmation text, draft archive skipping, selection pruning, status scoping, and 80-column hint/table fit.
- [x] Demo tmux captures cover Timeline selection at `220x50`, `80x24`, and the `50x15` minimum-size guard.
- [x] SSH and MCP smoke checks are run as post-completion surfaces per repo policy.
