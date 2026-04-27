---
title: Settings
description: Edit Herald account, server, AI, sync, cleanup, and OAuth settings from the TUI.
---

Settings is a full-screen panel opened from the main TUI. It lets you adjust configuration without manually editing YAML for common account, AI, sync, and cleanup fields.

## Overview

Press `S` from the main UI to open settings. The panel reads the current config, lets you edit supported fields, writes the config path, updates AI provider details, and can trigger OAuth wait behavior for supported OAuth flows.

## Screen Anatomy

| Area | What it shows |
| --- | --- |
| Account/provider fields | Vendor, username, password/app password, and provider-specific options. |
| IMAP fields | Host, port, and related server settings. |
| SMTP fields | Host, port, and send settings. |
| AI provider fields | Ollama local/custom, Claude, OpenAI-compatible, or disabled. |
| AI model fields | Chat/classification model and embedding model. |
| Sync fields | Poll interval minutes and IMAP IDLE setting. |
| Cleanup fields | Cleanup schedule hours and related automation timing. |
| Save/cancel controls | Form-level completion or cancellation behavior. |
| OAuth wait overlay | URL/open-browser state while waiting for OAuth callback and token storage. |

<!-- HERALD_SCREENSHOT id="settings-main-panel" page="settings" alt="Settings panel open" state="demo mode, 120x40, settings panel active" desc="Shows settings form fields for provider, server, SMTP, AI, sync, cleanup, and save/cancel affordances." capture="tmux demo 120x40; ./bin/herald --demo; press S" -->

![Settings panel open](/screenshots/settings-main-panel.png)

## Controls

| Key | Context | Preconditions | Result |
| --- | --- | --- | --- |
| `S` | Main UI | Settings closed. | Opens settings panel. |
| `tab` | Settings form | A form field is active. | Moves through form controls according to the form component. |
| `enter` | Settings form | Current field or form action is valid. | Accepts selection, advances, or saves when on final action. |
| `esc` | Settings form | Settings active. | Cancels/closes panel when supported by current form state. |
| `enter` | OAuth wait overlay | OAuth URL available. | Opens browser to the authorization URL. |
| `ctrl+c` | Any settings state | Any state. | Quits Herald. |

## Workflows

### Open and Save Settings

1. Press `S`.
2. Move through fields with form navigation.
3. Update provider, server, SMTP, AI, sync, or cleanup fields.
4. Save the form.
5. Herald writes config and applies runtime changes that can be applied immediately.

### Change AI Provider

1. Press `S`.
2. Choose an AI provider.
3. Enter host, model, API key, or compatible base URL as required.
4. Save.
5. Watch the AI status chip after returning to the main UI.

### Start OAuth

1. Choose an OAuth-capable provider path.
2. Confirm you are using Homebrew/a release binary with OAuth defaults, or that your shell has `HERALD_GOOGLE_CLIENT_ID` and `HERALD_GOOGLE_CLIENT_SECRET` exported.
3. In the OAuth wait overlay, press `enter` to open the browser.
4. Complete provider consent.
5. Wait for Herald to save token data and return to the app.

## States

| State | What happens |
| --- | --- |
| Panel mode | Settings replaces the normal tabs until saved or cancelled. |
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

If OAuth fails before showing a URL with `Google OAuth credentials are not configured`, use Homebrew or another release binary, export the two `HERALD_GOOGLE_*` variables before starting Herald, or build locally with `make build-release-local`. A plain `make build` binary does not embed `.herald-release.env`.

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
