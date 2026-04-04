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
	// composeAIPanel should default to false
	if m.composeAIPanel {
		t.Fatal("composeAIPanel should default to false")
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
