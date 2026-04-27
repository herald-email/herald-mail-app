---
title: Getting Started
description: Install Herald, run it for the first time, and understand the generated config.
---

This page covers the shortest route to a usable Herald session. For every visible screen after launch, continue with [Global UI](/using-herald/global-ui/) and the tab pages.

## Requirements

- An IMAP account and SMTP settings, unless you run demo mode
- Optional: Ollama for local AI features
- Recommended: a terminal with OSC 8 hyperlink support for Herald's hardened clickable link rendering. Popular supported terminals include iTerm2, Kitty, WezTerm, GNOME Terminal and other VTE-based terminals, and Windows Terminal; see the [full OSC 8 adoption list](https://github.com/Alhadis/OSC8-Adoption/).
- For source builds only: Go 1.25 or newer and a C compiler such as `clang` or `gcc` for SQLite CGO support

## macOS with Homebrew

```sh
brew tap herald-email/herald
brew install herald
herald
```

Homebrew installs the release binaries for `herald`, `herald-mcp-server`, and `herald-ssh-server`, including the Gmail OAuth defaults used only when experimental OAuth onboarding is enabled.

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

## Build from source

```sh
git clone https://github.com/herald-email/herald-mail-app.git
cd herald-mail-app
make build
./bin/herald
```

For development, you can also run:

```sh
make run
```

## First launch

Herald uses `~/.herald/conf.yaml` by default. If that file is missing or empty, Herald opens a first-run setup wizard.

The wizard can fill IMAP presets for common accounts, including Gmail, Proton Mail Bridge, Fastmail, iCloud, and Outlook. Gmail IMAP with an App Password is the normal Gmail path; Gmail OAuth is experimental and appears in first-run onboarding only when Herald starts with `-experimental`. See [First-run Wizard](/first-run-wizard/) for the screen-by-screen details.

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

Use `./bin/herald` instead when running from a source checkout. `-experimental` shows experimental first-run email service onboarding options such as Gmail OAuth. `-debug` and `-verbose` both enable DEBUG-level file logging. Herald does not write logs to the terminal because that would corrupt the TUI.

## Example config

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

- [Demo Mode](/demo-mode/) if you want to explore without credentials.
- [Provider Setup](/provider-setup/) for provider presets and authentication choices.
- [Timeline](/using-herald/timeline/) for the default inbox workflow.
- [All Keybindings](/reference/keybindings/) for a compact command table.
