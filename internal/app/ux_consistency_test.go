package app

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/ai"
	"mail-processor/internal/models"
)

type aiStatusStub struct {
	status ai.SchedulerStatus
}

func (s *aiStatusStub) Chat(_ []ai.ChatMessage) (string, error) { return "", nil }
func (s *aiStatusStub) ChatWithTools(_ []ai.ChatMessage, _ []ai.Tool) (string, []ai.ToolCall, error) {
	return "", nil, ai.ErrToolsNotSupported
}
func (s *aiStatusStub) Classify(_, _ string) (ai.Category, error)             { return "", nil }
func (s *aiStatusStub) Embed(_ string) ([]float32, error)                     { return nil, nil }
func (s *aiStatusStub) SetEmbeddingModel(_ string)                            {}
func (s *aiStatusStub) GenerateQuickReplies(_, _, _ string) ([]string, error) { return nil, nil }
func (s *aiStatusStub) EnrichContact(_ string, _ []string) (string, []string, error) {
	return "", nil, nil
}
func (s *aiStatusStub) HasVisionModel() bool { return false }
func (s *aiStatusStub) DescribeImage(_ context.Context, _ []byte, _ string) (string, error) {
	return "", nil
}
func (s *aiStatusStub) Ping() error                  { return nil }
func (s *aiStatusStub) AIStatus() ai.SchedulerStatus { return s.status }

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
	m.statusMessage = "Install missing Ollama model: ollama pull nomic-embed-text-v2-moe"

	status := stripANSI(m.renderStatusBar())
	if !strings.Contains(status, "ollama pull nomic-embed-text-v2-moe") {
		t.Fatalf("expected status bar to include status message guidance, got %q", status)
	}
}

func TestRenderStatusBar_ShowsGlobalAIChip(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.classifier = &aiStatusStub{status: ai.SchedulerStatus{
		ActiveKind:             ai.TaskKindEmbedding,
		ActivePriority:         ai.PriorityBackground,
		QueuedInteractiveKind:  ai.TaskKindQuickReply,
		QueuedInteractiveCount: 1,
	}}

	status := stripANSI(m.renderStatusBar())
	if !strings.Contains(status, "AI: quick reply (+1)") {
		t.Fatalf("expected status bar to prefer queued interactive AI chip, got %q", status)
	}
}

func TestHandleTimelineKey_QuickReplyOpensCurrentEmailFromList(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.focusedPanel = panelTimeline

	model, cmd, handled := m.handleTimelineKey(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if !handled {
		t.Fatal("expected ctrl+q to be handled")
	}
	if cmd == nil {
		t.Fatal("expected ctrl+q from list to start opening the current email")
	}
	updated := model.(*Model)
	if updated.timeline.selectedEmail == nil {
		t.Fatal("expected current email to be selected")
	}
	if !updated.timeline.quickReplyPending {
		t.Fatal("expected quick reply to remain pending until body load completes")
	}
}

func TestHandleTimelineMsg_EmailBodyOpensPendingQuickReply(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.selectedEmail = &models.EmailData{
		MessageID: "msg-1",
		Sender:    "alice@example.com",
		Subject:   "Hello",
	}
	m.timeline.quickReplyPending = true

	model, _, handled := m.handleTimelineMsg(EmailBodyMsg{
		MessageID: "msg-1",
		Body:      &models.EmailBody{TextPlain: "hello there"},
	})
	if !handled {
		t.Fatal("expected EmailBodyMsg to be handled")
	}
	updated := model.(*Model)
	if !updated.timeline.quickReplyOpen {
		t.Fatal("expected pending quick reply to open after body load")
	}
	if updated.focusedPanel != panelPreview {
		t.Fatalf("expected focus to move to preview, got %d", updated.focusedPanel)
	}
}
