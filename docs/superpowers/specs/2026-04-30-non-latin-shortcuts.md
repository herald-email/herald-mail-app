# Non-Latin Shortcut Layouts

## Intent

This feature lets users keep non-US and non-Latin keyboard layouts active while using Herald's browse-mode shortcuts. It matters because printable Latin layouts such as QWERTZ should produce their actual characters, while non-Latin layouts still need compatibility aliases for frequent commands such as `j`, `k`, `l`, `c`, and `/`.

- [x] Herald-owned browse shortcuts prefer Bubble Tea v2 printable associated text for Latin/ASCII command keys so QWERTZ, AZERTY, and other non-US Latin layouts stay layout-correct when the terminal reports keyboard enhancements.
- [x] `BaseCode` remains available for non-printable navigation keys and for non-Latin physical shortcut compatibility when associated text is not a Latin/ASCII command character.
- [x] Russian/Ukrainian Cyrillic fallback aliases still work from the same physical QWERTY positions when `BaseCode` is unavailable.
- [x] Direct Japanese kana fallback aliases work from the same physical QWERTY positions when `BaseCode` is unavailable and the terminal sends one committed kana per keypress.
- [x] Text-entry surfaces keep raw native characters, including Compose fields, Timeline search, Contacts search, attachment paths, and AI prompt fields.
- [x] Shortcut help and bottom hints continue to advertise the canonical Latin keys so documentation remains stable.

## Approach

Bubble Tea v2 exposes printable `Text`, `BaseCode`, and keyboard enhancements. The compatibility helper prefers printable Latin/ASCII associated text, preserves uppercase and modified commands, falls back to `BaseCode` for non-printable or non-Latin physical-key compatibility, and only uses printable layout aliases when physical-key data is absent. Japanese romaji IME pre-edit is held by the terminal/input method until text is committed, so true physical shortcuts in that mode depend on `BaseCode` support.

- [x] Add a small `internal/app` shortcut-normalization helper for command matching only.
- [x] Prefer `tea.KeyPressMsg.Text` for printable Latin/ASCII shortcut matching.
- [x] Use `tea.KeyPressMsg.BaseCode` only when printable text is absent or represents non-Latin physical shortcut compatibility.
- [x] Map common Cyrillic ЙЦУКЕН-family printable characters to their physical Latin shortcut key as a fallback.
- [x] Map common direct Japanese kana printable characters to their physical Latin shortcut key as a fallback.
- [x] Route command handlers through the helper while continuing to pass raw `tea.KeyPressMsg` values into text inputs and Bubble components.

## Acceptance

Acceptance is split between focused Go tests and tmux/SSH smoke evidence because the core behavior is key routing, while the user-visible promise is a terminal workflow. Demo mode is enough for visual checks because no live IMAP or SMTP behavior is required.

- [x] Unit tests prove QWERTZ-style printable associated text wins over US `BaseCode` for `y`, `z`, and `?`.
- [x] Unit tests prove `BaseCode` shortcuts still normalize to advertised physical shortcuts for non-Latin examples.
- [x] Unit tests prove Cyrillic fallback aliases normalize to the advertised physical shortcuts.
- [x] Unit tests prove direct Japanese kana fallback aliases normalize to the advertised physical shortcuts.
- [x] Unit tests prove Timeline/Contacts/Cleanup browse handlers respond to layout-independent and fallback keys.
- [x] Unit tests prove Timeline search and Compose text input preserve native typed text.
- [x] TUI demo tmux checks confirm the aliases work at `220x50` and `80x24`.
- [x] SSH build coverage confirms the shared TUI still compiles for the remote surface.
