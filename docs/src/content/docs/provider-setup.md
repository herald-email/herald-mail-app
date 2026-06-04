---
title: Provider Setup
description: Choose and configure the mail provider path that Herald should use.
---

Herald talks to mail providers through provider-specific mail sources. Gmail OAuth uses the Gmail API for core reads, drafts, mailbox mutations, and send, while Gmail App Password and other credential paths use IMAP for reading and SMTP for sending.

## Overview

Choose the narrowest supported path that matches your account. Gmail users should start with Gmail OAuth and use Gmail IMAP with an App Password only as a fallback. Proton Mail users should run Proton Mail Bridge and use Bridge-generated credentials; other providers can use standard IMAP/SMTP settings or an IMAP preset. First-run setup validates the selected provider access immediately after account details; in-app account settings validate before saving or applying account changes.

## Provider Matrix

| Provider path | Read transport | Send transport | Credential type |
| --- | --- | --- | --- |
| Gmail OAuth | Gmail API | Gmail API | Browser OAuth |
| Gmail IMAP | `imap.gmail.com:993` | `smtp.gmail.com:587` | Google App Password |
| Proton Mail Bridge | `127.0.0.1:1143` | `127.0.0.1:1025` | Bridge-generated username and password |
| Fastmail | `imap.fastmail.com:993` | `smtp.fastmail.com:587` | Provider password or app password |
| iCloud | `imap.mail.me.com:993` | `smtp.mail.me.com:587` | App-specific password |
| Outlook | `outlook.office365.com:993` | `smtp.office365.com:587` | Provider-supported IMAP credential |
| Custom IMAP | Your provider value | Your provider value | Provider-specific |

## Calendar Provider Matrix

Herald can add standalone calendar sources from `Settings > Accounts > Add account > Add Calendar`. Google Calendar uses Herald's supported OAuth path, while CalDAV providers use a URL, username, and provider-specific password.

| Provider path | Calendar URL or API | Credential type |
| --- | --- | --- |
| Google Calendar | Google Calendar OAuth/API | Browser OAuth |
| Fastmail Calendar | `https://caldav.fastmail.com/` | Fastmail app password |
| iCloud Calendar | `https://caldav.icloud.com/` | Apple app-specific password |
| Yahoo Calendar | `https://caldav.calendar.yahoo.com` | Yahoo app-generated password |
| Custom CalDAV | Your provider value | Provider-specific |

Microsoft Calendar is not shown as a CalDAV preset because the live integration path is Microsoft Graph/OAuth or read-only ICS subscription work, not a basic CalDAV username/password setup. Proton Calendar is also not shown as a live CalDAV preset; Proton documents ICS import, export, and subscription flows, and subscribed calendars are view-only.

## Workflows

### Gmail OAuth

1. Install with Homebrew or another release binary. For source builds, prepare OAuth defaults with [Local OAuth Builds](/development/local-oauth-builds/) first.
2. Run `herald`.
3. Choose `Gmail OAuth`.
4. Complete browser authorization and return to Herald.
5. Wait for Herald to validate the selected Google mail and optional calendar access before it continues to optional preferences.
6. Save the generated config and let Herald sync through the Gmail API mail source.

### Google Calendar OAuth

1. Open `Settings > Accounts > Add account > Add Calendar`.
2. Choose `Google Calendar`.
3. Enter the Google Calendar identity if you want Google to preselect an account.
4. Complete browser authorization and return to Herald.
5. Wait for Herald to validate the calendar source by listing calendars before it saves settings.

Do not use Google's CalDAV URL with an app password in Herald. Google Calendar requires OAuth for CalDAV access, and Herald's supported Google Calendar path uses the Google Calendar API plus the same desktop OAuth credentials as Gmail OAuth.

Source-built Google Calendar OAuth uses the same local build options as Gmail OAuth. See [Local OAuth Builds](/development/local-oauth-builds/) if OAuth fails before Herald shows an authorization URL.

### Gmail with an App Password

1. Enable 2-Step Verification on the Google account.
2. Create a Google App Password.
3. Run `herald` or `./bin/herald`.
4. Choose `Gmail (IMAP + App Password)`.
5. Enter the Gmail address and App Password.
6. Wait for Herald to validate Gmail IMAP and SMTP.
7. Save the generated config and let Herald sync.

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

### CalDAV calendars

1. Open `Settings > Accounts > Add account > Add Calendar`.
2. Choose Fastmail, iCloud, Yahoo, or Custom CalDAV.
3. Use the prefilled CalDAV URL or enter your provider's value.
4. Enter the provider username and app password or app-specific password shown in that provider's help.
5. Save and let Herald validate the source by listing calendars before writing the config.

Helpful references:

- [Fastmail server names and CalDAV settings](https://www.fastmail.help/hc/en-us/articles/1500000278342)
- [Apple app-specific passwords](https://support.apple.com/en-us/102654)
- [Yahoo Calendar CalDAV setup](https://ca.help.yahoo.com/kb/new-mail-for-desktop/sync-access-calendar-multiple-devices-applications-sln4707.html)
- [Yahoo app passwords](https://help.yahoo.com/kb/account/confirm-delete-password-sln15241.html)
- [Google Calendar CalDAV OAuth requirements](https://developers.google.com/workspace/calendar/caldav/v2/guide)
- [Microsoft Graph calendar events](https://learn.microsoft.com/en-us/graph/api/calendar-list-events?view=graph-rest-1.0)
- [Outlook ICS import and subscription](https://support.microsoft.com/en-us/office/import-or-subscribe-to-a-calendar-in-outlook-com-or-outlook-on-the-web-cff1429c-5af6-41ec-a5b4-74f2c278e98c)
- [Proton external calendar subscriptions](https://proton.me/support/subscribe-to-external-calendar)
- [Proton Calendar ICS import](https://proton.me/support/how-to-import-calendar-to-proton-calendar)
- [Proton Calendar ICS export](https://proton.me/support/protoncalendar-calendars)

## Data And Privacy

Provider credentials live in the Herald config file. Gmail OAuth stores refresh-token data and uses the Gmail API mail source for core mail operations. IMAP/App Password providers open IMAP connections, cache message metadata in SQLite, fetch body text when needed, and send outgoing messages over SMTP.

## Troubleshooting

If setup validation fails, fix the IMAP or SMTP section named in the error and try saving again. If an already configured mailbox later stays empty, verify the selected folder and check [Sync and Status](/features/sync-status/).

## Related Pages

- [First-run Wizard](/first-run-wizard/)
- [Gmail Setup](/gmail-setup/)
- [Local OAuth Builds](/development/local-oauth-builds/)
- [Custom IMAP](/custom-imap/)
- [Privacy and Security](/security-privacy/)
