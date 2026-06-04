---
title: Gmail Setup
description: Configure Gmail with OAuth or an App Password fallback.
---

Gmail OAuth is the recommended Gmail setup path. Herald opens a browser authorization flow, validates the selected Google access, then saves a Gmail API mail source so sync, body reads, drafts, mailbox mutations, and send use Google's API instead of IMAP.

## Recommended: Gmail OAuth

1. Install with Homebrew:

   ```sh
   brew tap herald-email/herald
   brew install herald
   ```

2. Run `herald`.
3. Choose `Gmail OAuth` in the setup wizard.
4. Complete browser authorization, then return to Herald.
5. Wait for Herald to validate Gmail API access before it continues to optional preferences.
6. Finish the remaining setup steps to save the validated config.

Homebrew and release binaries include the desktop OAuth defaults needed by the wizard.

OAuth stores refresh token data in the Herald config only after validation succeeds and you finish setup so it can refresh access tokens later. Treat the config file like a credential.

If you keep Google Calendar enabled during setup, Herald also creates a Google Calendar source from the same OAuth flow. You can add or remove calendar sources later from Settings > Accounts.

OAuth desktop client secrets are convenience defaults, not a protection boundary. Once a secret is embedded in a distributed binary, users can extract it, so Google account consent and token storage remain the real security controls.

## Fallback: Gmail with an App Password

Use this path for personal Gmail accounts with 2-Step Verification when you do not want OAuth or cannot use the Gmail API path.

1. Make sure 2-Step Verification is enabled for your Google account.
2. Create a Google App Password for Herald.
3. Run `herald` or `./bin/herald`.
4. Choose `Gmail (IMAP + App Password)` in the setup wizard.
5. Enter your Gmail address and the App Password.
6. Let Herald validate Gmail IMAP and SMTP before continuing to optional preferences.

The wizard fills:

```yaml
vendor: gmail
server:
  host: "imap.gmail.com"
  port: 993
smtp:
  host: "smtp.gmail.com"
  port: 587
```

For personal Gmail, IMAP is generally already enabled. Google Workspace accounts may require an admin to allow IMAP or may require OAuth instead of password-based IMAP.

## Source builds with OAuth

Plain `make build` embeds OAuth defaults when both `HERALD_GOOGLE_CLIENT_ID` and `HERALD_GOOGLE_CLIENT_SECRET` are available in the environment or `.herald-dev.env`; otherwise it creates a normal development binary that still builds successfully. If you run `make build && ./bin/herald` without build-time defaults or exported runtime credentials, the OAuth wizard can fail with `Google OAuth credentials are not configured`.

For a one-off local run, export credentials in the same shell that launches Herald:

```sh
export HERALD_GOOGLE_CLIENT_ID="your-client-id.apps.googleusercontent.com"
export HERALD_GOOGLE_CLIENT_SECRET="your-client-secret"
./bin/herald -config ~/.herald/conf.yaml
```

For a local development binary with OAuth defaults built in:

```sh
cp .herald-dev.env.example .herald-dev.env
$EDITOR .herald-dev.env
make build
./bin/herald -config ~/.herald/conf.yaml
```

For release-style local builds, custom env file paths, and troubleshooting details, see [Local OAuth Builds](/development/local-oauth-builds/).

## Helpful Google references

- [Set up Gmail with a third-party email client](https://knowledge.workspace.google.com/admin/sync/set-up-gmail-with-a-third-party-email-client)
- [Add Gmail to another email client](https://support.google.com/mail/answer/75726?hl=en)
- [Sign in with app passwords](https://support.google.com/mail/answer/185833?hl=en)
