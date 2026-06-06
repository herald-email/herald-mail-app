---
title: Herald Docs
description: End-user manual for Herald, the GUI-like terminal mail and calendar workspace with low-friction setup.
---

Herald is a GUI-like, terminal-native mail and calendar workspace built to keep setup from becoming a project. Try demo mode before connecting an inbox, click or scroll when that is comfortable, use keys when faster, press `?` for context help, and keep AI optional and local-first for classification, semantic search, quick replies, or chat.

It combines a chronological Timeline, transient Markdown Compose, Contacts, Calendar, local caching, bulk cleanup, hardened preview links, and integration surfaces for MCP and SSH mode.

This manual is organized around the screens you use every day. Start with setup if you are new, then use the tab pages for precise behavior, controls, states, and privacy notes.

## Fastest path

```sh
brew tap herald-email/herald
brew install herald
herald
```

On macOS, Homebrew is the default install path and includes release binaries
with Google OAuth defaults built in for Gmail and Google Calendar setup.

For source installs or development:

```sh
go install github.com/herald-email/herald-mail-app/cmd/herald@latest
herald

# Or build from a checkout:
git clone https://github.com/herald-email/herald-mail-app.git
cd herald-mail-app
make build
./bin/herald
```

Source-built OAuth flows need local Google OAuth defaults or runtime variables. See [Local OAuth Builds](/development/local-oauth-builds/) before using Gmail OAuth or Google Calendar OAuth from a checkout.

On first launch, Herald opens the setup wizard if `~/.herald/conf.yaml` is missing or empty. Choose Gmail OAuth, Gmail IMAP with an App Password, another IMAP provider path, or standard IMAP, decide whether to configure AI, and save the generated config.

Nightly builds are available as short-lived GitHub Actions artifacts for testers who want the latest successful `main` build before the next beta tag. See [Nightly Builds](/nightly-builds/) for download steps and channel rules.

## New in v0.6

The `v0.6.2-beta.1` release line graduates Gmail OAuth onto the Gmail API mail source, hardens multi-account mail behavior, adds provider-backed Calendar mutations and invitation import, and makes previews safer to inspect and copy. See [What's New in v0.6](/whats-new-in-v0-6/) for the release-delta checklist and updated demo GIFs; older release pages remain available under Release Archive so stable links keep working.

![Calendar workspace demo](/demo/calendar-workspace.gif)

<!-- HERALD_SCREENSHOT id="overview-first-launch" page="overview" alt="Herald first-run wizard entry screen" state="fresh config, 120x40" desc="Shows the initial setup path users see before connecting a real mailbox." capture="vhs docs media; rm -f /tmp/herald-docs-wizard.yaml; launch ./bin/herald -config /tmp/herald-docs-wizard.yaml" -->

![Herald first-run wizard entry screen](/screenshots/overview-first-launch.png)

## Mouse-friendly terminal controls

Herald's keyboard model stays complete, but you can also click and scroll the main TUI. Top tabs, folder rows, Timeline rows, and Calendar rows respond to clicks; Timeline lists, Calendar lists, and message previews respond to wheel or trackpad scrolling. Email links render as OSC 8 terminal hyperlinks when supported, so readable labels and shortened URLs still open the original target.

<!-- HERALD_SCREENSHOT id="mouse-navigation-links" page="overview" alt="Mouse navigation and clickable email links in Herald" state="demo mode, 120x40, Timeline preview with OSC 8 links" desc="Shows clickable tabs, a selected Timeline row, a scrollable email preview, and OSC 8-rendered links." capture="tmux demo 120x40; ./bin/herald --demo; search Link rendering stress; open preview; focus preview; scroll to links" -->

![Mouse navigation and clickable email links in Herald](/screenshots/mouse-navigation-links.png)

## Main features

- [Timeline](/using-herald/timeline/) lists mail chronologically, groups threads, opens split or full-screen previews, supports mouse row clicks, preview scrolling, search, quick replies, attachment saves, starring, reading, reply, forward, text copy, and sender/domain grouping for cleanup review.
- [Compose](/using-herald/compose/) opens from Timeline for new mail, replies, forwards, quick replies, and draft editing with To/CC/BCC fields, address autocomplete, Markdown preview, attachments, drafts, account-aware sending, signatures, and optional AI assistance.
- [Calendar](/using-herald/calendar/) shows Week, Day, 3-Day, Agenda, Search, Event Detail, RSVP, and edit surfaces with a colored source rail.
- [Cleanup](/using-herald/cleanup/) now lives in Timeline grouping and Settings Sync & Cleanup managers for bulk delete, archive, hide-future-mail rules, unsubscribe actions, automation rules, custom prompts, and cleanup schedules.
- [Contacts](/using-herald/contacts/) lists known senders, opens contact details, shows recent mail, previews messages inline, and supports keyword or semantic contact search.
- [Global UI](/using-herald/global-ui/) covers the tab bar, folder sidebar, mouse navigation, status bar, `?` shortcut help, key hints, logs overlay, chat panel, focus cycling, and narrow terminal behavior.
- [Feature guides](/features/search/) cover cross-tab behavior such as search, AI, destructive actions, rules, attachments, text selection, settings, themes, and sync status.
- [Advanced guides](/advanced/mcp/) cover MCP, SSH mode, daemon commands, demo GIF generation, and privacy/security expectations.
- [FAQ](/faq/) answers common questions about terminal image previews, multiple accounts, and other sharp edges.

## Local docs commands

```sh
cd docs
npm install
npm run dev
```

Use `npm run build` to verify the docs compile.
