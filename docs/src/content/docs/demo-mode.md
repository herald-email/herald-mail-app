---
title: Demo Mode
description: Run Herald with synthetic mail, no IMAP account, and no credentials.
---

Demo mode starts Herald with synthetic data. It is useful for trying the interface, recording demos, and testing visual changes without touching a real inbox.

```sh
make build
./bin/herald --demo
```

Demo mode skips real IMAP setup, uses a fake account, and does not require SMTP credentials. AI features are optional and depend on your local AI configuration.

## Browser demo

You can combine demo mode with `ttyd`:

```sh
ttyd -W ./bin/herald --demo
```

## Regenerate demo GIFs

Demo tapes live in `demos/*.tape`, and generated GIFs go to `static/*.gif`.

```sh
brew install vhs
make build
for f in demos/*.tape; do vhs "$f"; done
```

Run tapes from the repository root because they reference `./bin/herald`.

## What demo mode is not

Demo mode is not a local mailbox emulator. It is a presentation and UI testing mode with synthetic data. Use a real IMAP account or a test IMAP server when you need to verify provider behavior.
