---
title: Herald Docs
description: Complete end-user manual for Herald, the terminal email client and inbox cleanup tool.
---

Herald is a keyboard-first terminal email client and inbox cleanup tool. It combines a chronological Timeline, Markdown Compose, bulk Cleanup, Contacts, local caching, optional AI classification, semantic search, quick replies, chat over your mailbox, and integration surfaces for MCP and SSH mode.

This manual is organized around the screens you use every day. Start with setup if you are new, then use the tab pages for precise behavior, controls, states, and privacy notes.

## Fastest path

```sh
git clone https://github.com/herald-email/herald-mail-app.git
cd herald-mail-app
make build
./bin/herald
```

On first launch, Herald opens the setup wizard if `~/.herald/conf.yaml` is missing or empty. Choose a provider, enter credentials or app-password details, decide whether to configure AI, and save the generated config.

<!-- HERALD_SCREENSHOT id="overview-first-launch" page="overview" alt="Herald first-run wizard entry screen" state="fresh config, 120x40" desc="Shows the initial setup path users see before connecting a real mailbox." capture="vhs docs media; rm -f /tmp/herald-docs-wizard.yaml; launch ./bin/herald -config /tmp/herald-docs-wizard.yaml" -->

![Herald first-run wizard entry screen](/screenshots/overview-first-launch.png)

## Main features

- [Timeline](/using-herald/timeline/) lists mail chronologically, groups threads, opens split or full-screen previews, supports search, quick replies, attachment saves, starring, reading, reply, forward, and text copy.
- [Compose](/using-herald/compose/) sends new mail, replies, and forwards with To/CC/BCC fields, address autocomplete, Markdown preview, attachments, drafts, and optional AI assistance.
- [Cleanup](/using-herald/cleanup/) groups mail by sender or domain for bulk delete, archive, hide-future-mail rules, unsubscribe actions, automation rules, custom prompts, and cleanup schedules.
- [Contacts](/using-herald/contacts/) lists known senders, opens contact details, shows recent mail, previews messages inline, and supports keyword or semantic contact search.
- [Global UI](/using-herald/global-ui/) covers the tab bar, folder sidebar, status bar, key hints, logs overlay, chat panel, focus cycling, and narrow terminal behavior.
- [Feature guides](/features/search/) cover cross-tab behavior such as search, AI, destructive actions, rules, attachments, text selection, settings, and sync status.
- [Advanced guides](/advanced/mcp/) cover MCP, SSH mode, daemon commands, demo GIF generation, and privacy/security expectations.

## Local docs commands

```sh
cd docs
npm install
npm run dev
```

Use `npm run build` to verify the docs compile.
