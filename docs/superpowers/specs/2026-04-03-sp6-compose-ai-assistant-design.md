# SP6 — Compose AI Writing Assistant

## Overview

A right-side AI panel in the compose tab that rewrites, shortens, lengthens, or adjusts the tone of the email body, suggests subject lines, and shows word-level diffs of its changes. The user can edit the AI suggestion before accepting it.

---

## Features

- **Body rewrite** — quick actions (Improve, Shorten, Lengthen, Formal, Casual) + free-form prompt input
- **Word-level diff** — only changed words highlighted inline (red strikethrough for deletions, green for additions); unchanged words stay plain
- **Editable suggestion** — AI response placed in an editable textarea; user modifies before accepting
- **Accept all** — `Ctrl+Enter` copies the (possibly edited) AI version into the compose body
- **Subject suggestion** — `Ctrl+J` fires a subject hint; `Tab` to accept, `Esc` to dismiss
- **Thread context** — by default uses the full reply thread as context; toggle to "Draft only" in panel
- **Free-form prompt** — text input for custom instructions ("make this more concise", "add a P.S. about the deadline")

---

## UX

### Panel layout (right column, toggled with `Ctrl+G`)

```
┌─ 🤖 AI ASSISTANT ──────────────────┐
│ Context: [● Thread] [ Draft only]  │
│                                    │
│ [Improve] [Shorten] [Lengthen]     │
│ [Formal]  [Casual]                 │
│                                    │
│ ┌─ Ask AI anything… ─────────────┐ │
│ └────────────────────────────────┘ │
│                                    │
│ SUGGESTION — edit freely           │
│ ┌────────────────────────────────┐ │
│ │ ~~Hey~~ Hi Alice,              │ │
│ │                                │ │
│ │ Are you ~~free~~ available     │ │
│ │ tomorrow for a ~~sync~~        │ │
│ │ catch-up?                      │ │
│ └────────────────────────────────┘ │
│                                    │
│ [✓ Accept  Ctrl+Enter] [Discard]   │
└────────────────────────────────────┘
```

### Subject suggestion overlay

When `Ctrl+J` is pressed from anywhere in compose:
```
Subject: [Quick sync tomorrow?]  ✨ Suggested: "Meeting request: project updates" — Tab to accept
```

### Keyboard shortcuts

| Key | Action |
|-----|--------|
| `Ctrl+G` | Toggle AI panel |
| `Ctrl+J` | Suggest subject |
| `Ctrl+Enter` | Accept AI suggestion |
| `Tab` | Accept subject hint (when visible) |
| `Esc` | Dismiss panel or subject hint |
| `↑` / `↓` | Navigate quick actions (when panel focused) |

---

## Architecture

### New model fields (`internal/app/app.go`)

```go
// Compose AI panel
composeAIPanel       bool
composeAIInput       textinput.Model  // free-form prompt
composeAIResponse    textarea.Model   // editable AI rewrite
composeAIDiff        string           // rendered word-level diff (lipgloss styled)
composeAILoading     bool
composeAIThread      bool             // true = use thread context (default)
composeAISubjectHint string           // pending subject suggestion ("" = none)
```

### New message types

```go
// AIAssistMsg carries the AI body rewrite result.
type AIAssistMsg struct {
    Result string
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
4. Returns `AIAssistMsg{Result: response}`

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

**`renderAIPanel(width int) string`**
- Returns `""` when `composeAIPanel == false`
- Shows spinner when `composeAILoading == true`
- Shows `composeAIResponse.View()` (editable textarea with diff content) when result available
- Quick action buttons rendered as a row of lipgloss-styled labels
- Context toggle rendered as two styled labels

**`renderComposeView()` changes**
- When `composeAIPanel == true`: compose body narrows to ~60% width, panel takes remaining ~40%
- When `composeAISubjectHint != ""`: hint line appended below Subject row with `Tab=accept Esc=dismiss` indicator

### Update() handlers

- `Ctrl+G`: toggle `composeAIPanel`; initialize `composeAIInput` and `composeAIResponse` on open
- `Ctrl+J`: dispatch `aiSubjectCmd()`, set `composeAILoading = true`
- `Ctrl+Enter` (panel open): copy `composeAIResponse.Value()` into `composeBody.SetValue()`; close panel
- `Tab` (subject hint visible): `composeSubject.SetValue(composeAISubjectHint)`; clear hint
- `AIAssistMsg`: set `composeAIResponse` content to result; compute `composeAIDiff = wordDiff(original, result)`; `composeAILoading = false`
- `AISubjectMsg`: set `composeAISubjectHint`; `composeAILoading = false`

---

## Error handling

- AI call fails: show error in `composeStatus` bar ("AI assistant unavailable"), clear loading state
- No AI configured (`m.ai == nil`): `Ctrl+G` shows status message "No AI backend configured"
- Empty body when requesting assist: show status "Write something first"

---

## Files changed

| File | Change |
|------|--------|
| `internal/app/app.go` | New fields, message types, `Ctrl+G`/`Ctrl+J` key routing |
| `internal/app/helpers.go` | `renderAIPanel()`, `wordDiff()`, `aiAssistCmd()`, `aiSubjectCmd()`, updated `renderComposeView()`, updated `handleComposeKey()` |
| `internal/app/snapshot_test.go` | Snapshot: compose with AI panel open and diff visible |

---

## Testing plan

### Unit tests
- `TestWordDiff_SingleWordChange` — "Hey" → "Hi" produces one deletion + one addition token
- `TestWordDiff_Unchanged` — identical strings produce no highlighted tokens
- `TestWordDiff_PhraseSwap` — multi-word replacement highlights only the changed words

### Snapshot test
- `TestSnapshot_ComposeAIPanel` — panel open, suggestion loaded, diff visible at 120×40

### Manual (tmux)
- `Ctrl+G` opens panel, compose body narrows
- Quick actions fire and populate suggestion with word-level diff
- Free-form prompt works
- `Ctrl+Enter` accepts and closes panel
- `Ctrl+J` shows subject hint; `Tab` accepts
- Thread context toggle works (verify different output vs. draft-only)
- No AI configured → friendly error, no panic
