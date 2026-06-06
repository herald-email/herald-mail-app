package backend

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/memory"
	"github.com/herald-email/herald-mail-app/internal/models"
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

func TestLocalBackendDeleteCachedEmailRemovesScopedRowAndPreview(t *testing.T) {
	c, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	email := &models.EmailData{
		SourceID:  "gmail-api",
		AccountID: "work",
		LocalID:   "mail:gmail-api:work:INBOX:gmail:missing",
		MessageID: "msg-missing",
		UID:       55,
		Sender:    "person@example.com",
		Subject:   "Missing provider row",
		Date:      time.Now(),
		Folder:    "INBOX",
	}
	if err := c.CacheEmail(email); err != nil {
		t.Fatalf("CacheEmail: %v", err)
	}
	if err := c.CachePreviewBodyByRef(email.MessageRef(), &models.EmailBody{TextPlain: "cached preview"}, config.CacheStoragePolicyPreserveAll); err != nil {
		t.Fatalf("CachePreviewBodyByRef: %v", err)
	}

	b := &LocalBackend{
		cache:     c,
		bodyCache: map[string]*models.EmailBody{"INBOX:55": &models.EmailBody{TextPlain: "cached body"}},
		validIDsByFolder: map[string]map[string]bool{
			"INBOX": {email.MessageID: true},
		},
	}
	if err := b.DeleteCachedEmail(email.MessageRef()); err != nil {
		t.Fatalf("DeleteCachedEmail: %v", err)
	}

	if _, err := c.GetEmailByRef(email.MessageRef()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetEmailByRef after delete err = %v, want sql.ErrNoRows", err)
	}
	preview, err := c.GetPreviewBodyByRef(email.MessageRef())
	if err != nil {
		t.Fatalf("GetPreviewBodyByRef: %v", err)
	}
	if preview != nil {
		t.Fatalf("preview remained after cache delete: %#v", preview)
	}
	if len(b.bodyCache) != 0 {
		t.Fatalf("bodyCache length = %d, want cleared", len(b.bodyCache))
	}
	if b.validIDsByFolder["INBOX"][email.MessageID] {
		t.Fatalf("valid ID set still contains deleted message")
	}
}

func TestLocalBackendDeleteCachedEmailMarksMemorySourceMissing(t *testing.T) {
	c, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	email := &models.EmailData{
		SourceID:  models.DefaultMailSourceID,
		AccountID: models.DefaultAccountID,
		LocalID:   "mail:default-mail:default:INBOX:msg-source",
		MessageID: "msg-source",
		UID:       71,
		Sender:    "sergey@example.com",
		Subject:   "Take-home follow-up",
		Date:      time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC),
		Folder:    "INBOX",
	}
	if err := c.CacheEmail(email); err != nil {
		t.Fatalf("CacheEmail: %v", err)
	}
	store, err := memory.NewFileStoreWithClock(t.TempDir(), func() time.Time {
		return time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatalf("NewFileStoreWithClock: %v", err)
	}
	settings := memory.DefaultSettings()
	settings.Directory = store.Root()
	mem := memory.PrepareMemoryForAppend(memory.Memory{
		Kind:       memory.KindOpenQuestion,
		Claim:      "Sergey asked for a take-home follow-up.",
		Summary:    "Sergey asked for a take-home follow-up.",
		Topic:      "Take-home follow-up",
		People:     []string{"sergey@example.com"},
		Status:     memory.StatusWaiting,
		Confidence: 0.95,
		Evidence: []memory.Evidence{{
			SourceType: memory.SourceEmail,
			SourceID:   string(models.DefaultMailSourceID),
			AccountID:  string(models.DefaultAccountID),
			ID:         email.LocalID,
			MessageID:  email.MessageID,
			LocalID:    email.LocalID,
			Folder:     email.Folder,
			Date:       email.Date,
		}},
	}, time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC))
	if _, _, err := store.Append(context.Background(), mem); err != nil {
		t.Fatalf("Append memory: %v", err)
	}

	b := &LocalBackend{
		cache:         c,
		memoryService: memory.NewServiceWithStore(settings, store, nil),
		bodyCache:     map[string]*models.EmailBody{"INBOX:71": {TextPlain: "cached body"}},
		validIDsByFolder: map[string]map[string]bool{
			"INBOX": {email.MessageID: true},
		},
	}
	if err := b.DeleteCachedEmail(email.MessageRef()); err != nil {
		t.Fatalf("DeleteCachedEmail: %v", err)
	}

	raw, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 1 || raw[0].Status == memory.StatusSourceMissing {
		t.Fatalf("source-missing propagation mutated immutable record: %#v", raw)
	}
	effective, err := store.EffectiveList(context.Background(), settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(effective) != 1 || effective[0].Status != memory.StatusSourceMissing || effective[0].Freshness != memory.FreshnessStale {
		t.Fatalf("effective memory = %#v, want source_missing/stale", effective)
	}
	events, err := store.ReadControlEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Action != memory.ControlSourceMissing || events[0].SourceReference.MessageID != email.MessageID {
		t.Fatalf("control events = %#v", events)
	}
}

func TestLocalBackendPrimesFolderCacheFromPersistedFolderList(t *testing.T) {
	c, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if err := c.StoreFolderList([]string{"INBOX", "Sent", "Projects/Launch"}); err != nil {
		t.Fatalf("StoreFolderList: %v", err)
	}

	b := &LocalBackend{cache: c}
	if err := b.primeCachedFoldersFromCache(); err != nil {
		t.Fatalf("primeCachedFoldersFromCache: %v", err)
	}

	got, err := b.ListFolders()
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	want := []string{"INBOX", "Projects/Launch", "Sent"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListFolders from cached folder list = %#v, want %#v", got, want)
	}
}

type connectOnListMailSource struct {
	*fakeMailSource
	connected bool
}

func (s *connectOnListMailSource) Connect(ctx context.Context) error {
	s.connected = true
	return s.fakeMailSource.Connect(ctx)
}

func (s *connectOnListMailSource) ListFolders(ctx context.Context) ([]string, error) {
	if !s.connected {
		return nil, errors.New("not connected")
	}
	return s.fakeMailSource.ListFolders(ctx)
}

func TestLocalBackendListFoldersConnectsDisconnectedSourceWhenCacheEmpty(t *testing.T) {
	c, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	source := &connectOnListMailSource{fakeMailSource: newFakeMailSource()}
	source.folders = []string{"INBOX", "Archive", "Projects/Launch"}
	b := &LocalBackend{cache: c, mailSource: source}

	got, err := b.ListFolders()
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	want := []string{"INBOX", "Archive", "Projects/Launch"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListFolders after connect = %#v, want %#v", got, want)
	}
	if !source.connected {
		t.Fatal("ListFolders did not connect the source before retrying")
	}
}

// --- filterByValidIDs ---

func TestFilterByValidIDs_NilSet(t *testing.T) {
	b := &LocalBackend{} // validIDsByFolder is nil by default
	emails := []*models.EmailData{makeEmail("<a@x.com>"), makeEmail("<b@x.com>")}
	got := b.filterByValidIDs(emails)
	if len(got) != 2 {
		t.Errorf("nil validIDs: expected all 2 emails, got %d", len(got))
	}
}

func TestFilterByValidIDs_WithSet(t *testing.T) {
	b := &LocalBackend{}
	b.setValidIDsForFolder("INBOX", map[string]bool{"<a@x.com>": true, "<c@x.com>": true})

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

func TestFilterByValidIDs_IsScopedByFolder(t *testing.T) {
	b := &LocalBackend{}
	b.setValidIDsForFolder("INBOX", map[string]bool{"<inbox-live@x.com>": true})

	archiveEmail := makeEmail("<archive-not-yet-reconciled@x.com>")
	archiveEmail.Folder = "Archive"
	emails := []*models.EmailData{
		makeEmail("<inbox-live@x.com>"),
		makeEmail("<inbox-stale@x.com>"),
		archiveEmail,
	}

	got := b.filterByValidIDs(emails)
	if len(got) != 2 {
		t.Fatalf("expected INBOX to be filtered without hiding Archive, got %d emails", len(got))
	}
	if got[0].MessageID != "<inbox-live@x.com>" || got[1].MessageID != "<archive-not-yet-reconciled@x.com>" {
		t.Fatalf("unexpected scoped valid-ID result: %q, %q", got[0].MessageID, got[1].MessageID)
	}
}

func TestFilterSemanticResultsByValidIDs_WithSet(t *testing.T) {
	b := &LocalBackend{}
	b.setValidIDsForFolder("INBOX", map[string]bool{"<a@x.com>": true, "<c@x.com>": true})

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

func TestApplyCacheStoragePolicyPrunesRowsClearsBodyCacheAndAffectsFutureWrites(t *testing.T) {
	c, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	b := &LocalBackend{
		cache: c,
		cfg:   &config.Config{},
		bodyCache: map[string]*models.EmailBody{
			"INBOX:1": {TextPlain: "stale full body"},
		},
	}
	b.cfg.Cache.StoragePolicy = config.CacheStoragePolicyPreserveAll
	body := &models.EmailBody{
		TextPlain: "body",
		InlineImages: []models.InlineImage{
			{ContentID: "logo", MIMEType: "image/png", Data: []byte("png-bytes")},
		},
		Attachments: []models.Attachment{
			{Filename: "invoice.pdf", MIMEType: "application/pdf", Size: 9, PartPath: "2", Data: []byte("pdf-bytes")},
		},
	}
	if err := b.CachePreviewBody("existing", body); err != nil {
		t.Fatalf("CachePreviewBody existing: %v", err)
	}

	result, err := b.ApplyCacheStoragePolicy(config.CacheStoragePolicyLightweight)
	if err != nil {
		t.Fatalf("ApplyCacheStoragePolicy: %v", err)
	}
	if b.cfg.Cache.StoragePolicy != config.CacheStoragePolicyLightweight {
		t.Fatalf("backend policy = %q, want lightweight", b.cfg.Cache.StoragePolicy)
	}
	if result.RowsChanged != 1 || result.AttachmentBytesRemoved != int64(len("pdf-bytes")) || result.InlineImageBytesRemoved != int64(len("png-bytes")) {
		t.Fatalf("unexpected prune result: %#v", result)
	}
	b.bodyCacheMu.Lock()
	bodyCacheLen := len(b.bodyCache)
	b.bodyCacheMu.Unlock()
	if bodyCacheLen != 0 {
		t.Fatalf("body cache length = %d, want cleared", bodyCacheLen)
	}

	pruned, err := b.GetCachedPreviewBody("existing")
	if err != nil {
		t.Fatalf("GetCachedPreviewBody existing: %v", err)
	}
	if len(pruned.InlineImages) != 0 {
		t.Fatalf("inline image bytes lingered: %#v", pruned.InlineImages)
	}
	if len(pruned.Attachments) != 1 || pruned.Attachments[0].PartPath != "2" || len(pruned.Attachments[0].Data) != 0 {
		t.Fatalf("attachment metadata/data after prune = %#v", pruned.Attachments)
	}

	if err := b.CachePreviewBody("future", body); err != nil {
		t.Fatalf("CachePreviewBody future: %v", err)
	}
	future, err := b.GetCachedPreviewBody("future")
	if err != nil {
		t.Fatalf("GetCachedPreviewBody future: %v", err)
	}
	if len(future.InlineImages) != 0 {
		t.Fatalf("future write stored inline images under lightweight: %#v", future.InlineImages)
	}
	if len(future.Attachments) != 1 || len(future.Attachments[0].Data) != 0 {
		t.Fatalf("future write stored attachment bytes under lightweight: %#v", future.Attachments)
	}
}

func TestReclaimOfflineCacheStorageEstimatesPrunesAndClearsBodyCache(t *testing.T) {
	c, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	b := &LocalBackend{
		cache: c,
		cfg:   &config.Config{},
		bodyCache: map[string]*models.EmailBody{
			"INBOX:1": {TextPlain: "stale full body"},
		},
	}
	b.cfg.Cache.StoragePolicy = config.CacheStoragePolicyLightweight
	body := &models.EmailBody{
		TextPlain: "body",
		InlineImages: []models.InlineImage{
			{ContentID: "logo", MIMEType: "image/png", Data: []byte("png-bytes")},
		},
		Attachments: []models.Attachment{
			{Filename: "invoice.pdf", MIMEType: "application/pdf", Size: 9, PartPath: "2", Data: []byte("pdf-bytes")},
		},
	}
	if err := c.CachePreviewBody("existing", body, config.CacheStoragePolicyPreserveAll); err != nil {
		t.Fatalf("CachePreviewBody existing: %v", err)
	}

	estimate, err := b.EstimateOfflineCacheStorageReclaim(config.CacheStoragePolicyLightweight)
	if err != nil {
		t.Fatalf("EstimateOfflineCacheStorageReclaim: %v", err)
	}
	wantRemoved := int64(len("png-bytes") + len("pdf-bytes"))
	if estimate.ReclaimableBytes != wantRemoved {
		t.Fatalf("ReclaimableBytes = %d, want %d", estimate.ReclaimableBytes, wantRemoved)
	}

	result, err := b.ReclaimOfflineCacheStorage(config.CacheStoragePolicyLightweight)
	if err != nil {
		t.Fatalf("ReclaimOfflineCacheStorage: %v", err)
	}
	if result.PruneResult.RowsChanged != 1 || !result.Compacted {
		t.Fatalf("unexpected reclaim result: %#v", result)
	}
	b.bodyCacheMu.Lock()
	bodyCacheLen := len(b.bodyCache)
	b.bodyCacheMu.Unlock()
	if bodyCacheLen != 0 {
		t.Fatalf("body cache length = %d, want cleared", bodyCacheLen)
	}

	pruned, err := b.GetCachedPreviewBody("existing")
	if err != nil {
		t.Fatalf("GetCachedPreviewBody existing: %v", err)
	}
	if len(pruned.InlineImages) != 0 {
		t.Fatalf("inline image bytes lingered after reclaim: %#v", pruned.InlineImages)
	}
	if len(pruned.Attachments) != 1 || pruned.Attachments[0].PartPath != "2" || len(pruned.Attachments[0].Data) != 0 {
		t.Fatalf("attachment metadata/data after reclaim = %#v", pruned.Attachments)
	}
}

// --- isValidID ---

func TestIsValidID_NilSet(t *testing.T) {
	b := &LocalBackend{}
	if !b.isValidID("INBOX", "<anything@x.com>") {
		t.Error("nil validIDs: all IDs should be considered valid")
	}
}

func TestIsValidID_WithSet(t *testing.T) {
	b := &LocalBackend{}
	b.setValidIDsForFolder("INBOX", map[string]bool{"<a@x.com>": true})

	if !b.isValidID("INBOX", "<a@x.com>") {
		t.Error("expected <a> to be valid")
	}
	if b.isValidID("INBOX", "<b@x.com>") {
		t.Error("expected <b> to be invalid")
	}
	if !b.isValidID("Archive", "<b@x.com>") {
		t.Error("unreconciled folders should remain valid until their own valid-ID set arrives")
	}
}

func TestPrepareValidIDsChannelPublishesBeforeValuesAndStoresByFolder(t *testing.T) {
	b := &LocalBackend{}
	internalCh := b.prepareValidIDsChannelForFolder("INBOX")
	publicCh := b.ValidIDsCh()
	if publicCh == nil {
		t.Fatal("expected public valid-ID channel to be available before reconcile sends values")
	}

	internalCh <- map[string]bool{"<a@x.com>": true}
	close(internalCh)

	select {
	case ids, ok := <-publicCh:
		if !ok {
			t.Fatal("public valid-ID channel closed before forwarding IDs")
		}
		if !ids["<a@x.com>"] {
			t.Fatalf("forwarded IDs missing <a@x.com>: %v", ids)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for public valid-ID channel")
	}

	if !b.isValidID("INBOX", "<a@x.com>") {
		t.Fatal("expected forwarded IDs to be stored for INBOX")
	}
	if b.isValidID("INBOX", "<b@x.com>") {
		t.Fatal("expected missing INBOX ID to be invalid")
	}
	if !b.isValidID("Archive", "<b@x.com>") {
		t.Fatal("expected other folders to remain unfiltered")
	}
}

// --- GetUnclassifiedIDs filtering ---

// filterUnclassifiedIDs applies the valid-ID filter to a slice of IDs.
// This mirrors what the implementation will do inside GetUnclassifiedIDs.
func filterUnclassifiedIDs(b *LocalBackend, ids []string) []string {
	out := ids[:0:0]
	for _, id := range ids {
		if b.isValidID("INBOX", id) {
			out = append(out, id)
		}
	}
	return out
}

func TestGetUnclassifiedIDs_FiltersStale(t *testing.T) {
	b := &LocalBackend{}
	b.setValidIDsForFolder("INBOX", map[string]bool{
		"<a@x.com>": true,
		"<c@x.com>": true,
		"<e@x.com>": true,
	})

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
		if event.SourceID != models.DefaultMailSourceID || event.AccountID != models.DefaultAccountID {
			t.Fatalf("unexpected sync event source scope: %+v", event)
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

func TestDemoBackendLoadEmitsDefaultSourceScope(t *testing.T) {
	b := NewDemoBackend()
	b.Load("INBOX")

	select {
	case event := <-b.SyncEvents():
		if event.SourceID != models.DefaultMailSourceID || event.AccountID != models.DefaultAccountID {
			t.Fatalf("unexpected demo sync event source scope: %+v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for demo sync event")
	}
}

func TestRemoteBackendEventsUseDefaultSourceScope(t *testing.T) {
	b := &RemoteBackend{
		progressCh:   make(chan models.ProgressInfo, 1),
		syncEventsCh: make(chan models.FolderSyncEvent, 1),
		newEmailsCh:  make(chan models.NewEmailsNotification, 1),
	}

	b.handleSSEEvent("progress", []byte(`{"phase":"complete","message":"done"}`))
	select {
	case event := <-b.SyncEvents():
		if event.SourceID != models.DefaultMailSourceID || event.AccountID != models.DefaultAccountID {
			t.Fatalf("unexpected remote sync event source scope: %+v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for remote sync event")
	}

	b.handleSSEEvent("new_emails", []byte(`{"folder":"INBOX","emails":[]}`))
	select {
	case notification := <-b.NewEmailsCh():
		if notification.SourceID != models.DefaultMailSourceID || notification.AccountID != models.DefaultAccountID {
			t.Fatalf("unexpected remote new-email source scope: %+v", notification)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for remote new-email notification")
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

func TestBuildAllMailOnlyView_ExplainsFolderUnassignedSemantics(t *testing.T) {
	allMail := []*models.EmailData{
		{MessageID: "<keep@x.com>", Sender: "a@x.com", Subject: "keep", Folder: "All Mail"},
	}
	membership := map[string]map[string]bool{
		"All Mail": {
			"<keep@x.com>": true,
		},
	}

	view := buildAllMailOnlyView("All Mail", allMail, membership, true, "")
	if !view.Supported {
		t.Fatalf("expected supported view, got unsupported: %s", view.Reason)
	}
	if !strings.Contains(view.Reason, "no other folder") {
		t.Fatalf("expected supported All Mail only view to explain folder-unassigned semantics, got %q", view.Reason)
	}
}

func TestAttachmentMatchesLookupAcceptsFilenameOrPartPath(t *testing.T) {
	attachment := models.Attachment{
		Filename: "Invoice.pdf",
		PartPath: "2.1",
	}

	for _, lookup := range []string{"invoice.pdf", "2.1"} {
		if !attachmentMatchesLookup(attachment, lookup) {
			t.Fatalf("expected lookup %q to match %#v", lookup, attachment)
		}
	}
	if attachmentMatchesLookup(attachment, "other.pdf") {
		t.Fatal("unexpected match for unrelated attachment lookup")
	}
}
