---
title: Settings
description: Edit Herald account, server, AI, sync, cleanup, and OAuth settings from the TUI.
---

Settings is a compact centered overlay opened from the main TUI. It starts with a top-level menu for `Account setup`, `AI`, `Sync & Cleanup`, and `Signature`, so you can adjust one area without stepping through unrelated fields while the current Herald screen remains visible behind it.

## Overview

Press `S` from the main UI to open settings. The overlay reads the current config, opens the selected category, writes the config path when that category is saved, updates supported runtime state, and can trigger OAuth wait behavior for supported OAuth flows.

## Screen Anatomy

| Area | What it shows |
| --- | --- |
| Settings menu | `Account setup`, `AI`, `Sync & Cleanup`, and `Signature` category choices. |
| Account setup | Vendor, username, password/app password, IMAP, SMTP, and provider-specific options. |
| IMAP fields | Host, port, and related server settings. |
| SMTP fields | Host, port, and send settings. |
| AI | Ollama local/custom, Claude, OpenAI-compatible, disabled, chat/classification model, and embedding model fields. |
| Sync & Cleanup | Poll interval minutes, IMAP IDLE setting, cleanup schedule hours, and related automation timing. |
| Signature | Multiline default Compose signature text. |
| Save/cancel controls | Category-level save returns to the menu; `esc` backs out one level before exiting and does not save unsaved category edits. |
| OAuth wait overlay | URL/open-browser state while waiting for OAuth callback and token storage. |

<!-- HERALD_SCREENSHOT id="settings-main-panel" page="settings" alt="Settings overlay open" state="demo mode, 120x40, settings overlay active" desc="Shows compact centered settings form fields for provider, server, SMTP, AI, sync, cleanup, and save/cancel affordances over the current Herald view." capture="tmux demo 120x40; ./bin/herald --demo; press S" -->

![Settings overlay open](/screenshots/settings-main-panel.png)

## Controls

| Key | Context | Preconditions | Result |
| --- | --- | --- | --- |
| `S` | Main UI | Settings closed. | Opens settings as a compact centered overlay. |
| `/` | Settings menu | Category menu is focused. | Opens the category filter prompt. |
| `enter` | Settings menu | A category is highlighted. | Opens that settings category. |
| `tab` | Settings category form | A form field is active. | Moves through form controls according to the form component. |
| `enter` | Settings category form | Current field or form action is valid. | Accepts selection, advances, or saves when on final action. |
| `esc` | Settings menu filter | Filter is active or applied. | Exits filter entry or clears the applied filter before panel close. |
| `esc` | Settings category form | No field-local escape action is active. | Returns to the top-level settings menu without saving unsaved category edits. |
| `esc` | Settings menu | No filter is active. | Exits Settings and returns to the underlying screen. |
| `enter` | OAuth wait overlay | OAuth URL available. | Opens browser to the authorization URL. |
| `ctrl+c` | Any settings state | Any state. | Quits Herald. |

## Workflows

### Open and Save Settings

1. Press `S`.
2. Choose `Account setup`, `AI`, `Sync & Cleanup`, or `Signature`.
3. Update the fields in that category.
4. Save the category.
5. Herald writes config, applies runtime changes that can be applied immediately, and returns to the settings menu.

### Change AI Provider

1. Press `S`.
2. Choose `AI`.
3. Choose an AI provider.
4. Enter host, model, API key, or compatible base URL as required.
5. Save.
6. Watch the AI status chip after returning to the main UI.

### Start OAuth

1. Choose an OAuth-capable provider path.
2. In first-run onboarding, launch Herald with `-experimental` first; in the in-app settings overlay, choose the OAuth path directly.
3. Confirm you are using Homebrew/a release binary with OAuth defaults, or that your shell has `HERALD_GOOGLE_CLIENT_ID` and `HERALD_GOOGLE_CLIENT_SECRET` exported.
4. In the OAuth wait overlay, press `enter` to open the browser.
5. Complete provider consent.
6. Wait for Herald to save token data and return to the app.

## States

| State | What happens |
| --- | --- |
| Menu mode | Settings shows the top-level category menu and any saved/error notice from the last category save. Menu hints show `enter open`, `esc exit`, and bottom-screen Settings hints. |
| Menu filter | `/` filters the category menu; `esc exit filter` leaves filter entry, `esc clear filter` clears applied text, and only the next `esc` closes Settings. |
| Category mode | Settings shows only the selected category's fields plus save/cancel controls. `Esc` returns to the menu first; the next menu-level `Esc` exits. |
| Overlay mode | Settings appears over the current screen at supported sizes; at `80x24` it fits inside the viewport, and at `50x15` the standard minimum-size guard appears instead of a clipped form. |
| First-run mode | Settings/wizard completion is required before the main mailbox opens. |
| OAuth waiting | Herald shows authorization URL state and waits for callback. |
| OAuth saved | Token data is written to config. |
| AI model changed | Herald may reset embeddings so stale vectors do not mix with a new embedding model. |
| Config permission warning | Startup warns if group/other users can read the config file. |
| Save error | The panel reports an error if config cannot be written. |

## Data And Privacy

Settings reads and writes credentials, app passwords, OAuth tokens, server hosts, SMTP settings, AI provider keys, model names, sync options, cleanup schedule, and cache path values. OAuth refresh tokens and external AI keys should be treated as credentials.

## Troubleshooting

If settings will not save, check file permissions for the config path and parent directory.

If OAuth does not complete, copy the displayed URL into a browser, finish consent, and confirm the callback server is reachable.

If OAuth is missing from first-run onboarding, relaunch with `-experimental`. If OAuth fails before showing a URL with `Google OAuth credentials are not configured`, use Homebrew or another release binary, export the two `HERALD_GOOGLE_*` variables before starting Herald, or build locally with `make build-release-local`. A plain `make build` binary does not embed `.herald-release.env`.

If AI stops working after model changes, verify the new model is installed or reachable and allow embedding regeneration to complete.

## Screenshot Placeholders

<!-- HERALD_SCREENSHOT id="settings-oauth-wait" page="settings" alt="OAuth wait overlay" state="live OAuth setup, 120x40, OAuth wait active" desc="Shows OAuth authorization URL, enter-to-open behavior, waiting status, and cancellation path." capture="tmux live 120x40; launch Gmail OAuth setup; start OAuth flow" deferred="true" reason="requires live OAuth setup" -->

<!-- HERALD_SCREENSHOT id="settings-ai-provider" page="settings" alt="Settings AI provider selection" state="demo mode, 120x40, settings AI section focused" desc="Shows AI provider choices and model fields used for classification, chat, quick replies, and embeddings." capture="tmux demo 120x40; ./bin/herald --demo; press S; navigate to AI provider section" -->

![Settings AI provider selection](/screenshots/settings-ai-provider.png)

## Related Pages

- [First-run Wizard](/first-run-wizard/)
- [Provider Setup](/provider-setup/)
- [AI Features](/features/ai/)
- [Sync and Status](/features/sync-status/)
- [Config Reference](/reference/config/)
