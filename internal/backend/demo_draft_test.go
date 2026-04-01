package backend

import (
	"testing"
)

func TestDemoDraftRoundtrip(t *testing.T) {
	b := NewDemoBackend()

	uid, folder, err := b.SaveDraft("to@example.com", "Test Subject", "Hello body")
	if err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	if uid == 0 {
		t.Error("expected non-zero UID")
	}
	if folder != "Drafts" {
		t.Errorf("expected folder Drafts, got %s", folder)
	}

	drafts, err := b.ListDrafts()
	if err != nil {
		t.Fatalf("ListDrafts: %v", err)
	}
	if len(drafts) != 1 {
		t.Fatalf("expected 1 draft, got %d", len(drafts))
	}
	if drafts[0].Subject != "Test Subject" {
		t.Errorf("expected subject 'Test Subject', got %q", drafts[0].Subject)
	}
	if drafts[0].To != "to@example.com" {
		t.Errorf("expected to 'to@example.com', got %q", drafts[0].To)
	}

	if err := b.DeleteDraft(uid, folder); err != nil {
		t.Fatalf("DeleteDraft: %v", err)
	}

	drafts, _ = b.ListDrafts()
	if len(drafts) != 0 {
		t.Errorf("expected 0 drafts after delete, got %d", len(drafts))
	}
}

func TestDemoDraftDelete_NotFoundIsNoError(t *testing.T) {
	b := NewDemoBackend()
	if err := b.DeleteDraft(99, "Drafts"); err != nil {
		t.Errorf("DeleteDraft for nonexistent UID should not error: %v", err)
	}
}

func TestDemoDraftUID_AutoIncrement(t *testing.T) {
	b := NewDemoBackend()
	uid1, _, _ := b.SaveDraft("a@b.com", "S1", "B1")
	uid2, _, _ := b.SaveDraft("a@b.com", "S2", "B2")
	if uid2 <= uid1 {
		t.Errorf("expected uid2 > uid1, got %d <= %d", uid2, uid1)
	}
}
