---
title: Getting Started
description: Install Herald, run it for the first time, and understand the generated config.
---

This page covers the shortest route to a usable Herald session. For every visible screen after launch, continue with [Global UI](/using-herald/global-ui/) and the tab pages.

## Requirements

- An IMAP account and SMTP settings, unless you run demo mode
- No AI runtime is required for the core app. Mail sync, reading, compose, search, cleanup, Calendar, and settings work without Ollama or cloud model keys.
- Optional: Ollama for local semantic search, classification, chat, quick replies, and AI draft help. External AI providers are opt-in.
- Recommended: a modern terminal with mouse events and OSC 8 hyperlinks for clickable navigation and hardened email links. Common OSC 8-capable terminals include iTerm2, Kitty, WezTerm, GNOME Terminal and other VTE-based terminals, and Windows Terminal; see the [full OSC 8 adoption list](https://github.com/Alhadis/OSC8-Adoption/) for current compatibility. For inline image rendering, use a Kitty-protocol terminal such as Ghostty on macOS or Kitty itself; iTerm2 is also supported through its inline image protocol. Other terminals still get safe placeholders or local `open image` links when available.
- For source builds only: Go 1.25 or newer and a C compiler such as `clang` or `gcc` for SQLite CGO support

## macOS with Homebrew

```sh
brew tap herald-email/herald
brew install herald
herald
```

Homebrew installs the primary `herald` CLI. Use `herald mcp` for MCP and
`herald ssh` for SSH mode; the package also includes `herald-mcp-server` and
`herald-ssh-server` as compatibility wrappers for older configs and scripts.
Release builds include the Google OAuth defaults used by the recommended Gmail
and Google Calendar setup paths.

Update and upgrade:

```sh
brew update
brew upgrade herald
```

For a full tap reset:

```sh
brew uninstall herald
brew untap herald-email/herald
brew tap herald-email/herald
brew install herald
```

## Nightly builds

Nightly builds are short-lived GitHub Actions artifacts for testing the latest successful `main` build before the next beta tag. They are not signed releases or Homebrew packages; see [Nightly Builds](/nightly-builds/) for download steps and channel rules.

## Install from source with Go

```sh
go install github.com/herald-email/herald-mail-app/cmd/herald@latest
herald
```

That command installs the primary CLI binary as `herald`; its `mcp` and `ssh`
subcommands replace the older standalone command names for new setups. For a
local checkout:

```sh
git clone https://github.com/herald-email/herald-mail-app.git
cd herald-mail-app
make build
./bin/herald
```

If you need Gmail OAuth or Google Calendar OAuth from a source checkout, prepare local OAuth defaults before building or export runtime credentials. See [Local OAuth Builds](/development/local-oauth-builds/) for `.herald-dev.env`, runtime variables, and release-style local builds.

For development, you can also run:

```sh
make run
```

## First launch

Herald uses `~/.herald/conf.yaml` by default. If that file is missing or empty, Herald opens a first-run setup wizard.

The wizard recommends Gmail OAuth for Google accounts and can fill IMAP presets for common alternatives, including Gmail App Password, Proton Mail Bridge, Fastmail, iCloud, and Outlook. See [First-run Wizard](/first-run-wizard/) for the screen-by-screen details.

To explore safely before connecting accounts, run `herald --demo`, press or click `3 Calendar`, and try Week, Day, 3-Day, Agenda, Search, Event Detail, and the demo invitation import flow.

<!-- HERALD_SCREENSHOT id="getting-started-main-tui" page="getting-started" alt="Herald main interface after initial sync" state="demo mode, 120x40, Timeline tab active" desc="Shows the first usable Herald interface with tab bar, folder sidebar, Timeline list, status bar, and key hints." capture="tmux demo 120x40; ./bin/herald --demo; press 1" -->

![Herald main interface after initial sync](/screenshots/getting-started-main-tui.png)

## Useful flags

```sh
herald -help
herald -debug
herald -verbose
herald -config custom.yaml
herald -experimental
herald --demo
```

Use `./bin/herald` instead when running from a source checkout. `-experimental` remains available for future feature flags, but Gmail OAuth no longer requires it. `-debug` and `-verbose` both enable DEBUG-level file logging. Herald does not write logs to the terminal because that would corrupt the TUI.

## Example config

The `ollama:` block is optional and can be omitted until you enable local AI.

```yaml
credentials:
  username: "your@email.com"
  password: "your-password-or-app-password"
server:
  host: "imap.fastmail.com"
  port: 993
smtp:
  host: "smtp.fastmail.com"
  port: 587
ollama:
  host: "http://localhost:11434"
  model: "gemma3:4b"
  embedding_model: "nomic-embed-text-v2-moe"
```

Herald warns if the config file is readable by group or other users. Use `chmod 600 ~/.herald/conf.yaml` for credentials stored in YAML.

## Browser terminal option

Herald can run in a browser tab through `ttyd`:

```sh
brew install ttyd
ttyd -W herald
```

Open `http://localhost:7681`. The `-W` flag is required for keyboard input.

## What to read next

- [First 5 Minutes](/first-5-minutes/) for a short demo-mode loop before the full manual.
- [Demo Mode](/demo-mode/) if you want to explore without credentials.
- [Provider Setup](/provider-setup/) for provider presets and authentication choices.
- [Timeline](/using-herald/timeline/) for the default inbox workflow.
- [Calendar](/using-herald/calendar/) for the schedule workspace, demo calendar, and invitation import flow.
- [All Keybindings](/reference/keybindings/) for a compact command table.
