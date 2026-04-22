package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/models"
)

type attachmentBackend struct {
	stubBackend
	savedAttachment *models.Attachment
	savedPath       string
}

func (b *attachmentBackend) SaveAttachment(att *models.Attachment, destPath string) error {
	b.savedAttachment = att
	b.savedPath = destPath
	return nil
}

func TestRenderStatusBar_DoesNotLeakCleanupSelectionOutsideCleanup(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.selectedRows = map[int]bool{0: true}
	m.selectedMessages = map[string]bool{"msg-1": true}
	m.emailsBySender = map[string][]*models.EmailData{
		"alice@example.com": {{MessageID: "msg-1"}},
	}
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	status := stripANSI(m.renderStatusBar())
	if strings.Contains(status, "selected") {
		t.Fatalf("expected timeline status to hide cleanup selection state, got %q", status)
	}
}

func TestHandleTimelineKey_AttachmentNavigationMovesSelection(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.activeTab = tabTimeline
	m.loading = false
	m.focusedPanel = panelPreview
	m.timeline.selectedEmail = &models.EmailData{MessageID: "msg-1"}
	m.timeline.body = &models.EmailBody{
		Attachments: []models.Attachment{
			{Filename: "first.pdf"},
			{Filename: "second.pdf"},
			{Filename: "third.pdf"},
		},
	}

	model, _, handled := m.handleTimelineKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if !handled {
		t.Fatal("expected ] to be handled in preview with attachments")
	}
	updated := model.(*Model)
	if updated.timeline.selectedAttachment != 1 {
		t.Fatalf("expected selected attachment to advance to 1, got %d", updated.timeline.selectedAttachment)
	}

	model, _, handled = updated.handleTimelineKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	if !handled {
		t.Fatal("expected [ to be handled in preview with attachments")
	}
	updated = model.(*Model)
	if updated.timeline.selectedAttachment != 0 {
		t.Fatalf("expected selected attachment to move back to 0, got %d", updated.timeline.selectedAttachment)
	}
}

func TestRenderKeyHints_ShowAttachmentNavigationWhenMultipleAttachments(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	m.timeline.selectedEmail = &models.EmailData{MessageID: "msg-1"}
	m.timeline.body = &models.EmailBody{
		Attachments: []models.Attachment{
			{Filename: "first.pdf"},
			{Filename: "second.pdf"},
		},
	}

	hints := stripANSI(m.renderKeyHints())
	if !strings.Contains(hints, "[") || !strings.Contains(hints, "]") {
		t.Fatalf("expected attachment navigation hints for multi-attachment preview, got %q", hints)
	}
}

func TestHandleOverlayKey_AttachmentSaveUsesCurrentSelection(t *testing.T) {
	backend := &attachmentBackend{}
	m := New(backend, nil, "", nil, false)
	m.activeTab = tabTimeline
	m.loading = false
	m.focusedPanel = panelPreview
	m.timeline.selectedEmail = &models.EmailData{MessageID: "msg-1"}
	m.timeline.body = &models.EmailBody{
		Attachments: []models.Attachment{
			{Filename: "first.pdf"},
			{Filename: "second.pdf"},
		},
	}
	m.timeline.selectedAttachment = 1
	m.timeline.attachmentSavePrompt = true
	m.timeline.attachmentSaveInput.SetValue("/tmp/second.pdf")

	model, cmd, handled := m.handleOverlayKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatal("expected attachment save overlay to handle Enter")
	}
	if cmd == nil {
		t.Fatal("expected attachment save command")
	}
	updated := model.(*Model)
	if updated.timeline.attachmentSavePrompt {
		t.Fatal("expected attachment save prompt to close")
	}

	msg := cmd()
	if _, ok := msg.(AttachmentSavedMsg); !ok {
		t.Fatalf("expected AttachmentSavedMsg, got %T", msg)
	}
	if backend.savedAttachment == nil || backend.savedAttachment.Filename != "second.pdf" {
		t.Fatalf("expected second attachment to be saved, got %+v", backend.savedAttachment)
	}
	if backend.savedPath != "/tmp/second.pdf" {
		t.Fatalf("expected save path /tmp/second.pdf, got %q", backend.savedPath)
	}
}

func TestRenderStatusBar_ShowsStatusMessage(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.statusMessage = "Install missing Ollama model: ollama pull nomic-embed-text"

	status := stripANSI(m.renderStatusBar())
	if !strings.Contains(status, "ollama pull nomic-embed-text") {
		t.Fatalf("expected status bar to include status message guidance, got %q", status)
	}
}
