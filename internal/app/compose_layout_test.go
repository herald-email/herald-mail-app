package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// TestComposeCCBCCWidth_MatchesToField verifies that CC and BCC textinput
// fields are given the same width as the To field after a window resize.
// Regression test for the "tiny box" rendering bug where CC/BCC showed
// only a single character because their Width was never set.
func TestComposeCCBCCWidth_MatchesToField(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	// Simulate a window resize event — this triggers the width calculation
	// that sets composeTo.Width and composeSubject.Width.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)

	if m.composeCC.Width() == 0 {
		t.Fatal("composeCC.Width is 0 — width was never set")
	}
	if m.composeCC.Width() != m.composeTo.Width() {
		t.Fatalf("CC width %d != To width %d", m.composeCC.Width(), m.composeTo.Width())
	}
	if m.composeBCC.Width() != m.composeTo.Width() {
		t.Fatalf("BCC width %d != To width %d", m.composeBCC.Width(), m.composeTo.Width())
	}
}

// TestComposeBodyHeight_FitsTerminal verifies that the compose body textarea
// height is calculated from the currently visible header fields, divider, and
// body borders so the total compose view never overflows the terminal height.
func TestComposeBodyHeight_FitsTerminal(t *testing.T) {
	for _, h := range []int{24, 40, 50, 80} {
		b := &stubBackend{}
		m := New(b, nil, "", nil, false)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: h})
		m = updated.(*Model)

		tableHeight := m.buildLayoutPlan(120, h).ContentHeight

		// Compose renders directly in the main viewport, so it gets the two rows
		// that table panels spend on their own outer border.
		composeViewportRows := tableHeight + 2

		composeExtraRows := m.composeAdditionalRows(tableHeight)
		expectedBodyHeight := composeViewportRows - m.composeFixedRows() - composeExtraRows
		minExpectedBodyHeight := 3
		if composeExtraRows > 0 {
			minExpectedBodyHeight = 1
		}
		if expectedBodyHeight < minExpectedBodyHeight {
			expectedBodyHeight = minExpectedBodyHeight
		}

		got := m.composeBody.Height()
		if got != expectedBodyHeight {
			t.Errorf("h=%d: composeBody.Height()=%d, want %d (would overflow terminal by %d rows)",
				h, got, expectedBodyHeight, got-expectedBodyHeight)
		}
	}
}

func TestComposeBlankView_FillsTerminalHeight(t *testing.T) {
	for _, tc := range []struct {
		name   string
		width  int
		height int
	}{
		{name: "wide", width: 220, height: 50},
		{name: "snapshot", width: 120, height: 40},
		{name: "standard", width: 80, height: 24},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := makeSizedModel(t, tc.width, tc.height)
			m.activeTab = tabCompose
			m.composeField = composeFieldTo
			m.updateTableDimensions(tc.width, tc.height)
			freezeComposeCursors(m)

			rendered := m.renderMainView()
			lines := strings.Split(stripANSI(rendered), "\n")
			if len(lines) != tc.height {
				t.Fatalf("blank Compose rendered %d lines at %dx%d, want exactly %d lines:\n%s",
					len(lines), tc.width, tc.height, tc.height, stripANSI(rendered))
			}
			bottomRows := strings.Join(lines[len(lines)-4:], "\n")
			if !strings.Contains(bottomRows, primaryTabShortcutHint) || strings.TrimSpace(lines[len(lines)-1]) == "" {
				t.Fatalf("expected bottom rows to contain key hints at %dx%d, got:\n%s",
					tc.width, tc.height, bottomRows)
			}
			dividerSeen := false
			for _, line := range lines[len(lines)-4:] {
				if line == strings.Repeat("─", tc.width) {
					dividerSeen = true
					break
				}
			}
			if !dividerSeen {
				t.Fatalf("expected status/key-hint divider near the bottom at %dx%d, got:\n%s",
					tc.width, tc.height, strings.Join(lines[len(lines)-4:], "\n"))
			}
		})
	}
}

func TestComposeAIBarOpensByDefaultForBlankCompose(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline

	cmd := m.openBlankComposeFromCurrent()

	if cmd != nil {
		t.Fatalf("blank compose open should be synchronous, got %T", cmd)
	}
	if !m.composeAIPanel {
		t.Fatal("expected Compose AI bar to be open by default")
	}
	if m.composeAIInput.Focused() {
		t.Fatal("default-open AI bar must not steal focus from the compose fields")
	}
}

func TestComposeAIBarShowsDisabledWarningWhenAIUnavailable(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.classifier = nil
	m.composeAIPanel = true
	m.updateTableDimensions(120, 40)

	rendered := stripANSI(m.renderMainView())

	if !strings.Contains(rendered, "AI disabled") {
		t.Fatalf("expected disabled warning in Compose AI bar, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "Ctrl+T Translate") {
		t.Fatalf("disabled AI bar should not advertise active rewrite controls, got:\n%s", rendered)
	}
}

func TestComposeAIBarRendersCompactInlineAskToolbar(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.classifier = &stubClassifier{}
	m.composeAIPanel = true
	m.updateTableDimensions(120, 40)

	rendered := stripANSI(m.renderMainView())

	for _, want := range []string{"[Translate: ctrl+t]", "[Style: ctrl+y]", "[Fix: ctrl+f]", "[Shorten: ctrl+n]", "[Expand: ctrl+e]", "[Undo: ctrl+z]", "Ask: ctrl+k"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("compact AI toolbar missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "AI  [Translate:") {
		t.Fatalf("compact AI toolbar should not include a leading AI label:\n%s", rendered)
	}
	if strings.Contains(rendered, "Spanish") || strings.Contains(rendered, "Friendly") {
		t.Fatalf("default compact AI toolbar should show shortcuts until a menu option is selected:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Ask: ctrl+k >  ") {
		t.Fatalf("inline Ask field should keep extra spacing before placeholder:\n%s", rendered)
	}
	if strings.Contains(rendered, "Tell AI how to rewrite this draft") {
		t.Fatalf("inline Ask field should not render the old full-width prompt placeholder:\n%s", rendered)
	}
	if strings.Contains(rendered, "Ctrl+G: prompt") {
		t.Fatalf("compact AI toolbar should not advertise Ctrl+G prompt focus:\n%s", rendered)
	}
	toolbarLines := 0
	for _, line := range strings.Split(rendered, "\n") {
		if strings.Contains(line, "AI") || strings.Contains(line, "Ask:") {
			if strings.Contains(line, "Translate:") || strings.Contains(line, "Ask:") {
				toolbarLines++
			}
		}
	}
	if toolbarLines != 1 {
		t.Fatalf("AI toolbar should occupy one compact row, counted %d relevant rows:\n%s", toolbarLines, rendered)
	}
}

func TestComposeAIBarRendersSelectedMenuValuesAfterSelection(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.classifier = &stubClassifier{}
	m.composeAIPanel = true
	m.composeAITranslate = "Spanish"
	m.composeAIStyle = "Friendly"
	m.updateTableDimensions(120, 40)

	rendered := stripANSI(m.renderMainView())

	for _, want := range []string{"Spanish v", "Friendly v"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("compact AI toolbar missing selected value %q:\n%s", want, rendered)
		}
	}
}

func TestComposeAIDropdownRendersBelowDivider(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.classifier = &stubClassifier{}
	m.composeAIPanel = true
	m.composeAIMenu = composeAIMenuTranslate
	m.updateTableDimensions(120, 40)

	lines := strings.Split(stripANSI(m.renderMainView()), "\n")
	toolbarLine := -1
	dividerLine := -1
	dropdownLine := -1
	for i, line := range lines {
		switch {
		case strings.Contains(line, "[Translate: ctrl+t]"):
			toolbarLine = i
		case toolbarLine >= 0 && dividerLine < 0 && strings.Contains(line, "────"):
			dividerLine = i
		case strings.HasPrefix(strings.TrimSpace(line), "Translate:"):
			dropdownLine = i
		}
	}
	if toolbarLine < 0 || dividerLine < 0 || dropdownLine < 0 {
		t.Fatalf("expected toolbar, divider, and dropdown lines, got toolbar=%d divider=%d dropdown=%d:\n%s", toolbarLine, dividerLine, dropdownLine, strings.Join(lines, "\n"))
	}
	if !(toolbarLine < dividerLine && dividerLine < dropdownLine) {
		t.Fatalf("divider should render between toolbar and dropdown, got toolbar=%d divider=%d dropdown=%d:\n%s", toolbarLine, dividerLine, dropdownLine, strings.Join(lines, "\n"))
	}
}

func TestComposeAIBarUsesOneAdditionalRowByDefault(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.classifier = &stubClassifier{}
	m.composeAIPanel = true
	m.composeAIMenu = ""
	m.composeAILoading = false
	m.composeAIDiff = ""
	m.composeAIResponse.SetValue("")

	tableHeight := m.buildLayoutPlan(120, 40).ContentHeight
	if got := m.composeAdditionalRows(tableHeight); got != 1 {
		t.Fatalf("default compact AI toolbar extra rows = %d, want 1", got)
	}
}

func TestComposeCtrlKFocusesInlineAIInstruction(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.classifier = &stubClassifier{}
	m.composeAIPanel = true
	m.composeField = composeFieldBody
	m.composeBody.Focus()

	model, cmd := m.handleComposeKey(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	updated := model.(*Model)

	if cmd != nil {
		t.Fatalf("Ctrl+K should focus the inline AI input synchronously, got command %T", cmd)
	}
	if !updated.composeAIInput.Focused() {
		t.Fatal("Ctrl+K should focus the inline AI instruction input")
	}
	if updated.composeAIResponse.Focused() {
		t.Fatal("Ctrl+K should blur the AI response editor")
	}
	if updated.composeField != composeFieldBody {
		t.Fatalf("Ctrl+K should not change compose field, got %d", updated.composeField)
	}
}

func TestComposeAIInputEnterSubmitsCustomRewrite(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.classifier = &stubClassifier{}
	m.composeAIPanel = true
	m.composeBody.SetValue("Please review this draft.")
	m.composeAIInput.SetValue("make this warmer")
	m.composeAIInput.Focus()

	model, cmd := m.handleComposeKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	updated := model.(*Model)

	if cmd == nil {
		t.Fatal("Enter from focused inline AI input should dispatch a custom rewrite")
	}
	if !updated.composeAILoading {
		t.Fatal("Enter from focused inline AI input should show loading state")
	}
	if got := updated.composeAIInput.Value(); got != "" {
		t.Fatalf("AI input should clear after submit, got %q", got)
	}
}

func TestDefaultOpenComposeAIBarDoesNotStealBodyText(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.composeAIPanel = true
	m.composeField = composeFieldBody
	m.composeBody.Focus()

	for _, r := range []rune("taste fine") {
		model, _ := m.handleComposeKey(tea.KeyPressMsg{Text: string(r), Code: r})
		m = model.(*Model)
	}

	if got := m.composeBody.Value(); got != "taste fine" {
		t.Fatalf("compose body = %q, want literal text preserved with default-open AI bar", got)
	}
}

func TestComposeAIResultRefreshesBodyHeight(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.composeField = composeFieldBody
	m.composeBody.SetValue("pleese review this draft.")
	m.composeAIPanel = true
	m.updateTableDimensions(120, 40)
	heightBefore := m.composeBody.Height()

	updated, _ := m.Update(AIAssistMsg{Result: "Please review this draft."})
	m = updated.(*Model)

	if got := m.composeAIResponse.Height(); got >= heightBefore {
		t.Fatalf("AI response height after AI result = %d, want less than %d so the review chrome fits", got, heightBefore)
	}
	if m.composeAIResponse.Height() != m.composeBody.Height() {
		t.Fatalf("AI response height = %d, want body editor slot height %d", m.composeAIResponse.Height(), m.composeBody.Height())
	}
	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{"AI Assist", "Suggestion", "Original: tab", "Changes", "Please review this draft."} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected AI review to contain %q after layout refresh, got:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Suggestion (edit freely):") {
		t.Fatalf("AI suggestion should replace the body editor instead of rendering the old appended panel:\n%s", rendered)
	}
}

func TestComposeAIReviewRendersFramedReviewSurface(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.composeBody.SetValue("you are the best, Herald.")
	m.composeAIPanel = true

	updated, _ := m.Update(AIAssistMsg{
		Original: "you are the best, Herald.",
		Result:   "I'm sorry, but I cannot fulfill your request.",
	})
	m = updated.(*Model)
	freezeComposeCursors(m)

	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{
		"AI Assist · Suggestion replaces draft",
		"Original: tab",
		"Suggestion",
		"(edit freely)",
		"Changes",
		"Accept ctrl+enter",
		"Discard esc",
		"Edit in place",
		"Undo ctrl+z",
		"Tab original/suggestion",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("AI review surface missing %q:\n%s", want, rendered)
		}
	}
	if strings.Count(rendered, "Suggestion") < 2 {
		t.Fatalf("expected header and section title to both name Suggestion:\n%s", rendered)
	}
	if strings.Contains(rendered, "Suggestion (edit freely):") {
		t.Fatalf("review mode should use the framed mock surface, not the old appended panel:\n%s", rendered)
	}
}

func TestComposeAIResultStripsPromptScaffoldingBeforeReview(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.composeBody.SetValue("pleese review the budget note.")
	m.composeAIPanel = true

	raw := "Please review the budget note.\n\n" +
		"Demo AI: I found the most relevant mailbox context for \"Fix typos\".\n\n" +
		"Current draft:\npleese review the budget note."
	updated, _ := m.Update(AIAssistMsg{
		Original: "pleese review the budget note.",
		Result:   raw,
	})
	m = updated.(*Model)

	if got := m.composeAIResponse.Value(); got != "Please review the budget note." {
		t.Fatalf("cleaned AI suggestion = %q, want only rewritten body", got)
	}
	rendered := stripANSI(m.renderMainView())
	for _, leaked := range []string{"Current draft:", "Demo AI:", "most relevant mailbox context"} {
		if strings.Contains(rendered, leaked) {
			t.Fatalf("AI review leaked prompt scaffolding %q:\n%s", leaked, rendered)
		}
	}
}

func TestComposeAIReviewUsesWordDiffInsteadOfWholeLineReplacement(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.composeBody.SetValue("you are the best, Herald.")
	m.composeAIPanel = true

	updated, _ := m.Update(AIAssistMsg{
		Original: "you are the best, Herald.",
		Result:   "you are the best, Herald!",
	})
	m = updated.(*Model)

	rendered := stripANSI(m.renderMainView())
	for _, unwanted := range []string{"- you are the best, Herald.", "+ you are the best, Herald!"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("changes box should show word-level diff, not whole-line replacement %q:\n%s", unwanted, rendered)
		}
	}
	for _, want := range []string{"you are the best", "Herald", ".", "!"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("word-level diff missing %q:\n%s", want, rendered)
		}
	}
}

func TestComposeAIReviewView_FillsTerminalHeight(t *testing.T) {
	for _, tc := range []struct {
		name   string
		width  int
		height int
	}{
		{name: "wide", width: 220, height: 50},
		{name: "snapshot", width: 120, height: 40},
		{name: "standard", width: 80, height: 24},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := makeSizedModel(t, tc.width, tc.height)
			m.activeTab = tabCompose
			m.classifier = &stubClassifier{}
			m.composeBody.SetValue("you are the best, Herald.")
			m.composeAIPanel = true

			updated, _ := m.Update(AIAssistMsg{
				Original: "you are the best, Herald.",
				Result:   "I'm sorry, but I cannot fulfill your request.",
			})
			m = updated.(*Model)
			freezeComposeCursors(m)

			rendered := m.renderMainView()
			lines := strings.Split(stripANSI(rendered), "\n")
			if len(lines) != tc.height {
				t.Fatalf("AI review Compose rendered %d lines at %dx%d, want exactly %d lines:\n%s",
					len(lines), tc.width, tc.height, tc.height, stripANSI(rendered))
			}
			bottomRows := strings.Join(lines[len(lines)-4:], "\n")
			if !strings.Contains(bottomRows, "ctrl+enter: accept") || !strings.Contains(bottomRows, "ctrl+alt+c/b: CC/BCC") {
				t.Fatalf("expected review key hints near bottom at %dx%d, got:\n%s",
					tc.width, tc.height, bottomRows)
			}
		})
	}
}

func TestComposeAIReviewActionsStayAdjacentToChanges(t *testing.T) {
	m := makeSizedModel(t, 220, 50)
	m.activeTab = tabCompose
	m.classifier = &stubClassifier{}
	m.composeBody.SetValue("original draft")
	m.composeAIPanel = true

	result := strings.Repeat("This suggestion has enough text to wrap across the review editor. ", 8)
	updated, _ := m.Update(AIAssistMsg{Original: "original draft", Result: result})
	m = updated.(*Model)
	freezeComposeCursors(m)

	lines := strings.Split(stripANSI(m.renderMainView()), "\n")
	changesTop := -1
	for i, line := range lines {
		if strings.Contains(line, "Changes") {
			changesTop = i
			break
		}
	}
	if changesTop < 0 {
		t.Fatalf("expected Changes section:\n%s", strings.Join(lines, "\n"))
	}
	changesBottom := -1
	for i := changesTop + 1; i < len(lines); i++ {
		if strings.Contains(lines[i], "└") && strings.Contains(lines[i], "─") {
			changesBottom = i
			break
		}
	}
	acceptLine := -1
	for i := changesTop + 1; i < len(lines); i++ {
		if strings.Contains(lines[i], "Accept ctrl+enter") {
			acceptLine = i
			break
		}
	}
	if changesBottom < 0 || acceptLine < 0 {
		t.Fatalf("expected Changes bottom and Accept row, got bottom=%d accept=%d:\n%s", changesBottom, acceptLine, strings.Join(lines, "\n"))
	}
	if acceptLine-changesBottom > 2 {
		t.Fatalf("Accept row is detached from Changes by %d rows:\n%s", acceptLine-changesBottom, strings.Join(lines, "\n"))
	}
}

func TestComposeAIReviewDoesNotAdvertiseUnsupportedChangePager(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.composeBody.SetValue("hello team")
	m.composeAIPanel = true

	updated, _ := m.Update(AIAssistMsg{Original: "hello team", Result: "hello everyone"})
	m = updated.(*Model)

	rendered := stripANSI(m.renderMainView())
	for _, unwanted := range []string{"1/2", "▲▼", "Alt+[n/p]"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("review should not advertise unsupported change pager hint %q:\n%s", unwanted, rendered)
		}
	}
}

func TestComposeAIReviewCompactViewStartsAtSuggestionTop(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.classifier = &stubClassifier{}
	m.composeBody.SetValue("original draft")
	m.composeAIPanel = true

	result := "First sentence starts the suggestion so it must remain visible. " +
		"Second sentence adds enough text to wrap through the compact review. " +
		"Final sentence should not be the only visible line."
	updated, _ := m.Update(AIAssistMsg{Original: "original draft", Result: result})
	m = updated.(*Model)
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = resized.(*Model)
	freezeComposeCursors(m)

	rendered := stripANSI(m.renderMainView())
	if !strings.Contains(rendered, "First sentence") {
		t.Fatalf("compact review should show the start of the suggestion, got:\n%s", rendered)
	}
}

func TestComposeAIReviewWideSurfaceKeepsChangesAboveFooter(t *testing.T) {
	m := makeSizedModel(t, 220, 50)
	m.activeTab = tabCompose
	m.classifier = &stubClassifier{}
	m.composeBody.SetValue("original draft")
	m.composeAIPanel = true

	result := strings.Repeat("This suggestion has enough text to wrap across the review editor. ", 8)
	updated, _ := m.Update(AIAssistMsg{Original: "original draft", Result: result})
	m = updated.(*Model)
	freezeComposeCursors(m)

	lines := strings.Split(stripANSI(m.renderMainView()), "\n")
	changesLine := -1
	for i, line := range lines {
		if strings.Contains(line, "Changes") {
			changesLine = i
			break
		}
	}
	if changesLine < 0 {
		t.Fatalf("expected framed review to include Changes section:\n%s", strings.Join(lines, "\n"))
	}
	if changesLine > 36 {
		t.Fatalf("Changes section starts too low at line %d; suggestion editor is absorbing the review surface:\n%s", changesLine+1, strings.Join(lines, "\n"))
	}
}

func TestComposeCollapsedCCBCCRevealAndTabOrder(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.composeField = composeFieldTo
	m.composeTo.Focus()
	m.updateTableDimensions(120, 40)

	rendered := stripANSI(m.renderMainView())
	if strings.Contains(rendered, "CC:") || strings.Contains(rendered, "BCC:") {
		t.Fatalf("blank Compose should hide empty CC/BCC rows:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Ctrl+Alt+C CC") || !strings.Contains(rendered, "Ctrl+Alt+B BCC") {
		t.Fatalf("collapsed CC/BCC hint missing:\n%s", rendered)
	}

	m.cycleComposeField()
	if m.composeField != composeFieldSubject {
		t.Fatalf("tab from To should skip hidden CC/BCC and focus Subject, got field %d", m.composeField)
	}
	m.cycleComposeField()
	if m.composeField != composeFieldBody {
		t.Fatalf("tab from Subject should focus Body, got field %d", m.composeField)
	}

	model, cmd := m.handleComposeKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl | tea.ModAlt})
	if cmd != nil {
		t.Fatalf("Ctrl+Alt+C should reveal CC synchronously, got command %T", cmd)
	}
	m = model.(*Model)
	if !m.composeCCVisible() || m.composeField != composeFieldCC || !m.composeCC.Focused() {
		t.Fatalf("Ctrl+Alt+C should reveal and focus CC, visible=%v field=%d focused=%v", m.composeCCVisible(), m.composeField, m.composeCC.Focused())
	}

	model, cmd = m.handleComposeKey(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl | tea.ModAlt})
	if cmd != nil {
		t.Fatalf("Ctrl+Alt+B should reveal BCC synchronously, got command %T", cmd)
	}
	m = model.(*Model)
	if !m.composeBCCVisible() || m.composeField != composeFieldBCC || !m.composeBCC.Focused() {
		t.Fatalf("Ctrl+Alt+B should reveal and focus BCC, visible=%v field=%d focused=%v", m.composeBCCVisible(), m.composeField, m.composeBCC.Focused())
	}

	m.cycleComposeField()
	if m.composeField != composeFieldSubject {
		t.Fatalf("tab from visible BCC should focus Subject, got field %d", m.composeField)
	}
}

func TestComposeNonEmptyCCBCCRenderWithoutExpansion(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.composeCC.SetValue("cc@example.com")
	m.composeBCC.SetValue("bcc@example.com")
	m.composeCCExpanded = false
	m.composeBCCExpanded = false
	m.updateTableDimensions(120, 40)

	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{"CC:", "cc@example.com", "BCC:", "bcc@example.com"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("non-empty CC/BCC should render %q without expansion:\n%s", want, rendered)
		}
	}
}

func TestComposeAIReviewTabTogglesOriginalAndEscDismissesWithoutMutation(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.composeBody.SetValue("original draft")
	m.composeAIPanel = true

	updated, _ := m.Update(AIAssistMsg{Original: "original draft", Result: "better draft"})
	m = updated.(*Model)
	if !m.composeAIReviewActive() || m.composeAIShowOriginal {
		t.Fatalf("expected suggestion review active, active=%v original=%v", m.composeAIReviewActive(), m.composeAIShowOriginal)
	}

	model, _ := m.handleComposeKey(tea.KeyPressMsg{Code: tea.KeyTab})
	m = model.(*Model)
	if !m.composeAIShowOriginal {
		t.Fatal("Tab should switch AI review to the original draft")
	}
	rendered := stripANSI(m.renderMainView())
	if !strings.Contains(rendered, "AI Assist · Original") || !strings.Contains(rendered, "original draft") {
		t.Fatalf("original review missing:\n%s", rendered)
	}

	model, _ = m.handleComposeKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(*Model)
	if m.composeAIReviewActive() {
		t.Fatal("Esc should dismiss AI review")
	}
	if got := m.composeBody.Value(); got != "original draft" {
		t.Fatalf("Esc mutated compose body to %q", got)
	}
}

func TestComposeAIReviewCtrlEnterAcceptsAndUndoRestores(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.classifier = &stubClassifier{}
	m.composeBody.SetValue("helo team")
	m.composeAIPanel = true

	updated, _ := m.Update(AIAssistMsg{Original: "helo team", Result: "hello team"})
	m = updated.(*Model)

	model, cmd := m.handleComposeKey(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	if cmd != nil {
		t.Fatalf("Ctrl+Enter should accept synchronously, got command %T", cmd)
	}
	m = model.(*Model)
	if m.composeAIReviewActive() {
		t.Fatal("Ctrl+Enter should close AI review")
	}
	if got := m.composeBody.Value(); got != "hello team" {
		t.Fatalf("compose body after Ctrl+Enter = %q", got)
	}
	if got := m.composeAIUndoBody; got != "helo team" {
		t.Fatalf("composeAIUndoBody = %q", got)
	}
	if !m.composeAIPanel {
		t.Fatal("accepting a suggestion should leave the AI toolbar available")
	}
	rendered := stripANSI(m.renderMainView())
	if !strings.Contains(rendered, "[Fix: ctrl+f]") {
		t.Fatalf("AI toolbar missing after accept:\n%s", rendered)
	}
	model, cmd = m.handleComposeKey(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m = model.(*Model)
	if cmd == nil {
		t.Fatal("Ctrl+F should still start an AI rewrite after accepting a suggestion")
	}
	if !m.composeAILoading {
		t.Fatal("Ctrl+F after accept should show AI loading state")
	}

	m.undoComposeAIRewrite()
	if got := m.composeBody.Value(); got != "helo team" {
		t.Fatalf("compose body after undo = %q", got)
	}
}

func TestComposeAILengthShortcutsStartActions(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  rune
	}{
		{name: "shorten", key: 'n'},
		{name: "expand", key: 'e'},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := makeSizedModel(t, 120, 40)
			m.activeTab = tabCompose
			m.composeAIPanel = true
			m.classifier = &stubClassifier{}
			m.composeBody.SetValue("Please review this draft.")

			model, cmd := m.handleComposeKey(tea.KeyPressMsg{Code: tc.key, Mod: tea.ModCtrl})
			updated := model.(*Model)

			if cmd == nil {
				t.Fatal("expected length shortcut to dispatch an AI rewrite command")
			}
			if !updated.composeAILoading {
				t.Fatal("expected length shortcut to show AI loading state")
			}
		})
	}
}

// TestComposeFunctionKeyF3_SwitchesToContacts verifies that F3 switches from Compose
// to Contacts while plain "3" remains available as draft text.
//
// Regression test for the compose-safe command layer: global tab switching uses
// function keys when a Compose text field is focused.
func TestComposeFunctionKeyF3_SwitchesToContacts(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCompose

	updated2, _ := m.Update(functionKey(3))
	m2 := updated2.(*Model)

	if m2.activeTab != tabContacts {
		t.Fatalf("pressing F3 in compose: activeTab=%d, want %d (tabContacts)", m2.activeTab, tabContacts)
	}
}

func TestComposeAutocomplete_DoesNotPushChromeOffscreen(t *testing.T) {
	contacts := []models.ContactData{
		{DisplayName: "Rowan Finch", Email: "rowan@protonmail.com"},
		{DisplayName: "Rowan Finch", Email: "rowan@proton.me"},
		{DisplayName: "Rowan Finch", Email: "rowan.finch@protonmail.com"},
		{DisplayName: "Rowan from Manager.dev", Email: "managerdotdev@mail.beehiiv.com"},
		{DisplayName: "Rowan Finch", Email: "rowan@pm.me"},
	}

	lineCount := func(rendered string) int {
		stripped := strings.TrimRight(stripANSI(rendered), "\n")
		if stripped == "" {
			return 0
		}
		return len(strings.Split(stripped, "\n"))
	}

	for _, tc := range []struct {
		name   string
		width  int
		height int
	}{
		{name: "wide", width: 220, height: 50},
		{name: "standard", width: 80, height: 24},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := makeSizedModel(t, tc.width, tc.height)
			m.activeTab = tabCompose
			m.composeField = 0
			m.composeTo.SetValue("rowan")
			updated, _ := m.Update(ContactSuggestionsMsg{Contacts: contacts})
			m = updated.(*Model)
			freezeComposeCursors(m)

			rendered := m.renderMainView()
			if got := lineCount(rendered); got > tc.height {
				t.Fatalf("compose autocomplete rendered %d lines at %dx%d, exceeding viewport height\n%s", got, tc.width, tc.height, stripANSI(rendered))
			}

			stripped := stripANSI(rendered)
			if !strings.Contains(stripped, "Herald") {
				t.Fatalf("expected compose chrome to remain visible, got:\n%s", stripped)
			}
			if !strings.Contains(stripped, "To:") {
				t.Fatalf("expected active To field to remain visible, got:\n%s", stripped)
			}
		})
	}
}
