package imap

import "testing"

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
