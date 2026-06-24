---
title: What's New in v0.7
description: User-facing changes in the v0.7 beta release line through v0.7.3.
---

The v0.7 beta line turns Herald's source-aware mail and calendar foundation into a more memory-aware writing and review workspace. Through `v0.7.3-beta.1`, the headline work is local Herald Memories, the Gollem-backed chat agent path, Compose Radar, macOS preview printing, denser everyday Timeline polish, and safer multi-account mail behavior.

## Release Delta

These are the changes most visible to people upgrading from the v0.6 beta line. The theme of the release is source-backed context: Herald can use local relationship memory, typed chat intents, and safer review surfaces without giving AI permission to mutate mail or calendar state directly.

- [x] Herald Memories stores local, source-backed relationship context for people, companies, threads, open loops, Obsidian-friendly Markdown sync, and daily briefing diffs.
- [x] Compose Radar shows bounded memory nudges in reply drafts when there is high-confidence evidence, while leaving drafts unchanged until the user chooses an action.
- [x] The chat panel now uses the Gollem-backed UI chat agent path with read-only email and memory tools plus typed Timeline, summary, people, and Compose review intents.
- [x] Chat can project typed Timeline results, so search-like answers can narrow the Timeline without relying on prose filters or legacy control markup.
- [x] macOS email-preview printing opens the standard print dialog for Original Visual or Rendered Markdown modes while preserving remote-image privacy.
- [x] Timeline reading gained range selection, faster chat reply handling, chat drawer scrolling/focus fixes, and clearer responsive layout coverage.
- [x] AI setup and Settings gained role-based chat and embedding provider choices, external embedding provider support, managed AI scheduling, and repair states for unavailable local models.
- [x] Shared chat retrieval planning makes search-like chat answers, daemon tools, and MCP reads use the same structured query path.
- [x] Cross-account fixes keep Google invitation imports, IMAP new-mail sync, archive state, and bulk cleanup mutations scoped to the right account and message refs.
- [x] Preview and Compose polish clears stale inline images before drafting and waits for a loaded preview to dwell before marking a message read.
- [x] Docs and demo-first onboarding now emphasize the v0.7 product state, including Memories, Gollem chat, AI role assignments, and the safer first five minutes loop.

## Screenshots

These screenshots use committed demo-mode media from the docs site so the page can render without touching a real inbox, calendar, AI provider, or private memory store. The dedicated Memories guide also tracks a Compose Radar screenshot capture for the next media refresh.

![AI provider settings with role assignments](/screenshots/settings-ai-provider.png)

![Chat panel open beside Timeline](/screenshots/chat-panel-open.png)

![Timeline filtered by chat result](/screenshots/chat-filtered-timeline.png)

![Timeline range selection in a themed terminal](/screenshots/showcase-range-selection-pastel-dark.png)

![Compose AI assistant panel](/screenshots/compose-ai-assistant.png)

## Beta Notes

The v0.7 beta line started as a capability release and then tightened cross-account behavior, cleanup mutations, and reading polish. This breakdown helps testers decide what to re-check after upgrading from v0.6 or from an earlier v0.7 beta.

- [x] `v0.7.0-beta.1` adds Herald Memories, Compose Radar, Gollem chat intents and tools, macOS preview printing, AI role assignment polish, and Timeline/chat hardening.
- [x] `v0.7.1-beta.1` adds the shared chat retrieval planner and fixes cross-account Google invitation imports.
- [x] `v0.7.2-beta.1` aligns mail reply and archive shortcuts, then batches bulk cleanup/deletion by stable message refs for safer multi-account mutations.
- [x] `v0.7.3-beta.1` scopes new IMAP emails to their account, prunes archived emails from source state, clears stale inline images before Compose, and delays preview read marking until a loaded message has stayed open for 2.5 seconds.
- [x] `beta-latest` points at `v0.7.3-beta.1`, and Homebrew installs the same release-built macOS binaries.
- [x] The release artifacts include `herald`, `herald-mcp-server`, and `herald-ssh-server` for both Apple Silicon and Intel macOS.

## How To Try It

Demo mode remains the safest path for exploring the visible workflows. Memories and chat features are local and optional; AI-backed chat, Compose suggestions, semantic search, and memory extraction require configured AI before they can produce live assistant output.

```sh
brew tap herald-email/herald
brew install herald
herald --demo
```

From demo mode, press `g` to open chat, `S` to inspect AI and Memories settings, `V` then `j`/`k` to try Timeline range selection, and `R` from a Timeline row to open a reply draft where Compose Radar can appear when source-backed memory is available.

## Previous Release

The v0.6 release line graduated Gmail OAuth onto the Gmail API mail source, made Calendar provider-backed, and hardened multi-account source identity. See [What's New in v0.6](/whats-new-in-v0-6/) for that historical release checklist.
