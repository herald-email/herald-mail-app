---
title: What's New in v0.12
description: See external calendar edits in an open Herald session with v0.12.0-beta.1.
---

When someone renames or reschedules a meeting in Google Calendar, Herald updates the agenda and open event details during your session.

## Release Delta

Calendar checks for updates every minute. The provider sync interval is five minutes, so allow up to roughly six minutes for an external edit to appear.

- [x] Keep the same event selected when its time changes and it stays visible in Agenda, Day, Week, or 3-Day.
- [x] Keep working in Timeline while the calendar refreshes in the background.

## Beta Notes

This is the first beta on the v0.12 line. Herald defers refreshes during event editing so your draft stays intact.

- [x] Keep cached events visible during a failed refresh and retry on subsequent ticks.

## How To Try It

Upgrade Herald, then open Calendar and select a test event. Change its title in Google Calendar and leave Herald open until the next sync.

```sh
brew update
brew upgrade herald
herald --version
```

## Previous Release

For multi-account mail polling and sign-in recovery, see [What's New in v0.11](/whats-new-in-v0-11/).
