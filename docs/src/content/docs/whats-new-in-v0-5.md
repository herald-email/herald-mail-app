---
title: What's New in v0.5
description: User-facing changes in the v0.5 beta release.
---

This page summarizes the user-visible work in the `v0.5.0-beta.1` release, including the broad Calendar and multi-account feature wave that landed after `v0.4.1-beta.1`.

## Release Delta

The post-release branch is a broad feature wave rather than a small patch. The main themes are Calendar, multi-source identity, account-aware mail behavior, source-scoped safety, and updated demo/evidence tooling.

- [x] Calendar is now a top-level `3 Calendar` surface with Week Time-Grid, Day Agenda, 3-Day Command, Agenda List, Calendar Search, Event Detail, edit boundaries, RSVP actions, invitation intake, and source rail filtering.
- [x] Calendar providers include Google Calendar OAuth and CalDAV/iCloud-style setup paths, with sync-token or polling fallback behavior recorded in the source platform.
- [x] Source identity is threaded through mail and calendar models using source/account/collection/item refs so multi-account reads and mutations can target the right account.
- [x] Timeline is now the cleanup browse surface: `G` cycles thread, sender, and domain grouping while the old top-level Cleanup tab is retired.
- [x] Multi-account mail work includes account-aware Compose routing, source badges, account-scoped signatures, Mail.app-style folder sections, and safer all-account behavior.
- [x] Search expanded into calendar and cross-source foundations while existing Timeline search remains intact.
- [x] MCP and daemon APIs gained scoped read and mutation refs, calendar read tools, and guardrails that prevent ambiguous multi-account writes.
- [x] Settings now carries account and calendar-source management, Google OAuth readiness, source validation, AI repair states, keyboard profiles, theme selection/editor, and sync/cleanup managers.
- [x] The virtual mail/calendar lab and tmux/ttyd evidence harnesses grew so screenshots and surface checks can be generated without private mailboxes.

## Calendar Screenshots

These screenshots were captured from demo mode with `HERALD_THEME=sonokai-signal` so the calendar source colors, highlighted event rows, and contrast handling are easy to inspect.

![Calendar week time-grid with source rail and inspector](/screenshots/calendar-week-time-grid.png)

![Calendar day agenda with detail drawer](/screenshots/calendar-day-agenda-drawer.png)

![Calendar 3-day command view with open slots](/screenshots/calendar-three-day-command.png)

![Calendar search results with selected event detail](/screenshots/calendar-search-detail.png)

## Documentation Updates

The docs now treat Calendar as a first-class daily surface instead of only a roadmap topic.

- [x] Added a [Calendar guide](/using-herald/calendar/) covering views, controls, workflows, states, privacy, and troubleshooting.
- [x] Updated the docs overview, Global UI guide, and keybinding reference for `1 Timeline`, `2 Contacts`, and `3 Calendar`.
- [x] Added colorful Calendar screenshots under `docs/public/screenshots/`.

## How To Try It

```sh
make build
HERALD_THEME=sonokai-signal ./bin/herald --demo
```

Press `3` to open Calendar, then try `w`, `d`, `t`, `a`, `/`, `j`/`k`, `h`/`l`, `tab`, and `enter`.
