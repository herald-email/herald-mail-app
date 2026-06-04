# Preview Cursor Selection And Rich Copy

## Purpose

Read-only email previews need a visible selection cursor because hidden terminal cursors make Herald-owned visual selection hard to trust. This feature turns preview selection into an explicit reader mode that can copy plain text, preserve rich HTML where available, and expose image copy behavior without weakening terminal-native mouse selection.

- [x] First `v` opens cursor mode in Timeline split preview, Timeline full-screen preview, Contacts inline preview, and the Compose original-message pane without selecting text.
- [x] Second `v` starts range selection from the current cursor position and highlights only the selected anchor-to-cursor range.
- [x] `Esc` leaves cursor or selection mode before closing preview, leaving full-screen, or leaving Compose.

## Interaction Contract

The preview-selection model belongs only to read-only email preview surfaces. Editable fields keep their own input model and should receive printable characters normally.

- [x] `h/j/k/l` and arrow keys move the preview cursor left, down, up, and right inside the active preview surface.
- [x] Vertical movement preserves the preferred visual column and scrolls the preview as needed.
- [x] Cursor-only movement keeps the anchor at the cursor so users can position first, then press `v` again to start selection from that point.
- [x] `yy` copies the current row, `y` copies the active selection or image under the cursor, and `Y` copies all preview text.
- [x] Existing `m` mouse-selection mode remains available and keeps its release/restore behavior.
- [x] Compose body, address fields, search inputs, Settings fields, prompt editor fields, and rule editor fields do not treat preview-selection keys as shortcuts.

## Clipboard Contract

Clipboard writes should use the richest safe payload the current platform supports while always preserving a plain-text fallback. The app should report bounded status when a richer payload cannot be written.

- [x] Text selections write exact plain text after ANSI/control stripping and include an HTML fragment when the selection comes from HTML-derived content.
- [x] Whole-message copy writes plain text plus the original/sanitized HTML body when available.
- [x] A single inline image row under the cursor writes image bytes on macOS with cgo; fallback builds save the bytes to a temporary file and copy the path.
- [x] Mixed text-plus-image selections copy text with readable image placeholders instead of attempting an unreliable mixed MIME clipboard.

## Rendering Contract

Selection rendering should stay layered on top of the preview document so image placement, link rendering, and row budgets remain stable. Native raster rows should never hide the selection cursor.

- [x] Preview document rows retain source metadata for copyable text, HTML fragments, and image identity.
- [x] Normal preview rendering is unchanged when selection mode is inactive.
- [x] Selection mode renders image rows as selectable text placeholders/captions when needed so the cursor remains visible.
- [x] The status and hint bars advertise cursor movement before selection starts, then selection extension and copy actions after the second `v`.

## Verification

This feature changes visible TUI behavior, key routing, and clipboard payloads, so it needs unit coverage plus terminal evidence. Demo mode is sufficient for visual gates because it includes selection, HTML, contacts, and inline-image fixtures.

- [x] Focused Go tests cover cursor movement, clamping, preferred-column vertical movement, scroll-follow behavior, selection rendering, plain copy, HTML copy, image copy, and mixed-selection fallback.
- [x] App-level tests cover Timeline split/full-screen, Contacts inline preview, and Compose original-message selection.
- [x] Input-routing tests prove printable text still works in Compose, search, Settings, prompt editor, and rule editor surfaces.
- [x] Autopilot evidence captures before/after `220x50`, `80x24`, and `50x15` TUI states plus SSH and MCP smoke checks.
