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

<!-- HERALD_SCREENSHOT id="demo-mode-timeline" page="demo-mode" alt="Demo mode Timeline with synthetic messages" state="demo mode, 120x40, Timeline tab active" desc="Shows the default screenshot source for documentation: synthetic folders and messages without a live mailbox." capture="tmux demo 120x40; ./bin/herald --demo; press 1" -->

![Demo mode Timeline with synthetic messages](/screenshots/demo-mode-timeline.png)

## Browser demo

You can combine demo mode with `ttyd`:

```sh
ttyd -W ./bin/herald --demo
```

## Regenerate demo GIFs

Demo tapes live in `demos/*.tape`, canonical GIFs go to `assets/demo/*.gif`, and docs-facing copies go to `docs/public/demo/*.gif`.

```sh
brew install vhs ffmpeg
make docs-media
```

Run media generation from the repository root because the tapes reference `./bin/herald`.

See [Demo GIF Workflow](/advanced/demo-gifs/) for the full recording flow.

## What demo mode is not

Demo mode is not a local mailbox emulator. It is a presentation and UI testing mode with synthetic data. Use a real IMAP account or a test IMAP server when you need to verify provider behavior.
