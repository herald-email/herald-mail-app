---
title: What's New in v0.11
description: Account sign-in recovery, polling across all accounts, and more reliable Gmail sync in v0.11.0-beta.1.
---

`v0.11.0-beta.1` keeps every configured account’s Inbox current and makes expired sign-ins easier to spot and repair. It also improves Gmail arrivals, consent settings, credential setup, and accidental-exit protection.

## Release Delta

These changes are new since `v0.10.0-beta.1`. They focus on reliable mail delivery and recovering an account without disrupting other accounts.

- [x] Poll every configured account’s Inbox in multi-account sessions, while continuing to poll the selected folder.
- [x] See account sign-in warnings in the interface and recover the affected account through Accounts. Google accounts can reauthorize; password-based accounts open credential settings.
- [x] Detect Gmail arrivals using history updates, including delayed mail and messages moved into a folder, and show new mail before reconciliation finishes.
- [x] Avoid repeated IMAP arrival notifications by tracking cached UIDs and overlapping the date search window.
- [x] Review AI, background processing, and MCP consent together, with the resulting on/off state shown before saving.
- [x] Configure a masked password command directly in account settings and setup.
- [x] Confirm exit with a second Ctrl+C within three seconds to reduce accidental quits.
- [x] Build on Fedora with the updated SQLite dependency.

## Beta Notes

This is the first beta on the v0.11 line. The secure token storage and explicit sharing permissions introduced in v0.10 continue to apply.

- [x] `v0.11.0-beta.1` uses polling for multi-account sessions so inactive accounts continue receiving mail.
- [x] Authentication recovery targets the affected account and its linked Google services; cancelling authorization preserves the existing configuration.
- [x] Native archives include Herald, MCP, and SSH binaries for macOS and Linux on arm64 and amd64. Linux packages require glibc 2.39 or newer.

## How To Try It

Upgrade through Homebrew, check the installed version, and use demo mode to explore the interface with synthetic data.

```sh
brew update
brew upgrade herald
herald --version
herald --demo
```

- [x] Open Accounts to inspect sign-in status and recover an account that needs authentication.
- [x] Review Settings → AI before enabling optional data sharing.

## Previous Release

The v0.10 line introduced secure Google OAuth token storage, sharing consent, subject rules, and local image styles. See [What's New in v0.10](/whats-new-in-v0-10/) for upgrade requirements.
