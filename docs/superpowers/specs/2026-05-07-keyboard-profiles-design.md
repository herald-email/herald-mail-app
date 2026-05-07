# Keyboard Profiles And Configurable Shortcuts

## Summary

Herald owns a central command catalog for keyboard routing, bottom hints, shortcut help, safety metadata, and profile defaults. The default profile keeps text-entry surfaces insert-first while making browse navigation Vim-compatible: `h/j/k/l` move left/down/up/right, `/` searches the active context, `r` replies all, `R` replies sender-only, `f` forwards, `a` archives the current message, and `D` deletes after confirmation.

## User-Visible Behavior

- [x] Default, Vim, Emacs, and Custom keyboard profiles are exposed through YAML and the Settings panel.
- [x] Custom keymaps live in a separate YAML file so users can keep mappings in version control.
- [x] Custom bindings map keys only to predefined command IDs; unknown command IDs are validation errors.
- [x] Text-entry surfaces preserve literal printable input, including `?`, `/`, and macOS Option-generated characters.
- [x] Vim profile Compose fields use a minimal modal wrapper with normal/insert/visual modes and `i`/`a`/`A`.
- [x] Legacy aliases remain where they do not conflict with text entry or `h/j/k/l` navigation.

## Configuration

Main account config:

```yaml
keyboard:
  profile: default # default | vim | emacs | custom
  custom_keymap: ~/.config/herald/keymaps/work.yaml
```

Custom keymap file:

```yaml
extends: vim
bindings:
  timeline:
    normal:
      r: mail.reply_all
      R: mail.reply_sender
      a: mail.archive_current
fields:
  compose:
    default_mode: normal
```

## Implementation Contract

- [x] Command IDs are stable API-like strings such as `mail.reply_all`, `mail.archive_current`, `pane.left`, `compose.new`, and `help.search`.
- [x] Routing precedence is overlay scope, focused field mode, focused pane scope, then global scope.
- [x] Help and bottom hints advertise the resolved profile and primary remap consistently.
- [x] Bounded multi-key sequences such as `yy` are supported with visible pending-key state and timeout.
- [x] Delete and bulk archive remain confirmed by default; single-message archive may be immediate.

## Acceptance

- [x] Focused tests cover profile defaults, custom YAML validation, command resolution, and text-entry preservation.
- [x] TUI verification captures help/profile/hint behavior at `220x50`, `80x24`, and `50x15`.
- [x] Input-routing safety evidence proves Compose, prompt/editor, and settings fields keep literal text.
- [x] SSH smoke exercises the affected shortcut paths.
