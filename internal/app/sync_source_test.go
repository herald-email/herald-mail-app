package app

import (
	"testing"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestListenForNewEmailsPreservesSourceScope(t *testing.T) {
	ch := make(chan models.NewEmailsNotification, 1)
	m := Model{backend: &stubBackend{newEmailsCh: ch}}

	ch <- models.NewEmailsNotification{
		SourceID:  models.SourceID("work-mail"),
		AccountID: models.AccountID("work"),
		Folder:    "INBOX",
		Emails: []*models.EmailData{
			{SourceID: models.SourceID("work-mail"), AccountID: models.AccountID("work"), MessageID: "<m@example.com>"},
		},
	}

	msg := m.listenForNewEmails()()
	got, ok := msg.(NewEmailsMsg)
	if !ok {
		t.Fatalf("message type = %T, want NewEmailsMsg", msg)
	}
	if got.SourceID != models.SourceID("work-mail") || got.AccountID != models.AccountID("work") {
		t.Fatalf("source scope = %q/%q, want work-mail/work", got.SourceID, got.AccountID)
	}
	if got.Folder != "INBOX" || len(got.Emails) != 1 {
		t.Fatalf("message payload = %#v, want folder and email preserved", got)
	}
}
