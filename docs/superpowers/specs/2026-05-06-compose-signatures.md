# Compose Signatures

## Summary

This spec defines v1 account-scoped signatures for Herald Compose. The goal is to make a configured signature visible and editable in the compose textarea instead of silently changing sent mail.

- [x] Users can configure one default signature per account config under `compose.signature.text`.
- [x] The existing `S` settings panel exposes the signature as a multiline `Email Signature` field.
- [x] Empty or missing signature text disables automatic signature insertion.

## Compose Behavior

This section defines when Herald mutates the editable Compose body. Signatures are inserted only when Compose opens, so users can revise or delete them before send, draft save, or AI rewrite.

- [x] Blank Compose opens with two empty editable lines before the configured signature, and the cursor starts on the first line so the user can type above it.
- [x] Reply and forward Compose append the configured signature to the editable top-note body, not to the preserved original message context, with the cursor starting above the signature.
- [x] Quick replies append the configured signature below the selected reply text with two empty lines between the reply and signature, and leave the cursor at the start of the editable note.
- [x] Existing draft edits restore the saved draft body exactly and do not append the configured signature.
- [x] Herald does not append a duplicate when the body already ends with the configured signature.
- [x] Herald does not append signatures invisibly at send time.

## Draft Safety

This section keeps automatic signatures from creating noisy or surprising draft records. A signature-only Compose screen should behave like an untouched blank compose screen.

- [x] Leaving a blank Compose screen that contains only the auto-inserted signature does not trigger draft autosave.
- [x] Adding recipients, subject, or body text still makes Compose contentful and eligible for the existing draft autosave path.
- [x] Draft replacement and send-success draft deletion keep their existing behavior.

## Verification

This section identifies the acceptance evidence needed because the feature changes config parsing, settings UI, Compose state, and terminal rendering.

- [x] Focused Go tests cover config parsing, settings prefill/save, Compose insertion, cursor placement, duplicate prevention, draft edit opt-out, and signature-only autosave suppression.
- [x] TUI captures cover Compose and settings at `220x50`, `80x24`, and `50x15`.
- [x] Large-feature handoff includes focused tests, full tests, build, SSH smoke, and MCP smoke evidence.
