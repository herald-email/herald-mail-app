# Browser Image Rendering Repro Design

## Overview

This spec defines a repeatable way to reproduce and document Herald's full-screen inline image rendering behavior in demo mode. The primary target is a browser terminal path so future debugging can inspect xterm.js rendering rather than relying only on a native terminal screenshot.

## Problem

Herald's demo mode includes a deterministic email titled "Creative Commons image sampler for terminal previews" with four embedded inline MIME images. Pressing `z` from its preview enters full-screen mode, where image protocol rendering can behave differently between stock ttyd, custom xterm.js clients, tmux captures, and native iTerm2.

## Goals

These outcomes make the repro useful for future image-rendering debugging sessions.

- [x] Provide browser-first instructions that launch `./bin/herald --demo` through ttyd.
- [x] Make xterm.js image protocol support explicit by using a custom frontend with an image addon when stock ttyd is insufficient.
- [x] Capture a screenshot artifact proving what happens after pressing `z`.
- [x] Record enough environment detail that a later Codex session can repeat the same investigation without rediscovering the setup.
- [x] Include a native iTerm2 fallback only when browser rendering cannot be made reliable in a reasonable pass.

## Non-Goals

The repro should stay narrow and avoid turning into an application feature change.

- [x] Do not change Herald's production rendering behavior as part of the repro setup.
- [x] Do not replace the existing tmux layout tests; this repro complements them for raster image placement.
- [x] Do not fetch remote email images at runtime; use the embedded demo MIME bytes.
- [x] Do not require real IMAP, SMTP, Ollama, or ProtonMail Bridge credentials.

## Recommended Approach

Run ttyd as the PTY and websocket host, but serve a custom browser terminal page through `ttyd -I`. The custom page loads xterm.js, its fit or attach plumbing, and an image addon with iTerm inline image protocol support enabled, then connects to ttyd's websocket stream for the Herald demo process.

Relevant upstream behavior:

- [x] ttyd supports a custom index page through `-I` and writable browser input through `-W`.
- [x] xterm.js addons are loaded with `Terminal.loadAddon(...)`.
- [x] `@xterm/addon-image` supports inline image output, including iTerm's inline image protocol.
- [x] iTerm2 inline images use OSC 1337 `File=...` escape sequences, which is the protocol Herald currently emits in iTerm-compatible mode.

## Fallback Approach

If the browser client cannot render the inline images after the custom frontend is in place, use native iTerm2 to capture proof. The fallback is acceptable only after recording what failed in the browser path, because the user's main goal is future guidance for browser-based image debugging.

Fallback acceptance:

- [x] Build Herald and launch `./bin/herald --demo` in iTerm2.
- [x] Navigate to the same Creative Commons sampler email.
- [x] Press `z` and capture a screenshot that shows the rendered state.
- [x] Document why the browser route was not sufficient, including the failing command, browser URL, and visible behavior.

## Repro Flow

The happy path should be short enough to run during a debugging session and explicit enough to paste into a future test report.

- [x] Build the app with `make build`.
- [x] Start ttyd on a known local port with a custom index and `./bin/herald --demo`.
- [x] Open the browser URL served by ttyd.
- [x] Use Herald's demo data to open the Timeline sampler email titled "Creative Commons image sampler for terminal previews".
- [x] Press `z` from the preview to enter full-screen mode.
- [x] Capture a screenshot immediately after full-screen mode renders.
- [x] Save the screenshot and a written repro report under `reports/`.

## Artifacts

The output should give both human proof and machine-readable breadcrumbs for future sessions.

- [x] Screenshot file under `reports/`, named with the date and short bug context.
- [x] Markdown report under `reports/`, including commands, ports, browser, terminal dimensions, Herald commit, and observed behavior.
- [x] If the browser path uses temporary harness files, record their paths and whether they are intended to be committed or disposable.
- [x] If fallback was used, include the browser failure notes before the iTerm2 screenshot notes.

## Error Handling

The repro should fail loudly and leave useful clues instead of silently falling back.

- [x] If ttyd is missing, record the install command and stop before using fallback.
- [x] If npm dependencies cannot be loaded for the custom frontend, record the package or CDN failure.
- [x] If the browser connects but image output is absent, inspect whether Herald emitted OSC 1337 sequences and whether the image addon loaded.
- [x] If `z` does not enter full-screen, capture the pre-`z` state and note the selected row and focus.
- [x] If the screenshot cannot be captured automatically, use a manual macOS screenshot and record the file path.

## Testing And Verification

Verification focuses on proof of reproduction rather than a production code regression test. tmux remains useful for ANSI/layout captures, but it cannot prove raster image placement by itself.

- [x] Confirm `make build` succeeds before starting ttyd.
- [x] Confirm browser input works by navigating within Herald.
- [x] Confirm the selected email is the Creative Commons image sampler.
- [x] Confirm `z` changes Herald into full-screen preview mode.
- [x] Confirm the screenshot visibly shows the post-`z` rendering state.
- [x] Confirm the report contains enough commands and observations for a future Codex run to repeat the work.

