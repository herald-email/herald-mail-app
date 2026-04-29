---
title: Demo GIF Workflow
description: Regenerate Herald demo GIFs from VHS tapes using demo mode.
---

Demo GIFs are recorded from synthetic data so documentation and project media can be refreshed without touching a real mailbox. The canonical tapes use VHS's `Builtin Solarized Dark` theme so terminal screenshots stay readable in docs.

## Prerequisites

```sh
brew install vhs ffmpeg
```

## Regenerate All GIFs

```sh
make docs-media
```

Demo tapes live in `demos/*.tape`. Canonical GIFs are written to `assets/demo/*.gif`, docs-facing copies are written to `docs/public/demo/*.gif`, and still screenshots are written to `docs/public/screenshots/*.png`. Run media generation from the repository root because the tapes reference `./bin/herald`. The image-preview tape forces `-image-protocol=kitty` against the Creative Commons sampler so the generated media can exercise the Kitty/Ghostty rendering path once the capture stack can render raster blocks.

<!-- HERALD_SCREENSHOT id="demo-gif-vhs-run" page="demo-gifs" alt="VHS demo tape generation command" state="local shell, demo tapes present" desc="Shows the command used to regenerate all Herald demo GIFs and screenshots." capture="terminal; make docs-media" -->

![VHS demo tape generation command](/screenshots/demo-gif-vhs-run.png)

## Recording Guidance

- Keep tapes focused and under 30 seconds.
- Use `./bin/herald --demo` unless the demo explicitly needs live provider behavior.
- Use `Builtin Solarized Dark` for new VHS tapes unless a specific visual comparison needs another theme.
- Prefer terminal sizes that match documentation screenshot states, such as `120x40`, `80x24`, and `50x15`.
- For inline image demos, use the Creative Commons sampler fixture and force `-image-protocol=kitty`; reject captures that show raw protocol text or hide the image area.
- If the installed VHS/ttyd stack cannot render Kitty or iTerm2 raster blocks, keep the tape as key-flow coverage and record native Ghostty/Kitty evidence separately instead of committing a blank raster capture. Default `make docs-media` skips raster image-preview media; set `HERALD_DOC_MEDIA_INCLUDE_RASTER=1` only after confirming the local capture stack paints the embedded image.
- After changing a visible feature, regenerate the relevant tape.

## Related Pages

- [Demo Mode](/demo-mode/)
- [Global UI](/using-herald/global-ui/)
- [Timeline](/using-herald/timeline/)
