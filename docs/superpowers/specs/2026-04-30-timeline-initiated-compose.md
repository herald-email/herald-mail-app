# Timeline-Initiated Compose

This spec defines Compose as a transient writing screen launched from Timeline instead of a top-level tab. It keeps writing close to the reading workflow while preserving existing reply, forward, draft, attachment, AI, and send behavior.

## User-Visible Behavior

This section covers what users should see and which keys should be discoverable after Compose stops being a tab.

- [x] Top-level navigation shows three tabs: `F1` Timeline, `F2` Cleanup, and `F3` Contacts.
- [x] Browse-number aliases map to the same three tabs: `1` Timeline, `2` Cleanup, and `3` Contacts.
- [x] Timeline uppercase `C` opens a blank Compose screen for a new outgoing message.
- [x] Timeline `R`, `F`, `E`, and quick replies keep opening Compose with their existing contextual draft state.
- [x] Lowercase `c` continues to open the chat panel and is not reused for Compose.
- [x] `Esc` in Compose first dismisses local Compose transient state, then returns to the screen that opened Compose.

## Routing Rules

This section describes the state transition contract so key handling stays predictable across plain Compose, reply/forward Compose, draft editing, and quick replies.

- [x] Compose remains a full-screen internal app state, but it is not advertised or reachable as a top-level tab.
- [x] Opening Compose records the current origin so `Esc` can restore Timeline list, Timeline preview, or Timeline search context.
- [x] Leaving a non-empty Compose screen with `Esc`, `F1`, `F2`, `F3`, or `Alt+1/2/3` uses the existing draft persistence path before switching away.
- [x] Successful send clears the Compose fields and leaves the success status visible until the user presses `Esc` to return.

## Verification

This section defines the acceptance evidence needed because the change affects visible chrome, key routing, and transient state restoration.

- [x] Focused Go tests cover Timeline `C`, three-tab routing, Compose `Esc` return behavior, contextual Compose origins, and Compose-safe plain text entry.
- [x] Golden snapshots show the three-tab bar and updated hints.
- [x] tmux demo captures verify Timeline `C` to Compose and `Esc` return at `220x50`, `80x24`, and minimum-size behavior.
