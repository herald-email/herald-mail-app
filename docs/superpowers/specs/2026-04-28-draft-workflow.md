# Draft Workflow

## Overview

Herald should treat Gmail and IMAP drafts as first-class messages in Timeline instead of making users infer draft state from sender names or folder context. This spec covers draft discovery, Timeline and preview indicators, edit routing into Compose, and safe draft replacement while preserving the existing send and reply workflows.

## User-Visible Behavior

- [x] Timeline thread rows show `Draft` when one draft exists and `Draft N` when multiple drafts exist in the collapsed thread.
- [x] Individual Timeline draft rows show `Draft` in the subject text without consuming the classification `Tag` column.
- [x] Preview headers for drafts include `State: Draft - E edit draft`.
- [x] Draft-focused key hints prioritize `E: edit draft` and `D: discard draft`.
- [x] Pressing `E` on a draft row, a draft preview, or a collapsed thread containing drafts opens Compose with the saved draft contents.

## Data And Sync Contract

- [x] `models.EmailData` includes `IsDraft`, persisted as `emails.is_draft` in SQLite.
- [x] IMAP sync maps the `\Draft` flag and canonical draft folders (`Drafts`, `[Gmail]/Drafts`, `INBOX.Drafts`, `INBOX/Drafts`) into `IsDraft`.
- [x] Active-folder reconcile refreshes cached draft flags for existing rows, so Gmail-created drafts become visible after refresh.
- [x] Parsed `models.EmailBody` exposes editable draft headers: `From`, `To`, `CC`, `BCC`, and `Subject`.

## Compose Contract

- [x] Compose opened from a draft restores recipients, subject, and terminal-friendly body text from the saved draft.
- [x] Compose tracks the source draft UID and folder while editing.
- [x] Sending deletes the source draft only after SMTP/demo send success.
- [x] Autosave replacement saves the new draft first and deletes the previous draft only after the save succeeds.
- [x] V1 uses `text/plain` when present and the existing HTML-to-Markdown fallback otherwise; exact Gmail rich-text fidelity is out of scope.

## Verification

- [x] Focused Go tests cover cache migration/read/write, IMAP draft flag mapping, MIME header parsing, Timeline draft labels, preview header text, `E` draft routing, and safe draft replacement.
- [x] Demo mode includes at least one draft fixture for repeatable tmux captures.
- [x] TUI evidence covers `220x50`, `80x24`, and `50x15`.
- [x] SSH and MCP smoke checks run before handoff because the feature changes shared models and build surfaces.
