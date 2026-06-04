# Timeline-Initiated Compose

This spec defines Compose as a transient writing screen launched from Timeline instead of a top-level tab. It keeps writing close to the reading workflow while preserving existing reply, forward, draft, attachment, AI, and send behavior.

## User-Visible Behavior

This section covers what users should see and which keys should be discoverable after Compose stops being a tab.

- [x] Top-level navigation advertises two tabs: `1` Timeline and `2` Contacts.
- [x] Function-key aliases map to `F1` Timeline, `F2` Contacts, and `F3` Contacts as a legacy alias.
- [x] Timeline lowercase `c` opens a blank Compose screen for a new outgoing message; uppercase `C` remains a legacy alias.
- [x] Timeline `r`/`R`, `f`/`F`, `E`, and quick replies keep opening Compose with their existing contextual draft state.
- [x] Lowercase `g` opens the chat panel so lowercase `c` can launch Compose from Timeline.
- [x] `Esc` in Compose first dismisses local Compose transient state, then returns to the screen that opened Compose.
- [x] Successful send from blank Compose, reply Compose, forward Compose, quick reply, or draft edit returns directly to Timeline with the send status visible there.
- [x] Exiting a non-empty reply or forward Compose asks whether to keep the response as a draft or discard it before leaving Compose.

## Routing Rules

This section describes the state transition contract so key handling stays predictable across plain Compose, reply/forward Compose, draft editing, and quick replies.

- [x] Compose remains a full-screen internal app state, but it is not advertised or reachable as a top-level tab.
- [x] Opening Compose records the current origin so `Esc` can restore Timeline list, Timeline preview, or Timeline search context.
- [x] Leaving a non-empty blank Compose or draft-edit screen with `Esc`, `F1`, `F2`, or `F3` keeps using the existing draft persistence path before switching away.
- [x] Leaving a non-empty reply or forward Compose with `Esc`, `F1`, `F2`, or `F3` opens a compact keep/discard draft prompt and waits for the user's answer before switching away.
- [x] Choosing keep from the reply/forward exit prompt saves the draft through the existing draft persistence path, then returns to the requested Timeline, Contacts, or Calendar destination.
- [x] Choosing discard from the reply/forward exit prompt clears the reply/forward Compose state without saving and returns to the requested destination.
- [x] Successful send clears the Compose fields, clears reply/forward context, deletes any replaceable source draft after send success, and immediately restores the Timeline surface.

## Verification

This section defines the acceptance evidence needed because the change affects visible chrome, key routing, and transient state restoration.

- [x] Focused Go tests cover Timeline `c` and legacy `C`, three-tab routing, Compose `Esc` return behavior, contextual Compose origins, and Compose-safe plain text entry.
- [x] Focused Go tests cover send success returning to Timeline and reply/forward exit prompt keep/discard outcomes.
- [x] Golden snapshots show the three-tab bar and updated hints.
- [x] tmux demo captures verify Timeline `c` to Compose and `Esc` return at `220x50`, `80x24`, and minimum-size behavior.
