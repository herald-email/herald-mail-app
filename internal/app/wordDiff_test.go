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

func TestComposeAIFields_Initialized(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	// composeAIThread should default to false (no reply context yet)
	if m.composeAIThread {
		t.Fatal("composeAIThread should default to false")
	}
	// composeAIPanel defaults open so Compose can show enabled controls or
	// a disabled warning immediately.
	if !m.composeAIPanel {
		t.Fatal("composeAIPanel should default to true")
	}
	// composeAISubjectHint should default to empty
	if m.composeAISubjectHint != "" {
		t.Fatalf("composeAISubjectHint should be empty, got %q", m.composeAISubjectHint)
	}
}

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

func TestComposeAIQuickActionsIncludeWritingImprovements(t *testing.T) {
	actions := map[string]composeAIAction{}
	for _, action := range composeAIQuickActions() {
		actions[action.Key] = action
	}

	checkInstruction := func(key, want string) {
		t.Helper()
		action, ok := actions[key]
		if !ok {
			t.Fatalf("missing action %q", key)
		}
		if !strings.Contains(strings.ToLower(action.Instruction), want) {
			t.Fatalf("action %q instruction = %q, want to contain %q", key, action.Instruction, want)
		}
	}

	checkInstruction("f", "typos")
	checkInstruction("n", "shorten")
	checkInstruction("e", "expand")
	if !strings.Contains(strings.ToLower(composeAITranslateInstruction("French")), "translate") {
		t.Fatal("translate dropdown instruction should include translate")
	}
	if !strings.Contains(strings.ToLower(composeAIStyleInstruction("Direct")), "direct") {
		t.Fatal("style dropdown instruction should include selected style")
	}
}

func TestComposeAITranslateDropdownCustomPrefillsFreeformInstruction(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	m.composeAIPanel = true
	m.composeBody.SetValue("Please review the proposal by Friday.")
	m.openComposeAIMenu(composeAIMenuTranslate)
	cmd, handled := m.selectComposeAIMenuOption("6")
	if !handled {
		t.Fatal("expected custom translate option to be handled")
	}
	if cmd != nil {
		t.Fatal("custom translate setup should not dispatch AI before the target language is entered")
	}
	if got := m.composeAIInput.Value(); got != "Translate this email to " {
		t.Fatalf("composeAIInput = %q", got)
	}
	if !m.composeAIInput.Focused() {
		t.Fatal("translate setup should focus the freeform instruction input")
	}
	if m.composeStatus == "" {
		t.Fatal("translate setup should tell the user what to enter next")
	}
}

func TestStartComposeAIActionImmediateRequiresDraft(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	m.classifier = &stubClassifier{}
	action, ok := composeAIActionByKey("f")
	if !ok {
		t.Fatal("missing typo-fix action")
	}
	model, cmd := m.startComposeAIAction(action)
	if cmd != nil {
		t.Fatal("empty draft should not dispatch AI")
	}
	updated := model.(*Model)
	if updated.composeStatus != "Write something first" {
		t.Fatalf("composeStatus = %q", updated.composeStatus)
	}
}

func TestAcceptComposeAIResponseStoresUndoBody(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	m.composeAIPanel = true
	m.composeBody.SetValue("helo team")
	m.composeAIResponse.SetValue("hello team")
	if !m.acceptComposeAIResponse() {
		t.Fatal("expected AI response to be accepted")
	}
	if got := m.composeBody.Value(); got != "hello team" {
		t.Fatalf("composeBody = %q", got)
	}
	if got := m.composeAIUndoBody; got != "helo team" {
		t.Fatalf("composeAIUndoBody = %q", got)
	}
	m.undoComposeAIRewrite()
	if got := m.composeBody.Value(); got != "helo team" {
		t.Fatalf("composeBody after undo = %q", got)
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

// --- styledSender tests ---

func TestStyledSender_NameAndEmailDistinct(t *testing.T) {
	raw := "NerdWallet <nerdwallet@mail.nerdwallet.com>"
	result := styledSender(raw, 50)
	stripped := stripANSI(result)
	// Both name and email should appear in the output
	if !strings.Contains(stripped, "NerdWallet") {
		t.Fatalf("name missing from styledSender output: %q", stripped)
	}
	if !strings.Contains(stripped, "nerdwallet@mail.nerdwallet.com") {
		t.Fatalf("email missing from styledSender output: %q", stripped)
	}
	// The stripped output must contain both parts (lipgloss may suppress colors in
	// non-TTY test environments, so we only assert on content, not ANSI codes).
	_ = result
}

func TestStyledSender_TruncatesAtMaxWidth(t *testing.T) {
	raw := "Very Long Display Name <verylongemail@verylongdomain.example.com>"
	result := styledSender(raw, 20)
	stripped := stripANSI(result)
	// Visual length should not exceed maxWidth (20)
	if len([]rune(stripped)) > 21 { // allow 1 for ellipsis char
		t.Fatalf("styledSender exceeded maxWidth: %q (len %d)", stripped, len([]rune(stripped)))
	}
}

func TestStyledSender_ReadingFirstShowsNameOnlyWhenNarrow(t *testing.T) {
	raw := "NerdWallet <nerdwallet@mail.nerdwallet.com>"
	result := styledSender(raw, 24)
	stripped := stripANSI(result)
	if stripped != "NerdWallet" {
		t.Fatalf("narrow sender cell should show the display name without email noise, got %q", stripped)
	}
}

func TestStyledSender_ReadingFirstIncludesEmailWhenWide(t *testing.T) {
	raw := "NerdWallet <nerdwallet@mail.nerdwallet.com>"
	result := styledSender(raw, 48)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "NerdWallet") || !strings.Contains(stripped, "nerdwallet@mail.nerdwallet.com") {
		t.Fatalf("wide sender cell should include name and email, got %q", stripped)
	}
}

func TestStyledSender_NoAngleBrackets_PlainFallback(t *testing.T) {
	raw := "noreply@example.com"
	result := styledSender(raw, 30)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "noreply@example.com") {
		t.Fatalf("email-only address missing from output: %q", stripped)
	}
}

func TestStyledSender_EmptyString(t *testing.T) {
	result := styledSender("", 20)
	_ = result // should not panic
}
