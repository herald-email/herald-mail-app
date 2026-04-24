# Unsubscribe Discoverability And Semantics

This spec defines how Herald should expose mailing-list unsubscribe and sender-level inbox hiding so the UI stops depending on prior knowledge. The goal is a consistent contract across Timeline and Cleanup where users can see the available action from the preview itself before they press a key.

## Problem

This section captures the UX failures that made the current behavior feel broken even though parts of the feature existed in code. The work is intentionally about semantics and discoverability, not a backend redesign.

- [x] The current UI hides unsubscribe affordances from the places where users actually look: the preview header and the global hint bar
- [x] Cleanup currently overloads `u` with sender-level behavior that is not the same as real mailing-list unsubscribe
- [x] End-user copy like `hard unsubscribe` and `soft unsubscribe` makes the feature sound more technical and more capable than it feels in use

## Goals

This section defines the user-visible outcomes this pass must achieve. Each item is phrased as an observable behavior rather than an implementation detail.

- [x] A first-time user can discover list and sender actions from an open email preview without prior knowledge
- [x] Timeline preview and Cleanup preview expose the same `u` / `h` semantics and wording
- [x] Cleanup sender summary exposes only the sender-level action and does not pretend it can do a message-level unsubscribe
- [x] User-facing wording prefers `Unsubscribe` and `Hide Future Mail` over `hard unsubscribe` and `soft unsubscribe`

## Key Contract

This section defines the exact meaning of the relevant keys so the implementation does not drift across views. The contract is decision-complete for the execution pass and should be treated as the product source of truth for this feature.

- [x] `u` means `Unsubscribe this mailing-list email`
- [x] `u` is available only from an open email preview whose loaded message body exposes a non-empty `List-Unsubscribe` header
- [x] `h` means `Hide Future Mail from this sender`
- [x] `h` maps to the existing rule-based behavior that moves future mail from the sender into `Disabled Subscriptions`
- [x] Cleanup sender summary may advertise and trigger `h`
- [x] Cleanup sender summary must not advertise `u`, because sender rows do not have message-level unsubscribe context
- [x] Technical implementation details like RFC 8058 one-click POST, browser fallback, and mailto fallback remain valid internals of `u`

## Preview Metadata

This section defines the preview-header requirements for both split and full-screen email previews. The metadata rows are the primary discoverability surface for this change and should remain aligned between Timeline and Cleanup.

- [x] Timeline preview and Cleanup preview continue to show `From`, `Date`, and `Subj`
- [x] Both previews add a `Tags:` row
- [x] Both previews add an `Actions:` row
- [x] `Tags:` shows the current classification tag when present and otherwise uses an explicit empty-state value
- [x] When the previewed email has `List-Unsubscribe`, `Actions:` shows `u unsubscribe` and `h hide future mail`
- [x] When the previewed email does not have `List-Unsubscribe`, `Actions:` shows only `h hide future mail`
- [x] The action rows may wrap within the preview width, but they must remain readable at `220x50` and `80x24`

## Hint Bar

This section defines the required parity between the preview metadata and the persistent bottom hint bar. The hint bar should reinforce what the preview already told the user, not introduce a different mental model.

- [x] Timeline preview hint bars advertise `h: hide future mail` in all normal read-write preview states
- [x] Timeline preview hint bars advertise `u: unsubscribe` only when the previewed email exposes `List-Unsubscribe`
- [x] Cleanup preview hint bars mirror the same `u` / `h` availability rules as Timeline preview
- [x] Cleanup sender summary hint bars advertise `h: hide future mail`
- [x] Cleanup sender summary hint bars do not advertise `u: unsubscribe`
- [x] End-user hint bars and preview metadata do not use `hard unsubscribe` or `soft unsubscribe`

## Non-Goals

This section marks the adjacent work that is intentionally out of scope so the execution pass stays focused. Anything listed here should be deferred to a later task even if the code makes it tempting to expand the change.

- [x] No batch unsubscribe picker or batch execution flow in this pass
- [x] No MCP or backend contract redesign in this pass
- [x] No `ARCHITECTURE.md` update in this pass
- [x] No change to the internal rule storage format for sender-hiding behavior in this pass
