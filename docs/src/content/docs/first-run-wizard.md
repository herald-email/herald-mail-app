---
title: First-run Wizard
description: Configure Herald the first time it starts without an existing config file.
---

The first-run wizard appears when Herald cannot find a usable config at the default path or at the path passed with `-config`. It creates the account, server, SMTP, sync, cleanup, and AI settings that the normal TUI uses afterward.

## Overview

Use the wizard to choose an account type, enter credentials, configure optional AI, and save `conf.yaml`. The default first-run wizard focuses on IMAP-based setup: Standard IMAP, Gmail with an App Password, Proton Mail Bridge, Fastmail, iCloud, and Outlook. Gmail OAuth is experimental and appears only when Herald starts with `-experimental`.

<!-- HERALD_SCREENSHOT id="wizard-account-type" page="first-run-wizard" alt="First-run wizard account type selection" state="fresh config, 120x40" desc="Shows the default account type choices including standard IMAP, Gmail IMAP App Password, Proton Mail Bridge, Fastmail, iCloud, and Outlook; Gmail OAuth is hidden unless launched with -experimental." capture="tmux demo 120x40; launch ./bin/herald -config /tmp/herald-new.yaml" -->

![First-run wizard account type selection](/screenshots/wizard-account-type.png)

## Screen Anatomy

The wizard replaces the normal tabbed interface until it completes or is cancelled.

| Area | What it shows |
| --- | --- |
| Account type | A provider or generic IMAP path. Presets fill host and port values but do not invent credentials. |
| Identity fields | Email address, username, and password or app password, depending on the selected provider. |
| Server fields | IMAP host and port, SMTP host and port, and optional advanced overrides. |
| AI provider | Ollama local, Ollama custom host, Claude API, OpenAI-compatible API, or disabled. |
| Sync and cleanup | Poll interval in minutes, IMAP IDLE toggle, and cleanup schedule in hours. |
| Review/save step | Writes the config file and launches the main Herald UI. |

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
5. Pick an AI provider. Choose disabled if you only want mail sync, reading, composing, and cleanup.
6. Review the generated settings and save.
7. Herald connects to IMAP, creates or opens the SQLite cache, processes new mail, and opens the Timeline tab.

## Provider Choices

| Choice | Use when | Notes |
| --- | --- | --- |
| Standard IMAP | You know your IMAP and SMTP host and port. | Most portable path. |
| Gmail IMAP + App Password | You use personal Gmail with 2-Step Verification and an app password. | Normal Gmail path while OAuth onboarding is experimental. |
| Proton Mail Bridge | You run Proton Mail Bridge locally. | Uses Bridge host, ports, username, and password. |
| Fastmail, iCloud, Outlook | You want preset host/port values. | IMAP presets; provider app passwords may still be required. |
| Gmail OAuth (Experimental) | You launched with `-experimental` and want browser-based Gmail authorization. | Hidden by default; stores refresh token data in config. |

## Data And Privacy

The wizard writes credentials, app passwords, OAuth refresh tokens, AI provider keys, host names, ports, sync settings, and cache path configuration to the selected YAML file. Treat that file as a credential store and use `chmod 600` on Unix-like systems.

If you configure Ollama, AI requests stay local to the Ollama host. If you configure Claude or an OpenAI-compatible provider, later AI features may send selected email context to that provider when you invoke them.

## Troubleshooting

If the wizard reappears every time, verify that Herald is reading the config path you expect and that the file was saved successfully.

If a provider preset connects to IMAP but sending fails, check the SMTP host, port, username, and app password. Some providers use different credentials for IMAP and SMTP.

If Gmail OAuth is missing from the first-run choices, relaunch with `-experimental`. If OAuth stalls, confirm that the browser callback completes and that the config file can be written. See [Settings](/features/settings/) for the OAuth wait overlay behavior.

If OAuth fails immediately with `Google OAuth credentials are not configured`, you are probably using a source-built development binary. Homebrew and release binaries include OAuth defaults for the experimental path. For source builds, export `HERALD_GOOGLE_CLIENT_ID` and `HERALD_GOOGLE_CLIENT_SECRET` before launching `herald -experimental`, or rebuild with `make build-release-local` after filling `.herald-release.env`. Plain `make build` does not embed OAuth defaults.

## Related Pages

- [Provider Setup](/provider-setup/)
- [Gmail Setup](/gmail-setup/)
- [Custom IMAP](/custom-imap/)
- [Config Reference](/reference/config/)
