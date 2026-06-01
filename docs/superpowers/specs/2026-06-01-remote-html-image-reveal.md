# Remote HTML Image Reveal

## Purpose

HTML newsletters often publish their images as remote `http` or `https` URLs instead of MIME `cid:` parts. Herald should make those images discoverable and previewable without silently loading remote content or weakening the existing local-inline-image safety model.

## Behavior

- [x] Remote HTML `<img>` tags render in authored order as `image: <label> (press o to reveal)` placeholders in full-screen preview.
- [x] Placeholder text uses the image `alt`, then `title`, then URL host as the label.
- [x] Placeholder text is wrapped in an OSC 8 hyperlink to the original image URL, but the visible preview never shows the raw tracking URL.
- [x] Pressing `o` from Timeline split preview or full-screen preview fetches only the current message's unrevealed remote images.
- [x] `r` and `R` remain reply-all and reply-sender shortcuts.
- [x] Split preview remains compact and shows a linked-image count/hint rather than large raster images.
- [x] Revealed remote images reuse the same full-screen renderer path as MIME inline images: iTerm2, Kitty, local OSC 8 open-image links, or placeholders depending on protocol/session.

## Network Safety

- [x] Herald never fetches remote image URLs automatically.
- [x] Fetches use only HTTP(S), no cookies, no authorization headers, and no referrer.
- [x] Fetches reject localhost, private, link-local, multicast, unspecified, and unsafe redirect destinations.
- [x] Fetches enforce a short timeout, a bounded redirect count, image content-type validation, and the existing inline preview byte ceiling.
- [x] Fetched bytes live only in the current TUI model state and are cleared when the preview message changes, previews are revoked, or the process exits.

## Fixtures And Acceptance

- [x] A sanitized Buttondown-style fixture under `internal/testmail/testdata/corpus/remote-html-images/` covers realistic remote `<img>` markup.
- [x] Demo mode includes linked remote images that can be revealed without internet access.
- [x] Focused tests cover document parsing, placeholder rendering, shortcut routing, fetch safety, demo behavior, and virtual-lab rendering at common terminal sizes.
- [x] Raster-mode evidence includes rendered-image screenshots before scrolling, after scrolling down through the message, farther down the message, and after scrolling back upward, with the screenshots inspected before handoff.
