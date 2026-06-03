# Full-Screen Inline Image Document Preview Design

## Purpose

Full-screen email preview should behave more like a GUI email reader: text and inline raster images appear in the same scrollable document flow, with images near their authored positions when Herald can infer those positions. This fixes the current iTerm2 failure mode where rendered images can push the full-screen title/header out of view because the preview renderer and terminal disagree about consumed rows.

## Scope

This design originally covered the full-screen preview path for Timeline first, then the shared Cleanup full-screen preview path where the same body renderer was reused. As of the split-preview image rendering slice, Timeline split preview also uses the same ordered document layer with tighter row budgets so bounded thumbnails, local links, or placeholders can appear in the side panel without turning it into a full HTML renderer.

- [x] Build an ordered preview document from `models.EmailBody` instead of rendering a pre-body image block plus flattened text.
- [x] Preserve `cid:` inline image placement from `TextHTML` when HTML is available.
- [x] Keep local inline raster images scrollable as part of the email document, below the pinned preview header.
- [x] Keep split preview compact while allowing bounded raster thumbnails, local links, or placeholders in the ordered body flow.
- [ ] Clip partially visible native raster images to the preview viewport so images can enter or leave from the top or bottom without overflowing panel borders.
- [x] Keep remote HTML image URLs as readable OSC 8 placeholders without fetching remote bytes automatically.
- [x] Include orphan inline MIME images in a deterministic scrollable fallback section when no authored placement is available.
- [x] Add a protocol selection foundation for iTerm2 now and Kitty/Sixel later.
- [x] Update TUI test protocols so real-terminal raster behavior is captured, not only ANSI text output.

## Architecture

Add a preview document layer between `models.EmailBody` and the full-screen TUI renderer. The document is an ordered stream of blocks: wrapped text, local inline image blocks, remote image links, missing-image placeholders, and orphan-image fallback sections. Preview chrome such as From/Date/Subject, tags, actions, quick replies, and attachments remains outside this document layer unless a later design deliberately folds it in.

- [x] Add a preview document builder that takes `EmailBody`, selected message metadata, image descriptions, and rendering context.
- [x] Prefer `TextHTML` for full-screen document construction because HTML is the reliable source for `cid:` image placement.
- [x] Fall back to existing `TextPlain`/markdown rendering when no HTML exists or HTML parsing fails.
- [x] Track which inline MIME images were placed from HTML so unplaced images can be rendered once in a fallback section.
- [x] Keep the document layer independent of Bubble Tea so it can be unit-tested without a terminal.

## Components

The change should stay split across small, testable components rather than making `email_preview.go` responsible for parsing, layout, and graphics protocol details. `email_preview.go` should orchestrate header chrome, viewport height, scroll offset, and bottom hints while delegating document construction and image rendering.

- [x] `preview_document` builder: converts `EmailBody` into ordered preview blocks.
- [x] `preview_document` layout: wraps text blocks and computes block row ranges for a given width.
- [x] Image renderer interface: renders one image block and returns both content and physical rows consumed.
- [x] iTerm2 image renderer: reuses OSC 1337 rendering while reporting exact rows, including separator rows.
- [x] Fallback image renderers: local OSC 8 `open image` links for local TUI, bounded placeholders for SSH/unsupported terminals.
- [x] Full-screen viewport renderer: renders only the visible document blocks beneath the pinned header.

## Data Flow

When an email body loads or the terminal width/protocol changes, Herald should rebuild or invalidate the preview document cache. Full-screen rendering then scrolls through one virtual document rather than treating images as a fixed prefix and text as a separate scrollable body.

- [x] On body load, clear cached wrapped body lines and preview document layout for the selected message.
- [x] During full-screen render, choose graphics mode from user override first, autodetection second, and safe fallback last.
- [x] Build the document from HTML when possible, mapping `cid:` references to `InlineImages` by normalized `ContentID`.
- [x] Compute document row heights before clamping scroll offset.
- [x] Clamp scroll offset against total document rows, not just text rows.
- [x] Render visible text/image blocks under the pinned header and above the bottom hint/scroll indicator.
- [ ] When only part of a native raster image is visible, crop and re-encode the visible slice before emitting the terminal graphics escape.
- [x] Update the Creative Commons demo sampler to include HTML/CID placement so demo mode exercises real inline-image positioning.

## Graphics Protocol Selection

Protocol selection should be explicit enough to support future terminals without hard-coding protocol decisions into preview rendering. Autodetection remains the default, but users should be able to override it when terminal detection is wrong.

- [x] Support `auto`, `iterm2`, `kitty`, `links`, `placeholder`, and `off` as internal modes.
- [x] Design the mode enum/API so future protocols such as `sixel` can be added without changing preview document block semantics.
- [x] Prefer iTerm2 when `TERM_PROGRAM` indicates iTerm2 and no user override is set.
- [x] Prefer Kitty graphics when Kitty or Ghostty is detected and no higher-priority user override or iTerm2 detection applies.
- [x] Use local OSC 8 image links only for local TUI sessions where localhost points at the user's machine.
- [x] Use placeholders in SSH auto mode, while allowing explicit raster overrides with `-image-protocol=iterm2` or `-image-protocol=kitty`.
- [x] Expose `-image-protocol` with tests for forced mode selection.

## Error Handling

Image failures should degrade in place inside the scrollable document instead of silently disappearing or corrupting terminal layout. The preview should favor stable row accounting over attempted high-fidelity rendering when a protocol cannot safely draw the image.

- [x] Render `[missing inline image: <cid>]` when HTML references a `cid:` that is not present in `InlineImages`.
- [x] Render `[image unavailable: empty data]` for empty MIME image parts.
- [x] Render a too-large placeholder when an image exceeds the configured inline rendering byte limit.
- [x] Avoid hidden trailing newlines from image renderers; callers should know every consumed row.
- [x] Keep remote image URLs clickable/readable and never fetch them automatically.
- [x] Preserve current safe local-link and SSH placeholder behavior when raster graphics are unavailable.
- [ ] Preserve the existing hide-on-overlap behavior as a safe fallback when an image cannot be decoded or cropped.

## Testing

Unit tests should cover deterministic document construction and row accounting. TUI and manual QA should cover the terminal behaviors that ANSI snapshots cannot prove, especially raster graphics in real iTerm2 and later Kitty/Sixel environments.

- [x] Add unit tests showing HTML `cid:` images become ordered image blocks at their authored positions.
- [x] Add unit tests for orphan inline MIME images, missing CIDs, remote image links, empty images, and oversized images.
- [x] Add unit tests proving iTerm2 and Kitty image row accounting includes raster rows and avoids hidden trailing newlines.
- [x] Add viewport tests proving full-screen rendering never emits more visible rows than the available terminal budget.
- [ ] Add viewport tests proving native raster images are clipped at the top and bottom of the visible preview bounds.
- [x] Update `TUI_TESTPLAN.md` TC-23A to require app-scroll, terminal-native scrollback, and Kitty/Ghostty stale-placement checks for the Creative Commons sampler.
- [x] Update `TUI_TESTING.md` with guidance that tmux can verify layout and escape output but cannot validate actual raster placement.
- [x] Require real-terminal test report evidence for raster modes: terminal app/version, selected protocol mode, screenshots, and ANSI capture when possible.
- [x] Keep the standard `220x50`, `120x40`, `80x24`, and `50x15` size checks.

## Out Of Scope

This design intentionally avoids turning Herald into a full HTML email renderer. The target is trustworthy text plus well-placed, bounded inline images in terminal preview, not CSS-perfect email layout.

- [x] Do not fetch remote image bytes automatically.
- [x] Implement Kitty after the iTerm2 abstraction is stable; Sixel remains out of scope.
- [x] Do not change send/compose inline image MIME behavior.
- [x] Do not redesign split preview chrome or panel layout beyond rendering bounded image blocks inside the existing body viewport.
- [x] Do not attempt CSS table/layout fidelity beyond preserving sensible text flow and image placement.
