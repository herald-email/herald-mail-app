---
title: Provider Setup
description: Choose and configure the mail provider path that Herald should use.
---

Herald talks to mail providers through IMAP for reading and SMTP for sending. Provider setup is mostly about supplying the correct host, port, username, password, and optional OAuth or bridge details.

## Overview

Choose the narrowest supported path that matches your account. Gmail users on Homebrew or release binaries should prefer Gmail OAuth; Gmail IMAP with an App Password remains available as a fallback. Proton Mail users should run Proton Mail Bridge and use Bridge-generated credentials; other providers can use standard IMAP/SMTP settings or a preset.

## Provider Matrix

| Provider path | IMAP | SMTP | Credential type |
| --- | --- | --- | --- |
| Gmail OAuth | `imap.gmail.com:993` | `smtp.gmail.com:587` | Browser OAuth |
| Gmail IMAP | `imap.gmail.com:993` | `smtp.gmail.com:587` | Google App Password |
| Proton Mail Bridge | `127.0.0.1:1143` | `127.0.0.1:1025` | Bridge-generated username and password |
| Fastmail | `imap.fastmail.com:993` | `smtp.fastmail.com:587` | Provider password or app password |
| iCloud | `imap.mail.me.com:993` | `smtp.mail.me.com:587` | App-specific password |
| Outlook | `outlook.office365.com:993` | `smtp.office365.com:587` | Provider-supported IMAP credential |
| Custom IMAP | Your provider value | Your provider value | Provider-specific |

## Workflows

### Gmail OAuth

1. Install with Homebrew or another release binary.
2. Run `herald`.
3. Choose `Gmail OAuth`.
4. Complete browser authorization and return to Herald.
5. Save the generated config and let Herald sync.

### Gmail with an App Password

1. Enable 2-Step Verification on the Google account.
2. Create a Google App Password.
3. Run `herald` or `./bin/herald`.
4. Choose `Gmail (IMAP + App Password)`.
5. Enter the Gmail address and App Password.
6. Save the generated config and let Herald sync.

### Proton Mail Bridge

1. Start Proton Mail Bridge locally.
2. Copy the Bridge-generated IMAP and SMTP credentials.
3. Choose the Proton Mail Bridge preset or enter the host and ports manually.
4. Save and keep Bridge running while Herald is connected.

### Custom IMAP

1. Collect IMAP host/port and SMTP host/port from your provider.
2. Choose standard IMAP in the wizard or edit YAML directly.
3. Use provider-specific app passwords when required.
4. Launch Herald with `herald -config path/to/conf.yaml` or `./bin/herald -config path/to/conf.yaml` from a source checkout.

## Data And Privacy

Provider credentials live in the Herald config file. Herald opens an IMAP connection for the lifetime of the app, caches message metadata in SQLite, fetches body text when needed, and sends outgoing messages over SMTP.

## Troubleshooting

If IMAP works but send fails, the SMTP section is wrong or the provider requires a separate app password. If the mailbox stays empty, verify the selected folder and check [Sync and Status](/features/sync-status/).

## Related Pages

- [First-run Wizard](/first-run-wizard/)
- [Gmail Setup](/gmail-setup/)
- [Custom IMAP](/custom-imap/)
- [Privacy and Security](/security-privacy/)
