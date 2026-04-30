# Reply Compose Original Context

## Summary

This spec defines the Issue #15 behavior for Timeline replies opened in Compose. Reply composition must keep the original email visible as read-only context while preserving Herald's existing top-note-only editor and preserved HTML send path.

- [x] Reply Compose opens with `To`, `Re:` subject, and an empty editable response body.
- [x] Reply Compose shows a read-only `Original message` pane sourced from the preserved reply context.
- [x] Reply send and draft assembly continue to preserve the original HTML/plain fallback and threading headers outside the editable textarea.

## TUI Behavior

The Compose screen should make replies and forwards feel consistent: the response is the only editable body, and the original message is visible below it as source material. The pane should remain useful at common terminal sizes without hiding the main fields or causing layout overflow.

- [x] Replies and forwards both label the editable body region as `Response`.
- [x] The `Original message` pane shows sender, subject, date, and a wrapped plain-text preview when plain text is available.
- [x] HTML-only originals show a concise fallback note explaining that Herald will preserve the HTML when sending.
- [x] At cramped heights, the pane compacts to a bounded preview instead of pushing Compose chrome off-screen.
- [x] `Tab` can focus the original-message pane, and `j`/`k` or arrow keys scroll it without editing the response.

## Testing

The implementation should prove both state behavior and layout behavior because this is a terminal UI regression. Automated tests own the top-note-only invariant and scroll state, while tmux captures verify that the rendered Compose screen is readable at the required sizes.

- [x] Focused app tests cover reply rendering, unchanged top-note-only body content, original-pane scrolling, and `80x24` fit.
- [x] TC-14C is the manual/tmux acceptance case for reply and forward preserved Compose.
- [x] Broader verification includes `go test ./...`, `make build`, SSH server build/smoke, and MCP `tools/list` smoke.
