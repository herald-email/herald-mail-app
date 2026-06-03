---
title: Notifications and Deep Links
description: Use desktop notifications and Herald deep links to return to mailbox context.
---

Herald notifications are local-first mailbox nudges. On macOS they use the system notification center and can activate a running Herald TUI through `herald://mail/...` deep links; on other platforms Herald either uses delivery-only support or no-ops cleanly.

## Event Defaults

New mail and sync failures are enabled by default. Completion notifications for delete/archive batches, classification runs, and chat results exist as opt-in events because those can become noisy during regular inbox work.

## Deep-Link Contexts

Herald supports folder, message, sender, search, and compose links:

```text
herald://mail/folder?folder=INBOX
herald://mail/message?folder=INBOX&message_id=<message-id>
herald://mail/sender?folder=INBOX&sender=alerts%40example.com
herald://mail/search?folder=INBOX&q=invoice
herald://mail/compose?to=a%40b.com&subject=Hello
```

Use `--open` for deterministic testing:

```bash
herald --demo --open 'herald://mail/search?folder=INBOX&q=invoice'
```

## macOS Permissions

The first native macOS notification may ask for Notification Center permission for the terminal or Herald binary that launched the TUI. If notifications do not appear, check System Settings > Notifications for that launcher.

## Other Platforms

Linux preserves delivery-only behavior through `notify-send` when available. Unsupported platforms keep the app behavior unchanged and do not advertise click-through activation.
