---
title: FAQ
description: Short answers to common Herald setup and usage questions.
---

## Why don't I see images in email previews?

Inline images are visible in full-screen preview when your terminal supports a raster image protocol. Herald supports iTerm2's inline image protocol and the [Kitty graphics protocol](https://sw.kovidgoyal.net/kitty/graphics-protocol/), which is also used by terminals such as Kitty and Ghostty.

Run Herald in a compatible terminal, open a message with inline images, and press `z` for full-screen preview. You can also force a protocol when testing:

```sh
herald --demo -image-protocol=kitty
herald --demo -image-protocol=iterm2
```

If your terminal does not support inline graphics, Herald falls back to safe placeholders or local `open image` links when available. Remote HTML images are shown as links and are not fetched automatically.

## Does Herald support multiple accounts?

Yes. Current Herald configs can use `sources[]` entries for multiple mail and calendar sources, and Settings > Accounts can list configured accounts, add mail accounts, add calendar-only sources, and pair supported mail providers with calendars.

You can still run separate Herald instances with different configs when you want hard separation between mailboxes:

```sh
herald -config ~/.herald/personal.yaml
herald -config ~/.herald/work.yaml
```

Keep the account names and cache paths separate so each instance syncs and sends mail through the intended account.
