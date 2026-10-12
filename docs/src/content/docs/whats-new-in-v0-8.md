---
title: What's New in v0.8
description: Sending identities, selected-message folder moves, Linux packages, and connection recovery in v0.8.0-beta.1.
---

`v0.8.0-beta.1` adds account-specific sending identities, a folder picker for moving selected mail, and native Linux release packages. It also improves IMAP connection recovery and fixes Calendar setup and archive progress messages.

## Release Delta

These changes are new since `v0.7.5-beta.1`. Existing accounts continue to use their primary sending address unless you choose an identity or change the account default.

- [x] Manage sending identities in Settings → Accounts → edit an account → **Edit sending identities**, below the **Include Google Calendar** block when present. Add, edit, remove, and choose a default identity, then save to apply the changes.
- [x] Choose or search the **From** address in Compose. Replies recognize your configured identities, Reply All excludes your own aliases, and reopened drafts retain their sending identity.
- [x] Move highlighted or selected received mail with `M`, search for a destination folder, and press Enter to move. Moves stay within one account and report partial failures without losing failed selections.
- [x] Install native Linux x86_64 and arm64 packages through Homebrew or GitHub release archives, alongside Apple Silicon and Intel macOS packages. Linux binaries require glibc 2.39 or newer.
- [x] Daemon IMAP reads reconnect on demand and recover from expired authentication sessions. Confirmed missing messages are removed from the local cache without treating ordinary fetch errors as deletions.
- [x] Adding or editing a CalDAV account no longer starts Google OAuth authorization.
- [x] Archive progress says **Archiving** while queued or reconnecting, and Compose AI review keeps its controls visible in smaller terminals.

## Beta Notes

This is the first beta on the v0.8 line. Sending identities use the selected account's existing mail connection; your provider must already permit each alias you use.

- [x] `v0.8.0-beta.1` includes sending identities (#74), selected-mail folder moves (#68), Linux Homebrew packaging (#70), CalDAV authorization fixes (#71), archive progress wording (#69), and IMAP lifecycle recovery (#76).
- [x] Release archives include `herald`, `herald-mcp-server`, and `herald-ssh-server` for macOS and Linux on arm64 and amd64.
- [x] The release workflow updates the `beta-latest` channel and Homebrew tap from the immutable versioned release.

## How To Try It

Upgrade an existing Homebrew installation, then explore folder moves with demo mail. Open your normal configured session to manage sending identities for a real account.

```sh
brew update
brew upgrade herald
herald --demo
```

Select mail with Space and press `M` to open the folder picker. For identities, run `herald`, press `S`, open Accounts, edit your account, and choose **Edit sending identities**. After saving, open Compose with `c` and press Shift+Tab from To to select From. See [Compose](/using-herald/compose/), [Timeline](/using-herald/timeline/), and [installation instructions](/getting-started/) for details.

## Previous Release

The v0.7 line introduced Herald Memories, Compose Radar, expanded chat capabilities, and reading improvements. See [What's New in v0.7](/whats-new-in-v0-7/) for that release history.
