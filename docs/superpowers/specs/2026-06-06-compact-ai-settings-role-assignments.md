# Compact AI Settings Role Assignments

This spec defines the user-visible contract for compact AI setup presets and separate role assignments in first-run customization and Settings > AI. It keeps the existing YAML schema compatible while making the UI behave as if vendors and AI capabilities are separate concepts.

## User Experience

The AI setup surface should start from a dense preset chooser instead of a serial provider wizard. Presets set sensible defaults quickly, while advanced manual config exposes role assignment and vendor details on compact screens.

- [x] First-run customization and Settings > AI show an `AI Setup` preset chooser with Ollama local default, OpenAI default, Claude API default, OpenAI-compatible endpoint, AI disabled, and Advanced manual config.
- [x] Choosing a preset populates chat/classification/Compose assistance and embedding assignments without asking one question per screen.
- [x] Advanced manual config exposes `Chat Role` and `Embedding Role` selectors so chat and embeddings can use different configured vendors.
- [x] Save appears on the last useful AI configuration screen when the modal has room, not on a standalone save-only screen.

## Compatibility

The implementation should preserve the current config file shape so existing users do not need a migration. The UI can introduce role language, but saves still write the legacy-compatible fields that the runtime already understands.

- [x] Existing `ai.provider` values load into the chat role, including `ollama`, `openai`, `claude`, and `disabled`.
- [x] Existing `ollama`, `openai`, and `claude` credential/model blocks remain independent and are not cleared unless the user disables AI.
- [x] Existing `semantic.provider` and `semantic.model` load into the embedding role and model fields.
- [x] OpenAI-compatible chat can keep OpenAI-compatible embeddings or switch embeddings to Ollama without re-entering OpenAI credentials.
- [x] Ollama validation checks only selected Ollama chat or embedding models.

## Verification

Verification should prove both the schema-compatible config behavior and the rendered settings surfaces. The focused tests should protect role changes, while tmux captures should protect modal density at common terminal sizes.

- [x] Add focused Settings tests for preset choices, advanced chat/embedding role selectors, and inline AI save.
- [x] Add config or Settings tests proving compatibility backfill from existing Ollama, OpenAI, Claude, disabled, and semantic embedding configs.
- [x] Add a test proving embedding role/model switching preserves chat vendor settings and changes the effective embedding identity.
- [x] Capture Settings > AI and first-run customized AI at `220x50`, `80x24`, and `50x15`.
