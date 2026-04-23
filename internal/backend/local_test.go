package backend

import (
	"testing"
	"time"

	"mail-processor/internal/models"
)

func makeEmail(id string) *models.EmailData {
	return &models.EmailData{
		MessageID: id,
		Sender:    "a@x.com",
		Subject:   "test",
		Date:      time.Now(),
		Folder:    "INBOX",
	}
}

// --- filterByValidIDs ---

func TestFilterByValidIDs_NilSet(t *testing.T) {
	b := &LocalBackend{} // validIDs is nil by default
	emails := []*models.EmailData{makeEmail("<a@x.com>"), makeEmail("<b@x.com>")}
	got := b.filterByValidIDs(emails)
	if len(got) != 2 {
		t.Errorf("nil validIDs: expected all 2 emails, got %d", len(got))
	}
}

func TestFilterByValidIDs_WithSet(t *testing.T) {
	b := &LocalBackend{}
	b.validIDs = map[string]bool{"<a@x.com>": true, "<c@x.com>": true}

	emails := []*models.EmailData{
		makeEmail("<a@x.com>"),
		makeEmail("<b@x.com>"), // not in valid set
		makeEmail("<c@x.com>"),
		makeEmail("<d@x.com>"), // not in valid set
		makeEmail("<e@x.com>"), // not in valid set
	}
	got := b.filterByValidIDs(emails)
	if len(got) != 2 {
		t.Fatalf("expected 2 emails, got %d", len(got))
	}
	ids := map[string]bool{got[0].MessageID: true, got[1].MessageID: true}
	if !ids["<a@x.com>"] || !ids["<c@x.com>"] {
		t.Errorf("expected <a> and <c>, got %v", ids)
	}
}

func TestFilterSemanticResultsByValidIDs_WithSet(t *testing.T) {
	b := &LocalBackend{}
	b.validIDs = map[string]bool{"<a@x.com>": true, "<c@x.com>": true}

	results := []*models.SemanticSearchResult{
		{Email: makeEmail("<a@x.com>"), Score: 0.91},
		{Email: makeEmail("<b@x.com>"), Score: 0.88},
		nil,
		{Email: nil, Score: 0.70},
		{Email: makeEmail("<c@x.com>"), Score: 0.77},
	}

	got := b.filterSemanticResultsByValidIDs(results)
	if len(got) != 2 {
		t.Fatalf("expected 2 semantic results, got %d", len(got))
	}
	if got[0].Email.MessageID != "<a@x.com>" || got[1].Email.MessageID != "<c@x.com>" {
		t.Fatalf("unexpected semantic results after filtering: %q, %q", got[0].Email.MessageID, got[1].Email.MessageID)
	}
}

func TestSenderStatisticsFromGroups_IgnoresEmptyGroups(t *testing.T) {
	now := time.Now()
	grouped := map[string][]*models.EmailData{
		"active@example.com": {
			{
				MessageID:      "<a@x.com>",
				Sender:         "active@example.com",
				Date:           now.Add(-2 * time.Hour),
				Size:           100,
				HasAttachments: true,
			},
			{
				MessageID:      "<b@x.com>",
				Sender:         "active@example.com",
				Date:           now,
				Size:           300,
				HasAttachments: false,
			},
		},
		"stale@example.com": nil,
	}

	stats := senderStatisticsFromGroups(grouped)

	if _, ok := stats["stale@example.com"]; ok {
		t.Fatal("expected empty sender group to be skipped")
	}
	stat, ok := stats["active@example.com"]
	if !ok {
		t.Fatal("expected non-empty sender group to produce statistics")
	}
	if stat.TotalEmails != 2 {
		t.Fatalf("expected 2 emails, got %d", stat.TotalEmails)
	}
	if stat.WithAttachments != 1 {
		t.Fatalf("expected 1 attachment-bearing email, got %d", stat.WithAttachments)
	}
	if stat.AvgSize != 200 {
		t.Fatalf("expected avg size 200, got %f", stat.AvgSize)
	}
}

// --- isValidID ---

func TestIsValidID_NilSet(t *testing.T) {
	b := &LocalBackend{}
	if !b.isValidID("<anything@x.com>") {
		t.Error("nil validIDs: all IDs should be considered valid")
	}
}

func TestIsValidID_WithSet(t *testing.T) {
	b := &LocalBackend{}
	b.validIDs = map[string]bool{"<a@x.com>": true}

	if !b.isValidID("<a@x.com>") {
		t.Error("expected <a> to be valid")
	}
	if b.isValidID("<b@x.com>") {
		t.Error("expected <b> to be invalid")
	}
}

// --- GetUnclassifiedIDs filtering ---

// filterUnclassifiedIDs applies the valid-ID filter to a slice of IDs.
// This mirrors what the implementation will do inside GetUnclassifiedIDs.
func filterUnclassifiedIDs(b *LocalBackend, ids []string) []string {
	out := ids[:0:0]
	for _, id := range ids {
		if b.isValidID(id) {
			out = append(out, id)
		}
	}
	return out
}

func TestGetUnclassifiedIDs_FiltersStale(t *testing.T) {
	b := &LocalBackend{}
	b.validIDs = map[string]bool{
		"<a@x.com>": true,
		"<c@x.com>": true,
		"<e@x.com>": true,
	}

	all := []string{"<a@x.com>", "<b@x.com>", "<c@x.com>", "<d@x.com>", "<e@x.com>"}
	got := filterUnclassifiedIDs(b, all)

	if len(got) != 3 {
		t.Fatalf("expected 3 valid IDs, got %d: %v", len(got), got)
	}
	valid := map[string]bool{}
	for _, id := range got {
		valid[id] = true
	}
	for _, id := range []string{"<a@x.com>", "<c@x.com>", "<e@x.com>"} {
		if !valid[id] {
			t.Errorf("expected %s in result", id)
		}
	}
}

func TestSendProgress_DoesNotPanicAfterClose(t *testing.T) {
	b := &LocalBackend{
		rawProgressCh: make(chan models.ProgressInfo, 1),
	}
	b.closed.Store(true)
	close(b.rawProgressCh)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("sendProgress should not panic after Close, got %v", r)
		}
	}()

	b.sendProgress(models.ProgressInfo{Phase: "complete", Message: "done"})
}

func TestFanoutProgressLoop_EmitsCompleteSyncEventForBackendProgress(t *testing.T) {
	rawProgressCh := make(chan models.ProgressInfo, 1)
	progressCh := make(chan models.ProgressInfo, 1)
	syncEventsCh := make(chan models.FolderSyncEvent, 1)

	b := &LocalBackend{
		rawProgressCh:    rawProgressCh,
		progressCh:       progressCh,
		syncEventsCh:     syncEventsCh,
		lastFetchCurrent: make(map[int64]int),
	}
	b.setActiveLoad(folderLoadRequest{Folder: "INBOX", Generation: 7})

	done := make(chan struct{})
	go func() {
		b.fanoutProgressLoop()
		close(done)
	}()

	rawProgressCh <- models.ProgressInfo{Phase: "complete", Message: "Found 12 senders"}

	select {
	case event := <-syncEventsCh:
		if event.Phase != models.SyncPhaseComplete {
			t.Fatalf("expected complete sync phase, got %q", event.Phase)
		}
		if event.Folder != "INBOX" || event.Generation != 7 {
			t.Fatalf("unexpected sync event identity: %+v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sync completion event")
	}

	select {
	case progress := <-progressCh:
		if progress.Phase != "complete" {
			t.Fatalf("expected progress copy to reach UI, got %+v", progress)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for UI progress copy")
	}

	close(rawProgressCh)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("fanout progress loop did not exit after raw progress channel closed")
	}
}

func TestBuildAllMailOnlyView_StrictExclusions(t *testing.T) {
	allMail := []*models.EmailData{
		{MessageID: "<keep@x.com>", Sender: "a@x.com", Subject: "keep", Folder: "All Mail"},
		{MessageID: "<inbox@x.com>", Sender: "b@x.com", Subject: "inbox", Folder: "All Mail"},
		{MessageID: "<custom@x.com>", Sender: "c@x.com", Subject: "custom", Folder: "All Mail"},
		{MessageID: "", Sender: "d@x.com", Subject: "missing id", Folder: "All Mail"},
	}
	membership := map[string]map[string]bool{
		"All Mail": {
			"<keep@x.com>":   true,
			"<inbox@x.com>":  true,
			"<custom@x.com>": true,
		},
		"INBOX": {
			"<inbox@x.com>": true,
		},
		"Labels/Home": {
			"<custom@x.com>": true,
		},
	}

	view := buildAllMailOnlyView("All Mail", allMail, membership, true, "")

	if !view.Supported {
		t.Fatalf("expected supported view, got unsupported: %s", view.Reason)
	}
	if len(view.Emails) != 1 {
		t.Fatalf("expected exactly 1 all-mail-only message, got %d", len(view.Emails))
	}
	if view.Emails[0].MessageID != "<keep@x.com>" {
		t.Fatalf("expected <keep@x.com>, got %q", view.Emails[0].MessageID)
	}
}

func TestBuildAllMailOnlyView_UnsupportedWhenAllMailMissing(t *testing.T) {
	view := buildAllMailOnlyView("", nil, nil, true, "")
	if view.Supported {
		t.Fatalf("expected unsupported view when All Mail is missing")
	}
	if view.Reason == "" {
		t.Fatalf("expected unsupported reason when All Mail is missing")
	}
}

func TestBuildAllMailOnlyView_FailsClosedWhenMembershipIncomplete(t *testing.T) {
	allMail := []*models.EmailData{
		{MessageID: "<maybe@x.com>", Sender: "a@x.com", Subject: "maybe", Folder: "All Mail"},
	}

	view := buildAllMailOnlyView("All Mail", allMail, nil, false, "membership inspection incomplete")
	if view.Supported {
		t.Fatalf("expected unsupported view when membership inspection is incomplete")
	}
	if view.Reason == "" {
		t.Fatalf("expected an error reason for incomplete membership inspection")
	}
	if len(view.Emails) != 0 {
		t.Fatalf("expected no partial emails on fail-closed result, got %d", len(view.Emails))
	}
}
