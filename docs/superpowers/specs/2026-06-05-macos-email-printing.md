# macOS Email Printing

## Purpose

Herald should let local macOS users print a loaded read-only email preview through the system print dialog without turning the terminal renderer into the print renderer. The feature covers two document modes so users can print either the sender's original visual message or Herald's cleaner Markdown-derived reading version.

- [x] Pressing `p` from a loaded Timeline split preview, Timeline full-screen preview, or Contacts inline preview opens a compact print chooser.
- [x] The chooser offers `1` Original Visual, `2` Markdown Swiss, `3` Markdown GitHub, `4` Markdown Manuscript, `5` Markdown Academic, and `Esc` cancel.
- [x] Original Visual prints the sender HTML body when present, otherwise escaped plain text.
- [x] Rendered Markdown prints Herald's HTML-to-Markdown reading representation as printable HTML using the selected print theme.

## Document Contract

The printable document is generated from message metadata and the fetched body model, not from ANSI output or visible viewport rows. It must preserve useful email context while keeping the same privacy posture as the preview.

- [x] Both modes include From, To, Cc, Date, Subject, body content, and attachment metadata.
- [x] Local CID inline images are embedded as data URIs when bytes are available.
- [x] Rendered Markdown mode preserves local CID images as printable data URI images when bytes are available.
- [x] Remote image URLs are not fetched and are not emitted as loadable `<img src="http...">` resources; rendered Markdown mode shows blocked-image placeholders with sanitized source links.
- [x] Scripts, event handlers, unsafe URL schemes, and remote CSS/background image loads are stripped before printing.
- [x] Temporary print HTML files are written with private owner-only permissions and removed after the print helper returns where practical.

## Platform Contract

Printing is a local macOS integration. Other surfaces must fail closed with clear status instead of trying to reuse host UI unexpectedly.

- [x] Darwin+cgo builds open the standard macOS print panel through a hidden helper subcommand and AppKit/WebKit printing.
- [x] Non-macOS and non-cgo builds compile with an unsupported printer implementation.
- [x] SSH sessions use the unsupported printer path so a remote client never opens the server host's print dialog.
- [x] Printing does not change mailbox state, cache state, reply/forward preservation, clipboard payloads, or preview remote-image reveal state.

## Verification

The feature changes product-visible TUI behavior, key routing, and a platform integration boundary, so verification needs unit tests, package tests, tmux evidence, and a manual macOS smoke.

- [x] Unit tests cover document generation, sanitization, CID image embedding, attachment metadata, temp-file permissions, and unsupported printer results.
- [x] App tests cover chooser open/cancel/mode selection, fake printer success/cancel/error, hidden/unsupported SSH behavior, and stale or missing body status.
- [x] Input-routing tests prove printable `p` remains text inside Compose, search, Settings, prompt, and editor fields.
- [x] Tmux captures cover the chooser at `220x50`, `80x24`, and `50x15`.
- [x] Manual macOS smoke opens both print modes from demo preview, verifies the system print dialog appears, then cancels.
