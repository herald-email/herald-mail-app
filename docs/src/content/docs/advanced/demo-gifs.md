---
title: Demo GIF Workflow
description: Regenerate Herald demo GIFs from VHS tapes using demo mode.
---

Demo GIFs are recorded from synthetic data so documentation and project media can be refreshed without touching a real mailbox.

## Prerequisites

```sh
brew install vhs
make build
```

## Regenerate All GIFs

```sh
for f in demos/*.tape; do vhs "$f"; done
```

Demo tapes live in `demos/*.tape`. Output GIFs are written to `static/*.gif` according to each tape's `Output` line. Run tapes from the repository root because they reference `./bin/herald`.

<!-- HERALD_SCREENSHOT id="demo-gif-vhs-run" page="demo-gifs" alt="VHS demo tape generation command" state="local shell, demo tapes present" desc="Shows the command used to regenerate all Herald demo GIFs from demos/*.tape into static/*.gif." capture="terminal; make build; for f in demos/*.tape; do vhs $f; done" -->

## Recording Guidance

- Keep tapes focused and under 30 seconds.
- Use `./bin/herald --demo` unless the demo explicitly needs live provider behavior.
- Prefer terminal sizes that match documentation screenshot states, such as `120x40`, `80x24`, and `50x15`.
- After changing a visible feature, regenerate the relevant tape.

## Related Pages

- [Demo Mode](/demo-mode/)
- [Global UI](/using-herald/global-ui/)
- [Timeline](/using-herald/timeline/)
