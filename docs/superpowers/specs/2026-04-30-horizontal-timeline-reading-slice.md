# Horizontal Timeline Reading Slice

## Goal
This slice reduces Timeline reading friction without taking on the full Cycle 2 navigation redesign. Users can move horizontally into and out of the preview with arrow or bracket keys while preserving the existing split preview, thread expansion, Compose, Cleanup, and attachment behaviors.

- [x] Right arrow and `]` open the preview for the highlighted Timeline row without moving focus away from the Timeline list when no preview is open.
- [x] When a preview is already open, right arrow and `]` from Timeline focus move focus into the preview without changing the previewed message.
- [x] Right arrow and `]` on a collapsed thread preview the newest message without unfolding the thread.
- [x] Right arrow and `]` from the folder sidebar move focus back to the Timeline list.
- [x] Left arrow from preview focus moves focus back to the Timeline list without closing the preview.
- [x] Left arrow and `[` from Timeline focus fold the current expanded thread row before moving farther left.
- [x] Left arrow and `[` from Timeline focus close an open preview and show/focus the folder sidebar when the current row is a single email or collapsed thread and the layout can render folders.
- [x] Left arrow and `[` with no open preview show and focus the folder sidebar when the layout can render it.
- [x] Bracket keys keep their existing attachment navigation behavior when the preview panel itself has focus and an email has multiple attachments.

## Read State
Unread state is intentional user state, so preview movement must not mark a message read before Herald has content to show. The existing read-after-body-load behavior remains the default, and this slice adds one explicit command for keeping a message unread after inspection.

- [x] A message is marked read only after its body fetch succeeds and the preview belongs to that message.
- [x] `U` marks the current previewed or focused Timeline message unread through the existing backend read/unread contract.
- [x] Timeline rows refresh immediately after read or unread changes so the unread dot reflects the latest local state.
- [x] Read-only diagnostic views such as `All Mail only` block `U`, read marking, and other read/write mutations.

## Hints And Help
The bottom hint bar and shortcut help are part of the feature contract because this change introduces a new movement model. Visible copy should teach the slice without documenting future Cycle 2 behaviors that are not implemented yet.

- [x] Timeline list hints advertise horizontal preview movement with right arrow / `]` and folder movement with left arrow / `[`.
- [x] Timeline preview/list hints advertise `U` as mark unread.
- [x] The `?` shortcut help describes horizontal preview movement, contextual bracket behavior, and `U`.
