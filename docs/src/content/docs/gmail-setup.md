---
title: Gmail Setup
description: Configure personal Gmail through IMAP and an App Password.
---

Herald's stable Gmail path uses Gmail IMAP with an App Password. OAuth support exists but is experimental and requires Google client credentials unless you are using a release binary that was built with Herald's bundled desktop OAuth defaults.

## Personal Gmail with an App Password

1. Make sure 2-Step Verification is enabled for your Google account.
2. Create a Google App Password for Herald.
3. Run `./bin/herald`.
4. Choose `Gmail (IMAP + App Password)` in the setup wizard.
5. Enter your Gmail address and the App Password.

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

## Experimental Gmail OAuth

OAuth is available as an explicit experimental path. Release binaries produced by Herald's GitHub release workflow include Google OAuth defaults once the repository secrets are configured. If you build Herald from source, choose one of these paths before selecting `Gmail OAuth (Experimental)` in the wizard.

For a one-off local run, export credentials in the same shell that launches Herald:

```sh
export HERALD_GOOGLE_CLIENT_ID="your-client-id.apps.googleusercontent.com"
export HERALD_GOOGLE_CLIENT_SECRET="your-client-secret"
./bin/herald -config ~/.herald/conf.yaml
```

For a local binary with OAuth defaults built in:

```sh
cp .herald-release.env.example .herald-release.env
$EDITOR .herald-release.env
make build-release-local
./bin/herald -config ~/.herald/conf.yaml
```

Plain `make build` intentionally does not embed OAuth defaults from `.herald-release.env`; it creates a normal development binary. If you run `make build && ./bin/herald` without exported runtime credentials, the OAuth wizard can fail with `Google OAuth credentials are not configured`.

OAuth stores refresh token data in the Herald config so it can refresh access tokens later. Treat the config file like a credential.

OAuth desktop client secrets are convenience defaults, not a protection boundary. Once a secret is embedded in a distributed binary, users can extract it, so Google account consent and token storage remain the real security controls.

## Helpful Google references

- [Set up Gmail with a third-party email client](https://knowledge.workspace.google.com/admin/sync/set-up-gmail-with-a-third-party-email-client)
- [Add Gmail to another email client](https://support.google.com/mail/answer/75726?hl=en)
- [Sign in with app passwords](https://support.google.com/mail/answer/185833?hl=en)
