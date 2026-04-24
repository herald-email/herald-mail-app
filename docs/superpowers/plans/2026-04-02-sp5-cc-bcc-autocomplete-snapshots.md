# SP5 — Compose CC/BCC + Autocomplete + TUI Snapshot Tests

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add CC/BCC fields with contact autocomplete to the compose tab, and establish a `teatest`-based golden-file snapshot test infrastructure.

**Architecture:** CC/BCC threads bottom-up: SMTP wire format → Draft model → Backend interface → TUI model fields → render + key handlers. Autocomplete fires `SearchContacts` on every keystroke when the active field token is ≥ 2 chars; results populate a dropdown overlay rendered below the active input. Snapshot tests use `charmbracelet/x/exp/teatest` + `charmbracelet/x/vt` to render the model to a virtual terminal at 120×40 and compare against checked-in golden files.

**Tech Stack:** Go, Bubble Tea, lipgloss, `charmbracelet/x/exp/teatest`, `charmbracelet/x/vt`, existing `backend.SearchContacts`

---

## File Map

| File | What changes |
|------|-------------|
| `internal/smtp/client.go` | Add CC/BCC to `buildMIMEMessage`, `SendWithInlineImages`; refactor `sendPlain` |
| `internal/smtp/mime.go` | Add `cc, bcc string` to `BuildDraftMessage` |
| `internal/smtp/mime_test.go` | Tests for CC/BCC header presence |
| `internal/models/email.go` | Add `CC`, `BCC string` to `Draft` |
| `internal/backend/backend.go` | Update `SaveDraft` signature |
| `internal/backend/local.go` | Update `SaveDraft` impl |
| `internal/backend/remote.go` | Update `SaveDraft` impl |
| `internal/backend/demo.go` | Update `SaveDraft` impl |
| `internal/daemon/server.go` | Update `saveDraftRequest` + handler |
| `internal/app/chat_tools_test.go` | Update `stubBackend.SaveDraft` stub |
| `internal/cleanup/noop_backend_test.go` | Update `noopBackend.SaveDraft` stub |
| `internal/app/app.go` | New model fields, message types, field routing |
| `internal/app/helpers.go` | Updated `renderComposeView`, `cycleComposeField`, `sendCompose`, `saveDraftCmd`; new autocomplete helpers |
| `go.mod` / `go.sum` | Add `teatest` + `vt` |
| `internal/app/snapshot_test.go` | New: test infrastructure + all 4 snapshot tests |
| `internal/app/testdata/snapshots/*.txt` | Golden files (generated) |

---

## Task 1: SMTP — CC/BCC in buildMIMEMessage and SendWithInlineImages

**Files:**
- Modify: `internal/smtp/client.go`
- Test: `internal/smtp/mime_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/smtp/mime_test.go`:

```go
func TestBuildMIMEMessage_CCHeader(t *testing.T) {
    msg := buildMIMEMessage("from@e.com", "to@e.com", "Subj", "plain", "", "cc@e.com", "")
    if !strings.Contains(msg, "Cc: cc@e.com\r\n") {
        t.Fatalf("expected Cc header, got:\n%s", msg)
    }
}

func TestBuildMIMEMessage_BCCNotInHeaders(t *testing.T) {
    msg := buildMIMEMessage("from@e.com", "to@e.com", "Subj", "plain", "", "", "bcc@e.com")
    if strings.Contains(msg, "Bcc:") {
        t.Fatalf("Bcc: must not appear in message headers, got:\n%s", msg)
    }
}

func TestBuildMIMEMessage_NoCCOmitsHeader(t *testing.T) {
    msg := buildMIMEMessage("from@e.com", "to@e.com", "Subj", "plain", "", "", "")
    if strings.Contains(msg, "Cc:") {
        t.Fatalf("Cc: must not appear when cc is empty, got:\n%s", msg)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/zoomacode/Developer/mail-processor
go test ./internal/smtp/... -run "TestBuildMIMEMessage_CC|TestBuildMIMEMessage_BCC|TestBuildMIMEMessage_NoCC" -v
```

Expected: FAIL — `buildMIMEMessage` has wrong signature.

- [ ] **Step 3: Refactor `sendPlain` to accept `rcpts []string`**

In `internal/smtp/client.go`, replace:

```go
func (c *Client) sendPlain(addr, from, to, rawMsg string) error {
    host := c.cfg.SMTP.Host
    auth := smtp.PlainAuth("", c.cfg.Credentials.Username, c.cfg.Credentials.Password, host)
    return smtp.SendMail(addr, auth, from, []string{to}, []byte(rawMsg))
}
```

With:

```go
func (c *Client) sendPlain(addr, from string, rcpts []string, rawMsg string) error {
    host := c.cfg.SMTP.Host
    auth := smtp.PlainAuth("", c.cfg.Credentials.Username, c.cfg.Credentials.Password, host)
    return smtp.SendMail(addr, auth, from, rcpts, []byte(rawMsg))
}
```

- [ ] **Step 4: Update `buildMIMEMessage` to accept cc and bcc**

Replace the existing `buildMIMEMessage` function signature and add the `Cc:` header:

```go
// buildMIMEMessage assembles the raw RFC 2822 message. cc is written as a
// Cc: header when non-empty. bcc recipients are delivered via RCPT TO by the
// caller but must NOT appear in the message headers per RFC 5321.
func buildMIMEMessage(from, to, subject, plainText, htmlBody, cc, bcc string) string {
    var msg strings.Builder
    msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
    msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
    if cc != "" {
        msg.WriteString(fmt.Sprintf("Cc: %s\r\n", cc))
    }
    msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
    msg.WriteString("MIME-Version: 1.0\r\n")

    if htmlBody == "" {
        msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
        msg.WriteString("\r\n")
        msg.WriteString(plainText)
        return msg.String()
    }

    boundary := fmt.Sprintf("boundary_%d", time.Now().UnixNano())
    msg.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=%q\r\n", boundary))
    msg.WriteString("\r\n")

    msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
    msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
    msg.WriteString("\r\n")
    msg.WriteString(plainText)
    msg.WriteString("\r\n")

    msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
    msg.WriteString("Content-Type: text/html; charset=utf-8\r\n")
    msg.WriteString("\r\n")
    msg.WriteString(htmlBody)
    msg.WriteString("\r\n")

    msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
    return msg.String()
}
```

- [ ] **Step 5: Update `Send` to pass empty cc/bcc and use new sendPlain signature**

In `Send`, change:
```go
rawMsg := buildMIMEMessage(from, to, subject, plainText, htmlBody)
```
to:
```go
rawMsg := buildMIMEMessage(from, to, subject, plainText, htmlBody, "", "")
```

And change the fallback:
```go
return c.sendPlain(addr, from, to, rawMsg)
```
to:
```go
return c.sendPlain(addr, from, []string{to}, rawMsg)
```

And the `client.Rcpt` call stays the same (single to).

- [ ] **Step 6: Update `SendWithAttachments` for new sendPlain signature**

In `SendWithAttachments`, change:
```go
return c.sendPlain(addr, from, to, rawMsg)
```
to:
```go
return c.sendPlain(addr, from, []string{to}, rawMsg)
```

- [ ] **Step 7: Update `SendReply` for new sendPlain signature**

In `SendReply`, change:
```go
return c.sendPlain(addr, from, to, rawMsg)
```
to:
```go
return c.sendPlain(addr, from, []string{to}, rawMsg)
```

- [ ] **Step 8: Add `parseAddrs` helper and update `SendWithInlineImages`**

Add this helper near the bottom of `client.go` (before or after `extFromMIME`):

```go
// parseAddrs splits a comma-separated address string into a slice of trimmed
// non-empty addresses.
func parseAddrs(s string) []string {
    if s == "" {
        return nil
    }
    parts := strings.Split(s, ",")
    out := make([]string, 0, len(parts))
    for _, p := range parts {
        if t := strings.TrimSpace(p); t != "" {
            out = append(out, t)
        }
    }
    return out
}
```

Change `SendWithInlineImages` signature from:
```go
func (c *Client) SendWithInlineImages(from, to, subject, plainText, htmlBody string, attachments []models.ComposeAttachment, inlines []InlineImage) error {
```
to:
```go
func (c *Client) SendWithInlineImages(from, to, subject, plainText, htmlBody, cc, bcc string, attachments []models.ComposeAttachment, inlines []InlineImage) error {
```

At the top of `SendWithInlineImages`, change the early-return delegation:
```go
if len(inlines) == 0 {
    return c.SendWithAttachments(from, to, subject, plainText, htmlBody, attachments)
}
```
to:
```go
if len(inlines) == 0 && cc == "" && bcc == "" {
    return c.SendWithAttachments(from, to, subject, plainText, htmlBody, attachments)
}
```

Inside the inline TLS dial block, after `client.Rcpt(to)`, add:
```go
for _, addr := range parseAddrs(cc) {
    if err := client.Rcpt(addr); err != nil {
        return fmt.Errorf("smtp RCPT CC %s: %w", addr, err)
    }
}
for _, addr := range parseAddrs(bcc) {
    if err := client.Rcpt(addr); err != nil {
        return fmt.Errorf("smtp RCPT BCC %s: %w", addr, err)
    }
}
```

Add the `Cc:` header in the `SendWithInlineImages` message builder (after the `To:` line):
```go
msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
if cc != "" {
    msg.WriteString(fmt.Sprintf("Cc: %s\r\n", cc))
}
```

Change the sendPlain fallback in `SendWithInlineImages`:
```go
return c.sendPlain(addr, from, to, rawMsg)
```
to:
```go
allRcpts := append([]string{to}, parseAddrs(cc)...)
allRcpts = append(allRcpts, parseAddrs(bcc)...)
return c.sendPlain(addr, from, allRcpts, rawMsg)
```

- [ ] **Step 9: Run tests**

```bash
go test ./internal/smtp/... -v
```

Expected: all pass.

- [ ] **Step 10: Build check**

```bash
make build
```

Expected: FAIL — `sendCompose` in helpers.go calls `SendWithInlineImages` with old signature. That's expected; it will be fixed in Task 8.

- [ ] **Step 11: Commit**

```bash
git add internal/smtp/client.go internal/smtp/mime_test.go
git commit -m "feat: add CC/BCC support to SMTP send — buildMIMEMessage, SendWithInlineImages"
```

---

## Task 2: Draft model + BuildDraftMessage CC/BCC

**Files:**
- Modify: `internal/models/email.go`
- Modify: `internal/smtp/mime.go`
- Modify: `internal/smtp/mime_test.go`

- [ ] **Step 1: Write failing test**

Add to `internal/smtp/mime_test.go`:

```go
func TestBuildDraftMessage_CCHeader(t *testing.T) {
    raw, err := BuildDraftMessage("from@e.com", "to@e.com", "cc@e.com", "", "Subj", "body")
    if err != nil {
        t.Fatalf("BuildDraftMessage: %v", err)
    }
    if !strings.Contains(string(raw), "Cc: cc@e.com\r\n") {
        t.Fatalf("expected Cc header in draft, got:\n%s", raw)
    }
}

func TestBuildDraftMessage_NoBCCHeader(t *testing.T) {
    raw, err := BuildDraftMessage("from@e.com", "to@e.com", "", "bcc@e.com", "Subj", "body")
    if err != nil {
        t.Fatalf("BuildDraftMessage: %v", err)
    }
    if strings.Contains(string(raw), "Bcc:") {
        t.Fatalf("Bcc: must not appear in draft headers, got:\n%s", raw)
    }
}
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/smtp/... -run "TestBuildDraftMessage_CC|TestBuildDraftMessage_NoBCC" -v
```

Expected: FAIL — `BuildDraftMessage` has wrong signature.

- [ ] **Step 3: Add CC/BCC to Draft struct**

In `internal/models/email.go`, change:

```go
type Draft struct {
    UID     uint32
    Folder  string
    To      string
    Subject string
    Body    string // Markdown body as stored
    Date    time.Time
}
```

to:

```go
type Draft struct {
    UID     uint32
    Folder  string
    To      string
    CC      string
    BCC     string
    Subject string
    Body    string // Markdown body as stored
    Date    time.Time
}
```

- [ ] **Step 4: Update BuildDraftMessage signature**

In `internal/smtp/mime.go`, change:

```go
func BuildDraftMessage(from, to, subject, body string) ([]byte, error) {
    var msg strings.Builder
    msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
    msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
    msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
```

to:

```go
func BuildDraftMessage(from, to, cc, bcc, subject, body string) ([]byte, error) {
    var msg strings.Builder
    msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
    msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
    if cc != "" {
        msg.WriteString(fmt.Sprintf("Cc: %s\r\n", cc))
    }
    msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/smtp/... -v
```

Expected: all pass. Note: `local.go` now fails to compile (calls `BuildDraftMessage` with old 4-arg signature). That's expected — fixed in Task 3.

- [ ] **Step 6: Commit**

```bash
git add internal/models/email.go internal/smtp/mime.go internal/smtp/mime_test.go
git commit -m "feat: add CC/BCC fields to Draft model and BuildDraftMessage"
```

---

## Task 3: Backend SaveDraft signature — interface + all implementations + stubs

**Files:**
- Modify: `internal/backend/backend.go`
- Modify: `internal/backend/local.go`
- Modify: `internal/backend/remote.go`
- Modify: `internal/backend/demo.go`
- Modify: `internal/daemon/server.go`
- Modify: `internal/app/chat_tools_test.go`
- Modify: `internal/cleanup/noop_backend_test.go`
- Test: `internal/backend/demo_draft_test.go`

- [ ] **Step 1: Write failing test**

In `internal/backend/demo_draft_test.go`, add:

```go
func TestDemoBackend_SaveDraft_WithCC(t *testing.T) {
    b := NewDemoBackend(nil)
    uid, folder, err := b.SaveDraft("to@e.com", "cc@e.com", "", "Subj", "Body")
    if err != nil {
        t.Fatalf("SaveDraft: %v", err)
    }
    if uid == 0 {
        t.Fatal("expected non-zero uid")
    }
    if folder == "" {
        t.Fatal("expected non-empty folder")
    }
    drafts, _ := b.ListDrafts()
    if len(drafts) != 1 {
        t.Fatalf("expected 1 draft, got %d", len(drafts))
    }
    if drafts[0].CC != "cc@e.com" {
        t.Fatalf("expected CC=cc@e.com, got %q", drafts[0].CC)
    }
}
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/backend/... -run "TestDemoBackend_SaveDraft_WithCC" -v
```

Expected: FAIL.

- [ ] **Step 3: Update Backend interface**

In `internal/backend/backend.go`, change:

```go
SaveDraft(to, subject, body string) (uid uint32, folder string, err error)
```

to:

```go
SaveDraft(to, cc, bcc, subject, body string) (uid uint32, folder string, err error)
```

- [ ] **Step 4: Update LocalBackend.SaveDraft**

In `internal/backend/local.go`, change:

```go
func (b *LocalBackend) SaveDraft(to, subject, body string) (uint32, string, error) {
    from := b.cfg.Credentials.Username
    raw, err := appsmtp.BuildDraftMessage(from, to, subject, body)
```

to:

```go
func (b *LocalBackend) SaveDraft(to, cc, bcc, subject, body string) (uint32, string, error) {
    from := b.cfg.Credentials.Username
    raw, err := appsmtp.BuildDraftMessage(from, to, cc, bcc, subject, body)
```

- [ ] **Step 5: Update RemoteBackend.SaveDraft**

In `internal/backend/remote.go`, change:

```go
func (b *RemoteBackend) SaveDraft(to, subject, body string) (uint32, string, error) {
    var resp struct {
        UID    uint32 `json:"uid"`
        Folder string `json:"folder"`
    }
    if err := b.postOut("/v1/drafts", map[string]string{"to": to, "subject": subject, "body": body}, &resp); err != nil {
        return 0, "", err
    }
    return resp.UID, resp.Folder, nil
}
```

to:

```go
func (b *RemoteBackend) SaveDraft(to, cc, bcc, subject, body string) (uint32, string, error) {
    var resp struct {
        UID    uint32 `json:"uid"`
        Folder string `json:"folder"`
    }
    payload := map[string]string{"to": to, "cc": cc, "bcc": bcc, "subject": subject, "body": body}
    if err := b.postOut("/v1/drafts", payload, &resp); err != nil {
        return 0, "", err
    }
    return resp.UID, resp.Folder, nil
}
```

- [ ] **Step 6: Update DemoBackend.SaveDraft**

In `internal/backend/demo.go`, change:

```go
func (d *DemoBackend) SaveDraft(to, subject, body string) (uint32, string, error) {
```

to:

```go
func (d *DemoBackend) SaveDraft(to, cc, bcc, subject, body string) (uint32, string, error) {
```

Inside the function body, when creating the draft, also set `CC` and `BCC`:

```go
draft := &models.Draft{
    UID:     d.nextDraftUID,
    Folder:  "Drafts",
    To:      to,
    CC:      cc,
    BCC:     bcc,
    Subject: subject,
    Body:    body,
    Date:    time.Now(),
}
```

(Replace whatever the existing draft literal looks like — add the `CC` and `BCC` fields.)

- [ ] **Step 7: Update daemon saveDraftRequest and handler**

In `internal/daemon/server.go`, change:

```go
type saveDraftRequest struct {
    To      string `json:"to"`
    Subject string `json:"subject"`
    Body    string `json:"body"`
}
```

to:

```go
type saveDraftRequest struct {
    To      string `json:"to"`
    CC      string `json:"cc"`
    BCC     string `json:"bcc"`
    Subject string `json:"subject"`
    Body    string `json:"body"`
}
```

And change the handler call:

```go
uid, folder, err := s.backend.SaveDraft(req.To, req.Subject, req.Body)
```

to:

```go
uid, folder, err := s.backend.SaveDraft(req.To, req.CC, req.BCC, req.Subject, req.Body)
```

- [ ] **Step 8: Update stub backends**

In `internal/app/chat_tools_test.go`, change:

```go
func (s *stubBackend) SaveDraft(_, _, _ string) (uint32, string, error) { return 0, "", nil }
```

to:

```go
func (s *stubBackend) SaveDraft(_, _, _, _, _ string) (uint32, string, error) { return 0, "", nil }
```

In `internal/cleanup/noop_backend_test.go`, change:

```go
func (noopBackend) SaveDraft(_, _, _ string) (uint32, string, error) { return 0, "", nil }
```

to:

```go
func (noopBackend) SaveDraft(_, _, _, _, _ string) (uint32, string, error) { return 0, "", nil }
```

- [ ] **Step 9: Run tests**

```bash
go test ./internal/backend/... -v
go test ./internal/app/... -v
go test ./internal/cleanup/... -v
```

Expected: all pass.

- [ ] **Step 10: Build check**

```bash
make build
```

Expected: FAIL — `saveDraftCmd` in helpers.go still calls old signature. Expected; fixed in Task 8.

- [ ] **Step 11: Commit**

```bash
git add internal/backend/backend.go internal/backend/local.go internal/backend/remote.go \
    internal/backend/demo.go internal/daemon/server.go \
    internal/app/chat_tools_test.go internal/cleanup/noop_backend_test.go \
    internal/backend/demo_draft_test.go
git commit -m "feat: update SaveDraft signature to include CC/BCC"
```

---

## Task 4: Compose model fields, message types, and field routing

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Add new fields to Model struct**

In `internal/app/app.go`, in the `// Compose` block (around line 359), add after `composeField`:

```go
// Compose
mailer               *appsmtp.Client
fromAddress          string
composeTo            textinput.Model
composeCC            textinput.Model
composeBCC           textinput.Model
composeSubject       textinput.Model
composeBody          textarea.Model
composeField         int    // 0=To, 1=CC, 2=BCC, 3=Subject, 4=Body
composeStatus        string // last send result message
composePreview       bool   // show glamour markdown preview
composeAttachments   []models.ComposeAttachment
attachmentPathInput  textinput.Model
attachmentInputActive bool

// Autocomplete (compose address fields)
suggestions   []models.ContactData // current autocomplete candidates (empty = dropdown hidden)
suggestionIdx int                  // selected row index (-1 = none selected)
```

- [ ] **Step 2: Add ContactSuggestionsMsg type**

Near the other message type declarations (around line 225), add:

```go
// ContactSuggestionsMsg carries autocomplete results for the compose address fields.
type ContactSuggestionsMsg struct {
    Contacts []models.ContactData
}
```

- [ ] **Step 3: Initialize new fields in New()**

In the `New()` function, after the existing `composeTo` initialization, add:

```go
composeCC := textinput.New()
composeCC.Placeholder = "cc@example.com, ..."
composeCC.CharLimit = 512

composeBCC := textinput.New()
composeBCC.Placeholder = "bcc@example.com, ..."
composeBCC.CharLimit = 512
```

And in the returned `Model` struct literal, add:
```go
composeCC:    composeCC,
composeBCC:   composeBCC,
suggestionIdx: -1,
```

- [ ] **Step 4: Update field routing in Update() for non-key messages**

In `app.go` around line 1540, the compose field routing block routes all messages to the active field:

```go
if m.activeTab == tabCompose {
    var cmd tea.Cmd
    switch m.composeField {
    case 0:
        m.composeTo, cmd = m.composeTo.Update(msg)
    case 1:
        m.composeSubject, cmd = m.composeSubject.Update(msg)
    case 2:
        m.composeBody, cmd = m.composeBody.Update(msg)
    }
    cmds = append(cmds, cmd)
    return m, tea.Batch(cmds...)
}
```

Change to:

```go
if m.activeTab == tabCompose {
    var cmd tea.Cmd
    switch m.composeField {
    case 0:
        m.composeTo, cmd = m.composeTo.Update(msg)
    case 1:
        m.composeCC, cmd = m.composeCC.Update(msg)
    case 2:
        m.composeBCC, cmd = m.composeBCC.Update(msg)
    case 3:
        m.composeSubject, cmd = m.composeSubject.Update(msg)
    case 4:
        m.composeBody, cmd = m.composeBody.Update(msg)
    }
    cmds = append(cmds, cmd)
    return m, tea.Batch(cmds...)
}
```

- [ ] **Step 5: Update any hardcoded composeField==1 (Subject) and composeField==2 (Body) references**

Search for all places that compare `composeField` to 1 or 2 and shift them to 3 and 4 respectively:

```bash
grep -n "composeField ==" internal/app/app.go internal/app/helpers.go
```

Update each:
- `composeField == 1` (was Subject) → `composeField == 3`
- `composeField == 2` (was Body) → `composeField == 4`
- `m.composeField = 1` (set to Subject) → `m.composeField = 3`
- `m.composeField = 2` (set to Body) → `m.composeField = 4`

Do NOT change `composeField == 0` (To field — unchanged).

- [ ] **Step 6: Handle ContactSuggestionsMsg in Update()**

In `app.go`, find the `switch msg.(type)` in `Update`. Add:

```go
case ContactSuggestionsMsg:
    m.suggestions = msg.Contacts
    if len(m.suggestions) == 0 {
        m.suggestionIdx = -1
    } else {
        m.suggestionIdx = 0
    }
    return m, nil
```

- [ ] **Step 7: Build check**

```bash
make build
```

Expected: FAIL due to Tasks 1–3 pending callers (sendCompose, saveDraftCmd, cycleComposeField). That's fine — the compile errors show what's left.

- [ ] **Step 8: Commit**

```bash
git add internal/app/app.go
git commit -m "feat: add composeCC/BCC fields, suggestionIdx, ContactSuggestionsMsg to app model"
```

---

## Task 5: renderComposeView CC/BCC rows + cycleComposeField

**Files:**
- Modify: `internal/app/helpers.go`

- [ ] **Step 1: Update `cycleComposeField`**

In `internal/app/helpers.go`, replace `cycleComposeField`:

```go
// cycleComposeField advances focus to the next compose input field.
// Order: To(0) → CC(1) → BCC(2) → Subject(3) → Body(4) → wrap.
func (m *Model) cycleComposeField() {
    m.composeField = (m.composeField + 1) % 5
    m.composeTo.Blur()
    m.composeCC.Blur()
    m.composeBCC.Blur()
    m.composeSubject.Blur()
    m.composeBody.Blur()
    switch m.composeField {
    case 0:
        m.composeTo.Focus()
    case 1:
        m.composeCC.Focus()
    case 2:
        m.composeBCC.Focus()
    case 3:
        m.composeSubject.Focus()
    case 4:
        m.composeBody.Focus()
    }
}
```

- [ ] **Step 2: Update key handler field routing in handleComposeKey**

In `helpers.go` around line 2502, the compose key handler routes keys to the focused field:

```go
switch m.composeField {
case 0:
    m.composeTo, cmd = m.composeTo.Update(msg)
case 1:
    m.composeSubject, cmd = m.composeSubject.Update(msg)
case 2:
    m.composeBody, cmd = m.composeBody.Update(msg)
}
```

Change to:

```go
switch m.composeField {
case 0:
    m.composeTo, cmd = m.composeTo.Update(msg)
case 1:
    m.composeCC, cmd = m.composeCC.Update(msg)
case 2:
    m.composeBCC, cmd = m.composeBCC.Update(msg)
case 3:
    m.composeSubject, cmd = m.composeSubject.Update(msg)
case 4:
    m.composeBody, cmd = m.composeBody.Update(msg)
}
```

- [ ] **Step 3: Update `renderComposeView` to add CC/BCC rows**

In `renderComposeView`, after the `// To field` block and before the `// Subject field` block, add CC and BCC rows:

```go
// CC field
ccStyle := inactiveFieldStyle
if m.composeField == 1 {
    ccStyle = activeFieldStyle
}
sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
    labelStyle.Render("CC:"),
    ccStyle.Render(m.composeCC.View()),
) + "\n")

// BCC field
bccStyle := inactiveFieldStyle
if m.composeField == 2 {
    bccStyle = activeFieldStyle
}
sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
    labelStyle.Render("BCC:"),
    bccStyle.Render(m.composeBCC.View()),
) + "\n")
```

Also change the Subject active-check:
```go
if m.composeField == 1 {   // old
```
to:
```go
if m.composeField == 3 {   // new (Subject is now field 3)
```

And the Body active-check:
```go
if m.composeField == 2 {   // old
```
to:
```go
if m.composeField == 4 {   // new (Body is now field 4)
```

- [ ] **Step 4: Build check**

```bash
make build
```

Expected: FAIL on `sendCompose` and `saveDraftCmd` (still use old signatures). Expected.

- [ ] **Step 5: Commit**

```bash
git add internal/app/helpers.go
git commit -m "feat: update compose view — CC/BCC rows, cycleComposeField extends to 5 fields"
```

---

## Task 6: Autocomplete — trigger and search command

**Files:**
- Modify: `internal/app/helpers.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Add `currentComposeToken` helper**

In `internal/app/helpers.go`, add:

```go
// currentComposeToken returns the text after the last comma in s, trimmed.
// This is the fragment being typed for autocomplete in a comma-separated
// address field.
func currentComposeToken(s string) string {
    if i := strings.LastIndex(s, ","); i >= 0 {
        return strings.TrimSpace(s[i+1:])
    }
    return strings.TrimSpace(s)
}
```

- [ ] **Step 2: Add `searchContactsCmd`**

In `internal/app/helpers.go`, add:

```go
// searchContactsCmd queries SearchContacts with token and returns a
// ContactSuggestionsMsg. Clears suggestions when token is shorter than 2 chars.
func (m *Model) searchContactsCmd(token string) tea.Cmd {
    if len(token) < 2 {
        return func() tea.Msg { return ContactSuggestionsMsg{} }
    }
    backend := m.backend
    return func() tea.Msg {
        contacts, err := backend.SearchContacts(token)
        if err != nil || len(contacts) == 0 {
            return ContactSuggestionsMsg{}
        }
        if len(contacts) > 5 {
            contacts = contacts[:5]
        }
        return ContactSuggestionsMsg{Contacts: contacts}
    }
}
```

- [ ] **Step 3: Fire search after keypress in address fields**

In `helpers.go`, in the compose key handler (the `switch m.composeField` block for routing keys to fields), after forwarding the keystroke to the active field, add autocomplete triggering for fields 0, 1, 2:

```go
var cmd tea.Cmd
switch m.composeField {
case 0:
    m.composeTo, cmd = m.composeTo.Update(msg)
    cmds := []tea.Cmd{cmd}
    cmds = append(cmds, m.searchContactsCmd(currentComposeToken(m.composeTo.Value())))
    return m, tea.Batch(cmds...)
case 1:
    m.composeCC, cmd = m.composeCC.Update(msg)
    cmds := []tea.Cmd{cmd}
    cmds = append(cmds, m.searchContactsCmd(currentComposeToken(m.composeCC.Value())))
    return m, tea.Batch(cmds...)
case 2:
    m.composeBCC, cmd = m.composeBCC.Update(msg)
    cmds := []tea.Cmd{cmd}
    cmds = append(cmds, m.searchContactsCmd(currentComposeToken(m.composeBCC.Value())))
    return m, tea.Batch(cmds...)
case 3:
    m.composeSubject, cmd = m.composeSubject.Update(msg)
case 4:
    m.composeBody, cmd = m.composeBody.Update(msg)
}
return m, cmd
```

- [ ] **Step 4: Clear suggestions when focus leaves address fields**

In `cycleComposeField`, after the field switch, clear suggestions when advancing past field 2:

```go
func (m *Model) cycleComposeField() {
    m.composeField = (m.composeField + 1) % 5
    // Clear autocomplete when moving away from address fields
    if m.composeField > 2 {
        m.suggestions = nil
        m.suggestionIdx = -1
    }
    m.composeTo.Blur()
    // ... (rest of focus logic unchanged)
}
```

- [ ] **Step 5: Commit**

```bash
git add internal/app/helpers.go internal/app/app.go
git commit -m "feat: autocomplete — searchContactsCmd fires on keypress in To/CC/BCC fields"
```

---

## Task 7: Autocomplete — dropdown rendering and keyboard accept/dismiss

**Files:**
- Modify: `internal/app/helpers.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Add `renderSuggestionDropdown` helper**

In `internal/app/helpers.go`, add:

```go
// renderSuggestionDropdown renders the autocomplete dropdown list.
// Returns an empty string when there are no suggestions.
func (m *Model) renderSuggestionDropdown() string {
    if len(m.suggestions) == 0 {
        return ""
    }
    selectedStyle := lipgloss.NewStyle().
        Background(lipgloss.Color("57")).
        Foreground(lipgloss.Color("255"))
    normalStyle := lipgloss.NewStyle().
        Foreground(lipgloss.Color("245"))
    boxStyle := lipgloss.NewStyle().
        Border(lipgloss.NormalBorder()).
        BorderForeground(lipgloss.Color("57")).
        Padding(0, 1)

    var rows []string
    for i, c := range m.suggestions {
        label := c.DisplayName
        if label == "" {
            label = c.Email
        } else {
            label = fmt.Sprintf("%s <%s>", label, c.Email)
        }
        if i == m.suggestionIdx {
            rows = append(rows, selectedStyle.Render(label))
        } else {
            rows = append(rows, normalStyle.Render(label))
        }
    }
    return boxStyle.Render(strings.Join(rows, "\n"))
}
```

- [ ] **Step 2: Insert dropdown into renderComposeView**

In `renderComposeView`, after the CC field row is written, add the dropdown insertion. The dropdown appears below the active address field. Insert it after the BCC row (so it always renders at the same vertical position regardless of which field is active):

```go
// Autocomplete dropdown (shown when address field has suggestions)
if drop := m.renderSuggestionDropdown(); drop != "" {
    sb.WriteString(drop + "\n")
}
```

Place this block after the BCC row write and before the Subject row.

- [ ] **Step 3: Handle dropdown navigation and accept in the compose key handler**

In the compose key handler (the `case "tab":` and surrounding key checks in `helpers.go`), add dropdown-aware handling BEFORE the existing `case "tab":` check:

```go
// Autocomplete dropdown interactions take priority
if len(m.suggestions) > 0 {
    switch msg.String() {
    case "up":
        if m.suggestionIdx > 0 {
            m.suggestionIdx--
        }
        return m, nil
    case "down":
        if m.suggestionIdx < len(m.suggestions)-1 {
            m.suggestionIdx++
        }
        return m, nil
    case "enter", "tab":
        // Accept selected suggestion
        if m.suggestionIdx >= 0 && m.suggestionIdx < len(m.suggestions) {
            c := m.suggestions[m.suggestionIdx]
            label := c.DisplayName
            if label == "" {
                label = c.Email
            } else {
                label = fmt.Sprintf("%s <%s>", label, c.Email)
            }
            m = m.acceptSuggestion(label)
        }
        m.suggestions = nil
        m.suggestionIdx = -1
        return m, nil
    case "esc":
        m.suggestions = nil
        m.suggestionIdx = -1
        return m, nil
    }
    // Any other key: dismiss and fall through to normal key handling
    m.suggestions = nil
    m.suggestionIdx = -1
}
```

- [ ] **Step 4: Add `acceptSuggestion` helper**

```go
// acceptSuggestion replaces the current token in the active address field
// with the accepted label (DisplayName <email>), followed by ", ".
func (m *Model) acceptSuggestion(label string) *Model {
    replaceToken := func(existing, token, replacement string) string {
        if i := strings.LastIndex(existing, ","); i >= 0 {
            return existing[:i+1] + " " + replacement + ", "
        }
        return replacement + ", "
    }

    switch m.composeField {
    case 0:
        token := currentComposeToken(m.composeTo.Value())
        m.composeTo.SetValue(replaceToken(m.composeTo.Value(), token, label))
        m.composeTo.CursorEnd()
    case 1:
        token := currentComposeToken(m.composeCC.Value())
        m.composeCC.SetValue(replaceToken(m.composeCC.Value(), token, label))
        m.composeCC.CursorEnd()
    case 2:
        token := currentComposeToken(m.composeBCC.Value())
        m.composeBCC.SetValue(replaceToken(m.composeBCC.Value(), token, label))
        m.composeBCC.CursorEnd()
    }
    return m
}
```

- [ ] **Step 5: Build check**

```bash
make build
```

Expected: FAIL only on `sendCompose`/`saveDraftCmd` (will be fixed in Task 8).

- [ ] **Step 6: Commit**

```bash
git add internal/app/helpers.go
git commit -m "feat: autocomplete dropdown — render, navigation, accept, dismiss"
```

---

## Task 8: Wire CC/BCC into sendCompose and saveDraftCmd

**Files:**
- Modify: `internal/app/helpers.go`

This task makes the build green.

- [ ] **Step 1: Update sendCompose**

In `helpers.go`, `sendCompose()`, add cc/bcc snapshots:

```go
func (m *Model) sendCompose() tea.Cmd {
    mailer := m.mailer // snapshot before goroutine to avoid data races
    from := m.fromAddress
    to := m.composeTo.Value()
    cc := m.composeCC.Value()
    bcc := m.composeBCC.Value()
    subject := m.composeSubject.Value()
    markdownBody := m.composeBody.Value()
    attachments := m.composeAttachments
    return func() tea.Msg {
        if mailer == nil {
            return ComposeStatusMsg{Message: "Error: SMTP not configured", Err: fmt.Errorf("smtp not configured")}
        }
        if to == "" {
            return ComposeStatusMsg{Message: "Error: To field is empty"}
        }
        if subject == "" {
            return ComposeStatusMsg{Message: "Error: Subject is empty"}
        }
        htmlBody, inlines, inlineErr := appsmtp.BuildInlineImages(markdownBody)
        if inlineErr != nil {
            logger.Warn("inline image embedding failed: %v", inlineErr)
            htmlBody, _ = appsmtp.MarkdownToHTMLAndPlain(markdownBody)
            inlines = nil
        }
        _, plainText := appsmtp.MarkdownToHTMLAndPlain(markdownBody)
        err := mailer.SendWithInlineImages(from, to, subject, plainText, htmlBody, cc, bcc, attachments, inlines)
        if err != nil {
            return ComposeStatusMsg{Message: fmt.Sprintf("Send failed: %v", err), Err: err}
        }
        return ComposeStatusMsg{Message: "Message sent!"}
    }
}
```

- [ ] **Step 2: Update saveDraftCmd**

```go
func (m *Model) saveDraftCmd() tea.Cmd {
    backend := m.backend
    to := m.composeTo.Value()
    cc := m.composeCC.Value()
    bcc := m.composeBCC.Value()
    subject := m.composeSubject.Value()
    body := m.composeBody.Value()
    return func() tea.Msg {
        uid, folder, err := backend.SaveDraft(to, cc, bcc, subject, body)
        return DraftSavedMsg{UID: uid, Folder: folder, Err: err}
    }
}
```

- [ ] **Step 3: Build**

```bash
make build
```

Expected: SUCCESS.

- [ ] **Step 4: Run all tests**

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/app/helpers.go
git commit -m "feat: wire CC/BCC into sendCompose and saveDraftCmd — build green"
```

---

## Task 9: Add teatest + vt deps and snapshot test infrastructure

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `internal/app/snapshot_test.go`
- Create: `internal/app/testdata/snapshots/` (directory)

- [ ] **Step 1: Add dependencies**

```bash
cd /Users/zoomacode/Developer/mail-processor
go get github.com/charmbracelet/x/exp/teatest@latest
```

`teatest` pulls in `charmbracelet/x/vt` and related packages as transitive dependencies. No need to import `vt` directly.

- [ ] **Step 2: Verify go.mod updated**

```bash
grep "teatest" go.mod
```

Expected: `github.com/charmbracelet/x/exp/teatest` present.

- [ ] **Step 3: Create golden file directory**

```bash
mkdir -p internal/app/testdata/snapshots
```

- [ ] **Step 4: Create snapshot_test.go with requireGolden helper**

Create `internal/app/snapshot_test.go`:

```go
package app

import (
    "bytes"
    "flag"
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/charmbracelet/x/exp/teatest"
    "mail-processor/internal/models"
)

var update = flag.Bool("update", false, "update golden snapshot files")

// requireGolden compares got against the golden file at path.
// With -update it writes got to the file instead.
func requireGolden(t *testing.T, path string, got []byte) {
    t.Helper()
    if *update {
        if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
            t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
        }
        if err := os.WriteFile(path, got, 0o644); err != nil {
            t.Fatalf("write golden %s: %v", path, err)
        }
        return
    }
    want, err := os.ReadFile(path)
    if err != nil {
        t.Fatalf("read golden %s: %v (run with -update to create)", path, err)
    }
    if !bytes.Equal(want, got) {
        t.Fatalf("snapshot mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", path, want, got)
    }
}

// testModelWithEmails creates a fully initialised Model via New() suitable for
// snapshot tests. All textinput/textarea fields are initialised; no live IMAP
// or SMTP is needed.
func testModelWithEmails(emails []*models.EmailData) *Model {
    b := &stubBackend{}
    m := New(b, nil, "", nil, false)
    m.windowWidth = 120
    m.windowHeight = 40
    m.timelineEmails = emails
    if len(emails) > 0 {
        rows := m.buildTimelineRows(emails)
        m.timelineTable.SetRows(rows)
    }
    return m
}
```

> **Note to implementer:** `New(b, nil, "", nil, false)` initialises all TUI fields including compose inputs, tables, and sidebar. Override `windowWidth`/`windowHeight` after construction to fix the terminal size. If `buildTimelineRows` is unexported or doesn't exist, substitute with whatever helper populates `m.timelineTable` rows from a `[]*models.EmailData` slice — check `helpers.go` for the right call. The key constraint: `m.View()` must not panic.

- [ ] **Step 5: Verify the test file compiles**

```bash
go build ./internal/app/...
```

Expected: SUCCESS (the test file won't be compiled here, that's fine).

```bash
go vet ./internal/app/...
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/app/snapshot_test.go internal/app/testdata/
git commit -m "feat: add teatest+vt deps and snapshot test infrastructure"
```

---

## Task 10: Timeline snapshot tests

**Files:**
- Modify: `internal/app/snapshot_test.go`
- Create: `internal/app/testdata/snapshots/timeline_empty.txt`
- Create: `internal/app/testdata/snapshots/timeline_populated.txt`

- [ ] **Step 1: Add mock email factory**

In `snapshot_test.go`, add:

```go
func mockEmails() []*models.EmailData {
    return []*models.EmailData{
        {
            MessageID: "msg-001",
            Sender:    "alice@example.com",
            Subject:   "Meeting tomorrow",
            Date:      time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC),
            Size:      1200,
            Folder:    "INBOX",
        },
        {
            MessageID: "msg-002",
            Sender:    "bob@example.com",
            Subject:   "Invoice #4521",
            Date:      time.Date(2026, 4, 1, 8, 30, 0, 0, time.UTC),
            Size:      3400,
            Folder:    "INBOX",
        },
        {
            MessageID: "msg-003",
            Sender:    "carol@example.com",
            Subject:   "Quarterly report",
            Date:      time.Date(2026, 3, 31, 14, 0, 0, 0, time.UTC),
            Size:      8900,
            Folder:    "INBOX",
        },
    }
}
```

- [ ] **Step 2: Write timeline_empty test**

```go
func TestSnapshot_TimelineEmpty(t *testing.T) {
    m := testModelWithEmails(nil)
    m.activeTab = tabTimeline
    tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
    // Wait for initial render
    teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
        return bytes.Contains(bts, []byte("Timeline")) || bytes.Contains(bts, []byte("INBOX"))
    }, teatest.WithCheckInterval(50*time.Millisecond), teatest.WithDuration(3*time.Second))
    tm.Quit()
    requireGolden(t, "testdata/snapshots/timeline_empty.txt", tm.FinalOutput(t))
}
```

- [ ] **Step 3: Write timeline_populated test**

```go
func TestSnapshot_TimelinePopulated(t *testing.T) {
    m := testModelWithEmails(mockEmails())
    m.activeTab = tabTimeline
    tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
    teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
        return bytes.Contains(bts, []byte("alice@example.com"))
    }, teatest.WithCheckInterval(50*time.Millisecond), teatest.WithDuration(3*time.Second))
    tm.Quit()
    requireGolden(t, "testdata/snapshots/timeline_populated.txt", tm.FinalOutput(t))
}
```

- [ ] **Step 4: Generate golden files**

```bash
go test ./internal/app/... -run "TestSnapshot_Timeline" -update -v
```

Expected: golden files written to `testdata/snapshots/`.

- [ ] **Step 5: Verify tests pass without -update**

```bash
go test ./internal/app/... -run "TestSnapshot_Timeline" -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/app/snapshot_test.go internal/app/testdata/snapshots/
git commit -m "test: add timeline snapshot tests (empty + populated)"
```

---

## Task 11: Compose snapshot tests

**Files:**
- Modify: `internal/app/snapshot_test.go`
- Create: `internal/app/testdata/snapshots/compose_blank.txt`
- Create: `internal/app/testdata/snapshots/compose_with_cc_bcc.txt`

- [ ] **Step 1: Write compose_blank test**

```go
func TestSnapshot_ComposeBlank(t *testing.T) {
    m := testModelWithEmails(nil)
    m.activeTab = tabCompose
    m.composeField = 0
    tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
    teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
        return bytes.Contains(bts, []byte("To:"))
    }, teatest.WithCheckInterval(50*time.Millisecond), teatest.WithDuration(3*time.Second))
    tm.Quit()
    requireGolden(t, "testdata/snapshots/compose_blank.txt", tm.FinalOutput(t))
}
```

- [ ] **Step 2: Write compose_with_cc_bcc test**

```go
func TestSnapshot_ComposeWithCCBCC(t *testing.T) {
    m := testModelWithEmails(nil)
    m.activeTab = tabCompose
    m.composeField = 0 // To field focused
    m.composeTo.SetValue("alice@example.com")
    m.composeCC.SetValue("bob@example.com")
    m.composeBCC.SetValue("carol@example.com")
    m.composeSubject.SetValue("Hello world")
    tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
    teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
        return bytes.Contains(bts, []byte("CC:")) && bytes.Contains(bts, []byte("BCC:"))
    }, teatest.WithCheckInterval(50*time.Millisecond), teatest.WithDuration(3*time.Second))
    tm.Quit()
    requireGolden(t, "testdata/snapshots/compose_with_cc_bcc.txt", tm.FinalOutput(t))
}
```

- [ ] **Step 3: Generate golden files**

```bash
go test ./internal/app/... -run "TestSnapshot_Compose" -update -v
```

Expected: golden files written.

- [ ] **Step 4: Verify all snapshot tests pass**

```bash
go test ./internal/app/... -run "TestSnapshot" -v
```

Expected: all 4 pass.

- [ ] **Step 5: Run full test suite**

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 6: Final build**

```bash
make build
```

Expected: SUCCESS.

- [ ] **Step 7: Commit**

```bash
git add internal/app/snapshot_test.go internal/app/testdata/snapshots/
git commit -m "test: add compose snapshot tests (blank + CC/BCC populated)"
```

---

## Verification

```bash
# Full build
make build

# All tests
go test ./...

# Snapshot tests only
go test ./internal/app/... -run TestSnapshot -v

# SMTP CC/BCC tests
go test ./internal/smtp/... -v

# Backend tests
go test ./internal/backend/... -v
```

**Manual check (tmux):**
```bash
tmux new-session -d -s test -x 220 -y 50
tmux send-keys -t test './bin/herald --demo' Enter
sleep 3
tmux send-keys -t test '2' ''   # Compose tab
sleep 0.5
tmux capture-pane -t test -p -e > /tmp/compose.txt
cat /tmp/compose.txt
# Verify CC/BCC rows visible, Tab cycles through all 5 fields
tmux kill-session -t test
```
