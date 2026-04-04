# SP6 — Compose AI Writing Assistant Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a right-side AI panel to the compose tab that rewrites email bodies with word-level diff display, suggests subject lines, and lets users edit the AI suggestion before accepting it.

**Architecture:** A `composeAIPanel bool` toggle on the Model opens a right-side panel rendered by `renderAIPanel()`. Quick actions and a free-form prompt fire `aiAssistCmd()` which calls `m.classifier.Chat()`. The AI result populates an editable `composeAIResponse textarea.Model`; a lipgloss-styled diff computed by `wordDiff()` is displayed above it as a read-only reference. `Ctrl+Enter` copies the edited response into `composeBody`. Subject suggestion (`Ctrl+J`) fires `aiSubjectCmd()` and shows a one-line hint below the Subject row. Thread context is stored as `replyContextEmail *models.EmailData` when the user presses `R`.

**Tech Stack:** Go, Bubble Tea, lipgloss, `m.classifier` (existing `ai.AIClient`), `ai.ChatMessage`

---

## File Map

| File | What changes |
|------|-------------|
| `internal/app/helpers.go` | Add `wordDiff`, `tokenizeWords`, `lcsTokens`; add `aiAssistCmd`, `aiSubjectCmd`; add `renderAIPanel`; update `renderComposeView`, `handleComposeKey` |
| `internal/app/app.go` | Add model fields, `AIAssistMsg`, `AISubjectMsg`; initialize in `New()`; handle messages in `Update()`; capture `replyContextEmail` in reply handler |
| `internal/app/wordDiff_test.go` | New: unit tests for `wordDiff` and `tokenizeWords` |
| `internal/app/snapshot_test.go` | Add `TestSnapshot_ComposeAIPanel` |
| `internal/app/testdata/snapshots/compose_ai_panel.txt` | New golden file |

---

## Task 1: wordDiff — word-level diff helper

**Files:**
- Create: `internal/app/wordDiff_test.go`
- Modify: `internal/app/helpers.go`

### Step 1: Write failing tests

Create `internal/app/wordDiff_test.go`:

```go
package app

import (
	"strings"
	"testing"
)

func TestTokenizeWords_SimpleWords(t *testing.T) {
	got := tokenizeWords("Hello world")
	want := []string{"Hello", " ", "world"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTokenizeWords_PunctuationSeparated(t *testing.T) {
	got := tokenizeWords("sync.")
	// Should split into: "sync", "."
	if len(got) != 2 || got[0] != "sync" || got[1] != "." {
		t.Fatalf("got %v", got)
	}
}

func TestWordDiff_Unchanged(t *testing.T) {
	result := wordDiff("Hello world", "Hello world")
	// No diff markers — result should not contain strikethrough or green styles
	// but should contain the words themselves
	if !strings.Contains(result, "Hello") || !strings.Contains(result, "world") {
		t.Fatalf("unchanged words missing from diff: %q", result)
	}
}

func TestWordDiff_SingleWordChange(t *testing.T) {
	result := wordDiff("Hey Alice", "Hi Alice")
	// "Hey" should be marked as deleted, "Hi" as added, "Alice" unchanged
	// We check by stripping ANSI and looking for both words present
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "Hey") {
		t.Fatalf("deleted word 'Hey' missing from diff: %q", stripped)
	}
	if !strings.Contains(stripped, "Hi") {
		t.Fatalf("added word 'Hi' missing from diff: %q", stripped)
	}
	if !strings.Contains(stripped, "Alice") {
		t.Fatalf("unchanged word 'Alice' missing from diff: %q", stripped)
	}
}

func TestWordDiff_PhraseSwap(t *testing.T) {
	original := "Can we meet tomorrow for a quick sync?"
	revised := "Are you available tomorrow for a quick catch-up?"
	result := wordDiff(original, revised)
	stripped := stripANSI(result)
	// "tomorrow", "for", "a", "quick" should appear as unchanged
	for _, word := range []string{"tomorrow", "for", "a", "quick"} {
		if !strings.Contains(stripped, word) {
			t.Fatalf("unchanged word %q missing from diff: %q", word, stripped)
		}
	}
}

// stripANSI removes ANSI escape codes for test assertions.
func stripANSI(s string) string {
	var out strings.Builder
	inEscape := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z') {
				inEscape = false
			}
			continue
		}
		out.WriteByte(s[i])
	}
	return out.String()
}
```

### Step 2: Run tests to verify they fail

```bash
cd /Users/zoomacode/Developer/mail-processor
go test ./internal/app/... -run "TestTokenizeWords|TestWordDiff" -v 2>&1 | head -20
```

Expected: FAIL — `tokenizeWords` and `wordDiff` not defined.

### Step 3: Implement tokenizeWords, lcsTokens, wordDiff

Add to `internal/app/helpers.go` (near the bottom, after existing helpers):

```go
// tokenizeWords splits s into a slice of word and non-word tokens,
// preserving whitespace and punctuation as separate tokens.
// "Hello, world" → ["Hello", ",", " ", "world"]
func tokenizeWords(s string) []string {
	var tokens []string
	var cur strings.Builder
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
			tokens = append(tokens, string(r))
		} else if r == ',' || r == '.' || r == '!' || r == '?' || r == ';' || r == ':' || r == '"' || r == '\'' || r == '(' || r == ')' {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
			tokens = append(tokens, string(r))
		} else {
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// lcsTokens returns the longest common subsequence of token slices a and b.
func lcsTokens(a, b []string) []string {
	m, n := len(a), len(b)
	// Build DP table
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}
	// Backtrack
	result := make([]string, 0, dp[m][n])
	i, j := m, n
	for i > 0 && j > 0 {
		if a[i-1] == b[j-1] {
			result = append(result, a[i-1])
			i--
			j--
		} else if dp[i-1][j] >= dp[i][j-1] {
			i--
		} else {
			j--
		}
	}
	// Reverse
	for l, r := 0, len(result)-1; l < r; l, r = l+1, r-1 {
		result[l], result[r] = result[r], result[l]
	}
	return result
}

// wordDiff computes a word-level diff between original and revised and returns
// a lipgloss-styled string. Deleted tokens appear red with strikethrough,
// added tokens appear green, unchanged tokens are unstyled.
func wordDiff(original, revised string) string {
	delStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Strikethrough(true).
		Background(lipgloss.Color("52"))
	addStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("46")).
		Background(lipgloss.Color("22"))

	origTokens := tokenizeWords(original)
	revTokens := tokenizeWords(revised)
	common := lcsTokens(origTokens, revTokens)

	var sb strings.Builder
	i, j, k := 0, 0, 0
	for k < len(common) {
		// Emit deletions and additions up to the next common token
		for i < len(origTokens) && origTokens[i] != common[k] {
			sb.WriteString(delStyle.Render(origTokens[i]))
			i++
		}
		for j < len(revTokens) && revTokens[j] != common[k] {
			sb.WriteString(addStyle.Render(revTokens[j]))
			j++
		}
		// Emit common token unstyled
		sb.WriteString(common[k])
		i++
		j++
		k++
	}
	// Trailing deletions
	for i < len(origTokens) {
		sb.WriteString(delStyle.Render(origTokens[i]))
		i++
	}
	// Trailing additions
	for j < len(revTokens) {
		sb.WriteString(addStyle.Render(revTokens[j]))
		j++
	}
	return sb.String()
}
```

### Step 4: Run tests to verify they pass

```bash
go test ./internal/app/... -run "TestTokenizeWords|TestWordDiff" -v
```

Expected: all 5 tests PASS.

### Step 5: Commit

```bash
git add internal/app/helpers.go internal/app/wordDiff_test.go
git commit -m "feat: add wordDiff helper — word-level diff with LCS for compose AI panel"
```

---

## Task 2: Model fields, message types, New() initialization

**Files:**
- Modify: `internal/app/app.go`

### Step 1: Write a compile-check test

Add to the end of `internal/app/wordDiff_test.go`:

```go
func TestComposeAIFields_Initialized(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	// composeAIThread should default to false (no reply context yet)
	if m.composeAIThread {
		t.Fatal("composeAIThread should default to false")
	}
	// composeAIPanel should default to false
	if m.composeAIPanel {
		t.Fatal("composeAIPanel should default to false")
	}
	// composeAISubjectHint should default to empty
	if m.composeAISubjectHint != "" {
		t.Fatalf("composeAISubjectHint should be empty, got %q", m.composeAISubjectHint)
	}
}
```

### Step 2: Run to verify it fails

```bash
go test ./internal/app/... -run "TestComposeAIFields_Initialized" -v 2>&1 | head -10
```

Expected: FAIL — fields not defined.

### Step 3: Add fields to the Model struct

In `internal/app/app.go`, find the compose fields block (around line 367, after `composeAttachments`). After the `// Autocomplete` comment block, add a new `// Compose AI panel` block:

```go
// Compose AI panel (Ctrl+G)
composeAIPanel       bool
composeAIInput       textinput.Model // free-form prompt
composeAIResponse    textarea.Model  // editable AI rewrite
composeAIDiff        string          // lipgloss-styled word diff (display only)
composeAILoading     bool
composeAIThread      bool            // true = include reply context in prompt
composeAISubjectHint string          // pending subject suggestion ("" = none)
replyContextEmail    *models.EmailData // set when reply is initiated; nil for new emails
```

### Step 4: Add AIAssistMsg and AISubjectMsg message types

Near the other message type declarations in `app.go` (after `ContactSuggestionsMsg` around line 238), add:

```go
// AIAssistMsg carries the result of an AI body-rewrite request.
type AIAssistMsg struct {
	Result string
	Err    error
}

// AISubjectMsg carries the result of an AI subject-suggestion request.
type AISubjectMsg struct {
	Subject string
	Err     error
}
```

### Step 5: Initialize new fields in New()

In the `New()` function, after the `composeCC`/`composeBCC` initialization block, add:

```go
composeAIInput := textinput.New()
composeAIInput.Placeholder = "Ask AI anything about this email…"
composeAIInput.CharLimit = 512

composeAIResponse := textarea.New()
composeAIResponse.Placeholder = "AI suggestion will appear here…"
composeAIResponse.SetWidth(38)
composeAIResponse.SetHeight(8)
composeAIResponse.CharLimit = 0
```

In the returned `Model{}` literal, add:

```go
composeAIInput:    composeAIInput,
composeAIResponse: composeAIResponse,
```

(`composeAIPanel`, `composeAILoading`, `composeAIThread`, `composeAISubjectHint`, `replyContextEmail` default to zero values — no explicit initialization needed.)

### Step 6: Capture replyContextEmail when reply is initiated

In `app.go`, in the `Update()` function, find the `case "R":` reply handler (around line 2375). After `m.activeTab = tabCompose`, add:

```go
m.replyContextEmail = email
m.composeAIThread = true  // thread context available for replies
```

Add it so the full updated block looks like:

```go
case "R":
    if !m.loading && m.activeTab == tabTimeline {
        cursor := m.timelineTable.Cursor()
        if cursor < len(m.threadRowMap) {
            ref := m.threadRowMap[cursor]
            var email *models.EmailData
            if ref.kind == rowKindThread {
                email = ref.group.emails[0]
            } else {
                email = ref.group.emails[ref.emailIdx]
            }
            m.activeTab = tabCompose
            m.replyContextEmail = email      // ← ADD THIS
            m.composeAIThread = true         // ← ADD THIS
            m.composeTo.SetValue(email.Sender)
            subject := email.Subject
            if !strings.HasPrefix(strings.ToLower(subject), "re:") {
                subject = "Re: " + subject
            }
            m.composeSubject.SetValue(subject)
            m.composeField = 4
            m.composeTo.Blur()
            m.composeSubject.Blur()
            m.composeBody.Focus()
        }
    }
```

### Step 7: Handle AIAssistMsg and AISubjectMsg in Update()

In the `switch msg.(type)` block in `Update()`, add:

```go
case AIAssistMsg:
    m.composeAILoading = false
    if msg.Err != nil {
        m.composeStatus = fmt.Sprintf("AI error: %v", msg.Err)
        return m, nil
    }
    original := m.composeBody.Value()
    m.composeAIDiff = wordDiff(original, msg.Result)
    m.composeAIResponse.SetValue(msg.Result)
    return m, nil

case AISubjectMsg:
    m.composeAILoading = false
    if msg.Err != nil {
        m.composeStatus = fmt.Sprintf("AI error: %v", msg.Err)
        return m, nil
    }
    m.composeAISubjectHint = strings.TrimSpace(msg.Subject)
    return m, nil
```

### Step 8: Build check

```bash
make build 2>&1 | head -20
```

Expected: SUCCESS (or only pre-existing errors — no errors from the new fields).

### Step 9: Run the compile-check test

```bash
go test ./internal/app/... -run "TestComposeAIFields_Initialized" -v
```

Expected: PASS.

### Step 10: Commit

```bash
git add internal/app/app.go internal/app/wordDiff_test.go
git commit -m "feat: add compose AI panel model fields, message types, reply context capture"
```

---

## Task 3: aiAssistCmd and aiSubjectCmd

**Files:**
- Modify: `internal/app/helpers.go`

### Step 1: Write failing tests

Add to `internal/app/wordDiff_test.go`:

```go
func TestAiAssistCmd_NilClassifier(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	// m.classifier is nil — cmd should return an error AIAssistMsg immediately
	cmd := m.aiAssistCmd("Improve")
	msg := cmd()
	assistMsg, ok := msg.(AIAssistMsg)
	if !ok {
		t.Fatalf("expected AIAssistMsg, got %T", msg)
	}
	if assistMsg.Err == nil {
		t.Fatal("expected error when classifier is nil")
	}
}

func TestAiSubjectCmd_NilClassifier(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	cmd := m.aiSubjectCmd()
	msg := cmd()
	subjectMsg, ok := msg.(AISubjectMsg)
	if !ok {
		t.Fatalf("expected AISubjectMsg, got %T", msg)
	}
	if subjectMsg.Err == nil {
		t.Fatal("expected error when classifier is nil")
	}
}
```

### Step 2: Run to verify they fail

```bash
go test ./internal/app/... -run "TestAiAssistCmd|TestAiSubjectCmd" -v 2>&1 | head -15
```

Expected: FAIL — functions not defined.

### Step 3: Implement aiAssistCmd

Add to `internal/app/helpers.go`:

```go
// aiAssistCmd fires an AI body-rewrite request with the given instruction.
// If m.composeAIThread is true and m.replyContextEmail is non-nil, the
// original email's sender and subject are included as context.
func (m *Model) aiAssistCmd(instruction string) tea.Cmd {
	classifier := m.classifier
	draft := m.composeBody.Value()
	threadCtx := m.composeAIThread
	replyEmail := m.replyContextEmail

	return func() tea.Msg {
		if classifier == nil {
			return AIAssistMsg{Err: fmt.Errorf("no AI backend configured")}
		}
		if strings.TrimSpace(draft) == "" {
			return AIAssistMsg{Err: fmt.Errorf("draft is empty")}
		}

		var contextParts []string
		if threadCtx && replyEmail != nil {
			contextParts = append(contextParts,
				fmt.Sprintf("This email is a reply to:\nFrom: %s\nSubject: %s",
					replyEmail.Sender, replyEmail.Subject))
		}
		contextParts = append(contextParts, "Current draft:\n"+draft)
		context := strings.Join(contextParts, "\n\n")

		messages := []ai.ChatMessage{
			{
				Role: "system",
				Content: "You are an expert email writing assistant. " +
					"Rewrite the email body according to the user's instruction. " +
					"Return only the rewritten body text, no explanations or preamble.",
			},
			{
				Role:    "user",
				Content: instruction + "\n\n" + context,
			},
		}
		result, err := classifier.Chat(messages)
		if err != nil {
			return AIAssistMsg{Err: err}
		}
		return AIAssistMsg{Result: strings.TrimSpace(result)}
	}
}
```

### Step 4: Implement aiSubjectCmd

Add to `internal/app/helpers.go`:

```go
// aiSubjectCmd fires an AI subject-suggestion request using the current
// draft body and, if available, the thread context.
func (m *Model) aiSubjectCmd() tea.Cmd {
	classifier := m.classifier
	draft := m.composeBody.Value()
	threadCtx := m.composeAIThread
	replyEmail := m.replyContextEmail

	return func() tea.Msg {
		if classifier == nil {
			return AISubjectMsg{Err: fmt.Errorf("no AI backend configured")}
		}

		var contextParts []string
		if threadCtx && replyEmail != nil {
			contextParts = append(contextParts,
				fmt.Sprintf("Original email subject: %s\nFrom: %s",
					replyEmail.Subject, replyEmail.Sender))
		}
		if strings.TrimSpace(draft) != "" {
			contextParts = append(contextParts, "Email body:\n"+draft)
		}
		if len(contextParts) == 0 {
			return AISubjectMsg{Err: fmt.Errorf("nothing to base a subject on")}
		}

		messages := []ai.ChatMessage{
			{
				Role: "system",
				Content: "You are an email writing assistant. " +
					"Suggest a concise, specific email subject line (maximum 10 words). " +
					"Return only the subject line text, no quotes, no explanation.",
			},
			{
				Role:    "user",
				Content: strings.Join(contextParts, "\n\n"),
			},
		}
		result, err := classifier.Chat(messages)
		if err != nil {
			return AISubjectMsg{Err: err}
		}
		return AISubjectMsg{Subject: strings.TrimSpace(result)}
	}
}
```

### Step 5: Run tests

```bash
go test ./internal/app/... -run "TestAiAssistCmd|TestAiSubjectCmd" -v
```

Expected: both PASS.

### Step 6: Build check

```bash
make build
```

Expected: SUCCESS.

### Step 7: Commit

```bash
git add internal/app/helpers.go internal/app/wordDiff_test.go
git commit -m "feat: add aiAssistCmd and aiSubjectCmd for compose AI panel"
```

---

## Task 4: renderAIPanel

**Files:**
- Modify: `internal/app/helpers.go`

### Step 1: Add renderAIPanel

Add to `internal/app/helpers.go` (near `renderComposeView`):

```go
// renderAIPanel renders the compose AI assistant panel.
// Returns an empty string when composeAIPanel is false.
// width is the panel's character width.
func (m *Model) renderAIPanel(width int) string {
	if !m.composeAIPanel {
		return ""
	}
	if width < 20 {
		width = 20
	}

	var sb strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Bold(true).
		Width(width)
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Width(width)
	activeToggleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("25")).
		Padding(0, 1)
	inactiveToggleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Padding(0, 1)
	actionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Background(lipgloss.Color("236")).
		Padding(0, 1).
		Margin(0, 1, 0, 0)
	acceptStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("28")).
		Padding(0, 1)
	discardStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Background(lipgloss.Color("236")).
		Padding(0, 1)
	spinnerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86"))

	// Title
	sb.WriteString(titleStyle.Render("🤖 AI Assistant") + "\n")
	sb.WriteString(strings.Repeat("─", width) + "\n")

	// Context toggle — only shown when replying (replyContextEmail != nil)
	if m.replyContextEmail != nil {
		sb.WriteString(labelStyle.Render("Context:") + "\n")
		threadLabel := inactiveToggleStyle.Render("Thread")
		draftLabel := inactiveToggleStyle.Render("Draft only")
		if m.composeAIThread {
			threadLabel = activeToggleStyle.Render("● Thread")
		} else {
			draftLabel = activeToggleStyle.Render("● Draft only")
		}
		sb.WriteString(threadLabel + "  " + draftLabel + "\n\n")
	}

	// Quick action buttons
	actions := []string{"Improve", "Shorten", "Lengthen", "Formal", "Casual"}
	var actionRow strings.Builder
	for _, a := range actions {
		actionRow.WriteString(actionStyle.Render(a))
	}
	sb.WriteString(truncate(actionRow.String(), width*2) + "\n\n") // width*2 allows for ANSI overhead

	// Free-form prompt input
	sb.WriteString(labelStyle.Render("Custom prompt:") + "\n")
	m.composeAIInput.Width = width - 2
	sb.WriteString(m.composeAIInput.View() + "\n\n")

	// Loading spinner
	if m.composeAILoading {
		sb.WriteString(spinnerStyle.Render("⠋ Thinking…") + "\n")
		return lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("86")).
			Width(width).
			Render(sb.String())
	}

	// Diff view (if result available)
	if m.composeAIDiff != "" {
		sb.WriteString(labelStyle.Render("Changes:") + "\n")
		diffStyle := lipgloss.NewStyle().
			Width(width - 2).
			MaxWidth(width - 2)
		sb.WriteString(diffStyle.Render(m.composeAIDiff) + "\n\n")
	}

	// Editable response textarea
	if m.composeAIResponse.Value() != "" || m.composeAIDiff != "" {
		sb.WriteString(labelStyle.Render("Suggestion (edit freely):") + "\n")
		m.composeAIResponse.SetWidth(width - 2)
		m.composeAIResponse.SetHeight(8)
		sb.WriteString(m.composeAIResponse.View() + "\n\n")

		// Accept / Discard
		sb.WriteString(acceptStyle.Render("✓ Accept") + "  " + discardStyle.Render("Discard") + "\n")
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(
			"Ctrl+Enter: accept  Esc: discard") + "\n")
	}

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("86")).
		Width(width).
		Render(sb.String())
}
```

### Step 2: Build check

```bash
make build
```

Expected: SUCCESS.

### Step 3: Commit

```bash
git add internal/app/helpers.go
git commit -m "feat: add renderAIPanel for compose AI assistant"
```

---

## Task 5: renderComposeView updates — panel layout + subject hint

**Files:**
- Modify: `internal/app/helpers.go`

### Step 1: Update renderComposeView to show AI panel alongside body

Read the current `renderComposeView` function (around line 2691 in helpers.go) to find exactly where the body textarea and preview are rendered. The body section looks roughly like:

```go
// existing: render body or preview filling remaining height
bodyStyle := inactiveFieldStyle
if m.composeField == 4 {
    bodyStyle = activeFieldStyle
}
sb.WriteString(bodyStyle.Render(m.composeBody.View()) + "\n")
```

Replace the **body rendering section** (body + divider onwards) with a side-by-side layout when the panel is open:

```go
// Divider between header fields and body
sb.WriteString(strings.Repeat("─", width) + "\n")

// Subject hint (shown below Subject row when a suggestion is pending)
if m.composeAISubjectHint != "" {
    hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
    dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
    hint := hintStyle.Render("✨ "+truncate(m.composeAISubjectHint, width-30)) +
        "  " + dimStyle.Render("Tab: accept  Esc: dismiss")
    sb.WriteString(hint + "\n")
}

// Body + optional AI panel
if m.composeAIPanel {
    // Split: body ~60%, panel ~40% (min panel width = 36)
    panelWidth := 40
    if width < 80 {
        panelWidth = width / 2
    }
    bodyWidth := width - panelWidth - 3 // 3 for padding/border
    if bodyWidth < 20 {
        bodyWidth = 20
    }

    // Body side
    bodyStyle := inactiveFieldStyle.Width(bodyWidth)
    if m.composeField == 4 {
        bodyStyle = activeFieldStyle.Width(bodyWidth)
    }
    if m.composePreview {
        rendered, err := glamour.Render(m.composeBody.Value(), "dark")
        if err != nil {
            rendered = m.composeBody.Value()
        }
        bodyPane := lipgloss.NewStyle().Width(bodyWidth).Render(rendered)
        panelPane := m.renderAIPanel(panelWidth)
        sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, bodyPane, "  ", panelPane) + "\n")
    } else {
        m.composeBody.SetWidth(bodyWidth)
        bodyPane := bodyStyle.Render(m.composeBody.View())
        panelPane := m.renderAIPanel(panelWidth)
        sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, bodyPane, "  ", panelPane) + "\n")
    }
} else {
    // Normal full-width body
    bodyStyle := inactiveFieldStyle
    if m.composeField == 4 {
        bodyStyle = activeFieldStyle
    }
    if m.composePreview {
        rendered, err := glamour.Render(m.composeBody.Value(), "dark")
        if err != nil {
            rendered = m.composeBody.Value()
        }
        sb.WriteString(lipgloss.NewStyle().Width(width).Render(rendered) + "\n")
    } else {
        m.composeBody.SetWidth(width - 2)
        sb.WriteString(bodyStyle.Render(m.composeBody.View()) + "\n")
    }
}
```

> **Note to implementer:** Read the existing `renderComposeView` carefully before replacing. The exact code for the body/preview section may differ slightly. Match the surrounding context (label styles, attachment rendering below). Only replace the body-rendering section — leave To/CC/BCC/Subject fields unchanged. The `glamour` import is already present.

### Step 2: Build check

```bash
make build
```

Expected: SUCCESS.

### Step 3: Commit

```bash
git add internal/app/helpers.go
git commit -m "feat: renderComposeView — AI panel side-by-side layout, subject hint line"
```

---

## Task 6: handleComposeKey — Ctrl+G, Ctrl+J, Ctrl+Enter, subject hint

**Files:**
- Modify: `internal/app/helpers.go`

### Step 1: Add Ctrl+G (toggle panel) and Ctrl+J (suggest subject)

In `handleComposeKey`, find the section that checks for `ctrl+s`, `ctrl+p`, `ctrl+a`. Add BEFORE those checks:

```go
case "ctrl+g":
    if m.classifier == nil {
        m.composeStatus = "No AI backend configured"
        return m, nil
    }
    m.composeAIPanel = !m.composeAIPanel
    if m.composeAIPanel {
        // Focus the prompt input when panel opens
        m.composeAIInput.Focus()
    } else {
        m.composeAIInput.Blur()
        m.composeAIResponse.Blur()
    }
    return m, nil

case "ctrl+j":
    if m.classifier == nil {
        m.composeStatus = "No AI backend configured"
        return m, nil
    }
    if m.composeBody.Value() == "" && m.replyContextEmail == nil {
        m.composeStatus = "Write something first"
        return m, nil
    }
    m.composeAILoading = true
    return m, m.aiSubjectCmd()
```

### Step 2: Add Tab to accept subject hint

In `handleComposeKey`, find the existing `case "tab":` handler (which calls `cycleComposeField`). Change it to check for the subject hint first:

```go
case "tab":
    // If a subject hint is pending, Tab accepts it
    if m.composeAISubjectHint != "" {
        m.composeSubject.SetValue(m.composeAISubjectHint)
        m.composeAISubjectHint = ""
        return m, nil
    }
    // If autocomplete dropdown is open, Tab accepts the suggestion
    // (this check is already present — keep it)
    if len(m.suggestions) > 0 {
        // ... existing autocomplete accept logic ...
    }
    // Otherwise cycle compose field
    m.cycleComposeField()
    return m, nil
```

> **Note to implementer:** Read the existing `case "tab":` in `handleComposeKey` to see the exact existing code. Prepend the subject hint check at the top of that case without removing any existing logic.

### Step 3: Add Ctrl+Enter to accept AI suggestion

Add before or after the `ctrl+s` case:

```go
case "ctrl+enter":
    if m.composeAIPanel && m.composeAIResponse.Value() != "" {
        m.composeBody.SetValue(m.composeAIResponse.Value())
        // Clear panel state
        m.composeAIPanel = false
        m.composeAIDiff = ""
        m.composeAIResponse.SetValue("")
        m.composeAIInput.Blur()
        m.composeAIResponse.Blur()
        m.composeBody.Focus()
        m.composeField = 4
    }
    return m, nil
```

### Step 4: Add Esc to dismiss subject hint and close AI panel

Find the existing `case "esc":` handler in `handleComposeKey`. Add at the top:

```go
case "esc":
    // Dismiss subject hint if present
    if m.composeAISubjectHint != "" {
        m.composeAISubjectHint = ""
        return m, nil
    }
    // Close AI panel if open
    if m.composeAIPanel {
        m.composeAIPanel = false
        m.composeAIDiff = ""
        m.composeAIInput.Blur()
        m.composeAIResponse.Blur()
        return m, nil
    }
    // ... existing esc logic (clear composeStatus) ...
```

### Step 5: Route keystrokes to AI panel input when panel is focused

In `handleComposeKey`, in the field-routing switch (cases 0–4), add routing for when the AI panel prompt is active. Add BEFORE the main `switch m.composeField` block:

```go
// When AI panel is open and prompt input is focused, route keystrokes to it
if m.composeAIPanel && m.composeAIInput.Focused() {
    if msg.String() == "enter" {
        instruction := strings.TrimSpace(m.composeAIInput.Value())
        if instruction == "" {
            return m, nil
        }
        m.composeAILoading = true
        m.composeAIInput.SetValue("")
        return m, m.aiAssistCmd(instruction)
    }
    var cmd tea.Cmd
    m.composeAIInput, cmd = m.composeAIInput.Update(msg)
    return m, cmd
}

// When AI panel response textarea is focused, route to it
if m.composeAIPanel && m.composeAIResponse.Focused() {
    var cmd tea.Cmd
    m.composeAIResponse, cmd = m.composeAIResponse.Update(msg)
    return m, cmd
}
```

### Step 6: Handle quick action clicks via keyboard shortcut numbers

Add inside `handleComposeKey`, after the panel-input routing above, when panel is open:

```go
// When AI panel is open, number keys 1-5 trigger quick actions
if m.composeAIPanel && !m.composeAIInput.Focused() {
    actions := map[string]string{
        "1": "Improve the clarity and professionalism of this email",
        "2": "Shorten this email to be more concise",
        "3": "Lengthen this email with more detail",
        "4": "Rewrite this email in a formal tone",
        "5": "Rewrite this email in a casual, friendly tone",
    }
    if instruction, ok := actions[msg.String()]; ok {
        if m.composeBody.Value() == "" {
            m.composeStatus = "Write something first"
            return m, nil
        }
        m.composeAILoading = true
        return m, m.aiAssistCmd(instruction)
    }
}
```

### Step 7: Build check

```bash
make build
```

Expected: SUCCESS.

### Step 8: Commit

```bash
git add internal/app/helpers.go
git commit -m "feat: compose key handler — Ctrl+G panel toggle, Ctrl+J subject, Ctrl+Enter accept"
```

---

## Task 7: Snapshot test for compose AI panel

**Files:**
- Modify: `internal/app/snapshot_test.go`
- Create: `internal/app/testdata/snapshots/compose_ai_panel.txt`

### Step 1: Add the snapshot test

Add to `internal/app/snapshot_test.go`:

```go
func TestSnapshot_ComposeAIPanel(t *testing.T) {
	m := testModelWithEmails(nil)
	m.activeTab = tabCompose
	m.composeField = 4
	m.composeBody.SetValue("Hey Alice,\n\nCan we meet tomorrow for a quick sync?\n\nThanks")
	m.composeAIPanel = true
	// Pre-populate with a fake AI result so the diff renders
	original := m.composeBody.Value()
	revised := "Hi Alice,\n\nAre you available tomorrow for a quick catch-up?\n\nBest regards"
	m.composeAIDiff = wordDiff(original, revised)
	m.composeAIResponse.SetValue(revised)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	time.Sleep(200 * time.Millisecond)
	tm.Quit()
	requireGolden(t, "testdata/snapshots/compose_ai_panel.txt",
		readAll(t, tm.FinalOutput(t, teatest.WithFinalTimeout(5*time.Second))))
}
```

### Step 2: Generate the golden file

```bash
cd /Users/zoomacode/Developer/mail-processor
go test ./internal/app/... -run "TestSnapshot_ComposeAIPanel" -update -v
```

Expected: golden file written to `testdata/snapshots/compose_ai_panel.txt`.

### Step 3: Inspect the golden file to verify it contains the AI panel

```bash
strings internal/app/testdata/snapshots/compose_ai_panel.txt | grep -E "AI|panel|Improve|Shorten|Changes|Suggestion|Accept" | head -10
```

Expected: lines containing "AI Assistant", "Improve", "Shorten", "Changes", "Suggestion", "Accept".

### Step 4: Run all snapshot tests without -update

```bash
go test ./internal/app/... -run "TestSnapshot" -v
```

Expected: all 5 tests PASS.

### Step 5: Run full test suite

```bash
go test ./...
```

Expected: all pass.

### Step 6: Final build

```bash
make build
```

Expected: SUCCESS.

### Step 7: Commit

```bash
git add internal/app/snapshot_test.go internal/app/testdata/snapshots/compose_ai_panel.txt
git commit -m "test: add compose AI panel snapshot test"
```

---

## Verification

```bash
# Full build
make build

# All tests
go test ./...

# AI panel specific tests
go test ./internal/app/... -run "TestWordDiff|TestTokenizeWords|TestAiAssistCmd|TestAiSubjectCmd|TestComposeAIFields|TestSnapshot_ComposeAI" -v

# Snapshot regenerate (if needed)
go test ./internal/app/... -run "TestSnapshot" -update -v
```

**Manual check (tmux):**
```bash
tmux new-session -d -s test -x 220 -y 50
tmux send-keys -t test './bin/herald --demo' Enter
sleep 3
tmux send-keys -t test '2' ''        # Compose tab
sleep 0.5
tmux capture-pane -t test -p -e > /tmp/compose.txt
cat /tmp/compose.txt
# Should show compose form with To/CC/BCC/Subject/Body

# Open AI panel
tmux send-keys -t test 'G' ''   # Ctrl+G
sleep 0.5
tmux capture-pane -t test -p -e > /tmp/compose_ai.txt
cat /tmp/compose_ai.txt
# Should show body narrowed, AI panel on right with quick actions

# Test subject hint
tmux send-keys -t test 'J' ''   # Ctrl+J
sleep 2
tmux capture-pane -t test -p -e > /tmp/subject_hint.txt
cat /tmp/subject_hint.txt
# Should show hint below Subject row

tmux kill-session -t test
```
