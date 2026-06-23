package imap

import (
	"testing"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestArchiveFoldersForVendor_DefaultExcludesAllMail(t *testing.T) {
	folders := archiveFoldersForVendor("")
	for _, folder := range folders {
		if folder == "All Mail" || folder == "[Gmail]/All Mail" {
			t.Fatalf("default archive targets must not include All Mail, got %v", folders)
		}
	}
}

func TestArchiveFoldersForVendor_ProtonmailExcludesAllMail(t *testing.T) {
	folders := archiveFoldersForVendor("protonmail")
	for _, folder := range folders {
		if folder == "All Mail" || folder == "[Gmail]/All Mail" {
			t.Fatalf("protonmail archive targets must not include All Mail, got %v", folders)
		}
	}
}

func TestArchiveFoldersForVendor_GmailUsesProviderSpecificAllMail(t *testing.T) {
	folders := archiveFoldersForVendor("gmail")
	found := false
	for _, folder := range folders {
		if folder == "[Gmail]/All Mail" {
			found = true
		}
		if folder == "All Mail" {
			t.Fatalf("gmail archive targets should use the Gmail-specific mailbox, got %v", folders)
		}
	}
	if !found {
		t.Fatalf("gmail archive targets should include [Gmail]/All Mail, got %v", folders)
	}
}

func TestSplitFreshUIDRefsRequiresCurrentUIDValidity(t *testing.T) {
	current := models.MessageRef{Folder: "INBOX", MessageID: "current", UID: 101, UIDValidity: 999}.WithDefaults()
	missingUID := models.MessageRef{Folder: "INBOX", MessageID: "missing", UIDValidity: 999}.WithDefaults()
	missingValidity := models.MessageRef{Folder: "INBOX", MessageID: "missing-validity", UID: 102}.WithDefaults()
	staleValidity := models.MessageRef{Folder: "INBOX", MessageID: "stale", UID: 103, UIDValidity: 998}.WithDefaults()

	fresh, fallback := splitFreshUIDRefs([]models.MessageRef{
		current,
		missingUID,
		missingValidity,
		staleValidity,
	}, 999)

	if len(fresh) != 1 || fresh[0] != current {
		t.Fatalf("fresh refs = %#v, want only %#v", fresh, current)
	}
	if len(fallback) != 3 {
		t.Fatalf("fallback refs = %#v, want 3 stale/missing refs", fallback)
	}
	for _, want := range []models.MessageRef{missingUID, missingValidity, staleValidity} {
		found := false
		for _, got := range fallback {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("fallback refs = %#v, missing %#v", fallback, want)
		}
	}
}
