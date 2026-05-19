---
title: Gmail Setup
description: Configure Gmail with an App Password or experimental OAuth.
---

Gmail IMAP with an App Password is the normal Gmail setup path while Gmail OAuth onboarding is experimental. The first-run wizard hides OAuth unless Herald starts with `-experimental`.

## Recommended: Gmail with an App Password

Use this path for personal Gmail accounts with 2-Step Verification.

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

## Experimental: Gmail OAuth

OAuth opens a local browser authorization flow, then Herald validates Gmail IMAP and SMTP with XOAUTH2 before it continues to optional preferences or stores the resulting refresh token in config. This path is opt-in because Google OAuth onboarding and verification can take weeks and significant cost.

1. Install with Homebrew:

   ```sh
   brew tap herald-email/herald
   brew install herald
   ```

2. Run `herald -experimental`.
3. Choose `Gmail OAuth (Experimental)` in the setup wizard.
4. Complete browser authorization, then return to Herald.
5. Wait for Herald to validate IMAP and SMTP before it continues to optional preferences.
6. Finish the remaining setup steps to save the validated config.

Homebrew and release binaries include the desktop OAuth defaults needed by the experimental wizard.

On Google's test-app warning page, choose `Continue` to reach the real consent screen. `Back to safety` does not authorize Herald. On the consent screen, `Cancel` returns an authorization-cancelled error and Herald does not save settings.

OAuth stores refresh token data in the Herald config only after validation succeeds and you finish setup so it can refresh access tokens later. Treat the config file like a credential.

OAuth desktop client secrets are convenience defaults, not a protection boundary. Once a secret is embedded in a distributed binary, users can extract it, so Google account consent and token storage remain the real security controls.

## Source builds with OAuth

Plain `make build` intentionally does not embed OAuth defaults from `.herald-release.env`; it creates a normal development binary. If you run `make build && ./bin/herald -experimental` without exported runtime credentials, the OAuth wizard can fail with `Google OAuth credentials are not configured`.

For a one-off local run, export credentials in the same shell that launches Herald:

```sh
export HERALD_GOOGLE_CLIENT_ID="your-client-id.apps.googleusercontent.com"
export HERALD_GOOGLE_CLIENT_SECRET="your-client-secret"
./bin/herald -experimental -config ~/.herald/conf.yaml
```

For a local binary with OAuth defaults built in:

```sh
cp .herald-release.env.example .herald-release.env
$EDITOR .herald-release.env
make build-release-local
./bin/herald -experimental -config ~/.herald/conf.yaml
```

## Helpful Google references

- [Set up Gmail with a third-party email client](https://knowledge.workspace.google.com/admin/sync/set-up-gmail-with-a-third-party-email-client)
- [Add Gmail to another email client](https://support.google.com/mail/answer/75726?hl=en)
- [Sign in with app passwords](https://support.google.com/mail/answer/185833?hl=en)
