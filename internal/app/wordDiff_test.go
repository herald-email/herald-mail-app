package app

import (
	"errors"
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

func TestParseComposeAIRewriteResponseStructuredSuccess(t *testing.T) {
	got, err := parseComposeAIRewriteResponse(`{"status":"ok","text":"よろしくお願いいたします。","error_code":"","message":""}`)
	if err != nil {
		t.Fatalf("parseComposeAIRewriteResponse returned error: %v", err)
	}
	if got != "よろしくお願いいたします。" {
		t.Fatalf("parsed rewrite = %q", got)
	}
}

func TestParseComposeAIRewriteResponseStructuredArrayWithEscapedUnicode(t *testing.T) {
	got, err := parseComposeAIRewriteResponse(`[{"status":"ok","text":"\u3042\u306a\u305f\u306f\u6700\u9ad8\u3067\u3059\u3001Herald\u3002"}]`)
	if err != nil {
		t.Fatalf("parseComposeAIRewriteResponse returned error: %v", err)
	}
	if got != "あなたは最高です、Herald。" {
		t.Fatalf("parsed rewrite = %q", got)
	}
}

func TestParseComposeAIRewriteResponseFindsEmbeddedStructuredJSON(t *testing.T) {
	got, err := parseComposeAIRewriteResponse("Here is the JSON:\n\n{\"status\":\"ok\",\"text\":\"あなたは最高です、Herald。\"}")
	if err != nil {
		t.Fatalf("parseComposeAIRewriteResponse returned error: %v", err)
	}
	if got != "あなたは最高です、Herald。" {
		t.Fatalf("parsed rewrite = %q", got)
	}
}

func TestParseComposeAIRewriteResponseStructuredRefusal(t *testing.T) {
	_, err := parseComposeAIRewriteResponse(`{"status":"error","error_code":"safety_refusal","message":"The model declined this rewrite."}`)
	if err == nil {
		t.Fatal("expected structured refusal error")
	}
	var rewriteErr *composeAIRewriteError
	if !errors.As(err, &rewriteErr) {
		t.Fatalf("error = %T %v, want composeAIRewriteError", err, err)
	}
	if rewriteErr.Code != "safety_refusal" {
		t.Fatalf("rewrite error code = %q", rewriteErr.Code)
	}
}

func TestParseComposeAIRewriteResponsePlainRefusal(t *testing.T) {
	_, err := parseComposeAIRewriteResponse("I'm sorry, but I cannot fulfill your request to rewrite an email that uses derogatory language.")
	if err == nil {
		t.Fatal("expected plain refusal to become an error")
	}
	var rewriteErr *composeAIRewriteError
	if !errors.As(err, &rewriteErr) {
		t.Fatalf("error = %T %v, want composeAIRewriteError", err, err)
	}
	if rewriteErr.Code != "safety_refusal" {
		t.Fatalf("rewrite error code = %q", rewriteErr.Code)
	}
}

func TestParseComposeAIRewriteResponsePlainTextFallback(t *testing.T) {
	got, err := parseComposeAIRewriteResponse("Please review the proposal by Friday.")
	if err != nil {
		t.Fatalf("plain rewrite fallback returned error: %v", err)
	}
	if got != "Please review the proposal by Friday." {
		t.Fatalf("plain rewrite fallback = %q", got)
	}
}

func TestAiAssistCmdParsesStructuredRewriteAndRequestsJSON(t *testing.T) {
	classifier := &stubClassifier{chatResponse: `{"status":"ok","text":"あなたは最高です、Herald。"} `}
	m := New(&stubBackend{}, nil, "", classifier, false)
	m.composeBody.SetValue("you are the best, Herald.")

	msg, ok := m.aiAssistCmd(composeAITranslateInstruction("Japanese"))().(AIAssistMsg)
	if !ok {
		t.Fatalf("expected AIAssistMsg")
	}
	if msg.Err != nil {
		t.Fatalf("aiAssistCmd returned error: %v", msg.Err)
	}
	if msg.Result != "あなたは最高です、Herald。" {
		t.Fatalf("AIAssistMsg.Result = %q", msg.Result)
	}
	if len(classifier.chatMessages) != 2 {
		t.Fatalf("captured %d chat messages, want 2", len(classifier.chatMessages))
	}
	system := classifier.chatMessages[0].Content
	user := classifier.chatMessages[1].Content
	for _, want := range []string{`"status":"ok"`, `"status":"error"`, "error_code", "Return JSON only"} {
		if !strings.Contains(system, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, system)
		}
	}
	if !strings.Contains(user, "Translate this email to Japanese") {
		t.Fatalf("user prompt missing Japanese translation instruction:\n%s", user)
	}
}

func TestAiAssistCmdJapaneseTranslatePromptRequestsNaturalTranslation(t *testing.T) {
	classifier := &stubClassifier{chatResponse: `{"status":"ok","text":"Herald、あなたは最高です。"}`}
	m := New(&stubBackend{}, nil, "", classifier, false)
	m.composeBody.SetValue("you are the best, Herald.")

	msg, ok := m.aiAssistCmd(composeAITranslateInstruction("Japanese"))().(AIAssistMsg)
	if !ok {
		t.Fatalf("expected AIAssistMsg")
	}
	if msg.Err != nil {
		t.Fatalf("aiAssistCmd returned error: %v", msg.Err)
	}
	if len(classifier.chatMessages) != 2 {
		t.Fatalf("captured %d chat messages, want 2", len(classifier.chatMessages))
	}

	system := classifier.chatMessages[0].Content
	user := classifier.chatMessages[1].Content
	for _, want := range []string{
		"natural, idiomatic translation",
		"Do not transliterate source-language sentences",
		"Do not output random kana",
		"standard modern Japanese",
		"Preserve names, signatures, separators, and line breaks",
		"same number of lines",
		"no longer than",
		"Do not add examples, alternatives, explanations, or new content",
	} {
		if !strings.Contains(system+"\n"+user, want) {
			t.Fatalf("Japanese translation prompt missing %q:\nsystem:\n%s\n\nuser:\n%s", want, system, user)
		}
	}
}

func TestAiAssistCmdJapaneseTranslateRejectsKanaNoise(t *testing.T) {
	noisy := "わたしはすをりおいうましんたすえるえりしだたちだになちつすがせつるひつおりしつぜらぜう"
	classifier := &stubClassifier{chatResponse: `{"status":"ok","text":"` + noisy + `"}`}
	m := New(&stubBackend{}, nil, "", classifier, false)
	m.composeBody.SetValue("you are the best, Herald.")

	msg, ok := m.aiAssistCmd(composeAITranslateInstruction("Japanese"))().(AIAssistMsg)
	if !ok {
		t.Fatalf("expected AIAssistMsg")
	}
	if msg.Err == nil {
		t.Fatalf("expected noisy Japanese translation to be rejected, got result %q", msg.Result)
	}
	var rewriteErr *composeAIRewriteError
	if !errors.As(msg.Err, &rewriteErr) {
		t.Fatalf("expected composeAIRewriteError, got %T: %v", msg.Err, msg.Err)
	}
	if rewriteErr.Code != "translation_quality" {
		t.Fatalf("rewriteErr.Code = %q, want translation_quality", rewriteErr.Code)
	}
}

func TestAiAssistCmdJapaneseTranslateRejectsRunawayLength(t *testing.T) {
	runaway := strings.Repeat("Herald、あなたは最高です。", 12)
	classifier := &stubClassifier{chatResponse: `{"status":"ok","text":"` + runaway + `"}`}
	m := New(&stubBackend{}, nil, "", classifier, false)
	m.composeBody.SetValue("you are the best, Herald.")

	msg, ok := m.aiAssistCmd(composeAITranslateInstruction("Japanese"))().(AIAssistMsg)
	if !ok {
		t.Fatalf("expected AIAssistMsg")
	}
	if msg.Err == nil {
		t.Fatalf("expected runaway Japanese translation to be rejected, got result length %d", len([]rune(msg.Result)))
	}
	var rewriteErr *composeAIRewriteError
	if !errors.As(msg.Err, &rewriteErr) {
		t.Fatalf("expected composeAIRewriteError, got %T: %v", msg.Err, msg.Err)
	}
	if rewriteErr.Code != "translation_quality" {
		t.Fatalf("rewriteErr.Code = %q, want translation_quality", rewriteErr.Code)
	}
}

func TestAIAssistRefusalShowsStatusWithoutReplacingSuggestion(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.composeAIPanel = true
	m.composeBody.SetValue("Original draft")
	m.composeAIDiff = "existing diff"
	m.composeAIResponse.SetValue("Existing suggestion")
	m.composeAILoading = true

	updatedModel, _ := m.Update(AIAssistMsg{Err: &composeAIRewriteError{Code: "safety_refusal", Message: "Declined"}})
	updated := updatedModel.(*Model)

	if updated.composeAILoading {
		t.Fatal("refusal should clear loading state")
	}
	if !strings.Contains(updated.composeStatus, "AI warning") || !strings.Contains(updated.composeStatus, "draft was not changed") {
		t.Fatalf("composeStatus = %q", updated.composeStatus)
	}
	if got := updated.composeBody.Value(); got != "Original draft" {
		t.Fatalf("compose body changed to %q", got)
	}
	if got := updated.composeAIResponse.Value(); got != "Existing suggestion" {
		t.Fatalf("AI suggestion changed to %q", got)
	}
	if got := updated.composeAIDiff; got != "existing diff" {
		t.Fatalf("AI diff changed to %q", got)
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
