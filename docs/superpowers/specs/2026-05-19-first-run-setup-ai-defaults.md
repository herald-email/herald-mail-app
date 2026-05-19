# First-Run Setup Wizard And AI Defaults

## Purpose

This spec defines the first-run onboarding polish for setup navigation, provider presets, advanced preference scoping, and AI model recommendations. It matters because the wizard should help a new user reach a working inbox without exposing settings that require prior IMAP or local-model knowledge.

- [x] `Esc` from a first-run wizard step returns to the previous step without validating required fields on the current step.
- [x] `Shift+Tab` keeps normal previous-field movement and can cross to the previous step without validating required fields on the current step.
- [x] The first wizard step remains stable when there is no previous screen to navigate to.

## Account Presets

Provider presets should make their promised defaults visible before the user types. Fields stay editable so a user can correct non-standard local bridges or provider settings.

- [x] ProtonMail Bridge first-run setup pre-populates `127.0.0.1`, IMAP port `1143`, and SMTP port `1025`.
- [x] Fastmail, iCloud, Outlook, and Gmail advanced server setup pre-populate known IMAP and SMTP host/port defaults.
- [x] Switching providers only replaces blank fields or values that still match the previous provider preset.

## Preferences Scope

First-run preferences should stay small and approachable after account validation. Advanced operational controls belong in Settings where the user can revisit them later with more context.

- [x] First-run preferences include AI provider setup, offline-cache policy, keyboard profile, theme, signature, and final save.
- [x] First-run preferences do not show poll interval, IMAP IDLE, reclaim offline cache storage, or auto-cleanup schedule controls.
- [x] In-app `Settings > Sync & Cleanup` keeps poll interval, IMAP IDLE, offline-cache policy, reclaim, and auto-cleanup schedule controls.

## AI Defaults

Local AI defaults should prefer good translation and writing quality while making the hardware cost explicit. The YAML schema stays unchanged so existing configs and scripts keep working, and users on constrained machines can intentionally downgrade through custom Ollama selection.

- [x] Blank/new Ollama chat and classification model defaults to `gemma3:4b`.
- [x] Blank/new Ollama embedding model defaults to `nomic-embed-text-v2-moe`, with `semantic.model` following that default.
- [x] Existing explicit `ollama.model`, `ollama.embedding_model`, and `semantic.model` values are not overwritten.
- [x] The default Ollama wizard copy warns that the recommended defaults are comfortable with at least 16GB RAM, while 8GB can work more slowly.
- [x] Custom Ollama setup offers curated chat choices plus a freeform custom model name, including a downgrade path for constrained machines and a warning that `llama3.x` models are weaker for translation.
- [x] Custom Ollama setup offers curated embedding choices plus a freeform custom model name.
- [x] First-run Ollama setup verifies that the selected chat/classification model and embedding model are installed before writing the initial config.
- [x] Missing first-run Ollama models keep the config unwritten and show exact `ollama pull <model>` commands.
- [x] In-app AI settings validate newly selected or changed Ollama models before applying them.
- [x] Previously saved Ollama configs that become unavailable keep cached/offline startup non-blocking, show `AI down`, disable AI actions, and expose install commands plus a Save Disabled action in Settings > AI.
