# SP5 — Contacts Tab, TUI Snapshot Tests, Compose CC/BCC + Autocomplete

## Overview

Three tightly related features that together make Herald's compose flow and contact management first-class:

1. **Contacts Tab** — a browsable Tab 4 backed by the existing contacts SQLite table, with macOS import and pre-fill-compose action.
2. **TUI Snapshot Tests** — `teatest` + `charmbracelet/x/vt` infrastructure with golden-file snapshots for key views.
3. **Compose CC/BCC + Autocomplete** — two new address fields in compose with a live dropdown backed by `SearchContacts`.

---

## Feature 1: Contacts Tab

### What it does

A 4th tab (`4` key, label "Contacts") is added to the existing tab bar (Timeline / Compose / Cleanup / Contacts).

The view is a `bubbles/table` with three columns: **Name**, **Email**, **Source** (macOS / sent / manual). Contacts are loaded from the existing `contacts` SQLite table via a new `ListContacts() ([]*models.Contact, error)` method on the Backend interface.

The tab is read-only for editing — contacts are populated by import or auto-add on send. Users can search, navigate, and act on rows.

### Key bindings

| Key | Action |
|-----|--------|
| `4` | Switch to Contacts tab |
| `/` | Focus search input; filters table live as you type |
| `Esc` | Clear search / unfocus search input |
| `Enter` | Pre-fill Compose `To:` with selected contact's email; switch to Tab 2 |
| `i` | Trigger macOS Contacts.app AppleScript import (shows progress message) |
| `d` | Delete selected contact (confirmation prompt: `y` / `n`) |
| `j` / `k`, `↑` / `↓` | Navigate rows |

### Backend interface addition

```go
// ListContacts returns all contacts, optionally filtered by query (empty = all).
ListContacts(query string) ([]*models.Contact, error)
```

`LocalBackend` delegates to the existing `cache.SearchContacts` (query non-empty) or a new `cache.ListAllContacts` (query empty). `RemoteBackend` proxies `GET /v1/contacts?q=<query>`. `DemoBackend` returns a small hardcoded slice of synthetic contacts.

### Daemon endpoint

```
GET /v1/contacts?q=<query>
```

Returns JSON array of `{name, email, source}`. No write endpoints — contacts are managed locally.

### Data flow

```
4 key → ContactsTabMsg → load ContactsMsg (stubBackend.ListContacts)
/ key  → search input focused → ContactsSearchMsg(query) → refilter table in Update
Enter  → ContactSelectedMsg{Email} → set composeTo, switch to tab 2
i key  → ImportContactsMsg → backend.ImportContacts() → ImportDoneMsg (progress shown in status bar)
d key  → ContactDeleteConfirmMsg → y → backend.DeleteContact(email) → reload
```

---

## Feature 2: TUI Snapshot Tests

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

### Snapshots (initial set)

| Golden file | State |
|---|---|
| `timeline_empty.txt` | App init, `stubBackend` returns 0 emails |
| `timeline_populated.txt` | 3 mock `EmailData` entries loaded into table |
| `compose_blank.txt` | Tab 2, all fields empty |
| `compose_with_cc_bcc.txt` | CC and BCC fields visible, To field focused |
| `contacts_empty.txt` | Tab 4, `ListContacts` returns 0 results |
| `contacts_populated.txt` | Tab 4, 3 mock contacts loaded |

### Test pattern

```go
func TestSnapshot_TimelinePopulated(t *testing.T) {
    backend := &stubBackend{emails: mockEmails()}
    m := app.New(testConfig(), backend)
    tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
    teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
        return bytes.Contains(bts, []byte("alice@example.com"))
    }, teatest.WithCheckInterval(50*time.Millisecond), teatest.WithDuration(3*time.Second))
    // requireGolden is a file-local helper: reads golden file, compares;
    // with -update flag, writes tm.FinalOutput(t) to the file instead.
    requireGolden(t, "testdata/snapshots/timeline_populated.txt", tm.FinalOutput(t))
}
```

Terminal size fixed at **120×40** for all snapshots — matches the "wide terminal" case and keeps golden files stable across machines.

### Scope boundary

- No live IMAP or SMTP — `stubBackend` only.
- No timing-sensitive tests — `WaitFor` polls on content, not on sleep.
- Snapshot tests do not replace tmux QA (layout stress, narrow sizes) — they complement it.

---

## Feature 3: Compose CC/BCC + Autocomplete

### New model fields

```go
composeCC    textinput.Model   // CC addresses (comma-separated)
composeBCC   textinput.Model   // BCC addresses (comma-separated)
suggestions  []models.Contact  // current autocomplete candidates
suggestionIdx int              // selected row in dropdown (-1 = none)
```

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

On each keystroke in To, CC, or BCC: extract the current token (text after the last `,`, trimmed). If token length ≥ 2, fire `SearchContacts(token)` as a `tea.Cmd`. Result arrives as `ContactSuggestionsMsg{contacts []models.Contact}`.

If token < 2 characters or field loses focus: clear `suggestions`.

### Dropdown rendering

Up to 5 rows rendered immediately below the active input field as a lipgloss-styled box. Each row: `Name <email>`. Selected row highlighted. Rendered in `View()` using overlay technique (same layer as `quickReplyPicker`).

### Keyboard (dropdown active)

| Key | Action |
|-----|--------|
| `↑` / `↓` | Move `suggestionIdx` |
| `Enter` or `Tab` | Accept: replace current token with `Name <email>, `; clear dropdown |
| `Esc` | Dismiss dropdown without accepting |
| Any printable key | Dismiss dropdown; type normally |

### Multi-value

Fields accept comma-separated addresses. Autocomplete token is always the segment after the last `,` — supports adding multiple recipients sequentially.

### SMTP wiring

`sendCompose` snapshots `composeCC` and `composeBCC` values before the goroutine (same snapshot pattern as `composeTo`). CC/BCC are passed to `smtp.SendWithInlineImages` — add `cc, bcc string` params if not already present. The SMTP `buildMIMEMessage` adds `Cc:` and `Bcc:` headers when non-empty.

---

## Architecture changes summary

| File | Change |
|------|--------|
| `internal/backend/backend.go` | Add `ListContacts(query string) ([]*models.Contact, error)` |
| `internal/backend/local.go` | `ListContacts` → `cache.ListAllContacts` or `SearchContacts` |
| `internal/backend/remote.go` | `ListContacts` → `GET /v1/contacts?q=` |
| `internal/backend/demo.go` | `ListContacts` → synthetic slice |
| `internal/cache/cache.go` | Add `ListAllContacts()` (no-filter variant of `SearchContacts`) |
| `internal/daemon/server.go` | Add `GET /v1/contacts` handler |
| `internal/app/app.go` | Tab 4 state, CC/BCC fields, suggestions fields, new message types |
| `internal/app/helpers.go` | `renderContacts()`, `renderSuggestionDropdown()`, `saveDraftCmd` CC/BCC, contact actions |
| `internal/smtp/client.go` | Add `cc, bcc string` params to `SendWithInlineImages` |
| `internal/smtp/mime.go` | Add `Cc:` / `Bcc:` headers to `buildMIMEMessage` |
| `go.mod` / `go.sum` | Add `teatest` + `vt` deps |
| `internal/app/snapshot_test.go` | New snapshot test file |
| `internal/app/testdata/snapshots/` | Golden files (6 initial) |
| `internal/app/chat_tools_test.go` | Add `ListContacts` no-op stub |
| `internal/cleanup/noop_backend_test.go` | Add `ListContacts` no-op stub |

---

## Error handling

- `ListContacts` failure: show "Failed to load contacts" in status bar; table shows empty state.
- `SearchContacts` failure during autocomplete: silently clear suggestions (don't interrupt typing).
- macOS import failure: show error in status bar (AppleScript may fail if Contacts.app access is denied).
- Contact delete: if backend returns error, show in status bar; do not remove row from table.

---

## Testing plan

### Snapshot tests (automated)
- All 6 golden files pass `go test ./internal/app/...`
- Update flow works: `-update` flag regenerates files, subsequent run passes

### Unit tests
- `TestListContacts_Empty` — returns empty slice, no error
- `TestListContacts_Populated` — returns expected rows
- `TestAutocomplete_TriggerAt2Chars` — verify `SearchContacts` not called at 1 char, called at 2
- `TestAutocomplete_AcceptAppends` — accept inserts `Name <email>, ` and clears suggestions
- `TestSMTP_CCBCCHeaders` — verify `Cc:` and `Bcc:` present in built MIME when non-empty

### Manual (tmux)
- Tab 4 renders at 220×50, 80×24
- Dropdown appears below To field, dismisses cleanly
- `Enter` on contact pre-fills Compose To and switches tab
- macOS import shows progress then success/error
