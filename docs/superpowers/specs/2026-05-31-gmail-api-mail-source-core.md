# Gmail API Mail Source Core

This spec defines the first Gmail API mail-provider slice. It matters because Herald should reduce Google mail OAuth access for normal Gmail use without removing the existing app-password IMAP Gmail paths that users already rely on.

## User-Visible Behavior

The Gmail API source should feel like the current Gmail mail account in Timeline, search, preview, compose, and cleanup flows. The transport change must stay behind the source platform boundary, so TUI, SSH, daemon, and MCP callers keep using scoped Herald refs instead of raw Gmail provider IDs.

- [x] Gmail OAuth can use a Gmail API mail source with narrower Gmail API access for sync, body reads, common mailbox mutations, and sending.
- [x] Older `provider: gmail_api` source configs remain readable as a compatibility alias for the canonical `provider: gmail` Gmail API source.
- [x] Gmail IMAP App Password setup remains available and continues to use the IMAP adapter.
- [x] Gmail API delete moves messages to Trash instead of permanent provider deletion.
- [x] Gmail API draft create/list/delete/send parity uses `users.drafts.*` while preserving Herald draft UIDs, scoped refs, and send-after-success cleanup behavior.
- [x] Gmail API history polling stores source/folder cursors, applies added/deleted/label-change events, and falls back to bounded list/get sync when the cursor is missing or expired.
- [x] Gmail API list, label, draft, and history calls page through provider results and retry bounded 429/5xx responses with UI-safe errors.

## OAuth And Provider Boundaries

Provider-specific OAuth scopes must be selected from configured source capabilities rather than requested globally. This keeps Calendar access and Gmail API mail access independent while preserving shared token storage and refresh behavior.

- [x] OAuth Gmail mail sources using `provider: gmail` request Gmail API mail access only, using `https://www.googleapis.com/auth/gmail.modify`.
- [x] `provider: gmail_api` remains accepted as a compatibility alias for the Gmail API adapter.
- [x] Google Calendar sources request Calendar scopes only when a Calendar source is configured.
- [x] Gmail App Password and credential-based Gmail IMAP configs remain on the IMAP adapter without using Google OAuth scopes.
- [x] Provider tokens, Gmail message IDs, label IDs, sync details, and raw OAuth details stay out of user-facing TUI and MCP output except through existing scoped refs.

## Core Mail Operations

The first implementation slice should cover everyday Herald mail operations while keeping advanced provider features out of scope. The adapter should use direct HTTP calls so tests can run against a deterministic fake Gmail API server.

- [x] Gmail labels map into Herald folders, including `INBOX`, `SENT`, `DRAFT`, `TRASH`, `SPAM`, `STARRED`, and an `All Mail` view.
- [x] Sync lists messages for the selected label/folder, fetches metadata/raw content as needed, and writes source-scoped cache rows.
- [x] Sync uses `users.history.list` after an initial bounded sync so read/star/label/trash/delete state can be refreshed without relisting the whole folder when Gmail accepts the stored cursor.
- [x] Body preview/full-body reads fetch raw RFC 2822 content and reuse the existing MIME parsing and preview cache behavior.
- [x] Read/unread, star/unstar, archive, trash, and move-to-label operations call Gmail API mutation endpoints and update cache only after provider success.
- [x] Compose send posts RFC 2822 MIME through `users.messages.send`.
- [x] Compose send preserves CC/BCC delivery headers and attachment MIME parts on the Gmail API path.
