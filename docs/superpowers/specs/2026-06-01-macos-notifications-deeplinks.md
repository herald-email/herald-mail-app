# macOS Notifications And Herald Deep Links

This spec defines the v1 notification and deep-link behavior for issue #32. The goal is useful local notifications that can return a macOS user to the relevant Herald mailbox context while keeping non-macOS builds honest and harmless.

## User-Visible Behavior

- [ ] macOS users can receive native notifications for new mail and sync failures by default.
- [ ] A clicked macOS notification activates the running TUI through a Herald deep link when the native bridge receives the response.
- [ ] Non-macOS users do not see broken click-through UI; supported platforms may deliver notifications without activation, and unsupported platforms no-op cleanly.
- [ ] Demo mode can exercise deep-link activation through `--open` without IMAP, SMTP, Ollama, or real notification delivery.

## Deep-Link Format

- [ ] Folder context: `herald://mail/folder?folder=INBOX&source_id=...&account_id=...`.
- [ ] Message context: `herald://mail/message?folder=INBOX&message_id=...&local_id=...`.
- [ ] Sender context: `herald://mail/sender?folder=INBOX&sender=alerts%40example.com`.
- [ ] Search context: `herald://mail/search?folder=INBOX&q=invoice`.
- [ ] Compose context: `herald://mail/compose?to=a%40b.com&subject=Hello`.
- [ ] Unknown schemes, hosts, contexts, or missing required fields fail closed with a parse error.

## Notification Events

- [ ] `new_mail` is enabled by default; one new email links to the message, and multiple new emails link to the folder.
- [ ] `sync_failures` is enabled by default and links to the affected folder.
- [ ] `deletion_completion`, `classification_completion`, and `chat_results` are supported but disabled by default.
- [ ] Rule action `notify` uses the same notification delivery path and attaches a message deep link when the triggering email is known.
- [ ] Notification failures are logged or surfaced as bounded status feedback without interrupting mail sync or TUI updates.

## Platform Boundary

- [ ] `internal/notifications` owns platform-specific delivery behind a small notifier interface.
- [ ] Darwin uses a native `UNUserNotificationCenter` bridge and returns clicked notification deep links through a response channel.
- [ ] Linux preserves the existing `notify-send` delivery-only behavior.
- [ ] Other platforms implement a no-op notifier that reports unsupported capability without failing normal app behavior.

## Acceptance And Verification

- [ ] Unit tests cover deep-link parse/build, invalid links, config defaults, notifier fake behavior, rule notify dispatch, TUI activation, and CLI `--open`.
- [ ] Darwin-specific code compiles behind build tags, while non-Darwin package tests prove clean degradation.
- [ ] Tmux demo captures prove `--open` activation at `220x50`, `80x24`, and `50x15`.
- [ ] The final report records `demo`, `tmux`, and `code` evidence, and records `macos` native click-through limits if Notification Center cannot be exercised headlessly.
