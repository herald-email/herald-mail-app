package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func pressQuestion(m *Model) *Model {
	model, _ := m.handleKeyMsg(keyRunes("?"))
	return model.(*Model)
}

func TestShortcutHelpQuestionMarkOpensOverlayFromTimeline(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	updated := pressQuestion(m)

	rendered := stripANSI(updated.View().Content)
	if !strings.Contains(rendered, "Shortcut Help") {
		t.Fatalf("expected ? to open shortcut help, got:\n%s", rendered)
	}
	if updated.timeline.searchMode {
		t.Fatal("expected plain ? not to open Timeline semantic search")
	}
	if !strings.Contains(rendered, "F1-F3") || !strings.Contains(rendered, "Timeline") || !strings.Contains(rendered, "open a blank Compose") {
		t.Fatalf("expected global and Timeline shortcuts in help, got:\n%s", rendered)
	}

	model, _ := updated.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	closed := model.(*Model)
	if strings.Contains(stripANSI(closed.View().Content), "Shortcut Help") {
		t.Fatal("expected Esc to close shortcut help")
	}
}

func TestShortcutHelpRendersCompactCenteredModalOverCurrentView(t *testing.T) {
	m := makeSizedModel(t, 220, 50)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	updated := pressQuestion(m)

	rendered := updated.View().Content
	assertFitsWidth(t, 220, rendered)
	lines := strings.Split(stripANSI(rendered), "\n")
	if len(lines) < 40 {
		t.Fatalf("expected full terminal-height view with modal overlay, got %d lines:\n%s", len(lines), stripANSI(rendered))
	}
	if !strings.Contains(lines[0], "Herald") {
		t.Fatalf("expected current view to remain visible above centered help modal, got first line %q", lines[0])
	}

	titleRow := -1
	titleCol := -1
	for i, line := range lines {
		if col := strings.Index(line, "Shortcut Help - Timeline"); col >= 0 {
			titleRow = i
			titleCol = col
			break
		}
	}
	if titleRow < 8 {
		t.Fatalf("expected help title to be vertically centered below the top chrome, row=%d:\n%s", titleRow, stripANSI(rendered))
	}
	if titleCol < 40 || titleCol > 80 {
		t.Fatalf("expected help title to be horizontally centered in a compact modal, col=%d:\n%s", titleCol, stripANSI(rendered))
	}
}

func TestShortcutHelpFitsAt80ColsAsModal(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	updated := pressQuestion(m)

	rendered := updated.View().Content
	assertFitsWidth(t, 80, rendered)
	lines := strings.Split(strings.TrimRight(stripANSI(rendered), "\n"), "\n")
	if len(lines) > 24 {
		t.Fatalf("expected shortcut help modal to fit 80x24 height, got %d lines:\n%s", len(lines), stripANSI(rendered))
	}
	if !strings.Contains(stripANSI(rendered), "Shortcut Help - Timeline") {
		t.Fatalf("expected shortcut help title at 80x24, got:\n%s", stripANSI(rendered))
	}
}

func TestShortcutHelpPageStepUsesModalVisibleRows(t *testing.T) {
	m := makeSizedModel(t, 220, 50)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	updated := pressQuestion(m)

	if got, want := updated.shortcutHelpPageStep(), 19; got != want {
		t.Fatalf("page step should use compact modal visible rows, got %d want %d", got, want)
	}
}

func TestShortcutHelpQuestionMarkClosesOverlay(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	opened := pressQuestion(m)
	closed := pressQuestion(opened)

	if strings.Contains(stripANSI(closed.View().Content), "Shortcut Help") {
		t.Fatal("expected ? to close shortcut help when it is already open")
	}
}

func TestComposeQuestionMarkTypesIntoEditableFieldsAndDoesNotOpenHelp(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Model)
		value func(*Model) string
	}{
		{
			name: "to",
			setup: func(m *Model) {
				focusComposeTextField(m, composeFieldTo)
			},
			value: func(m *Model) string { return m.composeTo.Value() },
		},
		{
			name: "cc",
			setup: func(m *Model) {
				focusComposeTextField(m, composeFieldCC)
			},
			value: func(m *Model) string { return m.composeCC.Value() },
		},
		{
			name: "bcc",
			setup: func(m *Model) {
				focusComposeTextField(m, composeFieldBCC)
			},
			value: func(m *Model) string { return m.composeBCC.Value() },
		},
		{
			name: "subject",
			setup: func(m *Model) {
				focusComposeTextField(m, composeFieldSubject)
			},
			value: func(m *Model) string { return m.composeSubject.Value() },
		},
		{
			name: "body",
			setup: func(m *Model) {
				focusComposeTextField(m, composeFieldBody)
			},
			value: func(m *Model) string { return m.composeBody.Value() },
		},
		{
			name: "attachment path",
			setup: func(m *Model) {
				m.attachmentInputActive = true
				m.attachmentPathInput.Focus()
			},
			value: func(m *Model) string { return m.attachmentPathInput.Value() },
		},
		{
			name: "AI prompt",
			setup: func(m *Model) {
				m.composeAIPanel = true
				m.composeAIInput.Focus()
			},
			value: func(m *Model) string { return m.composeAIInput.Value() },
		},
		{
			name: "AI response",
			setup: func(m *Model) {
				m.composeAIPanel = true
				m.composeAIResponse.Focus()
			},
			value: func(m *Model) string { return m.composeAIResponse.Value() },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := makeSizedModel(t, 120, 40)
			m.activeTab = tabCompose
			tc.setup(m)

			model, _ := m.handleKeyMsg(keyRunes("?"))
			updated := model.(*Model)

			if updated.showHelp {
				t.Fatal("expected plain ? to stay in the editable Compose field, not open shortcut help")
			}
			if got := tc.value(updated); got != "?" {
				t.Fatalf("compose editable value=%q, want literal ?", got)
			}
		})
	}
}

func TestComposeQuestionMarkNotAdvertisedAsHelp(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	focusComposeTextField(m, composeFieldBody)

	hints := stripANSI(m.renderKeyHints())
	if strings.Contains(hints, "?: help") {
		t.Fatalf("expected Compose editable hints not to advertise ? help, got:\n%s", hints)
	}
}

func TestPromptEditorQuestionMarkTypesIntoNameAndDoesNotOpenHelp(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCleanup
	m.showPromptEditor = true
	m.promptEditor = NewPromptEditor(nil, m.windowWidth, m.windowHeight)
	_ = m.promptEditor.Init()

	model, _ := m.Update(keyRunes("?"))
	updated := model.(*Model)

	if updated.showHelp {
		t.Fatal("expected prompt editor ? to stay in the form, not open shortcut help")
	}
	if got := updated.promptEditor.name; got != "?" {
		t.Fatalf("prompt editor name=%q, want literal ?", got)
	}
}

func focusComposeTextField(m *Model, field int) {
	m.composeTo.Blur()
	m.composeCC.Blur()
	m.composeBCC.Blur()
	m.composeSubject.Blur()
	m.composeBody.Blur()
	m.composeAIInput.Blur()
	m.composeAIResponse.Blur()
	m.composeField = field
	switch field {
	case composeFieldTo:
		m.composeTo.Focus()
	case composeFieldCC:
		m.composeCC.Focus()
	case composeFieldBCC:
		m.composeBCC.Focus()
	case composeFieldSubject:
		m.composeSubject.Focus()
	case composeFieldBody:
		m.composeBody.Focus()
	}
}

func TestShortcutHelpTimelineDraftPreviewIncludesDraftActions(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = []*models.EmailData{
		{
			MessageID: "draft",
			UID:       42,
			Sender:    "me@example.com",
			Subject:   "Re: Interview",
			Folder:    "Drafts",
			IsDraft:   true,
		},
	}
	m.updateTimelineTable()
	m.timeline.selectedEmail = m.timeline.emails[0]
	m.timeline.bodyMessageID = "draft"
	m.timeline.body = &models.EmailBody{TextPlain: "draft body"}
	m.focusedPanel = panelPreview

	updated := pressQuestion(m)

	rendered := stripANSI(updated.View().Content)
	for _, want := range []string{"Timeline Draft", "E", "Ctrl+S", "send draft", "D", "discard draft"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected draft preview help to include %q, got:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "reply or forward") {
		t.Fatalf("draft preview help should not advertise normal reply/forward actions, got:\n%s", rendered)
	}
}

func TestShortcutHelpOpensFromLogsChatCleanupAndConfirmation(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Model)
		want  string
	}{
		{
			name: "logs",
			setup: func(m *Model) {
				m.showLogs = true
			},
			want: "Logs",
		},
		{
			name: "chat",
			setup: func(m *Model) {
				m.showChat = true
				m.focusedPanel = panelChat
				m.chatInput.Focus()
			},
			want: "Chat",
		},
		{
			name: "cleanup preview",
			setup: func(m *Model) {
				m.activeTab = tabCleanup
				m.showCleanupPreview = true
				m.cleanupPreviewEmail = &models.EmailData{MessageID: "cleanup-a", Subject: "Cleanup A"}
				m.cleanupEmailBody = &models.EmailBody{TextPlain: "cleanup body"}
			},
			want: "Cleanup Preview",
		},
		{
			name: "delete confirmation",
			setup: func(m *Model) {
				m.pendingDeleteConfirm = true
				m.pendingDeleteDesc = "Delete selected mail?"
			},
			want: "Confirmation",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := makeSizedModel(t, 140, 40)
			m.activeTab = tabTimeline
			m.timeline.emails = mockEmails()
			m.updateTimelineTable()
			tc.setup(m)

			updated := pressQuestion(m)

			rendered := stripANSI(updated.View().Content)
			if !strings.Contains(rendered, "Shortcut Help") || !strings.Contains(rendered, tc.want) {
				t.Fatalf("expected help for %s context, got:\n%s", tc.want, rendered)
			}
		})
	}
}

func TestRenderKeyHintsAdvertisesShortcutHelpAt80Cols(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	hints := m.renderKeyHints()
	assertFitsWidth(t, 80, hints)
	if !strings.Contains(stripANSI(hints), "?: help") {
		t.Fatalf("expected key hints to advertise ? help, got:\n%s", stripANSI(hints))
	}
}

func TestRenderKeyHintsAdvertisesSettingsAt80Cols(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	hints := m.renderKeyHints()
	assertFitsWidth(t, 80, hints)
	if !strings.Contains(stripANSI(hints), "S: settings") {
		t.Fatalf("expected key hints to advertise S settings, got:\n%s", stripANSI(hints))
	}
}

func TestTimelinePlainQuestionMarkDoesNotOpenSemanticSearch(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	updated := pressQuestion(m)

	if updated.timeline.searchMode {
		t.Fatal("expected plain ? to open help instead of Timeline semantic search")
	}
}

func TestContactsPlainQuestionMarkOpensHelpAndSemanticSearchUsesSlashPrefix(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabContacts
	m.contactsList = []models.ContactData{{Email: "mara@forgepoint.example", DisplayName: "Mara Vale", Company: "Forgepoint Labs"}}
	m.contactsFiltered = m.contactsList

	updated := pressQuestion(m)

	if updated.contactSearchMode == "semantic" {
		t.Fatal("expected plain ? not to enter Contacts semantic search")
	}
	if !strings.Contains(stripANSI(updated.View().Content), "Shortcut Help") {
		t.Fatal("expected plain ? to open Contacts shortcut help")
	}

	model, _ := updated.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(*Model)
	model, _ = m.handleContactsKey(keyRunes("/"))
	m = model.(*Model)
	for _, r := range "? mara" {
		model, _ = m.handleContactsKey(keyRune(r))
		m = model.(*Model)
	}

	if m.contactSearchMode != "semantic" {
		t.Fatalf("expected / followed by ? query to switch Contacts search to semantic mode, got %q", m.contactSearchMode)
	}
	if got, want := m.contactSearch, "mara"; got != want {
		t.Fatalf("semantic contact query got %q, want %q", got, want)
	}
}
