package app

import (
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
)

// newDraftTestModel creates a minimal Model suitable for draft auto-save tests.
func newDraftTestModel() *Model {
	to := textinput.New()
	subject := textinput.New()
	body := textarea.New()
	return &Model{
		backend:         &stubBackend{},
		timeline:        TimelineState{expandedThreads: make(map[string]bool)},
		classifications: make(map[string]string),
		composeTo:       to,
		composeSubject:  subject,
		composeBody:     body,
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

// TestDraftSaveTickDeletesOldDraftFirst verifies that when lastDraftUID is set,
// the tick clears it before saving a new draft.
func TestDraftSaveTickDeletesOldDraftFirst(t *testing.T) {
	m := newDraftTestModel()
	m.activeTab = tabCompose
	m.composeTo.SetValue("to@test.com")
	m.lastDraftUID = 10
	m.lastDraftFolder = "Drafts"

	updatedModel, _ := m.Update(DraftSaveTickMsg{})
	um := updatedModel.(*Model)

	// After the tick the UID should be cleared (delete was issued) and draftSaving true
	if um.lastDraftUID != 0 {
		t.Errorf("expected lastDraftUID to be cleared, got %d", um.lastDraftUID)
	}
	if um.lastDraftFolder != "" {
		t.Errorf("expected lastDraftFolder to be cleared, got %q", um.lastDraftFolder)
	}
	if !um.draftSaving {
		t.Error("expected draftSaving=true after tick with prior draft UID")
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
