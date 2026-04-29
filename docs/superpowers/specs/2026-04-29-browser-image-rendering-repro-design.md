# Browser Image Rendering Repro Design

## Overview

This spec defines a repeatable way to reproduce and document Herald's full-screen inline image rendering behavior in demo mode. The primary target is a browser terminal path so future debugging can inspect xterm.js rendering rather than relying only on a native terminal screenshot.

## Problem

Herald's demo mode includes a deterministic email titled "Creative Commons image sampler for terminal previews" with four embedded inline MIME images. Pressing `z` from its preview enters full-screen mode, where image protocol rendering can behave differently between stock ttyd, custom xterm.js clients, tmux captures, and native iTerm2.

## Goals

These outcomes make the repro useful for future image-rendering debugging sessions.

- [ ] Provide browser-first instructions that launch `./bin/herald --demo` through ttyd.
- [ ] Make xterm.js image protocol support explicit by using a custom frontend with an image addon when stock ttyd is insufficient.
- [ ] Capture a screenshot artifact proving what happens after pressing `z`.
- [ ] Record enough environment detail that a later Codex session can repeat the same investigation without rediscovering the setup.
- [ ] Include a native iTerm2 fallback only when browser rendering cannot be made reliable in a reasonable pass.

## Non-Goals

The repro should stay narrow and avoid turning into an application feature change.

- [ ] Do not change Herald's production rendering behavior as part of the repro setup.
- [ ] Do not replace the existing tmux layout tests; this repro complements them for raster image placement.
- [ ] Do not fetch remote email images at runtime; use the embedded demo MIME bytes.
- [ ] Do not require real IMAP, SMTP, Ollama, or ProtonMail Bridge credentials.

## Recommended Approach

Run ttyd as the PTY and websocket host, but serve a custom browser terminal page through `ttyd -I`. The custom page loads xterm.js, its fit or attach plumbing, and an image addon with iTerm inline image protocol support enabled, then connects to ttyd's websocket stream for the Herald demo process.

Relevant upstream behavior:

- [ ] ttyd supports a custom index page through `-I` and writable browser input through `-W`.
- [ ] xterm.js addons are loaded with `Terminal.loadAddon(...)`.
- [ ] `@xterm/addon-image` supports inline image output, including iTerm's inline image protocol.
- [ ] iTerm2 inline images use OSC 1337 `File=...` escape sequences, which is the protocol Herald currently emits in iTerm-compatible mode.

## Fallback Approach

If the browser client cannot render the inline images after the custom frontend is in place, use native iTerm2 to capture proof. The fallback is acceptable only after recording what failed in the browser path, because the user's main goal is future guidance for browser-based image debugging.

Fallback acceptance:

- [ ] Build Herald and launch `./bin/herald --demo` in iTerm2.
- [ ] Navigate to the same Creative Commons sampler email.
- [ ] Press `z` and capture a screenshot that shows the rendered state.
- [ ] Document why the browser route was not sufficient, including the failing command, browser URL, and visible behavior.

## Repro Flow

The happy path should be short enough to run during a debugging session and explicit enough to paste into a future test report.

- [ ] Build the app with `make build`.
- [ ] Start ttyd on a known local port with a custom index and `./bin/herald --demo`.
- [ ] Open the browser URL served by ttyd.
- [ ] Use Herald's demo data to open the Timeline sampler email titled "Creative Commons image sampler for terminal previews".
- [ ] Press `z` from the preview to enter full-screen mode.
- [ ] Capture a screenshot immediately after full-screen mode renders.
- [ ] Save the screenshot and a written repro report under `reports/`.

## Artifacts

The output should give both human proof and machine-readable breadcrumbs for future sessions.

- [ ] Screenshot file under `reports/`, named with the date and short bug context.
- [ ] Markdown report under `reports/`, including commands, ports, browser, terminal dimensions, Herald commit, and observed behavior.
- [ ] If the browser path uses temporary harness files, record their paths and whether they are intended to be committed or disposable.
- [ ] If fallback was used, include the browser failure notes before the iTerm2 screenshot notes.

## Error Handling

The repro should fail loudly and leave useful clues instead of silently falling back.

- [ ] If ttyd is missing, record the install command and stop before using fallback.
- [ ] If npm dependencies cannot be loaded for the custom frontend, record the package or CDN failure.
- [ ] If the browser connects but image output is absent, inspect whether Herald emitted OSC 1337 sequences and whether the image addon loaded.
- [ ] If `z` does not enter full-screen, capture the pre-`z` state and note the selected row and focus.
- [ ] If the screenshot cannot be captured automatically, use a manual macOS screenshot and record the file path.

## Testing And Verification

Verification focuses on proof of reproduction rather than a production code regression test. tmux remains useful for ANSI/layout captures, but it cannot prove raster image placement by itself.

- [ ] Confirm `make build` succeeds before starting ttyd.
- [ ] Confirm browser input works by navigating within Herald.
- [ ] Confirm the selected email is the Creative Commons image sampler.
- [ ] Confirm `z` changes Herald into full-screen preview mode.
- [ ] Confirm the screenshot visibly shows the post-`z` rendering state.
- [ ] Confirm the report contains enough commands and observations for a future Codex run to repeat the work.

