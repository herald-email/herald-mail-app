---
title: What's New in v0.1
description: User-facing changes across the v0.1 beta release line from v0.1.0 through v0.1.6.
---

The v0.1 beta line was Herald's first public release series. It established the macOS beta packaging path, made demo mode useful for trying Herald without an inbox, and rapidly tightened the early Timeline, Compose, cleanup, image-preview, MCP, and SSH workflows.

## Release Delta

These are the changes most visible to people looking back at the v0.1 beta line. The theme of the release is turning a working terminal mail client into something easier to install, safer to explore, and better prepared for later v0.2-v0.6 feature work.

- [x] `v0.1.0-beta.1` shipped the first macOS beta artifacts, including the Herald TUI, MCP server, SSH server, docs site, release workflow, checksums, and release-asset refresh.
- [x] `v0.1.1-beta.1` added Homebrew tap automation so macOS testers could install release-built binaries from `herald-email/homebrew-herald`.
- [x] `v0.1.2-beta.1` promoted Gmail IMAP/App Password onboarding, kept Gmail OAuth experimental, and fixed cache database paths so launcher context did not scatter local cache files.
- [x] `v0.1.3-beta.1` rendered email links with OSC 8 labels in supported terminals and clarified that Gmail OAuth was still an experimental onboarding path.
- [x] `v0.1.4-beta.1` added mouse navigation, OSC 8 support docs, and the compose-safe keybinding command layer that kept printable text from being swallowed by global shortcuts.
- [x] `v0.1.4-beta.5` was the broad usability patch: timeline bulk selection, draft workflows, attachment path autocomplete, safer attachment downloads, HTML-preserving replies and forwards, full-screen image previews, read-state cache sync, demo compose sending, and stronger MCP daemon readiness.
- [x] `v0.1.5-beta.1` introduced the canonical `herald` CLI with `mcp` and `ssh` subcommands, context-sensitive shortcut help, shared Markdown-aware previews, Kitty and iTerm2 image rendering work, and refreshed install/command docs.
- [x] `v0.1.6-beta.1` migrated the TUI to Bubble Tea v2, added horizontal timeline reading, fixed reply/draft/cleanup refresh issues, and codified the autopilot evidence gates used to harden later UI work.

## Beta Timeline

The v0.1 line moved quickly, so the patch-level release notes are more useful as a historical trail than as everyday upgrade guidance. These links go to the enriched GitHub release bodies for each immutable tag.

| Release | Published | Primary focus |
| --- | --- | --- |
| [`v0.1.0-beta.1`](https://github.com/herald-email/herald-mail-app/releases/tag/v0.1.0-beta.1) | April 27, 2026 | First public macOS beta artifacts and docs site. |
| [`v0.1.1-beta.1`](https://github.com/herald-email/herald-mail-app/releases/tag/v0.1.1-beta.1) | April 27, 2026 | Homebrew tap automation. |
| [`v0.1.2-beta.1`](https://github.com/herald-email/herald-mail-app/releases/tag/v0.1.2-beta.1) | April 27, 2026 | Gmail IMAP onboarding and cache path fix. |
| [`v0.1.3-beta.1`](https://github.com/herald-email/herald-mail-app/releases/tag/v0.1.3-beta.1) | April 27, 2026 | OSC 8 link labels and OAuth experiment labeling. |
| [`v0.1.4-beta.1`](https://github.com/herald-email/herald-mail-app/releases/tag/v0.1.4-beta.1) | April 28, 2026 | Mouse navigation and compose-safe key routing. |
| [`v0.1.4-beta.5`](https://github.com/herald-email/herald-mail-app/releases/tag/v0.1.4-beta.5) | April 29, 2026 | Timeline, drafts, attachments, previews, and MCP hardening. |
| [`v0.1.5-beta.1`](https://github.com/herald-email/herald-mail-app/releases/tag/v0.1.5-beta.1) | April 29, 2026 | Canonical CLI, shortcut help, and image protocol work. |
| [`v0.1.6-beta.1`](https://github.com/herald-email/herald-mail-app/releases/tag/v0.1.6-beta.1) | May 1, 2026 | Bubble Tea v2, horizontal reading, and UI evidence gates. |

## How To Try It

Use a current release for everyday work. To inspect the final historical v0.1 line, check out the immutable tag and run demo mode:

```sh
git checkout v0.1.6-beta.1
make build
./bin/herald --demo
```

The v0.1 tags predate the current Calendar and Gmail API source platform. Treat them as historical beta snapshots, not recommended daily builds.

## Next Release

The v0.2 release line continued the setup, command, and demo hardening work after v0.1. The next docs archive page currently starts at [What's New in v0.4](/whats-new-in-v0-4/), where the theme system and Compose AI polish became the main user-facing story.
