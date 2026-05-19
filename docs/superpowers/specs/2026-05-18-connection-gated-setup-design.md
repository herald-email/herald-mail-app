# Connection-Gated Setup Design

## Purpose

This spec defines the account-setup behavior for first-run onboarding and in-app account settings. It matters because Herald must not save or apply a mail account that has not proven both read and send connectivity.

- [x] First-run account setup must validate IMAP and SMTP immediately after account details, before optional preferences or writes to the selected config path.
- [x] In-app account settings must validate IMAP and SMTP before replacing the active account config, backend, or SMTP client.
- [x] Normal startup for an already configured account must keep the existing cached/offline startup behavior.
- [x] Demo mode must remain offline and must not require IMAP, SMTP, or OAuth.

## OAuth Behavior

OAuth produces a candidate account config, not a saved config. This keeps token exchange, connection validation, and user-visible failure states in one setup flow instead of letting the OAuth wait screen silently persist credentials.

- [x] Google consent cancellation must show a clear authorization-cancelled message and must not save account settings.
- [x] Local `Esc` or `q` cancellation on the OAuth wait screen must stop waiting and report that setup was cancelled.
- [x] OAuth waits must time out with guidance that mentions Google test-app warning screens and the `Continue` path.
- [x] Gmail OAuth must support SMTP XOAUTH2 so the same OAuth account can validate and send mail.

## Connection Validation

Validation uses a shared checker so first-run setup and in-app account settings agree about what "ready" means. The checker reports IMAP and SMTP independently while giving the UI a single concise summary.

- [x] Validation must use bounded contexts/timeouts for IMAP and SMTP.
- [x] Validation must sanitize errors for display while logging the detailed failure.
- [x] IMAP validation must authenticate and close without syncing the mailbox.
- [x] SMTP validation must authenticate and close without sending a message.

## User Interface

Validation failures must be visible in the same TUI surface where the user is configuring the account. Status-bar-only errors are not enough for setup and account-settings failures.

- [x] First-run validation errors must render in the wizard and leave the user in setup rather than exiting into a half-configured state.
- [x] In-app validation errors must render as a compact centered modal over the current Herald screen.
- [x] Success flow must continue to preferences only after the account was validated; settings are saved or applied only after that gate passes.
- [x] Error copy must include the config path behavior, debug/log hint, and the failed connection surface.
