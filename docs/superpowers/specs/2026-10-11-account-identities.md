# Account sending identities — issue #74

## Contract

Users configure aliases under each mail source's `compose.identities`, with `name` and bare `address`, and an optional `compose.default_identity` address. The primary address remains selectable; without a default, it remains the default.

- [x] Validate mailbox syntax, duplicate addresses and unknown defaults. An identity is a sender header, not another authentication account.
- [x] From options carry both source and formatted sender. A single account with aliases shows From; existing single-account configurations stay unchanged.
- [x] The From field filters by typed text and cycles matching identities with arrows; clearing the filter restores all choices. Text entry elsewhere remains native.
- [x] New messages use the account default; replies prefer a matching To/CC identity in that account; forwards use its default. Drafts restore configured From addresses.
- [x] Sending uses existing ComposeSendRequest.SourceID and From. Identity-enabled ordinary drafts use BuildDraftMessage and SaveRawDraft, preserving the From header without changing backend interfaces.
- [x] Providers must already allow the configured address. Documentation explains this requirement and a deterministic demo configuration.

## Settings interface

Existing mail account settings offer an Edit sending identities button immediately after the Include Google Calendar block (or after the address for accounts without that block). The manager stages changes and saves only the selected source's identity fields, leaving connection credentials, signatures, and other sources intact.

- [x] List the primary address and configured aliases, label the default, and offer Add identity and Save changes.
- [x] Edit name/address and default status; validate addresses and duplicates with the same rules as YAML. Removing an alias requires confirmation and restores the primary default if needed.
- [x] Escape from an editor discards that edit; Escape from the manager discards all staged edits. Failed writes retain the staged manager for retry; successful writes update Compose immediately without reconnecting.
