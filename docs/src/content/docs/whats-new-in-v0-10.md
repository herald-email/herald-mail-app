---
title: What's New in v0.10
description: Secure Google OAuth credentials, explicit AI and MCP sharing consent, subject rules, and local image styles in v0.10.0-beta.1.
---

`v0.10.0-beta.1` protects Google OAuth credentials with secure storage and requires separate consent before AI, background processing, or MCP can access private account data. It also adds subject-based automation rules and local image styles.

## Release Delta

These changes are new since `v0.9.0-beta.1`. Review the upgrade notes before restarting existing AI or headless integrations.

- [x] Protect desktop Google authorization with S256 PKCE and store OAuth tokens in the operating system credential store instead of configuration YAML.
- [x] Review chat, embedding, and agent destinations in Settings → AI, then separately allow AI sharing, background processing, and MCP access. Existing configurations start without grants.
- [x] Block subsequent requests after consent is revoked or account identities or recipients change. Requests already dispatched cannot be recalled.
- [x] Match automation rules against email subjects with case-insensitive Unicode matching and decoded MIME subjects. Blank subject conditions do not match mail.
- [x] Apply cached local CRT and analog image styles, including Gloom CRT, without sending images to a remote service.
- [x] Update the default OpenAI model to `gpt-5.6-terra` while preserving explicitly chosen alternatives.

## Beta Notes

This is the first beta on the v0.10 line. Secure storage and sharing permissions change startup and integration behavior for existing users.

- [x] `v0.10.0-beta.1` migrates inline Google tokens only after validation and successful secure storage. A locked or unavailable store fails closed; no plaintext fallback is used.
- [x] macOS uses Keychain, Windows uses Credential Manager, and Linux uses Secret Service. Headless hosts without a credential service require a stable `HERALD_TOKEN_STORE_KEY`: a base64-encoded random 32-byte key injected from a secret manager into every process using the config. Encrypted token files live beside the config in `<config-path>.tokens/`; retain the key separately and back up both appropriately.
- [x] Review and explicitly enable required sharing permissions in Settings → AI before using existing AI features, background indexing, or a headless MCP client. Local AI providers also require consent. Editing AI settings resets the choices to No for review.
- [x] Token storage does not encrypt the mail cache or AI API keys, erase old config backups, or automatically remove unused keyring entries. Moving a keyring-backed config to another machine requires reauthorization or deliberate credential migration.
- [x] Native release archives include TUI, MCP, and SSH binaries for macOS and Linux on arm64 and amd64. Linux packages require glibc 2.39 or newer.

## How To Try It

Upgrade through Homebrew and check the installed version. For the verification demonstration, reconnect the dedicated test account and use synthetic mail and events.

```sh
brew update
brew upgrade herald
herald --version
```

- [x] Open Settings → AI to review destinations and grant only the sharing permissions you intend to use.
- [x] Use `herald --demo` to explore the interface with synthetic data and without granting access to a real mailbox.

## Previous Release

The v0.9 line introduced external mailbox password commands and reliable account-title updates. See [What's New in v0.9](/whats-new-in-v0-9/) for that release history.
