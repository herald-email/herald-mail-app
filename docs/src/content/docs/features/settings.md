---
title: Settings
description: Edit Herald account, server, AI, sync, cleanup, keyboard, theme, and OAuth settings from the TUI.
---

Settings is a compact centered overlay opened from the main TUI. It starts with a top-level menu for `Account setup`, `AI`, `Sync & Cleanup`, `Keyboard`, `Theme`, and `Signature`, so you can adjust one area without stepping through unrelated fields while the current Herald screen remains visible behind it.

## Overview

Press `S` from the main UI to open settings. The overlay reads the current config, opens the selected category, writes the config path when that category is saved, updates supported runtime state, and can trigger OAuth wait behavior for supported OAuth flows.

## Screen Anatomy

| Area | What it shows |
| --- | --- |
| Settings menu | `Account setup`, `AI`, `Sync & Cleanup`, `Keyboard`, `Theme Selection`, `Theme Editor`, and `Signature` category choices. |
| Account setup | Vendor, username, password/app password, IMAP, SMTP, and provider-specific options. |
| IMAP fields | Host, port, and related server settings. |
| SMTP fields | Host, port, and send settings. |
| AI | Ollama local/custom, Claude, OpenAI-compatible, disabled, chat/classification model, and embedding model fields. |
| Sync & Cleanup | Poll interval minutes, IMAP IDLE setting, offline cache policy, manual offline-cache reclaim, cleanup schedule hours, automation-rule launcher, custom-prompt launcher, and cleanup-rule manager launcher. |
| Keyboard | Keyboard profile and optional custom keymap YAML path. |
| Theme Selection | App theme selection and local YAML install. |
| Theme Editor | Semantic role color editing, live preview, reset controls, and custom theme creation. |
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
2. Choose `Account setup`, `AI`, `Sync & Cleanup`, `Keyboard`, `Theme Selection`, `Theme Editor`, or `Signature`.
3. Update the fields in that category.
4. Save the category.
5. Herald writes config, applies runtime changes that can be applied immediately, and returns to the settings menu.

### Reclaim Offline Cache Storage

1. Press `S`.
2. Choose `Sync & Cleanup`.
3. Enable `Reclaim offline cache storage`.
4. Save the category.
5. Review the before/after byte estimate and the preserved-data note.
6. Press `y` to prune disallowed cached preview bytes and compact SQLite, or press `n`/`Esc` to cancel.

### Manage Cleanup Automation

1. Press `S`.
2. Choose `Sync & Cleanup`.
3. Choose the automation-rule, custom-prompt, or cleanup-rule manager launcher.
4. Use the compact overlay to edit, preview, save, enable, or run the chosen rule surface.

### Choose Offline Cache Policy

The `Offline Cache` selector controls how much fetched preview data Herald keeps for later reading. New configs default to `Message bodies without attachments`, which lets background preview loading support offline reading without keeping downloadable attachment bytes. `Lightweight previews` keeps preview text, headers, and attachment metadata only. `Full offline archive` keeps fetched preview data including attachment bytes, using more disk and keeping more mail data locally.

### Change AI Provider

1. Press `S`.
2. Choose `AI`.
3. Choose an AI provider.
4. Enter host, model, API key, or compatible base URL as required.
5. Save.
6. Watch the AI status chip after returning to the main UI.

### Change Keyboard Profile

1. Press `S`.
2. Choose `Keyboard`.
3. Choose `Default`, `Vim`, `Emacs`, or `Custom YAML`.
4. For `Custom YAML`, enter the keymap file path.
5. Save.
6. Reopen shortcut help with `?` to verify the active profile shown in the overlay.

### Change Theme

Theme settings stay local-first while separating quick selection from deeper editing. Theme Selection switches built-in themes and installs validated YAML files from disk; Theme Editor edits semantic roles without adding a new global shortcut.

1. Press `S`.
2. Choose `Theme Selection`.
3. Choose `Inherited`, `Herald dark`, `Herald light`, a built-in palette such as `Jade Signal` or `Amber Furnace`, or an installed theme.
4. Optionally enter a local YAML theme file path to install it into `~/.herald/themes`.
5. Save.
6. Reopen Settings and choose `Theme Editor`.
7. Select a semantic role, edit foreground/background with `inherit`, `ansi:N`, `xterm:N`, or `#RRGGBB`, and review the swatches/live preview.
8. Use reset controls for one role or all overrides, or provide a new theme name to save the current overrides as a reusable local theme.
9. Save.

### Start OAuth

1. Choose an OAuth-capable provider path.
2. In first-run onboarding or the in-app settings overlay, choose the OAuth path directly.
3. Confirm you are using Homebrew/a release binary with OAuth defaults, a source build made with `.herald-dev.env` or exported Google OAuth variables, or a shell that has `HERALD_GOOGLE_CLIENT_ID` and `HERALD_GOOGLE_CLIENT_SECRET` exported. See [Local OAuth Builds](/development/local-oauth-builds/) for source-build options.
4. In the OAuth wait overlay, press `enter` to open the browser.
5. Complete provider consent.
6. Wait for Herald to validate IMAP and SMTP. First-run setup then continues to optional preferences; in-app account settings save token data and return to the app after validation succeeds.

## States

| State | What happens |
| --- | --- |
| Menu mode | Settings shows the top-level category menu and any saved/error notice from the last category save. Menu hints show `enter open`, `esc exit`, and bottom-screen Settings hints. |
| Menu filter | `/` filters the category menu; `esc exit filter` leaves filter entry, `esc clear filter` clears applied text, and only the next `esc` closes Settings. |
| Category mode | Settings shows only the selected category's fields plus save/cancel controls. `Esc` returns to the menu first; the next menu-level `Esc` exits. |
| Overlay mode | Settings appears over the current screen at supported sizes; at `80x24` it fits inside the viewport, and at `50x15` the standard minimum-size guard appears instead of a clipped form. |
| First-run mode | Account details are validated before optional preference steps, and wizard completion is required before the main mailbox opens. |
| Account validation | Account settings are held in memory while Herald checks IMAP and SMTP before saving or applying them. |
| OAuth waiting | Herald shows authorization URL state, waits for callback, and lets `Esc`/`q` cancel without saving settings. |
| OAuth cancelled or timed out | Herald shows a clear error, keeps previous settings active, and does not write token data. |
| OAuth saved | Token data is written to config only after OAuth, IMAP, SMTP validation, and final setup save succeed. |
| AI model changed | Herald may reset embeddings so stale vectors do not mix with a new embedding model. |
| Offline cache reclaim pending | Herald shows the current policy, before/after byte estimate, and note that preview text, headers, and attachment metadata stay cached before it accepts `y` to proceed. |
| Offline cache reclaimed | Herald reports rows pruned, bytes removed, and whether SQLite compaction completed. |
| Config permission warning | Startup warns if group/other users can read the config file. |
| Save error | The panel reports an error if config cannot be written. |

## Data And Privacy

Settings reads and writes credentials, app passwords, OAuth tokens, server hosts, SMTP settings, AI provider keys, model names, sync options, cleanup schedule, cleanup automation entrypoints, and cache path values. Account settings are validated before they replace the active account; failed validation leaves the previous config, backend, and SMTP client active. OAuth refresh tokens and external AI keys should be treated as credentials. Offline-cache reclaim removes only preview binary payloads that the current cache policy disallows; preview text, headers, unsubscribe data, and attachment metadata remain available.

## Troubleshooting

If settings will not save, check file permissions for the config path and parent directory.

If account validation fails, check the IMAP or SMTP section named in the error. Run `herald -debug` and open the latest file in `~/Library/Logs/Herald` on macOS, `${XDG_STATE_HOME:-~/.local/state}/herald/logs` on Linux/BSD, or `%LOCALAPPDATA%\\Herald\\Logs` on Windows.

If OAuth does not complete, copy the displayed URL into a browser and finish consent. If you choose `Cancel` on the consent screen, Herald reports that authorization was cancelled and does not save settings.

If OAuth fails before showing a URL with `Google OAuth credentials are not configured`, use Homebrew or another release binary, export the two `HERALD_GOOGLE_*` variables before starting Herald, or build locally after filling `.herald-dev.env`. Plain `make build` embeds those values when both are available and still succeeds without them; `make build-release-local` reads `.herald-release.env` and remains strict if either value is missing. See [Local OAuth Builds](/development/local-oauth-builds/) for the detailed local build paths.

If AI stops working after model changes, verify the new model is installed or reachable and allow embedding regeneration to complete.

Use `./bin/herald --demo -theme jade-signal` to preview a theme for one session without changing Settings.

## Screenshot Placeholders

<!-- HERALD_SCREENSHOT id="settings-oauth-wait" page="settings" alt="OAuth wait overlay" state="live OAuth setup, 120x40, OAuth wait active" desc="Shows OAuth authorization URL, enter-to-open behavior, waiting status, and cancellation path." capture="tmux live 120x40; launch Gmail OAuth setup; start OAuth flow" deferred="true" reason="requires live OAuth setup" -->

<!-- HERALD_SCREENSHOT id="settings-ai-provider" page="settings" alt="Settings AI provider selection" state="demo mode, 120x40, settings AI section focused" desc="Shows AI provider choices and model fields used for classification, chat, quick replies, and embeddings." capture="tmux demo 120x40; ./bin/herald --demo; press S; navigate to AI provider section" -->

![Settings AI provider selection](/screenshots/settings-ai-provider.png)

## Related Pages

- [First-run Wizard](/first-run-wizard/)
- [Provider Setup](/provider-setup/)
- [Local OAuth Builds](/development/local-oauth-builds/)
- [AI Features](/features/ai/)
- [Themes](/themes/)
- [Sync and Status](/features/sync-status/)
- [Config Reference](/reference/config/)
