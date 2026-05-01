# Non-Latin Shortcut Layouts

## Intent

This feature lets users keep a non-Latin keyboard layout active while using Herald's browse-mode shortcuts. It matters because frequent commands such as `j`, `k`, `l`, `c`, and `/` should follow the physical keys shown in the TUI instead of forcing users to switch back to an English layout.

- [x] Herald-owned browse shortcuts use Bubble Tea v2 `BaseCode` values so physical QWERTY shortcut positions work across layouts when the terminal reports keyboard enhancements.
- [x] Russian/Ukrainian Cyrillic fallback aliases still work from the same physical QWERTY positions when `BaseCode` is unavailable.
- [x] Direct Japanese kana fallback aliases work from the same physical QWERTY positions when `BaseCode` is unavailable and the terminal sends one committed kana per keypress.
- [x] Text-entry surfaces keep raw native characters, including Compose fields, Timeline search, Contacts search, attachment paths, and AI prompt fields.
- [x] Shortcut help and bottom hints continue to advertise the canonical Latin keys so documentation remains stable.

## Approach

Bubble Tea v2 exposes `BaseCode` and keyboard enhancements, which is now Herald's primary primitive for layout-independent key routing. The compatibility helper prefers `BaseCode`, preserves uppercase and modified commands, and only falls back to printable layout aliases when physical-key data is absent. Japanese romaji IME pre-edit is held by the terminal/input method until text is committed, so true physical shortcuts in that mode depend on `BaseCode` support.

- [x] Add a small `internal/app` shortcut-normalization helper for command matching only.
- [x] Prefer `tea.KeyPressMsg.BaseCode` for physical shortcut matching.
- [x] Map common Cyrillic ЙЦУКЕН-family printable characters to their physical Latin shortcut key as a fallback.
- [x] Map common direct Japanese kana printable characters to their physical Latin shortcut key as a fallback.
- [x] Route command handlers through the helper while continuing to pass raw `tea.KeyPressMsg` values into text inputs and Bubble components.

## Acceptance

Acceptance is split between focused Go tests and tmux/SSH smoke evidence because the core behavior is key routing, while the user-visible promise is a terminal workflow. Demo mode is enough for visual checks because no live IMAP or SMTP behavior is required.

- [x] Unit tests prove `BaseCode` shortcuts normalize to the advertised physical shortcuts with non-Cyrillic examples.
- [x] Unit tests prove Cyrillic fallback aliases normalize to the advertised physical shortcuts.
- [x] Unit tests prove direct Japanese kana fallback aliases normalize to the advertised physical shortcuts.
- [x] Unit tests prove Timeline/Contacts/Cleanup browse handlers respond to layout-independent and fallback keys.
- [x] Unit tests prove Timeline search and Compose text input preserve native typed text.
- [x] TUI demo tmux checks confirm the aliases work at `220x50` and `80x24`.
- [x] SSH build coverage confirms the shared TUI still compiles for the remote surface.
