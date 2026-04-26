---
title: Demo GIF Workflow
description: Regenerate Herald demo GIFs from VHS tapes using demo mode.
---

Demo GIFs are recorded from synthetic data so documentation and project media can be refreshed without touching a real mailbox.

## Prerequisites

```sh
brew install vhs ffmpeg
```

## Regenerate All GIFs

```sh
make docs-media
```

Demo tapes live in `demos/*.tape`. Canonical GIFs are written to `assets/demo/*.gif`, docs-facing copies are written to `docs/public/demo/*.gif`, and still screenshots are written to `docs/public/screenshots/*.png`. Run media generation from the repository root because the tapes reference `./bin/herald`.

<!-- HERALD_SCREENSHOT id="demo-gif-vhs-run" page="demo-gifs" alt="VHS demo tape generation command" state="local shell, demo tapes present" desc="Shows the command used to regenerate all Herald demo GIFs and screenshots." capture="terminal; make docs-media" -->

![VHS demo tape generation command](/screenshots/demo-gif-vhs-run.png)

## Recording Guidance

- Keep tapes focused and under 30 seconds.
- Use `./bin/herald --demo` unless the demo explicitly needs live provider behavior.
- Prefer terminal sizes that match documentation screenshot states, such as `120x40`, `80x24`, and `50x15`.
- After changing a visible feature, regenerate the relevant tape.

## Related Pages

- [Demo Mode](/demo-mode/)
- [Global UI](/using-herald/global-ui/)
- [Timeline](/using-herald/timeline/)
