# SP6 — Compose AI Writing Assistant

## Overview

An AI command bar in Compose that opens by default and rewrites, translates, fixes typos, changes style/tone, shortens, or expands the email body without narrowing the editor. It keeps Translate, Style, quick actions, undo, and a freeform chat-style instruction input in one compact row, suggests subject lines, supports one-step undo after accepting a rewrite, strips prompt scaffolding from model responses, and shows word-level diffs so the user can edit the AI suggestion before accepting it.

---

## Features

This section lists the user-visible writing-assistant capabilities that Compose must expose. Each item should be observable from the AI command bar or its accept/dismiss flows.

- [x] **Body rewrite** — quick actions for Improve, Fix typos, Shorten, and Expand
- [x] **Default-open command bar** — every Compose entrypoint shows the compact one-row AI bar immediately without stealing body text input
- [x] **Disabled warning** — when no AI provider is configured, the default bar says `AI disabled` and hides active rewrite controls
- [x] **Translation dropdown** — Translate opens a language dropdown in the AI bar and rewrites into the selected language
- [x] **Style dropdown** — Style opens a style dropdown in the AI bar and rewrites in the selected style
- [x] **Freeform instruction chat** — the inline `Ask:` field accepts natural-language writing requests such as "make this warmer and translate it to Spanish"
- [x] **Word-level diff** — only changed words highlighted inline (red strikethrough for deletions, green for additions); unchanged words stay plain
- [x] **Editable suggestion** — AI response placed in an editable textarea; user modifies before accepting
- [x] **Clean suggestion body** — request context, `Current draft:` echoes, and demo/context explanations are removed before review display
- [x] **Accept all** — `Ctrl+Enter` copies the (possibly edited) AI version into the compose body
- [x] **Toolbar continuity** — after accepting or dismissing review mode, the compact AI command bar remains available for another action
- [x] **Undo accepted rewrite** — `Ctrl+Z` restores the previous body after accepting an AI suggestion
- [x] **Subject suggestion** — `Ctrl+J` fires a subject hint; `Tab` to accept, `Esc` to dismiss
- [x] **Thread context** — by default uses the full reply thread as context; toggle to "Draft only" in panel

---

## UX

This section defines the terminal interaction model for the Compose AI panel. The panel should remain compact enough for common terminal sizes while still making the rewrite actions discoverable.

### Command bar layout (between headers and body, open by default)

```
AI  Translate: Spanish v  Style: Friendly v  [Fix] [Shorten] [Expand] Undo  Ask: > make this warmer
──────────────────────────────────────────────────────────────────────────────
┌─ AI Assist · Suggestion replaces draft ────────────────────────────────────┐
│ Suggestion (edit freely)                                                    │
│ Hi Alice, ...                                                              │
│ Changes · word diff                                                        │
│ ~~Hey~~ Hi Alice, ...                                                      │
│ Accept ctrl+enter  │  Discard esc  │  Undo ctrl+z  │  Tab original/suggestion
└────────────────────────────────────────────────────────────────────────────┘
```

### Subject suggestion overlay

When `Ctrl+J` is pressed from anywhere in compose:
```
Subject: [Quick sync tomorrow?]  ✨ Suggested: "Meeting request: project updates" — Tab to accept
```

### Keyboard shortcuts

| Key | Action |
|-----|--------|
| `Ctrl+K` | Focus the inline `Ask:` instruction field |
| `Ctrl+J` | Suggest subject |
| `Ctrl+Enter` | Accept AI suggestion |
| `Ctrl+T` | Open Translate dropdown |
| `Ctrl+Y` | Open Style dropdown |
| `Ctrl+F` | Fix typos |
| `Ctrl+N` | Shorten draft |
| `Ctrl+E` | Expand draft |
| `Ctrl+Z` | Undo accepted AI rewrite |
| `Tab` | Accept subject hint (when visible) |
| `Esc` | Dismiss dropdown, AI bar, or subject hint |

---

## Architecture

This section describes how the Compose-owned UI state coordinates AI requests without changing backend boundaries. The assistant continues to use the configured AI client and keeps generated text editable until the user accepts it.

### New model fields (`internal/app/app.go`)

```go
// Compose AI panel
composeAIPanel       bool
composeAIInput       textinput.Model  // free-form prompt
composeAIResponse    textarea.Model   // editable AI rewrite
composeAIDiff        string           // rendered word-level diff (lipgloss styled)
composeAIOriginal    string           // draft snapshot used for the request
composeAIShowOriginal bool            // true while review shows original draft
composeAILoading     bool
composeAIThread      bool             // true = use thread context (default)
composeAISubjectHint string           // pending subject suggestion ("" = none)
```

### New message types

```go
// AIAssistMsg carries the AI body rewrite result.
type AIAssistMsg struct {
    Result string
    Original string
    Err    error
}

// AISubjectMsg carries the AI subject suggestion result.
type AISubjectMsg struct {
    Subject string
    Err     error
}
```

### New commands (`internal/app/helpers.go`)

**`aiAssistCmd(instruction string)`**
1. Snapshots `composeBody.Value()` as the draft
2. If `composeAIThread == true` AND `replyToMessageID != ""` (i.e. this is a reply): fetches thread context from `m.replyThread` (already loaded when reply was initiated) and prepends to context. For new emails (no thread), `composeAIThread` is forced false and the context toggle is hidden in the panel.
3. Calls `m.ai.Chat([]ChatMessage{system, user})` where system = "You are an email writing assistant. Rewrite the email body following the instruction. Return only the rewritten body, no explanation." and user = `instruction + "\n\nDraft:\n" + draft`
4. Returns `AIAssistMsg{Result: response, Original: draft}`

**`composeAIQuickAction(key string)`**
1. Maps AI-bar quick actions to immediate rewrite instructions.
2. Immediate actions validate that the draft has content, set the loading state, and call `aiAssistCmd`.

**`selectComposeAIMenuOption(key string)`**
1. Applies the selected Translate or Style dropdown option.
2. Translate options build instructions such as `Translate this email to French`.
3. Style options build instructions such as `Rewrite this email in a direct style`.
4. A custom Translate option focuses `composeAIInput` with `Translate this email to ` so the user can type an unlisted language.

**`aiSubjectCmd()`**
1. Snapshots `composeBody.Value()` + thread context
2. Calls `m.ai.Chat()` with prompt: "Suggest a concise email subject line (max 10 words) for this email. Return only the subject, no explanation."
3. Returns `AISubjectMsg{Subject: response}`

### Word-level diff (`internal/app/helpers.go`)

```go
// wordDiff computes a word-level diff between original and revised text
// and returns a lipgloss-styled string with deletions in red strikethrough
// and additions in green. Unchanged words are unstyled.
func wordDiff(original, revised string) string
```

Implementation: tokenize both strings into words+whitespace+punctuation tokens, run Myers diff on the token slices, render each token with appropriate lipgloss style.

Tokens are split on word boundaries (spaces, newlines, punctuation) so that "sync" and "catch-up" diff correctly without marking the whole sentence as changed.

### Rendering (`internal/app/helpers.go`)

**`renderComposeAIBar(width int) string`**
- Returns `""` when `composeAIPanel == false`
- Shows an `AI disabled` warning and suppresses action controls when no AI provider is configured
- Shows spinner when `composeAILoading == true`
- Shows Translate and Style dropdown triggers, typo-fix action, undo status, and the inline freeform instruction input in one compact toolbar row
- Shows a dropdown row when Translate or Style is open

**`renderComposeAIResult(width int) string`**
- Shows only a bounded loading state when a rewrite is pending
- Does not render an appended suggestion panel below the compose body; completed suggestions are handled by review mode in the main editor slot

**`renderComposeView()` changes**
- When `composeAIPanel == true`: command bar renders between Subject and the body, and body height is recalculated for dropdown/loading/result rows
- When `composeAISubjectHint != ""`: hint line appended below Subject row with `Tab=accept Esc=dismiss` indicator
- When `composeAIResponse` has content: the editable suggestion replaces the main body editor until accepted or dismissed, and `Tab` toggles between Suggestion and Original review views

### Update() handlers

- Compose entrypoints call `resetComposeAIBar()` so the bar starts open with clean menu, diff, prompt, response, and undo state
- `Ctrl+K`: focus the inline AI instruction field
- `Ctrl+J`: dispatch `aiSubjectCmd()`, set `composeAILoading = true`
- `Ctrl+T`: open Translate dropdown
- `Ctrl+Y`: open Style dropdown
- `Ctrl+F`: run typo-fix rewrite
- `Ctrl+N` / `Ctrl+E`: run shorten or expand rewrites
- `Ctrl+Z`: restore the prior body after an accepted rewrite
- `Ctrl+Enter` (AI review open): copy `composeAIResponse.Value()` into `composeBody.SetValue()`; close review mode while keeping the AI command bar available
- `Esc` (AI review open): close the review and restore focus to the draft body without mutating `composeBody`, while keeping the AI command bar available
- `Tab` (AI review open): toggle between the editable Suggestion view and read-only Original view
- `Tab` (subject hint visible): `composeSubject.SetValue(composeAISubjectHint)`; clear hint
- `AIAssistMsg`: clean prompt scaffolding from result, set `composeAIResponse` content to the clean suggestion, preserve the original draft snapshot, compute `composeAIDiff = wordDiff(original, suggestion)`, focus AI review, and set `composeAILoading = false`
- `AISubjectMsg`: set `composeAISubjectHint`; `composeAILoading = false`

---

## Error handling

This section captures the bounded failure states that must be visible to the user. Failures should leave Compose responsive and should never mutate the draft body.

- [x] AI call fails: show error in `composeStatus` bar ("AI assistant unavailable"), clear loading state
- [x] AI rewrite refusals: request structured rewrite responses, treat provider refusal/error payloads or common refusal prose as bounded Compose status warnings, and never place refusal text into the editable suggestion
- [x] No AI configured (`m.ai == nil`): Compose opens with an `AI disabled` warning in the bar, suppresses active rewrite controls, and shows a bounded status if an AI action is attempted
- [x] Empty body when requesting assist: show status "Write something first"

---

## Files changed

This section tracks the implementation files that carry the feature. It is intentionally concrete so future maintenance can quickly find the Compose AI surface area.

| File | Change |
|------|--------|
| `internal/app/app.go` | New fields and message types for Compose AI rewriting |
| `internal/app/compose.go` | `renderComposeAIBar()`, dropdown handling, one-step AI undo, `wordDiff()`, `aiAssistCmd()`, `aiSubjectCmd()`, updated `renderComposeView()`, updated `handleComposeKey()` |
| `internal/app/statusbar.go` and `internal/app/modifier_hints.go` | Disabled-AI hint text while the default bar is visible |
| `internal/app/snapshot_test.go` | Snapshot: compose with AI panel open and diff visible |

---

## Testing plan

This section defines the checks required before handoff. The tests cover prompt/action mapping, diff rendering, snapshot rendering, and the manual TUI flow.

### Unit tests
- `TestWordDiff_SingleWordChange` — "Hey" → "Hi" produces one deletion + one addition token
- `TestWordDiff_Unchanged` — identical strings produce no highlighted tokens
- `TestWordDiff_PhraseSwap` — multi-word replacement highlights only the changed words
- `TestComposeAIQuickActionInstructions` — quick actions and dropdown instruction builders map to typo-fix, style, translation, and length instructions
- `TestComposeAITranslateDropdownCustomPrefillsFreeformInstruction` — custom Translate focuses the freeform prompt with editable guidance instead of guessing
- `TestAcceptComposeAIResponseStoresUndoBody` — accepting a rewrite stores the prior body and `Ctrl+Z` can restore it
- `TestComposeAIBarOpensByDefaultForBlankCompose` — blank Compose starts with the AI bar visible and body focus preserved
- `TestComposeAIBarShowsDisabledWarningWhenAIUnavailable` — no configured AI renders the disabled warning instead of action controls
- `TestComposeAIBarRendersCompactInlineAskToolbar` — the toolbar renders as one row with grouped controls and inline `Ask:`
- `TestComposeCtrlKFocusesInlineAIInstruction` — `Ctrl+K` focuses the inline custom-instruction input
- `TestComposeAIInputEnterSubmitsCustomRewrite` — `Enter` from the inline input dispatches a custom rewrite
- `TestDefaultOpenComposeAIBarDoesNotStealBodyText` — normal typed letters still enter the body while the bar is open

### Snapshot test
- `TestSnapshot_ComposeAIPanel` — panel open, suggestion loaded, diff visible at 120×40

### Manual (tmux)
- AI command bar opens by default as one compact row between headers and body without narrowing the editor
- `Ctrl+K` focuses the inline `Ask:` field
- Quick actions include typo-fix, translation dropdown, style dropdown, and length adjustments
- Accepted AI rewrites can be undone once
- Freeform prompt works for natural-language writing directions
- `Ctrl+Enter` accepts and closes panel
- `Ctrl+J` shows subject hint; `Tab` accepts
- Thread context toggle works (verify different output vs. draft-only)
- No AI configured → default `AI disabled` warning, no active rewrite controls, no panic
