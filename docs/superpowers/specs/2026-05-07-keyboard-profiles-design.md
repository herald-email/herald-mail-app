# Keyboard Profiles And Configurable Shortcuts

## Summary

Herald owns a central command catalog for keyboard routing, bottom hints, shortcut help, safety metadata, and profile defaults. The Default profile keeps text-entry surfaces insert-first and uses calmer GUI-mail-style preferred shortcuts: `Ctrl+N` new message, `Ctrl+R` reply sender, `Ctrl+Shift+R` reply all, `Ctrl+F` forward, `Delete` confirmed delete, `Shift+Delete` immediate delete, `A` archive, `/` or `Ctrl+K` search, and `F6`/`Shift+F6` pane focus. Legacy terminal/Vim aliases remain active for at least one release and the Vim profile preserves terminal-oriented primaries.

## User-Visible Behavior

- [x] Default, Vim, Emacs, and Custom keyboard profiles are exposed through YAML and the Settings panel.
- [x] Custom keymaps live in a separate YAML file so users can keep mappings in version control.
- [x] Custom bindings map keys only to predefined command IDs; unknown command IDs are validation errors.
- [x] Text-entry surfaces preserve literal printable input, including `?`, `/`, and macOS Option-generated characters.
- [x] Vim profile Compose fields use a minimal modal wrapper with normal/insert/visual modes and `i`/`a`/`A`.
- [x] Legacy aliases remain where they do not conflict with text entry; Default bottom hints show preferred keys only, while `?` help and docs list legacy aliases.
- [x] Default assigns `A` to archive; account switching moves to `Alt+A` in Default browse contexts and re-classify stays on `T`.

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

- [x] Command IDs are stable API-like strings such as `mail.reply_all`, `mail.archive_current`, `pane.left`, `pane.next`, `account.switcher`, `compose.new`, and `help.search`.
- [x] Routing precedence is overlay scope, focused field mode, focused pane scope, then global scope.
- [x] Help and bottom hints advertise the resolved profile and primary remap consistently.
- [x] Bounded multi-key sequences such as `yy` are supported with visible pending-key state and timeout.
- [x] Delete and bulk archive remain confirmed by default; single-message archive may be immediate.

## Acceptance

- [x] Focused tests cover profile defaults, custom YAML validation, command resolution, and text-entry preservation.
- [x] TUI verification captures help/profile/hint behavior at `220x50`, `80x24`, and `50x15`.
- [x] Input-routing safety evidence proves Compose, prompt/editor, and settings fields keep literal text.
- [x] SSH smoke exercises the affected shortcut paths.
