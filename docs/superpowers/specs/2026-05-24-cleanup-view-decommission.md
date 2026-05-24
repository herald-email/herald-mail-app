# Cleanup View Decommission

## Summary

Herald removes Cleanup as a top-level TUI view and makes Timeline grouping the only browse cleanup workflow. Users review cleanup candidates by pressing `G` in Timeline to cycle thread, sender, and domain grouping, then use Timeline delete/archive/selection/preview controls; automation, prompt, and cleanup-rule management moves to `Settings > Sync & Cleanup`.

## User-Facing Contract

- [x] The top tab bar shows `1 Timeline` and `2 Contacts` only.
- [x] `F1` opens Timeline, `F2` opens Contacts, and `F3` remains a temporary Contacts alias.
- [x] Digit `3` is not advertised as a tab shortcut; it remains available for quick-reply selection only while the quick-reply picker is open.
- [x] Timeline sender/domain grouping uses cleanup-oriented delete/archive confirmation copy that names sender or domain groups, not threads.
- [x] Direct browse shortcuts `W`, `P`, and `C` no longer launch managers; the Settings Sync & Cleanup category owns those launchers.
- [x] Cleanup tab, Cleanup preview, Cleanup selection, and Cleanup mouse flows are intentionally retired from visible TUI navigation.
- [x] Cleanup scheduler, cleanup rules storage, daemon/MCP cleanup APIs, sender stats, and the deletion worker remain unchanged.

## Settings Contract

- [x] `Settings > Sync & Cleanup` includes launchers for automation rules, custom prompts, and cleanup rules.
- [x] Launchers reuse the existing compact rule editor, prompt editor, cleanup manager, and dry-run preview components.
- [x] Settings text fields and editor overlays still accept literal printable input after browse shortcuts are removed.

## Verification

- [x] Focused Go tests cover tab routing, keymap defaults, shortcut help, key hints, Settings launchers, and Timeline grouped delete/archive copy.
- [x] Visual evidence covers Timeline thread/sender/domain modes, Contacts, Settings Sync & Cleanup, and manager overlays at `220x50`, `80x24`, and `50x15`.
- [x] Surface smoke covers TUI build, SSH server build/top tabs, and MCP cleanup-rule tool listing.
