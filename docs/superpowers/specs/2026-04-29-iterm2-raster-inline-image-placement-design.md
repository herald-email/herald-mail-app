# iTerm2 Raster Inline Image Placement Design

## Overview

Full-screen preview should show real raster inline images in iTerm2-compatible terminals, including the custom ttyd+xterm image-addon repro path. Kitty and Ghostty remain the preferred robust raster path long term, but iTerm2 must be supported because it is a dominant macOS terminal.

- [ ] Preserve real raster rendering for iTerm2 OSC 1337 instead of degrading the target repro path to local `open image` links.
- [ ] Keep Kitty/Ghostty behavior as the reference for document-order raster rendering.
- [ ] Accept constrained iTerm2 sizing as long as images appear near their authored positions and never displace pinned preview chrome.

## Problem

The current iTerm2 OSC 1337 path can render images outside Herald's expected row model. In the TC-23A custom ttyd+xterm image-addon repro, raster pixels dominate the full-screen viewport and the pinned message header/body context disappears, while the Kitty/Ghostty path renders the same sampler in usable document order.

- [ ] Reproduce from `./bin/herald --demo` with the Creative Commons sampler, `z` full-screen mode, and `reports/ttyd-image-harness/index.html`.
- [ ] Treat tmux text captures as necessary layout evidence but insufficient proof for raster placement.
- [ ] Require browser or native terminal screenshots for real iTerm2 raster validation.

## Reproduction Evidence

The current repro packet was captured from the documented TC-23A flow before this design was written. It shows that the text/fallback path is stable, but the browser raster path still loses the pinned preview context.

- [ ] Current `main` custom ttyd+xterm image-addon raster bug: `reports/inline-image-repro_2026-04-29_0447z/browser_fullscreen_raster_bug.png`.
- [ ] Current `main` tmux full-screen fallback capture: `reports/inline-image-repro_2026-04-29_0447z/tmux_sampler_fullscreen_220x50.png`.
- [ ] Earlier isolated worktree still fails when forced into true iTerm2 raster mode: `reports/inline-image-repro_2026-04-29_0447z/browser_worktree_true_iterm_raster.png`.
- [ ] The fixed implementation must attach a new browser raster screenshot, not reuse the fallback-link evidence.

## Goals

The fix should make iTerm2 good enough and stable, not pixel-perfect. The expected result is a readable full-screen document with bounded raster images near their intended positions.

- [ ] Show real iTerm2 raster images in full-screen preview when iTerm2 mode is selected or auto-detected.
- [ ] Keep the From/Date/Subject/actions header visible above the scrollable document.
- [ ] Keep raster images in approximate authored order from the preview document.
- [ ] Use a small set of safe iTerm2 cell boxes instead of natural sizes when needed.
- [ ] Preserve the working Kitty/Ghostty renderer and continue to prefer Kitty where auto-detected.
- [ ] Add visual evidence that would fail if raster output hides the header or floats over unrelated text.

## Non-Goals

The change should stay narrowly focused on preview raster placement. It should not turn Herald into a CSS-accurate HTML mail client or alter compose/send MIME behavior.

- [ ] Do not fetch remote image bytes automatically.
- [ ] Do not change compose inline-image embedding.
- [ ] Do not require iTerm2 to match Kitty/Ghostty dimensions exactly.
- [ ] Do not remove local `open image` fallback links for unsupported local terminals.
- [ ] Do not change SSH auto-mode safety defaults.

## Recommended Approach

Introduce a protocol-specific raster placement contract under the existing preview document layer. The preview document remains protocol-agnostic, while each raster renderer chooses an explicit cell footprint and returns both terminal bytes and the rows Herald should reserve.

- [ ] Keep the preview document as ordered text, inline image, remote link, missing image, and orphan image blocks.
- [ ] Add or refine an image placement planner that maps each inline image block to a protocol-specific placement.
- [ ] Treat Kitty/Ghostty as the robust reference path with existing placement clearing on redraw.
- [ ] Treat iTerm2 as a constrained raster path with predefined safe cell boxes.
- [ ] Require each renderer to report rows consumed exactly, including any separator or padding rows.

## Rendering Contract

The renderer contract should make the row budget explicit before Herald commits rows to the scrollable document. This prevents a terminal renderer from honoring a wide image while Herald reserves only a short row count.

- [ ] A placement plan records protocol, cell width, cell height, emitted content, rows consumed, and a test/debug label.
- [ ] The iTerm2 planner classifies images by aspect ratio and terminal budget before selecting a safe box.
- [ ] Tiny badge-like images use their natural tiny footprint or the nearest tiny safe box.
- [ ] Square-ish charts use a bounded medium box.
- [ ] Wide landscape images use conservative wide/short boxes that fit below the header.
- [ ] If an image cannot fit the remaining budget, the planner chooses a smaller raster thumbnail before falling back.
- [ ] The iTerm2 renderer emits explicit `width` and `height` arguments for OSC 1337.
- [ ] The iTerm2 renderer avoids hidden trailing newline behavior that disagrees with the reported row count.

## Data Flow

On body load, resize, mode change, or image-protocol override change, Herald should rebuild or invalidate the preview layout. The resulting viewport stays a single ordered row list so app scrolling remains predictable across text and image blocks.

- [ ] Body load builds the preview document from `models.EmailBody`.
- [ ] Layout asks the active protocol planner for each inline image placement before rows are appended.
- [ ] Rendering writes exactly the rows returned by the placement plan.
- [ ] Kitty redraws continue to clear previous placements before repainting a viewport.
- [ ] iTerm2 redraws rely on conservative footprints and explicit row accounting to avoid drift.
- [ ] Local link and placeholder modes continue to use one-line rows.

## Error Handling

Failures should degrade in place without corrupting the viewport. The raster-first policy applies only when the renderer can produce a bounded image block within the selected protocol's safe limits.

- [ ] Empty image data renders `[image unavailable: empty data]`.
- [ ] Decode failures use a bounded placeholder or a conservative default thumbnail box.
- [ ] Oversized images are down-constrained into the largest safe raster box before any fallback.
- [ ] Unsupported terminals use current local links or placeholders.
- [ ] SSH auto mode remains placeholder-first unless the user explicitly forces `-image-protocol=iterm2` or `-image-protocol=kitty`.
- [ ] If iTerm2 output is impossible to keep bounded for a specific image, render a smaller raster thumbnail before using a non-raster fallback.

## Testing

Tests must cover both deterministic logic and the visual browser path that originally caught the bug. Text-only tests are valuable but cannot prove real raster placement.

- [ ] Add unit tests for iTerm2 placement planning, including 960x540-style landscape images in tall terminals.
- [ ] Add unit tests proving selected iTerm2 cell width and height stay aspect-compatible enough for the safe predefined box.
- [ ] Add unit tests for OSC 1337 output with explicit width and height.
- [ ] Keep existing Kitty/Ghostty renderer and stale-placement tests green.
- [ ] Add or update a test that verifies preview viewport row count never exceeds the visible body budget.
- [ ] Reproduce red visual evidence using custom ttyd+xterm image-addon before implementation.
- [ ] Capture fixed custom ttyd+xterm image-addon PNG evidence with real raster images, not fallback links.
- [ ] Capture tmux `220x50`, `80x24`, and `50x15` evidence for layout, fallback, and minimum-size guard behavior.
- [ ] Save the final report under `reports/` with terminal app/version, selected protocol, commands, screenshots, and ANSI captures.

## Acceptance Criteria

The acceptance bar is practical: iTerm2 should be stable, bounded, and recognizable as an inline image preview. Kitty/Ghostty can remain better and more faithful.

- [ ] Custom ttyd+xterm image-addon full-screen screenshot shows the preview header and real raster images together.
- [ ] Native iTerm2 or forced iTerm2 mode screenshot shows real raster images near authored positions.
- [ ] Ghostty/Kitty screenshot remains correct and serves as the higher-quality reference.
- [ ] No accepted proof relies only on fallback `open image` links for the iTerm2 target path.
- [ ] Scrolling with `j`, `k`, `PgDn`, and `PgUp` does not leave stale raster output over text.
- [ ] At `50x15`, the minimum-size guard appears and resizing back restores a clean full-screen preview.

## References

The implementation should be grounded in the current project docs and the upstream protocol behavior. iTerm2 OSC 1337 supports explicit cell dimensions, while xterm-addon-image documents cursor and scrolling behavior that directly affects browser raster placement.

- [ ] `TUI_TESTPLAN.md` TC-23A defines the full-screen inline image raster acceptance flow.
- [ ] `ARCHITECTURE.md` documents local image safety and protocol selection.
- [ ] `VISION.md` tracks bounded iTerm2/Kitty full-screen image viewing.
- [ ] `docs/superpowers/specs/2026-04-29-full-screen-inline-image-document-preview-design.md` defines the existing preview document layer.
- [ ] iTerm2 Inline Images Protocol: https://iterm2.com/documentation-images.html
- [ ] xterm-addon-image behavior notes: https://github.com/jerch/xterm-addon-image
