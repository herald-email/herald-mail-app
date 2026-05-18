# Herald Theme System

This spec defines Herald's app-level theme layer. It matters because the TUI should continue to respect terminal color profiles by default while also giving users durable, shareable themes when they want Herald-owned color choices.

## User-Visible Behavior

The first version is Settings-only and local-first. Users switch themes, install local YAML files, and edit semantic roles without adding new global shortcut risk.

- [x] Missing theme config behaves as `theme.name: inherited` and keeps terminal foreground/background inheritance.
- [x] Built-ins are `inherited`, `herald-dark`, and `herald-light`; `adaptive` maps to `inherited`, and `legacy-dark` maps to `herald-dark`.
- [x] Installed themes are loaded from `~/.herald/themes/*.yaml` and cannot override built-in names.
- [x] Invalid installed themes do not crash startup; Herald falls back to inherited and surfaces a bounded warning.
- [x] Settings includes a `Theme` category with theme selection, local YAML install, semantic role editing, xterm-256/hex inputs, xterm-grid and RGB color pickers, swatches, live preview, role reset, reset all, and save-as-new-theme.

## Config And Theme Files

Theme config lives in the main YAML file so each account/config can have its own appearance. Theme files are separate shareable YAML documents stored locally.

- [x] Main config supports `theme.name` and `theme.overrides`.
- [x] Override role IDs use `group.role` snake_case names such as `chrome.status_bar`, `focus.selection_active`, and `severity.error`.
- [x] Theme file colors accept `inherit`, `ansi:N`, `xterm:N`, and quoted `#RRGGBB`.
- [x] Theme settings color pickers write the same color tokens as the config: `/` opens the matching picker from a manual color field, xterm-grid movement emits `xterm:N`, RGB editing emits quoted `#RRGGBB` when saved, and `inherit` remains available.
- [x] Unknown roles, bad color tokens, unsupported versions, and invalid slugs fail validation.
- [x] Local install copies a validated file into `~/.herald/themes/<name>.yaml` with private permissions.

## Runtime Boundaries

The UI owns resolved theme state per model instance. This preserves separate local and SSH sessions using different configs.

- [x] `internal/config` only parses and preserves theme config; `internal/app` resolves roles into Lip Gloss styles.
- [x] `SetConfig` applies the selected theme and recomputes panel/table/log/form styles.
- [x] First-run onboarding remains linear and does not introduce theme steps.
- [x] MCP behavior remains config-compatible but does not render themes.

## Acceptance

The theme system is accepted when code tests and tmux evidence prove the old inherited behavior and new configured behavior both work. Theme editing is considered TUI-facing and must pass the visual evidence gate.

- [x] Unit tests cover config round-trip, alias resolution, YAML validation, local install permissions, override merging, Herald light contrast roles, and per-model theme isolation.
- [x] Settings tests cover the Theme category, immediate switching state, save preservation, install errors, color picker updates, role-specific working overrides, and text-entry safety for theme fields.
- [x] Tmux captures cover inherited, Herald dark, Herald light, Settings Theme, color picker preview, custom override preview, and the minimum-size guard at required sizes.
