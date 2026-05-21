---
title: Demo Mode
description: Run Herald with synthetic mail, no IMAP account, and no credentials.
---

Demo mode starts Herald with synthetic data. It is useful for trying the interface, recording demos, and testing visual changes without touching a real inbox.

```sh
make build
./bin/herald --demo
```

Demo mode skips real IMAP setup, uses a fake account, and does not require SMTP credentials. Demo AI features are deterministic and run offline, so classification, semantic search, chat, and quick replies can be exercised without Ollama.
Presentation tapes can add `--demo-keys` to show a compact keypress overlay without changing normal app key routing.

Use `-theme` with demo mode to try a built-in palette or local YAML theme without saving config:

```sh
./bin/herald --demo -theme jade-signal
```

<!-- HERALD_SCREENSHOT id="demo-mode-timeline" page="demo-mode" alt="Demo mode Timeline with synthetic messages" state="demo mode, 120x40, Timeline tab active" desc="Shows the default screenshot source for documentation: synthetic folders and messages without a live mailbox." capture="tmux demo 120x40; ./bin/herald --demo; press 1" -->

![Demo mode Timeline with synthetic messages](/screenshots/demo-mode-timeline.png)

## Image rendering demo

Demo mode includes a `Step 5: View inline images in full screen` Herald Image Lab email with embedded Creative Commons inline images. To test raster image rendering, run Herald in a Kitty-protocol terminal such as Ghostty on macOS or Kitty itself and force the Kitty graphics path:

```sh
./bin/herald --demo -image-protocol=kitty
```

Search for `Step 5: View inline images in full screen`, open the image message, and press `z` for full-screen preview. iTerm2 can use `-image-protocol=iterm2`; terminals without raster graphics show safe placeholders or local `open image` links when available.

## Browser demo

You can combine demo mode with `ttyd`:

```sh
ttyd -W ./bin/herald --demo
```

## Regenerate demo GIFs

Demo tapes live in `demos/*.tape`, canonical GIFs go to `assets/demo/*.gif`, and docs-facing copies go to `docs/public/demo/*.gif`. Broad docs captures use the brighter `Builtin Solarized Dark` VHS theme; selected showcase captures also use `Dark Pastel`, `Red Alert`, and `Builtin Pastel Dark`.

```sh
brew install vhs ffmpeg
make docs-media
```

Run media generation from the repository root because the tapes reference `./bin/herald`.

Theme gallery screenshots use a separate tmux flow:

```sh
scripts/regenerate-theme-screenshots.sh
HERALD_THEME_SCREENSHOT_VIEW=preview scripts/regenerate-theme-screenshots.sh jade-signal
```

See [Regenerate Screenshots](/development/regenerate-screenshots/) and [Demo GIF Workflow](/advanced/demo-gifs/) for the full recording flow.

## What demo mode is not

Demo mode is not a local mailbox emulator. It is a presentation and UI testing mode with synthetic data. Use a real IMAP account or a test IMAP server when you need to verify provider behavior.

For development regression tests, Herald also has an internal virtual mail lab in `internal/testmail`. That lab starts local IMAP/SMTP servers, routes mail between `alice@herald.test` and `bob@herald.test`, and can replay sanitized realistic fixtures from `internal/testmail/testdata/corpus`. Use demo mode for polished demos and broad UI smoke checks; use the virtual lab for realistic MIME, calendar invite, inline image, draft, reply, and send-flow regressions; use live config only for provider-specific behavior.
