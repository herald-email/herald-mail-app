# Full-Screen Inline Image Document Preview Test Report

Date: 2026-04-29

## Summary

- Result: PASS with documented environment limitations for real-terminal raster and SSH demo launch.
- Change: Full-screen preview document stream with inline image placement and graphics-mode fallbacks.

## Automated Tests

- Post-review edge-case regressions:
  - `go test ./internal/app -run 'NoCID|LongAlt|CleanupFullScreenScroll|OrphanInlineImageWithoutCID|ImagePreviewServer' -count=1`
  - Result: PASS
  - Covers no-CID inline images in Timeline/Cleanup full-screen, cleanup full-screen scroll routing with hidden focus, and long-alt local image link visibility.
- Final large-image link regression:
  - Red check first: `go test ./internal/app -run TestPreviewImageModeLinksPreservesOpenImageLabelForLargeImage -count=1`
  - Red result before fix: FAIL with `[image too large to render inline: image/jpeg]`
  - Green sweep: `go test ./internal/app -run 'PreviewImageModeLinks|TimelineFullScreen|CleanupFullScreen|NoCID|LongAlt' -count=1`
  - Result: PASS
- Package regression sweep after post-review fixes:
  - `go test ./internal/app ./internal/imap -count=1`
  - Result: PASS
- `go test ./internal/app ./internal/demo ./internal/backend -run 'PreviewDocument|PreviewImage|PreviewViewport|TimelineFullScreen|CleanupFullScreen|CreativeCommons|Sampler' -count=1`
  - Result: PASS
  - Output: `ok` for `internal/app`, `internal/demo`, and `internal/backend`.
- `go test ./...`
  - Result: PASS
  - Output: all packages passed or had no test files.

## TUI tmux Checks

- Binary: `/tmp/herald-test`, built with `go build -o /tmp/herald-test .`
- tmux version: `tmux 3.6a`
- Sizes checked: `220x50`, `80x24`, `50x15`
- Captures:
  - `/tmp/herald-image-doc-fullscreen.ansi`
  - `/tmp/herald-image-doc-80x24.ansi`
  - `/tmp/herald-image-doc-50x15.ansi`
  - Post-review rerun: `/tmp/herald-image-doc-final-fullscreen.ansi`
  - Post-review rerun: `/tmp/herald-image-doc-final-80x24.ansi`
  - Post-review rerun: `/tmp/herald-image-doc-final-50x15.ansi`
  - Post-review rerun: `/tmp/herald-image-doc-final-recovered.ansi`
  - Final renderer rerun: `/tmp/herald-image-doc-post-large-fullscreen.ansi`
  - Final renderer rerun: `/tmp/herald-image-doc-post-large-80x24.ansi`
- Result: PASS
  - `220x50` and `80x24` captures show the Creative Commons sampler in full-screen with the header pinned above the scrollable document.
  - Inline images appear in document order as local OSC 8 `open image` links in this non-raster tmux environment.
  - Raw `220x50` capture contains 4 localhost image links and 1 remote Commons URL link.
  - Post-review raw full-screen capture contains 4 localhost image links and 0 iTerm2 OSC 1337 image escapes in tmux.
  - Final renderer rerun after large-image link fix still contains 4 localhost image links and keeps the `80x24` header, scroll indicator, and pinned exit hint stable.
  - Raw captures contain no iTerm2 OSC 1337 image escapes because the verification terminal reported `TERM=dumb` and no `TERM_PROGRAM`.
  - `50x15` capture shows the minimum-size guard: `Terminal too narrow (50 cols). Resize to at least 60x15.`

## Real-Terminal Raster Check

- Terminal app/version: unavailable in this Codex execution environment.
- Selected graphics mode: non-raster local-link fallback in tmux; real iTerm2/Kitty/Sixel raster mode not exercised.
- Screenshot paths: none.
- Native scrollback result: not run. The environment reports `TERM=dumb` and no `TERM_PROGRAM`, so it cannot provide meaningful real-terminal raster placement evidence. TC-23A now requires this check for local manual QA in a compatible raster terminal.

## SSH Surface

- Command:
  - `go build -o ./bin/herald-ssh-server ./cmd/herald-ssh-server`
  - `./bin/herald-ssh-server -version`
  - `./bin/herald-ssh-server --demo`
- Result: PARTIAL
  - Build/version check passed: `herald-ssh-server dev`.
  - Demo SSH launch was not run because `herald-ssh-server` does not currently define a `--demo` flag; the command exits with `flag provided but not defined: -demo`.
  - No local YAML config was present in the worktree for a non-demo IMAP SSH launch.

## MCP Surface

- Command:
  - `go build -o ./bin/herald-mcp-server ./cmd/herald-mcp-server`
  - `echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | ./bin/herald-mcp-server --demo`
- Result: PASS
  - JSON-RPC tools list succeeded and returned the deterministic demo tools.

## Notes

- tmux verifies layout, key routing, resize behavior, OSC 8 fallback links, and minimum-size guard behavior, but cannot prove real raster image placement.
- A follow-up manual QA pass should run TC-23A in iTerm2, Kitty, or a Sixel-capable terminal and attach screenshots plus terminal app/version.
- A separate follow-up could add `--demo` support to `cmd/herald-ssh-server` so SSH preview behavior can be verified without live IMAP credentials.
