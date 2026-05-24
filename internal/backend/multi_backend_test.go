package backend

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/models"
)

type recordingAccountBackend struct {
	*DemoBackend
	name        string
	folders     []string
	status      map[string]models.FolderStatus
	timeline    map[string][]*models.EmailData
	bodies      map[string]*models.EmailBody
	loadCalls   []string
	deleteCalls []string
	closed      bool
}

func newRecordingAccountBackend(name string, folders []string, email *models.EmailData, body string) *recordingAccountBackend {
	b := &recordingAccountBackend{
		DemoBackend: NewDemoBackend(),
		name:        name,
		folders:     folders,
		status:      make(map[string]models.FolderStatus),
		timeline:    make(map[string][]*models.EmailData),
		bodies:      make(map[string]*models.EmailBody),
	}
	for _, folder := range folders {
		b.status[folder] = models.FolderStatus{Unseen: 1, Total: 3}
	}
	if email != nil {
		b.timeline[email.Folder] = []*models.EmailData{email}
		b.bodies[fmt.Sprintf("%s:%d", email.Folder, email.UID)] = &models.EmailBody{
			MessageID: email.MessageID,
			TextPlain: body,
		}
	}
	return b
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

func (b *recordingAccountBackend) DeleteEmail(messageID, folder string) error {
	b.deleteCalls = append(b.deleteCalls, folder+":"+messageID)
	return nil
}

func (b *recordingAccountBackend) FetchEmailBody(folder string, uid uint32) (*models.EmailBody, error) {
	if body, ok := b.bodies[fmt.Sprintf("%s:%d", folder, uid)]; ok {
		return body, nil
	}
	return nil, fmt.Errorf("missing body for %s:%d", folder, uid)
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
