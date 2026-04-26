---
title: Getting Started
description: Build Herald, run it for the first time, and understand the generated config.
---

This page covers the shortest route from source checkout to a usable Herald session. For every visible screen after launch, continue with [Global UI](/using-herald/global-ui/) and the tab pages.

## Requirements

- Go 1.25 or newer
- A C compiler such as `clang` or `gcc` for SQLite CGO support
- An IMAP account and SMTP settings, unless you run demo mode
- Optional: Ollama for local AI features

## Build and run

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

The wizard can fill provider presets for common accounts, including Gmail, Proton Mail Bridge, Fastmail, iCloud, and Outlook. Stable setup paths are standard IMAP and personal Gmail IMAP with an App Password. Experimental paths are labeled in the UI. See [First-run Wizard](/first-run-wizard/) for the screen-by-screen details.

<!-- HERALD_SCREENSHOT id="getting-started-main-tui" page="getting-started" alt="Herald main interface after initial sync" state="demo mode, 120x40, Timeline tab active" desc="Shows the first usable Herald interface with tab bar, folder sidebar, Timeline list, status bar, and key hints." capture="tmux demo 120x40; ./bin/herald --demo; press 1" -->

![Herald main interface after initial sync](/screenshots/getting-started-main-tui.png)

## Useful flags

```sh
./bin/herald -help
./bin/herald -debug
./bin/herald -verbose
./bin/herald -config custom.yaml
./bin/herald --demo
```

`-debug` and `-verbose` both enable DEBUG-level file logging. Herald does not write logs to the terminal because that would corrupt the TUI.

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
ttyd -W ./bin/herald
```

Open `http://localhost:7681`. The `-W` flag is required for keyboard input.

## What to read next

- [Demo Mode](/demo-mode/) if you want to explore without credentials.
- [Provider Setup](/provider-setup/) for provider presets and authentication choices.
- [Timeline](/using-herald/timeline/) for the default inbox workflow.
- [All Keybindings](/reference/keybindings/) for a compact command table.
