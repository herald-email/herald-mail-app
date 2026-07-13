package app

import (
	"reflect"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func makeDeletionPruneEmail(id, sender, subject string, offsetMinutes int) *models.EmailData {
	return &models.EmailData{
		MessageID: id,
		Sender:    sender,
		Subject:   subject,
		Folder:    "INBOX",
		Date:      time.Date(2026, 4, 30, 12, offsetMinutes, 0, 0, time.UTC),
	}
}

func requireMessageAbsent(t *testing.T, emails []*models.EmailData, messageID string) {
	t.Helper()
	for _, email := range emails {
		if email != nil && email.MessageID == messageID {
			t.Fatalf("expected %s to be absent, got %#v", messageID, emails)
		}
	}
}

func requireMessagePresent(t *testing.T, emails []*models.EmailData, messageID string) {
	t.Helper()
	for _, email := range emails {
		if email != nil && email.MessageID == messageID {
			return
		}
	}
	t.Fatalf("expected %s to remain present, got %#v", messageID, emails)
}

func TestDeletionResultPrunesTimelineMessageStateImmediately(t *testing.T) {
	deleted := makeDeletionPruneEmail("delete-me", "sender@example.com", "Delete me", 1)
	keep := makeDeletionPruneEmail("keep-me", "friend@example.com", "Keep me", 0)

	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.deleting = true
	m.deletionsPending = 1
	m.deletionsTotal = 1
	m.timeline.emails = []*models.EmailData{deleted, keep}
	m.timeline.searchMode = true
	m.timeline.searchResults = []*models.EmailData{deleted, keep}
	m.timeline.chatFilteredEmails = []*models.EmailData{deleted, keep}
	m.timeline.selectedMessageIDs = map[string]bool{
		deleted.MessageID: true,
		keep.MessageID:    true,
	}
	m.timeline.selectedEmail = deleted
	m.timeline.body = &models.EmailBody{TextPlain: "stale body"}
	m.timeline.bodyMessageID = deleted.MessageID
	m.timeline.bodyLoading = true
	m.updateTimelineTable()

	updatedModel, _ := m.Update(models.DeletionResult{
		MessageID:    deleted.MessageID,
		Folder:       "INBOX",
		DeletedCount: 1,
	})
	updated := updatedModel.(*Model)

	requireMessageAbsent(t, updated.timeline.emails, deleted.MessageID)
	requireMessageAbsent(t, updated.timeline.searchResults, deleted.MessageID)
	requireMessageAbsent(t, updated.timeline.chatFilteredEmails, deleted.MessageID)
	requireMessagePresent(t, updated.timeline.emails, keep.MessageID)
	if updated.timeline.selectedMessageIDs[deleted.MessageID] {
		t.Fatalf("expected deleted message selection to be cleared")
	}
	if updated.timeline.selectedEmail != nil {
		t.Fatalf("expected stale preview to close, got %#v", updated.timeline.selectedEmail)
	}
	if updated.timeline.body != nil || updated.timeline.bodyMessageID != "" || updated.timeline.bodyLoading {
		t.Fatalf("expected stale preview body state to clear")
	}
}

func TestDeletionResultPrunesTimelineBatchAffectedIDsForArchive(t *testing.T) {
	first := makeDeletionPruneEmail("batch-1", "news@example.com", "Newsletter one", 2)
	second := makeDeletionPruneEmail("batch-2", "news@example.com", "Newsletter two", 1)
	keep := makeDeletionPruneEmail("keep-me", "friend@example.com", "Keep me", 0)

	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.deleting = true
	m.deletionsPending = 1
	m.deletionsTotal = 1
	m.timeline.emails = []*models.EmailData{first, second, keep}
	m.timeline.searchMode = true
	m.timeline.searchResults = []*models.EmailData{first, keep}
	m.timeline.chatFilteredEmails = []*models.EmailData{second, keep}
	m.timeline.selectedMessageIDs = map[string]bool{
		first.MessageID:  true,
		second.MessageID: true,
		keep.MessageID:   true,
	}
	m.timeline.selectedEmail = second
	m.timeline.body = &models.EmailBody{TextPlain: "stale archived body"}
	m.timeline.bodyMessageID = second.MessageID
	m.updateTimelineTable()

	updatedModel, _ := m.Update(models.DeletionResult{
		Sender:             "news@example.com",
		Folder:             "INBOX",
		DeletedCount:       2,
		IsArchive:          true,
		AffectedMessageIDs: []string{first.MessageID, second.MessageID},
	})
	updated := updatedModel.(*Model)

	for _, id := range []string{first.MessageID, second.MessageID} {
		requireMessageAbsent(t, updated.timeline.emails, id)
		requireMessageAbsent(t, updated.timeline.searchResults, id)
		requireMessageAbsent(t, updated.timeline.chatFilteredEmails, id)
		if updated.timeline.selectedMessageIDs[id] {
			t.Fatalf("expected %s selection to be cleared", id)
		}
	}
	requireMessagePresent(t, updated.timeline.emails, keep.MessageID)
	if updated.timeline.selectedEmail != nil || updated.timeline.body != nil || updated.timeline.bodyMessageID != "" {
		t.Fatalf("expected archived preview state to clear")
	}
}

func TestDeletionResultBatchAdvancesMessageCountProgress(t *testing.T) {
	first := makeDeletionPruneEmail("batch-progress-1", "news@example.com", "Newsletter one", 2)
	second := makeDeletionPruneEmail("batch-progress-2", "news@example.com", "Newsletter two", 1)

	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.deleting = true
	m.deletionsPending = 2
	m.deletionsTotal = 2
	m.timeline.emails = []*models.EmailData{first, second}
	m.timeline.selectedMessageIDs = map[string]bool{
		first.MessageID:  true,
		second.MessageID: true,
	}
	m.updateTimelineTable()

	updatedModel, _ := m.Update(models.DeletionResult{
		Folder:             "INBOX",
		DeletedCount:       2,
		AffectedMessageIDs: []string{first.MessageID, second.MessageID},
	})
	updated := updatedModel.(*Model)

	if updated.deleting {
		t.Fatal("expected batch result to finish deletion")
	}
	if updated.deletionsPending != 0 || updated.deletionsTotal != 0 {
		t.Fatalf("expected completed batch progress to reset, got pending=%d total=%d", updated.deletionsPending, updated.deletionsTotal)
	}
	for _, id := range []string{first.MessageID, second.MessageID} {
		requireMessageAbsent(t, updated.timeline.emails, id)
		if updated.timeline.selectedMessageIDs[id] {
			t.Fatalf("expected %s selection to be cleared", id)
		}
	}
}

type folderStatusRecordingBackend struct {
	layoutBackend
	requestedFolders [][]string
	status           map[string]models.FolderStatus
	snapshots        []backend.AccountFolderSnapshot
}

func (b *folderStatusRecordingBackend) GetFolderStatus(folders []string) (map[string]models.FolderStatus, error) {
	b.requestedFolders = append(b.requestedFolders, append([]string(nil), folders...))
	result := make(map[string]models.FolderStatus, len(folders))
	for _, folder := range folders {
		if st, ok := b.status[folder]; ok {
			result[folder] = st
		}
	}
	return result, nil
}

func (b *folderStatusRecordingBackend) ListAccountFolderSnapshots() ([]backend.AccountFolderSnapshot, error) {
	return b.snapshots, nil
}

func TestDeletionResultCompletionRefreshesAllVisibleSidebarFolderCounts(t *testing.T) {
	b := &folderStatusRecordingBackend{
		status: map[string]models.FolderStatus{
			"INBOX":   {Unseen: 10, Total: 20},
			"Archive": {Unseen: 1, Total: 7},
		},
	}
	b.snapshots = []backend.AccountFolderSnapshot{
		{
			Account: backend.AccountInfo{
				SourceID:  models.DefaultMailSourceID,
				AccountID: models.DefaultAccountID,
			},
			Folders: []string{"INBOX", "Archive"},
			Status:  b.status,
		},
	}
	m := New(b, nil, "", nil, false)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = model.(*Model)
	m.loading = false
	m.activeTab = tabTimeline
	m.currentFolder = "INBOX"
	m.folders = []string{"INBOX"}
	m.folderTree = buildFolderTree([]string{"INBOX", "Archive"})
	m.folderStatus = map[string]models.FolderStatus{
		"INBOX":   {Unseen: 12, Total: 38},
		"Archive": {Unseen: 2, Total: 9},
	}
	m.accountFolderSnapshots = []backend.AccountFolderSnapshot{
		{
			Account: backend.AccountInfo{
				SourceID:  models.DefaultMailSourceID,
				AccountID: models.DefaultAccountID,
			},
			Folders: []string{"INBOX", "Archive"},
			Status: map[string]models.FolderStatus{
				"INBOX":   {Unseen: 12, Total: 38},
				"Archive": {Unseen: 2, Total: 9},
			},
		},
	}
	m.deleting = true
	m.deletionsPending = 2
	m.deletionsTotal = 2

	updatedModel, cmd := m.Update(models.DeletionResult{
		Folder:             "INBOX",
		DeletedCount:       2,
		IsArchive:          true,
		AffectedMessageIDs: []string{"archived-1", "archived-2"},
	})
	updated := updatedModel.(*Model)

	for _, msg := range collectCommandMessagesForTest(cmd) {
		if folderMsg, ok := msg.(FolderStatusMsg); ok {
			updatedModel, _ = updated.Update(folderMsg)
			updated = updatedModel.(*Model)
		}
	}

	wantFolders := []string{"INBOX", "Archive"}
	if len(b.requestedFolders) != 1 {
		t.Fatalf("GetFolderStatus calls = %#v, want one call for %#v", b.requestedFolders, wantFolders)
	}
	if !reflect.DeepEqual(b.requestedFolders[0], wantFolders) {
		t.Fatalf("GetFolderStatus folders = %#v, want %#v", b.requestedFolders[0], wantFolders)
	}
	if got := updated.folderStatus["Archive"]; got != b.status["Archive"] {
		t.Fatalf("Archive folder status = %#v, want %#v", got, b.status["Archive"])
	}
	if got := updated.accountFolderSnapshots[0].Status["Archive"]; got != b.status["Archive"] {
		t.Fatalf("Archive account snapshot status = %#v, want %#v", got, b.status["Archive"])
	}
}
