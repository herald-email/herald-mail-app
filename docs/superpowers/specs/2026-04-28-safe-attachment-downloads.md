# Safe Attachment Downloads

## Overview

This spec defines how Herald protects local files when users or tools download email attachments. The goal is simple: no attachment download path should silently replace an existing file.

- [x] Attachment saves detect path collisions before writing.
- [x] Attachment writes use create-exclusive file creation so a race cannot overwrite a file created after the check.
- [x] Suggested filenames preserve directory, base name, and extension while appending ` (1)`, ` (2)`, and so on.

## User-Visible Behavior

These requirements cover the interactive TUI and SSH-served TUI, where users can edit the save path before confirming. The experience should be protective without adding a new overwrite command.

- [x] Opening the attachment save prompt with an existing default path pre-fills the next available filename.
- [x] The save prompt shows a warning when Herald changed or rejected a path because a file already exists.
- [x] Pressing `Enter` on any existing path keeps the prompt open, replaces the input with a safe suggestion, and does not write attachment bytes.
- [x] Successful saves keep the existing `Saved: <path>` status message.

## API Behavior

These requirements cover non-interactive download surfaces that cannot safely choose a different path for the caller. The daemon and MCP server should report the collision and let the caller decide whether to retry with the suggestion.

- [x] `GET /v1/emails/{id}/attachments/{filename}?dest_path=...` refuses existing files with `409 Conflict`.
- [x] The daemon conflict body includes `error`, `path`, and `suggested_path`.
- [x] MCP `get_attachment` reports the existing path and suggested path instead of overwriting.
- [x] Attachment filename and destination path are URL-escaped when MCP calls the daemon.

## Verification

The acceptance checks combine focused Go tests with surface checks because this change touches shared file-writing logic, TUI state, daemon HTTP behavior, and MCP messaging. The TUI evidence should use demo or otherwise safe data and stop before destructive live operations.

- [x] Unit tests cover path suggestions, repeated collisions, extension handling, and hidden files.
- [x] Unit tests cover TUI prompt suggestions and refusal to save an existing custom path.
- [x] Unit tests cover backend and daemon refusal to overwrite existing files.
- [x] Unit tests or command evidence cover MCP conflict reporting.
- [x] `go test ./...`, `make build`, `make build-ssh`, and `make build-mcp` pass.
- [x] tmux captures cover the attachment save prompt at `220x50`, `80x24`, and `50x15`.
