package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/models"
)

// makeCleanupPreviewModel builds a minimal Model with the cleanup preview open.
func makeCleanupPreviewModel() *Model {
	email := &models.EmailData{
		MessageID: "test-msg-1",
		Subject:   "Test Subject",
		Sender:    "sender@example.com",
		Folder:    "INBOX",
		Date:      time.Now(),
	}
	m := &Model{
		activeTab:           tabCleanup,
		showCleanupPreview:  true,
		cleanupPreviewEmail: email,
		cleanupEmailBody:    &models.EmailBody{TextPlain: "Hello world"},
		detailsEmails: []*models.EmailData{
			email,
			{MessageID: "test-msg-2", Subject: "Other", Sender: "other@example.com", Folder: "INBOX", Date: time.Now()},
		},
		deletionRequestCh: make(chan models.DeletionRequest, 10),
		deletionResultCh:  make(chan models.DeletionResult, 10),
		timeline:          TimelineState{expandedThreads: make(map[string]bool)},
		backend:           &stubBackend{},
		stats:             map[string]*models.SenderStats{},
	}
	return m
}

// TestCleanupFullScreen verifies that pressing z toggles cleanupFullScreen.
func TestCleanupFullScreen(t *testing.T) {
	m := makeCleanupPreviewModel()

	if m.cleanupFullScreen {
		t.Fatal("expected cleanupFullScreen=false initially")
	}

	// Press z — should enter full-screen
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	updated := newM.(*Model)

	if !updated.cleanupFullScreen {
		t.Error("expected cleanupFullScreen=true after pressing z")
	}
	if !updated.showCleanupPreview {
		t.Error("expected showCleanupPreview still true after pressing z")
	}

	// Press z again — should exit full-screen
	newM2, _ := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	updated2 := newM2.(*Model)

	if updated2.cleanupFullScreen {
		t.Error("expected cleanupFullScreen=false after pressing z again")
	}
	if !updated2.showCleanupPreview {
		t.Error("expected showCleanupPreview still true after toggling full-screen off")
	}
}

// TestCleanupEscInFullScreen verifies that Esc in full-screen exits full-screen but keeps preview open.
func TestCleanupEscInFullScreen(t *testing.T) {
	m := makeCleanupPreviewModel()
	m.cleanupFullScreen = true

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := newM.(*Model)

	if updated.cleanupFullScreen {
		t.Error("expected cleanupFullScreen=false after Esc in full-screen")
	}
	if !updated.showCleanupPreview {
		t.Error("expected showCleanupPreview=true — Esc in full-screen should keep preview open")
	}
}

// TestCleanupEscClosesPreview verifies that Esc (not full-screen) closes the preview.
func TestCleanupEscClosesPreview(t *testing.T) {
	m := makeCleanupPreviewModel()
	m.cleanupFullScreen = false

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := newM.(*Model)

	if updated.showCleanupPreview {
		t.Error("expected showCleanupPreview=false after Esc when not in full-screen")
	}
	if updated.cleanupPreviewEmail != nil {
		t.Error("expected cleanupPreviewEmail=nil after closing preview")
	}
}

func TestCleanupEmailBodyMsgDropsStaleResultForDifferentPreviewEmail(t *testing.T) {
	m := makeCleanupPreviewModel()
	current := m.detailsEmails[1]
	m.cleanupPreviewEmail = current
	m.cleanupBodyLoading = true
	m.cleanupEmailBody = &models.EmailBody{TextPlain: "body for current preview"}
	m.cleanupPreviewDocLayout = &previewDocumentLayout{TotalRows: 1}
	m.cleanupPreviewDocWidth = 44
	m.cleanupPreviewDocRows = 9
	m.cleanupPreviewDocMode = previewImageModeLinks
	m.cleanupPreviewDocMessageID = current.MessageID

	newM, _ := m.Update(CleanupEmailBodyMsg{
		MessageID: "test-msg-1",
		Body:      &models.EmailBody{TextPlain: "stale body from previous preview"},
	})
	updated := newM.(*Model)

	if updated.cleanupBodyLoading != true {
		t.Fatalf("stale result should leave current loading state alone")
	}
	if got := updated.cleanupEmailBody.TextPlain; got != "body for current preview" {
		t.Fatalf("stale result overwrote current body: %q", got)
	}
	if updated.cleanupPreviewDocLayout == nil ||
		updated.cleanupPreviewDocWidth != 44 ||
		updated.cleanupPreviewDocRows != 9 ||
		updated.cleanupPreviewDocMode != previewImageModeLinks ||
		updated.cleanupPreviewDocMessageID != current.MessageID {
		t.Fatalf("stale result should not clear current cleanup document cache")
	}

	newM, _ = updated.Update(CleanupEmailBodyMsg{
		MessageID: current.MessageID,
		Body:      &models.EmailBody{TextPlain: "accepted body for current preview"},
	})
	accepted := newM.(*Model)
	if accepted.cleanupBodyLoading {
		t.Fatalf("matching result should finish loading")
	}
	if got := accepted.cleanupEmailBody.TextPlain; got != "accepted body for current preview" {
		t.Fatalf("matching result was not accepted: %q", got)
	}
	if accepted.cleanupPreviewDocLayout != nil {
		t.Fatalf("matching result should clear stale document layout cache")
	}
}

func TestCleanupFullScreenUsesTerminalHeightBudget(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	defer m.cleanup()
	m.activeTab = tabCleanup
	m.focusedPanel = panelDetails
	m.showCleanupPreview = true
	m.cleanupFullScreen = true
	m.cleanupPreviewWidth = 80
	m.cleanupPreviewEmail = &models.EmailData{
		MessageID: "tall-cleanup",
		Subject:   "Tall Cleanup",
		Sender:    "sender@example.com",
		Folder:    "INBOX",
		Date:      time.Now(),
	}
	m.detailsTable.SetHeight(5)

	var html strings.Builder
	for i := 1; i <= 20; i++ {
		html.WriteString("Line ")
		html.WriteString(string(rune('A' + i - 1)))
		html.WriteString("<br>")
	}
	m.cleanupEmailBody = &models.EmailBody{TextHTML: html.String()}

	rendered := m.renderCleanupPreview()
	assertFitsHeight(t, 24, rendered)
	lines := strings.Split(stripANSI(rendered), "\n")
	if len(lines) != 24 {
		t.Fatalf("expected cleanup full-screen preview to fill terminal height, got %d lines:\n%s", len(lines), stripANSI(rendered))
	}
	if !strings.Contains(stripANSI(rendered), "Line L") {
		t.Fatalf("expected full-screen document budget to show rows beyond split preview height, got:\n%s", stripANSI(rendered))
	}
}

func TestCleanupFullScreenScrollIgnoresHiddenFocus(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	defer m.cleanup()
	m.activeTab = tabCleanup
	m.focusedPanel = panelSummary
	m.showCleanupPreview = true
	m.cleanupFullScreen = true
	m.cleanupPreviewWidth = 80
	m.cleanupPreviewEmail = &models.EmailData{
		MessageID: "cleanup-scroll",
		Subject:   "Cleanup Scroll",
		Sender:    "sender@example.com",
		Folder:    "INBOX",
		Date:      time.Now(),
	}
	m.cleanupEmailBody = &models.EmailBody{TextPlain: strings.Repeat("line\n", 40)}

	model, _ := m.handleKeyMsg(keyRunes("j"))
	updated := model.(*Model)
	if updated.cleanupBodyScrollOffset != 1 {
		t.Fatalf("cleanup full-screen j should scroll preview regardless of hidden focus, got offset=%d", updated.cleanupBodyScrollOffset)
	}

	model, _ = updated.handleKeyMsg(keyRunes("k"))
	updated = model.(*Model)
	if updated.cleanupBodyScrollOffset != 0 {
		t.Fatalf("cleanup full-screen k should scroll preview regardless of hidden focus, got offset=%d", updated.cleanupBodyScrollOffset)
	}
}

// TestCleanupPreviewDelete verifies that pressing D in cleanup preview sends a deletion request
// and that, on receiving a DeletionResult for the previewed message, the preview closes and
// the email is removed from detailsEmails.
func TestCleanupPreviewDelete(t *testing.T) {
	m := makeCleanupPreviewModel()

	// Press D
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("D")})
	updated := newM.(*Model)

	// The DeletionRequest is sent in a goroutine; wait briefly for it.
	var req models.DeletionRequest
	select {
	case req = <-updated.deletionRequestCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for DeletionRequest on channel")
	}
	if req.MessageID != "test-msg-1" {
		t.Errorf("expected MessageID=test-msg-1, got %q", req.MessageID)
	}
	if req.IsArchive {
		t.Error("expected IsArchive=false for D key")
	}

	if !updated.cleanupPreviewDeleting {
		t.Error("expected cleanupPreviewDeleting=true after pressing D")
	}
	if updated.cleanupPreviewIsArchive {
		t.Error("expected cleanupPreviewIsArchive=false for D key")
	}

	// Simulate receiving a successful DeletionResult for the previewed email
	result := models.DeletionResult{
		MessageID:    "test-msg-1",
		Folder:       "INBOX",
		DeletedCount: 1,
	}
	newM2, _ := updated.Update(result)
	updated2 := newM2.(*Model)

	if updated2.showCleanupPreview {
		t.Error("expected showCleanupPreview=false after successful deletion result")
	}
	if updated2.cleanupPreviewEmail != nil {
		t.Error("expected cleanupPreviewEmail=nil after deletion")
	}
	// Deleted email should be removed from detailsEmails
	for _, e := range updated2.detailsEmails {
		if e.MessageID == "test-msg-1" {
			t.Error("expected test-msg-1 to be removed from detailsEmails")
		}
	}
	if len(updated2.detailsEmails) != 1 {
		t.Errorf("expected 1 remaining email in detailsEmails, got %d", len(updated2.detailsEmails))
	}
	if updated2.statusMessage != "Deleted" {
		t.Errorf("expected statusMessage=%q, got %q", "Deleted", updated2.statusMessage)
	}
}

// TestCleanupPreviewArchive verifies that pressing e in cleanup preview sends an archive request
// and that the result closes the preview with an "Archived" toast.
func TestCleanupPreviewArchive(t *testing.T) {
	m := makeCleanupPreviewModel()

	// Press e
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	updated := newM.(*Model)

	// The DeletionRequest (with IsArchive=true) is sent in a goroutine; wait briefly.
	var req models.DeletionRequest
	select {
	case req = <-updated.deletionRequestCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for DeletionRequest on channel")
	}
	if req.MessageID != "test-msg-1" {
		t.Errorf("expected MessageID=test-msg-1, got %q", req.MessageID)
	}
	if !req.IsArchive {
		t.Error("expected IsArchive=true for e key")
	}

	if !updated.cleanupPreviewDeleting {
		t.Error("expected cleanupPreviewDeleting=true after pressing e")
	}
	if !updated.cleanupPreviewIsArchive {
		t.Error("expected cleanupPreviewIsArchive=true for e key")
	}

	// Simulate receiving a successful DeletionResult
	result := models.DeletionResult{
		MessageID:    "test-msg-1",
		Folder:       "INBOX",
		DeletedCount: 1,
	}
	newM2, _ := updated.Update(result)
	updated2 := newM2.(*Model)

	if updated2.showCleanupPreview {
		t.Error("expected showCleanupPreview=false after archive result")
	}
	if updated2.statusMessage != "Archived" {
		t.Errorf("expected statusMessage=%q, got %q", "Archived", updated2.statusMessage)
	}
	for _, e := range updated2.detailsEmails {
		if e.MessageID == "test-msg-1" {
			t.Error("expected test-msg-1 to be removed from detailsEmails after archive")
		}
	}
}
