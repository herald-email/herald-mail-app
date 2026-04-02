# SP5 — TUI Snapshot Tests + Compose CC/BCC + Autocomplete

## Overview

Two features:

1. **TUI Snapshot Tests** — `teatest` + `charmbracelet/x/vt` golden-file infrastructure for key views.
2. **Compose CC/BCC + Autocomplete** — two new address fields in compose with a live dropdown backed by the existing `SearchContacts` backend method.

> **Note:** The Contacts tab (Tab 4) is already fully implemented. "Contacts (small)" refers only to wiring the existing `SearchContacts` into the compose autocomplete.

---

## Feature 1: TUI Snapshot Tests

### Infrastructure

Add two new Go module dependencies:

```
github.com/charmbracelet/x/exp/teatest
github.com/charmbracelet/x/vt
```

`teatest` provides `TestModel` — wraps a Bubble Tea program in a test harness with `Send`, `WaitFor`, and output access. `vt` provides a virtual terminal that renders ANSI escape codes to plain text, enabling deterministic string comparison.

### Test file location

`internal/app/snapshot_test.go` — alongside existing `app_test.go`.

Golden files: `internal/app/testdata/snapshots/*.txt` — checked into git.

### Update flag

```bash
go test ./internal/app/... -update   # regenerate golden files
go test ./internal/app/...           # diff against golden files (CI)
```

Implemented via a package-level `var update = flag.Bool("update", false, "update golden files")`.

### Golden file helper

`requireGolden` is a file-local helper in `snapshot_test.go`:
- With `-update`: writes `got` bytes to the golden file path (creating dirs as needed)
- Without `-update`: reads the golden file and calls `t.Fatalf` with a diff if content differs

### Snapshots (initial set)

| Golden file | State |
|---|---|
| `timeline_empty.txt` | App init, `stubBackend` returns 0 emails |
| `timeline_populated.txt` | 3 mock `EmailData` entries loaded into table |
| `compose_blank.txt` | Tab 2, all fields empty |
| `compose_with_cc_bcc.txt` | CC and BCC fields visible, To field focused |

Terminal size fixed at **120×40** for all snapshots — keeps golden files stable across machines.

### Test pattern

```go
func TestSnapshot_TimelinePopulated(t *testing.T) {
    backend := &stubBackend{emails: mockEmails()}
    m := app.New(testConfig(), backend)
    tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
    teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
        return bytes.Contains(bts, []byte("alice@example.com"))
    }, teatest.WithCheckInterval(50*time.Millisecond), teatest.WithDuration(3*time.Second))
    requireGolden(t, "testdata/snapshots/timeline_populated.txt", tm.FinalOutput(t))
}
```

### Scope boundary

- No live IMAP or SMTP — `stubBackend` only.
- No timing-sensitive tests — `WaitFor` polls on content, not on sleep.
- Snapshot tests complement (not replace) the tmux QA workflow in CLAUDE.md.

---

## Feature 2: Compose CC/BCC + Autocomplete

### New model fields

```go
composeCC     textinput.Model   // CC addresses (comma-separated)
composeBCC    textinput.Model   // BCC addresses (comma-separated)
suggestions   []models.ContactData  // current autocomplete candidates
suggestionIdx int               // selected row in dropdown (-1 = none)
```

`composeField` currently cycles through 3 values (0=To, 1=Subject, 2=Body). It extends to 5: 0=To, 1=CC, 2=BCC, 3=Subject, 4=Body.

### Layout change

The compose form gains two rows between To and Subject:

```
To:      [____________________________]
CC:      [____________________________]
BCC:     [____________________________]
Subject: [____________________________]
─────────────────────────────────────────
[body textarea]
```

### Focus cycle

`Tab` advances: To → CC → BCC → Subject → Body → (wrap).
`Shift+Tab` reverses. Active field highlighted with existing focus style.

**Exception**: when the autocomplete dropdown is open (`len(suggestions) > 0`), `Tab` accepts the selected suggestion instead of advancing focus. Focus advances only after the dropdown is cleared.

### Autocomplete trigger

On each keystroke in To, CC, or BCC: extract the current token (text after the last `,`, trimmed). If token length ≥ 2, fire `backend.SearchContacts(token)` as a `tea.Cmd`. Result arrives as `ContactSuggestionsMsg{contacts []models.ContactData}`.

If token < 2 characters or field loses focus: clear `suggestions`.

### Dropdown rendering

Up to 5 rows rendered immediately below the active input field as a lipgloss-styled box. Each row: `DisplayName <email>`. Selected row highlighted. Rendered in `View()` using overlay technique (same layer as `quickReplyPicker`).

### Keyboard (dropdown active)

| Key | Action |
|-----|--------|
| `↑` / `↓` | Move `suggestionIdx` |
| `Enter` or `Tab` | Accept: replace current token with `DisplayName <email>, `; clear dropdown |
| `Esc` | Dismiss dropdown without accepting |
| Any printable key | Dismiss dropdown; type normally |

### Multi-value

Fields accept comma-separated addresses. Autocomplete token is always the segment after the last `,` — supports adding multiple recipients sequentially.

### SMTP wiring

`sendCompose` snapshots `composeCC` and `composeBCC` before the goroutine (same pattern as `composeTo`). CC/BCC are passed to `smtp.SendWithInlineImages` — add `cc, bcc string` params. `buildMIMEMessage` adds `Cc:` and `Bcc:` headers when non-empty.

### Draft model update

`models.Draft` gains `CC` and `BCC string` fields so auto-save preserves them. `saveDraftCmd` snapshots both fields; daemon `handleSaveDraft` stores them in IMAP draft body headers.

---

## Architecture changes summary

| File | Change |
|------|--------|
| `internal/app/app.go` | Add `composeCC`, `composeBCC`, `suggestions`, `suggestionIdx` fields; extend `composeField` cycle to 5; new message types `ContactSuggestionsMsg` |
| `internal/app/helpers.go` | Update `renderCompose()` for CC/BCC rows + dropdown overlay; update `cycleComposeField()`; update `sendCompose()` to snapshot + pass CC/BCC; update `saveDraftCmd()` to snapshot CC/BCC |
| `internal/smtp/client.go` | Add `cc, bcc string` params to `SendWithInlineImages` |
| `internal/smtp/mime.go` | Add `Cc:` / `Bcc:` headers to `buildMIMEMessage` when non-empty |
| `internal/models/email.go` | Add `CC`, `BCC string` to `Draft` struct |
| `internal/daemon/server.go` | Update `handleSaveDraft` to read/write CC/BCC from draft headers |
| `go.mod` / `go.sum` | Add `teatest` + `vt` deps |
| `internal/app/snapshot_test.go` | New snapshot test file |
| `internal/app/testdata/snapshots/` | Golden files (4 initial) |
| `internal/app/chat_tools_test.go` | No new Backend stubs needed (no new Backend methods) |

---

## Error handling

- `SearchContacts` failure during autocomplete: silently clear suggestions (don't interrupt typing).
- CC/BCC empty on send: treated as no CC/BCC — no error, headers simply omitted.

---

## Testing plan

### Snapshot tests (automated)
- All 4 golden files pass `go test ./internal/app/...`
- `-update` flag regenerates files; subsequent run passes

### Unit tests
- `TestAutocomplete_TriggerAt2Chars` — `SearchContacts` not called at 1 char, called at 2
- `TestAutocomplete_AcceptAppends` — accept inserts `DisplayName <email>, ` and clears suggestions
- `TestAutocomplete_TabConflict` — Tab with dropdown open accepts suggestion, not advance focus
- `TestSMTP_CCBCCHeaders` — `Cc:` and `Bcc:` present in built MIME when non-empty; absent when empty

### Manual (tmux)
- CC/BCC fields render at 220×50, 80×24
- Dropdown appears below active field, dismisses cleanly
- Multiple recipients work (comma-separated)
- Draft save/restore preserves CC/BCC values
