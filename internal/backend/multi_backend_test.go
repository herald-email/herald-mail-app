package backend

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type recordingAccountBackend struct {
	*DemoBackend
	name                  string
	folders               []string
	status                map[string]models.FolderStatus
	timeline              map[string][]*models.EmailData
	bodies                map[string]*models.EmailBody
	search                map[string][]*models.EmailData
	loadCalls             []string
	deleteCalls           []string
	deleteBatchRefs       [][]models.MessageRef
	archiveCalls          []string
	moveCalls             []string
	sendCalls             []string
	composeSends          []ComposeSendRequest
	saveDraftCalls        []string
	rawDraftCalls         []string
	draftDeletes          []string
	draftSends            []string
	getMessageRefs        []models.MessageRef
	embeddingRefs         []models.MessageRef
	storedEmbeddingRefs   []models.MessageRef
	calendarEvents        []models.CalendarEvent
	calendarSearch        []string
	createdCalendarEvents []models.CalendarEvent
	uidLookupRefs         []models.CollectionRef
	uidLookupUIDs         []string
	savedCalendarEvents   []models.CalendarEvent
	rsvpCalendarRefs      []models.EventRef
	rsvpCalendarStatus    []string
	closed                bool
}

func newRecordingAccountBackend(name string, folders []string, email *models.EmailData, body string) *recordingAccountBackend {
	b := &recordingAccountBackend{
		DemoBackend: NewDemoBackend(),
		name:        name,
		folders:     folders,
		status:      make(map[string]models.FolderStatus),
		timeline:    make(map[string][]*models.EmailData),
		bodies:      make(map[string]*models.EmailBody),
		search:      make(map[string][]*models.EmailData),
	}
	for _, folder := range folders {
		b.status[folder] = models.FolderStatus{Unseen: 1, Total: 3}
	}
	if email != nil {
		b.timeline[email.Folder] = []*models.EmailData{email}
		b.search[email.Subject] = []*models.EmailData{email}
		b.bodies[fmt.Sprintf("%s:%d", email.Folder, email.UID)] = &models.EmailBody{
			MessageID: email.MessageID,
			TextPlain: body,
		}
	}
	return b
}

func scopedTestEmail(email *models.EmailData) *models.EmailData {
	ref := email.MessageRef()
	email.SourceID = ref.SourceID
	email.AccountID = ref.AccountID
	email.LocalID = ref.LocalID
	return email
}

func (b *recordingAccountBackend) Load(folder string) {
	b.loadCalls = append(b.loadCalls, folder)
}

func (b *recordingAccountBackend) ListFolders() ([]string, error) {
	out := make([]string, len(b.folders))
	copy(out, b.folders)
	return out, nil
}

func (b *recordingAccountBackend) GetFolderStatus(folders []string) (map[string]models.FolderStatus, error) {
	out := make(map[string]models.FolderStatus, len(folders))
	for _, folder := range folders {
		if st, ok := b.status[folder]; ok {
			out[folder] = st
		}
	}
	return out, nil
}

func (b *recordingAccountBackend) GetTimelineEmails(folder string) ([]*models.EmailData, error) {
	out := make([]*models.EmailData, len(b.timeline[folder]))
	copy(out, b.timeline[folder])
	return out, nil
}

func (b *recordingAccountBackend) SearchEmails(folder, query string, bodySearch bool) ([]*models.EmailData, error) {
	candidates := b.timeline[folder]
	out := make([]*models.EmailData, 0, len(candidates))
	for _, email := range candidates {
		if email != nil && (query == "" || email.Subject == query || email.MessageID == query) {
			out = append(out, email)
		}
	}
	return out, nil
}

func (b *recordingAccountBackend) GetUnembeddedRefsWithBody(string) ([]models.MessageRef, error) {
	out := make([]models.MessageRef, len(b.embeddingRefs))
	copy(out, b.embeddingRefs)
	return out, nil
}

func (b *recordingAccountBackend) GetUncachedBodyRefs(string, int) ([]models.MessageRef, error) {
	return nil, nil
}

func (b *recordingAccountBackend) GetBodyTextByRef(models.MessageRef) (string, error) {
	return "body", nil
}

func (b *recordingAccountBackend) FetchAndCacheBodyByRef(models.MessageRef) (*models.EmailBody, error) {
	return nil, nil
}

func (b *recordingAccountBackend) StoreEmbeddingChunksByRef(ref models.MessageRef, _ []models.EmbeddingChunk) error {
	b.storedEmbeddingRefs = append(b.storedEmbeddingRefs, ref)
	return nil
}

func (b *recordingAccountBackend) DeleteEmail(messageID, folder string) error {
	b.deleteCalls = append(b.deleteCalls, folder+":"+messageID)
	return nil
}

func (b *recordingAccountBackend) DeleteEmailsByRef(refs []models.MessageRef) error {
	b.deleteBatchRefs = append(b.deleteBatchRefs, append([]models.MessageRef(nil), refs...))
	return nil
}

func (b *recordingAccountBackend) ArchiveEmail(messageID, folder string) error {
	b.archiveCalls = append(b.archiveCalls, folder+":"+messageID)
	return nil
}

func (b *recordingAccountBackend) MoveEmail(messageID, from, to string) error {
	b.moveCalls = append(b.moveCalls, from+":"+messageID+":"+to)
	return nil
}

func (b *recordingAccountBackend) SendEmail(to, subject, body, from string) error {
	b.sendCalls = append(b.sendCalls, fmt.Sprintf("%s|%s|%s|%s", from, to, subject, body))
	return nil
}

func (b *recordingAccountBackend) SendCompose(req ComposeSendRequest) error {
	b.composeSends = append(b.composeSends, req)
	return nil
}

func (b *recordingAccountBackend) SaveDraft(to, cc, bcc, subject, body string) (uint32, string, error) {
	b.saveDraftCalls = append(b.saveDraftCalls, fmt.Sprintf("%s|%s|%s|%s|%s", to, cc, bcc, subject, body))
	return 501, "Drafts", nil
}

func (b *recordingAccountBackend) SaveRawDraft(raw []byte) (uint32, string, error) {
	b.rawDraftCalls = append(b.rawDraftCalls, string(raw))
	return 502, "Drafts", nil
}

func (b *recordingAccountBackend) DeleteDraft(uid uint32, folder string) error {
	b.draftDeletes = append(b.draftDeletes, fmt.Sprintf("%s:%d", folder, uid))
	return nil
}

func (b *recordingAccountBackend) SendDraft(uid uint32, folder string) error {
	b.draftSends = append(b.draftSends, fmt.Sprintf("%s:%d", folder, uid))
	return nil
}

func (b *recordingAccountBackend) FetchEmailBody(folder string, uid uint32) (*models.EmailBody, error) {
	if body, ok := b.bodies[fmt.Sprintf("%s:%d", folder, uid)]; ok {
		return body, nil
	}
	return nil, fmt.Errorf("missing body for %s:%d", folder, uid)
}

func (b *recordingAccountBackend) GetMessage(ctx context.Context, ref models.MessageRef) (MessageReadResult, error) {
	b.getMessageRefs = append(b.getMessageRefs, ref)
	body, ok := b.bodies[fmt.Sprintf("%s:%d", ref.Folder, ref.UID)]
	if !ok {
		return MessageReadResult{}, fmt.Errorf("missing message for %s:%d", ref.Folder, ref.UID)
	}
	return MessageReadResult{Body: body, Source: MessageReadSourceProvider}, nil
}

func (b *recordingAccountBackend) CalendarAgendaAvailable() bool {
	return len(b.calendarEvents) > 0
}

func (b *recordingAccountBackend) ListCalendarAgenda(start, end time.Time) ([]models.CalendarEvent, error) {
	out := make([]models.CalendarEvent, 0, len(b.calendarEvents))
	for _, event := range b.calendarEvents {
		if !calendarEventInRange(event, start, end) {
			continue
		}
		out = append(out, event)
	}
	sortCalendarEvents(out)
	return out, nil
}

func (b *recordingAccountBackend) GetCalendarEvent(ref models.EventRef) (*models.CalendarEvent, error) {
	ref = ref.WithDefaults()
	for _, event := range b.calendarEvents {
		if event.Ref.WithDefaults().LocalID == ref.LocalID {
			got := event
			return &got, nil
		}
	}
	return nil, fmt.Errorf("missing calendar event %s", ref.LocalID)
}

func (b *recordingAccountBackend) SearchCalendarEvents(query string, start, end time.Time) ([]models.CalendarEvent, error) {
	b.calendarSearch = append(b.calendarSearch, query)
	query = strings.ToLower(strings.TrimSpace(query))
	out := make([]models.CalendarEvent, 0, len(b.calendarEvents))
	for _, event := range b.calendarEvents {
		haystack := strings.ToLower(event.Title + " " + event.Description + " " + event.Location + " " + string(event.Ref.SourceID))
		if query != "" && strings.Contains(haystack, query) && calendarEventInRange(event, start, end) {
			out = append(out, event)
		}
	}
	sortCalendarEvents(out)
	return out, nil
}

func (b *recordingAccountBackend) SaveCalendarEvent(event models.CalendarEvent) (*models.CalendarEvent, error) {
	event.Ref = event.Ref.WithDefaults()
	b.savedCalendarEvents = append(b.savedCalendarEvents, event)
	for i := range b.calendarEvents {
		if b.calendarEvents[i].Ref.WithDefaults().LocalID == event.Ref.LocalID {
			b.calendarEvents[i] = event
			saved := event
			return &saved, nil
		}
	}
	return nil, fmt.Errorf("missing calendar event %s", event.Ref.LocalID)
}

func (b *recordingAccountBackend) CreateCalendarEvent(event models.CalendarEvent) (*models.CalendarEvent, error) {
	event.Ref = event.Ref.WithDefaults()
	b.createdCalendarEvents = append(b.createdCalendarEvents, event)
	b.calendarEvents = append(b.calendarEvents, event)
	saved := event
	return &saved, nil
}

func (b *recordingAccountBackend) FindCalendarEventByUID(ref models.CollectionRef, uid string) (*models.CalendarEvent, error) {
	ref.Kind = models.SourceKindCalendar
	ref.SourceID = models.NormalizeSourceID(ref.SourceID, models.DefaultCalendarSourceID)
	ref.AccountID = models.NormalizeAccountID(ref.AccountID)
	b.uidLookupRefs = append(b.uidLookupRefs, ref)
	b.uidLookupUIDs = append(b.uidLookupUIDs, uid)
	for _, event := range b.calendarEvents {
		event.Ref = event.Ref.WithDefaults()
		if event.Ref.SourceID == ref.SourceID && event.Ref.AccountID == ref.AccountID &&
			event.Ref.CalendarID == ref.CollectionID && strings.TrimSpace(event.ProviderUID) == strings.TrimSpace(uid) {
			found := event
			return &found, nil
		}
	}
	return nil, nil
}

func (b *recordingAccountBackend) RespondCalendarEvent(ref models.EventRef, status string) (*models.CalendarEvent, error) {
	ref = ref.WithDefaults()
	b.rsvpCalendarRefs = append(b.rsvpCalendarRefs, ref)
	b.rsvpCalendarStatus = append(b.rsvpCalendarStatus, status)
	for i := range b.calendarEvents {
		if b.calendarEvents[i].Ref.WithDefaults().LocalID != ref.LocalID {
			continue
		}
		if len(b.calendarEvents[i].Attendees) == 0 {
			b.calendarEvents[i].Attendees = []models.CalendarAttendee{{Name: "Me", Email: "me@example.com"}}
		}
		b.calendarEvents[i].Attendees[0].RSVP = status
		saved := b.calendarEvents[i]
		return &saved, nil
	}
	return nil, fmt.Errorf("missing calendar event %s", ref.LocalID)
}

func (b *recordingAccountBackend) Close() error {
	b.closed = true
	return nil
}

func TestMultiBackendAccountsAndActiveSwitching(t *testing.T) {
	work := newRecordingAccountBackend("work", []string{"INBOX", "Clients"}, nil, "")
	personal := newRecordingAccountBackend("personal", []string{"INBOX", "Travel"}, nil, "")

	mb, err := NewMultiBackend([]AccountBackend{
		{Info: AccountInfo{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail", Provider: "imap"}, Backend: work},
		{Info: AccountInfo{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal", Provider: "imap"}, Backend: personal},
	})
	if err != nil {
		t.Fatalf("NewMultiBackend: %v", err)
	}

	if !mb.HasMultipleAccounts() {
		t.Fatal("expected multi-account backend to report multiple accounts")
	}
	if got := mb.ActiveAccount().SourceID; got != models.SourceID("work-mail") {
		t.Fatalf("active source = %q, want work-mail", got)
	}
	if folders, _ := mb.ListFolders(); !reflect.DeepEqual(folders, []string{"INBOX", "Clients"}) {
		t.Fatalf("initial folders = %#v, want work folders", folders)
	}

	if err := mb.SwitchAccount(models.SourceID("personal-mail")); err != nil {
		t.Fatalf("SwitchAccount: %v", err)
	}
	if got := mb.ActiveAccount().SourceID; got != models.SourceID("personal-mail") {
		t.Fatalf("active source = %q, want personal-mail", got)
	}
	if folders, _ := mb.ListFolders(); !reflect.DeepEqual(folders, []string{"INBOX", "Travel"}) {
		t.Fatalf("switched folders = %#v, want personal folders", folders)
	}
}

func TestMultiBackendAccountFolderSnapshotsKeepSameNamedFoldersScoped(t *testing.T) {
	work := newRecordingAccountBackend("work", []string{"INBOX", "Drafts", "Clients"}, nil, "")
	personal := newRecordingAccountBackend("personal", []string{"INBOX", "Sent", "Travel"}, nil, "")
	work.status["INBOX"] = models.FolderStatus{Unseen: 7, Total: 23}
	personal.status["INBOX"] = models.FolderStatus{Unseen: 4, Total: 31}

	mb, err := NewMultiBackend([]AccountBackend{
		{Info: AccountInfo{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail", Provider: "imap"}, Backend: work},
		{Info: AccountInfo{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal", Provider: "imap"}, Backend: personal},
	})
	if err != nil {
		t.Fatalf("NewMultiBackend: %v", err)
	}

	snapshots, err := mb.ListAccountFolderSnapshots()
	if err != nil {
		t.Fatalf("ListAccountFolderSnapshots: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("snapshots=%d, want 2", len(snapshots))
	}
	if snapshots[0].Account.SourceID != "work-mail" || !reflect.DeepEqual(snapshots[0].Folders, []string{"INBOX", "Drafts", "Clients"}) {
		t.Fatalf("work snapshot = %#v", snapshots[0])
	}
	if got := snapshots[0].Status["INBOX"]; got.Unseen != 7 || got.Total != 23 {
		t.Fatalf("work INBOX status = %+v, want 7/23", got)
	}
	if snapshots[1].Account.SourceID != "personal-mail" || !reflect.DeepEqual(snapshots[1].Folders, []string{"INBOX", "Sent", "Travel"}) {
		t.Fatalf("personal snapshot = %#v", snapshots[1])
	}
	if got := snapshots[1].Status["INBOX"]; got.Unseen != 4 || got.Total != 31 {
		t.Fatalf("personal INBOX status = %+v, want 4/31", got)
	}

	if folders, _ := mb.ListFolders(); !reflect.DeepEqual(folders, []string{"INBOX", "Drafts", "Clients"}) {
		t.Fatalf("legacy active folders changed = %#v", folders)
	}
}

func TestAccountInfoFromSourceCarriesComposeSignature(t *testing.T) {
	source := config.SourceConfig{
		ID:          "work-mail",
		Kind:        "mail",
		Provider:    "imap",
		DisplayName: "Work Mail",
		AccountID:   "work",
		Credentials: config.CredentialsConfig{Username: "work@example.test"},
		Compose:     config.ComposeConfig{Signature: config.SignatureConfig{Text: "-- \nWork Signature\n\n"}},
	}

	info := accountInfoFromSource(source)
	if got := info.Signature; got != "-- \nWork Signature" {
		t.Fatalf("signature = %q, want trimmed work signature", got)
	}
}

func TestConfigForMailSourceKeepsExplicitMailSourceWithoutCalendars(t *testing.T) {
	source := config.SourceConfig{
		ID:          "proton-mail",
		Kind:        string(models.SourceKindMail),
		Provider:    "protonmail",
		DisplayName: "Proton",
		AccountID:   "proton",
		Credentials: config.CredentialsConfig{Username: "me@example.test", Password: "bridge-password"},
		IMAP:        config.ServerConfig{Host: "127.0.0.1", Port: 1143},
		SMTP:        config.ServerConfig{Host: "127.0.0.1", Port: 1025},
	}
	profile := &config.Config{Sources: []config.SourceConfig{source}}

	child := configForMailSource(profile, "proton.yaml", source)
	sources := child.NormalizedSources()
	if len(sources) != 1 {
		t.Fatalf("normalized child sources = %d, want 1", len(sources))
	}
	if got := sources[0].ID; got != "proton-mail" {
		t.Fatalf("child source id = %q, want proton-mail", got)
	}
	if got := sources[0].AccountID; got != "proton" {
		t.Fatalf("child account id = %q, want proton", got)
	}
}

func TestMultiBackendDuplicateFoldersAndMessageIDsStayActiveAccountScoped(t *testing.T) {
	workEmail := &models.EmailData{SourceID: "work-mail", AccountID: "work", MessageID: "same-message", UID: 42, Folder: "INBOX"}
	personalEmail := &models.EmailData{SourceID: "personal-mail", AccountID: "personal", MessageID: "same-message", UID: 42, Folder: "INBOX"}
	work := newRecordingAccountBackend("work", []string{"INBOX"}, workEmail, "work body")
	personal := newRecordingAccountBackend("personal", []string{"INBOX"}, personalEmail, "personal body")

	mb, err := NewMultiBackend([]AccountBackend{
		{Info: AccountInfo{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"}, Backend: work},
		{Info: AccountInfo{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"}, Backend: personal},
	})
	if err != nil {
		t.Fatalf("NewMultiBackend: %v", err)
	}

	body, err := mb.FetchEmailBody("INBOX", 42)
	if err != nil {
		t.Fatalf("FetchEmailBody work: %v", err)
	}
	if body.TextPlain != "work body" {
		t.Fatalf("work body = %q, want work body", body.TextPlain)
	}
	if err := mb.DeleteEmail("same-message", "INBOX"); err != nil {
		t.Fatalf("DeleteEmail work: %v", err)
	}
	if got := work.deleteCalls; !reflect.DeepEqual(got, []string{"INBOX:same-message"}) {
		t.Fatalf("work delete calls = %#v", got)
	}
	if len(personal.deleteCalls) != 0 {
		t.Fatalf("personal delete calls before switch = %#v, want none", personal.deleteCalls)
	}

	if err := mb.SwitchAccount("personal-mail"); err != nil {
		t.Fatalf("SwitchAccount: %v", err)
	}
	body, err = mb.FetchEmailBody("INBOX", 42)
	if err != nil {
		t.Fatalf("FetchEmailBody personal: %v", err)
	}
	if body.TextPlain != "personal body" {
		t.Fatalf("personal body = %q, want personal body", body.TextPlain)
	}
	if err := mb.DeleteEmail("same-message", "INBOX"); err != nil {
		t.Fatalf("DeleteEmail personal: %v", err)
	}
	if got := personal.deleteCalls; !reflect.DeepEqual(got, []string{"INBOX:same-message"}) {
		t.Fatalf("personal delete calls = %#v", got)
	}
}

func TestMultiBackendLegacySendEmailRoutesByFromAddress(t *testing.T) {
	work := newRecordingAccountBackend("work", []string{"INBOX"}, nil, "")
	personal := newRecordingAccountBackend("personal", []string{"INBOX"}, nil, "")

	mb, err := NewMultiBackend([]AccountBackend{
		{Info: AccountInfo{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail", Address: "work@example.test"}, Backend: work},
		{Info: AccountInfo{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal", Address: "me@example.test"}, Backend: personal},
	})
	if err != nil {
		t.Fatalf("NewMultiBackend: %v", err)
	}
	if err := mb.SwitchAccount(AllAccountsSourceID); err != nil {
		t.Fatalf("SwitchAccount all accounts: %v", err)
	}

	if err := mb.SendEmail("friend@example.test", "Hello", "body", "me@example.test"); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if len(work.sendCalls) != 0 {
		t.Fatalf("work send calls=%#v, want none", work.sendCalls)
	}
	if got := personal.sendCalls; !reflect.DeepEqual(got, []string{"me@example.test|friend@example.test|Hello|body"}) {
		t.Fatalf("personal send calls=%#v", got)
	}
}

func TestMultiBackendComposeSendRoutesBySelectedSource(t *testing.T) {
	work := newRecordingAccountBackend("work", []string{"INBOX"}, nil, "")
	personal := newRecordingAccountBackend("personal", []string{"INBOX"}, nil, "")

	mb, err := NewMultiBackend([]AccountBackend{
		{Info: AccountInfo{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail", Address: "work@example.test"}, Backend: work},
		{Info: AccountInfo{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal", Address: "me@example.test"}, Backend: personal},
	})
	if err != nil {
		t.Fatalf("NewMultiBackend: %v", err)
	}
	if err := mb.SwitchAccount(AllAccountsSourceID); err != nil {
		t.Fatalf("SwitchAccount all accounts: %v", err)
	}

	err = mb.SendCompose(ComposeSendRequest{
		SourceID:     "personal-mail",
		From:         "me@example.test",
		To:           "friend@example.test",
		CC:           "copy@example.test",
		Subject:      "Selected account",
		MarkdownBody: "hello",
	})
	if err != nil {
		t.Fatalf("SendCompose: %v", err)
	}
	if len(work.composeSends) != 0 {
		t.Fatalf("work compose sends=%#v, want none", work.composeSends)
	}
	if len(personal.composeSends) != 1 {
		t.Fatalf("personal compose sends=%#v, want one", personal.composeSends)
	}
	if got := personal.composeSends[0].SourceID; got != models.SourceID("personal-mail") {
		t.Fatalf("compose source=%q, want personal-mail", got)
	}
}

func TestMultiBackendDraftOperationsRouteBySelectedSource(t *testing.T) {
	work := newRecordingAccountBackend("work", []string{"INBOX"}, nil, "")
	personal := newRecordingAccountBackend("personal", []string{"INBOX"}, nil, "")

	mb, err := NewMultiBackend([]AccountBackend{
		{Info: AccountInfo{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"}, Backend: work},
		{Info: AccountInfo{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"}, Backend: personal},
	})
	if err != nil {
		t.Fatalf("NewMultiBackend: %v", err)
	}

	uid, folder, err := mb.SaveDraftForAccount("personal-mail", "to@example.test", "cc@example.test", "", "Draft", "body")
	if err != nil {
		t.Fatalf("SaveDraftForAccount: %v", err)
	}
	if uid != 501 || folder != "Drafts" {
		t.Fatalf("saved draft uid/folder=%d/%s, want 501/Drafts", uid, folder)
	}
	if len(work.saveDraftCalls) != 0 {
		t.Fatalf("work save draft calls=%#v, want none", work.saveDraftCalls)
	}
	if got := personal.saveDraftCalls; !reflect.DeepEqual(got, []string{"to@example.test|cc@example.test||Draft|body"}) {
		t.Fatalf("personal save draft calls=%#v", got)
	}

	if _, _, err := mb.SaveRawDraftForAccount("personal-mail", []byte("raw draft")); err != nil {
		t.Fatalf("SaveRawDraftForAccount: %v", err)
	}
	if err := mb.DeleteDraftForAccount("personal-mail", 501, "Drafts"); err != nil {
		t.Fatalf("DeleteDraftForAccount: %v", err)
	}
	if err := mb.SendDraftForAccount("personal-mail", 501, "Drafts"); err != nil {
		t.Fatalf("SendDraftForAccount: %v", err)
	}
	if len(work.rawDraftCalls) != 0 || len(work.draftDeletes) != 0 || len(work.draftSends) != 0 {
		t.Fatalf("work draft calls raw=%#v delete=%#v send=%#v, want none", work.rawDraftCalls, work.draftDeletes, work.draftSends)
	}
	if got := personal.rawDraftCalls; !reflect.DeepEqual(got, []string{"raw draft"}) {
		t.Fatalf("personal raw draft calls=%#v", got)
	}
	if got := personal.draftDeletes; !reflect.DeepEqual(got, []string{"Drafts:501"}) {
		t.Fatalf("personal draft deletes=%#v", got)
	}
	if got := personal.draftSends; !reflect.DeepEqual(got, []string{"Drafts:501"}) {
		t.Fatalf("personal draft sends=%#v", got)
	}
}

func TestMultiBackendSaveDraftForAccountUsesSelectedAccountAddress(t *testing.T) {
	work := newRecordingAccountBackend("work", []string{"INBOX"}, nil, "")
	personal := newRecordingAccountBackend("personal", []string{"INBOX"}, nil, "")

	mb, err := NewMultiBackend([]AccountBackend{
		{Info: AccountInfo{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail", Address: "work@example.test"}, Backend: work},
		{Info: AccountInfo{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal", Address: "me@example.test"}, Backend: personal},
	})
	if err != nil {
		t.Fatalf("NewMultiBackend: %v", err)
	}

	uid, folder, err := mb.SaveDraftForAccount("personal-mail", "friend@example.test", "", "", "Draft identity", "body")
	if err != nil {
		t.Fatalf("SaveDraftForAccount: %v", err)
	}
	if uid != 502 || folder != "Drafts" {
		t.Fatalf("saved draft uid/folder=%d/%s, want 502/Drafts from raw draft path", uid, folder)
	}
	if len(work.rawDraftCalls) != 0 || len(work.saveDraftCalls) != 0 {
		t.Fatalf("work draft calls raw=%#v save=%#v, want none", work.rawDraftCalls, work.saveDraftCalls)
	}
	if len(personal.saveDraftCalls) != 0 {
		t.Fatalf("personal SaveDraft calls=%#v, want raw draft with selected account From", personal.saveDraftCalls)
	}
	if len(personal.rawDraftCalls) != 1 {
		t.Fatalf("personal raw draft calls=%#v, want one", personal.rawDraftCalls)
	}
	if !strings.Contains(personal.rawDraftCalls[0], "From: me@example.test\r\n") {
		t.Fatalf("raw draft missing selected account From header:\n%s", personal.rawDraftCalls[0])
	}
}

func TestMultiBackendAllAccountsTimelineAggregatesAndKeepsDuplicateIDsScoped(t *testing.T) {
	workEmail := scopedTestEmail(&models.EmailData{SourceID: "work-mail", AccountID: "work", MessageID: "same-message", UID: 11, Folder: "INBOX", Subject: "Work", Date: time.Date(2026, 5, 23, 13, 0, 0, 0, time.UTC)})
	personalEmail := scopedTestEmail(&models.EmailData{SourceID: "personal-mail", AccountID: "personal", MessageID: "same-message", UID: 22, Folder: "INBOX", Subject: "Personal", Date: time.Date(2026, 5, 23, 14, 0, 0, 0, time.UTC)})
	work := newRecordingAccountBackend("work", []string{"INBOX"}, workEmail, "work body")
	personal := newRecordingAccountBackend("personal", []string{"INBOX"}, personalEmail, "personal body")

	mb, err := NewMultiBackend([]AccountBackend{
		{Info: AccountInfo{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"}, Backend: work},
		{Info: AccountInfo{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"}, Backend: personal},
	})
	if err != nil {
		t.Fatalf("NewMultiBackend: %v", err)
	}
	if err := mb.SwitchAccount(AllAccountsSourceID); err != nil {
		t.Fatalf("SwitchAccount all accounts: %v", err)
	}

	emails, err := mb.GetTimelineEmails("INBOX")
	if err != nil {
		t.Fatalf("GetTimelineEmails all accounts: %v", err)
	}
	if len(emails) != 2 {
		t.Fatalf("email count=%d, want 2", len(emails))
	}
	if emails[0].SourceID != "personal-mail" || emails[1].SourceID != "work-mail" {
		t.Fatalf("emails not sorted newest-first with source identity: %#v", emails)
	}
	if emails[0].MessageID != emails[1].MessageID {
		t.Fatalf("test setup expected duplicate message IDs, got %q and %q", emails[0].MessageID, emails[1].MessageID)
	}
	if emails[0].MessageRef().LocalID == emails[1].MessageRef().LocalID {
		t.Fatalf("duplicate message IDs must keep distinct local IDs: %q", emails[0].MessageRef().LocalID)
	}
}

func TestMultiBackendAllAccountsSearchAggregatesVisibleAccounts(t *testing.T) {
	workEmail := scopedTestEmail(&models.EmailData{SourceID: "work-mail", AccountID: "work", MessageID: "work-1", UID: 11, Folder: "INBOX", Subject: "roadmap", Date: time.Date(2026, 5, 23, 13, 0, 0, 0, time.UTC)})
	personalEmail := scopedTestEmail(&models.EmailData{SourceID: "personal-mail", AccountID: "personal", MessageID: "personal-1", UID: 22, Folder: "INBOX", Subject: "roadmap", Date: time.Date(2026, 5, 23, 14, 0, 0, 0, time.UTC)})
	work := newRecordingAccountBackend("work", []string{"INBOX"}, workEmail, "work body")
	personal := newRecordingAccountBackend("personal", []string{"INBOX"}, personalEmail, "personal body")

	mb, err := NewMultiBackend([]AccountBackend{
		{Info: AccountInfo{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"}, Backend: work},
		{Info: AccountInfo{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"}, Backend: personal},
	})
	if err != nil {
		t.Fatalf("NewMultiBackend: %v", err)
	}
	if err := mb.SwitchAccount(AllAccountsSourceID); err != nil {
		t.Fatalf("SwitchAccount all accounts: %v", err)
	}

	emails, err := mb.SearchEmails("INBOX", "roadmap", false)
	if err != nil {
		t.Fatalf("SearchEmails all accounts: %v", err)
	}
	if len(emails) != 2 {
		t.Fatalf("search count=%d, want 2", len(emails))
	}
	if emails[0].SourceID != "personal-mail" || emails[1].SourceID != "work-mail" {
		t.Fatalf("search results not newest-first with source identity: %#v", emails)
	}
}

func TestMultiBackendActiveTimelineAndSearchResultsAreScoped(t *testing.T) {
	workEmail := &models.EmailData{MessageID: "same-message", UID: 11, Folder: "INBOX", Subject: "roadmap", Date: time.Date(2026, 5, 23, 13, 0, 0, 0, time.UTC)}
	personalEmail := &models.EmailData{MessageID: "same-message", UID: 22, Folder: "INBOX", Subject: "roadmap", Date: time.Date(2026, 5, 23, 14, 0, 0, 0, time.UTC)}
	work := newRecordingAccountBackend("work", []string{"INBOX"}, workEmail, "work body")
	personal := newRecordingAccountBackend("personal", []string{"INBOX"}, personalEmail, "personal body")

	mb, err := NewMultiBackend([]AccountBackend{
		{Info: AccountInfo{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"}, Backend: work},
		{Info: AccountInfo{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"}, Backend: personal},
	})
	if err != nil {
		t.Fatalf("NewMultiBackend: %v", err)
	}
	if err := mb.SwitchAccount("personal-mail"); err != nil {
		t.Fatalf("SwitchAccount: %v", err)
	}

	timeline, err := mb.GetTimelineEmails("INBOX")
	if err != nil {
		t.Fatalf("GetTimelineEmails: %v", err)
	}
	if len(timeline) != 1 {
		t.Fatalf("timeline len=%d, want 1", len(timeline))
	}
	if timeline[0].SourceID != "personal-mail" || timeline[0].AccountID != "personal" {
		t.Fatalf("timeline result scope=(%q,%q), want personal-mail/personal", timeline[0].SourceID, timeline[0].AccountID)
	}
	if timeline[0].MessageRef().SourceID != "personal-mail" {
		t.Fatalf("timeline message ref = %#v, want personal-mail source", timeline[0].MessageRef())
	}

	search, err := mb.SearchEmails("INBOX", "roadmap", false)
	if err != nil {
		t.Fatalf("SearchEmails: %v", err)
	}
	if len(search) != 1 {
		t.Fatalf("search len=%d, want 1", len(search))
	}
	if search[0].SourceID != "personal-mail" || search[0].AccountID != "personal" {
		t.Fatalf("search result scope=(%q,%q), want personal-mail/personal", search[0].SourceID, search[0].AccountID)
	}
	if err := mb.ArchiveEmailByRef(search[0].MessageRef()); err != nil {
		t.Fatalf("ArchiveEmailByRef with active search ref: %v", err)
	}
	if got := personal.archiveCalls; !reflect.DeepEqual(got, []string{"INBOX:same-message"}) {
		t.Fatalf("personal archive calls=%#v", got)
	}
	if len(work.archiveCalls) != 0 {
		t.Fatalf("work archive calls=%#v, want none", work.archiveCalls)
	}
}

func TestMultiBackendScopedMessageReadAndMutationRouteToSource(t *testing.T) {
	workEmail := scopedTestEmail(&models.EmailData{SourceID: "work-mail", AccountID: "work", MessageID: "same-message", UID: 11, Folder: "INBOX", Subject: "Work"})
	personalEmail := scopedTestEmail(&models.EmailData{SourceID: "personal-mail", AccountID: "personal", MessageID: "same-message", UID: 22, Folder: "INBOX", Subject: "Personal"})
	work := newRecordingAccountBackend("work", []string{"INBOX"}, workEmail, "work body")
	personal := newRecordingAccountBackend("personal", []string{"INBOX"}, personalEmail, "personal body")

	mb, err := NewMultiBackend([]AccountBackend{
		{Info: AccountInfo{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"}, Backend: work},
		{Info: AccountInfo{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"}, Backend: personal},
	})
	if err != nil {
		t.Fatalf("NewMultiBackend: %v", err)
	}
	if err := mb.SwitchAccount(AllAccountsSourceID); err != nil {
		t.Fatalf("SwitchAccount all accounts: %v", err)
	}

	read, err := mb.GetMessage(context.Background(), personalEmail.MessageRef())
	if err != nil {
		t.Fatalf("GetMessage personal ref: %v", err)
	}
	if read.Body.TextPlain != "personal body" {
		t.Fatalf("read body=%q, want personal body", read.Body.TextPlain)
	}
	if len(work.getMessageRefs) != 0 || len(personal.getMessageRefs) != 1 {
		t.Fatalf("GetMessage routed to wrong backend: work=%d personal=%d", len(work.getMessageRefs), len(personal.getMessageRefs))
	}

	if err := mb.ArchiveEmailByRef(personalEmail.MessageRef()); err != nil {
		t.Fatalf("ArchiveEmailByRef personal: %v", err)
	}
	if got := personal.archiveCalls; !reflect.DeepEqual(got, []string{"INBOX:same-message"}) {
		t.Fatalf("personal archive calls=%#v", got)
	}
	if len(work.archiveCalls) != 0 {
		t.Fatalf("work archive calls=%#v, want none", work.archiveCalls)
	}

	if err := mb.MoveEmailByRef(personalEmail.MessageRef(), "Later"); err != nil {
		t.Fatalf("MoveEmailByRef personal: %v", err)
	}
	if got := personal.moveCalls; !reflect.DeepEqual(got, []string{"INBOX:same-message:Later"}) {
		t.Fatalf("personal move calls=%#v", got)
	}
	if len(work.moveCalls) != 0 {
		t.Fatalf("work move calls=%#v, want none", work.moveCalls)
	}

	if err := mb.DeleteEmailByRef(workEmail.MessageRef()); err != nil {
		t.Fatalf("DeleteEmailByRef work: %v", err)
	}
	if got := work.deleteCalls; !reflect.DeepEqual(got, []string{"INBOX:same-message"}) {
		t.Fatalf("work delete calls=%#v", got)
	}
	if len(personal.deleteCalls) != 0 {
		t.Fatalf("personal delete calls=%#v, want none", personal.deleteCalls)
	}
}

func TestMultiBackendDeleteEmailsByRefBatchesByScopedAccount(t *testing.T) {
	workFirst := scopedTestEmail(&models.EmailData{SourceID: "work-mail", AccountID: "work", MessageID: "work-1", UID: 11, Folder: "INBOX", Subject: "Work 1"})
	workSecond := scopedTestEmail(&models.EmailData{SourceID: "work-mail", AccountID: "work", MessageID: "work-2", UID: 12, Folder: "INBOX", Subject: "Work 2"})
	personalEmail := scopedTestEmail(&models.EmailData{SourceID: "personal-mail", AccountID: "personal", MessageID: "personal-1", UID: 21, Folder: "INBOX", Subject: "Personal"})
	work := newRecordingAccountBackend("work", []string{"INBOX"}, workFirst, "")
	personal := newRecordingAccountBackend("personal", []string{"INBOX"}, personalEmail, "")
	mb, err := NewMultiBackend([]AccountBackend{
		{Info: AccountInfo{SourceID: "work-mail", AccountID: "work", DisplayName: "Work"}, Backend: work},
		{Info: AccountInfo{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"}, Backend: personal},
	})
	if err != nil {
		t.Fatalf("NewMultiBackend: %v", err)
	}

	err = mb.DeleteEmailsByRef([]models.MessageRef{
		workFirst.MessageRef(),
		personalEmail.MessageRef(),
		workSecond.MessageRef(),
	})
	if err != nil {
		t.Fatalf("DeleteEmailsByRef: %v", err)
	}

	if got := len(work.deleteBatchRefs); got != 1 {
		t.Fatalf("work batch calls = %d, want 1", got)
	}
	if got := len(personal.deleteBatchRefs); got != 1 {
		t.Fatalf("personal batch calls = %d, want 1", got)
	}
	if got := work.deleteBatchRefs[0]; !reflect.DeepEqual(got, []models.MessageRef{workFirst.MessageRef(), workSecond.MessageRef()}) {
		t.Fatalf("work batch refs = %#v", got)
	}
	if got := personal.deleteBatchRefs[0]; !reflect.DeepEqual(got, []models.MessageRef{personalEmail.MessageRef()}) {
		t.Fatalf("personal batch refs = %#v", got)
	}
	if len(work.deleteCalls) != 0 || len(personal.deleteCalls) != 0 {
		t.Fatalf("expected no single-delete fallback, got work=%#v personal=%#v", work.deleteCalls, personal.deleteCalls)
	}
}

func TestMultiBackendGetMessagePreviewFallsBackToLegacyFetch(t *testing.T) {
	workEmail := scopedTestEmail(&models.EmailData{SourceID: "work-mail", AccountID: "work", MessageID: "same-message", UID: 11, Folder: "INBOX", Subject: "Work"})
	personalEmail := scopedTestEmail(&models.EmailData{SourceID: "personal-mail", AccountID: "personal", MessageID: "same-message", UID: 22, Folder: "INBOX", Subject: "Personal"})
	work := newRecordingAccountBackend("work", []string{"INBOX"}, workEmail, "work body")
	personal := newRecordingAccountBackend("personal", []string{"INBOX"}, personalEmail, "personal body")

	mb, err := NewMultiBackend([]AccountBackend{
		{Info: AccountInfo{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"}, Backend: work},
		{Info: AccountInfo{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"}, Backend: personal},
	})
	if err != nil {
		t.Fatalf("NewMultiBackend: %v", err)
	}
	if err := mb.SwitchAccount(AllAccountsSourceID); err != nil {
		t.Fatalf("SwitchAccount all accounts: %v", err)
	}

	read, err := mb.GetMessagePreview(context.Background(), personalEmail.MessageRef(), MessageReadIntent{ViewID: "timeline-preview"})
	if err != nil {
		t.Fatalf("GetMessagePreview legacy fallback: %v", err)
	}
	if read.Body == nil || read.Body.TextPlain != "personal body" {
		t.Fatalf("preview body = %#v, want personal body", read.Body)
	}
	if read.Source != MessageReadSourceProvider {
		t.Fatalf("preview source = %q, want %q", read.Source, MessageReadSourceProvider)
	}
}

func TestMultiBackendCloseClosesAllAccountBackends(t *testing.T) {
	work := newRecordingAccountBackend("work", []string{"INBOX"}, nil, "")
	personal := newRecordingAccountBackend("personal", []string{"INBOX"}, nil, "")

	mb, err := NewMultiBackend([]AccountBackend{
		{Info: AccountInfo{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"}, Backend: work},
		{Info: AccountInfo{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"}, Backend: personal},
	})
	if err != nil {
		t.Fatalf("NewMultiBackend: %v", err)
	}
	if err := mb.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !work.closed || !personal.closed {
		t.Fatalf("Close did not close all accounts: work=%v personal=%v", work.closed, personal.closed)
	}
}

func TestMultiBackendAggregatesScopedEmbeddingRefsAcrossAllAccounts(t *testing.T) {
	workRef := models.MessageRef{SourceID: "work-mail", AccountID: "work", Folder: "INBOX", MessageID: "work-embed"}.WithDefaults()
	personalRef := models.MessageRef{SourceID: "personal-mail", AccountID: "personal", Folder: "INBOX", MessageID: "personal-embed"}.WithDefaults()
	work := newRecordingAccountBackend("work", []string{"INBOX"}, nil, "")
	personal := newRecordingAccountBackend("personal", []string{"INBOX"}, nil, "")
	work.embeddingRefs = []models.MessageRef{workRef}
	personal.embeddingRefs = []models.MessageRef{personalRef}

	mb, err := NewMultiBackend([]AccountBackend{
		{Info: AccountInfo{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"}, Backend: work},
		{Info: AccountInfo{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"}, Backend: personal},
	})
	if err != nil {
		t.Fatalf("NewMultiBackend: %v", err)
	}
	if err := mb.SwitchAccount(AllAccountsSourceID); err != nil {
		t.Fatalf("SwitchAccount(all): %v", err)
	}

	refs, err := mb.GetUnembeddedRefsWithBody("INBOX")
	if err != nil {
		t.Fatalf("GetUnembeddedRefsWithBody: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("refs len = %d, want 2: %#v", len(refs), refs)
	}
	if refs[0].SourceID != "work-mail" || refs[1].SourceID != "personal-mail" {
		t.Fatalf("refs not tagged by source: %#v", refs)
	}

	if err := mb.StoreEmbeddingChunksByRef(personalRef, []models.EmbeddingChunk{{ChunkIndex: 0, Embedding: []float32{1}}}); err != nil {
		t.Fatalf("StoreEmbeddingChunksByRef: %v", err)
	}
	if len(personal.storedEmbeddingRefs) != 1 || personal.storedEmbeddingRefs[0].LocalID != personalRef.LocalID {
		t.Fatalf("personal stored refs = %#v, want %s", personal.storedEmbeddingRefs, personalRef.LocalID)
	}
	if len(work.storedEmbeddingRefs) != 0 {
		t.Fatalf("work stored refs = %#v, want none", work.storedEmbeddingRefs)
	}
}
