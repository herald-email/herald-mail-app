# Gmail API Mail Source Core

This spec defines the first Gmail API mail-provider slice. It matters because Herald should reduce Google mail OAuth access for normal Gmail use without removing the existing IMAP-based Gmail paths that users already rely on.

## User-Visible Behavior

The Gmail API source should feel like the current Gmail mail account in Timeline, search, preview, compose, and cleanup flows. The transport change must stay behind the source platform boundary, so TUI, SSH, daemon, and MCP callers keep using scoped Herald refs instead of raw Gmail provider IDs.

- [x] Gmail OAuth can use a Gmail API mail source with narrower Gmail API access for sync, body reads, common mailbox mutations, and sending.
- [x] Existing Gmail IMAP OAuth remains available for compatibility and still uses the legacy full-mail OAuth scope required by IMAP/SMTP XOAUTH2.
- [x] Gmail IMAP App Password setup remains available and continues to use the IMAP adapter.
- [x] Gmail API delete moves messages to Trash instead of permanent provider deletion.
- [ ] Gmail API draft create/update/send parity is deferred; current draft behavior remains owned by existing IMAP-capable paths until a dedicated draft slice lands.
- [ ] Gmail API history/watch incremental sync is deferred; the core slice may use bounded list/get sync with cache reconciliation.

## OAuth And Provider Boundaries

Provider-specific OAuth scopes must be selected from configured source capabilities rather than requested globally. This keeps Calendar access, legacy IMAP OAuth, and Gmail API mail access independent while preserving shared token storage and refresh behavior.

- [x] `provider: gmail_api` mail sources request Gmail API mail access only, using `https://www.googleapis.com/auth/gmail.modify`.
- [x] Google Calendar sources request Calendar scopes only when a Calendar source is configured.
- [x] Legacy Gmail IMAP OAuth continues to request `https://mail.google.com/`.
- [x] Provider tokens, Gmail message IDs, label IDs, sync details, and raw OAuth details stay out of user-facing TUI and MCP output except through existing scoped refs.

## Core Mail Operations

The first implementation slice should cover everyday Herald mail operations while keeping advanced provider features out of scope. The adapter should use direct HTTP calls so tests can run against a deterministic fake Gmail API server.

- [x] Gmail labels map into Herald folders, including `INBOX`, `SENT`, `DRAFT`, `TRASH`, `SPAM`, `STARRED`, and an `All Mail` view.
- [x] Sync lists messages for the selected label/folder, fetches metadata/raw content as needed, and writes source-scoped cache rows.
- [x] Body preview/full-body reads fetch raw RFC 2822 content and reuse the existing MIME parsing and preview cache behavior.
- [x] Read/unread, star/unstar, archive, trash, and move-to-label operations call Gmail API mutation endpoints and update cache only after provider success.
- [x] Compose send posts RFC 2822 MIME through `users.messages.send`.
