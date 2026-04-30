package app

import (
	"testing"
	"time"

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
	m.activeTab = tabCleanup
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
	m.activeTab = tabCleanup
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

func TestQueueRequestsCleanupSenderCarriesAffectedMessageIDs(t *testing.T) {
	first := makeDeletionPruneEmail("sender-1", "news@example.com", "Newsletter one", 1)
	second := makeDeletionPruneEmail("sender-2", "news@example.com", "Newsletter two", 0)

	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCleanup
	m.currentFolder = "INBOX"
	m.deletionRequestCh = make(chan models.DeletionRequest, 2)
	m.deletionResultCh = make(chan models.DeletionResult, 2)
	m.emailsBySender = map[string][]*models.EmailData{
		"news@example.com": {first, second},
	}
	m.selectedSummaryKeys = map[string]bool{"news@example.com": true}
	m.stats = map[string]*models.SenderStats{
		"news@example.com": {TotalEmails: 2},
	}

	cmd := m.queueRequests(false)
	if cmd == nil {
		t.Fatal("expected deletion listener command")
	}

	req := <-m.deletionRequestCh
	if req.Sender != "news@example.com" {
		t.Fatalf("expected sender batch request, got %#v", req)
	}
	if len(req.AffectedMessageIDs) != 2 {
		t.Fatalf("expected affected message IDs to travel with request, got %#v", req.AffectedMessageIDs)
	}
	for _, want := range []string{first.MessageID, second.MessageID} {
		found := false
		for _, got := range req.AffectedMessageIDs {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected affected IDs to include %s, got %#v", want, req.AffectedMessageIDs)
		}
	}
}
