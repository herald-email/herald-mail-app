---
title: Provider Setup
description: Choose and configure the mail provider path that Herald should use.
---

Herald talks to mail providers through provider-specific mail sources. Gmail OAuth uses the Gmail API for core reads, drafts, mailbox mutations, and send, while Gmail App Password and other credential paths use IMAP for reading and SMTP for sending.

## Overview

Choose the narrowest supported path that matches your account. Gmail users should start with Gmail OAuth and use Gmail IMAP with an App Password only as a fallback. Proton Mail users should run Proton Mail Bridge and use Bridge-generated credentials; other providers can use standard IMAP/SMTP settings or an IMAP preset. First-run setup validates the selected provider access immediately after account details; in-app account settings validate before saving or applying account changes.

## Choose A Provider Path

Use these decision cards first, then check the detailed provider and calendar matrices below when you need exact transports, hosts, or credential types. Mail setup and calendar setup are both first-class choices: Gmail OAuth can cover Google mail and optional Google Calendar together, while standalone calendars can be added later from Settings.

### Gmail OAuth (recommended)

- Choose this if: you use Gmail or Google Workspace and installed Herald with Homebrew or another release binary.
- You need: browser consent for Google access, with Mail enabled and optional Google Calendar access.
- Expected setup time: a few minutes.
- Common failure: a source build is missing OAuth defaults, or the browser grant was cancelled or expired.
- Where to fix it: [Local OAuth Builds](/development/local-oauth-builds/) for source-build credentials, or [Gmail Setup](/gmail-setup/) for the Gmail flow.

### Gmail With An App Password (fallback)

- Choose this if: you use Gmail but cannot use the browser OAuth path.
- You need: 2-Step Verification enabled on the Google account and a Google App Password for Herald.
- Expected setup time: a few minutes after the App Password exists.
- Common failure: the App Password was copied incorrectly, IMAP is blocked, or a Google Workspace admin requires OAuth.
- Where to fix it: [Gmail Setup](/gmail-setup/) and the Google account or Workspace admin settings.

### Proton Mail Bridge

- Choose this if: you use Proton Mail and Proton Mail Bridge is running locally.
- You need: Bridge-generated IMAP and SMTP username/password values, plus the local Bridge host and ports.
- Expected setup time: a few minutes after Bridge is installed and signed in.
- Common failure: Bridge is not running, the wrong Bridge password was copied, or the local Bridge ports changed.
- Where to fix it: Proton Mail Bridge, then Herald's Proton preset or [Custom IMAP](/custom-imap/) fields.

### Fastmail, iCloud, Outlook, Or Another IMAP Preset

- Choose this if: your provider appears in Herald's account chooser and you want Herald to prefill common host and port values.
- You need: the provider username and password, app password, or app-specific password required by that provider.
- Expected setup time: one or two minutes once the provider credential is ready.
- Common failure: the provider requires an app-specific password or has IMAP/SMTP access disabled.
- Where to fix it: the provider account settings, then Herald's provider preset or [Custom IMAP](/custom-imap/).

### Custom IMAP

- Choose this if: your mail provider is not a preset or you need custom IMAP/SMTP host, port, or credential values.
- You need: IMAP and SMTP host names, ports, encryption expectations, username, and provider-specific password.
- Expected setup time: a few minutes if the provider's mail settings are available.
- Common failure: wrong SSL/TLS port, wrong SMTP auth setting, or a provider that requires app-password setup first.
- Where to fix it: [Custom IMAP](/custom-imap/) and your provider's mail client setup documentation.

### Google Calendar OAuth

- Choose this if: you want Google Calendar in Herald, either alongside Gmail OAuth or as a standalone calendar source.
- You need: browser consent for Google Calendar access.
- Expected setup time: a few minutes.
- Common failure: a source build is missing OAuth defaults, or the browser grant did not include the selected calendar access.
- Where to fix it: [Local OAuth Builds](/development/local-oauth-builds/) for source-build credentials, or `Settings > Accounts` to retry the Google Calendar source.

### Fastmail, iCloud, Yahoo, Or Custom CalDAV

- Choose this if: you want to add a non-Google calendar that supports CalDAV.
- You need: the CalDAV URL, username, and provider-specific app password or app-generated password.
- Expected setup time: a few minutes after the provider credential exists.
- Common failure: the calendar URL is wrong, the provider password is not an app password, or the selected calendar is read-only.
- Where to fix it: the provider-specific calendar references below, then `Settings > Accounts > Add calendar only`.

Microsoft Calendar and Proton Calendar are not basic CalDAV presets in Herald. Microsoft Calendar work uses Microsoft Graph/OAuth or read-only ICS subscription paths when supported, and Proton Calendar uses ICS import, export, and subscription flows rather than a username/password CalDAV preset.

## Mail Source Matrix

| Provider path | Read transport | Send transport | Credential type |
| --- | --- | --- | --- |
| Gmail OAuth | Gmail API | Gmail API | Browser OAuth |
| Gmail IMAP | `imap.gmail.com:993` | `smtp.gmail.com:587` | Google App Password |
| Proton Mail Bridge | `127.0.0.1:1143` | `127.0.0.1:1025` | Bridge-generated username and password |
| Fastmail | `imap.fastmail.com:993` | `smtp.fastmail.com:587` | Provider password or app password |
| iCloud | `imap.mail.me.com:993` | `smtp.mail.me.com:587` | App-specific password |
| Outlook | `outlook.office365.com:993` | `smtp.office365.com:587` | Provider-supported IMAP credential |
| Custom IMAP | Your provider value | Your provider value | Provider-specific |

## Calendar Source Matrix

Herald can add standalone calendar sources from `Settings > Accounts > Add calendar only`. Google Calendar uses Herald's supported OAuth path, while CalDAV providers use a URL, username, and provider-specific password. Gmail OAuth can also add Gmail and Google Calendar together from `Settings > Accounts > Add account`.

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

1. Open `Settings > Accounts > Add calendar only`.
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

1. Open `Settings > Accounts > Add calendar only`.
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
