---
title: First-run Wizard
description: Configure Herald the first time it starts without an existing config file.
---

The first-run wizard appears when Herald cannot find a usable config at the default path or at the path passed with `-config`. It creates the account, server, SMTP, sync, cleanup, and AI settings that the normal TUI uses afterward.

## Overview

Use the wizard to choose an account type, enter credentials, validate the account immediately, configure optional preferences, and save `conf.yaml`. The default first-run wizard focuses on IMAP-based setup: Standard IMAP, Gmail with an App Password, Proton Mail Bridge, Fastmail, iCloud, and Outlook. Gmail OAuth is experimental and appears only when Herald starts with `-experimental`.

<!-- HERALD_SCREENSHOT id="wizard-account-type" page="first-run-wizard" alt="First-run wizard account type selection" state="fresh config, 120x40" desc="Shows the default account type choices including standard IMAP, Gmail IMAP App Password, Proton Mail Bridge, Fastmail, iCloud, and Outlook; Gmail OAuth is hidden unless launched with -experimental." capture="tmux demo 120x40; launch ./bin/herald -config /tmp/herald-new.yaml" -->

![First-run wizard account type selection](/screenshots/wizard-account-type.png)

## Screen Anatomy

The wizard replaces the normal tabbed interface until it completes or is cancelled.

| Area | What it shows |
| --- | --- |
| Account type | A provider or generic IMAP path. Presets fill host and port values but do not invent credentials. |
| Identity fields | Email address, username, and password or app password, depending on the selected provider. |
| Server fields | IMAP host and port, SMTP host and port, and optional advanced overrides. |
| Account connection | Validates IMAP and SMTP after account details, before optional AI, sync, theme, keyboard, or signature steps. |
| AI provider | Ollama local, Ollama custom host, Claude API, OpenAI-compatible API, or disabled. |
| Sync and cleanup | Poll interval in minutes, IMAP IDLE toggle, and cleanup schedule in hours. |
| Review/save step | Writes the config file only after the account has already passed validation, then launches the main Herald UI. |

## Controls

| Control | Preconditions | Result |
| --- | --- | --- |
| `tab` / form navigation | A wizard field is active. | Moves to the next field according to the form component. |
| `enter` | Current field is valid or a choice is highlighted. | Accepts the value or advances the wizard. |
| `esc` | Wizard is active. | Cancels setup; in first-run mode this exits the setup path instead of opening a half-configured mailbox. |
| `ctrl+c` | Any wizard state. | Quits Herald. |

## Workflows

1. Install Herald with Homebrew or build it from source.
2. Run `herald`, or use `./bin/herald` from a source checkout. Pass an explicit path with `herald -config ~/.herald/conf.yaml` when needed.
3. Choose the provider path that matches your account.
4. Enter the provider credentials. For Gmail IMAP and iCloud, use an app password when required by the provider. If you launched with `-experimental` and selected Gmail OAuth, complete browser authorization instead.
5. Herald validates IMAP and SMTP before showing the optional preference steps. If either check fails, the config file is not written and the wizard returns you to account setup.
6. Pick optional AI, sync, keyboard, theme, and signature settings. Choose disabled if you only want mail sync, reading, composing, and cleanup.
7. Review the remaining settings and save.
8. After the final save, Herald writes the validated config, creates or opens the SQLite cache, processes new mail, and opens the Timeline tab.

## Offline Cache Policy

The Sync and cleanup step asks how much fetched preview data Herald should keep. New configs default to `Message bodies without attachments`, which lets background preview loading support offline reading without keeping downloadable attachment bytes. `Lightweight previews` keeps preview text, headers, and attachment metadata only. `Full offline archive` keeps fetched preview data including attachment bytes, using more disk and keeping more mail data locally.

## Provider Choices

| Choice | Use when | Notes |
| --- | --- | --- |
| Standard IMAP | You know your IMAP and SMTP host and port. | Most portable path. |
| Gmail IMAP + App Password | You use personal Gmail with 2-Step Verification and an app password. | Normal Gmail path while OAuth onboarding is experimental. |
| Proton Mail Bridge | You run Proton Mail Bridge locally. | Uses Bridge host, ports, username, and password. |
| Fastmail, iCloud, Outlook | You want preset host/port values. | IMAP presets; provider app passwords may still be required. |
| Gmail OAuth (Experimental) | You launched with `-experimental` and want browser-based Gmail authorization. | Hidden by default; stores refresh token data only after IMAP and SMTP XOAUTH2 validation pass. |

## Data And Privacy

The wizard keeps credentials, app passwords, OAuth refresh tokens, host names, and ports in memory until account validation passes, then writes them to the selected YAML file only when you finish the remaining setup. Treat that file as a credential store and use `chmod 600` on Unix-like systems.

If you configure Ollama, AI requests stay local to the Ollama host. If you configure Claude or an OpenAI-compatible provider, later AI features may send selected email context to that provider when you invoke them.

## Troubleshooting

If the wizard reappears every time, verify that Herald is reading the config path you expect and that the file was saved successfully.

If validation fails, read which check failed. IMAP failures usually mean the host, port, username, password, app password, OAuth grant, or provider IMAP setting is wrong. SMTP failures usually mean the SMTP host, port, username, password, app password, OAuth grant, or provider send setting is wrong. Some providers use different credentials for IMAP and SMTP.

If Gmail OAuth is missing from the first-run choices, relaunch with `-experimental`. On Google's test-app warning page, choose `Continue` to reach the real consent screen; `Back to safety` does not authorize Herald. If you choose `Cancel` on the consent screen, Herald reports that authorization was cancelled and does not save settings. See [Settings](/features/settings/) for the OAuth wait overlay behavior.

If OAuth fails immediately with `Google OAuth credentials are not configured`, you are probably using a source-built development binary without build-time or runtime OAuth credentials. Homebrew and release binaries include OAuth defaults for the experimental path. For source builds, export `HERALD_GOOGLE_CLIENT_ID` and `HERALD_GOOGLE_CLIENT_SECRET` before launching `herald -experimental`, or place both values in `.herald-dev.env` before running `make build`. Release-style local builds use `.herald-release.env` with `make build-release-local` and fail early if either value is missing. See [Local OAuth Builds](/development/local-oauth-builds/) for the full source-build contract.

## Related Pages

- [Provider Setup](/provider-setup/)
- [Gmail Setup](/gmail-setup/)
- [Local OAuth Builds](/development/local-oauth-builds/)
- [Custom IMAP](/custom-imap/)
- [Config Reference](/reference/config/)
