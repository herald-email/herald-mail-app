# Modifier-Aware Key Hints

This spec defines a presentation-only improvement for Herald's bottom key-hint bar. The goal is to teach users what Shift, Ctrl, and Alt do in the current context while preserving every existing shortcut and safety gate.

## Problem

The bottom hint bar is dense because it tries to advertise default, shifted, and control-key actions at the same time. Users can miss that uppercase letters, shifted arrows, and Ctrl chords are meaningful, especially at `80x24` where hint space is scarce.

- [x] Baseline captures recorded the default hint bar density in Timeline and preview states.
- [x] Baseline review confirmed existing Shift and Ctrl actions were only discoverable when they fit in the default hint set.
- [x] The implementation keeps modifier-aware hints informational and does not bypass confirmations.

## Goals

This pass should make modifier behavior discoverable without turning the hint bar into a new keybinding system. The feature is successful when the visible hints change with supported modifier state, but command routing remains governed by the existing catalog and handlers.

- [x] Pressing or holding Shift, Ctrl, or Alt in terminals that report event types changes only the bottom hint presentation.
- [x] Terminals without key-release reporting show the relevant modifier layer briefly after a modified keypress, then return to default hints.
- [x] The Shift layer advertises existing shifted or uppercase actions such as `Shift+Tab`, `Shift+Up/Down`, `R`, `D`, `C`, `S`, and other context-valid uppercase actions.
- [x] The Ctrl layer advertises existing Ctrl actions such as `Ctrl+C`, `Ctrl+R`, `Ctrl+D/U`, `Ctrl+I`, `Ctrl+Q`, `Ctrl+S`, `Ctrl+P`, `Ctrl+A`, and `Ctrl+K` only where those actions are already valid.
- [x] The Alt layer advertises existing Alt actions where a context already owns them; otherwise it keeps default hints visible with a compact no-Alt-actions notice.

## Behavior Contract

The modifier layers are derived from Herald's existing command and context state. They must not introduce new commands, new destructive shortcuts, or different precedence rules.

- [x] Existing shortcuts, aliases, and custom keymap resolution continue to decide what happens when a key is pressed.
- [x] Delete, archive, unsubscribe, and draft-send confirmation prompts still require their existing explicit confirmation keys.
- [x] `?` shortcut help remains the complete command reference and stays available from browse and non-text contexts.
- [x] Text-entry surfaces keep literal input, including printable characters produced with Option/Alt or non-Latin layouts.
- [x] Multiple active modifiers prefer Ctrl, then Alt, then Shift for the hint layer so the displayed copy stays deterministic.

## Verification

Testing must prove both the new hint presentation and the preserved routing behavior. The visual gate should compare the same demo states before and after the change so hint density and minimum-size behavior remain visible.

- [x] Go tests cover modifier press/release tracking, fallback expiry, layer precedence, and layer-specific hint copy.
- [x] Go tests confirm normal command routing and confirmation prompts are unchanged.
- [x] Input-routing checks prove Compose, search/prompt, and editor-like fields preserve literal text.
- [x] tmux captures compare Timeline list, Timeline preview, Cleanup summary, and Compose at `220x50`, `80x24`, and `50x15`.
