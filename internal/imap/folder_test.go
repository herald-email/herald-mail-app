//go:build integration

package imap

import (
	"testing"

	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/testutil"
)

// TestCreateMailbox verifies that CreateMailbox creates a new mailbox
// visible in the server's folder list.
func TestCreateMailbox(t *testing.T) {
	_, cfg, stop := testutil.StartMockIMAPServer(t)
	defer stop()

	progressCh := make(chan models.ProgressInfo, 10)
	c := New(cfg, "", newIntegrationCache(t), progressCh)

	if err := c.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	if err := c.CreateMailbox("TestCreate"); err != nil {
		t.Fatalf("CreateMailbox: %v", err)
	}

	folders, err := c.ListFolders()
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	found := false
	for _, f := range folders {
		if f == "TestCreate" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("created folder 'TestCreate' not found in folder list: %v", folders)
	}
}

// TestRenameMailbox verifies that RenameMailbox renames an existing mailbox.
func TestRenameMailbox(t *testing.T) {
	_, cfg, stop := testutil.StartMockIMAPServer(t)
	defer stop()

	progressCh := make(chan models.ProgressInfo, 10)
	c := New(cfg, "", newIntegrationCache(t), progressCh)

	if err := c.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	if err := c.CreateMailbox("BeforeRename"); err != nil {
		t.Fatalf("CreateMailbox: %v", err)
	}
	if err := c.RenameMailbox("BeforeRename", "AfterRename"); err != nil {
		t.Fatalf("RenameMailbox: %v", err)
	}

	folders, err := c.ListFolders()
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	foundOld, foundNew := false, false
	for _, f := range folders {
		if f == "BeforeRename" {
			foundOld = true
		}
		if f == "AfterRename" {
			foundNew = true
		}
	}
	if foundOld {
		t.Error("old folder 'BeforeRename' still exists after rename")
	}
	if !foundNew {
		t.Errorf("renamed folder 'AfterRename' not found in folder list: %v", folders)
	}
}

// TestDeleteMailbox verifies that DeleteMailbox removes an existing mailbox.
func TestDeleteMailbox(t *testing.T) {
	_, cfg, stop := testutil.StartMockIMAPServer(t)
	defer stop()

	progressCh := make(chan models.ProgressInfo, 10)
	c := New(cfg, "", newIntegrationCache(t), progressCh)

	if err := c.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	if err := c.CreateMailbox("ToDelete"); err != nil {
		t.Fatalf("CreateMailbox: %v", err)
	}
	if err := c.DeleteMailbox("ToDelete"); err != nil {
		t.Fatalf("DeleteMailbox: %v", err)
	}

	folders, err := c.ListFolders()
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	for _, f := range folders {
		if f == "ToDelete" {
			t.Errorf("deleted folder 'ToDelete' still present in folder list: %v", folders)
		}
	}
}

// TestCreateMailbox_NotConnected verifies that CreateMailbox returns an error
// when the client is not connected.
func TestCreateMailbox_NotConnected(t *testing.T) {
	_, cfg, stop := testutil.StartMockIMAPServer(t)
	defer stop()

	progressCh := make(chan models.ProgressInfo, 10)
	c := New(cfg, "", newIntegrationCache(t), progressCh)
	// Do NOT call Connect()

	err := c.CreateMailbox("ShouldFail")
	if err == nil {
		t.Fatal("expected error when not connected, got nil")
	}
}
