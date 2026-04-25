---
title: Gmail Setup
description: Configure personal Gmail through IMAP and an App Password.
---

Herald's stable Gmail path uses Gmail IMAP with an App Password. OAuth support exists but is experimental and requires Google client credentials.

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

OAuth is available as an explicit experimental path. Before choosing it, set:

```sh
export HERALD_GOOGLE_CLIENT_ID="your-client-id.apps.googleusercontent.com"
export HERALD_GOOGLE_CLIENT_SECRET="your-client-secret"
```

OAuth stores refresh token data in the Herald config so it can refresh access tokens later. Treat the config file like a credential.

## Helpful Google references

- [Set up Gmail with a third-party email client](https://knowledge.workspace.google.com/admin/sync/set-up-gmail-with-a-third-party-email-client)
- [Add Gmail to another email client](https://support.google.com/mail/answer/75726?hl=en)
- [Sign in with app passwords](https://support.google.com/mail/answer/185833?hl=en)
