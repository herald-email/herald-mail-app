package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/models"
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
	m.selectedSummaryKeys = map[string]bool{"alice@example.com": true}
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

func TestHandleTimelineKey_AttachmentSavePromptSuggestsAvailableDefaultPath(t *testing.T) {
	home := t.TempDir()
	downloads := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatalf("create downloads dir: %v", err)
	}
	existing := filepath.Join(downloads, "report.pdf")
	if err := os.WriteFile(existing, []byte("original"), 0o644); err != nil {
		t.Fatalf("write existing attachment path: %v", err)
	}
	t.Setenv("HOME", home)

	m := New(&attachmentBackend{}, nil, "", nil, false)
	m.activeTab = tabTimeline
	m.loading = false
	m.focusedPanel = panelPreview
	m.timeline.selectedEmail = &models.EmailData{MessageID: "msg-1"}
	m.timeline.body = &models.EmailBody{
		Attachments: []models.Attachment{{Filename: "report.pdf"}},
	}

	model, _, handled := m.handleTimelineKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if !handled {
		t.Fatal("expected s to be handled in preview with attachments")
	}
	updated := model.(*Model)
	if !updated.timeline.attachmentSavePrompt {
		t.Fatal("expected attachment save prompt to open")
	}

	want := filepath.Join(downloads, "report (1).pdf")
	if got := updated.timeline.attachmentSaveInput.Value(); got != want {
		t.Fatalf("save input got %q, want %q", got, want)
	}
	if !strings.Contains(updated.timeline.attachmentSaveWarning, "already exists") {
		t.Fatalf("expected collision warning, got %q", updated.timeline.attachmentSaveWarning)
	}
}

func TestHandleOverlayKey_AttachmentSaveRefusesExistingCustomPath(t *testing.T) {
	backend := &attachmentBackend{}
	dir := t.TempDir()
	existing := filepath.Join(dir, "second.pdf")
	if err := os.WriteFile(existing, []byte("original"), 0o644); err != nil {
		t.Fatalf("write existing attachment path: %v", err)
	}

	m := New(backend, nil, "", nil, false)
	m.activeTab = tabTimeline
	m.loading = false
	m.focusedPanel = panelPreview
	m.timeline.selectedEmail = &models.EmailData{MessageID: "msg-1"}
	m.timeline.body = &models.EmailBody{
		Attachments: []models.Attachment{{Filename: "first.pdf"}, {Filename: "second.pdf"}},
	}
	m.timeline.selectedAttachment = 1
	m.timeline.attachmentSavePrompt = true
	m.timeline.attachmentSaveInput.SetValue(existing)

	model, cmd, handled := m.handleOverlayKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatal("expected attachment save overlay to handle Enter")
	}
	if cmd != nil {
		t.Fatal("expected no save command for an existing destination")
	}
	updated := model.(*Model)
	if !updated.timeline.attachmentSavePrompt {
		t.Fatal("expected attachment save prompt to remain open")
	}
	if backend.savedAttachment != nil {
		t.Fatalf("expected backend save to be skipped, got %+v", backend.savedAttachment)
	}
	want := filepath.Join(dir, "second (1).pdf")
	if got := updated.timeline.attachmentSaveInput.Value(); got != want {
		t.Fatalf("save input got %q, want %q", got, want)
	}
	if !strings.Contains(updated.timeline.attachmentSaveWarning, "already exists") {
		t.Fatalf("expected collision warning, got %q", updated.timeline.attachmentSaveWarning)
	}

	contents, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("read existing path: %v", err)
	}
	if string(contents) != "original" {
		t.Fatalf("existing file was overwritten: %q", contents)
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
	if !strings.Contains(status, "AI reply") {
		t.Fatalf("expected status bar to prefer queued interactive AI chip, got %q", status)
	}
}

func TestRenderStatusBar_ShowsTaggingAndEmbeddingProgress(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.classifying = true
	m.classifyDone = 12
	m.classifyTotal = 48
	m.embeddingDone = 64
	m.embeddingTotal = 256

	status := stripANSI(m.renderStatusBar())
	if !strings.Contains(status, "tag 12/48") {
		t.Fatalf("expected status bar to show tagging progress, got %q", status)
	}
	if !strings.Contains(status, "embed 64/256") {
		t.Fatalf("expected status bar to show embedding progress, got %q", status)
	}
}

func TestRenderStatusBar_UsesCompactFragmentsAt80Cols(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.statusMessage = "Demo data loaded"
	m.demoMode = true
	m.sidebarTooWide = true

	status := stripANSI(m.renderStatusBar())
	if !strings.Contains(status, "12u/38t") {
		t.Fatalf("expected narrow status bar to compact folder counts, got %q", status)
	}
	if !strings.Contains(status, "sidebar hidden") {
		t.Fatalf("expected narrow status bar to keep the sidebar-hidden notice readable, got %q", status)
	}
	if strings.Contains(status, "sidebar hi…") {
		t.Fatalf("expected narrow status bar to avoid truncating the sidebar-hidden notice, got %q", status)
	}
}

func TestRenderKeyHints_HidesManualAITagHint(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	hints := stripANSI(m.renderKeyHints())
	if strings.Contains(hints, "a: AI tag") {
		t.Fatalf("expected automatic tagging flow to hide manual AI tag hint, got %q", hints)
	}
}

func requireHintSegments(t *testing.T, hints string, segments ...string) {
	t.Helper()
	for _, segment := range segments {
		if !strings.Contains(hints, segment) {
			t.Fatalf("expected hints to include %q, got %q", segment, hints)
		}
	}
}

func TestRenderKeyHints_TimelinePreviewFocusKeepsMessageActions(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.timeline.selectedEmail = m.timeline.emails[0]
	m.timeline.bodyMessageID = m.timeline.selectedEmail.MessageID
	m.timeline.body = &models.EmailBody{TextPlain: "hello world"}
	m.focusedPanel = panelPreview

	hints := stripANSI(m.renderKeyHints())
	requireHintSegments(t, hints, "*: star", "R: reply", "F: forward", "D: delete", "e: archive", "A: re-classify")
}

func TestRenderKeyHints_TimelineSearchPreviewFocusKeepsMessageActions(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.openTimelineSearch()
	m.timeline.searchInput.SetValue("test")
	m.timeline.searchResults = []*models.EmailData{m.timeline.emails[0]}
	m.timeline.searchResultsQuery = "test"
	m.timeline.searchFocus = timelineSearchFocusResults
	m.timeline.selectedEmail = m.timeline.emails[0]
	m.timeline.bodyMessageID = m.timeline.selectedEmail.MessageID
	m.timeline.body = &models.EmailBody{TextPlain: "hello world"}
	m.focusedPanel = panelPreview

	hints := stripANSI(m.renderKeyHints())
	requireHintSegments(t, hints, "*: star", "R: reply", "F: forward", "D: delete", "e: archive", "A: re-classify")
}

func TestRenderKeyHints_TimelinePreviewActionsStayVisibleAt80Cols(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.timeline.selectedEmail = m.timeline.emails[0]
	m.timeline.bodyMessageID = m.timeline.selectedEmail.MessageID
	m.timeline.body = &models.EmailBody{
		TextPlain: "hello world",
		Attachments: []models.Attachment{
			{Filename: "report.pdf"},
		},
	}
	m.focusedPanel = panelPreview

	hints := m.renderKeyHints()
	assertFitsWidth(t, 80, hints)
	requireHintSegments(t, stripANSI(hints), "R: reply", "F: forward", "D: delete", "e: archive")
}

func TestRenderKeyHints_ReadOnlyTimelinePreviewStillHidesMessageActions(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.currentFolder = virtualFolderAllMailOnly
	m.timeline.emails = []*models.EmailData{
		{MessageID: "<a@x.com>", Sender: "a@x.com", Subject: "only in all mail", Folder: "All Mail"},
	}
	m.updateTimelineTable()
	m.timeline.selectedEmail = m.timeline.emails[0]
	m.timeline.bodyMessageID = m.timeline.selectedEmail.MessageID
	m.timeline.body = &models.EmailBody{TextPlain: "read only"}
	m.focusedPanel = panelPreview

	hints := stripANSI(m.renderKeyHints())
	for _, forbidden := range []string{"*: star", "R: reply", "F: forward", "D: delete", "e: archive", "A: re-classify"} {
		if strings.Contains(hints, forbidden) {
			t.Fatalf("expected read-only preview hints to omit %q, got %q", forbidden, hints)
		}
	}
	if !strings.Contains(hints, "read-only") {
		t.Fatalf("expected read-only hint context, got %q", hints)
	}
}

func TestRenderKeyHints_PrefersChatControlsOverTimelineHints(t *testing.T) {
	m := makeSizedModel(t, 220, 50)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.showChat = true
	m.focusedPanel = panelChat
	m.updateTableDimensions(220, 50)

	hints := stripANSI(m.renderKeyHints())
	if !strings.Contains(hints, "enter: send") {
		t.Fatalf("expected chat controls when chat is visible, got %q", hints)
	}
	if strings.Contains(hints, "R: reply") {
		t.Fatalf("expected timeline hints to be hidden while chat is visible, got %q", hints)
	}
}

func TestRenderKeyHints_PrefersLogControlsOverTimelineHints(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.showLogs = true

	hints := stripANSI(m.renderKeyHints())
	if !strings.Contains(hints, "l: close logs") {
		t.Fatalf("expected log controls when overlay is visible, got %q", hints)
	}
	if strings.Contains(hints, "R: reply") {
		t.Fatalf("expected timeline hints to be hidden while logs are visible, got %q", hints)
	}
}

func TestRenderKeyHints_ShowsContactsPreviewControls(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabContacts
	m.contactFocusPanel = 1
	contact := models.ContactData{
		Email:       "newsletter@techweekly.example",
		DisplayName: "Tech Weekly",
		EmailCount:  8,
	}
	m.contactsFiltered = []models.ContactData{contact}
	m.contactDetail = &contact
	m.contactDetailEmails = []*models.EmailData{
		{
			MessageID: "msg-1",
			Sender:    "Tech Weekly <newsletter@techweekly.example>",
			Subject:   "This Week in Tech #1",
			Folder:    "INBOX",
		},
	}
	m.contactPreviewEmail = m.contactDetailEmails[0]
	m.contactPreviewBody = &models.EmailBody{TextPlain: "hello world"}

	hints := stripANSI(m.renderKeyHints())
	if !strings.Contains(strings.ToLower(hints), "back to contact") {
		t.Fatalf("expected contacts preview controls, got %q", hints)
	}
	if strings.Contains(hints, "nav emails") {
		t.Fatalf("expected email-list hints to be hidden while preview is open, got %q", hints)
	}
}

func TestContactEnrichmentStatus_IsScopedToContactsTab(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabContacts

	model, _ := m.Update(ContactEnrichedMsg{Count: 1})
	updated := model.(*Model)

	contactsStatus := stripANSI(updated.renderStatusBar())
	if !strings.Contains(contactsStatus, "Enriched 1 contacts") {
		t.Fatalf("expected contacts tab to show enrichment status, got %q", contactsStatus)
	}

	updated.activeTab = tabTimeline
	timelineStatus := stripANSI(updated.renderStatusBar())
	if strings.Contains(timelineStatus, "Enriched 1 contacts") {
		t.Fatalf("expected enrichment status to stay scoped to contacts, got %q", timelineStatus)
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

func TestHandleTimelineMsg_PrefixesAIGeneratedReplies(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline

	model, _, handled := m.handleTimelineMsg(QuickRepliesMsg{
		Replies: []string{
			"I'll get back to you.",
			"[AI] Already prefixed",
			"",
		},
	})
	if !handled {
		t.Fatal("expected QuickRepliesMsg to be handled")
	}
	updated := model.(*Model)
	if len(updated.timeline.quickReplies) != 2 {
		t.Fatalf("expected 2 non-empty replies, got %d", len(updated.timeline.quickReplies))
	}
	if updated.timeline.quickReplies[0] != "[AI] I'll get back to you." {
		t.Fatalf("expected first AI reply to be prefixed, got %q", updated.timeline.quickReplies[0])
	}
	if updated.timeline.quickReplies[1] != "[AI] Already prefixed" {
		t.Fatalf("expected existing prefix to be preserved, got %q", updated.timeline.quickReplies[1])
	}
}

func TestRenderEmailPreview_HidesStaleBodyWhenSelectedEmailChanges(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.selectedEmail = &models.EmailData{
		MessageID: "msg-new",
		Sender:    "new@example.com",
		Subject:   "New subject",
	}
	m.timeline.body = &models.EmailBody{TextPlain: "stale body should not render"}
	m.timeline.bodyMessageID = "msg-old"
	m.timeline.bodyLoading = false
	m.timeline.previewWidth = 60
	m.timelineTable.SetHeight(10)

	rendered := stripANSI(m.renderEmailPreview())
	if !strings.Contains(rendered, "Loading") {
		t.Fatalf("expected stale body association to render loading state, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "stale body should not render") {
		t.Fatalf("expected stale body text to stay hidden, got:\n%s", rendered)
	}
}

func TestLoadEmailBodyCmd_UIDZeroShowsUnavailablePlaceholder(t *testing.T) {
	backend := &stubBackend{}
	m := New(backend, nil, "", nil, false)

	msg := m.loadEmailBodyCmd("msg-legacy", "INBOX", 0)().(EmailBodyMsg)
	if msg.MessageID != "msg-legacy" {
		t.Fatalf("expected message id msg-legacy, got %q", msg.MessageID)
	}
	if msg.Body == nil || !strings.Contains(msg.Body.TextPlain, "Body unavailable") {
		t.Fatalf("expected unavailable placeholder for UID 0 body, got %#v", msg.Body)
	}
	if backend.fetchBodyCalls != 0 {
		t.Fatalf("expected no IMAP fetch for UID 0, got %d calls", backend.fetchBodyCalls)
	}
}
