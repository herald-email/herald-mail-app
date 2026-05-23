package backend

import (
	"context"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestLocalBackendDoesNotRetainConcreteIMAPClient(t *testing.T) {
	typ := reflect.TypeOf(LocalBackend{})
	if _, ok := typ.FieldByName("imapClient"); ok {
		t.Fatal("LocalBackend should depend on mailSource, not retain a concrete imapClient field")
	}
	if _, ok := typ.FieldByName("mailSource"); !ok {
		t.Fatal("LocalBackend should keep provider access behind a mailSource field")
	}
}

func TestLocalBackendRoutesProviderOperationsThroughMailSource(t *testing.T) {
	source := newFakeMailSource()
	source.folders = []string{"INBOX", "Archive"}
	source.statuses = map[string]models.FolderStatus{
		"INBOX": {Total: 2, Unseen: 1},
	}
	source.searchResults = []*models.EmailData{{MessageID: "<found@x>", Folder: "INBOX"}}
	source.appendUID = 42
	source.appendFolder = "Drafts"
	source.drafts = []*models.Draft{{UID: 42, Folder: "Drafts", Subject: "draft"}}

	b := &LocalBackend{
		mailSource:  source,
		cache:       newMemoryCache(t),
		cfg:         &config.Config{},
		newEmailsCh: make(chan models.NewEmailsNotification, 1),
	}

	folders, err := b.ListFolders()
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if !reflect.DeepEqual(folders, []string{"INBOX", "Archive"}) {
		t.Fatalf("folders = %#v", folders)
	}

	statuses, err := b.GetFolderStatus([]string{"INBOX"})
	if err != nil {
		t.Fatalf("GetFolderStatus: %v", err)
	}
	if statuses["INBOX"].Unseen != 1 {
		t.Fatalf("status = %#v", statuses)
	}

	results, err := b.SearchEmailsIMAP("INBOX", "needle")
	if err != nil {
		t.Fatalf("SearchEmailsIMAP: %v", err)
	}
	if len(results) != 1 || results[0].MessageID != "<found@x>" {
		t.Fatalf("search results = %#v", results)
	}

	if err := b.DeleteEmail("<delete@x>", "INBOX"); err != nil {
		t.Fatalf("DeleteEmail: %v", err)
	}
	if err := b.ArchiveEmail("<archive@x>", "INBOX"); err != nil {
		t.Fatalf("ArchiveEmail: %v", err)
	}
	if err := b.MoveEmail("<move@x>", "INBOX", "Archive"); err != nil {
		t.Fatalf("MoveEmail: %v", err)
	}
	uid, folder, err := b.SaveRawDraft([]byte("raw draft"))
	if err != nil {
		t.Fatalf("SaveRawDraft: %v", err)
	}
	if uid != 42 || folder != "Drafts" {
		t.Fatalf("draft append = (%d, %q), want (42, Drafts)", uid, folder)
	}
	drafts, err := b.ListDrafts()
	if err != nil {
		t.Fatalf("ListDrafts: %v", err)
	}
	if len(drafts) != 1 || drafts[0].UID != 42 {
		t.Fatalf("drafts = %#v", drafts)
	}
	if err := b.DeleteDraft(42, "Drafts"); err != nil {
		t.Fatalf("DeleteDraft: %v", err)
	}
	if err := b.CreateFolder("Projects"); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if err := b.RenameFolder("Projects", "Projects/2026"); err != nil {
		t.Fatalf("RenameFolder: %v", err)
	}
	if err := b.DeleteFolder("Projects/2026"); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}

	wantCalls := []string{
		"list-folders",
		"folder-status:INBOX",
		"search:INBOX:needle",
		"delete-email:INBOX:<delete@x>",
		"archive-email:INBOX:<archive@x>",
		"move-email:INBOX:Archive:<move@x>",
		"append-draft:9",
		"list-drafts",
		"delete-draft:Drafts:42",
		"create-mailbox:Projects",
		"rename-mailbox:Projects:Projects/2026",
		"delete-mailbox:Projects/2026",
	}
	if got := source.callsSnapshot(); !reflect.DeepEqual(got, wantCalls) {
		t.Fatalf("source calls = %#v, want %#v", got, wantCalls)
	}
}

func TestLocalBackendRunLoadUsesMailSourceInOrder(t *testing.T) {
	source := newFakeMailSource()
	source.folders = []string{"INBOX", "Archive"}
	source.stats = map[string]*models.SenderStats{
		"sender@example.com": {TotalEmails: 3},
	}
	source.reconcileIDs = map[string]bool{"<live@x>": true}

	b := &LocalBackend{
		mailSource:       source,
		cache:            newMemoryCache(t),
		cfg:              &config.Config{},
		progressCh:       make(chan models.ProgressInfo, 20),
		syncEventsCh:     make(chan models.FolderSyncEvent, 20),
		lastFetchCurrent: make(map[int64]int),
	}

	b.runLoad(folderLoadRequest{Folder: "INBOX", Generation: 7})

	wantCalls := []string{
		"connect",
		"list-folders",
		"process:INBOX",
		"stats:INBOX",
		"reconcile:INBOX",
	}
	if got := source.callsSnapshot(); !reflect.DeepEqual(got, wantCalls) {
		t.Fatalf("source calls = %#v, want %#v", got, wantCalls)
	}

	select {
	case ids := <-b.ValidIDsCh():
		if !ids["<live@x>"] {
			t.Fatalf("valid IDs = %#v, want <live@x>", ids)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for source reconcile IDs")
	}
}

func TestLocalBackendSourceBackedMessageServiceUsesMailSource(t *testing.T) {
	source := newFakeMailSource()
	ref := models.MessageRef{
		Folder:    "INBOX",
		UID:       42,
		MessageID: "<body@x>",
	}.WithDefaults()
	source.fullBodies[source.refKey(ref)] = &models.EmailBody{TextPlain: "full from source"}
	source.previewBodies[source.refKey(ref)] = &models.EmailBody{TextPlain: "preview from source"}

	c := newMemoryCache(t)
	cfg := &config.Config{}
	cfg.Cache.StoragePolicy = config.CacheStoragePolicyPreserveAll
	b := &LocalBackend{
		mailSource: source,
		cache:      c,
		cfg:        cfg,
	}

	full, err := b.GetMessageNoCache(context.Background(), ref)
	if err != nil {
		t.Fatalf("GetMessageNoCache: %v", err)
	}
	if full.Body == nil || full.Body.TextPlain != "full from source" || full.Body.MessageID != "<body@x>" {
		t.Fatalf("full body = %#v", full.Body)
	}
	cached, err := c.GetMessageBodyByRef(ref)
	if err != nil {
		t.Fatalf("GetMessageBodyByRef: %v", err)
	}
	if cached == nil || cached.TextPlain != "full from source" {
		t.Fatalf("cached full body = %#v", cached)
	}

	cfg.Cache.StoragePolicy = config.CacheStoragePolicyNoAttachments
	preview, err := b.GetMessagePreviewNoCache(context.Background(), ref)
	if err != nil {
		t.Fatalf("GetMessagePreviewNoCache: %v", err)
	}
	if preview.Body == nil || preview.Body.TextPlain != "preview from source" || preview.Body.MessageID != "<body@x>" {
		t.Fatalf("preview body = %#v", preview.Body)
	}

	wantCalls := []string{
		"fetch-message:INBOX:42",
		"fetch-preview:INBOX:42",
	}
	if got := source.callsSnapshot(); !reflect.DeepEqual(got, wantCalls) {
		t.Fatalf("source calls = %#v, want %#v", got, wantCalls)
	}
}

func TestAllMailOnlyViewUsesMailSourceMembership(t *testing.T) {
	source := newFakeMailSource()
	source.folders = []string{"INBOX", "All Mail", "Labels/Home"}
	source.membership = map[string]map[string]bool{
		"All Mail": {
			"<keep@x>":  true,
			"<inbox@x>": true,
		},
		"INBOX": {
			"<inbox@x>": true,
		},
	}

	c := newMemoryCache(t)
	for _, email := range []*models.EmailData{
		{MessageID: "<keep@x>", UID: 1, Folder: "All Mail", Sender: "a@example.com", Subject: "keep", Date: time.Now()},
		{MessageID: "<inbox@x>", UID: 2, Folder: "All Mail", Sender: "b@example.com", Subject: "inbox", Date: time.Now()},
	} {
		if err := c.CacheEmail(email); err != nil {
			t.Fatalf("CacheEmail(%s): %v", email.MessageID, err)
		}
	}
	b := &LocalBackend{
		mailSource: source,
		cache:      c,
		cfg:        &config.Config{},
	}

	view, err := b.GetAllMailOnlyView()
	if err != nil {
		t.Fatalf("GetAllMailOnlyView: %v", err)
	}
	if !view.Supported {
		t.Fatalf("expected supported All Mail only view, got %q", view.Reason)
	}
	if len(view.Emails) != 1 || view.Emails[0].MessageID != "<keep@x>" {
		t.Fatalf("view emails = %#v", view.Emails)
	}

	gotCalls := source.callsSnapshot()
	for _, want := range []string{"list-folders", "process:All Mail", "folder-message-ids:All Mail,INBOX,Labels/Home"} {
		if !slices.Contains(gotCalls, want) {
			t.Fatalf("source calls = %#v, missing %q", gotCalls, want)
		}
	}
}

func newMemoryCache(t *testing.T) *cache.Cache {
	t.Helper()
	c, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

type fakeMailSource struct {
	mu sync.Mutex

	calls []string

	folders       []string
	statuses      map[string]models.FolderStatus
	groupedEmails map[string][]*models.EmailData
	stats         map[string]*models.SenderStats
	searchResults []*models.EmailData
	reconcileIDs  map[string]bool
	membership    map[string]map[string]bool

	fullBodies    map[string]*models.EmailBody
	previewBodies map[string]*models.EmailBody

	appendUID    uint32
	appendFolder string
	drafts       []*models.Draft
	pollEmails   []*models.EmailData
}

func newFakeMailSource() *fakeMailSource {
	return &fakeMailSource{
		statuses:      make(map[string]models.FolderStatus),
		groupedEmails: make(map[string][]*models.EmailData),
		stats:         make(map[string]*models.SenderStats),
		reconcileIDs:  make(map[string]bool),
		membership:    make(map[string]map[string]bool),
		fullBodies:    make(map[string]*models.EmailBody),
		previewBodies: make(map[string]*models.EmailBody),
		appendFolder:  "Drafts",
	}
}

func (s *fakeMailSource) record(call string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call)
}

func (s *fakeMailSource) callsSnapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

func (s *fakeMailSource) refKey(ref models.MessageRef) string {
	ref = ref.WithDefaults()
	return ref.Folder + ":" + uintString(ref.UID)
}

func (s *fakeMailSource) Connect(context.Context) error {
	s.record("connect")
	return nil
}

func (s *fakeMailSource) Close() error {
	s.record("close")
	return nil
}

func (s *fakeMailSource) ListFolders(context.Context) ([]string, error) {
	s.record("list-folders")
	return append([]string(nil), s.folders...), nil
}

func (s *fakeMailSource) GetFolderStatus(_ context.Context, folders []string) (map[string]models.FolderStatus, error) {
	s.record("folder-status:" + stringsJoin(folders, ","))
	out := make(map[string]models.FolderStatus, len(folders))
	for _, folder := range folders {
		out[folder] = s.statuses[folder]
	}
	return out, nil
}

func (s *fakeMailSource) ProcessEmailsIncremental(_ context.Context, folder string) error {
	s.record("process:" + folder)
	return nil
}

func (s *fakeMailSource) GetSenderStatistics(_ context.Context, folder string) (map[string]*models.SenderStats, error) {
	s.record("stats:" + folder)
	return s.stats, nil
}

func (s *fakeMailSource) GetEmailsBySender(_ context.Context, folder string) (map[string][]*models.EmailData, error) {
	s.record("emails-by-sender:" + folder)
	return s.groupedEmails, nil
}

func (s *fakeMailSource) StartBackgroundReconcile(_ context.Context, folder string, validIDsCh chan<- map[string]bool) {
	s.record("reconcile:" + folder)
	validIDsCh <- s.reconcileIDs
	close(validIDsCh)
}

func (s *fakeMailSource) DeleteSenderEmails(_ context.Context, sender, folder string) error {
	s.record("delete-sender:" + folder + ":" + sender)
	return nil
}

func (s *fakeMailSource) DeleteDomainEmails(_ context.Context, domain, folder string) error {
	s.record("delete-domain:" + folder + ":" + domain)
	return nil
}

func (s *fakeMailSource) DeleteEmail(_ context.Context, messageID, folder string) error {
	s.record("delete-email:" + folder + ":" + messageID)
	return nil
}

func (s *fakeMailSource) FetchMessageNoCache(_ context.Context, ref models.MessageRef) (*models.EmailBody, error) {
	ref = ref.WithDefaults()
	s.record("fetch-message:" + ref.Folder + ":" + uintString(ref.UID))
	return cloneEmailBody(s.fullBodies[s.refKey(ref)]), nil
}

func (s *fakeMailSource) FetchMessagePreviewNoCache(_ context.Context, ref models.MessageRef) (*models.EmailBody, error) {
	ref = ref.WithDefaults()
	s.record("fetch-preview:" + ref.Folder + ":" + uintString(ref.UID))
	return cloneEmailBody(s.previewBodies[s.refKey(ref)]), nil
}

func (s *fakeMailSource) SetGroupByDomain(bool) {}

func (s *fakeMailSource) ArchiveEmail(_ context.Context, messageID, folder string) error {
	s.record("archive-email:" + folder + ":" + messageID)
	return nil
}

func (s *fakeMailSource) ArchiveSenderEmails(_ context.Context, sender, folder string) error {
	s.record("archive-sender:" + folder + ":" + sender)
	return nil
}

func (s *fakeMailSource) SearchIMAP(_ context.Context, folder, query string) ([]*models.EmailData, error) {
	s.record("search:" + folder + ":" + query)
	return append([]*models.EmailData(nil), s.searchResults...), nil
}

func (s *fakeMailSource) MarkRead(_ context.Context, uid uint32, folder string) error {
	s.record("mark-read:" + folder + ":" + uintString(uid))
	return nil
}

func (s *fakeMailSource) MarkUnread(_ context.Context, uid uint32, folder string) error {
	s.record("mark-unread:" + folder + ":" + uintString(uid))
	return nil
}

func (s *fakeMailSource) MarkStarred(_ context.Context, uid uint32, folder string) error {
	s.record("mark-starred:" + folder + ":" + uintString(uid))
	return nil
}

func (s *fakeMailSource) UnmarkStarred(_ context.Context, uid uint32, folder string) error {
	s.record("unmark-starred:" + folder + ":" + uintString(uid))
	return nil
}

func (s *fakeMailSource) StartIDLE(_ context.Context, folder string, _ chan<- models.NewEmailsNotification) error {
	s.record("start-idle:" + folder)
	return nil
}

func (s *fakeMailSource) StopIDLE() {
	s.record("stop-idle")
}

func (s *fakeMailSource) PollForNewEmails(_ context.Context, folder string, _ time.Time) ([]*models.EmailData, error) {
	s.record("poll:" + folder)
	return append([]*models.EmailData(nil), s.pollEmails...), nil
}

func (s *fakeMailSource) MoveEmail(_ context.Context, messageID, fromFolder, toFolder string) error {
	s.record("move-email:" + fromFolder + ":" + toFolder + ":" + messageID)
	return nil
}

func (s *fakeMailSource) AppendDraft(_ context.Context, raw []byte) (uint32, string, error) {
	s.record("append-draft:" + uintString(uint32(len(raw))))
	return s.appendUID, s.appendFolder, nil
}

func (s *fakeMailSource) ListDrafts(context.Context) ([]*models.Draft, error) {
	s.record("list-drafts")
	return append([]*models.Draft(nil), s.drafts...), nil
}

func (s *fakeMailSource) DeleteDraft(_ context.Context, uid uint32, folder string) error {
	s.record("delete-draft:" + folder + ":" + uintString(uid))
	return nil
}

func (s *fakeMailSource) CreateMailbox(_ context.Context, name string) error {
	s.record("create-mailbox:" + name)
	return nil
}

func (s *fakeMailSource) RenameMailbox(_ context.Context, existingName, newName string) error {
	s.record("rename-mailbox:" + existingName + ":" + newName)
	return nil
}

func (s *fakeMailSource) DeleteMailbox(_ context.Context, name string) error {
	s.record("delete-mailbox:" + name)
	return nil
}

func (s *fakeMailSource) GetFolderMessageIDs(_ context.Context, folders []string) (map[string]map[string]bool, error) {
	slices.Sort(folders)
	s.record("folder-message-ids:" + stringsJoin(folders, ","))
	return s.membership, nil
}

func stringsJoin(values []string, sep string) string {
	if len(values) == 0 {
		return ""
	}
	out := values[0]
	for _, value := range values[1:] {
		out += sep + value
	}
	return out
}

func uintString(v uint32) string {
	if v == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
