package app

import (
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// newDraftTestModel creates a minimal Model suitable for draft auto-save tests.
func newDraftTestModel() *Model {
	to := textinput.New()
	subject := textinput.New()
	body := textarea.New()
	aiResponse := textarea.New()
	return &Model{
		backend:           &stubBackend{},
		timeline:          TimelineState{expandedThreads: make(map[string]bool)},
		classifications:   make(map[string]string),
		composeTo:         to,
		composeSubject:    subject,
		composeBody:       body,
		composeAIResponse: aiResponse,
	}
}

// TestDraftSaveTickTriggersWhenComposing verifies that DraftSaveTickMsg triggers
// saveDraftCmd when on the compose tab with content and draftSaving=false.
func TestDraftSaveTickTriggersWhenComposing(t *testing.T) {
	m := newDraftTestModel()
	m.activeTab = tabCompose
	m.composeTo.SetValue("to@test.com")
	m.composeSubject.SetValue("Hello")
	m.composeBody.SetValue("World")

	updatedModel, cmd := m.Update(DraftSaveTickMsg{})
	um := updatedModel.(*Model)

	if !um.draftSaving {
		t.Error("expected draftSaving=true after tick with content on compose tab")
	}
	if cmd == nil {
		t.Error("expected non-nil cmd after tick with content")
	}
}

// TestDraftSavedMsgUpdatesUID verifies DraftSavedMsg updates lastDraftUID and
// clears draftSaving.
func TestDraftSavedMsgUpdatesUID(t *testing.T) {
	m := newDraftTestModel()
	m.draftSaving = true

	updatedModel, _ := m.Update(DraftSavedMsg{UID: 42, Folder: "Drafts"})
	um := updatedModel.(*Model)

	if um.draftSaving {
		t.Error("expected draftSaving=false after DraftSavedMsg")
	}
	if um.lastDraftUID != 42 {
		t.Errorf("expected lastDraftUID=42, got %d", um.lastDraftUID)
	}
	if um.lastDraftFolder != "Drafts" {
		t.Errorf("expected lastDraftFolder=Drafts, got %q", um.lastDraftFolder)
	}
	if um.statusMessage != "Draft saved" {
		t.Errorf("expected statusMessage='Draft saved', got %q", um.statusMessage)
	}
}

// TestDraftSaveTickNoSaveWhenEmpty verifies no save is triggered when compose
// fields are all empty.
func TestDraftSaveTickNoSaveWhenEmpty(t *testing.T) {
	m := newDraftTestModel()
	m.activeTab = tabCompose
	// composeTo, composeSubject, composeBody all empty (default)

	updatedModel, _ := m.Update(DraftSaveTickMsg{})
	um := updatedModel.(*Model)

	if um.draftSaving {
		t.Error("expected draftSaving=false when compose empty")
	}
}

// TestDraftSaveTickNoSaveWhenNotOnComposeTab verifies no save is triggered
// when the user is not on the compose tab.
func TestDraftSaveTickNoSaveWhenNotOnComposeTab(t *testing.T) {
	m := newDraftTestModel()
	m.activeTab = tabTimeline
	m.composeTo.SetValue("someone@example.com")
	m.composeSubject.SetValue("Draft subject")

	updatedModel, _ := m.Update(DraftSaveTickMsg{})
	um := updatedModel.(*Model)

	if um.draftSaving {
		t.Error("expected draftSaving=false when not on compose tab")
	}
}

// TestDraftSaveTickNoSaveWhenAlreadySaving verifies concurrent saves are
// prevented when draftSaving=true.
func TestDraftSaveTickNoSaveWhenAlreadySaving(t *testing.T) {
	m := newDraftTestModel()
	m.activeTab = tabCompose
	m.composeTo.SetValue("to@test.com")
	m.draftSaving = true // already in flight

	updatedModel, _ := m.Update(DraftSaveTickMsg{})
	um := updatedModel.(*Model)

	// draftSaving should still be true (no new save started)
	if !um.draftSaving {
		t.Error("expected draftSaving to remain true when already saving")
	}
}

// TestDraftSaveTickKeepsOldDraftUntilReplacementSaves verifies that when
// lastDraftUID is set, autosave does not discard the previous draft before the
// replacement save succeeds.
func TestDraftSaveTickKeepsOldDraftUntilReplacementSaves(t *testing.T) {
	m := newDraftTestModel()
	m.activeTab = tabCompose
	m.composeTo.SetValue("to@test.com")
	m.lastDraftUID = 10
	m.lastDraftFolder = "Drafts"

	updatedModel, _ := m.Update(DraftSaveTickMsg{})
	um := updatedModel.(*Model)

	if um.lastDraftUID != 10 {
		t.Errorf("expected lastDraftUID to stay 10 until replacement saves, got %d", um.lastDraftUID)
	}
	if um.lastDraftFolder != "Drafts" {
		t.Errorf("expected lastDraftFolder to stay Drafts, got %q", um.lastDraftFolder)
	}
	if !um.draftSaving {
		t.Error("expected draftSaving=true after tick with prior draft UID")
	}
}

func TestDraftSavedMsgDeletesPreviousDraftAfterSuccessfulReplacement(t *testing.T) {
	backend := &draftDeleteRecordingBackend{}
	m := newDraftTestModel()
	m.backend = backend
	m.draftSaving = true
	m.lastDraftUID = 10
	m.lastDraftFolder = "Drafts"

	updatedModel, cmd := m.Update(DraftSavedMsg{
		UID:           11,
		Folder:        "Drafts",
		ReplaceUID:    10,
		ReplaceFolder: "Drafts",
	})
	um := updatedModel.(*Model)

	if um.lastDraftUID != 11 || um.lastDraftFolder != "Drafts" {
		t.Fatalf("expected current draft to become 11/Drafts, got %d/%q", um.lastDraftUID, um.lastDraftFolder)
	}
	if cmd == nil {
		t.Fatal("expected delete command for replaced draft")
	}
	if _, ok := cmd().(DraftDeletedMsg); !ok {
		t.Fatalf("expected DraftDeletedMsg from delete command")
	}
	if len(backend.deleted) != 1 || backend.deleted[0].uid != 10 || backend.deleted[0].folder != "Drafts" {
		t.Fatalf("expected old draft delete after replacement save, got %#v", backend.deleted)
	}
}

func TestSaveDraftCmdFromGmailInboxGraftDoesNotReplaceThreadVisibleDraft(t *testing.T) {
	backend := &draftDeleteRecordingBackend{saveUID: 8908, saveFolder: "[Gmail]/Drafts"}
	m := newDraftTestModel()
	m.backend = backend
	m.openTimelineDraftCompose(
		&models.EmailData{
			MessageID: "gmail-graft",
			UID:       58133,
			Folder:    "INBOX",
			Subject:   "Re: Staff Software Engineer at Fractional AI",
			IsDraft:   true,
		},
		&models.EmailBody{
			To:        "flavia.iespa@fractional.ai",
			Subject:   "Re: Staff Software Engineer at Fractional AI",
			TextPlain: "Hi Flavia,\n\nThanks for reaching out.",
		},
		"",
	)

	msg := m.saveDraftCmd()().(DraftSavedMsg)
	if msg.Err != nil {
		t.Fatalf("saveDraftCmd returned error: %v", msg.Err)
	}
	if msg.ReplaceUID != 0 || msg.ReplaceFolder != "" {
		t.Fatalf("expected Gmail INBOX graft not to be replacement target, got %d/%q", msg.ReplaceUID, msg.ReplaceFolder)
	}

	updatedModel, cmd := m.Update(msg)
	um := updatedModel.(*Model)
	if cmd != nil {
		t.Fatal("expected no delete command for thread-visible Gmail draft graft")
	}
	if len(backend.deleted) != 0 {
		t.Fatalf("expected no draft deletes, got %#v", backend.deleted)
	}
	if um.lastDraftUID != 8908 || um.lastDraftFolder != "[Gmail]/Drafts" {
		t.Fatalf("expected new canonical draft tracking 8908/[Gmail]/Drafts, got %d/%q", um.lastDraftUID, um.lastDraftFolder)
	}
}

func TestComposeStatusDeletesTrackedDraftAfterSendSuccess(t *testing.T) {
	backend := &draftDeleteRecordingBackend{}
	m := newDraftTestModel()
	m.backend = backend
	m.composeTo.SetValue("to@test.com")
	m.composeSubject.SetValue("Tracked draft")
	m.composeBody.SetValue("Body")
	m.lastDraftUID = 42
	m.lastDraftFolder = "Drafts"
	m.lastDraftReplaceable = true

	updatedModel, cmd := m.Update(ComposeStatusMsg{Message: "Message sent!"})
	um := updatedModel.(*Model)

	if um.lastDraftUID != 0 || um.lastDraftFolder != "" {
		t.Fatalf("expected tracked draft state to clear after send, got %d/%q", um.lastDraftUID, um.lastDraftFolder)
	}
	if cmd == nil {
		t.Fatal("expected send success to delete tracked draft")
	}
	if _, ok := cmd().(DraftDeletedMsg); !ok {
		t.Fatalf("expected DraftDeletedMsg from send-success delete command")
	}
	if len(backend.deleted) != 1 || backend.deleted[0].uid != 42 || backend.deleted[0].folder != "Drafts" {
		t.Fatalf("expected tracked draft delete after send success, got %#v", backend.deleted)
	}
}

func TestComposeStatusDoesNotDeleteThreadVisibleGmailGraftAfterSendSuccess(t *testing.T) {
	backend := &draftDeleteRecordingBackend{}
	m := newDraftTestModel()
	m.backend = backend
	m.composeTo.SetValue("to@test.com")
	m.composeSubject.SetValue("Tracked graft")
	m.composeBody.SetValue("Body")
	m.lastDraftUID = 58133
	m.lastDraftFolder = "INBOX"
	m.lastDraftReplaceable = false

	updatedModel, cmd := m.Update(ComposeStatusMsg{Message: "Message sent!"})
	um := updatedModel.(*Model)

	if um.lastDraftUID != 0 || um.lastDraftFolder != "" || um.lastDraftReplaceable {
		t.Fatalf("expected tracked graft state to clear after send, got %d/%q replaceable=%v", um.lastDraftUID, um.lastDraftFolder, um.lastDraftReplaceable)
	}
	if cmd != nil {
		t.Fatal("expected send success not to delete thread-visible Gmail draft graft")
	}
	if len(backend.deleted) != 0 {
		t.Fatalf("expected no tracked graft delete after send success, got %#v", backend.deleted)
	}
}

// TestDraftSavedMsgErrorSilent verifies that a failed save clears draftSaving
// but does not set a user-visible status message.
func TestDraftSavedMsgErrorSilent(t *testing.T) {
	m := newDraftTestModel()
	m.draftSaving = true

	updatedModel, _ := m.Update(DraftSavedMsg{Err: errDraftSaveStub})
	um := updatedModel.(*Model)

	if um.draftSaving {
		t.Error("expected draftSaving=false even on error")
	}
	if um.statusMessage == "Draft saved" {
		t.Error("expected no 'Draft saved' toast on error")
	}
	if um.lastDraftUID != 0 {
		t.Error("expected lastDraftUID to remain 0 on error")
	}
}

// TestDraftDeletedMsgHandled verifies DraftDeletedMsg is handled without panic.
func TestDraftDeletedMsgHandled(t *testing.T) {
	m := newDraftTestModel()

	// Both success and error cases must not panic and must return the model.
	result1, _ := m.Update(DraftDeletedMsg{})
	if result1 == nil {
		t.Error("expected non-nil model from DraftDeletedMsg")
	}

	result2, _ := m.Update(DraftDeletedMsg{Err: errDraftSaveStub})
	if result2 == nil {
		t.Error("expected non-nil model from DraftDeletedMsg with error")
	}
}

// errDraftSaveStub is a simple sentinel error used in draft tests.
var errDraftSaveStub = &draftStubErr{"draft save failed"}

type draftStubErr struct{ msg string }

func (e *draftStubErr) Error() string { return e.msg }

type draftDeleteRecordingBackend struct {
	stubBackend
	deleted    []recordedDraftDelete
	saveUID    uint32
	saveFolder string
}

type recordedDraftDelete struct {
	uid    uint32
	folder string
}

func (b *draftDeleteRecordingBackend) DeleteDraft(uid uint32, folder string) error {
	b.deleted = append(b.deleted, recordedDraftDelete{uid: uid, folder: folder})
	return nil
}

func (b *draftDeleteRecordingBackend) SaveDraft(_, _, _, _, _ string) (uint32, string, error) {
	return b.saveUID, b.saveFolder, nil
}
