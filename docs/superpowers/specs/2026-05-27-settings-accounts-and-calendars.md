# Settings Accounts And Calendars

This spec defines the in-app Settings source manager for Herald's existing multi-account mail and calendar platform. It matters because mail and calendar sources already exist in config and runtime code, but users currently have no Settings path for seeing, adding, editing, or disconnecting them.

## Product Shape

Settings should make configured sources visible without turning account management into a separate app. Users open `S`, choose `Accounts`, inspect a grouped source list, and edit one account/source at a time.

- [x] The top-level Settings menu shows `Accounts` instead of `Account setup`.
- [x] `Settings > Accounts` groups configured `sources[]` by `account_id`, materializing the legacy single-account config as `default-mail` for display and edits.
- [x] Account rows show a display name, provider/identity, and capability badge: `Mail`, `Calendar`, or `Mail + Calendar`.
- [x] The final rows are `Add account` and `Add calendar only`; no intermediate add-type submenu is required.
- [x] `Esc` returns from account detail or add flow to Accounts, then from Accounts to the top-level Settings menu.

## Account Detail And Add Flow

The detail forms should reuse proven setup controls where possible and keep provider deletion semantics explicit. Deleting from Herald disconnects config only; it never deletes provider data.

- [x] Mail-capable account detail shows the existing account setup fields scoped to that source.
- [x] Calendar-capable account detail shows Google Calendar or CalDAV fields scoped to that source.
- [x] Account detail and account rows include safe delete aliases that confirm before disconnecting, and fast delete aliases that immediately disconnect after selection.
- [x] Account delete removes Herald config sources and local cache rows for that account while never deleting provider-side mail or calendars.
- [x] Account delete blocks deletion of the last mail source.
- [x] `Add account` opens a provider-first mail setup form and exposes paired-calendar intent in that same flow when supported.
- [x] Gmail OAuth defaults paired Google Calendar on, derives calendar identity from the Gmail address, and validates mail plus calendar after one OAuth flow.
- [x] `Add calendar only` creates a standalone Google Calendar OAuth source or CalDAV source.
- [x] First-run Google onboarding reuses the paired Gmail + Google Calendar source shape instead of writing only legacy top-level Gmail account fields.

## Validation And Runtime Application

Config writes should happen only after validation succeeds. Runtime replacement should preserve the current safety behavior: failed validation leaves the previous config, backend, SMTP client, and visible state active.

- [x] Explicit `sources[]` configs validate without requiring legacy top-level `credentials`, `server`, or `smtp` fields.
- [x] Mail source saves validate IMAP and SMTP.
- [x] Calendar source saves validate by listing calendars, with no event mutation.
- [x] Successful source changes rebuild a single-mail `LocalBackend` or multi-mail `MultiBackend` based on the resulting config.
- [x] Failed validation shows a compact bounded message and does not replace runtime services.

## Provider Scope

This slice exposes only providers already supported by Herald's source platform. Local calendar is deliberately out of scope.

- [x] Gmail OAuth can add mail plus Google Calendar in one default-on paired setup when the OAuth token has calendar scopes.
- [x] Google Calendar setup is visible by default, and missing OAuth client credentials fail at OAuth start instead of forcing users into a CalDAV URL/password form.
- [x] Gmail IMAP app-password remains mail-only.
- [x] Fastmail and iCloud can optionally pair mail with CalDAV, using editable CalDAV fields.
- [x] Custom CalDAV is available from `Add Calendar`.
- [x] Outlook, ProtonMail Bridge, and Standard IMAP stay mail-only unless the user adds a separate calendar source.
