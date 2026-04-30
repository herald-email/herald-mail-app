package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

	rendered := stripANSI(updated.View())
	if !strings.Contains(rendered, "Shortcut Help") {
		t.Fatalf("expected ? to open shortcut help, got:\n%s", rendered)
	}
	if updated.timeline.searchMode {
		t.Fatal("expected plain ? not to open Timeline semantic search")
	}
	if !strings.Contains(rendered, "F1-F4") || !strings.Contains(rendered, "Timeline") {
		t.Fatalf("expected global and Timeline shortcuts in help, got:\n%s", rendered)
	}

	model, _ := updated.handleKeyMsg(tea.KeyMsg{Type: tea.KeyEsc})
	closed := model.(*Model)
	if strings.Contains(stripANSI(closed.View()), "Shortcut Help") {
		t.Fatal("expected Esc to close shortcut help")
	}
}

func TestShortcutHelpQuestionMarkClosesOverlay(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	opened := pressQuestion(m)
	closed := pressQuestion(opened)

	if strings.Contains(stripANSI(closed.View()), "Shortcut Help") {
		t.Fatal("expected ? to close shortcut help when it is already open")
	}
}

func TestShortcutHelpIncludesComposePreservationMode(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.composePreserved = &composePreservedContext{
		kind: models.PreservedMessageKindReply,
		mode: models.PreservationModeSafe,
	}

	updated := pressQuestion(m)

	rendered := stripANSI(updated.View())
	if !strings.Contains(rendered, "Shortcut Help") {
		t.Fatalf("expected ? to open shortcut help from Compose, got:\n%s", rendered)
	}
	for _, want := range []string{"Compose", "Ctrl+O", "preservation mode"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected Compose help to include %q, got:\n%s", want, rendered)
		}
	}
	if strings.Contains(updated.composeTo.Value(), "?") {
		t.Fatal("expected plain ? not to be typed into Compose fields")
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

	rendered := stripANSI(updated.View())
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

			rendered := stripANSI(updated.View())
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
	if !strings.Contains(stripANSI(updated.View()), "Shortcut Help") {
		t.Fatal("expected plain ? to open Contacts shortcut help")
	}

	model, _ := updated.handleKeyMsg(tea.KeyMsg{Type: tea.KeyEsc})
	m = model.(*Model)
	model, _ = m.handleContactsKey(keyRunes("/"))
	m = model.(*Model)
	for _, r := range "? mara" {
		model, _ = m.handleContactsKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = model.(*Model)
	}

	if m.contactSearchMode != "semantic" {
		t.Fatalf("expected / followed by ? query to switch Contacts search to semantic mode, got %q", m.contactSearchMode)
	}
	if got, want := m.contactSearch, "mara"; got != want {
		t.Fatalf("semantic contact query got %q, want %q", got, want)
	}
}
