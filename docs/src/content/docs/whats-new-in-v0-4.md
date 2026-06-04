---
title: What's New in v0.4
description: User-facing changes in the v0.4 beta release line through v0.4.1.
---

The v0.4 beta line made Herald feel more like a polished terminal app instead of a raw mailbox table. It focused on safer reading, stronger local AI defaults, a configurable visual system, and a first-run path that validates accounts before saving them.

## Release Delta

These are the changes most visible to people upgrading from `v0.3.0-beta.1`. The theme of the release is better setup, richer Compose assistance, and terminal UI polish that later v0.5/v0.6 work builds on.

- [x] Compose gained AI writing tools with quick rewrites, style and length changes, subject suggestions, review mode, error handling, and immediate model-setting updates.
- [x] Local AI defaults moved to stronger Ollama models, and setup/settings validation checks configured model availability before saving new AI choices.
- [x] First-run setup became more guided, including account connection validation, clearer offline cache policy choices, and a simplified theme step.
- [x] Preview reading became faster and safer with MIME rendering fixes, body-load telemetry, active-folder preview prewarming, offline cache policies, stricter cache pruning, and manual reclaim controls.
- [x] Timeline gained the first grouping switch for sender/domain cleanup review, starting the path toward retiring the separate Cleanup browse tab in v0.5.
- [x] Herald added the configurable theme system: inherited terminal colors, built-in themes, theme settings selection/editor separation, color pickers, a docs gallery, and a local YAML example.
- [x] Modifier-aware key hints teach valid Shift, Ctrl, and Alt commands without changing shortcut behavior.
- [x] Release-channel docs clarified `beta-latest` ownership and Homebrew install telemetry, while the docs gained richer theme and setup guidance.

## How To Try It

Use a current release for everyday work. To inspect the historical v0.4 line, check out an immutable tag and build from source:

```sh
git checkout v0.4.1-beta.1
make build
./bin/herald --demo
```

## Next Release

The v0.5 release moved cleanup browsing into Timeline, added Calendar as a top-level surface, and laid the broad source-identity foundation. See [What's New in v0.5](/whats-new-in-v0-5/) for that release checklist.
